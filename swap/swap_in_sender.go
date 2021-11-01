package swap

import (
	"encoding/hex"

	"github.com/sputn1ck/peerswap/lightning"
)

// SwapInSenderCreateSwapAction creates the swap data
type SwapInSenderCreateSwapAction struct{}

func (s *SwapInSenderCreateSwapAction) Execute(services *SwapServices, swap *SwapData) EventType {
	newSwap := NewSwap(swap.Id, swap.Asset, SWAPTYPE_IN, SWAPROLE_SENDER, swap.Amount, swap.InitiatorNodeId, swap.PeerNodeId, swap.ChannelId, swap.ProtocolVersion)
	*swap = *newSwap

	pubkey := swap.GetPrivkey().PubKey()

	swap.Role = SWAPROLE_SENDER
	swap.MakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())

	nextMessage, nextMessageType, err := MarshalPeerswapMessage(&SwapInRequest{
		SwapId:          swap.Id,
		ChannelId:       swap.ChannelId,
		Amount:          swap.Amount,
		Asset:           swap.Asset,
		ProtocolVersion: swap.ProtocolVersion,
	})
	if err != nil {
		return swap.HandleError(err)
	}
	swap.NextMessage = nextMessage
	swap.NextMessageType = nextMessageType

	return Event_ActionSucceeded
}

type CreateAndBroadcastOpeningTransaction struct{}

func (c *CreateAndBroadcastOpeningTransaction) Execute(services *SwapServices, swap *SwapData) EventType {
	onchain, err := services.getOnchainAsset(swap.Asset)
	if err != nil {
		return swap.HandleError(err)
	}
	pubkey := swap.GetPrivkey().PubKey()

	swap.Role = SWAPROLE_SENDER
	swap.MakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())
	// Generate Preimage
	preimage, err := lightning.GetPreimage()
	if err != nil {
		return swap.HandleError(err)
	}
	pHash := preimage.Hash()
	payreq, err := services.lightning.GetPayreq((swap.Amount)*1000, preimage.String(), "claim_"+swap.Id)
	if err != nil {
		return swap.HandleError(err)
	}

	swap.ClaimInvoice = payreq
	swap.ClaimPreimage = preimage.String()
	swap.ClaimPaymentHash = pHash.String()

	err = SetRefundAddress(services, swap)
	if err != nil {
		return swap.HandleError(err)
	}

	err = CreateOpeningTransaction(services, swap)
	if err != nil {
		return swap.HandleError(err)
	}

	txId, txHex, err := onchain.BroadcastOpeningTx(swap.OpeningTxUnpreparedHex)
	if err != nil {
		return swap.HandleError(err)
	}
	swap.OpeningTxHex = txHex
	swap.OpeningTxId = txId

	refundFee, err := onchain.GetRefundFee()
	if err != nil {
		return swap.HandleError(err)
	}
	swap.RefundFee = refundFee
	nextMessage, nextMessageType, err := MarshalPeerswapMessage(&TxOpenedMessage{
		SwapId:          swap.Id,
		MakerPubkeyHash: swap.MakerPubkeyHash,
		Invoice:         swap.ClaimInvoice,
		TxId:            swap.OpeningTxId,
		RefundAddr:      swap.MakerRefundAddr,
		RefundFee:       swap.RefundFee,
	})
	if err != nil {
		return swap.HandleError(err)
	}
	swap.NextMessage = nextMessage
	swap.NextMessageType = nextMessageType

	return Event_ActionSucceeded
}

// AwaitCsvAction adds the opening tx to the txwatcher
type AwaitCsvAction struct{}

//todo this will never throw an error
func (w *AwaitCsvAction) Execute(services *SwapServices, swap *SwapData) EventType {
	onchain, err := services.getOnchainAsset(swap.Asset)
	if err != nil {
		return swap.HandleError(err)
	}
	err = onchain.AddWaitForCsvTx(swap.Id, swap.OpeningTxId, swap.OpeningTxVout)
	if err != nil {
		return swap.HandleError(err)
	}
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
	return &SwapStateMachine{
		Id:           newSwapId(),
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
			Action: &SwapInSenderCreateSwapAction{},
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
				Event_SwapInSender_OnAgreementReceived: State_SwapInSender_BroadcastOpeningTx,
				Event_OnCancelReceived:                 State_SwapCanceled,
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
			Action: &SendMessageAction{},
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
			},
		},
		State_SwapInSender_ClaimSwapCsv: {
			Action: &ClaimSwapTransactionWithCsv{},
			Events: Events{
				Event_ActionSucceeded: State_ClaimedCsv,
				Event_OnRetry:         State_SwapInSender_ClaimSwapCsv,
			},
		},
		State_SwapInSender_ClaimSwapCoop: {
			Action: &ClaimSwapTransactionCoop{},
			Events: Events{
				Event_ActionSucceeded: State_ClaimedCoop,
				Event_ActionFailed:    State_WaitCsv,
			},
		},
		State_WaitCsv: {
			Action: &AwaitCsvAction{},
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
