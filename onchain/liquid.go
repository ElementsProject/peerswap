package onchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/btcsuite/btcd/btcec"
	"log"

	"github.com/btcsuite/btcd/txscript"
	"github.com/sputn1ck/glightning/gelements"
	"github.com/sputn1ck/peerswap/lightning"
	"github.com/sputn1ck/peerswap/swap"
	"github.com/sputn1ck/peerswap/wallet"
	"github.com/vulpemventures/go-elements/address"
	address2 "github.com/vulpemventures/go-elements/address"
	"github.com/vulpemventures/go-elements/elementsutil"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/payment"
	"github.com/vulpemventures/go-elements/transaction"
)

const (
	LiquidCsv          = 60
	LiquidConfs        = 2
	LiquidTargetBlocks = 1
)

type LiquidOnChain struct {
	elements     *gelements.Elements
	liquidWallet wallet.Wallet
	network      *network.Network
	asset        []byte
}

func NewLiquidOnChain(elements *gelements.Elements, liquidWallet wallet.Wallet, network *network.Network) *LiquidOnChain {
	lbtc := append(
		[]byte{0x01},
		elementsutil.ReverseBytes(h2b(network.AssetID))...,
	)
	return &LiquidOnChain{elements: elements, liquidWallet: liquidWallet, network: network, asset: lbtc}
}

func (l *LiquidOnChain) CreateOpeningTransaction(swapParams *swap.OpeningParams) (string, uint64, uint32, error) {
	redeemScript, err := ParamsToTxScript(swapParams, LiquidCsv)
	if err != nil {
		return "", 0, 0, err
	}
	scriptPubKey := []byte{0x00, 0x20}
	witnessProgram := sha256.Sum256(redeemScript)
	scriptPubKey = append(scriptPubKey, witnessProgram[:]...)

	redeemPayment, _ := payment.FromScript(scriptPubKey, l.network, nil)
	sats, _ := elementsutil.SatoshiToElementsValue(swapParams.Amount)
	output := transaction.NewTxOutput(l.asset, sats, redeemPayment.WitnessScript)
	tx := transaction.NewTx(2)
	tx.Outputs = append(tx.Outputs, output)

	unpreparedTxHex, fee, err := l.liquidWallet.CreateFundedTransaction(tx)
	if err != nil {
		return "", 0, 0, err
	}

	vout, err := l.VoutFromTxHex(unpreparedTxHex, redeemScript)
	if err != nil {
		return "", 0, 0, err
	}

	return unpreparedTxHex, fee, vout, nil
}

func (l *LiquidOnChain) BroadcastOpeningTx(unpreparedTxHex string) (string, string, error) {
	txHex, err := l.liquidWallet.FinalizeFundedTransaction(unpreparedTxHex)
	if err != nil {
		return "", "", err
	}

	txId, err := l.elements.SendRawTx(txHex)
	if err != nil {
		return "", "", err
	}
	return txId, txHex, nil
}

func (l *LiquidOnChain) CreatePreimageSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams) (string, string, error) {
	newAddr, err := l.liquidWallet.GetAddress()
	if err != nil {
		return "", "", err
	}

	tx, sigBytes, redeemScript, err := l.prepareSpendingTransaction(swapParams, claimParams, newAddr, 0, 0)
	if err != nil {
		return "", "", err
	}

	preimage, err := lightning.MakePreimageFromStr(claimParams.Preimage)
	if err != nil {
		return "", "", err
	}

	tx.Inputs[0].Witness = GetPreimageWitness(sigBytes, preimage[:], redeemScript)

	txHex, err := tx.ToHex()
	if err != nil {
		return "", "", err
	}

	txId, err := l.elements.SendRawTx(txHex)
	if err != nil {
		return "", "", err
	}
	return txId, txHex, nil
}

func (l *LiquidOnChain) CreateCsvSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams) (txId, txHex string, error error) {
	newAddr, err := l.liquidWallet.GetAddress()
	if err != nil {
		return "", "", err
	}
	tx, sigBytes, redeemScript, err := l.prepareSpendingTransaction(swapParams, claimParams, newAddr, LiquidCsv, 0)
	if err != nil {
		return "", "", err
	}
	tx.Inputs[0].Witness = GetCsvWitness(sigBytes, redeemScript)
	txHex, err = tx.ToHex()
	if err != nil {
		return "", "", err
	}
	txId, err = l.elements.SendRawTx(txHex)
	if err != nil {
		return "", "", err
	}
	return txId, txHex, nil
}
func (l *LiquidOnChain) TakerCreateCoopSigHash(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, refundAddress string, refundFee uint64) (sigHash string, error error) {
	_, sigBytes, _, err := l.prepareSpendingTransaction(swapParams, claimParams, refundAddress, 0, refundFee)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sigBytes), nil
}

func (l *LiquidOnChain) CreateCooperativeSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, refundAddress string, vout uint32, takerSignatureHex string, refundFee uint64) (txId, txHex string, error error) {
	tx, sigBytes, redeemScript, err := l.prepareSpendingTransaction(swapParams, claimParams, refundAddress, 0, refundFee)
	if err != nil {
		return "", "", err
	}
	takerSigBytes, err := hex.DecodeString(takerSignatureHex)
	if err != nil {
		return "", "", err
	}

	tx.Inputs[0].Witness = GetCooperativeWitness(takerSigBytes, sigBytes, redeemScript)

	txHex, err = tx.ToHex()
	if err != nil {
		return "", "", err
	}
	txId, err = l.elements.SendRawTx(txHex)
	if err != nil {
		return "", "", err
	}
	return txId, txHex, nil
}

func (l *LiquidOnChain) NewAddress() (string, error) {
	addr, err := l.liquidWallet.GetAddress()
	if err != nil {
		return "", err
	}
	return addr, nil
}

func (l *LiquidOnChain) prepareSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, spendingAddr string, csv uint32, preparedFee uint64) (tx *transaction.Transaction, sigBytes, redeemScript []byte, err error) {
	outputScript, err := l.blech32ToScript(spendingAddr)
	if err != nil {
		return nil, nil, nil, err
	}
	redeemScript, err = ParamsToTxScript(swapParams, LiquidCsv)
	if err != nil {
		return nil, nil, nil, err
	}
	spendingTx, sigHash, err := l.createSpendingTransaction(claimParams.OpeningTxHex, swapParams.Amount, csv, l.asset, redeemScript, outputScript, preparedFee)
	if err != nil {
		return nil, nil, nil, err
	}
	sig, err := claimParams.Signer.Sign(sigHash[:])
	if err != nil {
		return nil, nil, nil, err
	}
	return spendingTx, sig.Serialize(), redeemScript, nil
}

// CreateSpendingTransaction returns the spendningTransaction for the swap
func (l *LiquidOnChain) createSpendingTransaction(openingTxHex string, swapAmount uint64, csv uint32, asset, redeemScript, outputScript []byte, preparedFee uint64) (tx *transaction.Transaction, sigHash [32]byte, err error) {
	firstTx, err := transaction.NewTxFromHex(openingTxHex)
	if err != nil {
		log.Printf("error creating first tx %s, %v", openingTxHex, err)
		return nil, [32]byte{}, err
	}

	swapInValue, err := elementsutil.SatoshiToElementsValue(swapAmount)
	if err != nil {
		log.Printf("error getting swapin value")
		return nil, [32]byte{}, err
	}
	vout, err := l.findVout(firstTx.Outputs, redeemScript)
	if err != nil {
		log.Printf("error finding vour")
		return nil, [32]byte{}, err
	}

	txHash := firstTx.TxHash()
	spendingInput := transaction.NewTxInput(txHash[:], vout)
	spendingInput.Sequence = 0 | csv
	feeAmountPlaceholder := uint64(500)
	spendingSatsBytes, _ := elementsutil.SatoshiToElementsValue(swapAmount - feeAmountPlaceholder)

	var txOutputs = []*transaction.TxOutput{}

	spendingOutput := transaction.NewTxOutput(asset, spendingSatsBytes[:], outputScript)
	txOutputs = append(txOutputs, spendingOutput)

	spendingTx := &transaction.Transaction{
		Version: 2,
		Flag:    0,
		Inputs:  []*transaction.TxInput{spendingInput},
		Outputs: txOutputs,
	}
	txSize := spendingTx.SerializeSize(true, true)
	fee := preparedFee
	if preparedFee == 0 {
		fee, err = l.getFee(txSize)
		if err != nil {
			fee = feeAmountPlaceholder
		}
	}
	log.Printf("txsize: %v fee: %v", txSize, fee)
	if fee > 0 {
		spendingSatsBytes, _ = elementsutil.SatoshiToElementsValue(swapAmount - fee)
		spendingTx.Outputs[0].Value = spendingSatsBytes
		feeValue, _ := elementsutil.SatoshiToElementsValue(fee)
		feeScript := []byte{}
		feeOutput := transaction.NewTxOutput(asset, feeValue, feeScript)
		spendingTx.Outputs = append(spendingTx.Outputs, feeOutput)
	}

	sigHash = spendingTx.HashForWitnessV0(
		0,
		redeemScript[:],
		swapInValue,
		txscript.SigHashAll,
	)
	return spendingTx, sigHash, nil
}

func (l *LiquidOnChain) TxIdFromHex(txHex string) (string, error) {
	openingTx, err := transaction.NewTxFromHex(txHex)
	if err != nil {
		return "", err
	}
	return openingTx.TxHash().String(), nil
}

func (l *LiquidOnChain) ValidateTx(swapParams *swap.OpeningParams, txHex string) (bool, error) {
	redeemScript, err := ParamsToTxScript(swapParams, LiquidCsv)
	if err != nil {
		return false, err
	}

	openingTx, err := transaction.NewTxFromHex(txHex)
	if err != nil {
		return false, err
	}

	vout, err := l.findVout(openingTx.Outputs, redeemScript)
	if err != nil {
		return false, err
	}

	swapInValue, err := elementsutil.SatoshiToElementsValue(swapParams.Amount)
	if err != nil {
		return false, err
	}

	if bytes.Compare(openingTx.Outputs[vout].Value, swapInValue) != 0 {
		return false, errors.New("swap value does not match tx value")
	}
	//todo check script
	return true, nil
}

func (l *LiquidOnChain) VoutFromTxHex(txHex string, redeemScript []byte) (uint32, error) {
	tx, err := transaction.NewTxFromHex(txHex)
	if err != nil {
		return 0, err
	}
	vout, err := l.findVout(tx.Outputs, redeemScript)
	if err != nil {
		return 0, err
	}
	return vout, nil
}

func (b *LiquidOnChain) GetOutputScript(params *swap.OpeningParams) ([]byte, error) {
	redeemScript, err := ParamsToTxScript(params, LiquidCsv)
	if err != nil {
		return nil, err
	}
	wantAddr, err := b.CreateOpeningAddress(redeemScript)
	if err != nil {
		return nil, err
	}
	wantBytes, err := address2.ToOutputScript(wantAddr)
	if err != nil {
		return nil, err
	}
	return wantBytes, nil
}

func (l *LiquidOnChain) findVout(outputs []*transaction.TxOutput, redeemScript []byte) (uint32, error) {
	wantAddr, err := l.CreateOpeningAddress(redeemScript)
	if err != nil {
		return 0, err
	}
	wantBytes, err := address2.ToOutputScript(wantAddr)
	if err != nil {
		return 0, err
	}

	for i, v := range outputs {
		if bytes.Compare(v.Script, wantBytes) == 0 {
			return uint32(i), nil
		}
	}
	return 0, errors.New("vout not found")
}

// creatOpeningAddress returns the address for the opening tx
func (l *LiquidOnChain) CreateOpeningAddress(redeemScript []byte) (string, error) {
	scriptPubKey := []byte{0x00, 0x20}
	witnessProgram := sha256.Sum256(redeemScript)
	scriptPubKey = append(scriptPubKey, witnessProgram[:]...)

	redeemPayment, err := payment.FromScript(scriptPubKey, l.network, nil)
	if err != nil {
		return "", err
	}
	addr, err := redeemPayment.WitnessScriptHash()
	if err != nil {
		return "", err
	}
	return addr, nil
}

// creatOpeningAddress returns the address for the opening tx
func (l *LiquidOnChain) CreateBlindedOpeningAddress(redeemScript []byte, blindingPubkey *btcec.PublicKey) (string, error) {
	scriptPubKey := []byte{0x00, 0x20}
	witnessProgram := sha256.Sum256(redeemScript)
	scriptPubKey = append(scriptPubKey, witnessProgram[:]...)

	redeemPayment, err := payment.FromScript(scriptPubKey, l.network, blindingPubkey)
	if err != nil {
		return "", err
	}
	addr, err := redeemPayment.ConfidentialWitnessScriptHash()
	if err != nil {
		return "", err
	}
	return addr, nil
}

// Blech32ToScript returns an elements script from a Blech32 Address
func (l *LiquidOnChain) blech32ToScript(blech32Addr string) ([]byte, error) {
	blechAddr, err := address.FromBlech32(blech32Addr)
	if err != nil {
		return nil, err
	}
	blechscript, err := txscript.NewScriptBuilder().AddOp(txscript.OP_0).AddData(blechAddr.Program).Script()
	if err != nil {
		return nil, err
	}
	blechPayment, err := payment.FromScript(blechscript[:], l.network, nil)
	if err != nil {
		return nil, err
	}
	return blechPayment.WitnessScript, nil
}

func (l *LiquidOnChain) getFee(txSize int) (uint64, error) {
	feeRes, err := l.elements.EstimateFee(LiquidTargetBlocks, "ECONOMICAL")
	if err != nil {
		return 0, err
	}
	satPerByte := float64(feeRes.SatPerKb()) / float64(1000)
	if satPerByte < 1 {
		satPerByte = 1
	}
	if len(feeRes.Errors) > 0 {
		//todo sane default sat per byte
		satPerByte = 1
	}
	// assume largest witness
	fee := satPerByte * float64(txSize)
	return uint64(fee), nil
}

func (l *LiquidOnChain) GetRefundFee() (uint64, error) {
	// todo get tx size
	return l.getFee(250)
}
