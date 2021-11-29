package onchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcutil/psbt"
	"github.com/sputn1ck/glightning/gbitcoin"
	"github.com/sputn1ck/peerswap/swap"
	"github.com/sputn1ck/peerswap/txwatcher"
)

const (
	BitcoinCsv            = 100
	Bitcoin_Target_Blocks = 6
)

type BitcoinOnChain struct {
	gbitcoin  *gbitcoin.Bitcoin
	txWatcher *txwatcher.BlockchainRpcTxWatcher

	chain *chaincfg.Params
}

func NewBitcoinOnChain(gbitcoin *gbitcoin.Bitcoin, txWatcher *txwatcher.BlockchainRpcTxWatcher, chain *chaincfg.Params) *BitcoinOnChain {
	return &BitcoinOnChain{gbitcoin: gbitcoin, txWatcher: txWatcher, chain: chain}
}

func (b *BitcoinOnChain) GetChain() *chaincfg.Params {
	return b.chain
}
func (b *BitcoinOnChain) AddWaitForConfirmationTx(swapId, txId string) (err error) {
	b.txWatcher.AddConfirmationsTx(swapId, txId)
	return nil
}

func (b *BitcoinOnChain) AddWaitForCsvTx(swapId, txId string, vout uint32) (err error) {
	b.txWatcher.AddCsvTx(swapId, txId, vout, BitcoinCsv)
	return nil
}

func (b *BitcoinOnChain) AddConfirmationCallback(f func(swapId string) error) {
	b.txWatcher.AddTxConfirmedHandler(f)
}

func (b *BitcoinOnChain) AddCsvCallback(f func(swapId string) error) {
	b.txWatcher.AddCsvPassedHandler(f)
}

func (b *BitcoinOnChain) ValidateTx(swapParams *swap.OpeningParams, openingTxId string) (bool, error) {
	txHex, err := b.GetRawTxFromTxId(openingTxId, 0)
	if err != nil {
		return false, err
	}
	ok, _, err := b.GetVoutAndVerify(txHex, swapParams)
	if err != nil {
		return false, err
	}
	return ok, nil
}

// GetRawTxFromTxId returns the txhex from the txid. This only works when the tx is not spent
func (b *BitcoinOnChain) GetRawTxFromTxId(txId string, vout uint32) (string, error) {
	txOut, err := b.gbitcoin.GetTxOut(txId, vout)
	if err != nil {
		return "", err
	}
	if txOut == nil {
		return "", errors.New("txout not set")
	}
	blockheight, err := b.gbitcoin.GetBlockHeight()
	if err != nil {
		return "", err
	}

	blockhash, err := b.gbitcoin.GetBlockHash(uint32(blockheight) - txOut.Confirmations + 1)
	if err != nil {
		return "", err
	}

	rawTxHex, err := b.gbitcoin.GetRawtransactionWithBlockHash(txId, blockhash)
	if err != nil {
		return "", err
	}
	return rawTxHex, nil
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

	redeemScript, err := ParamsToTxScript(params, BitcoinCsv)
	if err != nil {
		return false, 0, err
	}
	witnessProgram := sha256.Sum256(redeemScript)
	addr, err := btcutil.NewAddressWitnessScriptHash(witnessProgram[:], b.chain)
	if err != nil {
		return false, 0, err
	}
	wantScript, err := txscript.NewScriptBuilder().AddData([]byte{0x00}).AddData(addr.ScriptAddress()).Script()
	if err != nil {
		return false, 0, err
	}

	if bytes.Compare(wantScript, scriptOut.PkScript) != 0 {
		return false, 0, err
	}
	return true, vout, nil
}
func (b *BitcoinOnChain) PrepareSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, spendingAddr, openingTxHex string, vout uint32, csv uint32, preparedFee uint64) (tx *wire.MsgTx, sigHash, redeemScript []byte, err error) {
	openingMsgTx := wire.NewMsgTx(2)
	txBytes, err := hex.DecodeString(openingTxHex)
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
	feeRes, err := b.gbitcoin.EstimateFee(Bitcoin_Target_Blocks, "ECONOMICAL")
	if err != nil {
		return 0, err
	}
	satPerByte := float64(feeRes.SatPerKb()) / float64(1000)
	if len(feeRes.Errors) > 0 {
		//todo sane default sat per byte
		satPerByte = 10
	}
	// assume largest witness
	fee := satPerByte * float64(txSize)
	return uint64(fee), nil
}
