package lwk

import (
	"context"
	"errors"
	"fmt"

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

// SatPerVByte represents a fee rate in sat/vb.
type SatPerVByte float64

const (
	// 1 kb = 1000 bytes
	kb              = 1000
	btcToSatoshiExp = 8
	// TODO: Basically, the inherited ctx should be used
	// and there is no need to specify a timeout here.
	// Set up here because ctx is not inherited throughout the current codebase.
	defaultContextTimeout             = time.Second * 20
	minimumFee            SatPerVByte = 0.1
	supportedCLIVersion               = "0.8.0"
)

func SatPerVByteFromFeeBTCPerKb(feeBTCPerKb float64) SatPerVByte {
	s := SatPerVByte(feeBTCPerKb * math.Pow10(btcToSatoshiExp) / kb)
	if s < minimumFee {
		log.Debugf("using minimum fee rate of %v sat/vbyte",
			minimumFee)
		return minimumFee
	}
	return s
}

func (s SatPerVByte) getValue() float64 {
	return float64(s)
}

func (s SatPerVByte) GetFee(txSizeBytes int64) Satoshi {
	return Satoshi(s.getValue() * float64(txSizeBytes))
}

// LWKRpcWallet uses the elementsd rpc wallet
type LWKRpcWallet struct {
	c              *Conf
	lwkClient      *lwkclient
	electrumClient electrum.RPC
	lwkVersion     string
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

func (r *LWKRpcWallet) IsSupportedVersion() bool {
	return r.lwkVersion == supportedCLIVersion
}

// setupWallet checks if the swap wallet is already loaded in elementsd, if not it loads/creates it
func (r *LWKRpcWallet) setupWallet(ctx context.Context) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, defaultContextTimeout)
	defer cancel()
	vres, err := r.lwkClient.version(timeoutCtx)
	if err != nil {
		return err
	}
	r.lwkVersion = vres.Version
	if !r.IsSupportedVersion() {
		return errors.New("unsupported lwk version. expected: " + supportedCLIVersion + " got: " + r.lwkVersion)
	}

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
		Persist:    true,
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

// CreateAndBroadcastTransaction takes a tx with outputs and adds inputs in order to spend the tx
func (r *LWKRpcWallet) CreateAndBroadcastTransaction(swapParams *swap.OpeningParams,
	outputs []wallet.TxOutput) (txid, rawTx string, fee Satoshi, err error) {
	if len(outputs) == 0 {
		return "", "", 0, errors.New("missing outputs")
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultContextTimeout)
	defer cancel()
	feerate := r.getFeeSatPerVByte(ctx).getValue() * kb
	addressees := make([]*unvalidatedAddressee, 0, len(outputs))
	for _, out := range outputs {
		if out.AssetID == "" {
			return "", "", 0, errors.New("missing asset id")
		}
		addressees = append(addressees, &unvalidatedAddressee{
			Address: swapParams.OpeningAddress,
			Asset:   out.AssetID,
			Satoshi: out.Amount,
		})
	}
	// todo: There will be an option in the tx builder to enable the discount.
	fundedTx, err := r.lwkClient.send(ctx, &sendRequest{
		Addressees:      addressees,
		WalletName:       r.c.GetWalletName(),
		FeeRate:          &feerate,
		EnableCtDiscount: true,
	})
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to fund transaction: %w", err)
	}
	signed, err := r.lwkClient.sign(ctx, &signRequest{
		SignerName: r.c.GetSignerName(),
		Pset:       fundedTx.Pset,
	})
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to sign transaction: %w", err)
	}
	broadcasted, err := r.lwkClient.broadcast(ctx, &broadcastRequest{
		WalletName: r.c.GetWalletName(),
		Pset:       signed.Pset,
	})
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to broadcast transaction: %w", err)
	}
	hex, err := r.electrumClient.GetRawTransaction(ctx, broadcasted.Txid)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to get raw transaction: %w", err)
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
		EnableCtDiscount: true,
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
		rpcErr, pErr := parseRPCError(err)
		if pErr != nil {
			return "", fmt.Errorf("error parsing rpc error: %v", pErr)
		}
		if rpcErr.Code == -26 {
			return "", wallet.MinRelayFeeNotMetError
		}
		return "", err
	}
	return res, nil
}

func (r *LWKRpcWallet) getFeeSatPerVByte(ctx context.Context) SatPerVByte {
	feeBTCPerKb, err := r.electrumClient.GetFee(ctx, wallet.LiquidTargetBlocks)
	if err != nil {
		log.Infof("error getting fee: %v.", err)
	}
	return SatPerVByteFromFeeBTCPerKb(float64(feeBTCPerKb))
}

func (r *LWKRpcWallet) GetFee(txSizeBytes int64) (Satoshi, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultContextTimeout)
	defer cancel()
	return r.getFeeSatPerVByte(ctx).GetFee(txSizeBytes), nil
}

func (r *LWKRpcWallet) SetLabel(txID, address, label string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultContextTimeout)
	defer cancel()
	return r.lwkClient.walletSetTxMemo(ctx, &WalletSetTxMemoRequest{
		WalletName: r.c.GetWalletName(),
		Txid:       txID,
		Memo:       label,
	})
}

func (r *LWKRpcWallet) Ping() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultContextTimeout)
	defer cancel()
	_, err := r.lwkClient.version(ctx)
	if err != nil {
		return false, errors.New("lwk connection failed: " + err.Error())
	}
	err = r.electrumClient.Ping(ctx)
	return err == nil, err
}
