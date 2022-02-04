package swap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/sputn1ck/peerswap/isdev"
	"github.com/sputn1ck/peerswap/messages"
)

// SwapInSenderCreateSwapAction creates the swap data
type CreateSwapRequestAction struct{}

//todo validate data
func (a *CreateSwapRequestAction) Execute(services *SwapServices, swap *SwapData) EventType {
	nextMessage, nextMessageType, err := MarshalPeerswapMessage(swap.GetRequest())
	if err != nil {
		return swap.HandleError(err)
	}

	swap.NextMessage = nextMessage
	swap.NextMessageType = nextMessageType

	toCtx, cancel := context.WithCancel(context.Background())
	swap.toCancel = cancel
	services.toService.addNewTimeOut(toCtx, 10*time.Minute, swap.Id.String())

	return Event_ActionSucceeded
}

type SendMessageAction struct{}

func (s *SendMessageAction) Execute(services *SwapServices, swap *SwapData) EventType {
	if swap.NextMessage == nil {
		return swap.HandleError(errors.New("swap.NextMessage is nil"))
	}

	err := services.messenger.SendMessage(swap.PeerNodeId, swap.NextMessage, swap.NextMessageType)
	if err != nil {
		return swap.HandleError(err)
	}
	return Event_ActionSucceeded
}

type SendMessageWithRetryAction struct{}

func (s *SendMessageWithRetryAction) Execute(services *SwapServices, swap *SwapData) EventType {
	if swap.NextMessage == nil {
		return swap.HandleError(errors.New("swap.NextMessage is nil"))
	}

	// Send message repeated as we really want the message to be received at some point!
	rm := messages.NewRedundantMessenger(services.messenger, 10*time.Second)
	err := services.messengerManager.AddSender(swap.Id.String(), rm)
	if err != nil {
		return swap.HandleError(err)
	}
	rm.SendMessage(swap.PeerNodeId, swap.NextMessage, swap.NextMessageType)

	return Event_ActionSucceeded
}

// PayFeeInvoiceAction checks the feeinvoice and pays it
type PayFeeInvoiceAction struct{}

func (r *PayFeeInvoiceAction) Execute(services *SwapServices, swap *SwapData) EventType {
	ll := services.lightning
	// policy := services.policy
	_, msatAmt, err := ll.DecodePayreq(swap.SwapOutAgreement.Payreq)
	if err != nil {
		log.Printf("error decoding %v", err)
		return swap.HandleError(err)
	}
	swap.OpeningTxFee = msatAmt / 1000
	// todo check peerId
	/*
		if !policy.ShouldPayFee(swap.Amount, invoice.Amount, swap.PeerNodeId, swap.ChannelId) {

			log.Printf("won't pay fee %v", err)
			return Event_ActionFailed
		}
	*/
	preimage, err := ll.PayInvoice(swap.SwapOutAgreement.Payreq)
	if err != nil {
		log.Printf("error paying out %v", err)
		return swap.HandleError(err)
	}
	swap.FeePreimage = preimage
	return Event_ActionSucceeded
}

// AwaitTxConfirmationAction  checks the claim invoice and adds the transaction to the txwatcher
type AwaitTxConfirmationAction struct{}

//todo this will not ever throw an error
func (t *AwaitTxConfirmationAction) Execute(services *SwapServices, swap *SwapData) EventType {
	txWatcher, wallet, _, err := services.getOnChainServices(swap.GetChain())
	if err != nil {
		return swap.HandleError(err)
	}

	phash, msatAmount, err := services.lightning.DecodePayreq(swap.OpeningTxBroadcasted.Payreq)
	if err != nil {
		return swap.HandleError(err)
	}

	if msatAmount != swap.GetAmount()*1000 {
		return swap.HandleError(fmt.Errorf("invoice amount does not equal swap amount, invoice: %v, swap %v", swap.OpeningTxBroadcasted.Payreq, swap.GetAmount()))
	}

	swap.ClaimPaymentHash = phash

	wantScript, err := wallet.GetOutputScript(swap.GetOpeningParams())
	if err != nil {
		return swap.HandleError(err)
	}

	txWatcher.AddWaitForConfirmationTx(swap.Id.String(), swap.OpeningTxBroadcasted.TxId, swap.OpeningTxBroadcasted.ScriptOut, swap.StartingBlockHeight, wantScript)
	return NoOp
}

// todo

// ValidateTxAndPayClaimInvoiceAction pays the claim invoice
type ValidateTxAndPayClaimInvoiceAction struct{}

func (p *ValidateTxAndPayClaimInvoiceAction) Execute(services *SwapServices, swap *SwapData) EventType {
	lc := services.lightning
	_, _, validator, err := services.getOnChainServices(swap.GetChain())
	if err != nil {
		return swap.HandleError(err)
	}

	// todo get opening tx hex
	ok, err := validator.ValidateTx(swap.GetOpeningParams(), swap.OpeningTxHex)
	if err != nil {
		return swap.HandleError(err)
	}
	if !ok {
		return swap.HandleError(errors.New("tx is not valid"))
	}

	var retryTime time.Duration = 120 * time.Second
	if isdev.FastTests() {
		// Retry time should be in [s].
		prtStr := os.Getenv("PAYMENT_RETRY_TIME")
		if prtStr != "" {
			prtInt, err := strconv.Atoi(prtStr)
			if err != nil {
				log.Printf("could not read from PAYMENT_RETRY_TIME")
			} else if prtInt < 1 {
				log.Printf("PAYMENT_RETRY_TIME must be be positive int representing seconds")
			} else {
				retryTime = time.Duration(prtInt) * time.Second
			}
		}
	}

	ctx, done := context.WithTimeout(context.Background(), retryTime)
	defer done()
	var preimageString string
paymentLoop:
	for {
		select {
		case <-ctx.Done():
			break paymentLoop
		default:
			preimageString, err = lc.RebalancePayment(swap.OpeningTxBroadcasted.Payreq, swap.GetScid())
			if err != nil {
				log.Printf("error trying to pay invoice: %v", err)
			}
			if preimageString != "" {
				swap.ClaimPreimage = preimageString
				break paymentLoop
			}
			time.Sleep(time.Second * 10)
			log.Printf("RETRY paying invoice")
		}
	}
	if preimageString == "" {
		return swap.HandleError(fmt.Errorf("could not pay invoice, lastErr %w", err))
	}
	return Event_ActionSucceeded
}

type NoOpAction struct{}

func (n *NoOpAction) Execute(services *SwapServices, swap *SwapData) EventType {
	return NoOp
}

type NoOpDoneAction struct{}

func (a *NoOpDoneAction) Execute(services *SwapServices, swap *SwapData) EventType {
	// Remove possible message sender
	services.messengerManager.RemoveSender(swap.Id.String())

	return Event_Done
}

// swapOutSenderFromStore recovers a swap statemachine from the swap store
func swapOutSenderFromStore(smData *SwapStateMachine, services *SwapServices) *SwapStateMachine {
	smData.swapServices = services
	smData.States = getSwapOutSenderStates()
	return smData
}

// newSwapOutSenderFSM returns a new swap statemachine for a swap-out sender
func newSwapOutSenderFSM(services *SwapServices) *SwapStateMachine {
	swapId := NewSwapId()
	return &SwapStateMachine{
		Id:           swapId.String(),
		SwapId:       swapId,
		swapServices: services,
		Type:         SWAPTYPE_OUT,
		Role:         SWAPROLE_SENDER,
		States:       getSwapOutSenderStates(),
		Data:         &SwapData{},
	}
}

// getSwapOutSenderStates returns the states for the swap-out sender
func getSwapOutSenderStates() States {
	return States{
		Default: State{
			Events: Events{
				Event_OnSwapOutStarted: State_SwapOutSender_CreateSwap,
			},
		},
		State_SwapOutSender_CreateSwap: {
			Action: &CreateSwapRequestAction{},
			Events: Events{
				Event_ActionSucceeded: State_SwapOutSender_SendRequest,
			},
		},
		State_SwapOutSender_SendRequest: {
			Action: &SendMessageAction{},
			Events: Events{
				Event_ActionFailed:    State_SwapCanceled,
				Event_ActionSucceeded: State_SwapOutSender_AwaitAgreement,
			},
		},
		State_SwapOutSender_AwaitAgreement: {
			Action: &NoOpAction{},
			Events: Events{
				Event_OnCancelReceived:     State_SwapCanceled,
				Event_OnTimeout:            State_SendCancel,
				Event_OnFeeInvoiceReceived: State_SwapOutSender_PayFeeInvoice,
				Event_OnInvalid_Message:    State_SendCancel,
			},
		},
		State_SwapOutSender_PayFeeInvoice: {
			Action: &PayFeeInvoiceAction{},
			Events: Events{
				Event_ActionFailed:    State_SendCancel,
				Event_ActionSucceeded: State_SwapOutSender_AwaitTxBroadcastedMessage,
			},
		},
		State_SwapOutSender_AwaitTxBroadcastedMessage: {
			Action: &SetStartingBlockHeightAction{},
			Events: Events{
				Event_OnCancelReceived:  State_SwapCanceled,
				Event_OnTxOpenedMessage: State_SwapOutSender_AwaitTxConfirmation,
				Event_OnInvalid_Message: State_SendCancel,
				Event_OnTimeout:         State_SwapOutSender_SendPrivkey,
			},
		},
		State_SendCancel: {
			Action: &SendCancelAction{},
			Events: Events{
				Event_ActionSucceeded: State_SwapCanceled,
				Event_ActionFailed:    State_SwapCanceled,
			},
		},
		State_SwapOutSender_AwaitTxConfirmation: {
			Action: &AwaitTxConfirmationAction{},
			Events: Events{
				Event_ActionFailed:  State_SwapOutSender_SendPrivkey,
				Event_OnTxConfirmed: State_SwapOutSender_ValidateTxAndPayClaimInvoice,
			},
		},
		State_SwapOutSender_ValidateTxAndPayClaimInvoice: {
			Action: &ValidateTxAndPayClaimInvoiceAction{},
			Events: Events{
				Event_ActionFailed:    State_SwapOutSender_SendPrivkey,
				Event_ActionSucceeded: State_SwapOutSender_ClaimSwap,
			},
		},
		State_SwapOutSender_ClaimSwap: {
			Action: &ClaimSwapTransactionWithPreimageAction{},
			Events: Events{
				Event_ActionSucceeded: State_ClaimedPreimage,
				Event_OnRetry:         State_SwapOutSender_ClaimSwap,
			},
		},
		State_SwapOutSender_SendPrivkey: {
			Action: &TakerSendPrivkeyAction{},
			Events: Events{
				Event_ActionFailed:    State_SendCancel,
				Event_ActionSucceeded: State_SwapOutSender_SendCoopClose,
			},
		},
		State_SwapOutSender_SendCoopClose: {
			Action: &SendMessageAction{},
			Events: Events{
				Event_ActionFailed:    State_SendCancel,
				Event_ActionSucceeded: State_ClaimedCoop,
			},
		},
		State_SwapCanceled: {
			Action: &CancelAction{},
		},
		State_ClaimedPreimage: {
			Action: &NoOpDoneAction{},
		},
		State_ClaimedCoop: {
			Action: &NoOpDoneAction{},
		},
	}
}
