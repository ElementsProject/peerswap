package swap

import (
	"encoding/hex"
	"log"

	"github.com/btcsuite/btcd/btcec"

	"github.com/sputn1ck/peerswap/lightning"
)

// todo every send message should be it's own state / action, if msg sending fails, tx will be broadcasted again / error occurs
// or make the sender a more sophisticated program which tries resending...
const ()

// CreateSwapOutFromRequestAction creates the swap-out process and prepares the opening transaction
type CreateSwapOutFromRequestAction struct{}

func (c *CreateSwapOutFromRequestAction) Execute(services *SwapServices, swap *SwapData) EventType {
	// Generate Preimage
	preimage, err := lightning.GetPreimage()
	if err != nil {
		return swap.HandleError(err)
	}
	pHash := preimage.Hash()
	pubkey := swap.SwapOutRequest.Pubkey
	// todo replace with premium estimation https://github.com/sputn1ck/peerswap/issues/109
	openingTxRes, err := CreateOpeningTransaction(services, swap.GetChain(), pubkey, pubkey, pHash.String(), swap.SwapOutRequest.Amount)
	if err != nil {
		swap.LastErr = err
		return swap.HandleError(err)
	}

	/*
		 feeSat, err := services.policy.GetMakerFee(swap.Amount, swap.OpeningTxFee)
		 if err != nil {

		return Event_ActionFailed
		}
	*/

	// Generate Preimage
	feepreimage, err := lightning.GetPreimage()
	if err != nil {
		return swap.HandleError(err)
	}
	feeInvoice, err := services.lightning.GetPayreq(openingTxRes.Fee*1000, feepreimage.String(), "fee_"+swap.Id.String(), 600)
	if err != nil {
		return swap.HandleError(err)
	}

	message := &SwapOutAgreementMessage{
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
		SwapId:          swap.Id,
		Pubkey:          hex.EncodeToString(swap.GetPrivkey().PubKey().SerializeCompressed()),
		Payreq:          feeInvoice,
	}
	swap.SwapOutAgreement = message

	nextMessage, nextMessageType, err := MarshalPeerswapMessage(message)
	if err != nil {
		return swap.HandleError(err)
	}
	swap.NextMessage = nextMessage
	swap.NextMessageType = nextMessageType

	return Event_ActionSucceeded
}

// ClaimSwapTransactionWithCsv spends the opening transaction with a signature
type ClaimSwapTransactionWithCsv struct{}

func (c *ClaimSwapTransactionWithCsv) Execute(services *SwapServices, swap *SwapData) EventType {
	txId, err := CreateCsvSpendingTransaction(services, swap.GetChain(), swap.GetOpeningParams(), swap.GetClaimParams())
	if err != nil {
		swap.HandleError(err)
		return Event_OnRetry
	}
	swap.ClaimTxId = txId
	return Event_ActionSucceeded
}

// ClaimSwapTransactionWithCsv spends the opening transaction with maker and taker Signatures
type ClaimSwapTransactionCoop struct{}

func (c *ClaimSwapTransactionCoop) Execute(services *SwapServices, swap *SwapData) EventType {
	_, wallet, _, err := services.getOnChainServices(swap.GetChain())
	if err != nil {
		return swap.HandleError(err)
	}

	takerKeyBytes, err := hex.DecodeString(swap.CoopClose.Privkey)
	if err != nil {
		return swap.HandleError(err)
	}
	takerKey, _ := btcec.PrivKeyFromBytes(btcec.S256(), takerKeyBytes)

	txId, _, err := wallet.CreateCoopSpendingTransaction(swap.GetOpeningParams(), swap.GetClaimParams(), takerKey)
	if err != nil {
		return swap.HandleError(err)
	}
	swap.ClaimTxId = txId
	return Event_ActionSucceeded
}

// SendCancelAction sends a cancel message to the swap peer
type SendCancelAction struct{}

func (s *SendCancelAction) Execute(services *SwapServices, swap *SwapData) EventType {
	if swap.LastErr != nil {
		log.Printf("[FSM] Canceling because of %s", swap.LastErr.Error())
	}
	messenger := services.messenger

	msgBytes, msgType, err := MarshalPeerswapMessage(&CancelMessage{
		SwapId:  swap.Id,
		Message: swap.CancelMessage,
	})
	if err != nil {
		return swap.HandleError(err)
	}

	err = messenger.SendMessage(swap.PeerNodeId, msgBytes, msgType)
	if err != nil {
		return swap.HandleError(err)
	}
	return Event_ActionSucceeded
}

// TakerSendPrivkeyAction builds the sighash to send the maker for cooperatively closing the swap
type TakerSendPrivkeyAction struct{}

func (s *TakerSendPrivkeyAction) Execute(services *SwapServices, swap *SwapData) EventType {
	privkeystring := hex.EncodeToString(swap.PrivkeyBytes)
	nextMessage, nextMessageType, err := MarshalPeerswapMessage(&CoopCloseMessage{
		SwapId:  swap.Id,
		Message: swap.CancelMessage,
		Privkey: privkeystring,
	})
	if err != nil {
		return swap.HandleError(err)
	}
	swap.NextMessage = nextMessage
	swap.NextMessageType = nextMessageType

	return Event_ActionSucceeded
}

// swapOutReceiverFromStore recovers a swap statemachine from the swap store
func swapOutReceiverFromStore(smData *SwapStateMachine, services *SwapServices) *SwapStateMachine {
	smData.swapServices = services
	smData.States = getSwapOutReceiverStates()
	return smData
}

// newSwapOutReceiverFSM returns a new swap statemachine for a swap-out receiver
func newSwapOutReceiverFSM(swapId *SwapId, services *SwapServices) *SwapStateMachine {
	return &SwapStateMachine{
		Id:           swapId.String(),
		SwapId:       swapId,
		swapServices: services,
		Type:         SWAPTYPE_OUT,
		Role:         SWAPROLE_RECEIVER,
		States:       getSwapOutReceiverStates(),
		Data:         &SwapData{},
	}
}

// getSwapOutReceiverStates returns the states for the swap-out receiver
func getSwapOutReceiverStates() States {
	return States{
		Default: State{
			Events: Events{
				Event_OnSwapOutRequestReceived: State_SwapOutReceiver_CreateSwap,
			},
		},
		State_SwapOutReceiver_CreateSwap: {
			Action: &CheckRequestWrapperAction{next: &CreateSwapOutFromRequestAction{}},
			Events: Events{
				Event_ActionSucceeded: State_SwapOutReceiver_SendFeeInvoice,
				Event_ActionFailed:    State_SendCancel,
			},
		},
		State_SwapOutReceiver_SendFeeInvoice: {
			Action: &SendMessageAction{},
			Events: Events{
				Event_ActionFailed:    State_SendCancel,
				Event_ActionSucceeded: State_SwapOutReceiver_AwaitFeeInvoicePayment,
			},
		},
		State_SwapOutReceiver_AwaitFeeInvoicePayment: {
			Action: &NoOpAction{},
			Events: Events{
				Event_OnFeeInvoicePaid: State_SwapOutReceiver_BroadcastOpeningTx,
				Event_OnCancelReceived: State_SwapCanceled,
			},
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
				Event_ActionFailed:    State_SwapOutReceiver_SwapAborted,
			},
		},
		State_SwapOutReceiver_AwaitClaimInvoicePayment: {
			Action: &AwaitCsvAction{},
			Events: Events{
				Event_OnClaimInvoicePaid:  State_ClaimedPreimage,
				Event_OnCancelReceived:    State_SwapOutReceiver_ClaimSwapCsv,
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
			Action: &AwaitCsvAction{},
			Events: Events{
				Event_OnCsvPassed: State_SwapOutReceiver_ClaimSwapCsv,
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
			Action: &NoOpDoneAction{},
		},
		State_ClaimedPreimage: {
			Action: &NoOpDoneAction{},
		},
		State_ClaimedCoop: {
			Action: &NoOpDoneAction{},
		},
	}
}
