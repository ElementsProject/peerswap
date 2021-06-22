package messaging

import (
	"encoding/hex"
	"encoding/json"
	"log"
)

type PeerCommunicator interface {
	SendMessage(peerId string, message PeerMessage) error
	AddMessageHandler(func(peerId string, messageType string, payload string) error) error
}

type PeerMessage interface {
	MessageType() string
}

type MessageSubscriber interface {
	OnSwapInRequest(peerId string, req SwapInRequest) error
	OnSwapOutRequest(peerId string, req SwapOutRequest) error
	OnSwapInAgreementResponse(peerId string, req SwapInAgreementResponse) error
	OnFeeResponse(peerId string, req FeeResponse) error
	OnTxOpenedResponse(peerId string, req TxOpenedResponse) error
	OnClaimedMessage(peerId string, req ClaimedMessage) error
	OnCancelResponse(peerId string, req CancelResponse) error

}
type MessageHandler struct {
	pc   PeerCommunicator
	subscriber MessageSubscriber
}

func NewMessageHandler(pc PeerCommunicator, subscriber MessageSubscriber) *MessageHandler {
	return &MessageHandler{pc: pc, subscriber: subscriber}
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
		err = sh.subscriber.OnFeeResponse(peerId, req)
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
		err = sh.subscriber.OnSwapInAgreementResponse(peerId, req)
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
		err = sh.subscriber.OnTxOpenedResponse(peerId, req)
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
		err = sh.subscriber.OnClaimedMessage(peerId, req)
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
		err = sh.subscriber.OnCancelResponse(peerId, req)
		if err != nil {
			return err
		}
	}

	return nil
}
