package swap

import (
	"encoding/hex"
	"encoding/json"
	"github.com/sputn1ck/peerswap/lightning"
	"log"
)

type LightningClient interface {
	GetPayreq(amount uint64, preImage string, label string) (string, error)
	DecodePayreq(payreq string) (*lightning.Invoice, error)
	PayInvoice(payreq string) (preimage string, err error)
	GetPreimage() (lightning.Preimage, error)
	GetNodeId() string
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

		log.Printf("incoming swaprequest %s", string(messageBytes))
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
		log.Printf("incoming makerresponse %s", string(messageBytes))
		var req MakerResponse
		err = json.Unmarshal(messageBytes, &req)
		if err != nil {
			return err
		}
		err = sh.swap.OnMakerResponse(peerId, req)
		if err != nil {
			return err
		}
	case MESSAGETYPE_TAKERRESPONSE:
		log.Printf("incoming takerresponse %s", string(messageBytes))
		var req TakerResponse
		err = json.Unmarshal(messageBytes, &req)
		if err != nil {
			return err
		}
		err = sh.swap.OnTakerResponse(peerId, req)
		if err != nil {
			return err
		}
	case MESSAGETYPE_CLAIMED:
		log.Printf("incoming claimedResponse %s", string(messageBytes))
		var req ClaimedMessage
		err = json.Unmarshal(messageBytes, &req)
		if err != nil {
			return err
		}
		err = sh.swap.OnClaimedResponse(peerId, req)
		if err != nil {
			return err
		}
	case MESSAGETYPE_ERRORRESPONSE:
		log.Printf("incoming erroreRespons %s", string(messageBytes))
		var req ErrorResponse
		err = json.Unmarshal(messageBytes, &req)
		if err != nil {
			return err
		}
		err = sh.swap.OnErrorMessage(peerId, req)
		if err != nil {
			return err
		}
	}

	return nil
}
