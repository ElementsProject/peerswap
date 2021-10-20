package onchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/btcsuite/btcd/txscript"
	"github.com/sputn1ck/glightning/gelements"
	"github.com/sputn1ck/peerswap/lightning"
	"github.com/sputn1ck/peerswap/swap"
	"github.com/sputn1ck/peerswap/txwatcher"
	"github.com/sputn1ck/peerswap/wallet"
	"github.com/vulpemventures/go-elements/address"
	address2 "github.com/vulpemventures/go-elements/address"
	"github.com/vulpemventures/go-elements/elementsutil"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/payment"
	"github.com/vulpemventures/go-elements/transaction"
	"log"
)

const (
	LiquidCsv = 60
)

type LiquidOnChain struct {
	elements  *gelements.Elements
	txWatcher *txwatcher.BlockchainRpcTxWatcher
	wallet    wallet.Wallet
	network   *network.Network
	asset     []byte
}

func NewLiquidOnChain(elements *gelements.Elements, txWatcher *txwatcher.BlockchainRpcTxWatcher, wallet wallet.Wallet, network *network.Network) *LiquidOnChain {
	lbtc := append(
		[]byte{0x01},
		elementsutil.ReverseBytes(h2b(network.AssetID))...,
	)
	return &LiquidOnChain{elements: elements, txWatcher: txWatcher, wallet: wallet, network: network, asset: lbtc}
}

func (l *LiquidOnChain) CreateOpeningTransaction(swapParams *swap.OpeningParams) (unpreparedTxHex string, txId string, fee uint64, csv uint32, vout uint32, err error) {
	redeemScript, err := ParamsToTxScript(swapParams, LiquidCsv)
	if err != nil {
		return "", "", 0, 0, 0, err
	}
	scriptPubKey := []byte{0x00, 0x20}
	witnessProgram := sha256.Sum256(redeemScript)
	scriptPubKey = append(scriptPubKey, witnessProgram[:]...)

	redeemPayment, _ := payment.FromScript(scriptPubKey, l.network, nil)
	sats, _ := elementsutil.SatoshiToElementsValue(swapParams.Amount)
	output := transaction.NewTxOutput(l.asset, sats, redeemPayment.WitnessScript)
	tx := transaction.NewTx(2)
	tx.Outputs = append(tx.Outputs, output)

	unpreparedTxHex, fee, err = l.wallet.CreateFundedTransaction(tx)
	if err != nil {
		return "", "", 0, 0, 0, err
	}

	vout, err = l.voutFromTxHex(unpreparedTxHex, redeemScript)
	if err != nil {
		return "", "", 0, 0, 0, err
	}

	return unpreparedTxHex, "", fee, LiquidCsv, vout, nil
}

func (l *LiquidOnChain) BroadcastOpeningTx(unpreparedTxHex string) (txId, txHex string, err error) {
	txHex, err = l.wallet.FinalizeFundedTransaction(unpreparedTxHex)
	if err != nil {
		return "", "", err
	}

	txId, err = l.elements.SendRawTx(txHex)
	if err != nil {
		return "", "", err
	}
	return txId, txHex, nil
}

func (l *LiquidOnChain) CreatePreimageSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, openingTxId string) (txId, txHex string, err error) {
	txHex, err = l.getRawTxFromTxId(openingTxId, 0)
	if err != nil {
		return "", "", err
	}
	newAddr, err := l.wallet.GetAddress()
	if err != nil {
		return "", "", err
	}

	tx, sigBytes, redeemScript, err := l.prepareSpendingTransaction(swapParams, claimParams, newAddr, txHex, 0)
	if err != nil {
		return "", "", err
	}

	preimage, err := lightning.MakePreimageFromStr(claimParams.Preimage)
	if err != nil {
		return "", "", err
	}

	tx.Inputs[0].Witness = GetPreimageWitness(sigBytes, preimage[:], redeemScript)

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

func (l *LiquidOnChain) CreateCsvSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, openingTxHex string, vout uint32) (txId, txHex string, error error) {
	newAddr, err := l.wallet.GetAddress()
	if err != nil {
		return "", "", err
	}
	tx, sigBytes, redeemScript, err := l.prepareSpendingTransaction(swapParams, claimParams, newAddr, openingTxHex, LiquidCsv)
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
func (l *LiquidOnChain) TakerCreateCoopSigHash(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, openingTxId, refundAddress string) (sigHash string, error error) {
	txHex, err := l.getRawTxFromTxId(openingTxId, 0)
	if err != nil {
		return "", err
	}
	_, sigBytes, _, err := l.prepareSpendingTransaction(swapParams, claimParams, refundAddress, txHex, 0)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sigBytes), nil
}

func (l *LiquidOnChain) CreateCooperativeSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, refundAddress, openingTxHex string, vout uint32, takerSignatureHex string) (txId, txHex string, error error) {
	tx, sigBytes, redeemScript, err := l.prepareSpendingTransaction(swapParams, claimParams, refundAddress, txHex, 0)
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

func (l *LiquidOnChain) CreateRefundAddress() (string, error) {
	addr, err := l.wallet.GetAddress()
	if err != nil {
		return "", err
	}
	return addr, nil
}

func (l *LiquidOnChain) prepareSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, spendingAddr, openingTxHex string, csv uint32) (tx *transaction.Transaction, sigBytes, redeemScript []byte, err error) {
	outputScript, err := l.blech32ToScript(spendingAddr)
	if err != nil {
		return nil, nil, nil, err
	}
	redeemScript, err = ParamsToTxScript(swapParams, claimParams.Csv)
	if err != nil {
		return nil, nil, nil, err
	}
	fee, err := l.getFee("")
	if err != nil {
		return nil, nil, nil, err
	}
	spendingTx, sigHash, err := l.createSpendingTransaction(openingTxHex, swapParams.Amount, fee, csv, l.asset, redeemScript, outputScript)
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
func (l *LiquidOnChain) createSpendingTransaction(openingTxHex string, swapAmount, feeAmount uint64, csv uint32, asset, redeemScript, outputScript []byte) (tx *transaction.Transaction, sigHash [32]byte, err error) {
	firstTx, err := transaction.NewTxFromHex(openingTxHex)
	if err != nil {
		log.Printf("error creating first tx %s", openingTxHex)
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
	spendingSatsBytes, _ := elementsutil.SatoshiToElementsValue(swapAmount - feeAmount)

	var txOutputs = []*transaction.TxOutput{}

	spendingOutput := transaction.NewTxOutput(asset, spendingSatsBytes[:], outputScript)
	txOutputs = append(txOutputs, spendingOutput)

	if feeAmount > 0 {
		feeValue, _ := elementsutil.SatoshiToElementsValue(feeAmount)
		feeScript := []byte{}
		feeOutput := transaction.NewTxOutput(asset, feeValue, feeScript)
		txOutputs = append(txOutputs, feeOutput)
	}

	spendingTx := &transaction.Transaction{
		Version:  2,
		Flag:     0,
		Inputs:   []*transaction.TxInput{spendingInput},
		Outputs:  txOutputs,
	}

	sigHash = spendingTx.HashForWitnessV0(
		0,
		redeemScript[:],
		swapInValue,
		txscript.SigHashAll,
	)
	return spendingTx, sigHash, nil
}

func (l *LiquidOnChain) AddWaitForConfirmationTx(swapId, txId string) (err error) {
	l.txWatcher.AddConfirmationsTx(swapId, txId)
	return nil
}

func (l *LiquidOnChain) AddWaitForCsvTx(swapId, txId string, vout, csv uint32) (err error) {
	l.txWatcher.AddCsvTx(swapId,txId,vout, csv)
	return nil
}

func (l *LiquidOnChain) AddConfirmationCallback(f func(swapId string) error) {
	l.txWatcher.AddTxConfirmedHandler(f)
}

func (l *LiquidOnChain) AddCsvCallback(f func(swapId string) error) {
	l.txWatcher.AddCsvPassedHandler(f)
}

func (l *LiquidOnChain) ValidateTx(swapParams *swap.OpeningParams, csv uint32, openingTxId string) (bool, error) {
	redeemScript, err := ParamsToTxScript(swapParams, csv)
	if err != nil {
		return false, err
	}

	txHex, err := l.getRawTxFromTxId(openingTxId, 0)
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

func (l *LiquidOnChain) voutFromTxHex(txHex string, redeemScript []byte) (uint32, error) {
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

func (l *LiquidOnChain) findVout(outputs []*transaction.TxOutput, redeemScript []byte) (uint32, error) {
	wantAddr, err := l.createOpeningAddress(redeemScript)
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
func (l *LiquidOnChain) createOpeningAddress(redeemScript []byte) (string, error) {
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

// GetRawTxFromTxId returns the txhex from the txid. This only works when the tx is not spent
func (l *LiquidOnChain) getRawTxFromTxId(txId string, vout uint32) (string, error) {
	txOut, err := l.elements.GetTxOut(txId, vout)
	if err != nil {
		return "", err
	}
	if txOut == nil {
		return "", errors.New("txout not set")
	}
	blockheight, err := l.elements.GetBlockHeight()
	if err != nil {
		return "", err
	}

	blockhash, err := l.elements.GetBlockHash(uint32(blockheight) - txOut.Confirmations + 1)
	if err != nil {
		return "", err
	}

	rawTxHex, err := l.elements.GetRawtransactionWithBlockHash(txId, blockhash)
	if err != nil {
		return "", err
	}
	return rawTxHex, nil
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

func (l *LiquidOnChain) getFee(txHex string) (uint64, error) {
	return 500, nil
}
