package swap

import "sync"

// swapInSenderFromStore recovers a swap statemachine from the swap store
func swapInSenderFromStore(smData *SwapStateMachine, services *SwapServices) *SwapStateMachine {
	smData.swapServices = services
	smData.States = getSwapInSenderStates()
	return smData
}

// newSwapInSenderFSM returns a new swap statemachine for a swap-in sender
func newSwapInSenderFSM(services *SwapServices, initiatorNodeId, peerNodeId string) *SwapStateMachine {
	swapId := NewSwapId()
	fsm := &SwapStateMachine{
		SwapId:       swapId,
		swapServices: services,
		Type:         SWAPTYPE_IN,
		Role:         SWAPROLE_SENDER,
		States:       getSwapInSenderStates(),
		Data:         NewSwapData(swapId, initiatorNodeId, peerNodeId),
	}
	fsm.stateChange = sync.NewCond(&fsm.stateMutex)
	return fsm
}

// getSwapInSenderStates returns the states for the swap-in sender
func getSwapInSenderStates() States {
	return States{
		Default: State{
			Events: Events{
				Event_SwapInSender_OnSwapInRequested: State_SwapInSender_CreateSwap,
			},
		},
		State_SwapInSender_CreateSwap: {
			Action: &SetBlindingKeyActionWrapper{next: &CreateSwapRequestAction{}},
			Events: Events{
				Event_ActionSucceeded: State_SwapInSender_SendRequest,
				Event_ActionFailed:    State_SwapCanceled,
			},
			FailOnrecover: true,
		},
		State_SwapInSender_SendRequest: {
			Action: &SendMessageAction{},
			Events: Events{
				Event_ActionSucceeded: State_SwapInSender_AwaitAgreement,
				Event_ActionFailed:    State_SwapCanceled,
			},
		},
		State_SwapInSender_AwaitAgreement: {
			Action: &NoOpAction{},
			Events: Events{
				Event_OnCancelReceived:                 State_SwapCanceled,
				Event_OnTimeout:                        State_SendCancel,
				Event_SwapInSender_OnAgreementReceived: State_SwapInSender_BroadcastOpeningTx,
				Event_OnInvalid_Message:                State_SendCancel,
			},
		},
		State_SwapInSender_BroadcastOpeningTx: {
			Action: &CreateAndBroadcastOpeningTransaction{},
			Events: Events{
				Event_ActionSucceeded: State_SwapInSender_SendTxBroadcastedMessage,
				Event_ActionFailed:    State_SendCancel,
			},
		},
		State_SwapInSender_SendTxBroadcastedMessage: {
			Action: &SendMessageWithRetryAction{},
			Events: Events{
				Event_ActionSucceeded: State_SwapInSender_AwaitClaimPayment,
				Event_ActionFailed:    State_WaitCsv,
			},
		},
		State_SwapInSender_AwaitClaimPayment: {
			Action: &AwaitPaymentOrCsvAction{},
			Events: Events{
				Event_OnClaimInvoicePaid:  State_ClaimedPreimage,
				Event_OnCsvPassed:         State_SwapInSender_ClaimSwapCsv,
				Event_OnCancelReceived:    State_WaitCsv,
				Event_OnCoopCloseReceived: State_SwapInSender_ClaimSwapCoop,
				Event_OnInvalid_Message:   State_WaitCsv,
				Event_AlreadyClaimed:      State_ClaimedCsv,
			},
		},
		State_SwapInSender_ClaimSwapCsv: {
			Action: &StopSendMessageWithRetryWrapperAction{next: &ClaimSwapTransactionWithCsv{}},
			Events: Events{
				Event_ActionSucceeded: State_ClaimedCsv,
				Event_OnRetry:         State_SwapInSender_ClaimSwapCsv,
			},
		},
		State_SwapInSender_ClaimSwapCoop: {
			Action: &StopSendMessageWithRetryWrapperAction{next: &ClaimSwapTransactionCoop{}},
			Events: Events{
				Event_ActionSucceeded: State_ClaimedCoop,
				Event_ActionFailed:    State_WaitCsv,
			},
		},
		State_WaitCsv: {
			Action: &StopSendMessageWithRetryWrapperAction{next: &AwaitCsvAction{}},
			Events: Events{
				Event_OnCsvPassed:         State_SwapInSender_ClaimSwapCsv,
				Event_OnCoopCloseReceived: State_SwapInSender_ClaimSwapCoop,
				Event_AlreadyClaimed:      State_ClaimedCsv,
			},
		},
		State_SendCancel: {
			Action: &SendCancelAction{},
			Events: Events{
				Event_ActionSucceeded: State_SwapCanceled,
				Event_ActionFailed:    State_SwapCanceled,
			},
		},
		State_SwapCanceled: {
			Action: &CancelAction{},
		},
		State_ClaimedPreimage: {
			Action: &NoOpDoneAction{},
		},
		State_ClaimedCsv: {
			Action: &NoOpDoneAction{},
		},
		State_ClaimedCoop: {
			Action: &NoOpDoneAction{},
		},
	}
}
