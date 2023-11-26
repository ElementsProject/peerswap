package swap

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/elementsproject/peerswap/messages"

	"github.com/btcsuite/btcd/btcec/v2"
	btecdsa "github.com/btcsuite/btcd/btcec/v2/ecdsa"
)

const (
	btc_chain   = "btc"
	l_btc_chain = "lbtc"
)

type Messenger interface {
	SendMessage(peerId string, message []byte, messageType int) error
	AddMessageHandler(func(peerId string, msgType string, payload []byte) error)
}

type MessengerManager interface {
	AddSender(id string, messenger messages.StoppableMessenger) error
	RemoveSender(id string)
}
type PeerMessage interface {
	MessageType() messages.MessageType
}

type Policy interface {
	IsPeerAllowed(peer string) bool
	IsPeerSuspicious(peer string) bool
	AddToSuspiciousPeerList(pubkey string) error
	GetReserveOnchainMsat() uint64
	GetMinSwapAmountMsat() uint64
	NewSwapsAllowed() bool
}

type LightningClient interface {
	DecodePayreq(payreq string) (paymentHash string, amountMsat uint64, expiry int64, err error)
	PayInvoice(payreq string) (preImage string, err error)
	GetPayreq(msatAmount uint64, preimage string, swapId string, memo string, invoiceType InvoiceType, expirySeconds, expiryCltv uint64) (string, error)
	PayInvoiceViaChannel(payreq string, channel string) (preimage string, err error)
	AddPaymentCallback(f func(swapId string, invoiceType InvoiceType))
	AddPaymentNotifier(swapId string, payreq string, invoiceType InvoiceType)
	RebalancePayment(payreq string, channel string) (preimage string, err error)
	CanSpend(amountMsat uint64) error
	Implementation() string
	SpendableMsat(scid string) (uint64, error)
	ReceivableMsat(scid string) (uint64, error)
	ProbePayment(scid string, amountMsat uint64) (bool, string, error)
}

type TxWatcher interface {
	AddWaitForConfirmationTx(swapId, txId string, vout, startingHeight uint32, scriptpubkey []byte)
	AddWaitForCsvTx(swapId, txId string, vout uint32, startingHeight uint32, scriptpubkey []byte)
	AddConfirmationCallback(func(swapId string, txHex string, err error) error)
	AddCsvCallback(func(swapId string) error)
	GetBlockHeight() (uint32, error)
}

type Validator interface {
	TxIdFromHex(txHex string) (string, error)
	ValidateTx(swapParams *OpeningParams, txHex string) (bool, error)
	GetCSVHeight() uint32
}

type Wallet interface {
	CreateOpeningTransaction(swapParams *OpeningParams) (unpreparedTxHex string, fee uint64, vout uint32, err error)
	BroadcastOpeningTx(unpreparedTxHex string) (txId, txHex string, error error)
	CreatePreimageSpendingTransaction(swapParams *OpeningParams, claimParams *ClaimParams) (string, string, error)
	CreateCsvSpendingTransaction(swapParams *OpeningParams, claimParams *ClaimParams) (txId, txHex string, error error)
	CreateCoopSpendingTransaction(swapParams *OpeningParams, claimParams *ClaimParams, takerSigner Signer) (txId, txHex string, error error)
	GetOutputScript(params *OpeningParams) ([]byte, error)
	NewAddress() (string, error)
	GetRefundFee() (uint64, error)
	GetFlatSwapOutFee() (uint64, error)
	GetAsset() string
	GetNetwork() string
	GetOnchainBalance() (uint64, error)
}

type OpeningParams struct {
	TakerPubkey      string
	MakerPubkey      string
	ClaimPaymentHash string
	Amount           uint64
	BlindingKey      *btcec.PrivateKey
	OpeningAddress   string
}

func (o *OpeningParams) String() string {
	var bk string
	if o.BlindingKey != nil {
		bk = string(o.BlindingKey.Serialize())
	}
	return fmt.Sprintf("takerpkh: %s, makerpkh: %s, claimPhash: %s amount: %v, blindingKey: %s", o.TakerPubkey, o.MakerPubkey, o.ClaimPaymentHash, o.Amount, bk)
}

type ClaimParams struct {
	Preimage     string
	Signer       Signer
	OpeningTxHex string

	// blinded tx stuff
	BlindingSeed              []byte
	OutputAssetBlindingFactor []byte
	EphemeralKey              *btcec.PrivateKey
}

func (o *ClaimParams) String() string {
	return fmt.Sprintf("preimage %s, openingtxHex %s", hex.EncodeToString([]byte(o.Preimage)), o.OpeningTxHex)
}

type Signer interface {
	Sign(hash []byte) (*btecdsa.Signature, error)
}

type TimeOutService interface {
	addNewTimeOut(ctx context.Context, d time.Duration, id string)
}

type SwapServices struct {
	swapStore           Store
	requestedSwapsStore RequestedSwapsStore
	lightning           LightningClient
	messenger           Messenger
	messengerManager    MessengerManager
	policy              Policy
	bitcoinTxWatcher    TxWatcher
	bitcoinValidator    Validator
	bitcoinWallet       Wallet
	bitcoinEnabled      bool
	liquidTxWatcher     TxWatcher
	liquidValidator     Validator
	liquidWallet        Wallet
	liquidEnabled       bool
	toService           TimeOutService
}

func NewSwapServices(
	swapStore Store,
	requestedSwapsStore RequestedSwapsStore,
	lightning LightningClient,
	messenger Messenger,
	messengerManager MessengerManager,
	policy Policy,
	bitcoinEnabled bool,
	bitcoinWallet Wallet,
	bitcoinValidator Validator,
	bitcoinTxWatcher TxWatcher,
	liquidEnabled bool,
	liquidWallet Wallet,
	liquidValidator Validator,
	liquidTxWatcher TxWatcher) *SwapServices {
	return &SwapServices{
		swapStore:           swapStore,
		requestedSwapsStore: requestedSwapsStore,
		lightning:           lightning,
		messenger:           messenger,
		messengerManager:    messengerManager,
		policy:              policy,
		bitcoinTxWatcher:    bitcoinTxWatcher,
		bitcoinWallet:       bitcoinWallet,
		bitcoinValidator:    bitcoinValidator,
		bitcoinEnabled:      bitcoinEnabled,
		liquidEnabled:       liquidEnabled,
		liquidWallet:        liquidWallet,
		liquidValidator:     liquidValidator,
		liquidTxWatcher:     liquidTxWatcher,
	}
}

func (s *SwapServices) getOnChainServices(asset string) (TxWatcher, Wallet, Validator, error) {
	if asset == "" {
		return nil, nil, nil, fmt.Errorf("missing asset")
	}
	if asset == btc_chain {
		return s.bitcoinTxWatcher, s.bitcoinWallet, s.bitcoinValidator, nil
	}
	if asset == l_btc_chain {
		return s.liquidTxWatcher, s.liquidWallet, s.liquidValidator, nil
	}
	return nil, nil, nil, WrongAssetError(asset)
}
