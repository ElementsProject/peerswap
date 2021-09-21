package swap

// Shared States
const (
	State_SendCancel      StateType = "State_SendCancel"
	State_SwapCanceled    StateType = "State_SwapCanceled"
	State_WaitCltv        StateType = "State_WaitCltv"
	State_ClaimedCltv     StateType = "State_ClaimedCltv"
	State_ClaimedPreimage StateType = "State_ClaimedPreimage"
	State_Done            StateType = "State_Done"
)

// Swap Out Sender States
const (
	State_SwapOutSender_CreateSwap                   StateType = "State_SwapOutSender_CreateSwap"
	State_SwapOutSender_SendRequest                  StateType = "State_SwapOutSender_SendRequest"
	State_SwapOutSender_AwaitFeeResponse             StateType = "State_SwapOutSender_AwaitFeeResponse"
	State_SwapOutSender_PayFeeInvoice                StateType = "State_SwapOutSender_PayFeeInvoice"
	State_SwapOutSender_AwaitTxBroadcastedMessage    StateType = "State_SwapOutSender_AwaitTxBroadcastedMessage"
	State_SwapOutSender_AwaitTxConfirmation          StateType = "State_SwapOutSender_AwaitTxConfirmation"
	State_SwapOutSender_ValidateTxAndPayClaimInvoice StateType = "State_SwapOutSender_ValidateTxAndPayClaimInvoice"
	State_SwapOutSender_ClaimSwap                    StateType = "State_SwapOutSender_ClaimSwap"
)

// Swap Out Receiver states
const (
	State_SwapOutReceiver_CreateSwap               StateType = "State_SwapOutReceiver_CreateSwap"
	State_SwapOutReceiver_SendFeeInvoice           StateType = "State_SwapOutReceiver_SendFeeInvoice"
	State_SwapOutReceiver_AwaitFeeInvoicePayment   StateType = "State_SwapOutReceiver_AwaitFeeInvoicePayment"
	State_SwapOutReceiver_BroadcastOpeningTx       StateType = "State_SwapOutReceiver_BroadcastOpeningTx"
	State_SwapOutReceiver_SendTxBroadcastedMessage StateType = "State_SwapOutReceiver_SendTxBroadcastedMessage"
	State_SwapOutReceiver_AwaitClaimInvoicePayment StateType = "State_SwapOutReceiver_AwaitClaimInvoicePayment"
	State_SwapOutReceiver_SwapAborted              StateType = "State_SwapOutReceiver_Aborted"
	State_SwapOutReceiver_ClaimSwap                StateType = "State_SwapOutReceiver_ClaimSwap"
)

// Swap In Sender States
const (
	State_SwapInSender_CreateSwap               StateType = "State_SwapInSender_CreateSwap"
	State_SwapInSender_SendRequest              StateType = "State_SwapInSender_SendRequest"
	State_SwapInSender_AwaitAgreement           StateType = "State_SwapInSender_AwaitAgreement"
	State_SwapInSender_BroadcastOpeningTx       StateType = "State_SwapInSender_BroadcastOpeningTx"
	State_SwapInSender_SendTxBroadcastedMessage StateType = "State_SwapInSender_SendTxBroadcastedMessage"
	State_SwapInSender_AwaitClaimPayment        StateType = "State_SwapInSender_AwaitClaimPayment"
	State_SwapInSender_ClaimInvPaid             StateType = "State_SwapInSender_ClaimInvPaid"
	State_SwapInSender_ClaimSwap                StateType = "State_SwapInSender_ClaimSwap"
)

// Swap In Receiver States
const (
	State_SwapInReceiver_CreateSwap                   StateType = "State_SwapInReceiver_CreateSwap"
	State_SwapInReceiver_SendAgreement                StateType = "State_SwapInReceiver_SendAgreement"
	State_SwapInReceiver_AwaitTxBroadcastedMessage    StateType = "State_SwapInReceiver_AwaitTxBroadcastedMessage"
	State_SwapInReceiver_AwaitTxConfirmation          StateType = "State_SwapInReceiver_AwaitTxConfirmation"
	State_SwapInReceiver_ValidateTxAndPayClaimInvoice StateType = "State_SwapInReceiver_ValidateTxAndPayClaimInvoice"
	State_SwapInReceiver_ClaimSwap                    StateType = "State_SwapInReceiver_ClaimSwap"
)

// Events
const (
	Event_OnSwapOutStarted     EventType = "Event_OnSwapOutStarted"
	Event_OnFeeInvoiceReceived EventType = "Event_OnFeeInvoiceReceived"

	Event_OnTxOpenedMessage EventType = "Event_OnTxOpenedMessage"
	Event_OnTxConfirmed     EventType = "Event_OnTxConfirmed"

	// todo retrystate? failstate? refundstate?
	Event_OnRetry       EventType = "Event_OnRetry"
	Event_OnClaimedCltv EventType = "Event_OnClaimedCltv"

	Event_OnSwapOutRequestReceived EventType = "Event_OnSwapOutRequestReceived"

	Event_OnFeeInvoicePaid   EventType = "Event_OnFeeInvoicePaid"
	Event_OnClaimInvoicePaid EventType = "Event_OnClaimInvoicePaid"
	Event_OnClaimedPreimage  EventType = "Event_OnClaimedPreimage"
	Event_OnCltvPassed       EventType = "Event_OnCltvPassed"

	Event_OnCancelReceived EventType = "Event_OnCancelReceived"

	Event_ActionSucceeded                  EventType = "Event_ActionSucceeded"
	Event_SwapInSender_OnSwapInRequested   EventType = "Event_SwapInSender_OnSwapInRequested"
	Event_SwapInSender_OnAgreementReceived EventType = "Event_SwapInSender_OnAgreementReceived"
	Event_ActionFailed                     EventType = "Event_ActionFailed"
	Event_SwapInReceiver_OnRequestReceived EventType = "Event_SwapInReceiver_OnRequestReceived"
	Event_Done                             EventType = "Event_Done"
)
