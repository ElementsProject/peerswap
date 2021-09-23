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

	nextMessage, nextMessageType, err := MarshalPeerswapMessage(&TxOpenedMessage{
		SwapId:          swap.Id,
		MakerPubkeyHash: swap.MakerPubkeyHash,
		Invoice:         swap.ClaimInvoice,
		TxId:            swap.OpeningTxId,
		Cltv:            swap.Cltv,
		RefundAddr:      swap.MakerRefundAddr,
	})
	if err != nil {
		return swap.HandleError(err)
	}
	swap.NextMessage = nextMessage
	swap.NextMessageType = nextMessageType

	return Event_ActionSucceeded
}

// AwaitCltvAction adds the opening tx to the txwatcher
type AwaitCltvAction struct{}

//todo this will never throw an error
func (w *AwaitCltvAction) Execute(services *SwapServices, swap *SwapData) EventType {
	onchain, err := services.getOnchainAsset(swap.Asset)
	if err != nil {
		return swap.HandleError(err)
	}
	err = onchain.AddWaitForCltvTx(swap.Id, swap.OpeningTxId, uint64(swap.Cltv))
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
				Event_ActionFailed:    State_WaitCltv,
			},
		},
		State_SwapInSender_AwaitClaimPayment: {
			Action: &AwaitCltvAction{},
			Events: Events{
				Event_OnClaimInvoicePaid: State_ClaimedPreimage,
				Event_OnCltvPassed:       State_SwapInSender_ClaimSwapCltv,
				Event_OnCancelReceived:   State_SwapInSender_ClaimSwapCoop,
			},
		},
		State_SwapInSender_ClaimSwapCltv: {
			Action: &ClaimSwapTransactionWithCltv{},
			Events: Events{
				Event_ActionSucceeded: State_ClaimedCltv,
				Event_OnRetry:         State_SwapInSender_ClaimSwapCltv,
			},
		},
		State_SwapInSender_ClaimSwapCoop: {
			Action: &ClaimSwapTransactionCoop{},
			Events: Events{
				Event_ActionFailed:    State_WaitCltv,
				Event_ActionSucceeded: State_ClaimedCoop,
			},
		},
		State_WaitCltv: {
			Action: &AwaitCltvAction{},
			Events: Events{
				Event_OnCltvPassed: State_SwapInSender_ClaimSwapCltv,
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
		State_ClaimedCltv: {
			Action: &NoOpDoneAction{},
		},
		State_ClaimedCoop: {
			Action: &NoOpDoneAction{},
		},
	}
}
