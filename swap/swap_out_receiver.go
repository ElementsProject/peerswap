package swap

import "sync"

// todo every send message should be it's own state / action, if msg sending fails, tx will be broadcasted again / error occurs
// or make the sender a more sophisticated program which tries resending...
const ()

// swapOutReceiverFromStore recovers a swap statemachine from the swap store
func swapOutReceiverFromStore(smData *SwapStateMachine, services *SwapServices) *SwapStateMachine {
	smData.swapServices = services
	smData.States = getSwapOutReceiverStates()
	return smData
}

// newSwapOutReceiverFSM returns a new swap statemachine for a swap-out receiver
func newSwapOutReceiverFSM(swapId *SwapId, services *SwapServices, peer string) *SwapStateMachine {
	fsm := &SwapStateMachine{
		SwapId:       swapId,
		swapServices: services,
		Type:         SWAPTYPE_OUT,
		Role:         SWAPROLE_RECEIVER,
		States:       getSwapOutReceiverStates(),
		Data:         NewSwapDataFromRequest(swapId, peer),
	}
	fsm.stateChange = sync.NewCond(&fsm.stateMutex)
	return fsm
}

// getSwapOutReceiverStates returns the states for the swap-out receiver
func getSwapOutReceiverStates() States {
	return States{
		Default: State{
			Events: Events{
				Event_OnSwapOutRequestReceived: State_SwapOutReceiver_CreateSwap,
				Event_OnInvalid_Message:        State_SendCancel,
			},
		},
		State_SwapOutReceiver_CreateSwap: {
			Action: &CheckRequestWrapperAction{next: &SetBlindingKeyActionWrapper{next: &CreateSwapOutFromRequestAction{}}},
			Events: Events{
				Event_ActionSucceeded: State_SwapOutReceiver_SendFeeInvoice,
				Event_ActionFailed:    State_SendCancel,
			},
			FailOnrecover: true,
		},
		State_SwapOutReceiver_SendFeeInvoice: {
			Action: &SendMessageAction{},
			Events: Events{
				Event_ActionFailed:    State_SendCancel,
				Event_ActionSucceeded: State_SwapOutReceiver_AwaitFeeInvoicePayment,
			},
		},
		State_SwapOutReceiver_AwaitFeeInvoicePayment: {
			Action: &AwaitFeeInvoicePayment{},
			Events: Events{
				Event_OnFeeInvoicePaid: State_SwapOutReceiver_BroadcastOpeningTx,
				Event_OnCancelReceived: State_SwapCanceled,
				Event_ActionFailed:     State_SendCancel,
			},
			FailOnrecover: true,
		},
		State_SwapOutReceiver_BroadcastOpeningTx: {
			Action: &CreateAndBroadcastOpeningTransaction{},
			Events: Events{
				Event_ActionSucceeded: State_SwapOutReceiver_SendTxBroadcastedMessage,
				Event_ActionFailed:    State_SendCancel,
			},
		},
		State_SwapOutReceiver_SendTxBroadcastedMessage: {
			Action: &SendMessageWithRetryAction{},
			Events: Events{
				Event_ActionSucceeded: State_SwapOutReceiver_AwaitClaimInvoicePayment,
				Event_ActionFailed:    State_WaitCsv,
			},
		},
		State_SwapOutReceiver_AwaitClaimInvoicePayment: {
			Action: &AwaitPaymentOrCsvAction{},
			Events: Events{
				Event_OnClaimInvoicePaid:  State_ClaimedPreimage,
				Event_OnCancelReceived:    State_WaitCsv,
				Event_OnCoopCloseReceived: State_SwapOutReceiver_ClaimSwapCoop,
				Event_OnCsvPassed:         State_SwapOutReceiver_ClaimSwapCsv,
				Event_OnInvalid_Message:   State_WaitCsv,
			},
		},
		State_SwapOutReceiver_ClaimSwapCoop: {
			Action: &StopSendMessageWithRetryWrapperAction{next: &ClaimSwapTransactionCoop{}},
			Events: Events{
				Event_ActionSucceeded: State_ClaimedCoop,
				Event_ActionFailed:    State_WaitCsv,
			},
		},
		State_WaitCsv: {
			Action: &StopSendMessageWithRetryWrapperAction{next: &AwaitCsvAction{}},
			Events: Events{
				Event_OnCsvPassed:         State_SwapOutReceiver_ClaimSwapCsv,
				Event_OnCoopCloseReceived: State_SwapOutReceiver_ClaimSwapCoop,
			},
		},
		State_SwapOutReceiver_ClaimSwapCsv: {
			Action: &StopSendMessageWithRetryWrapperAction{next: &ClaimSwapTransactionWithCsv{}},
			Events: Events{
				Event_ActionSucceeded: State_ClaimedCsv,
				Event_OnRetry:         State_SwapOutReceiver_ClaimSwapCsv,
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
		State_ClaimedCsv: {
			Action: &AddSuspiciousPeerAction{next: &NoOpDoneAction{}},
		},
		State_ClaimedPreimage: {
			Action: &NoOpDoneAction{},
		},
		State_ClaimedCoop: {
			Action: &NoOpDoneAction{},
		},
	}
}
