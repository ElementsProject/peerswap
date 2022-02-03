package swap

import (
	"fmt"

	"github.com/sputn1ck/peerswap/messages"

	"github.com/btcsuite/btcd/btcec"
)

const (
	btc_asset   = "btc"
	l_btc_asset = "l-btc"
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
	GetReserveOnchainMsat() uint64
}

// todo add check if invoice paid dinges
type LightningClient interface {
	DecodePayreq(payreq string) (paymentHash string, amountMsat uint64, err error)
	PayInvoice(payreq string) (preImage string, err error)
	GetPayreq(msatAmount uint64, preimage string, label string, expirySeconds uint64) (string, error)
	AddPaymentCallback(f func(paymentLabel string))
	RebalancePayment(payreq string, channel string) (preimage string, err error)
}

type TxWatcher interface {
	AddWaitForConfirmationTx(swapId, txId string, vout, startingHeight uint32, scriptpubkey []byte)
	AddWaitForCsvTx(swapId, txId string, vout uint32, startingHeight uint32, scriptpubkey []byte)
	AddConfirmationCallback(func(swapId string, txHex string) error)
	AddCsvCallback(func(swapId string) error)
	GetBlockHeight() (uint32, error)
}

type Validator interface {
	TxIdFromHex(txHex string) (string, error)
	ValidateTx(swapParams *OpeningParams, txHex string) (bool, error)
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
	GetAsset() string
	GetNetwork() string
}

type OpeningParams struct {
	TakerPubkeyHash  string
	MakerPubkeyHash  string
	ClaimPaymentHash string
	Amount           uint64
	BlindingKey      *btcec.PrivateKey
	OpeningAddress   string
}

func (o *OpeningParams) String() string {
	return fmt.Sprintf("takerpkh: %s, makerpkh: %s, claimPhash: %s amount: %v", o.TakerPubkeyHash, o.MakerPubkeyHash, o.ClaimPaymentHash, o.Amount)
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

type Signer interface {
	Sign(hash []byte) (*btcec.Signature, error)
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
	if asset == btc_asset {
		return s.bitcoinTxWatcher, s.bitcoinWallet, s.bitcoinValidator, nil
	}
	if asset == l_btc_asset {
		return s.liquidTxWatcher, s.liquidWallet, s.liquidValidator, nil
	}
	return nil, nil, nil, WrongAssetError(asset)
}
