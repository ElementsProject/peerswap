package lwk

import (
	"context"
	"errors"

	"math"
	"strings"
	"time"

	"github.com/elementsproject/peerswap/electrum"
	"github.com/elementsproject/peerswap/log"
	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/wallet"
)

// Satoshi represents a Satoshi value.
type Satoshi = uint64

const (
	// 1 kb = 1000 bytes
	kb              = 1000
	btcToSatoshiExp = 8
	// TODO: Basically, the inherited ctx should be used
	// and there is no need to specify a timeout here.
	// Set up here because ctx is not inherited throughout the current codebase.
	defaultContextTimeout              = time.Second * 5
	minimumSatPerByte     SatPerKVByte = 0.1
)

// SatPerKVByte represents a fee rate in sat/kb.
type SatPerKVByte float64

func SatPerKVByteFromFeeBTCPerKb(feeBTCPerKb float64) SatPerKVByte {
	s := SatPerKVByte(feeBTCPerKb * math.Pow10(btcToSatoshiExp) / kb)
	if s < minimumSatPerByte {
		log.Infof("using minimum fee: %v.", minimumSatPerByte)
		return minimumSatPerByte
	}
	return s
}

func (s SatPerKVByte) GetSatPerKVByte() float64 {
	return float64(s)
}

func (s SatPerKVByte) GetFee(txSize int64) Satoshi {
	return Satoshi(s.GetSatPerKVByte() * float64(txSize))
}

// LWKRpcWallet uses the elementsd rpc wallet
type LWKRpcWallet struct {
	c              *Conf
	lwkClient      *lwkclient
	electrumClient electrum.RPC
}

func NewLWKRpcWallet(ctx context.Context, c *Conf) (*LWKRpcWallet, error) {
	if !c.Enabled() {
		return nil, errors.New("LWKRpcWallet is not enabled")
	}
	ec, err := electrum.NewElectrumClient(ctx, c.GetElectrumEndpoint(), c.IsElectrumWithTLS())
	if err != nil {
		return nil, err
	}
	rpcWallet := &LWKRpcWallet{
		lwkClient:      NewLwk(c.GetLWKEndpoint()),
		electrumClient: ec,
		c:              c,
	}
	err = rpcWallet.setupWallet(ctx) // Evaluate rpcWallet.setupWallet(ctx) before the return statement
	if err != nil {
		return nil, err
	}
	return rpcWallet, nil
}

// GetElectrumClient returns the electrum client.
func (c *LWKRpcWallet) GetElectrumClient() electrum.RPC {
	return c.electrumClient
}

// setupWallet checks if the swap wallet is already loaded in elementsd, if not it loads/creates it
func (r *LWKRpcWallet) setupWallet(ctx context.Context) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, defaultContextTimeout)
	defer cancel()
	res, err := r.lwkClient.walletDetails(timeoutCtx, &walletDetailsRequest{
		WalletName: r.c.GetWalletName(),
	})
	if err != nil {
		// 32008 is the error code for wallet not found of lwk
		if strings.HasPrefix(err.Error(), "-32008") {
			log.Infof("wallet not found, creating wallet with name %s", r.c.GetWalletName)
			return r.createWallet(timeoutCtx, r.c.GetWalletName(), r.c.GetSignerName())
		}
		return err
	}
	signers := res.Signers
	if len(signers) != 1 {
		return errors.New("invalid number of signers")
	}
	if signers[0].Name != r.c.GetSignerName() {
		return errors.New("signer name is not correct. expected: " + r.c.GetSignerName() + " got: " + signers[0].Name)
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
	asset []byte) (txid, rawTx string, fee Satoshi, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultContextTimeout)
	defer cancel()
	feerate := float64(r.getFeePerKb(ctx)) * kb
	fundedTx, err := r.lwkClient.send(ctx, &sendRequest{
		Addressees: []*unvalidatedAddressee{
			{
				Address: swapParams.OpeningAddress,
				Satoshi: swapParams.Amount,
			},
		},
		WalletName: r.c.GetWalletName(),
		FeeRate:    &feerate,
	})
	if err != nil {
		return "", "", 0, err
	}
	signed, err := r.lwkClient.sign(ctx, &signRequest{
		SignerName: r.c.GetSignerName(),
		Pset:       fundedTx.Pset,
	})
	if err != nil {
		return "", "", 0, err
	}
	broadcasted, err := r.lwkClient.broadcast(ctx, &broadcastRequest{
		WalletName: r.c.GetWalletName(),
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
	ctx, cancel := context.WithTimeout(context.Background(), defaultContextTimeout)
	defer cancel()
	balance, err := r.lwkClient.balance(ctx, &balanceRequest{
		WalletName: r.c.GetWalletName(),
	})
	if err != nil {
		return 0, err
	}
	return uint64(balance.Balance[r.c.GetAssetID()]), nil
}

// GetAddress returns a new blech32 address
func (r *LWKRpcWallet) GetAddress() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultContextTimeout)
	defer cancel()
	address, err := r.lwkClient.address(ctx, &addressRequest{
		WalletName: r.c.GetWalletName()})
	if err != nil {
		return "", err
	}
	return address.Address, nil
}

// SendToAddress sends an amount to an address
func (r *LWKRpcWallet) SendToAddress(address string, amount Satoshi) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultContextTimeout)
	defer cancel()
	sendres, err := r.lwkClient.send(ctx, &sendRequest{
		WalletName: r.c.GetWalletName(),
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
		SignerName: r.c.GetSignerName(),
		Pset:       sendres.Pset,
	})
	if err != nil {
		return "", err
	}
	broadcastres, err := r.lwkClient.broadcast(ctx, &broadcastRequest{
		WalletName: r.c.GetWalletName(),
		Pset:       signed.Pset,
	})
	if err != nil {
		return "", err
	}
	return broadcastres.Txid, nil
}

func (r *LWKRpcWallet) SendRawTx(txHex string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultContextTimeout)
	defer cancel()
	res, err := r.electrumClient.BroadcastTransaction(ctx, txHex)
	if err != nil {
		return "", err
	}
	return res, nil
}

func (r *LWKRpcWallet) getFeePerKb(ctx context.Context) SatPerKVByte {
	feeBTCPerKb, err := r.electrumClient.GetFee(ctx, wallet.LiquidTargetBlocks)
	if err != nil {
		log.Infof("error getting fee: %v.", err)
	}
	return SatPerKVByteFromFeeBTCPerKb(float64(feeBTCPerKb))
}

func (r *LWKRpcWallet) GetFee(txSize int64) (Satoshi, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultContextTimeout)
	defer cancel()
	return r.getFeePerKb(ctx).GetFee(txSize), nil
}

func (r *LWKRpcWallet) SetLabel(txID, address, label string) error {
	// TODO: call set label
	return nil
}
