package lnd

import (
	"bytes"
	"context"
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

func (l *Lnd) CreatePreimageSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams) (string, string, error) {
	_, vout, err := l.bitcoinOnChain.GetVoutAndVerify(claimParams.OpeningTxHex, swapParams)
	if err != nil {
		return "", "", err
	}

	newAddr, err := l.NewAddress()
	if err != nil {
		return "", "", err
	}

	tx, sigHash, redeemScript, err := l.bitcoinOnChain.PrepareSpendingTransaction(swapParams, claimParams, newAddr, vout, 0, 0)
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

func (l *Lnd) CreateCsvSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams) (string, string, error) {
	newAddr, err := l.NewAddress()
	if err != nil {
		return "", "", err
	}
	_, vout, err := l.bitcoinOnChain.GetVoutAndVerify(claimParams.OpeningTxHex, swapParams)
	if err != nil {
		return "", "", err
	}
	tx, sigHash, redeemScript, err := l.bitcoinOnChain.PrepareSpendingTransaction(swapParams, claimParams, newAddr, vout, onchain.BitcoinCsv, 0)
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

func (l *Lnd) CreateCoopSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, takerSigner swap.Signer) (txId, txHex string, error error) {
	refundAddr, err := l.NewAddress()
	if err != nil {
		return "", "", err
	}
	refundFee, err := l.GetRefundFee()
	if err != nil {
		return "", "", err
	}
	_, vout, err := l.bitcoinOnChain.GetVoutAndVerify(claimParams.OpeningTxHex, swapParams)
	if err != nil {
		return "", "", err
	}
	spendingTx, sigHashBytes, redeemScript, err := l.bitcoinOnChain.PrepareSpendingTransaction(swapParams, claimParams, refundAddr, vout, 0, refundFee)
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

	_, err = l.walletClient.PublishTransaction(l.ctx, &walletrpc.Transaction{TxHex: bytesBuffer.Bytes()})
	if err != nil {
		return "", "", err
	}
	return spendingTx.TxHash().String(), txHex, nil
}

func (l *Lnd) GetOutputScript(params *swap.OpeningParams) ([]byte, error) {
	return l.bitcoinOnChain.GetOutputScript(params)
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
func (cl *Lnd) GetAsset() string {
	return ""
}

func (cl *Lnd) GetNetwork() string {
	return cl.bitcoinOnChain.GetChain().Name
}

type LndFeeEstimator struct {
	ctx       context.Context
	walletkit walletrpc.WalletKitClient
	lndrpc    lnrpc.LightningClient
}

func NewLndFeeEstimator(ctx context.Context, walletkit walletrpc.WalletKitClient) *LndFeeEstimator {
	return &LndFeeEstimator{ctx: ctx, walletkit: walletkit}
}

func (l *LndFeeEstimator) GetFeePerKw(targetBlocks uint32) (float64, error) {
	res, err := l.walletkit.EstimateFee(l.ctx, &walletrpc.EstimateFeeRequest{ConfTarget: int32(targetBlocks)})
	if err != nil {
		return 0, err
	}

	return float64(res.SatPerKw / 4000), nil
}
