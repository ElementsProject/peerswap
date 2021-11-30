package lnd

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil/psbt"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/walletrpc"
	"github.com/sputn1ck/peerswap/lightning"
	"github.com/sputn1ck/peerswap/onchain"
	"github.com/sputn1ck/peerswap/swap"
)

func (l *Lnd) CreateOpeningTransaction(swapParams *swap.OpeningParams) (unpreparedTxHex string, fee uint64, vout uint32, err error) {
	addr, err := l.bitcoinOnChain.CreateOpeningAddress(swapParams, onchain.BitcoinCsv)
	if err != nil {
		return "", 0, 0, err
	}

	fundPsbtTemplate := &walletrpc.TxTemplate{
		Outputs: map[string]uint64{
			addr: swapParams.Amount,
		},
	}
	fundRes, err := l.walletClient.FundPsbt(l.ctx, &walletrpc.FundPsbtRequest{
		Template: &walletrpc.FundPsbtRequest_Raw{Raw: fundPsbtTemplate},
		Fees:     &walletrpc.FundPsbtRequest_TargetConf{TargetConf: 3},
	})
	if err != nil {
		return "", 0, 0, err
	}
	unsignedPacket, err := psbt.NewFromRawBytes(bytes.NewReader(fundRes.FundedPsbt), false)
	if err != nil {
		return "", 0, 0, err
	}

	bytesBuffer := new(bytes.Buffer)
	err = unsignedPacket.Serialize(bytesBuffer)
	if err != nil {
		return "", 0, 0, err
	}
	finalizeRes, err := l.walletClient.FinalizePsbt(l.ctx, &walletrpc.FinalizePsbtRequest{
		FundedPsbt: bytesBuffer.Bytes(),
	})
	if err != nil {
		return "", 0, 0, err
	}
	psbtString := base64.StdEncoding.EncodeToString(finalizeRes.SignedPsbt)
	rawTxHex := hex.EncodeToString(finalizeRes.RawFinalTx)

	fee, err = l.bitcoinOnChain.GetFeeSatsFromTx(psbtString, rawTxHex)
	if err != nil {
		return "", 0, 0, err
	}

	_, vout, err = l.bitcoinOnChain.GetVoutAndVerify(rawTxHex, swapParams)
	if err != nil {
		return "", 0, 0, err
	}
	return rawTxHex, fee, vout, nil
}

func (l *Lnd) BroadcastOpeningTx(unpreparedTxHex string) (txId, txHex string, error error) {
	txBytes, err := hex.DecodeString(unpreparedTxHex)
	if err != nil {
		return "", "", err
	}
	openingTx := wire.NewMsgTx(2)
	err = openingTx.Deserialize(bytes.NewReader(txBytes))
	if err != nil {
		return "", "", err
	}

	_, err = l.walletClient.PublishTransaction(l.ctx, &walletrpc.Transaction{TxHex: txBytes})
	if err != nil {
		return "", "", err
	}
	return openingTx.TxHash().String(), unpreparedTxHex, nil
}

func (l *Lnd) CreatePreimageSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, openingTxId string) (string, string, error) {
	openingTxHex, err := l.bitcoinOnChain.GetRawTxFromTxId(openingTxId, 0)
	if err != nil {
		return "", "", err
	}

	_, vout, err := l.bitcoinOnChain.GetVoutAndVerify(openingTxHex, swapParams)
	if err != nil {
		return "", "", err
	}

	newAddr, err := l.NewAddress()
	if err != nil {
		return "", "", err
	}

	tx, sigHash, redeemScript, err := l.bitcoinOnChain.PrepareSpendingTransaction(swapParams, claimParams, newAddr, openingTxHex, vout, 0, 0)
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

	txHex := hex.EncodeToString(bytesBuffer.Bytes())

	_, err = l.walletClient.PublishTransaction(l.ctx, &walletrpc.Transaction{TxHex: bytesBuffer.Bytes()})
	if err != nil {
		return "", "", err
	}
	return tx.TxHash().String(), txHex, nil
}

func (l *Lnd) CreateCsvSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, openingTxHex string, vout uint32) (string, string, error) {
	newAddr, err := l.NewAddress()
	if err != nil {
		return "", "", err
	}

	tx, sigHash, redeemScript, err := l.bitcoinOnChain.PrepareSpendingTransaction(swapParams, claimParams, newAddr, openingTxHex, vout, onchain.BitcoinCsv, 0)
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

	txHex := hex.EncodeToString(bytesBuffer.Bytes())

	_, err = l.walletClient.PublishTransaction(l.ctx, &walletrpc.Transaction{TxHex: bytesBuffer.Bytes()})
	if err != nil {
		return "", "", err
	}
	return tx.TxHash().String(), txHex, nil
}

func (l *Lnd) TakerCreateCoopSigHash(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, openingTxId, refundAddress string, refundFee uint64) (sigHash string, error error) {
	openingTxHex, err := l.bitcoinOnChain.GetRawTxFromTxId(openingTxId, 0)
	if err != nil {
		return "", err
	}
	_, vout, err := l.bitcoinOnChain.GetVoutAndVerify(openingTxHex, swapParams)
	if err != nil {
		return "", err
	}
	_, sigHashBytes, _, err := l.bitcoinOnChain.PrepareSpendingTransaction(swapParams, claimParams, refundAddress, openingTxHex, vout, 0, refundFee)
	if err != nil {
		return "", err
	}
	sigBytes, err := claimParams.Signer.Sign(sigHashBytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sigBytes.Serialize()), nil
}

func (l *Lnd) CreateCooperativeSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, refundAddress, openingTxHex string, vout uint32, takerSignatureHex string, refundFee uint64) (string, string, error) {
	tx, sigHashBytes, redeemScript, err := l.bitcoinOnChain.PrepareSpendingTransaction(swapParams, claimParams, refundAddress, openingTxHex, vout, 0, refundFee)
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

	txHex := hex.EncodeToString(bytesBuffer.Bytes())

	_, err = l.walletClient.PublishTransaction(l.ctx, &walletrpc.Transaction{TxHex: bytesBuffer.Bytes()})
	if err != nil {
		return "", "", err
	}
	return tx.TxHash().String(), txHex, nil
}

func (l *Lnd) NewAddress() (string, error) {
	res, err := l.lndClient.NewAddress(l.ctx, &lnrpc.NewAddressRequest{Type: lnrpc.AddressType_WITNESS_PUBKEY_HASH})
	if err != nil {
		return "", err
	}
	return res.Address, nil
}

func (l *Lnd) GetRefundFee() (uint64, error) {
	return l.bitcoinOnChain.GetFee(250)
}
