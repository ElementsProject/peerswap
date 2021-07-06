package swap

import (
	"encoding/hex"
	"github.com/sputn1ck/peerswap/lightning"
)

const (
	State_SwapInSender_Init              StateType = "State_SwapInSender_Init"
	State_SwapInSender_Created           StateType = "State_SwapInSender_Created"
	State_SwapInSender_SwapInRequestSent StateType = "State_SwapInSender_SwapInRequestSent"
	State_SwapInSender_AgreementReceived StateType = "State_SwapInSender_AgreementReceived"
	State_SwapInSender_TxBroadcasted     StateType = "State_SwapInSender_TxBroadcasted"
	State_SwapInSender_TxMsgSent         StateType = "State_SwapInSender_TxMsgSent"
	State_SwapInSender_ClaimInvPaid      StateType = "State_SwapInSender_ClaimInvPaid"
	State_SwapInSender_CltvPassed        StateType = "State_SwapInSender_CltvPassed"
	State_SwapInSender_ClaimedPreimage   StateType = "State_SwapInSender_ClaimedPreimage"
	State_SwapInSender_ClaimedCltv       StateType = "State_SwapInSender_ClaimedCltv"

	State_SwapCanceled           StateType = "State_SwapCanceled"
	State_SendCancelThenWaitCltv StateType = "State_SendCancelThenWaitCltv"
	State_WaitCltv               StateType = "State_WaitCltv"

	Event_SwapInSender_OnSwapInRequested   EventType = "Event_SwapInSender_OnSwapInRequested"
	Event_SwapInSender_OnSwapInCreated     EventType = "Event_SwapInSender_OnSwapInCreated"
	Event_SwapInSender_OnSwapInRequestSent EventType = "Event_SwapInSender_OnSwapInRequestSent"
	Event_SwapInSender_OnAgreementReceived EventType = "Event_SwapInSender_OnAgreementReceived"
	Event_SwapInSender_OnTxBroadcasted     EventType = "Event_SwapInSender_OnTxBroadcasted"
	Event_SwapInSender_OnTxMsgSent         EventType = "Event_SwapInSender_OnTxMsgSent"
	Event_SwapInSender_OnClaimInvPaid      EventType = "Event_SwapInSender_OnClaimInvPaid"
	Event_SwapInSender_OnCltvPassed        EventType = "Event_SwapInSender_OnCltvPassed"
	Event_SwapInSender_OnClaimTxPreimage   EventType = "Event_SwapInSender_OnClaimTxPreimage"
	Event_SwapInSender_OnClaimTxCltv       EventType = "Event_SwapInSender_OnClaimTxCltv"

	Event_ActionFailed EventType = "Event_ActionFailed"
)

// SwapInSenderInitAction creates the swap strcut
type SwapInSenderInitAction struct{}

func (s *SwapInSenderInitAction) Execute(services *SwapServices, swap *Swap) EventType {
	newSwap := NewSwap(swap.Id, SWAPTYPE_IN, SWAPROLE_SENDER, swap.Amount, swap.InitiatorNodeId, swap.PeerNodeId, swap.ChannelId)
	*swap = *newSwap
	return Event_SwapInSender_OnSwapInCreated
}

// SwapInSenderCreatedAction sends the request to the swap peer
type SwapInSenderCreatedAction struct{}

func (s *SwapInSenderCreatedAction) Execute(services *SwapServices, swap *Swap) EventType {
	messenger := services.messenger

	pubkey := swap.GetPrivkey().PubKey()
	swap.TakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())

	msg := &SwapInRequest{
		SwapId:    swap.Id,
		ChannelId: swap.ChannelId,
		Amount:    swap.Amount,
	}
	err := messenger.SendMessage(swap.PeerNodeId, msg)
	if err != nil {
		swap.LastErr = err
		return Event_ActionFailed
	}
	return Event_SwapInSender_OnSwapInRequestSent
}

// SwapInSenderAgreementReceivedAction creates and broadcasts the redeem transaction
type SwapInSenderAgreementReceivedAction struct{}

func (s *SwapInSenderAgreementReceivedAction) Execute(services *SwapServices, swap *Swap) EventType {
	node := services.blockchain
	txwatcher := services.txWatcher

	pubkey := swap.GetPrivkey().PubKey()

	swap.Role = SWAPROLE_SENDER
	swap.MakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())
	// Generate Preimage
	var preimage lightning.Preimage
	preimage, swap.LastErr = lightning.GetPreimage()
	if swap.LastErr != nil {
		return Event_ActionFailed
	}
	pHash := preimage.Hash()
	var payreq string
	payreq, swap.LastErr = services.lightning.GetPayreq((swap.Amount)*1000, preimage.String(), "claim_"+swap.Id)
	if swap.LastErr != nil {
		return Event_ActionFailed
	}

	swap.ClaimPayreq = payreq
	swap.ClaimPreimage = preimage.String()
	swap.ClaimPaymenHash = pHash.String()

	swap.LastErr = CreateOpeningTransaction(services, swap)
	if swap.LastErr != nil {
		return Event_ActionFailed
	}

	var finalizedTx string
	finalizedTx, swap.LastErr = services.wallet.FinalizeTransaction(swap.OpeningTxUnpreparedHex)
	if swap.LastErr != nil {
		return Event_ActionFailed
	}
	swap.OpeningTxHex = finalizedTx

	var txId string
	txId, swap.LastErr = node.SendRawTx(finalizedTx)
	if swap.LastErr != nil {
		return Event_ActionFailed
	}

	swap.OpeningTxId = txId
	txwatcher.AddCltvTx(swap.Id, swap.Cltv)
	txwatcher.AddConfirmationsTx(swap.Id, txId)
	return Event_SwapInSender_OnTxBroadcasted
}

// SwapInSenderTxBroadcastedAction sends the claim tx information to the swap peer
type SwapInSenderTxBroadcastedAction struct{}

func (s *SwapInSenderTxBroadcastedAction) Execute(services *SwapServices, swap *Swap) EventType {
	messenger := services.messenger

	msg := &TxOpenedResponse{
		SwapId:          swap.Id,
		MakerPubkeyHash: swap.MakerPubkeyHash,
		Invoice:         swap.ClaimPayreq,
		TxId:            swap.OpeningTxId,
		TxHex:           swap.OpeningTxHex,
		Cltv:            swap.Cltv,
	}
	swap.LastErr = messenger.SendMessage(swap.PeerNodeId, msg)
	if swap.LastErr != nil {
		return Event_ActionFailed
	}
	return Event_SwapInSender_OnTxMsgSent
}

// SwapInSenderCltvPassedAction claims the claim tx and sends the claim msg to the swap peer
type SwapInSenderCltvPassedAction struct{}

func (s *SwapInSenderCltvPassedAction) Execute(services *SwapServices, swap *Swap) EventType {
	var claimId, claimTxHex string
	blockchain := services.blockchain
	messenger := services.messenger

	claimTxHex, swap.LastErr = CreateCltvSpendingTransaction(services, swap)
	if swap.LastErr != nil {
		return Event_ActionFailed
	}

	claimId, swap.LastErr = blockchain.SendRawTx(claimTxHex)
	if swap.LastErr != nil {
		return Event_ActionFailed
	}
	swap.ClaimTxId = claimId
	msg := &ClaimedMessage{
		SwapId:    swap.Id,
		ClaimType: CLAIMTYPE_CLTV,
		ClaimTxId: claimId,
	}
	swap.LastErr = messenger.SendMessage(swap.PeerNodeId, msg)
	if swap.LastErr != nil {
		return Event_ActionFailed
	}
	return Event_SwapInSender_OnClaimTxCltv
}

func getSwapInSenderStates() States {
	return States{
		Default: State{
			Events: Events{
				Event_SwapInSender_OnSwapInRequested: State_SwapInSender_Init,
			},
		},
		State_SwapInSender_Init: {
			Action: &SwapInSenderInitAction{},
			Events: Events{
				Event_SwapInSender_OnSwapInCreated: State_SwapInSender_Created,
				Event_ActionFailed:                 State_SwapCanceled,
			},
		},
		State_SwapInSender_Created: {
			Action: &SwapInSenderCreatedAction{},
			Events: Events{
				Event_SwapInSender_OnSwapInRequestSent: State_SwapInSender_SwapInRequestSent,
				Event_ActionFailed:                     State_SwapCanceled,
			},
		},
		State_SwapInSender_SwapInRequestSent: {
			Action: &NoOpAction{},
			Events: Events{
				Event_SwapInSender_OnAgreementReceived: State_SwapInSender_AgreementReceived,
				Event_OnCancelReceived:                 State_SwapOut_Canceled,
			},
		},
		State_SwapInSender_AgreementReceived: {
			Action: &SwapInSenderAgreementReceivedAction{},
			Events: Events{
				Event_SwapInSender_OnTxBroadcasted: State_SwapInSender_TxBroadcasted,
				Event_ActionFailed:                 State_SendCancel,
			},
		},
		State_SwapInSender_TxBroadcasted: {
			Action: &SwapInSenderTxBroadcastedAction{},
			Events: Events{
				Event_SwapInSender_OnTxMsgSent:  State_SwapInSender_TxMsgSent,
				Event_SwapInSender_OnCltvPassed: State_SwapInSender_CltvPassed,
				Event_ActionFailed:              State_SendCancelThenWaitCltv,
			},
		},
		State_SwapInSender_TxMsgSent: {
			Action: &NoOpAction{},
			Events: Events{
				Event_SwapInSender_OnClaimInvPaid: State_SwapInSender_ClaimInvPaid,
				Event_SwapInSender_OnCltvPassed:   State_SwapInSender_CltvPassed,
				Event_OnCancelReceived:            State_WaitCltv,
			},
		},
		State_SwapInSender_ClaimInvPaid: {
			Action: &NoOpAction{},
			Events: Events{
				Event_SwapInSender_OnClaimTxPreimage: State_SwapInSender_ClaimedPreimage,
			},
		},
		State_SwapInSender_ClaimedPreimage: {
			Action: &NoOpAction{},
		},
		State_SwapInSender_CltvPassed: {
			Action: &SwapInSenderCltvPassedAction{},
			Events: Events{
				Event_SwapInSender_OnClaimTxCltv: State_SwapInSender_ClaimedCltv,
				Event_ActionFailed:               State_SwapInSender_CltvPassed,
			},
		},
		State_SendCancelThenWaitCltv: {
			Action: &SendCancelAction{},
			Events: Events{
				Event_Action_Success: State_WaitCltv,
			},
		},
		State_WaitCltv: {
			Action: &NoOpAction{},
			Events: Events{
				Event_SwapInSender_OnCltvPassed: State_SwapInSender_CltvPassed,
			},
		},
		State_SwapInSender_ClaimedCltv: {
			Action: &NoOpAction{},
		},

		State_SendCancel: {
			Action: &SendCancelAction{},
			Events: Events{
				Event_Action_Success: State_SwapCanceled,
			},
		},
		State_SwapCanceled: {
			Action: &NoOpAction{},
		},
	}
}
