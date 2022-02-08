package swap

// swapInReceiverFromStore recovers a swap statemachine from the swap store
func swapInReceiverFromStore(smData *SwapStateMachine, services *SwapServices) *SwapStateMachine {
	smData.swapServices = services
	smData.States = getSwapInReceiverStates()
	return smData
}

// newSwapInReceiverFSM returns a new swap statemachine for a swap-in receiver
func newSwapInReceiverFSM(swapId *SwapId, services *SwapServices, peer string) *SwapStateMachine {
	return &SwapStateMachine{
		Id:           swapId.String(),
		SwapId:       swapId,
		swapServices: services,
		Type:         SWAPTYPE_IN,
		Role:         SWAPROLE_RECEIVER,
		States:       getSwapInReceiverStates(),
		Data:         NewSwapDataFromRequest(swapId, peer),
	}
}

// getSwapInReceiverStates returns the states for the swap-in receiver
func getSwapInReceiverStates() States {
	return States{
		Default: State{
			Events: Events{
				Event_SwapInReceiver_OnRequestReceived: State_SwapInReceiver_CreateSwap,
				Event_OnInvalid_Message:                State_SendCancel,
			},
		},
		State_SwapInReceiver_CreateSwap: {
			Action: &CheckRequestWrapperAction{next: &SwapInReceiverInitAction{}},
			Events: Events{
				Event_ActionSucceeded: State_SwapInReceiver_SendAgreement,
				Event_ActionFailed:    State_SendCancel,
			},
			FailOnrecover: true,
		},
		State_SwapInReceiver_SendAgreement: {
			Action: &SendMessageAction{},
			Events: Events{
				Event_ActionSucceeded: State_SwapInReceiver_AwaitTxBroadcastedMessage,
				Event_ActionFailed:    State_SendCancel,
			},
		},
		State_SwapInReceiver_AwaitTxBroadcastedMessage: {
			Action: &SetStartingBlockHeightAction{},
			Events: Events{
				Event_OnTxOpenedMessage: State_SwapInReceiver_AwaitTxConfirmation,
				Event_OnCancelReceived:  State_SwapCanceled,
				Event_ActionFailed:      State_SendCancel,
				Event_OnInvalid_Message: State_SendCancel,
				// fixme: We have to tinker about a good value for a timeout
				// here.
				Event_OnTimeout: State_SwapInReceiver_SendPrivkey,
			},
		},
		State_SwapInReceiver_AwaitTxConfirmation: {
			Action: &StopSendMessageWithRetryWrapperAction{next: &AwaitTxConfirmationAction{}},
			Events: Events{
				Event_OnTxConfirmed:    State_SwapInReceiver_ValidateTxAndPayClaimInvoice,
				Event_ActionFailed:     State_SendCancel,
				Event_OnCancelReceived: State_SwapInReceiver_SendPrivkey,
			},
		},
		State_SwapInReceiver_ValidateTxAndPayClaimInvoice: {
			Action: &ValidateTxAndPayClaimInvoiceAction{},
			Events: Events{
				Event_ActionSucceeded: State_SwapInReceiver_ClaimSwap,
				Event_ActionFailed:    State_SwapInReceiver_SendPrivkey,
			},
		},
		State_SwapInReceiver_SendPrivkey: {
			Action: &TakerSendPrivkeyAction{},
			Events: Events{
				Event_ActionFailed:    State_SendCancel,
				Event_ActionSucceeded: State_SwapInReceiver_SendCoopClose,
			},
		},
		State_SwapInReceiver_SendCoopClose: {
			Action: &SendMessageAction{},
			Events: Events{
				Event_ActionSucceeded: State_ClaimedCoop,
				Event_ActionFailed:    State_SendCancel,
			},
		},
		State_SwapInReceiver_ClaimSwap: {
			Action: &ClaimSwapTransactionWithPreimageAction{},
			Events: Events{
				Event_ActionSucceeded: State_ClaimedPreimage,
				Event_OnRetry:         State_SwapInReceiver_ClaimSwap,
			},
		},
		State_ClaimedPreimage: {
			Action: &NoOpDoneAction{},
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
		State_ClaimedCoop: {
			Action: &NoOpDoneAction{},
		},
	}
}
