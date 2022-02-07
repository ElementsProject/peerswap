package clightning

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/sputn1ck/glightning/glightning"
	"github.com/sputn1ck/peerswap/lightning"
	"github.com/sputn1ck/peerswap/onchain"
	"github.com/sputn1ck/peerswap/swap"
)

func (cl *ClightningClient) CreateOpeningTransaction(swapParams *swap.OpeningParams) (unpreparedTxHex string, fee uint64, vout uint32, err error) {
	addr, err := cl.bitcoinChain.CreateOpeningAddress(swapParams, onchain.BitcoinCsv)
	if err != nil {
		return "", 0, 0, err
	}
	outputs := []*glightning.Outputs{
		{
			Address: addr,
			Satoshi: swapParams.Amount,
		},
	}
	prepRes, err := cl.glightning.PrepareTx(outputs, &glightning.FeeRate{Directive: glightning.Urgent}, nil)
	if err != nil {
		return "", 0, 0, err
	}

	fee, err = cl.bitcoinChain.GetFeeSatsFromTx(prepRes.Psbt, prepRes.UnsignedTx)
	if err != nil {
		return "", 0, 0, err
	}

	_, vout, err = cl.bitcoinChain.GetVoutAndVerify(prepRes.UnsignedTx, swapParams)
	if err != nil {
		return "", 0, 0, err
	}
	cl.hexToIdMap[prepRes.UnsignedTx] = prepRes.TxId
	return prepRes.UnsignedTx, fee, vout, nil
}

func (cl *ClightningClient) BroadcastOpeningTx(unpreparedTxHex string) (txId, txHex string, error error) {
	var unpreparedTxId string
	var ok bool
	if unpreparedTxId, ok = cl.hexToIdMap[unpreparedTxHex]; !ok {
		return "", "", errors.New("tx was not prepared not found in map")
	}
	delete(cl.hexToIdMap, unpreparedTxHex)
	sendRes, err := cl.glightning.SendTx(unpreparedTxId)
	if err != nil {
		return "", "", errors.New(fmt.Sprintf("tx was not prepared %v", err))
	}
	return sendRes.TxId, sendRes.SignedTx, nil
}

func (cl *ClightningClient) CreatePreimageSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams) (txId, txHex string, err error) {

	_, vout, err := cl.bitcoinChain.GetVoutAndVerify(claimParams.OpeningTxHex, swapParams)
	if err != nil {
		return "", "", err
	}

	newAddr, err := cl.glightning.NewAddr()
	if err != nil {
		return "", "", err
	}

	tx, sigHash, redeemScript, err := cl.bitcoinChain.PrepareSpendingTransaction(swapParams, claimParams, newAddr, vout, 0, 0)
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

	txId, err = cl.gbitcoin.SendRawTx(txHex)
	if err != nil {
		return "", "", err
	}
	return txId, txHex, nil
}

func (cl *ClightningClient) CreateCsvSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams) (txId, txHex string, error error) {
	newAddr, err := cl.glightning.NewAddr()
	if err != nil {
		return "", "", err
	}

	_, vout, err := cl.bitcoinChain.GetVoutAndVerify(claimParams.OpeningTxHex, swapParams)
	if err != nil {
		return "", "", err
	}

	tx, sigHash, redeemScript, err := cl.bitcoinChain.PrepareSpendingTransaction(swapParams, claimParams, newAddr, vout, onchain.BitcoinCsv, 0)
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

	txId, err = cl.gbitcoin.SendRawTx(txHex)
	if err != nil {
		return "", "", err
	}
	return txId, txHex, nil
}

func (cl *ClightningClient) CreateCoopSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, takerSigner swap.Signer) (txId, txHex string, error error) {
	refundAddr, err := cl.NewAddress()
	if err != nil {
		return "", "", err
	}
	refundFee, err := cl.GetRefundFee()
	if err != nil {
		return "", "", err
	}
	_, vout, err := cl.bitcoinChain.GetVoutAndVerify(claimParams.OpeningTxHex, swapParams)
	if err != nil {
		return "", "", err
	}
	spendingTx, sigHashBytes, redeemScript, err := cl.bitcoinChain.PrepareSpendingTransaction(swapParams, claimParams, refundAddr, vout, 0, refundFee)
	if err != nil {
		return "", "", err
	}

	takerSig, err := takerSigner.Sign(sigHashBytes[:])
	if err != nil {
		return "", "", err
	}
	makerSig, err := claimParams.Signer.Sign(sigHashBytes[:])
	if err != nil {
		return "", "", err
	}

	spendingTx.TxIn[0].Witness = onchain.GetCooperativeWitness(takerSig.Serialize(), makerSig.Serialize(), redeemScript)

	bytesBuffer := new(bytes.Buffer)

	err = spendingTx.Serialize(bytesBuffer)
	if err != nil {
		return "", "", err
	}

	txHex = hex.EncodeToString(bytesBuffer.Bytes())

	txId, err = cl.gbitcoin.SendRawTx(txHex)
	if err != nil {
		return "", "", err
	}
	return spendingTx.TxHash().String(), txHex, nil
}

func (cl *ClightningClient) NewAddress() (string, error) {
	newAddr, err := cl.glightning.NewAddr()
	if err != nil {
		return "", err
	}
	return newAddr, nil
}

func (cl *ClightningClient) GetOutputScript(params *swap.OpeningParams) ([]byte, error) {
	return cl.bitcoinChain.GetOutputScript(params)
}

func (cl *ClightningClient) GetRefundFee() (uint64, error) {
	// todo correct size estimation
	return cl.bitcoinChain.GetFee(250)
}

func (cl *ClightningClient) GetFeePerKw(targetblocks uint32) (float64, error) {
	feeRes, err := cl.gbitcoin.EstimateFee(targetblocks, "ECONOMICAL")
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

func (cl *ClightningClient) GetAsset() string {
	return ""
}

func (cl *ClightningClient) GetNetwork() string {
	return cl.bitcoinChain.GetChain().Name
}

func (cl *ClightningClient) EstimateTxFee(swapAmount uint64) (uint64, error) {
	return cl.bitcoinChain.GetFee(2 * 250)
}
