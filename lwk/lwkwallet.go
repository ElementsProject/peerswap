package lwk

import (
	"context"
	"errors"
	"log"

	"github.com/checksum0/go-electrum/electrum"

	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/wallet"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/psetv2"
)

const (
	kiloByte          = 1000
	minimumSatPerByte = 0.1
)

// LWKRpcWallet uses the elementsd rpc wallet
type LWKRpcWallet struct {
	walletName     string
	signerName     string
	lwkClient      *lwkclient
	electrumClient *electrum.Client
}

func NewLWKRpcWallet(lwkClient *lwkclient, electrumClient *electrum.Client, walletName, signerName string) (*LWKRpcWallet, error) {
	if lwkClient == nil || electrumClient == nil {
		return nil, errors.New("rpc client is nil")
	}
	if walletName == "" || signerName == "" {
		return nil, errors.New("wallet name or signer name is empty")
	}
	rpcWallet := &LWKRpcWallet{
		walletName:     walletName,
		signerName:     signerName,
		lwkClient:      lwkClient,
		electrumClient: electrumClient,
	}
	err := rpcWallet.setupWallet()
	if err != nil {
		return nil, err
	}
	return rpcWallet, nil
}

// setupWallet checks if the swap wallet is already loaded in elementsd, if not it loads/creates it
func (r *LWKRpcWallet) setupWallet() error {
	ctx := context.Background()
	res, err := r.lwkClient.walletDetails(ctx, &walletDetailsRequest{
		WalletName: r.walletName,
	})
	if err != nil {
		return err
	}
	signers := res.Signers
	if len(signers) != 1 {
		return errors.New("invalid number of signers")
	}
	if signers[0].Name != r.signerName {
		return errors.New("signer name is not correct. expected: " + r.signerName + " got: " + signers[0].Name)
	}
	return nil
}

// CreateFundedTransaction takes a tx with outputs and adds inputs in order to spend the tx
func (r *LWKRpcWallet) CreateAndBroadcastTransaction(swapParams *swap.OpeningParams,
	asset []byte) (txid, rawTx string, fee uint64, err error) {
	ctx := context.Background()
	fundedTx, err := r.lwkClient.send(ctx, &sendRequest{
		Addressees: []*unvalidatedAddressee{
			{
				Address: swapParams.OpeningAddress,
				Satoshi: swapParams.Amount,
			},
		},
		WalletName: r.walletName,
	})
	if err != nil {
		return "", "", 0, err
	}
	signed, err := r.lwkClient.sign(ctx, &signRequest{
		SignerName: r.signerName,
		Pset:       fundedTx.Pset,
	})
	if err != nil {
		return "", "", 0, err
	}
	broadcasted, err := r.lwkClient.broadcast(ctx, &broadcastRequest{
		WalletName: r.walletName,
		Pset:       signed.Pset,
	})
	if err != nil {
		return "", "", 0, err
	}
	p, err := psetv2.NewPsetFromBase64(signed.Pset)
	if err != nil {
		return "", "", 0, err
	}
	err = psetv2.FinalizeAll(p)
	if err != nil {
		return "", "", 0, err
	}
	tx, err := psetv2.Extract(p)
	if err != nil {
		return "", "", 0, err
	}
	txhex, err := tx.ToHex()
	if err != nil {
		return "", "", 0, err
	}
	return broadcasted.Txid, txhex, 0, nil
}

// GetBalance returns the balance in sats
func (r *LWKRpcWallet) GetBalance() (uint64, error) {
	ctx := context.Background()
	balance, err := r.lwkClient.balance(ctx, &balanceRequest{
		WalletName: r.walletName,
	})
	if err != nil {
		return 0, err
	}
	return uint64(balance.Balance[network.Regtest.AssetID]), nil
}

// GetAddress returns a new blech32 address
func (r *LWKRpcWallet) GetAddress() (string, error) {
	ctx := context.Background()
	address, err := r.lwkClient.address(ctx, &addressRequest{
		WalletName: r.walletName})
	if err != nil {
		return "", err
	}
	return address.Address, nil
}

// SendToAddress sends an amount to an address
func (r *LWKRpcWallet) SendToAddress(address string, amount uint64) (string, error) {
	ctx := context.Background()
	sendres, err := r.lwkClient.send(ctx, &sendRequest{
		WalletName: r.walletName,
		Addressees: []*unvalidatedAddressee{
			{
				Address: address,
				Satoshi: amount,
			},
		},
	})
	if err != nil {
		return "", err
	}

	signed, err := r.lwkClient.sign(ctx, &signRequest{
		SignerName: r.signerName,
		Pset:       sendres.Pset,
	})
	if err != nil {
		log.Fatal(err)
	}
	broadcastres, err := r.lwkClient.broadcast(ctx, &broadcastRequest{
		WalletName: r.walletName,
		Pset:       signed.Pset,
	})
	if err != nil {
		return "", err
	}
	return broadcastres.Txid, nil
}

func (r *LWKRpcWallet) SendRawTx(txHex string) (string, error) {
	ctx := context.Background()
	res, err := r.electrumClient.BroadcastTransaction(ctx, txHex)
	if err != nil {
		return "", err
	}
	return res, nil
}

func (r *LWKRpcWallet) GetFee(txSize int64) (uint64, error) {
	ctx := context.Background()
	feeRes, err := r.electrumClient.GetFee(ctx, wallet.LiquidTargetBlocks)

	satPerByte := float64(feeRes) / float64(kiloByte)
	if satPerByte < minimumSatPerByte {
		satPerByte = minimumSatPerByte
	}
	if err != nil {
		satPerByte = minimumSatPerByte
	}
	// assume largest witness
	fee := satPerByte * float64(txSize)

	return uint64(fee), nil
}

func (r *LWKRpcWallet) SetLabel(txID, address, label string) error {
	// TODO: call set label
	return nil
}
