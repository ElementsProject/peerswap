package swap

// Shared States
const (
	State_SendCancel StateType = "State_SendCancel"
)

// Swap Out sender states
const (
	State_SwapOutSender_CreateSwap                   StateType = "State_SwapOutSender_CreateSwap"
	State_SwapOutSender_SendRequest                  StateType = "State_SwapOutSender_SendRequest"
	State_SwapOutSender_AwaitFeeResponse             StateType = "State_SwapOutSender_AwaitFeeResponse"
	State_SwapOutSender_PayFeeInvoice                StateType = "State_SwapOutSender_PayFeeInvoice"
	State_SwapOutSender_AwaitTxBroadcastedMessage    StateType = "State_SwapOutSender_AwaitTxBroadcastedMessage"
	State_SwapOutSender_AwaitTxConfirmation          StateType = "State_SwapOutSender_AwaitTxConfirmation"
	State_SwapOutSender_ValidateTxAndPayClaimInvoice StateType = "State_SwapOutSender_ValidateTxAndPayClaimInvoice"
	State_SwapOutSender_ClaimSwap                    StateType = "State_SwapOutSender_ClaimSwap"
	State_SwapOutSender_SendClaimMessage             StateType = "State_SwapOutSender_SendClaimMessage"
	State_SwapOutSender_AwaitCLTV                    StateType = "State_SwapOutSender_AwaitCLTV"
)

// Swap Out receiver states
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

	Event_ActionSucceeded EventType = "Event_ActionSucceeded"
)
