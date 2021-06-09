package lightning

import (
	"github.com/sputn1ck/liquid-loop/wallet"
)

type WalletService interface {
	ListAddresses() ([]string, error)
	GetBalance() (uint64, error)
	ListUtxos() ([]*wallet.Utxo, error)
}

type PeerCommunicator interface {
	SendMessage(peerId string, message PeerMessage) error
	AddMessageHandler(func(peerId string, messageType string, payload string) error) error
}

type PeerMessage interface {
	MessageType() string
}

type Invoice struct {
	PHash       []byte
	Amount      uint64
	Description string
}

type Swapper interface {
	StartSwapIn(peerNodeId string, channelId string, amount uint64) error
}