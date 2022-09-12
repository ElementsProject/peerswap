package onchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcutil/psbt"
	"github.com/elementsproject/peerswap/log"
	"github.com/elementsproject/peerswap/swap"
)

const (
	// BitcoinCsv is the amount of blocks that is set to the script OP_CSV. It
	// is the time in blocks after which the swap opening on-chain transaction
	// can be reclaimed by the maker. With an average time to mine a block of
	// 10m this value converts to 7 days.
	BitcoinCsv = 1008

	// BitcoinMinConfs is the amount of blocks after which it is assumed to be
	// reasonably safe to pay the claim invoice.
	BitcoinMinConfs = 3

	// BitcoinFeeTargetBlocks is the amount of blocks that is used to estimate
	// the on-chain fee.
	BitcoinFeeTargetBlocks = 6

	// BitcoinCsvSafetyLimit is the amount of blocks until which we assume it
	// to be safe to pay for the claim invoice. After this time we assume that
	// it is too close to the csv limit to pay the invoice.
	BitcoinCsvSafetyLimit = BitcoinCsv / 2

	// EstimatedOpeningTxSize in vByte is the estimated size of a swap opening
	// transaction with a security margin. The estimate is meant to express the
	// fees for most, but not all swap out opening tx fees. This is calculated
	// as follows: The average amount of inputs is 3 with an expected type of
	// P2WPKH that has a size of 68 vByte. The outputs are a P2WSH and a P2WPKH
	// output with a total size of 84 vByte including the tx overhead. This
	// leads to an expected size for the opening tx of (3*68 + 84) = 288 vByte.
	// We add a security margin to this which leads to the size of 350 vByte.
	EstimatedOpeningTxSize = 350

	// This defines the absolute floor of the feerate. This will be the minimum
	// feerate that will be used. The floor is set to 275 sat/kw so that we
	// always have a minimum fee rate of 1.1 sat/vb.
	floorFeeRateSatPerKw = 275
)

type BitcoinOnChain struct {
	chain *chaincfg.Params

	// estimator is an fee estimator that implements the Estimator interface.
	// This can be e.g. a GBitcoinEstimator or a LndEstimator. The Estimator
	// should already be running.
	estimator Estimator

	// fallbackFeeRateSatPerVb is the fee rate that is used to calculate the
	// fee of a transaction if the Estimator returned an error.
	fallbackFeeRateSatPerKw btcutil.Amount
}

func NewBitcoinOnChain(estimator Estimator, fallbackFeeRateSatPerKw btcutil.Amount, chain *chaincfg.Params) *BitcoinOnChain {
	return &BitcoinOnChain{
		chain:                   chain,
		estimator:               estimator,
		fallbackFeeRateSatPerKw: fallbackFeeRateSatPerKw,
	}
}

func (b *BitcoinOnChain) GetCSVHeight() uint32 {
	return BitcoinCsv
}

func (b *BitcoinOnChain) GetChain() *chaincfg.Params {
	return b.chain
}

func (b *BitcoinOnChain) ValidateTx(swapParams *swap.OpeningParams, openingTxHex string) (bool, error) {
	msgTx := wire.NewMsgTx(2)

	txBytes, err := hex.DecodeString(openingTxHex)
	if err != nil {
		return false, err
	}
	err = msgTx.Deserialize(bytes.NewReader(txBytes))
	if err != nil {
		return false, err
	}

	var scriptOut *wire.TxOut

	for _, out := range msgTx.TxOut {
		if out.Value == int64(swapParams.Amount) {
			scriptOut = out
			break
		}
	}
	if scriptOut == nil {
		return false, nil
	}

	redeemScript, err := ParamsToTxScript(swapParams, BitcoinCsv)
	if err != nil {
		return false, err
	}
	witnessProgram := sha256.Sum256(redeemScript)
	addr, err := btcutil.NewAddressWitnessScriptHash(witnessProgram[:], b.chain)
	if err != nil {
		return false, err
	}
	wantScript, err := txscript.NewScriptBuilder().AddData([]byte{0x00}).AddData(addr.ScriptAddress()).Script()
	if err != nil {
		return false, err
	}

	if bytes.Compare(wantScript, scriptOut.PkScript) != 0 {
		return false, err
	}
	return true, nil
}

func (b *BitcoinOnChain) TxIdFromHex(txHex string) (string, error) {
	msgTx := wire.NewMsgTx(2)

	txBytes, err := hex.DecodeString(txHex)
	if err != nil {
		return "", err
	}

	err = msgTx.Deserialize(bytes.NewReader(txBytes))
	if err != nil {
		return "", err
	}

	return msgTx.TxHash().String(), nil
}

func (b *BitcoinOnChain) GetVoutAndVerify(txHex string, params *swap.OpeningParams) (bool, uint32, error) {
	msgTx := wire.NewMsgTx(2)

	txBytes, err := hex.DecodeString(txHex)
	if err != nil {
		return false, 0, err
	}
	err = msgTx.Deserialize(bytes.NewReader(txBytes))
	if err != nil {
		return false, 0, err
	}

	var scriptOut *wire.TxOut
	var vout uint32
	for i, out := range msgTx.TxOut {
		if out.Value == int64(params.Amount) {
			scriptOut = out
			vout = uint32(i)
			break
		}
	}
	if scriptOut == nil {
		return false, 0, err
	}

	wantScript, err := b.GetOutputScript(params)
	if err != nil {
		return false, 0, err
	}

	if bytes.Compare(wantScript, scriptOut.PkScript) != 0 {
		return false, 0, err
	}

	return true, vout, nil
}

func (b *BitcoinOnChain) GetOutputScript(params *swap.OpeningParams) ([]byte, error) {
	redeemScript, err := ParamsToTxScript(params, BitcoinCsv)
	if err != nil {
		return nil, err
	}
	witnessProgram := sha256.Sum256(redeemScript)
	addr, err := btcutil.NewAddressWitnessScriptHash(witnessProgram[:], b.chain)
	if err != nil {
		return nil, err
	}
	wantScript, err := txscript.NewScriptBuilder().AddData([]byte{0x00}).AddData(addr.ScriptAddress()).Script()
	if err != nil {
		return nil, err
	}
	return wantScript, nil
}

func (b *BitcoinOnChain) PrepareSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, spendingAddr string, vout uint32, csv uint32, preparedFee uint64) (tx *wire.MsgTx, sigHash, redeemScript []byte, err error) {
	openingMsgTx := wire.NewMsgTx(2)
	txBytes, err := hex.DecodeString(claimParams.OpeningTxHex)
	if err != nil {
		return nil, nil, nil, err
	}
	err = openingMsgTx.Deserialize(bytes.NewReader(txBytes))
	if err != nil {
		return nil, nil, nil, err
	}

	// Add Input
	prevHash := openingMsgTx.TxHash()
	prevInput := wire.NewOutPoint(&prevHash, vout)

	// create spendingTx
	spendingTx := wire.NewMsgTx(2)

	scriptChangeAddr, err := btcutil.DecodeAddress(spendingAddr, b.chain)
	if err != nil {
		return nil, nil, nil, err
	}
	scriptChangeAddrScript := scriptChangeAddr.ScriptAddress()
	scriptChangeAddrScriptP2pkh, err := txscript.NewScriptBuilder().AddData([]byte{0x00}).AddData(scriptChangeAddrScript).Script()
	if err != nil {
		return nil, nil, nil, err
	}

	spendingTxOut := wire.NewTxOut(openingMsgTx.TxOut[vout].Value-200, scriptChangeAddrScriptP2pkh)
	spendingTx.AddTxOut(spendingTxOut)

	redeemScript, err = ParamsToTxScript(swapParams, BitcoinCsv)
	if err != nil {
		return nil, nil, nil, err
	}

	spendingTxInput := wire.NewTxIn(prevInput, nil, [][]byte{})
	spendingTxInput.Sequence = 0 | csv
	spendingTx.AddTxIn(spendingTxInput)

	// assume largest witness
	fee := preparedFee
	if preparedFee == 0 {
		fee, err = b.GetFee(int64(spendingTx.SerializeSizeStripped()) + 74)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	spendingTx.TxOut[0].Value = spendingTx.TxOut[0].Value - int64(fee)

	sigHashes := txscript.NewTxSigHashes(spendingTx)
	sigHash, err = txscript.CalcWitnessSigHash(redeemScript, sigHashes, txscript.SigHashAll, spendingTx, 0, int64(swapParams.Amount))
	if err != nil {
		return nil, nil, nil, err
	}

	return spendingTx, sigHash, redeemScript, nil
}

func (b *BitcoinOnChain) CreateOpeningAddress(params *swap.OpeningParams, csv uint32) (string, error) {
	redeemScript, err := ParamsToTxScript(params, csv)
	if err != nil {
		return "", err
	}
	witnessProgram := sha256.Sum256(redeemScript)
	addr, err := btcutil.NewAddressWitnessScriptHash(witnessProgram[:], b.chain)
	if err != nil {
		return "", err
	}
	return addr.EncodeAddress(), nil
}

func (b *BitcoinOnChain) GetFeeSatsFromTx(psbtString, txHex string) (uint64, error) {
	rawPsbt, err := psbt.NewFromRawBytes(bytes.NewReader([]byte(psbtString)), true)
	if err != nil {
		return 0, err
	}
	inputSats, err := psbt.SumUtxoInputValues(rawPsbt)
	if err != nil {
		return 0, err
	}
	txBytes, err := hex.DecodeString(txHex)
	if err != nil {
		return 0, err
	}

	tx, err := btcutil.NewTxFromBytes(txBytes)
	if err != nil {
		return 0, err
	}

	outputSats := int64(0)
	for _, out := range tx.MsgTx().TxOut {
		outputSats += out.Value
	}

	return uint64(inputSats - outputSats), nil
}

// GetFee returns the estimated fee in sat for a transaction of size txSize. It
// fetches the fee estimation from the Estimator in sat/kw and converts the
// returned fee estimation into sat/vb. The return value is in sat.
func (b *BitcoinOnChain) GetFee(txSize int64) (uint64, error) {
	// EstimateFeePerKw returns an btcutil.Amount that is in sat/kw.
	satPerKw, err := b.estimator.EstimateFeePerKW(BitcoinFeeTargetBlocks)
	switch {
	case err != nil:
		log.Debugf("Error fetching fee from estimator: %v", err)
		fallthrough
	case satPerKw == 0:
		// If we got no fee rate return we set the fee rate to the fallback
		// fee.
		satPerKw = btcutil.Amount(b.fallbackFeeRateSatPerKw)
	}

	// Ensure that the fee rate is at least as big as our fee floor.
	if satPerKw < floorFeeRateSatPerKw {
		log.Infof("Estimated fee rate is below floor of %d sat/kw, take floor "+
			"instead", floorFeeRateSatPerKw)
		satPerKw = floorFeeRateSatPerKw
	}

	// Convert to sat/vb. This operation is rounding down but should never be
	// below 1.0 sat/vb if we set the fallback fee above 250 sat/kw. We can set
	// this fallback fee in the fee estimator.
	satPerKb := satPerKw * witnessScaleFactor
	satPerVb := float64(satPerKb) / 1000

	// assume largest witness
	fee := uint64(satPerVb * float64(txSize))
	log.Debugf("Using a fee rate of %.2f sat/vb for a total fee of %d", satPerVb, fee)
	return fee, nil
}
