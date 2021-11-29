package swap

import (
	"fmt"
	"github.com/sputn1ck/peerswap/messages"

	"github.com/btcsuite/btcd/btcec"
)

type Messenger interface {
	SendMessage(peerId string, message []byte, messageType int) error
	AddMessageHandler(func(peerId string, msgType string, payload string) error)
}

type PeerMessage interface {
	MessageType() messages.MessageType
}

type Policy interface {
	IsPeerAllowed(peer string) bool
	GetReserveOnchainMsat() uint64
}
type LightningClient interface {
	DecodePayreq(payreq string) (paymentHash string, amountMsat uint64, err error)
	PayInvoice(payreq string) (preImage string, err error)
	CheckChannel(channelId string, amount uint64) error
	GetPayreq(msatAmount uint64, preimage string, label string) (string, error)
	AddPaymentCallback(f func(paymentLabel string))
	RebalancePayment(payreq string, channel string) (preimage string, err error)
}

type Onchain interface {
	AddWaitForConfirmationTx(swapId, txId string) (err error)
	AddWaitForCsvTx(swapId, txId string, vout uint32) (err error)
	AddConfirmationCallback(func(swapId string) error)
	AddCsvCallback(func(swapId string) error)
	ValidateTx(swapParams *OpeningParams, openingTxId string) (bool, error)
}

type Wallet interface {
	CreateOpeningTransaction(swapParams *OpeningParams) (unpreparedTxHex string, fee uint64, vout uint32, err error)
	BroadcastOpeningTx(unpreparedTxHex string) (txId, txHex string, error error)
	CreatePreimageSpendingTransaction(swapParams *OpeningParams, claimParams *ClaimParams, openingTxId string) (txId, txHex string, error error)
	CreateCsvSpendingTransaction(swapParams *OpeningParams, claimParams *ClaimParams, openingTxHex string, vout uint32) (txId, txHex string, error error)
	TakerCreateCoopSigHash(swapParams *OpeningParams, claimParams *ClaimParams, openingTxId, refundAddress string, refundFee uint64) (sigHash string, error error)
	CreateCooperativeSpendingTransaction(swapParams *OpeningParams, claimParams *ClaimParams, refundAddress, openingTxHex string, vout uint32, takerSignatureHex string, refundFee uint64) (txId, txHex string, error error)
	NewAddress() (string, error)
	GetRefundFee() (uint64, error)
}

type OpeningParams struct {
	TakerPubkeyHash  string
	MakerPubkeyHash  string
	ClaimPaymentHash string
	Amount           uint64
}

type ClaimParams struct {
	Preimage string
	Signer   Signer
}

type Signer interface {
	Sign(hash []byte) (*btcec.Signature, error)
}

type SwapServices struct {
	swapStore           Store
	requestedSwapsStore RequestedSwapsStore
	lightning           LightningClient
	messenger           Messenger
	policy              Policy
	bitcoinOnchain      Onchain
	bitcoinWallet       Wallet
	bitcoinEnabled      bool
	liquidOnchain       Onchain
	liquidWallet        Wallet
	liquidEnabled       bool
}

func NewSwapServices(
	swapStore Store,
	requestedSwapsStore RequestedSwapsStore,
	lightning LightningClient,
	messenger Messenger,
	policy Policy,
	bitcoinEnabled bool,
	bitcoinWallet Wallet,
	bitcoinOnchain Onchain,
	liquidEnabled bool,
	liquidWallet Wallet,
	liquidOnchain Onchain) *SwapServices {
	return &SwapServices{
		swapStore:           swapStore,
		requestedSwapsStore: requestedSwapsStore,
		lightning:           lightning,
		messenger:           messenger,
		policy:              policy,
		bitcoinOnchain:      bitcoinOnchain,
		bitcoinWallet:       bitcoinWallet,
		bitcoinEnabled:      bitcoinEnabled,
		liquidEnabled:       liquidEnabled,
		liquidWallet:        liquidWallet,
		liquidOnchain:       liquidOnchain,
	}
}

func (s *SwapServices) getOnchainAsset(asset string) (Onchain, Wallet, error) {
	if asset == "" {
		return nil, nil, fmt.Errorf("missing asset")
	}
	if asset == "btc" {
		return s.bitcoinOnchain, s.bitcoinWallet, nil
	}
	if asset == "l-btc" {
		return s.liquidOnchain, s.liquidWallet, nil
	}
	return nil, nil, WrongAssetError(asset)
}
