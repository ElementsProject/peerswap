package clightning

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcutil/psbt"
	"github.com/sputn1ck/glightning/glightning"
	"github.com/sputn1ck/peerswap/lightning"
	"github.com/sputn1ck/peerswap/onchain"
	"github.com/sputn1ck/peerswap/swap"
	"log"
)

func (b *ClightningClient) CreateOpeningTransaction(swapParams *swap.OpeningParams) (unpreparedTxHex string, txId string, fee uint64, csv uint32, vout uint32, err error) {

	addr, err := b.createOpeningAddress(swapParams, onchain.BitcoinCsv)
	if err != nil {
		return "", "", 0, 0, 0, err
	}
	outputs := []*glightning.Outputs{
		{
			Address: addr,
			Satoshi: swapParams.Amount,
		},
	}
	prepRes, err := b.glightning.PrepareTx(outputs, &glightning.FeeRate{Directive: glightning.Urgent}, nil)
	if err != nil {
		return "", "", 0, 0, 0, err
	}
	fee, err = getFeeSatsFromTx(prepRes.Psbt, prepRes.UnsignedTx)
	if err != nil {
		return "", "", 0, 0, 0, err
	}

	_, vout, err = b.bitcoinChain.GetVoutAndVerify(prepRes.UnsignedTx, swapParams)
	if err != nil {
		return "", "", 0, 0, 0, err
	}
	b.hexToIdMap[prepRes.UnsignedTx] = prepRes.TxId
	return prepRes.UnsignedTx, prepRes.TxId, fee, onchain.BitcoinCsv, vout, nil
}

func (b *ClightningClient) BroadcastOpeningTx(unpreparedTxHex string) (txId, txHex string, error error) {
	var unpreparedTxId string
	var ok bool
	if unpreparedTxId, ok = b.hexToIdMap[unpreparedTxHex]; !ok {
		return "", "", errors.New("tx was not prepared not found in map")
	}
	delete(b.hexToIdMap, unpreparedTxHex)
	sendRes, err := b.glightning.SendTx(unpreparedTxId)
	if err != nil {
		return "", "", errors.New(fmt.Sprintf("tx was not prepared %v", err))
	}
	return sendRes.TxId, sendRes.SignedTx, nil
}

func (b *ClightningClient) CreatePreimageSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, openingTxId string) (txId, txHex string, err error) {
	openingTxHex, err := b.bitcoinChain.GetRawTxFromTxId(openingTxId, 0)
	if err != nil {
		return "", "", err
	}

	_, vout, err := b.bitcoinChain.GetVoutAndVerify(openingTxHex, swapParams)
	if err != nil {
		return "", "", err
	}

	newAddr, err := b.glightning.NewAddr()
	if err != nil {
		return "", "", err
	}

	tx, sigHash, redeemScript, err := b.prepareSpendingTransaction(swapParams, claimParams, newAddr, openingTxHex, vout, 0, 0)
	if err != nil {
		return "", "", err
	}
	sigBytes, err := claimParams.Signer.Sign(sigHash)
	if err != nil {
		return "", "", err
	}

	preimage, err := lightning.MakePreimageFromStr(claimParams.Preimage)
	if err != nil {
		return "", "", err
	}

	tx.TxIn[0].Witness = onchain.GetPreimageWitness(sigBytes.Serialize(), preimage[:], redeemScript)

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

func (b *ClightningClient) CreateCsvSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, openingTxHex string, vout uint32) (txId, txHex string, error error) {
	newAddr, err := b.glightning.NewAddr()
	if err != nil {
		return "", "", err
	}

	tx, sigHash, redeemScript, err := b.prepareSpendingTransaction(swapParams, claimParams, newAddr, openingTxHex, vout, onchain.BitcoinCsv, 0)
	if err != nil {
		return "", "", err
	}

	sigBytes, err := claimParams.Signer.Sign(sigHash)
	if err != nil {
		return "", "", err
	}

	tx.TxIn[0].Witness = onchain.GetCsvWitness(sigBytes.Serialize(), redeemScript)

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

func (b *ClightningClient) TakerCreateCoopSigHash(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, openingTxId, refundAddress string, refundFee uint64) (sigHash string, error error) {
	openingTxHex, err := b.bitcoinChain.GetRawTxFromTxId(openingTxId, 0)
	if err != nil {
		return "", err
	}

	_, vout, err := b.bitcoinChain.GetVoutAndVerify(openingTxHex, swapParams)
	if err != nil {
		return "", err
	}
	_, sigHashBytes, _, err := b.prepareSpendingTransaction(swapParams, claimParams, refundAddress, openingTxHex, vout, 0, refundFee)
	if err != nil {
		return "", err
	}
	log.Printf("sighash at takercreate %s", hex.EncodeToString(sigHashBytes))
	sigBytes, err := claimParams.Signer.Sign(sigHashBytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sigBytes.Serialize()), nil

}

func (b *ClightningClient) CreateCooperativeSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, refundAddress, openingTxHex string, vout uint32, takerSignatureHex string, refundFee uint64) (txId, txHex string, error error) {
	tx, sigHashBytes, redeemScript, err := b.prepareSpendingTransaction(swapParams, claimParams, refundAddress, openingTxHex, vout, 0, refundFee)
	if err != nil {
		return "", "", err
	}

	sigBytes, err := claimParams.Signer.Sign(sigHashBytes)
	if err != nil {
		return "", "", err
	}

	takerSigBytes, err := hex.DecodeString(takerSignatureHex)
	if err != nil {
		return "", "", err
	}

	tx.TxIn[0].Witness = onchain.GetCooperativeWitness(takerSigBytes, sigBytes.Serialize(), redeemScript)

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

func (b *ClightningClient) CreateRefundAddress() (string, error) {
	newAddr, err := b.glightning.NewAddr()
	if err != nil {
		return "", err
	}
	return newAddr, nil
}

func (b *ClightningClient) prepareSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, spendingAddr, openingTxHex string, vout uint32, csv uint32, preparedFee uint64) (tx *wire.MsgTx, sigHash, redeemScript []byte, err error) {
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

	scriptChangeAddr, err := btcutil.DecodeAddress(spendingAddr, b.bitcoinNetwork)
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

	redeemScript, err = onchain.ParamsToTxScript(swapParams, onchain.BitcoinCsv)
	if err != nil {
		return nil, nil, nil, err
	}

	spendingTxInput := wire.NewTxIn(prevInput, nil, [][]byte{})
	spendingTxInput.Sequence = 0 | csv
	spendingTx.AddTxIn(spendingTxInput)

	// assume largest witness
	fee := preparedFee
	if preparedFee == 0 {
		fee, err = b.getFee(spendingTx.SerializeSizeStripped() + 74)
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

func (b *ClightningClient) createOpeningAddress(params *swap.OpeningParams, csv uint32) (string, error) {
	redeemScript, err := onchain.ParamsToTxScript(params, csv)
	if err != nil {
		return "", err
	}
	witnessProgram := sha256.Sum256(redeemScript)
	addr, err := btcutil.NewAddressWitnessScriptHash(witnessProgram[:], b.bitcoinNetwork)
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

func (b *ClightningClient) getFee(txSize int) (uint64, error) {
	feeRes, err := b.gbitcoin.EstimateFee(onchain.Bitcoin_Target_Blocks, "ECONOMICAL")
	if err != nil {
		return 0, err
	}
	satPerByte := float64(feeRes.SatPerKb()) / float64(1000)
	if len(feeRes.Errors) > 0 {
		//todo sane default sat per byte
		satPerByte = 5
	}
	// assume largest witness
	fee := satPerByte * float64(txSize)
	return uint64(fee), nil
}
func (b *ClightningClient) GetRefundFee() (uint64, error) {
	// todo correct size estimation
	return b.getFee(250)
}
