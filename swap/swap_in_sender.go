package swap

import (
	"github.com/sputn1ck/peerswap/lightning"
)

type CreateAndBroadcastOpeningTransaction struct{}

func (c *CreateAndBroadcastOpeningTransaction) Execute(services *SwapServices, swap *SwapData) EventType {
	txWatcher, wallet, _, err := services.getOnChainServices(swap.GetChain())
	if err != nil {
		return swap.HandleError(err)
	}

	// Generate Preimage
	preimage, err := lightning.GetPreimage()
	if err != nil {
		return swap.HandleError(err)
	}

	payreq, err := services.lightning.GetPayreq((swap.GetAmount())*1000, preimage.String(), "claim_"+swap.Id.String(), swap.GetInvoiceExpiry())
	if err != nil {
		return swap.HandleError(err)
	}

	openingTxRes, err := CreateOpeningTransaction(services, swap.GetChain(), swap.SwapInAgreement.Pubkey, swap.SwapInRequest.Pubkey, preimage.Hash().String(), swap.SwapInRequest.Amount)
	if err != nil {
		return swap.HandleError(err)
	}

	txId, txHex, err := wallet.BroadcastOpeningTx(openingTxRes.UnpreparedHex)
	if err != nil {
		// todo: idempotent states
		return swap.HandleError(err)
	}
	startingHeight, err := txWatcher.GetBlockHeight()
	if err != nil {
		return swap.HandleError(err)
	}
	swap.StartingBlockHeight = startingHeight

	swap.OpeningTxHex = txHex

	message := &OpeningTxBroadcastedMessage{
		SwapId:      swap.Id,
		Payreq:      payreq,
		TxId:        txId,
		ScriptOut:   openingTxRes.Vout,
		BlindingKey: openingTxRes.BlindingKey,
	}

	nextMessage, nextMessageType, err := MarshalPeerswapMessage(message)
	if err != nil {
		return swap.HandleError(err)
	}

	swap.NextMessage = nextMessage
	swap.NextMessageType = nextMessageType

	return Event_ActionSucceeded
}

type StopSendMessageWithRetryWrapperAction struct {
	next Action
}

func (a StopSendMessageWithRetryWrapperAction) Execute(services *SwapServices, swap *SwapData) EventType {
	// Stop sending repeated messages
	services.messengerManager.RemoveSender(swap.Id.String())

	// Call next Action
	return a.next.Execute(services, swap)
}

// AwaitCsvAction adds the opening tx to the txwatcher
type AwaitCsvAction struct{}

//todo this will never throw an error
func (w *AwaitCsvAction) Execute(services *SwapServices, swap *SwapData) EventType {
	onchain, wallet, _, err := services.getOnChainServices(swap.GetChain())
	if err != nil {
		return swap.HandleError(err)
	}

	wantScript, err := wallet.GetOutputScript(swap.GetOpeningParams())
	if err != nil {
		return swap.HandleError(err)
	}

	onchain.AddWaitForCsvTx(swap.Id.String(), swap.OpeningTxBroadcasted.TxId, swap.OpeningTxBroadcasted.ScriptOut, swap.StartingBlockHeight, wantScript)
	return NoOp
}

// swapInSenderFromStore recovers a swap statemachine from the swap store
func swapInSenderFromStore(smData *SwapStateMachine, services *SwapServices) *SwapStateMachine {
	smData.swapServices = services
	smData.States = getSwapInSenderStates()
	return smData
}

// newSwapInSenderFSM returns a new swap statemachine for a swap-in sender
func newSwapInSenderFSM(services *SwapServices) *SwapStateMachine {
	swapId := NewSwapId()
	return &SwapStateMachine{
		Id:           swapId.String(),
		SwapId:       swapId,
		swapServices: services,
		Type:         SWAPTYPE_IN,
		Role:         SWAPROLE_SENDER,
		States:       getSwapInSenderStates(),
		Data:         &SwapData{},
	}
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
			Action: &CreateSwapRequestAction{},
			Events: Events{
				Event_ActionSucceeded: State_SwapInSender_SendRequest,
				Event_ActionFailed:    State_SwapCanceled,
			},
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
			Action: &AwaitCsvAction{},
			Events: Events{
				Event_OnClaimInvoicePaid:  State_ClaimedPreimage,
				Event_OnCsvPassed:         State_SwapInSender_ClaimSwapCsv,
				Event_OnCancelReceived:    State_WaitCsv,
				Event_OnCoopCloseReceived: State_SwapInSender_ClaimSwapCoop,
				Event_OnInvalid_Message:   State_WaitCsv,
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
				Event_OnCsvPassed: State_SwapInSender_ClaimSwapCsv,
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
