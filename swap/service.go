package swap

import (
	"encoding/json"
	"errors"
	"github.com/sputn1ck/glightning/glightning"
	"log"
	"strings"
	"sync"
)

var (
	ErrSwapDoesNotExist = errors.New("swap does not exist")
)

type SwapService struct {
	swapServices *SwapServices

	activeSwaps map[string]*StateMachine

	sync.Mutex
}

func NewSwapService(swapStore Store, blockchain Blockchain, lightning LightningClient, messenger Messenger, policy Policy, txWatcher TxWatcher, wallet Wallet, utils Utility) *SwapService {
	services := NewSwapServices(
		swapStore,
		blockchain,
		lightning,
		messenger,
		policy,
		txWatcher,
		wallet,
		utils,
	)
	return &SwapService{swapServices: services, activeSwaps: map[string]*StateMachine{}}
}

func (s *SwapService) Start() error {
	s.swapServices.messenger.AddMessageHandler(s.OnMessageReceived)

	s.swapServices.txWatcher.AddCltvPassedHandler(s.OnCltvPassed)
	s.swapServices.txWatcher.AddTxConfirmedHandler(s.OnTxConfirmed)

	s.swapServices.lightning.AddPaymentCallback(s.OnPayment)

	return nil
}

func (s *SwapService) OnMessageReceived(peerId string, msgTypeString string, payload string) error {
	msgType, err := HexStrToMsgType(msgTypeString)
	if err != nil {
		return err
	}
	msgBytes := []byte(payload)
	log.Printf("[Messenger] From: %s got msgtype: %s payload: %s", peerId, msgTypeString, payload)
	switch msgType {
	case MESSAGETYPE_SWAPOUTREQUEST:
		var msg SwapOutRequest
		err := json.Unmarshal(msgBytes, &msg)
		if err != nil {
			return err
		}
		err = s.OnSwapOutRequestReceived(peerId, msg.ChannelId, msg.SwapId, msg.TakerPubkeyHash, msg.Amount)
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
		if msg.ClaimType == CLAIMTYPE_CLTV {
			err = s.OnCltvClaimMessageReceived(msg.SwapId, msg.ClaimTxId)
		} else if msg.ClaimType == CLAIMTYPE_PREIMAGE {
			err = s.OnPreimageClaimMessageReceived(msg.SwapId, msg.ClaimTxId)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SwapService) OnTxConfirmed(swapId string) error {
	swap, err := s.GetSwap(swapId)
	if err != nil {
		return err
	}
	err = swap.SendEvent(Event_SwapOutSender_OnTxConfirmations, nil)
	if err != nil {
		return err
	}
	return nil
}

func (s *SwapService) OnCltvPassed(swapId string) error {
	swap, err := s.GetSwap(swapId)
	if err != nil {
		return err
	}
	err = swap.SendEvent(Event_SwapOutReceiver_OnCltvPassed, nil)
	if err != nil {
		return err
	}
	return nil
}

// todo check prerequisites
func (s *SwapService) SwapOut(peer string, channelId string, initiator string, amount uint64) (*StateMachine, error) {
	log.Printf("[SwapService] Start swapping out: peer: %s chanId: %s initiator: %s amount %v", peer, channelId, initiator, amount)
	swap := newSwapOutSenderFSM(s.swapServices)
	s.AddSwap(swap.Id, swap)
	err := swap.SendEvent(Event_SwapOutSender_OnSwapOutRequested, &SwapCreationContext{
		amount:      amount,
		initiatorId: initiator,
		peer:        peer,
		channelId:   channelId,
		swapId:      swap.Id,
	})
	if err != nil {
		return nil, err
	}
	return swap, nil
}

func (s *SwapService) OnSwapOutRequestReceived(peer, channelId, swapId, takerPubkeyHash string, amount uint64) error {
	swap := newSwapOutReceiverFSM(swapId, s.swapServices)
	s.AddSwap(swap.Id, swap)
	err := swap.SendEvent(Event_SwapOutReceiver_OnSwapOutRequestReceived, &CreateSwapFromRequestContext{
		amount:          amount,
		peer:            peer,
		channelId:       channelId,
		swapId:          swapId,
		takerPubkeyHash: takerPubkeyHash,
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *SwapService) OnFeeInvoiceReceived(swapId, feeInvoice string) error {
	swap, err := s.GetSwap(swapId)
	if err != nil {
		return err
	}
	err = swap.SendEvent(Event_SwapOutSender_OnFeeInvReceived, &FeeResponse{Invoice: feeInvoice})
	if err != nil {
		return err
	}
	return nil
}

func (s *SwapService) OnFeeInvoicePaid(swapId string) error {
	swap, err := s.GetSwap(swapId)
	if err != nil {
		return err
	}
	err = swap.SendEvent(Event_SwapOutReceiver_OnFeeInvoicePaid, nil)
	if err != nil {
		return err
	}
	return nil
}

func (s *SwapService) OnClaimInvoicePaid(swapId string) error {
	swap, err := s.GetSwap(swapId)
	if err != nil {
		return err
	}
	err = swap.SendEvent(Event_SwapOutReceiver_OnClaimInvoicePaid, nil)
	if err != nil {
		return err
	}
	return nil
}

func (s *SwapService) OnPreimageClaimMessageReceived(swapId string, txId string) error {
	swap, err := s.GetSwap(swapId)
	if err != nil {
		return err
	}
	err = swap.SendEvent(Event_SwapOutReceiver_OnClaimMsgReceived, &ClaimedMessage{ClaimTxId: txId})
	if err != nil {
		return err
	}
	return nil
}

func (s *SwapService) OnCltvClaimMessageReceived(swapId string, txId string) error {
	swap, err := s.GetSwap(swapId)
	if err != nil {
		return err
	}
	err = swap.SendEvent(Event_SwapOutSender_OnCltvClaimMsgReceived, &ClaimedMessage{ClaimTxId: txId})
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
	err = swap.SendEvent(Event_SwapOutSender_OnTxOpenedMessage, &TxOpenedResponse{
		MakerPubkeyHash: makerPubkeyHash,
		Invoice:         claimInvoice,
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
	s.RemoveSwap(swap.Id)
	return nil
}

func (s *SwapService) OnPayment(payment *glightning.Payment) {
	// check if feelabel
	var swapId string
	var err error
	if strings.Contains(payment.Label, "claim_") && len(payment.Label) == (len("claim_")+64) {
		log.Printf("[SwapService] New claim payment received %s", payment.Label)
		swapId = payment.Label[6:]
		err = s.OnClaimInvoicePaid(swapId)
	} else if strings.Contains(payment.Label, "fee_") && len(payment.Label) == (len("fee_")+64) {
		log.Printf("[SwapService] New fee payment received %s", payment.Label)
		swapId = payment.Label[4:]
		err = s.OnFeeInvoicePaid(swapId)
	} else {
		return
	}

	if err != nil {
		log.Printf("error handling onfeeinvoice paid %v", err)
		return
	}
	return
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

func (s *SwapService) ListSwaps() ([]*StateMachine, error) {
	return s.swapServices.swapStore.ListAll()
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

func (s *SwapService) RemoveSwap(swapId string) {
	s.Lock()
	defer s.Unlock()
	delete(s.activeSwaps, swapId)
}
