package swap

import (
	"encoding/hex"
	"errors"
)

const (
	State_SwapInReceiver_Init                 StateType = "State_SwapInReceiver_Init"
	State_SwapInReceiver_RequestReceived      StateType = "State_SwapInReceiver_RequestReceived"
	State_SwapInReceiver_AgreementSent        StateType = "State_SwapInReceiver_AgreementSent"
	State_SwapInReceiver_OpeningTxBroadcasted StateType = "State_SwapInReceiver_OpeningTxBroadcasted"
	State_SwapInReceiver_WaitForConfirmations StateType = "State_SwapInReceiver_WaitForConfirmations"
	State_SwapInReceiver_OpeningTxConfirmed   StateType = "State_SwapInReceiver_OpeningTxConfirmed"
	State_SwapInReceiver_ClaimInvoicePaid     StateType = "State_SwapInReceiver_ClaimInvoicePaid"
	State_ClaimedCltv                         StateType = "State_ClaimedCltv"
	State_ClaimedPreimage                     StateType = "State_ClaimedPreimage"

	Event_SwapInReceiver_OnRequestReceived  EventType = "Event_SwapInReceiver_OnRequestReceived"
	Event_SwapInReceiver_OnSwapCreated      EventType = "Event_SwapInReceiver_OnSwapCreated"
	Event_SwapInReceiver_OnAgreementSent    EventType = "Event_SwapInReceiver_OnAgreementSent"
	Event_SwapInReceiver_OnClaimInvoicePaid EventType = "Event_SwapInReceiver_OnClaimInvoicePaid"
)

// todo check for policy / balance
// SwapInReceiverInitAction creates the swap-in process
type SwapInReceiverInitAction struct{}

func (s *SwapInReceiverInitAction) Execute(services *SwapServices, swap *SwapData) EventType {
	newSwap := NewSwapFromRequest(swap.PeerNodeId, swap.Id, swap.Amount, swap.ChannelId, SWAPTYPE_IN)
	*swap = *newSwap

	pubkey := swap.GetPrivkey().PubKey()
	swap.Role = SWAPROLE_RECEIVER
	swap.TakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())

	return Event_SwapInReceiver_OnSwapCreated
}

// SwapInReceiverRequestReceivedAction sends the agreement message to the peer
type SwapInReceiverRequestReceivedAction struct{}

func (s *SwapInReceiverRequestReceivedAction) Execute(services *SwapServices, swap *SwapData) EventType {
	response := &SwapInAgreementMessage{
		SwapId:          swap.Id,
		TakerPubkeyHash: swap.TakerPubkeyHash,
	}
	err := services.messenger.SendMessage(swap.PeerNodeId, response)
	if err != nil {
		return swap.HandleError(err)
	}
	return Event_SwapInReceiver_OnAgreementSent
}

// SwapInReceiverOpeningTxBroadcastedAction checks if the
// invoice is correct and adss the transaction to the txwatcher
type SwapInReceiverOpeningTxBroadcastedAction struct{}

func (s *SwapInReceiverOpeningTxBroadcastedAction) Execute(services *SwapServices, swap *SwapData) EventType {

	invoice, err := services.lightning.DecodePayreq(swap.ClaimInvoice)
	if err != nil {
		return swap.HandleError(err)
	}

	if invoice.Amount > (swap.Amount)*1000 {
		return swap.HandleError(errors.New("invalid invoice price"))
	}
	swap.ClaimPaymentHash = invoice.PHash

	return Event_Action_Success

}

func (s *SwapData) HandleError(err error) EventType {
	s.LastErr = err
	return Event_ActionFailed
}

// SwapInWaitForConfirmationsAction adds the swap opening tx to the txwatcher
type SwapInWaitForConfirmationsAction struct{}

func (s *SwapInWaitForConfirmationsAction) Execute(services *SwapServices, swap *SwapData) EventType {
	err := services.onchain.AddWaitForConfirmationTx(swap.Id, swap.OpeningTxId)
	if err != nil{
		return swap.HandleError(err)
	}
	return NoOp
}

// SwapInWaitForConfirmationsAction pays the claim invoice
type SwapInReceiverOpeningTxConfirmedAction struct{}

func (s *SwapInReceiverOpeningTxConfirmedAction) Execute(services *SwapServices, swap *SwapData) EventType {
	ok, err := services.onchain.ValidateTx(swap.GetOpeningParams(), swap.OpeningTxId, swap.OpeningTxVout)
	if err != nil {
		return swap.HandleError(err)
	}
	if !ok {
		return Event_SwapOutSender_OnAbortSwapInternal
	}
	preimage, err := services.lightning.RebalancePayment(swap.ClaimInvoice, swap.ChannelId)
	if err != nil {
		return swap.HandleError(err)
	}
	swap.ClaimPreimage = preimage

	return Event_SwapInReceiver_OnClaimInvoicePaid
}

// SwapInWaitForConfirmationsAction spends the opening transaction to the nodes liquid wallet
type SwapInReceiverClaimInvoicePaidAction struct{}

func (s *SwapInReceiverClaimInvoicePaidAction) Execute(services *SwapServices, swap *SwapData) EventType {
	if swap.ClaimTxId != "" {
		err := CreatePreimageSpendingTransaction(services, swap)
		if err != nil {
			return swap.HandleError(err)
		}
	}
	msg := &ClaimedMessage{
		SwapId:    swap.Id,
		ClaimType: CLAIMTYPE_PREIMAGE,
		ClaimTxId: swap.ClaimTxId,
	}
	err := services.messenger.SendMessage(swap.PeerNodeId, msg)
	if err != nil {
		return swap.HandleError(err)
	}
	return Event_OnClaimedPreimage
}

type CancelAction struct{}

func (c *CancelAction) Execute(services *SwapServices, swap *SwapData) EventType {
	if swap.LastErr != nil {
		swap.LastErrString = swap.LastErr.Error()
	}
	return NoOp
}

// swapInReceiverFromStore recovers a swap statemachine from the swap store
func swapInReceiverFromStore(smData *SwapStateMachine, services *SwapServices) *SwapStateMachine {
	smData.swapServices = services
	smData.States = getSwapInReceiverStates()
	return smData
}

// newSwapInReceiverFSM returns a new swap statemachine for a swap-in receiver
func newSwapInReceiverFSM(id string, services *SwapServices) *SwapStateMachine {
	return &SwapStateMachine{
		Id:           id,
		swapServices: services,
		Type:         SWAPTYPE_IN,
		Role:         SWAPROLE_RECEIVER,
		States:       getSwapInReceiverStates(),
		Data:         &SwapData{},
	}
}

// getSwapInReceiverStates returns the states for the swap-in receiver
func getSwapInReceiverStates() States {
	return States{
		Default: State{
			Events: Events{
				Event_SwapInReceiver_OnRequestReceived: State_SwapInReceiver_Init,
			},
		},
		State_SwapInReceiver_Init: {
			Action: &SwapInReceiverInitAction{},
			Events: Events{
				Event_SwapInReceiver_OnSwapCreated: State_SwapInReceiver_RequestReceived,
				Event_ActionFailed:                 State_SendCancel,
			},
		},
		State_SwapInReceiver_RequestReceived: {
			Action: &SwapInReceiverRequestReceivedAction{},
			Events: Events{
				Event_SwapInReceiver_OnAgreementSent: State_SwapInReceiver_AgreementSent,
				Event_ActionFailed:                   State_SendCancel,
			},
		},
		State_SwapInReceiver_AgreementSent: {
			Action: &NoOpAction{},
			Events: Events{
				Event_OnTxOpenedMessage: State_SwapInReceiver_OpeningTxBroadcasted,
				Event_OnCancelReceived:  State_SwapCanceled,
			},
		},
		State_SwapInReceiver_OpeningTxBroadcasted: {
			Action: &SwapInReceiverOpeningTxBroadcastedAction{},
			Events: Events{
				Event_Action_Success: State_SwapInReceiver_WaitForConfirmations,
				Event_ActionFailed:   State_SendCancel,
			},
		},
		State_SwapInReceiver_WaitForConfirmations: {
			Action: &SwapInWaitForConfirmationsAction{},
			Events: Events{
				Event_OnTxConfirmed:    State_SwapInReceiver_OpeningTxConfirmed,
				Event_OnCancelReceived: State_SwapCanceled,
			},
		},
		State_SwapInReceiver_OpeningTxConfirmed: {
			Action: &SwapInReceiverOpeningTxConfirmedAction{},
			Events: Events{
				Event_SwapInReceiver_OnClaimInvoicePaid: State_SwapInReceiver_ClaimInvoicePaid,
				Event_ActionFailed:                      State_SendCancel,
			},
		},
		State_SwapInReceiver_ClaimInvoicePaid: {
			Action: &SwapInReceiverClaimInvoicePaidAction{},
			Events: Events{
				Event_OnClaimedPreimage: State_ClaimedPreimage,
			},
		},
		State_ClaimedCltv: {
			Action: &NoOpAction{},
		},
		State_ClaimedPreimage: {
			Action: &NoOpAction{},
		},
		State_SendCancel: {
			Action: &SendCancelAction{},
			Events: Events{
				Event_Action_Success: State_SwapCanceled,
			},
		},

		State_SwapCanceled: {
			Action: &CancelAction{},
			Events: Events{
				Event_OnClaimedCltv: State_ClaimedCltv,
			},
		},
	}
}
