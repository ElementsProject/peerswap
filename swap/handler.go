package swap

import (
	"encoding/hex"
	"encoding/json"
	"github.com/sputn1ck/sugarmama/lightning"
	"github.com/vulpemventures/go-elements/transaction"
	"log"
)

type TxWatcher interface {
	GetCommitmentTx(txId string) (*transaction.Transaction, error)
}

type TxClaimer interface {
	ClaimCommitmentTx() (string, error)
}

type TxCreator interface {
	CreateCommitmentTx(takerPubkeyHash, makerPubkeyHash, pHash string, amount uint64) (txId string, fee int64, err error)
}

type LightningClient interface {
	GetPayreq(amount uint64, preImage string, label string) (string, error)
	DecodePayreq(payreq string) (*lightning.Invoice, error)
	PayInvoice(payreq string) (preimage string, err error)
}
type MessageHandler struct {
	pc   lightning.PeerCommunicator
	swap *Service
}

func NewMessageHandler(pc lightning.PeerCommunicator, swap *Service) *MessageHandler {
	return &MessageHandler{pc: pc, swap: swap}
}

func (sh *MessageHandler) Start() error {
	err := sh.pc.AddMessageHandler(sh.OnMessageReceived)
	if err != nil {
		return err
	}
	return nil
}

func (sh *MessageHandler) OnMessageReceived(peerId string, messageType string, message string) error {
	messageBytes, err := hex.DecodeString(message)
	if err != nil {
		return err
	}
	switch messageType {
	case MESSAGETYPE_SWAPREQUEST:
		var req SwapRequest
		err = json.Unmarshal(messageBytes, &req)
		if err != nil {
			return err
		}
		err = sh.swap.OnSwapRequest(peerId, req)
		if err != nil {
			return err
		}
	case MESSAGETYPE_MAKERRESPONSE:
		log.Println("incoming makerresponse")
		var req MakerResponse
		err = json.Unmarshal(messageBytes, &req)
		if err != nil {
			return err
		}
		err = sh.swap.OnMakerResponse(peerId, req)
		if err != nil {
			return err
		}
	}
	return nil
}
