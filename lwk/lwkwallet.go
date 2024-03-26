package lwk

import (
	"context"
	"errors"
	"log"
	"math"
	"strings"

	"github.com/checksum0/go-electrum/electrum"

	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/wallet"
	"github.com/vulpemventures/go-elements/network"
)

// Satoshi represents a satoshi value.
type Satoshi = uint64

// SatPerKVByte represents a fee rate in sat/kb.
type SatPerKVByte = uint64

const (
	minimumSatPerByte = 200
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
	err := rpcWallet.setupWallet(context.Background())
	if err != nil {
		return nil, err
	}
	return rpcWallet, nil
}

// setupWallet checks if the swap wallet is already loaded in elementsd, if not it loads/creates it
func (r *LWKRpcWallet) setupWallet(ctx context.Context) error {
	res, err := r.lwkClient.walletDetails(ctx, &walletDetailsRequest{
		WalletName: r.walletName,
	})
	if err != nil {
		// 32008 is the error code for wallet not found of lwk
		if strings.HasPrefix(err.Error(), "-32008") {
			return r.createWallet(ctx, r.walletName, r.signerName)
		}
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

func (r *LWKRpcWallet) createWallet(ctx context.Context, walletName, signerName string) error {
	res, err := r.lwkClient.generateSigner(ctx)
	if err != nil {
		return err
	}
	_, err = r.lwkClient.loadSoftwareSigner(ctx, &loadSoftwareSignerRequest{
		Mnemonic:   res.Mnemonic,
		SignerName: signerName,
	})
	// 32011 is the error code for signer already loaded
	if err != nil && !strings.HasPrefix(err.Error(), "-32011") {
		return err
	}
	descres, err := r.lwkClient.singlesigDescriptor(ctx, &singlesigDescriptorRequest{
		SignerName:            signerName,
		DescriptorBlindingKey: "slip77",
		SinglesigKind:         "wpkh",
	})
	if err != nil {
		return err
	}
	_, err = r.lwkClient.loadWallet(ctx, &loadWalletRequest{
		Descriptor: descres.Descriptor,
		WalletName: walletName,
	})
	// 32011 is the error code for wallet already loaded
	if err != nil && !strings.HasPrefix(err.Error(), "-32009") {
		return err
	}
	return nil
}

// CreateFundedTransaction takes a tx with outputs and adds inputs in order to spend the tx
func (r *LWKRpcWallet) CreateAndBroadcastTransaction(swapParams *swap.OpeningParams,
	asset []byte) (txid, rawTx string, fee SatPerKVByte, err error) {
	ctx := context.Background()
	feerate := float64(r.getFeePerKb(ctx))
	fundedTx, err := r.lwkClient.send(ctx, &sendRequest{
		Addressees: []*unvalidatedAddressee{
			{
				Address: swapParams.OpeningAddress,
				Satoshi: swapParams.Amount,
			},
		},
		WalletName: r.walletName,
		FeeRate:    &feerate,
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
	hex, err := r.electrumClient.GetRawTransaction(ctx, broadcasted.Txid)
	if err != nil {
		return "", "", 0, err
	}
	return broadcasted.Txid, hex, 0, nil
}

// GetBalance returns the balance in sats
func (r *LWKRpcWallet) GetBalance() (Satoshi, error) {
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
func (r *LWKRpcWallet) SendToAddress(address string, amount Satoshi) (string, error) {
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

func (r *LWKRpcWallet) getFeePerKb(ctx context.Context) SatPerKVByte {
	feeBTCPerKb, err := r.electrumClient.GetFee(ctx, wallet.LiquidTargetBlocks)
	// convert to sat per byte
	satPerByte := uint64(float64(feeBTCPerKb) * math.Pow10(int(8)))
	if satPerByte < minimumSatPerByte {
		satPerByte = minimumSatPerByte
	}
	if err != nil {
		satPerByte = minimumSatPerByte
	}
	return satPerByte
}

func (r *LWKRpcWallet) GetFee(txSize int64) (Satoshi, error) {
	ctx := context.Background()
	// assume largest witness
	fee := r.getFeePerKb(ctx) * uint64(txSize)
	return fee, nil
}

func (r *LWKRpcWallet) SetLabel(txID, address, label string) error {
	// TODO: call set label
	return nil
}
