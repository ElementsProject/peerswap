package lightning

import "github.com/sputn1ck/liquid-loop/wallet"

const (
	customMsgType = "A455"
)

type WalletService interface {
	ListAddresses() ([]string, error)
	GetBalance() (uint64, error)
	ListUtxos() ([]*wallet.Utxo, error)
}

type PeerCommunicator interface {
	SendMessage(peerId string, message []byte) error
	AddMessageHandler(func(peerId string, message string) error) error
}

type Invoice struct {
	PHash       string
	Amount      uint64
	Description string
}
