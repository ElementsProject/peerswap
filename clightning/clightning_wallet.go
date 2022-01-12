package clightning

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"log"

	"github.com/sputn1ck/glightning/glightning"
	"github.com/sputn1ck/peerswap/lightning"
	"github.com/sputn1ck/peerswap/onchain"
	"github.com/sputn1ck/peerswap/swap"
)

func (b *ClightningClient) CreateOpeningTransaction(swapParams *swap.OpeningParams) (unpreparedTxHex string, fee uint64, vout uint32, err error) {
	addr, err := b.bitcoinChain.CreateOpeningAddress(swapParams, onchain.BitcoinCsv)
	if err != nil {
		return "", 0, 0, err
	}
	outputs := []*glightning.Outputs{
		{
			Address: addr,
			Satoshi: swapParams.Amount,
		},
	}
	prepRes, err := b.glightning.PrepareTx(outputs, &glightning.FeeRate{Directive: glightning.Urgent}, nil)
	if err != nil {
		return "", 0, 0, err
	}

	fee, err = b.bitcoinChain.GetFeeSatsFromTx(prepRes.Psbt, prepRes.UnsignedTx)
	if err != nil {
		return "", 0, 0, err
	}

	_, vout, err = b.bitcoinChain.GetVoutAndVerify(prepRes.UnsignedTx, swapParams)
	if err != nil {
		return "", 0, 0, err
	}
	b.hexToIdMap[prepRes.UnsignedTx] = prepRes.TxId
	return prepRes.UnsignedTx, fee, vout, nil
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

func (b *ClightningClient) CreatePreimageSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams) (txId, txHex string, err error) {

	_, vout, err := b.bitcoinChain.GetVoutAndVerify(claimParams.OpeningTxHex, swapParams)
	if err != nil {
		return "", "", err
	}

	newAddr, err := b.glightning.NewAddr()
	if err != nil {
		return "", "", err
	}

	tx, sigHash, redeemScript, err := b.bitcoinChain.PrepareSpendingTransaction(swapParams, claimParams, newAddr, vout, 0, 0)
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

func (b *ClightningClient) CreateCsvSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams) (txId, txHex string, error error) {
	newAddr, err := b.glightning.NewAddr()
	if err != nil {
		return "", "", err
	}

	_, vout, err := b.bitcoinChain.GetVoutAndVerify(claimParams.OpeningTxHex, swapParams)
	if err != nil {
		return "", "", err
	}

	tx, sigHash, redeemScript, err := b.bitcoinChain.PrepareSpendingTransaction(swapParams, claimParams, newAddr, vout, onchain.BitcoinCsv, 0)
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

func (b *ClightningClient) TakerCreateCoopSigHash(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, refundAddress string, refundFee uint64) (sigHash string, error error) {
	_, vout, err := b.bitcoinChain.GetVoutAndVerify(claimParams.OpeningTxHex, swapParams)
	if err != nil {
		return "", err
	}
	_, sigHashBytes, _, err := b.bitcoinChain.PrepareSpendingTransaction(swapParams, claimParams, refundAddress, vout, 0, refundFee)
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

func (b *ClightningClient) CreateCooperativeSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, refundAddress string, takerSignatureHex string, refundFee uint64) (txId, txHex string, error error) {
	_, vout, err := b.bitcoinChain.GetVoutAndVerify(claimParams.OpeningTxHex, swapParams)
	if err != nil {
		return "", "", err
	}
	tx, sigHashBytes, redeemScript, err := b.bitcoinChain.PrepareSpendingTransaction(swapParams, claimParams, refundAddress, vout, 0, refundFee)
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

func (b *ClightningClient) NewAddress() (string, error) {
	newAddr, err := b.glightning.NewAddr()
	if err != nil {
		return "", err
	}
	return newAddr, nil
}

func (b *ClightningClient) GetOutputScript(params *swap.OpeningParams) ([]byte, error) {
	return b.bitcoinChain.GetOutputScript(params)
}

func (b *ClightningClient) GetRefundFee() (uint64, error) {
	// todo correct size estimation
	return b.bitcoinChain.GetFee(250)
}

func (b *ClightningClient) GetFeePerKw(targetblocks uint32) (float64, error) {
	feeRes, err := b.gbitcoin.EstimateFee(targetblocks, "ECONOMICAL")
	if err != nil {
		return 0, err
	}

	satPerByte := float64(feeRes.SatPerKb()) / float64(1000)
	if len(feeRes.Errors) > 0 {
		//todo sane default sat per byte
		satPerByte = 10
	}
	return satPerByte, nil
}
