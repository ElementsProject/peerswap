package onchain

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/elementsproject/peerswap/log"
	"github.com/elementsproject/peerswap/wallet"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/vulpemventures/go-elements/confidential"

	"github.com/btcsuite/btcd/txscript"
	"github.com/elementsproject/peerswap/lightning"
	"github.com/elementsproject/peerswap/swap"

	"github.com/vulpemventures/go-elements/address"
	"github.com/vulpemventures/go-elements/elementsutil"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/payment"
	"github.com/vulpemventures/go-elements/transaction"
)

const (
	LiquidCsv   = 60
	LiquidConfs = 2
	// EstimatedOpeningConfidentialTxSizeBytes is the estimated size of a opening transaction.
	// The size is a calculate 2672 bytes for 3 inputs and 3 outputs of which 2 are
	// blinded. An additional safety margin is added for a total of 3000 bytes.
	EstimatedOpeningConfidentialTxSizeBytes = 3000
)

type LiquidOnChain struct {
	liquidWallet wallet.Wallet
	network      *network.Network
	asset        []byte
}

func NewLiquidOnChain(wallet wallet.Wallet, network *network.Network) *LiquidOnChain {
	lbtc := append(
		[]byte{0x01},
		elementsutil.ReverseBytes(h2b(network.AssetID))...,
	)

	return &LiquidOnChain{liquidWallet: wallet, network: network, asset: lbtc}
}

func (l *LiquidOnChain) GetCSVHeight() uint32 {
	return LiquidCsv
}

func (l *LiquidOnChain) GetOnchainBalance() (uint64, error) {
	return l.liquidWallet.GetBalance()
}

func (l *LiquidOnChain) CreateOpeningTransaction(swapParams *swap.OpeningParams) (txHex, address, txid string, fee uint64, vout uint32, err error) {
	redeemScript, err := ParamsToTxScript(swapParams, LiquidCsv)
	if err != nil {
		return "", "", "", 0, 0, err
	}
	scriptPubKey := []byte{0x00, 0x20}
	witnessProgram := sha256.Sum256(redeemScript)
	scriptPubKey = append(scriptPubKey, witnessProgram[:]...)

	redeemPayment, _ := payment.FromScript(scriptPubKey, l.network, swapParams.BlindingKey.PubKey())
	blindedScriptAddr, err := redeemPayment.ConfidentialWitnessScriptHash()
	if err != nil {
		return "", "", "", 0, 0, err
	}
	swapParams.OpeningAddress = blindedScriptAddr
	txId, txHex, fee, err := l.liquidWallet.CreateAndBroadcastTransaction(swapParams, l.asset)
	if err != nil {
		return "", "", "", 0, 0, err
	}
	return txHex, blindedScriptAddr, txId, fee, vout, nil
}

// feeAmountPlaceholder is a placeholder for the fee amount
const feeAmountPlaceholder = uint64(500)

func (l *LiquidOnChain) CreatePreimageSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams) (string, string, string, error) {
	fee, err := l.liquidWallet.GetFee(int64(getEstimatedTxSize(transactionKindPreimageSpending)))
	if err != nil {
		log.Infof("error getting fee %v", err)
		fee = feeAmountPlaceholder
	}
	return l.createPreimageSpendingTransaction(swapParams, claimParams, fee)
}

func (l *LiquidOnChain) createPreimageSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, fee uint64) (string, string, string, error) {
	newAddr, err := l.liquidWallet.GetAddress()
	if err != nil {
		return "", "", "", err
	}
	l.AddBlindingRandomFactors(claimParams)

	tx, sigBytes, redeemScript, err := l.prepareSpendingTransaction(swapParams, claimParams, newAddr, 0, fee)
	if err != nil {
		return "", "", "", err
	}

	txHex, err := tx.ToHex()
	if err != nil {
		return "", "", "", err
	}

	preimage, err := lightning.MakePreimageFromStr(claimParams.Preimage)
	if err != nil {
		return "", "", "", err
	}

	tx.Inputs[0].Witness = GetPreimageWitness(sigBytes, preimage[:], redeemScript)

	txHex, err = tx.ToHex()
	if err != nil {
		return "", "", "", err
	}

	txId, err := l.liquidWallet.SendRawTx(txHex)
	if err != nil {
		return "", "", "", err
	}
	return txId, txHex, newAddr, nil
}

func (l *LiquidOnChain) CreateCsvSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams) (txId, txHex, address string, error error) {
	fee, err := l.liquidWallet.GetFee(int64(getEstimatedTxSize(transactionKindPreimageSpending)))
	if err != nil {
		log.Infof("error getting fee %v", err)
		fee = feeAmountPlaceholder
	}
	return l.createCsvSpendingTransaction(swapParams, claimParams, fee)
}

func (l *LiquidOnChain) createCsvSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, fee uint64) (txId, txHex, address string, error error) {
	newAddr, err := l.liquidWallet.GetAddress()
	if err != nil {
		return "", "", "", err
	}
	l.AddBlindingRandomFactors(claimParams)
	tx, sigBytes, redeemScript, err := l.prepareSpendingTransaction(swapParams, claimParams, newAddr, LiquidCsv, fee)
	if err != nil {
		return "", "", "", err
	}
	tx.Inputs[0].Witness = GetCsvWitness(sigBytes, redeemScript)
	txHex, err = tx.ToHex()
	if err != nil {
		return "", "", "", err
	}
	txId, err = l.liquidWallet.SendRawTx(txHex)
	if err != nil {
		return "", "", "", err
	}
	return txId, txHex, newAddr, nil
}

func (l *LiquidOnChain) CreateCoopSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, takerSigner swap.Signer) (txId, txHex, address string, error error) {
	fee, err := l.liquidWallet.GetFee(int64(getEstimatedTxSize(transactionKindCoop)))
	if err != nil {
		log.Infof("error getting fee %v", err)
		fee = feeAmountPlaceholder
	}
	return l.createCoopSpendingTransaction(swapParams, claimParams, takerSigner, fee)
}

func (l *LiquidOnChain) createCoopSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, takerSigner swap.Signer, fee uint64) (txId, txHex, address string, error error) {
	refundAddr, err := l.NewAddress()
	if err != nil {
		return "", "", "", err
	}
	redeemScript, err := ParamsToTxScript(swapParams, LiquidCsv)
	if err != nil {
		return "", "", "", err
	}
	err = l.AddBlindingRandomFactors(claimParams)
	if err != nil {
		return "", "", "", err
	}
	spendingTx, sigHash, err := l.createSpendingTransaction(claimParams.OpeningTxHex, swapParams.Amount, 0, l.asset, redeemScript, refundAddr, fee, swapParams.BlindingKey, claimParams.EphemeralKey, claimParams.OutputAssetBlindingFactor, claimParams.BlindingSeed)
	if err != nil {
		return "", "", "", err
	}
	takerSig, err := takerSigner.Sign(sigHash[:])
	if err != nil {
		return "", "", "", err
	}
	makerSig, err := claimParams.Signer.Sign(sigHash[:])
	if err != nil {
		return "", "", "", err
	}

	spendingTx.Inputs[0].Witness = GetCooperativeWitness(takerSig.Serialize(), makerSig.Serialize(), redeemScript)

	txHex, err = spendingTx.ToHex()
	if err != nil {
		return "", "", "", err
	}
	txId, err = l.liquidWallet.SendRawTx(txHex)
	if err != nil {
		return "", "", "", err
	}
	return txId, txHex, refundAddr, nil
}

// SetLabelRequest is a request to label a transaction.
// https://developer.bitcoin.org/reference/rpc/setlabel.html
type SetLabelRequest struct {
	Address string `json:"address"`
	Label   string `json:"label"`
}

func (l *LiquidOnChain) Name() string {
	return "LabelTransactionRequest"
}

func (l *LiquidOnChain) SetLabel(txID, address, label string) error {
	return l.liquidWallet.SetLabel(txID, address, label)
}

func (l *LiquidOnChain) AddBlindingRandomFactors(claimParams *swap.ClaimParams) (err error) {
	claimParams.OutputAssetBlindingFactor = generateRandom32Bytes()
	claimParams.BlindingSeed = generateRandom32Bytes()
	claimParams.EphemeralKey, err = btcec.NewPrivateKey()
	if err != nil {
		return err
	}
	return nil
}

func (l *LiquidOnChain) NewAddress() (string, error) {
	addr, err := l.liquidWallet.GetAddress()
	if err != nil {
		return "", err
	}
	return addr, nil
}

func (l *LiquidOnChain) prepareSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, spendingAddr string, csv uint32, preparedFee uint64) (tx *transaction.Transaction, sigBytes, redeemScript []byte, err error) {
	redeemScript, err = ParamsToTxScript(swapParams, LiquidCsv)
	if err != nil {
		return nil, nil, nil, err
	}
	spendingTx, sigHash, err := l.createSpendingTransaction(claimParams.OpeningTxHex, swapParams.Amount, csv, l.asset, redeemScript, spendingAddr, preparedFee, swapParams.BlindingKey, claimParams.EphemeralKey, claimParams.OutputAssetBlindingFactor, claimParams.BlindingSeed)
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
func (l *LiquidOnChain) createSpendingTransaction(openingTxHex string, swapAmount uint64, csv uint32, asset, redeemScript []byte, redeemAddr string, preparedFee uint64, blindingKey, ephemeralPrivKey *btcec.PrivateKey, outputAbf, seed []byte) (tx *transaction.Transaction, sigHash [32]byte, err error) {
	if preparedFee == 0 {
		return nil, [32]byte{}, errors.New("fee must be set other than 0")
	}
	firstTx, err := transaction.NewTxFromHex(openingTxHex)
	if err != nil {
		log.Infof("error creating first tx %s, %v", openingTxHex, err)
		return nil, [32]byte{}, err
	}

	vout, err := l.FindVout(firstTx.Outputs, redeemScript)
	if err != nil {
		return nil, [32]byte{}, err
	}

	// unblind output
	ubRes, err := confidential.UnblindOutputWithKey(firstTx.Outputs[vout], blindingKey.Serialize())
	if err != nil {
		log.Infof("error unblinding output %v", err)
		return nil, [32]byte{}, err
	}

	if bytes.Equal(ubRes.Asset, l.asset) {
		err = errors.New(fmt.Sprintf("invalid asset id got: %x, expected %x", ubRes.Asset, l.asset))
		return nil, [32]byte{}, err
	}

	//check output amounts
	if ubRes.Value != swapAmount {
		return nil, [32]byte{}, errors.New(fmt.Sprintf("Tx value is not equal to the swap contract expected: %v, tx: %v", swapAmount, ubRes.Value))
	}

	outputValue := ubRes.Value - preparedFee

	finalVbfArgs := confidential.FinalValueBlindingFactorArgs{
		InValues:      []uint64{ubRes.Value},
		OutValues:     []uint64{outputValue},
		InGenerators:  [][]byte{ubRes.AssetBlindingFactor},
		OutGenerators: [][]byte{outputAbf},
		InFactors:     [][]byte{ubRes.ValueBlindingFactor},
		OutFactors:    [][]byte{},
	}

	outputVbf, err := confidential.FinalValueBlindingFactor(finalVbfArgs)
	if err != nil {
		return nil, [32]byte{}, err
	}

	// get asset commitment
	assetcommitment, err := confidential.AssetCommitment(ubRes.Asset, outputAbf[:])
	if err != nil {
		return nil, [32]byte{}, err
	}

	valueCommitment, err := confidential.ValueCommitment(outputValue, assetcommitment[:], outputVbf[:])
	if err != nil {
		return nil, [32]byte{}, err
	}

	surjectionProofArgs := confidential.SurjectionProofArgs{
		OutputAsset:               ubRes.Asset,
		OutputAssetBlindingFactor: outputAbf[:],
		InputAssets:               [][]byte{ubRes.Asset},
		InputAssetBlindingFactors: [][]byte{ubRes.AssetBlindingFactor},
		Seed:                      seed[:],
	}

	surjectionProof, ok := confidential.SurjectionProof(surjectionProofArgs)
	if !ok {
		return nil, [32]byte{}, errors.New(
			"failed to generate surjection proof, please retry",
		)
	}
	confOutputScript, err := address.ToOutputScript(redeemAddr)
	if err != nil {
		return nil, [32]byte{}, err
	}

	confAddr, err := address.FromConfidential(redeemAddr)
	if err != nil {
		return nil, [32]byte{}, err
	}

	// create new transaction
	spendingTx := transaction.NewTx(2)

	// add input
	txHash := firstTx.TxHash()
	swapInput := transaction.NewTxInput(txHash[:], vout)
	swapInput.Sequence = 0 | csv
	spendingTx.Inputs = []*transaction.TxInput{swapInput}

	outputNonce := ephemeralPrivKey.PubKey()

	nonce, err := confidential.NonceHash(confAddr.BlindingKey, ephemeralPrivKey.Serialize())
	if err != nil {
		return nil, [32]byte{}, err
	}

	// build rangeproof
	rangeProofArgs := confidential.RangeProofArgs{
		Value:               outputValue,
		Nonce:               nonce,
		Asset:               ubRes.Asset,
		AssetBlindingFactor: outputAbf[:],
		ValueBlindFactor:    outputVbf,
		ValueCommit:         valueCommitment[:],
		ScriptPubkey:        confOutputScript,
		Exp:                 0,
		MinBits:             52,
	}

	rangeProof, err := confidential.RangeProof(rangeProofArgs)
	if err != nil {
		return nil, [32]byte{}, err
	}

	//create output
	receiverOutput := transaction.NewTxOutput(asset, valueCommitment, confOutputScript)
	receiverOutput.Asset = assetcommitment
	receiverOutput.Value = valueCommitment
	receiverOutput.Nonce = outputNonce.SerializeCompressed()
	receiverOutput.RangeProof = rangeProof
	receiverOutput.SurjectionProof = surjectionProof

	spendingTx.Outputs = append(spendingTx.Outputs, receiverOutput)

	// add feeoutput
	feeValue, _ := elementsutil.ValueToBytes(preparedFee)
	feeScript := []byte{}
	feeOutput := transaction.NewTxOutput(asset, feeValue, feeScript)
	spendingTx.Outputs = append(spendingTx.Outputs, feeOutput)

	// create sighash
	sigHash = spendingTx.HashForWitnessV0(
		0, redeemScript[:], firstTx.Outputs[vout].Value, txscript.SigHashAll)

	return spendingTx, sigHash, nil
}

type transactionKind string

const (
	transactionKindPreimageSpending transactionKind = "preimage"
	transactionKindCoop             transactionKind = "coop"
	transactionKindOpening          transactionKind = "open"
	transactionKindCSV              transactionKind = "csv"
)

// getEstimatedTxSize estimates the size of a transaction based on its kind and whether it's using Confidential Transactions (CT).
func getEstimatedTxSize(t transactionKind) int {
	txsize := 0
	switch t {
	case transactionKindPreimageSpending:
		// Preimage spending transactions have an estimated size of 1350 bytes.
		txsize = 1350
	case transactionKindCoop:
		// Cooperative close transactions have an estimated size of 1360 bytes.
		txsize = 1360
	case transactionKindOpening:
		// Opening transactions have a variable size, estimated in the constant EstimatedOpeningConfidentialTxSizeBytes.
		txsize = EstimatedOpeningConfidentialTxSizeBytes
	case transactionKindCSV:
		// CSV transactions have an estimated size of 1350 bytes.
		txsize = 1350
	default:
		// For unknown transaction types, assume a default size of 1360 bytes.
		return 1360
	}
	// the transaction size is reduced by 75% for ct discount.
	// TODO:
	//   This is a placeholder value, the actual discount should
	//   be calculated based on discount vsize.
	//   To do this, we would need to construct the transaction, decode it, and then get the discounted vsize.
	//   However, this would have a significant impact on the codebase.
	//   As a temporary measure, we're taking a conservative approach and applying a 75% discount.
	//   For the discount calculation, refer to https://github.com/ElementsProject/ELIPs/blob/main/elip-0200.mediawiki.
	return txsize / 4
}

func (l *LiquidOnChain) TxIdFromHex(txHex string) (string, error) {
	openingTx, err := transaction.NewTxFromHex(txHex)
	if err != nil {
		return "", err
	}
	return openingTx.TxHash().String(), nil
}

func (l *LiquidOnChain) ValidateTx(openingParams *swap.OpeningParams, txHex string) (bool, error) {
	redeemScript, err := ParamsToTxScript(openingParams, LiquidCsv)
	if err != nil {
		return false, err
	}

	openingTx, err := transaction.NewTxFromHex(txHex)
	if err != nil {
		return false, err
	}

	vout, err := l.FindVout(openingTx.Outputs, redeemScript)
	if err != nil {
		return false, err
	}

	// unblind output
	ubRes, err := confidential.UnblindOutputWithKey(openingTx.Outputs[vout], openingParams.BlindingKey.Serialize())
	if err != nil {
		return false, err
	}

	// todo muss ins protocol
	if bytes.Equal(ubRes.Asset, l.asset) {
		err = errors.New(fmt.Sprintf("invalid asset id got: %x, expected %x", ubRes.Asset, l.asset))
		return false, err
	}

	//check output amounts
	if ubRes.Value != openingParams.Amount {
		return false, errors.New(fmt.Sprintf("Tx value is not equal to the swap contract expected: %v, tx: %v", openingParams.Amount, ubRes.Value))
	}

	//todo check script
	return true, nil
}

func (l *LiquidOnChain) VoutFromTxHex(txHex string, redeemScript []byte) (uint32, error) {
	tx, err := transaction.NewTxFromHex(txHex)
	if err != nil {
		return 0, err
	}
	vout, err := l.FindVout(tx.Outputs, redeemScript)
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
	wantBytes, err := address.ToOutputScript(wantAddr)
	if err != nil {
		return nil, err
	}
	return wantBytes, nil
}

func (l *LiquidOnChain) FindVout(outputs []*transaction.TxOutput, redeemScript []byte) (uint32, error) {
	wantAddr, err := l.CreateOpeningAddress(redeemScript)
	if err != nil {
		return 0, err
	}
	wantBytes, err := address.ToOutputScript(wantAddr)
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

// CreateOpeningAddress returns the address for the opening tx
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

// CreateBlindedOpeningAddress returns the address for the opening tx
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
func (l *LiquidOnChain) Blech32ToScript(blech32Addr string) ([]byte, error) {
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

func (l *LiquidOnChain) GetRefundFee() (uint64, error) {
	return l.liquidWallet.GetFee(int64(getEstimatedTxSize(transactionKindCoop)))
}

// GetFlatOpeningTXFee returns an estimate of the fee for the opening transaction.
func (l *LiquidOnChain) GetFlatOpeningTXFee() (uint64, error) {
	return l.liquidWallet.GetFee(int64(getEstimatedTxSize(transactionKindOpening)))
}

func (l *LiquidOnChain) GetAsset() string {
	return hex.EncodeToString(l.asset)
}

func (l *LiquidOnChain) GetNetwork() string {
	return ""
}

func generateRandom32Bytes() []byte {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return []byte{}
	}
	return randomBytes
}
