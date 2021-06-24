package swap

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
)

type PeerCommunicator interface {
	SendMessage(peerId string, message PeerMessage) error
	AddMessageHandler(func(peerId string, messageType string, payload string) error) error
}

type PeerMessage interface {
	MessageType() MessageType
}

type MessageSubscriber interface {
	OnSwapInRequest(peerId string, req SwapInRequest) error
	OnSwapOutRequest(peerId string, req SwapOutRequest) error
	OnSwapInAgreementResponse(swap *Swap, req SwapInAgreementResponse) error
	OnFeeResponse(swap *Swap, req FeeResponse) error
	OnTxOpenedResponse(swap *Swap, req TxOpenedResponse) error
	OnClaimedMessage(swap *Swap, req ClaimedMessage) error
	OnCancelResponse(swap *Swap, req CancelResponse) error
}

type SwapGetter interface {
	GetById(s string) (*Swap, error)
}
type MessageHandler struct {
	pc   PeerCommunicator
	subscriber MessageSubscriber
	store SwapGetter
}

func NewMessageHandler(pc PeerCommunicator, subscriber MessageSubscriber, store SwapGetter) *MessageHandler {
	return &MessageHandler{pc: pc, subscriber: subscriber, store: store}
}

func (sh *MessageHandler) Start() error {
	err := sh.pc.AddMessageHandler(sh.OnMessageReceived)
	if err != nil {
		return err
	}
	return nil
}

func (sh *MessageHandler) OnMessageReceived(peerId string, messageTypeString string, message string) error {
	inRange, err := InRange(messageTypeString)
	if err != nil {
		return err
	}
	if !inRange {
		return nil
	}

	messageBytes, err := hex.DecodeString(message)
	if err != nil {
		return err
	}
	var baseMsg MessageBase
	err = json.Unmarshal(messageBytes, &baseMsg)
	if err != nil {
		return err
	}
	swap, err := sh.store.GetById(baseMsg.SwapId)
	if err != nil && err != ErrDoesNotExist{
		return err
	}
	if swap != nil && err != ErrDoesNotExist {
		if swap.PeerNodeId != peerId {
			return errors.New("saved peerId does not match request")
		}
	}
	messageType,err := HexStrToMsgType(messageTypeString)
	if err != nil {
		return err
	}
	switch messageType {
	case MESSAGETYPE_SWAPINREQUEST:
		log.Printf("incoming swapin request %s", string(messageBytes))
		var req SwapInRequest
		err = json.Unmarshal(messageBytes, &req)
		if err != nil {
			return err
		}
		err = sh.subscriber.OnSwapInRequest(peerId, req)
		if err != nil {
			return err
		}
	case MESSAGETYPE_SWAPOUTREQUEST:
		log.Printf("incoming swapout request %s", string(messageBytes))
		var req SwapOutRequest
		err = json.Unmarshal(messageBytes, &req)
		if err != nil {
			return err
		}
		err = sh.subscriber.OnSwapOutRequest(peerId, req)
		if err != nil {
			return err
		}
	case MESSAGETYPE_FEERESPONSE:
		log.Printf("incoming feeresponse %s", string(messageBytes))
		var req FeeResponse
		err = json.Unmarshal(messageBytes, &req)
		if err != nil {
			return err
		}
		err = sh.subscriber.OnFeeResponse(swap, req)
		if err != nil {
			return err
		}
	case MESSAGETYPE_SWAPINAGREEMENT:
		log.Printf("incoming swapinagreement %s", string(messageBytes))
		var req SwapInAgreementResponse
		err = json.Unmarshal(messageBytes, &req)
		if err != nil {
			return err
		}
		err = sh.subscriber.OnSwapInAgreementResponse(swap, req)
		if err != nil {
			return err
		}
	case MESSAGETYPE_TXOPENEDRESPONSE:
		log.Printf("incoming txopenedresponse %s", string(messageBytes))
		var req TxOpenedResponse
		err = json.Unmarshal(messageBytes, &req)
		if err != nil {
			return err
		}
		err = sh.subscriber.OnTxOpenedResponse(swap, req)
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
		err = sh.subscriber.OnClaimedMessage(swap, req)
		if err != nil {
			return err
		}
	case MESSAGETYPE_CANCELED:
		log.Printf("incoming canceledmessage %s", string(messageBytes))
		var req CancelResponse
		err = json.Unmarshal(messageBytes, &req)
		if err != nil {
			return err
		}
		err = sh.subscriber.OnCancelResponse(swap, req)
		if err != nil {
			return err
		}
	}

	return nil
}
