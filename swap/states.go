package swap

// Shared States
const (
	State_SendCancel      StateType = "State_SendCancel"
	State_SwapCanceled    StateType = "State_SwapCanceled"
	State_WaitCsv         StateType = "State_WaitCsv"
	State_ClaimedCsv      StateType = "State_ClaimedCsv"
	State_ClaimedPreimage StateType = "State_ClaimedPreimage"
	State_ClaimedCoop     StateType = "State_ClaimedCoop"
)

// Swap Out Sender States
const (
	State_SwapOutSender_CreateSwap                   StateType = "State_SwapOutSender_CreateSwap"
	State_SwapOutSender_SendRequest                  StateType = "State_SwapOutSender_SendRequest"
	State_SwapOutSender_AwaitAgreement               StateType = "State_SwapOutSender_AwaitAgreement"
	State_SwapOutSender_PayFeeInvoice                StateType = "State_SwapOutSender_PayFeeInvoice"
	State_SwapOutSender_AwaitTxBroadcastedMessage    StateType = "State_SwapOutSender_AwaitTxBroadcastedMessage"
	State_SwapOutSender_AwaitTxConfirmation          StateType = "State_SwapOutSender_AwaitTxConfirmation"
	State_SwapOutSender_ValidateTxAndPayClaimInvoice StateType = "State_SwapOutSender_ValidateTxAndPayClaimInvoice"
	State_SwapOutSender_ClaimSwap                    StateType = "State_SwapOutSender_ClaimSwap"
	State_SwapOutSender_SendPrivkey                  StateType = "State_SwapOutSender_SendPrivkey"
	State_SwapOutSender_SendCoopClose                StateType = "State_SwapOutSender_SendCoopClose"
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
	State_SwapOutReceiver_ClaimSwapCsv             StateType = "State_SwapOutReceiver_ClaimSwapCsv"
	State_SwapOutReceiver_ClaimSwapCoop            StateType = "State_SwapOutReceiver_ClaimSwapCoop"
)

// Swap In Sender States
const (
	State_SwapInSender_CreateSwap               StateType = "State_SwapInSender_CreateSwap"
	State_SwapInSender_SendRequest              StateType = "State_SwapInSender_SendRequest"
	State_SwapInSender_AwaitAgreement           StateType = "State_SwapInSender_AwaitAgreement"
	State_SwapInSender_BroadcastOpeningTx       StateType = "State_SwapInSender_BroadcastOpeningTx"
	State_SwapInSender_SendTxBroadcastedMessage StateType = "State_SwapInSender_SendTxBroadcastedMessage"
	State_SwapInSender_AwaitClaimPayment        StateType = "State_SwapInSender_AwaitClaimPayment"
	State_SwapInSender_ClaimSwapCsv             StateType = "State_SwapInSender_ClaimSwapCsv"
	State_SwapInSender_ClaimSwapCoop            StateType = "State_SwapInSender_ClaimSwapCoop"
)

// Swap In Receiver States
const (
	State_SwapInReceiver_CreateSwap                   StateType = "State_SwapInReceiver_CreateSwap"
	State_SwapInReceiver_SendAgreement                StateType = "State_SwapInReceiver_SendAgreement"
	State_SwapInReceiver_AwaitTxBroadcastedMessage    StateType = "State_SwapInReceiver_AwaitTxBroadcastedMessage"
	State_SwapInReceiver_AwaitTxConfirmation          StateType = "State_SwapInReceiver_AwaitTxConfirmation"
	State_SwapInReceiver_ValidateTxAndPayClaimInvoice StateType = "State_SwapInReceiver_ValidateTxAndPayClaimInvoice"
	State_SwapInReceiver_ClaimSwap                    StateType = "State_SwapInReceiver_ClaimSwap"
	State_SwapInReceiver_SendPrivkey                  StateType = "State_SwapOutSender_SendPrivkey"
	State_SwapInReceiver_SendCoopClose                StateType = "State_SwapInReceiver_SendCoopClose"
)

// Events
const (
	Event_OnSwapOutStarted     EventType = "Event_OnSwapOutStarted"
	Event_OnFeeInvoiceReceived EventType = "Event_OnFeeInvoiceReceived"

	Event_OnTxOpenedMessage EventType = "Event_OnTxOpenedMessage"
	Event_OnTxConfirmed     EventType = "Event_OnTxConfirmed"

	// todo retrystate? failstate? refundstate?
	Event_OnRetry      EventType = "Event_OnRetry"
	Event_OnClaimedCsv EventType = "Event_OnClaimedCsv"

	Event_OnSwapOutRequestReceived EventType = "Event_OnSwapOutRequestReceived"

	Event_OnFeeInvoicePaid   EventType = "Event_OnFeeInvoicePaid"
	Event_OnClaimInvoicePaid EventType = "Event_OnClaimInvoicePaid"
	Event_OnCsvPassed        EventType = "Event_OnCsvPassed"

	Event_OnCancelReceived    EventType = "Event_OnCancelReceived"
	Event_OnCoopCloseReceived EventType = "Event_OnCoopCloseReceived"

	Event_OnTimeout = "Event_OnTimeout"

	Event_ActionSucceeded                  EventType = "Event_ActionSucceeded"
	Event_SwapInSender_OnSwapInRequested   EventType = "Event_SwapInSender_OnSwapInRequested"
	Event_SwapInSender_OnAgreementReceived EventType = "Event_SwapInSender_OnAgreementReceived"
	Event_ActionFailed                     EventType = "Event_ActionFailed"
	Event_SwapInReceiver_OnRequestReceived EventType = "Event_SwapInReceiver_OnRequestReceived"
	Event_Done                             EventType = "Event_Done"

	Event_OnInvalid_Message EventType = "Event_Invalid_Message"
)
