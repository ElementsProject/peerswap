package onchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcutil/psbt"
	"github.com/sputn1ck/glightning/gbitcoin"
	"github.com/sputn1ck/glightning/glightning"
	"github.com/sputn1ck/peerswap/lightning"
	"github.com/sputn1ck/peerswap/swap"
	"github.com/sputn1ck/peerswap/txwatcher"
)

const (
	BitcoinCltv   = 2016
	Target_Blocks = 6
)

type BitcoinOnChain struct {
	gbitcoin  *gbitcoin.Bitcoin
	txWatcher *txwatcher.BlockchainRpcTxWatcher

	clightning *glightning.Lightning

	chain *chaincfg.Params

	hexToIdMap map[string]string
}

func NewBitcoinOnChain(gbitcoin *gbitcoin.Bitcoin, txWatcher *txwatcher.BlockchainRpcTxWatcher, clightning *glightning.Lightning, chain *chaincfg.Params) *BitcoinOnChain {
	hexMap := make(map[string]string)
	return &BitcoinOnChain{gbitcoin: gbitcoin, txWatcher: txWatcher, clightning: clightning, hexToIdMap: hexMap, chain: chain}
}

func (b *BitcoinOnChain) CreateOpeningTransaction(swapParams *swap.OpeningParams) (unpreparedTxHex string, txId string, fee uint64, cltv int64, vout uint32, err error) {
	blockheight, err := b.gbitcoin.GetBlockHeight()
	if err != nil {
		return "", "", 0, 0, 0, err
	}
	cltv = int64(blockheight + BitcoinCltv)
	addr, err := b.createOpeningAddress(swapParams, cltv)
	if err != nil {
		return "", "", 0, 0, 0, err
	}
	outputs := []*glightning.Outputs{
		&glightning.Outputs{
			Address: addr,
			Satoshi: swapParams.Amount,
		},
	}
	prepRes, err := b.clightning.PrepareTx(outputs, &glightning.FeeRate{Directive: glightning.Urgent}, nil)
	if err != nil {
		return "", "", 0, 0, 0, err
	}
	fee, err = getFeeSatsFromTx(prepRes.Psbt, prepRes.UnsignedTx)
	if err != nil {
		return "", "", 0, 0, 0, err
	}

	_, vout, err = b.GetVoutAndVerify(prepRes.UnsignedTx, swapParams, cltv)
	if err != nil {
		return "", "", 0, 0, 0, err
	}
	b.hexToIdMap[prepRes.UnsignedTx] = prepRes.TxId
	return prepRes.UnsignedTx, prepRes.TxId, fee, cltv, vout, nil
}

func (b *BitcoinOnChain) BroadcastOpeningTx(unpreparedTxHex string) (txId, txHex string, error error) {
	var unpreparedTxId string
	var ok bool
	if unpreparedTxId, ok = b.hexToIdMap[unpreparedTxHex]; !ok {
		return "", "", errors.New("tx was not prepared not found in map")
	}
	delete(b.hexToIdMap, unpreparedTxHex)
	sendRes, err := b.clightning.SendTx(unpreparedTxId)
	if err != nil {
		return "", "", errors.New(fmt.Sprintf("tx was not prepared %v", err))
	}
	return sendRes.TxId, sendRes.SignedTx, nil
}

func (b *BitcoinOnChain) CreatePreimageSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, openingTxId string) (txId, txHex string, err error) {
	openingTxHex, err := b.getRawTxFromTxId(openingTxId, 0)
	if err != nil {
		return "", "", err
	}
	_, vout, err := b.GetVoutAndVerify(openingTxHex, swapParams, claimParams.Cltv)
	if err != nil {
		return "", "", err
	}

	tx, sigBytes, redeemScript, err := b.prepareSpendingTransaction(swapParams, claimParams, openingTxHex, vout)
	if err != nil {
		return "", "", err
	}
	preimage, err := lightning.MakePreimageFromStr(claimParams.Preimage)
	if err != nil {
		return "", "", err
	}

	tx.TxIn[0].Witness = GetPreimageWitness(sigBytes, preimage[:], redeemScript)

	bytesBuffer := new(bytes.Buffer)

	err = tx.Serialize(bytesBuffer)
	if err != nil {
		return "", "", err
	}

	txHex = hex.EncodeToString(bytesBuffer.Bytes())

	txId, err = b.gbitcoin.SendRawTx(txHex)
	if err != nil {
		return "", "", err
	}
	return txId, txHex, nil
}

func (b *BitcoinOnChain) CreateCltvSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, openingTxHex string, vout uint32) (txId, txHex string, error error) {
	tx, sigBytes, redeemScript, err := b.prepareSpendingTransaction(swapParams, claimParams, openingTxHex, vout)
	if err != nil {
		return "", "", err
	}

	tx.TxIn[0].Witness = GetCltvWitness(sigBytes, redeemScript)

	bytesBuffer := new(bytes.Buffer)

	err = tx.Serialize(bytesBuffer)
	if err != nil {
		return "", "", err
	}

	txHex = hex.EncodeToString(bytesBuffer.Bytes())

	txId, err = b.gbitcoin.SendRawTx(txHex)
	if err != nil {
		return "", "", err
	}
	return txId, txHex, nil
}

func (b *BitcoinOnChain) prepareSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, openingTxHex string, vout uint32) (tx *wire.MsgTx, sigBytes, redeemScript []byte, err error) {
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

	blockheight, err := b.gbitcoin.GetBlockHeight()
	if err != nil {
		return nil, nil, nil, err
	}

	// create spendingTx
	spendingTx := wire.NewMsgTx(2)
	spendingTx.LockTime = uint32(blockheight)

	// Add Output
	newAddr, err := b.clightning.NewAddr()
	if err != nil {
		return nil, nil, nil, err
	}

	scriptChangeAddr, err := btcutil.DecodeAddress(newAddr,b.chain)
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

	redeemScript, err = ParamsToTxScript(swapParams, claimParams.Cltv)
	if err != nil {
		return nil, nil, nil, err
	}

	spendingTxInput := wire.NewTxIn(prevInput, nil, [][]byte{})
	spendingTxInput.Sequence = 0

	spendingTx.AddTxIn(spendingTxInput)

	feeRes, err := b.gbitcoin.EstimateFee(Target_Blocks, "ECONOMICAL")
	if err != nil {
		return nil, nil, nil, err
	}
	satPerByte := float64(feeRes.SatPerKb()) / float64(1000)
	if len(feeRes.Errors) > 0 {
		//todo sane default sat per byte
		satPerByte = 10
	}
	// assume largest witness
	fee := satPerByte * float64(spendingTx.SerializeSizeStripped()+74)
	spendingTx.TxOut[0].Value = spendingTx.TxOut[0].Value - int64(fee)

	sigHashes := txscript.NewTxSigHashes(spendingTx)
	sigHash, err := txscript.CalcWitnessSigHash(redeemScript, sigHashes, txscript.SigHashAll, spendingTx, 0, 10000)
	if err != nil {
		return nil, nil, nil, err
	}

	sig, err := claimParams.Signer.Sign(sigHash[:])
	if err != nil {
		return nil, nil, nil, err
	}
	return spendingTx, sig.Serialize(), redeemScript, nil
}

func (b *BitcoinOnChain) AddWaitForConfirmationTx(swapId, txId string) (err error) {
	b.txWatcher.AddConfirmationsTx(swapId, txId)
	return nil
}

func (b *BitcoinOnChain) AddWaitForCltvTx(swapId, txId string, blockheight uint64) (err error) {
	b.txWatcher.AddCltvTx(swapId, int64(blockheight))
	return nil
}

func (b *BitcoinOnChain) AddConfirmationCallback(f func(swapId string) error) {
	b.txWatcher.AddTxConfirmedHandler(f)
}

func (b *BitcoinOnChain) AddCltvCallback(f func(swapId string) error) {
	b.txWatcher.AddCltvPassedHandler(f)
}

func (b *BitcoinOnChain) ValidateTx(swapParams *swap.OpeningParams, cltv int64, openingTxId string) (bool, error) {
	txHex, err := b.getRawTxFromTxId(openingTxId, 0)
	if err != nil {
		return false, err
	}
	ok, _, err := b.GetVoutAndVerify(txHex, swapParams, cltv)
	if err != nil {
		return false, err
	}
	return ok, nil
}

// GetRawTxFromTxId returns the txhex from the txid. This only works when the tx is not spent
func (b *BitcoinOnChain) getRawTxFromTxId(txId string, vout uint32) (string, error) {
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

func (b *BitcoinOnChain) createOpeningAddress(params *swap.OpeningParams, locktimeHeight int64) (string, error) {
	redeemScript, err := ParamsToTxScript(params, locktimeHeight)
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

func getFeeSatsFromTx(psbtString, txHex string) (uint64, error) {
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

func (b *BitcoinOnChain) GetVoutAndVerify(txHex string, params *swap.OpeningParams, cltv int64) (bool, uint32, error) {
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

	redeemScript, err := ParamsToTxScript(params, cltv)
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
