package fsm

import (
	"github.com/sputn1ck/peerswap/lightning"
	"github.com/sputn1ck/peerswap/utils"
	"github.com/vulpemventures/go-elements/network"
)

type Messenger interface {
	SendMessage(peerId string, hexMsg string) error
}

type Policy interface {
	ShouldPayFee(feeAmount uint64, peerId, channelId string) bool
}
type LightningClient interface {
	DecodeInvoice(payreq string) (*lightning.Invoice, error)
	PayInvoice(payreq string) (preImage string, err error)
	CheckChannel(channelId string, amount uint64) (bool, error)
}

type TxWatcher interface {
	AddTx(swapId, txId, txHex string)
}

type Node interface {
	GetSwapScript(swap *Swap) ([]byte, error)
	GetBlockHeight() (uint64, error)
	GetAddress() (string, error)
	GetFee(txHex string) uint64
	GetAsset() []byte
	GetNetwork() *network.Network
	SendRawTx(txHex string) (string, error)
	CreatePreimageSpendingTransaction(params *utils.SpendingParams, preimage []byte) (string, error)
}
