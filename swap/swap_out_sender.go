package swap

import "sync"

// swapOutSenderFromStore recovers a swap statemachine from the swap store
func swapOutSenderFromStore(smData *SwapStateMachine, services *SwapServices) *SwapStateMachine {
	smData.swapServices = services
	smData.States = getSwapOutSenderStates()
	return smData
}

// newSwapOutSenderFSM returns a new swap statemachine for a swap-out sender
func newSwapOutSenderFSM(services *SwapServices, initiatorNodeId, peerNodeId string) *SwapStateMachine {
	swapId := NewSwapId()
	fsm := &SwapStateMachine{
		SwapId:       swapId,
		swapServices: services,
		Type:         SWAPTYPE_OUT,
		Role:         SWAPROLE_SENDER,
		States:       getSwapOutSenderStates(),
		Data:         NewSwapData(swapId, initiatorNodeId, peerNodeId),
	}
	fsm.stateChange = sync.NewCond(&fsm.stateMutex)
	return fsm
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
				Event_ActionFailed:    State_SwapCanceled,
			},
			FailOnrecover: true,
		},
		State_SwapOutSender_SendRequest: {
			Action: &SendMessageAction{},
			Events: Events{
				Event_ActionFailed:    State_SwapCanceled,
				Event_ActionSucceeded: State_SwapOutSender_AwaitAgreement,
			},
			FailOnrecover: true,
		},
		State_SwapOutSender_AwaitAgreement: {
			Action: &NoOpAction{},
			Events: Events{
				Event_OnCancelReceived:     State_SwapCanceled,
				Event_OnTimeout:            State_SendCancel,
				Event_OnFeeInvoiceReceived: State_SwapOutSender_PayFeeInvoice,
				Event_OnInvalid_Message:    State_SendCancel,
			},
			FailOnrecover: true,
		},
		State_SwapOutSender_PayFeeInvoice: {
			Action: &PayFeeInvoiceAction{},
			Events: Events{
				Event_ActionFailed:    State_SendCancel,
				Event_ActionSucceeded: State_SwapOutSender_AwaitTxBroadcastedMessage,
			},
			FailOnrecover: true,
		},
		State_SwapOutSender_AwaitTxBroadcastedMessage: {
			Action: &SetStartingBlockHeightAction{},
			Events: Events{
				Event_OnCancelReceived:  State_SwapCanceled,
				Event_OnTxOpenedMessage: State_SwapOutSender_AwaitTxConfirmation,
				Event_ActionFailed:      State_SwapOutSender_SendPrivkey,
				Event_OnInvalid_Message: State_SendCancel,
				// fixme: We might want to timeout here, but we have to be
				// careful not to loose our funds, maybe we want to set the
				// time in the range of a CSV delta, or we just say: 10m and go!
				// Event_OnTimeout:         State_SwapOutSender_SendPrivkey,
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
