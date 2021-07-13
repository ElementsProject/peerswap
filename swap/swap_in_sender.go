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

	State_SwapCanceled           StateType = "State_SwapCanceled"
	State_SendCancelThenWaitCltv StateType = "State_SendCancelThenWaitCltv"
	State_WaitCltv               StateType = "State_WaitCltv"

	Event_SwapInSender_OnSwapInRequested   EventType = "Event_SwapInSender_OnSwapInRequested"
	Event_SwapInSender_OnSwapInCreated     EventType = "Event_SwapInSender_OnSwapInCreated"
	Event_SwapInSender_OnSwapInRequestSent EventType = "Event_SwapInSender_OnSwapInRequestSent"
	Event_SwapInSender_OnAgreementReceived EventType = "Event_SwapInSender_OnAgreementReceived"
	Event_SwapInSender_OnTxBroadcasted     EventType = "Event_SwapInSender_OnTxBroadcasted"
	Event_SwapInSender_OnTxMsgSent         EventType = "Event_SwapInSender_OnTxMsgSent"

	Event_ActionFailed EventType = "Event_ActionFailed"
)

// SwapInSenderInitAction creates the swap data
type SwapInSenderInitAction struct{}

func (s *SwapInSenderInitAction) Execute(services *SwapServices, swap *SwapData) EventType {
	newSwap := NewSwap(swap.Id, SWAPTYPE_IN, SWAPROLE_SENDER, swap.Amount, swap.InitiatorNodeId, swap.PeerNodeId, swap.ChannelId)
	*swap = *newSwap
	return Event_SwapInSender_OnSwapInCreated
}

// SwapInSenderCreatedAction sends the request to the swap peer
type SwapInSenderCreatedAction struct{}

func (s *SwapInSenderCreatedAction) Execute(services *SwapServices, swap *SwapData) EventType {
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

func (s *SwapInSenderAgreementReceivedAction) Execute(services *SwapServices, swap *SwapData) EventType {
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

	swap.ClaimInvoice = payreq
	swap.ClaimPreimage = preimage.String()
	swap.ClaimPaymentHash = pHash.String()

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
	return Event_SwapInSender_OnTxBroadcasted
}

// SwapInSenderTxBroadcastedAction sends the claim tx information to the swap peer
type SwapInSenderTxBroadcastedAction struct{}

func (s *SwapInSenderTxBroadcastedAction) Execute(services *SwapServices, swap *SwapData) EventType {
	messenger := services.messenger

	msg := &TxOpenedMessage{
		SwapId:          swap.Id,
		MakerPubkeyHash: swap.MakerPubkeyHash,
		Invoice:         swap.ClaimInvoice,
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

// WaitCltvAction adds the opening tx to the txwatcher
type WaitCltvAction struct{}

func (w *WaitCltvAction) Execute(services *SwapServices, swap *SwapData) EventType {
	services.txWatcher.AddCltvTx(swap.Id, swap.Cltv)
	return NoOp
}

// SwapInSenderCltvPassedAction claims the claim tx and sends the claim msg to the swap peer
type SwapInSenderCltvPassedAction struct{}

func (s *SwapInSenderCltvPassedAction) Execute(services *SwapServices, swap *SwapData) EventType {
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
	return Event_OnClaimedCltv
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
				Event_OnCancelReceived:                 State_SwapCanceled,
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
				Event_SwapInSender_OnTxMsgSent: State_SwapInSender_TxMsgSent,
				Event_OnCltvPassed:             State_SwapInSender_CltvPassed,
				Event_ActionFailed:             State_SendCancelThenWaitCltv,
			},
		},
		State_SwapInSender_TxMsgSent: {
			Action: &WaitCltvAction{},
			Events: Events{
				Event_OnClaimInvoicePaid: State_SwapInSender_ClaimInvPaid,
				Event_OnCltvPassed:       State_SwapInSender_CltvPassed,
				Event_OnCancelReceived:   State_WaitCltv,
			},
		},
		State_SwapInSender_ClaimInvPaid: {
			Action: &NoOpAction{},
			Events: Events{
				Event_OnClaimedPreimage: State_ClaimedPreimage,
			},
		},
		State_ClaimedPreimage: {
			Action: &NoOpAction{},
		},
		State_SwapInSender_CltvPassed: {
			Action: &SwapInSenderCltvPassedAction{},
			Events: Events{
				Event_OnClaimedCltv: State_ClaimedCltv,
				Event_ActionFailed:  State_SwapCanceled,
			},
		},
		State_SendCancelThenWaitCltv: {
			Action: &SendCancelAction{},
			Events: Events{
				Event_Action_Success: State_WaitCltv,
			},
		},
		State_WaitCltv: {
			Action: &WaitCltvAction{},
			Events: Events{
				Event_OnCltvPassed: State_SwapInSender_CltvPassed,
			},
		},
		State_ClaimedCltv: {
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
