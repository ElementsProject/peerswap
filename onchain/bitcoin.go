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
	"github.com/elementsproject/peerswap/swap"
)

const (
	BitcoinCsv             = 1008
	BitcoinMinConfs        = 3
	BitcoinFeeTargetBlocks = 6
)

type BitcoinOnChain struct {
	chain     *chaincfg.Params
	estimator FeeEstimator
}

type FeeEstimator interface {
	GetFeePerKw(targetBlocks uint32) (float64, error)
}

func NewBitcoinOnChain(estimator FeeEstimator, chain *chaincfg.Params) *BitcoinOnChain {
	return &BitcoinOnChain{chain: chain, estimator: estimator}
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
		fee, err = b.GetFee(spendingTx.SerializeSizeStripped() + 74)
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

func (b *BitcoinOnChain) GetFee(txSize int) (uint64, error) {
	satPerByte, err := b.estimator.GetFeePerKw(BitcoinFeeTargetBlocks)
	if err != nil {
		return 0, err
	}
	// assume largest witness
	fee := satPerByte * float64(txSize)
	return uint64(fee), nil
}
