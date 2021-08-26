package swap

import (
	"github.com/sputn1ck/glightning/glightning"
	"github.com/sputn1ck/peerswap/lightning"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/transaction"
)

type Messenger interface {
	SendMessage(peerId string, msg PeerMessage) error
	AddMessageHandler(func(peerId string, msgType string, payload string) error)
}

type PeerMessage interface {
	MessageType() MessageType
}

type Policy interface {
	ShouldPayFee(swapAmount, feeAmount uint64, peerId, channelId string) bool
	GetMakerFee(swapValue uint64, swapFee uint64) (uint64, error)
}
type LightningClient interface {
	DecodePayreq(payreq string) (*lightning.Invoice, error)
	PayInvoice(payreq string) (preImage string, err error)
	CheckChannel(channelId string, amount uint64) error
	GetPayreq(msatAmount uint64, preimage string, label string) (string, error)
	AddPaymentCallback(f func(*glightning.Payment))
	RebalancePayment(payreq string, channel string) (preimage string, err error)
}

type TxWatcher interface {
	AddCltvTx(swapId string, cltv int64)
	AddConfirmationsTx(swapId, txId string)
	AddTxConfirmedHandler(func(swapId string) error)
	AddCltvPassedHandler(func(swapId string) error)
}

type Blockchain interface {
	GetBlockHeight() (uint64, error)
	GetBlockHash(blockheight uint32) (string, error)
	GetFee(txHex string) uint64
	GetAsset() []byte
	GetNetwork() *network.Network
	SendRawTx(txHex string) (string, error)
	GetLocktime() uint64
	GetRawTxFromTxId(txId string, vout uint32) (string, error)
}

type Wallet interface {
	GetAddress() (string, error)
	FinalizeTransaction(rawTx string) (txId string, err error)
	CreateFundedTransaction(preparedTx *transaction.Transaction) (rawTx string, fee uint64, err error)
}

type Utility interface {
	CreateOpeningTransaction(redeemScript []byte, asset []byte, amount uint64) (*transaction.Transaction, error)
	VoutFromTxHex(txHex string, redeemScript []byte) (uint32, error)
	Blech32ToScript(blech32Addr string, network *network.Network) ([]byte, error)
	CreateSpendingTransaction(openingTxHex string, swapAmount, feeAmount, currentBlock uint64, asset, redeemScript, outputScript []byte) (tx *transaction.Transaction, sigHash [32]byte, err error)
	GetSwapScript(takerPubkeyHash, makerPubkeyHash, paymentHash string, cltv int64) ([]byte, error)
	GetPreimageWitness(signature, preimage, redeemScript []byte) [][]byte
	GetCltvWitness(signature, redeemScript []byte) [][]byte
	CheckTransactionValidity(openingTxHex string, swapAmount uint64, redeemScript []byte) error
}

type SwapServices struct {
	swapStore  Store
	blockchain Blockchain
	lightning  LightningClient
	messenger  Messenger
	policy     Policy
	txWatcher  TxWatcher
	wallet     Wallet
	utils      Utility
}

func NewSwapServices(swapStore Store, blockchain Blockchain, lightning LightningClient, messenger Messenger, policy Policy, txWatcher TxWatcher, wallet Wallet, utils Utility) *SwapServices {
	return &SwapServices{swapStore: swapStore, blockchain: blockchain, lightning: lightning, messenger: messenger, policy: policy, txWatcher: txWatcher, wallet: wallet, utils: utils}
}
