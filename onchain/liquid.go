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

func (l *LiquidOnChain) CreateOpeningTransaction(swapParams *swap.OpeningParams) (txHex, openingAddress, txid string, fee uint64, vout uint32, err error) {
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

	// Fee reserve for the spending tx (paid in LBTC).
	feeEstimate, err := l.liquidWallet.GetFee(int64(getEstimatedTxSize(transactionKindPreimageSpending)))
	if err != nil {
		log.Infof("error getting fee estimate %v", err)
		feeEstimate = feeAmountPlaceholder
	}
	feeReserve := feeEstimate * 2

	// OpeningTx outputs:
	// 1) swap asset output (confidential) locked to swap script
	// 2) LBTC fee-reserve output (confidential) locked to swap script
	outputs := []wallet.TxOutput{
		{AssetID: swapParams.AssetId, Amount: swapParams.Amount},
		{AssetID: l.network.AssetID, Amount: feeReserve},
	}

	txId, txHex, fee, err := l.liquidWallet.CreateAndBroadcastTransaction(swapParams, outputs)
	if err != nil {
		return "", "", "", 0, 0, err
	}

	// Find vout of the swap-asset output (used for txwatcher and spending).
	assetIdBytes, err := hex.DecodeString(swapParams.AssetId)
	if err != nil {
		return "", "", "", 0, 0, err
	}
	if len(assetIdBytes) != 32 {
		return "", "", "", 0, 0, fmt.Errorf("invalid asset id length: %d", len(assetIdBytes))
	}
	wantAsset := elementsutil.ReverseBytes(assetIdBytes)
	wantScript, err := address.ToOutputScript(blindedScriptAddr)
	if err != nil {
		return "", "", "", 0, 0, err
	}
	openingTx, err := transaction.NewTxFromHex(txHex)
	if err != nil {
		return "", "", "", 0, 0, err
	}
	for i, out := range openingTx.Outputs {
		if !bytes.Equal(out.Script, wantScript) {
			continue
		}
		ubRes, err := confidential.UnblindOutputWithKey(out, swapParams.BlindingKey.Serialize())
		if err != nil {
			continue
		}
		if bytes.Equal(ubRes.Asset, wantAsset) && ubRes.Value == swapParams.Amount {
			return txHex, blindedScriptAddr, txId, fee, uint32(i), nil
		}
	}

	return "", "", "", 0, 0, errors.New("swap output vout not found")
}

// feeAmountPlaceholder is a placeholder for the fee amount
const feeAmountPlaceholder = uint64(500)

func (l *LiquidOnChain) CreatePreimageSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams) (string, string, string, error) {
	return l.createPreimageSpendingTransaction(swapParams, claimParams)
}

func (l *LiquidOnChain) createPreimageSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams) (string, string, string, error) {
	newAddr, err := l.liquidWallet.GetAddress()
	if err != nil {
		return "", "", "", err
	}
	l.AddBlindingRandomFactors(claimParams)

	tx, sigHashes, redeemScript, err := l.prepareSpendingTransaction(swapParams, claimParams, newAddr, 0)
	if err != nil {
		return "", "", "", err
	}

	preimage, err := lightning.MakePreimageFromStr(claimParams.Preimage)
	if err != nil {
		return "", "", "", err
	}

	for i := range tx.Inputs {
		sig, err := claimParams.Signer.Sign(sigHashes[i][:])
		if err != nil {
			return "", "", "", err
		}
		tx.Inputs[i].Witness = GetPreimageWitness(sig.Serialize(), preimage[:], redeemScript)
	}

	txHex, err := tx.ToHex()
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
	return l.createCsvSpendingTransaction(swapParams, claimParams)
}

func (l *LiquidOnChain) createCsvSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams) (txId, txHex, address string, error error) {
	newAddr, err := l.liquidWallet.GetAddress()
	if err != nil {
		return "", "", "", err
	}
	l.AddBlindingRandomFactors(claimParams)
	tx, sigHashes, redeemScript, err := l.prepareSpendingTransaction(swapParams, claimParams, newAddr, LiquidCsv)
	if err != nil {
		return "", "", "", err
	}

	for i := range tx.Inputs {
		sig, err := claimParams.Signer.Sign(sigHashes[i][:])
		if err != nil {
			return "", "", "", err
		}
		tx.Inputs[i].Witness = GetCsvWitness(sig.Serialize(), redeemScript)
	}

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
	return l.createCoopSpendingTransaction(swapParams, claimParams, takerSigner)
}

func (l *LiquidOnChain) createCoopSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, takerSigner swap.Signer) (txId, txHex, address string, error error) {
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
	spendingTx, sigHashes, err := l.createSpendingTransaction(claimParams.OpeningTxHex, swapParams.AssetId, swapParams.Amount, 0, redeemScript, refundAddr, swapParams.BlindingKey, claimParams.EphemeralKey, claimParams.OutputAssetBlindingFactor, claimParams.BlindingSeed)
	if err != nil {
		return "", "", "", err
	}
	for i := range spendingTx.Inputs {
		takerSig, err := takerSigner.Sign(sigHashes[i][:])
		if err != nil {
			return "", "", "", err
		}
		makerSig, err := claimParams.Signer.Sign(sigHashes[i][:])
		if err != nil {
			return "", "", "", err
		}
		spendingTx.Inputs[i].Witness = GetCooperativeWitness(takerSig.Serialize(), makerSig.Serialize(), redeemScript)
	}

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

func (l *LiquidOnChain) prepareSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, spendingAddr string, csv uint32) (tx *transaction.Transaction, sigHashes [][32]byte, redeemScript []byte, err error) {
	redeemScript, err = ParamsToTxScript(swapParams, LiquidCsv)
	if err != nil {
		return nil, nil, nil, err
	}
	spendingTx, sigHashes, err := l.createSpendingTransaction(claimParams.OpeningTxHex, swapParams.AssetId, swapParams.Amount, csv, redeemScript, spendingAddr, swapParams.BlindingKey, claimParams.EphemeralKey, claimParams.OutputAssetBlindingFactor, claimParams.BlindingSeed)
	if err != nil {
		return nil, nil, nil, err
	}
	return spendingTx, sigHashes, redeemScript, nil
}

// createSpendingTransaction builds a Liquid spending transaction consuming:
// - the swap-asset output (asset_id, asset_amount) and
// - an additional LBTC fee-reserve output (locked to the same script).
//
// The spending transaction pays the full swap asset amount to redeemAddr and
// burns the entire fee-reserve as a fee output (no LBTC change output).
func (l *LiquidOnChain) createSpendingTransaction(openingTxHex string, swapAssetId string, swapAmount uint64, csv uint32, redeemScript []byte, redeemAddr string, blindingKey, ephemeralPrivKey *btcec.PrivateKey, outputAbf, seed []byte) (tx *transaction.Transaction, sigHashes [][32]byte, err error) {
	if swapAssetId == "" {
		return nil, nil, errors.New("asset_id must be set for liquid spending transaction")
	}
	if blindingKey == nil {
		return nil, nil, errors.New("blinding_key must be set for liquid spending transaction")
	}
	if ephemeralPrivKey == nil {
		return nil, nil, errors.New("ephemeral_key must be set for liquid spending transaction")
	}

	firstTx, err := transaction.NewTxFromHex(openingTxHex)
	if err != nil {
		log.Infof("error creating first tx %s, %v", openingTxHex, err)
		return nil, nil, err
	}

	swapAssetIdBytes, err := hex.DecodeString(swapAssetId)
	if err != nil {
		return nil, nil, err
	}
	if len(swapAssetIdBytes) != 32 {
		return nil, nil, fmt.Errorf("invalid asset id length: %d", len(swapAssetIdBytes))
	}
	wantSwapAsset := elementsutil.ReverseBytes(swapAssetIdBytes)
	wantLbtcAsset := elementsutil.ReverseBytes(h2b(l.network.AssetID))

	scriptPubKey := []byte{0x00, 0x20}
	witnessProgram := sha256.Sum256(redeemScript)
	scriptPubKey = append(scriptPubKey, witnessProgram[:]...)

	var swapVout uint32
	var feeVout uint32
	var ubSwap *confidential.UnblindOutputResult
	var ubFee *confidential.UnblindOutputResult
	var foundSwap bool
	var foundFee bool

	for i, out := range firstTx.Outputs {
		if !bytes.Equal(out.Script, scriptPubKey) {
			continue
		}
		ubRes, err := confidential.UnblindOutputWithKey(out, blindingKey.Serialize())
		if err != nil {
			return nil, nil, err
		}

		if !foundSwap && bytes.Equal(ubRes.Asset, wantSwapAsset) && ubRes.Value == swapAmount {
			foundSwap = true
			swapVout = uint32(i)
			ubSwap = ubRes
			continue
		}

		if !foundFee && bytes.Equal(ubRes.Asset, wantLbtcAsset) && ubRes.Value > 0 {
			foundFee = true
			feeVout = uint32(i)
			ubFee = ubRes
			continue
		}
	}

	if !foundSwap {
		return nil, nil, errors.New("swap output not found in opening tx")
	}
	if !foundFee || feeVout == swapVout {
		return nil, nil, errors.New("fee-reserve output not found in opening tx")
	}

	feeValue := ubFee.Value
	outputValue := ubSwap.Value

	finalVbfArgs := confidential.FinalValueBlindingFactorArgs{
		InValues:      []uint64{ubSwap.Value, ubFee.Value},
		OutValues:     []uint64{outputValue},
		InGenerators:  [][]byte{ubSwap.AssetBlindingFactor, ubFee.AssetBlindingFactor},
		OutGenerators: [][]byte{outputAbf},
		InFactors:     [][]byte{ubSwap.ValueBlindingFactor, ubFee.ValueBlindingFactor},
		OutFactors:    [][]byte{},
	}

	outputVbf, err := confidential.FinalValueBlindingFactor(finalVbfArgs)
	if err != nil {
		return nil, nil, err
	}

	// get asset commitment
	assetcommitment, err := confidential.AssetCommitment(ubSwap.Asset, outputAbf[:])
	if err != nil {
		return nil, nil, err
	}

	valueCommitment, err := confidential.ValueCommitment(outputValue, assetcommitment[:], outputVbf[:])
	if err != nil {
		return nil, nil, err
	}

	surjectionProofArgs := confidential.SurjectionProofArgs{
		OutputAsset:               ubSwap.Asset,
		OutputAssetBlindingFactor: outputAbf[:],
		InputAssets:               [][]byte{ubSwap.Asset, ubFee.Asset},
		InputAssetBlindingFactors: [][]byte{ubSwap.AssetBlindingFactor, ubFee.AssetBlindingFactor},
		Seed:                      seed[:],
	}

	surjectionProof, ok := confidential.SurjectionProof(surjectionProofArgs)
	if !ok {
		return nil, nil, errors.New(
			"failed to generate surjection proof, please retry",
		)
	}
	confOutputScript, err := address.ToOutputScript(redeemAddr)
	if err != nil {
		return nil, nil, err
	}

	confAddr, err := address.FromConfidential(redeemAddr)
	if err != nil {
		return nil, nil, err
	}

	// create new transaction
	spendingTx := transaction.NewTx(2)

	// add inputs
	txHash := firstTx.TxHash()
	swapInput := transaction.NewTxInput(txHash[:], swapVout)
	swapInput.Sequence = 0 | csv
	feeInput := transaction.NewTxInput(txHash[:], feeVout)
	feeInput.Sequence = 0 | csv
	spendingTx.Inputs = []*transaction.TxInput{swapInput, feeInput}

	outputNonce := ephemeralPrivKey.PubKey()

	nonce, err := confidential.NonceHash(confAddr.BlindingKey, ephemeralPrivKey.Serialize())
	if err != nil {
		return nil, nil, err
	}

	// build rangeproof
	rangeProofArgs := confidential.RangeProofArgs{
		Value:               outputValue,
		Nonce:               nonce,
		Asset:               ubSwap.Asset,
		AssetBlindingFactor: outputAbf[:],
		ValueBlindFactor:    outputVbf,
		ValueCommit:         valueCommitment[:],
		ScriptPubkey:        confOutputScript,
		Exp:                 0,
		MinBits:             52,
	}

	rangeProof, err := confidential.RangeProof(rangeProofArgs)
	if err != nil {
		return nil, nil, err
	}

	//create output
	receiverOutput := transaction.NewTxOutput(l.asset, valueCommitment, confOutputScript)
	receiverOutput.Asset = assetcommitment
	receiverOutput.Value = valueCommitment
	receiverOutput.Nonce = outputNonce.SerializeCompressed()
	receiverOutput.RangeProof = rangeProof
	receiverOutput.SurjectionProof = surjectionProof

	spendingTx.Outputs = append(spendingTx.Outputs, receiverOutput)

	// add feeoutput
	feeValueBytes, _ := elementsutil.ValueToBytes(feeValue)
	feeScript := []byte{}
	feeOutput := transaction.NewTxOutput(l.asset, feeValueBytes, feeScript)
	spendingTx.Outputs = append(spendingTx.Outputs, feeOutput)

	// create sighashes (one per input)
	sigHashSwap := spendingTx.HashForWitnessV0(
		0, redeemScript[:], firstTx.Outputs[swapVout].Value, txscript.SigHashAll)
	sigHashFee := spendingTx.HashForWitnessV0(
		1, redeemScript[:], firstTx.Outputs[feeVout].Value, txscript.SigHashAll)

	return spendingTx, [][32]byte{sigHashSwap, sigHashFee}, nil
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
	if openingParams.AssetId == "" {
		return false, errors.New("asset_id must be set for liquid opening tx validation")
	}
	if openingParams.BlindingKey == nil {
		return false, errors.New("blinding_key must be set for liquid opening tx validation")
	}

	redeemScript, err := ParamsToTxScript(openingParams, LiquidCsv)
	if err != nil {
		return false, err
	}

	openingTx, err := transaction.NewTxFromHex(txHex)
	if err != nil {
		return false, err
	}

	swapAssetIdBytes, err := hex.DecodeString(openingParams.AssetId)
	if err != nil {
		return false, err
	}
	if len(swapAssetIdBytes) != 32 {
		return false, fmt.Errorf("invalid asset id length: %d", len(swapAssetIdBytes))
	}
	wantSwapAsset := elementsutil.ReverseBytes(swapAssetIdBytes)
	wantLbtcAsset := elementsutil.ReverseBytes(h2b(l.network.AssetID))

	feeEstimate, err := l.liquidWallet.GetFee(int64(getEstimatedTxSize(transactionKindPreimageSpending)))
	if err != nil {
		log.Infof("error getting fee estimate %v", err)
		feeEstimate = feeAmountPlaceholder
	}

	scriptPubKey := []byte{0x00, 0x20}
	witnessProgram := sha256.Sum256(redeemScript)
	scriptPubKey = append(scriptPubKey, witnessProgram[:]...)

	var foundSwap bool
	var foundFeeReserve bool
	var swapVout uint32
	var feeVout uint32

	for i, out := range openingTx.Outputs {
		if !bytes.Equal(out.Script, scriptPubKey) {
			continue
		}

		ubRes, err := confidential.UnblindOutputWithKey(out, openingParams.BlindingKey.Serialize())
		if err != nil {
			return false, err
		}

		if !foundSwap && bytes.Equal(ubRes.Asset, wantSwapAsset) && ubRes.Value == openingParams.Amount {
			foundSwap = true
			swapVout = uint32(i)
			continue
		}

		if !foundFeeReserve && bytes.Equal(ubRes.Asset, wantLbtcAsset) && ubRes.Value >= feeEstimate {
			foundFeeReserve = true
			feeVout = uint32(i)
			continue
		}
	}

	if !foundSwap {
		return false, errors.New("swap output not found in opening tx")
	}
	if !foundFeeReserve || feeVout == swapVout {
		return false, errors.New("fee-reserve output not found or too small in opening tx")
	}

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
	// Return the canonical (big-endian) asset id as commonly displayed in
	// explorers and Elements APIs.
	return l.network.AssetID
}

func (l *LiquidOnChain) GetNetwork() string {
	// `network.Network.Name` is "liquid", "testnet", "regtest" which would
	// collide with Bitcoin network names. For peerswap protocol we disambiguate
	// Liquid by prefixing the test networks.
	switch l.network.Name {
	case "liquid":
		return "liquid"
	case "testnet":
		return "liquid-testnet"
	case "regtest":
		return "liquid-regtest"
	default:
		return l.network.Name
	}
}

func generateRandom32Bytes() []byte {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return []byte{}
	}
	return randomBytes
}
