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

// Events
const (
	Event_OnSwapOutStarted     EventType = "Event_OnSwapOutStarted"
	Event_OnFeeInvoiceReceived EventType = "Event_OnFeeInvoiceReceived"

	Event_OnTxOpenedMessage EventType = "Event_OnTxOpenedMessage"
	Event_OnTxConfirmed     EventType = "Event_OnTxConfirmed"

	// todo retrystate? failstate? refundstate?
	Event_OnRetry       EventType = "Event_OnRetry"
	Event_OnClaimedCltv EventType = "Event_OnClaimedCltv"
)
