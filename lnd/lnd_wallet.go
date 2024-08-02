package lnd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"

	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/elementsproject/peerswap/lightning"
	"github.com/elementsproject/peerswap/onchain"
	"github.com/elementsproject/peerswap/swap"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/walletrpc"
)

func (l *Client) CreateOpeningTransaction(swapParams *swap.OpeningParams) (rawTxHex, address, txId string, fee uint64, vout uint32, err error) {
	addr, err := l.bitcoinOnChain.CreateOpeningAddress(swapParams, onchain.BitcoinCsv)
	if err != nil {
		return "", "", "", 0, 0, err
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
		return "", "", "", 0, 0, err
	}
	unsignedPacket, err := psbt.NewFromRawBytes(bytes.NewReader(fundRes.FundedPsbt), false)
	if err != nil {
		return "", "", "", 0, 0, err
	}

	bytesBuffer := new(bytes.Buffer)
	err = unsignedPacket.Serialize(bytesBuffer)
	if err != nil {
		return "", "", "", 0, 0, err
	}
	finalizeRes, err := l.walletClient.FinalizePsbt(l.ctx, &walletrpc.FinalizePsbtRequest{
		FundedPsbt: bytesBuffer.Bytes(),
	})
	if err != nil {
		return "", "", "", 0, 0, err
	}
	psbtString := base64.StdEncoding.EncodeToString(finalizeRes.SignedPsbt)
	rawTxHex = hex.EncodeToString(finalizeRes.RawFinalTx)

	fee, err = l.bitcoinOnChain.GetFeeSatsFromTx(psbtString, rawTxHex)
	if err != nil {
		return "", "", "", 0, 0, err
	}

	_, vout, err = l.bitcoinOnChain.GetVoutAndVerify(rawTxHex, swapParams)
	if err != nil {
		return "", "", "", 0, 0, err
	}
	txBytes, err := hex.DecodeString(rawTxHex)
	if err != nil {
		return "", "", "", 0, 0, err
	}
	openingTx := wire.NewMsgTx(2)
	err = openingTx.Deserialize(bytes.NewReader(txBytes))
	if err != nil {
		return "", "", "", 0, 0, err
	}

	_, err = l.walletClient.PublishTransaction(l.ctx, &walletrpc.Transaction{TxHex: txBytes})
	if err != nil {
		return "", "", "", 0, 0, err
	}
	return rawTxHex, addr, openingTx.TxHash().String(), fee, vout, nil
}

func (l *Client) CreatePreimageSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams) (string, string, string, error) {
	_, vout, err := l.bitcoinOnChain.GetVoutAndVerify(claimParams.OpeningTxHex, swapParams)
	if err != nil {
		return "", "", "", err
	}

	newAddr, err := l.NewAddress()
	if err != nil {
		return "", "", "", err
	}

	tx, sigHash, redeemScript, err := l.bitcoinOnChain.PrepareSpendingTransaction(swapParams, claimParams, newAddr, vout, 0, 0)
	if err != nil {
		return "", "", "", err
	}
	sigBytes, err := claimParams.Signer.Sign(sigHash)
	if err != nil {
		return "", "", "", err
	}

	preimage, err := lightning.MakePreimageFromStr(claimParams.Preimage)
	if err != nil {
		return "", "", "", err
	}

	tx.TxIn[0].Witness = onchain.GetPreimageWitness(sigBytes.Serialize(), preimage[:], redeemScript)

	bytesBuffer := new(bytes.Buffer)

	err = tx.Serialize(bytesBuffer)
	if err != nil {
		return "", "", "", err
	}

	txHex := hex.EncodeToString(bytesBuffer.Bytes())

	_, err = l.walletClient.PublishTransaction(l.ctx, &walletrpc.Transaction{TxHex: bytesBuffer.Bytes()})
	if err != nil {
		return "", "", "", err
	}
	return tx.TxHash().String(), txHex, newAddr, nil
}

func (l *Client) CreateCsvSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams) (string, string, string, error) {
	newAddr, err := l.NewAddress()
	if err != nil {
		return "", "", "", err
	}
	_, vout, err := l.bitcoinOnChain.GetVoutAndVerify(claimParams.OpeningTxHex, swapParams)
	if err != nil {
		return "", "", "", err
	}
	tx, sigHash, redeemScript, err := l.bitcoinOnChain.PrepareSpendingTransaction(swapParams, claimParams, newAddr, vout, onchain.BitcoinCsv, 0)
	if err != nil {
		return "", "", "", err
	}

	sigBytes, err := claimParams.Signer.Sign(sigHash)
	if err != nil {
		return "", "", "", err
	}

	tx.TxIn[0].Witness = onchain.GetCsvWitness(sigBytes.Serialize(), redeemScript)

	bytesBuffer := new(bytes.Buffer)

	err = tx.Serialize(bytesBuffer)
	if err != nil {
		return "", "", "", err
	}

	txHex := hex.EncodeToString(bytesBuffer.Bytes())

	_, err = l.walletClient.PublishTransaction(l.ctx, &walletrpc.Transaction{TxHex: bytesBuffer.Bytes()})
	if err != nil {
		return "", "", "", err
	}
	return tx.TxHash().String(), txHex, newAddr, nil
}

func (l *Client) CreateCoopSpendingTransaction(swapParams *swap.OpeningParams, claimParams *swap.ClaimParams, takerSigner swap.Signer) (txId, txHex, address string, error error) {
	refundAddr, err := l.NewAddress()
	if err != nil {
		return "", "", "", err
	}
	refundFee, err := l.GetRefundFee()
	if err != nil {
		return "", "", "", err
	}
	_, vout, err := l.bitcoinOnChain.GetVoutAndVerify(claimParams.OpeningTxHex, swapParams)
	if err != nil {
		return "", "", "", err
	}
	spendingTx, sigHashBytes, redeemScript, err := l.bitcoinOnChain.PrepareSpendingTransaction(swapParams, claimParams, refundAddr, vout, 0, refundFee)
	if err != nil {
		return "", "", "", err
	}

	takerSig, err := takerSigner.Sign(sigHashBytes[:])
	if err != nil {
		return "", "", "", err
	}
	makerSig, err := claimParams.Signer.Sign(sigHashBytes[:])
	if err != nil {
		return "", "", "", err
	}

	spendingTx.TxIn[0].Witness = onchain.GetCooperativeWitness(takerSig.Serialize(), makerSig.Serialize(), redeemScript)

	bytesBuffer := new(bytes.Buffer)

	err = spendingTx.Serialize(bytesBuffer)
	if err != nil {
		return "", "", "", err
	}

	txHex = hex.EncodeToString(bytesBuffer.Bytes())

	_, err = l.walletClient.PublishTransaction(l.ctx, &walletrpc.Transaction{TxHex: bytesBuffer.Bytes()})
	if err != nil {
		return "", "", "", err
	}
	return spendingTx.TxHash().String(), txHex, refundAddr, nil
}

// SetLabel labels a transaction with a given label.
// This makes it easier to audit the transactions from faraday.
// This is performed by LND's LabelTransaction RPC.
func (l *Client) SetLabel(txID, address, label string) error {
	txIDHash, err := chainhash.NewHashFromStr(txID)
	if err != nil {
		return err
	}
	_, err = l.walletClient.LabelTransaction(l.ctx,
		&walletrpc.LabelTransactionRequest{
			Txid:      txIDHash.CloneBytes(),
			Label:     label,
			Overwrite: true})
	return err
}

func (l *Client) GetOnchainBalance() (uint64, error) {
	res, err := l.lndClient.WalletBalance(l.ctx, &lnrpc.WalletBalanceRequest{})
	if err != nil {
		return 0, err
	}

	return uint64(res.TotalBalance), nil
}

func (l *Client) GetOutputScript(params *swap.OpeningParams) ([]byte, error) {
	return l.bitcoinOnChain.GetOutputScript(params)
}

func (l *Client) NewAddress() (string, error) {
	res, err := l.lndClient.NewAddress(l.ctx, &lnrpc.NewAddressRequest{Type: lnrpc.AddressType_WITNESS_PUBKEY_HASH})
	if err != nil {
		return "", err
	}
	return res.Address, nil
}

func (l *Client) GetRefundFee() (uint64, error) {
	return l.bitcoinOnChain.GetFee(250)
}

// GetFlatOpeningTXFee returns an estimated size for the opening transaction. This
// can be used to calculate the amount of the fee invoice and should cover most
// but not all cases. For an explanation of the estimation see comments of the
// onchain.EstimatedOpeningTxSize.
func (l *Client) GetFlatOpeningTXFee() (uint64, error) {
	return l.bitcoinOnChain.GetFee(onchain.EstimatedOpeningTxSize)
}

func (cl *Client) GetAsset() string {
	return ""
}

func (cl *Client) GetNetwork() string {
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
