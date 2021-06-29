package fsm

import (
	"encoding/json"
	"errors"
	"sync"
)

var (
	ErrSwapDoesNotExist = errors.New("swap does not exist")
)

type SwapServices struct {
	swapStore Store
	node      Node
	lightning LightningClient
	messenger Messenger
	policy    Policy
	txWatcher TxWatcher
}
type SwapService struct {
	swapServices *SwapServices

	activeSwaps map[string]*StateMachine

	sync.Mutex
}

func NewSwapService(swapStore Store, node Node, lightning LightningClient, messenger Messenger, policy Policy, txWatcher TxWatcher) *SwapService {
	services := &SwapServices{
		messenger: messenger,
		swapStore: swapStore,
		node:      node,
		lightning: lightning,
		policy:    policy,
		txWatcher: txWatcher,
	}
	return &SwapService{swapServices: services, activeSwaps: map[string]*StateMachine{}}
}

func (s *SwapService) Start() error {
	s.swapServices.messenger.AddMessageHandler(s.OnMessageReceived)

	s.swapServices.txWatcher.AddCltvPassedHandler(s.OnCltvPassed)
	s.swapServices.txWatcher.AddTxConfirmedHandler(s.OnTxConfirmed)

	return nil
}

func (s *SwapService) OnMessageReceived(peerId string, msgType MessageType, msgBytes []byte) error {
	switch msgType {
	case MESSAGETYPE_SWAPOUTREQUEST:
		var msg SwapOutRequest
		err := json.Unmarshal(msgBytes, &msg)
		if err != nil {
			return err
		}
	case MESSAGETYPE_FEERESPONSE:
		var msg FeeResponse
		err := json.Unmarshal(msgBytes, &msg)
		if err != nil {
			return err
		}
		err = s.OnFeeInvoiceReceived(msg.SwapId, msg.Invoice)
		if err != nil {
			return err
		}
	case MESSAGETYPE_TXOPENEDRESPONSE:
		var msg TxOpenedResponse
		err := json.Unmarshal(msgBytes, &msg)
		if err != nil {
			return err
		}
		err = s.OnTxOpenedMessage(msg.SwapId, msg.MakerPubkeyHash, msg.Invoice, msg.TxId, msg.TxHex, msg.Cltv)
		if err != nil {
			return err
		}
	case MESSAGETYPE_CANCELED:
		var msg CancelResponse
		err := json.Unmarshal(msgBytes, &msg)
		if err != nil {
			return err
		}
		err = s.OnCancelReceived(msg.SwapId)
		if err != nil {
			return err
		}
	case MESSAGETYPE_CLAIMED:
		var msg ClaimedMessage
		err := json.Unmarshal(msgBytes, &msg)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SwapService) OnTxConfirmed(swapId string) error {
	_, err := s.GetSwap(swapId)
	if err != nil {
		return err
	}
	return nil
}

func (s *SwapService) OnCltvPassed(swapId string) error {
	_, err := s.GetSwap(swapId)
	if err != nil {
		return err
	}
	return nil
}

// todo check prerequisites
func (s *SwapService) SwapOut(channelId string, amount uint64, peer string, initiator string) error {
	swap := newSwapOutSenderFSM("", s.swapServices.swapStore, s.swapServices)
	err := swap.SendEvent(Event_SwapOutSender_OnSwapOutCreated, &SwapCreationContext{
		amount:      amount,
		initiatorId: initiator,
		peer:        peer,
		channelId:   channelId,
	})
	if err != nil {
		return err
	}
	s.AddSwap(swap.Id, swap)
	return nil
}

func (s *SwapService) OnFeeInvoiceReceived(swapId, feeInvoice string) error {
	swap, err := s.GetSwap(swapId)
	if err != nil {
		return err
	}
	err = swap.SendEvent(Event_SwapOutSender_OnFeeInvReceived, &FeeRequestContext{FeeInvoice: feeInvoice})
	if err != nil {
		return err
	}
	return nil
}

func (s *SwapService) OnTxOpenedMessage(swapId, makerPubkeyHash, claimInvoice, txId, txHex string, cltv int64) error {
	swap, err := s.GetSwap(swapId)
	if err != nil {
		return err
	}
	err = swap.SendEvent(Event_SwapOutSender_OnTxOpenedMessage, &TxBroadcastedContext{
		MakerPubkeyHash: makerPubkeyHash,
		ClaimInvoice:    claimInvoice,
		TxId:            txId,
		TxHex:           txHex,
		Cltv:            cltv,
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *SwapService) SenderOnTxConfirmed(swapId string) error {
	swap, err := s.GetSwap(swapId)
	if err != nil {
		return err
	}
	if swap.Role != SWAPROLE_SENDER {
		return nil
	}
	err = swap.SendEvent(Event_SwapOutSender_OnTxConfirmations, nil)
	if err != nil {
		return err
	}
	return nil
}

func (s *SwapService) OnCancelReceived(swapId string) error {
	swap, err := s.GetSwap(swapId)
	if err != nil {
		return err
	}
	err = swap.SendEvent(Event_OnCancelReceived, nil)
	if err != nil {
		return err
	}
	return nil
}

func (s *SwapService) AddSwap(swapId string, swap *StateMachine) {
	s.Lock()
	defer s.Unlock()
	s.activeSwaps[swapId] = swap
}

func (s *SwapService) GetSwap(swapId string) (*StateMachine, error) {
	if swap, ok := s.activeSwaps[swapId]; ok {
		return swap, nil
	}
	return nil, ErrSwapDoesNotExist
}
