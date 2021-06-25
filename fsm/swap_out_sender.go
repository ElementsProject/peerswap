package fsm

import (
	"encoding/hex"
	"github.com/sputn1ck/peerswap/lightning"
	"github.com/sputn1ck/peerswap/utils"
)

const (
	State_SwapOutSender_Init               StateType = "State_SwapOutSender_Init"
	State_SwapOutSender_Created            StateType = "State_SwapOutSender_Created"
	State_SwapOutSender_RequestSent        StateType = "State_SwapOutSender_RequestSent"
	State_SwapOutSender_FeeInvoiceReceived StateType = "State_SwapOutSender_FeeInvoiceReceived"
	State_SwapOutSender_FeeInvoicePaid     StateType = "State_SwapOutSender_FeeInvoicePaid"
	State_SwapOutSender_TxBroadcasted      StateType = "State_SwapOutSender_TxBroadcasted"
	State_SwapOutSender_TxConfirmed        StateType = "State_SwapOutSender_TxConfirmed"
	State_SwapOutSender_ClaimInvPaid       StateType = "State_SwapOutSender_ClaimInvPaid"
	State_SwapOutSender_ClaimedPreimage    StateType = "State_SwapOutSender_ClaimedPreimage"
	State_SwapOutSender_ClaimedCltv        StateType = "State_SwapOutSender_ClaimedCltv"

	State_SwapOutSender_SendCancel StateType = "State_SwapOutSender_SendCancel"
	State_SwapOutSender_Canceled   StateType = "State_SwapOutSender_Canceled"
	State_SwapOutSender_Aborted    StateType = "State_SwapOutSender_Aborted"

	Event_SwapOutSender_OnSwapOutCreated     EventType = "Event_SwapOutSender_OnSwapOutCreated"
	Event_SwapOutSender_OnSwapOutRequestSent EventType = "Event_SwapOutSender_OnSwapOutRequestSent"
	Event_SwapOutSender_OnSendSwapOutSucceed EventType = "Event_SwapOutSender_OnSendSwapOutSucceed"
	Event_SwapOutSender_OnFeeInvReceived     EventType = "Event_SwapOutSender_OnFeeInvoiceReceived"
	Event_SwapOutSender_OnCancelMsgReceived  EventType = "Event_SwapOutSender_OnCancelMsgReceived"
	Event_SwapOutSender_OnCancelSwapOut      EventType = "Event_SwapOutSender_OnCancelSwapOut"
	Event_SwapOutSender_OnWaitInvoiceMsg     EventType = "Event_SwapOutSender_WaitInvoiceMessage"
	Event_SwapOutSender_OnTxOpenedMessage    EventType = "Event_SwapOutSender_OnTxOpenededMsg"

	Event_SwapOutSender_OnTxConfirmations EventType = "Event_SwapOutSender_OnTxConfirmations"
	Event_SwapOutSender_FinishSwap        EventType = "Event_SwapOutSender_FinishSwap"
	// todo retrystate? failstate? refundstate?
	RetryState                                 EventType = "RetryState"
	Event_SwapOutSender_OnAbortSwapInternal    EventType = "Event_SwapOutSender_OnAbortSwapInternal"
	Event_SwapOutSender_OnClaimTxPreimage      EventType = "Event_SwapOutSender_OnClaimTxPreimage"
	Event_SwapOutSender_OnCltvClaimMsgReceived EventType = "Event_SwapOutSender_OnCltvClaimMsgReceived"
)

type SwapCreationContext struct {
	amount      uint64
	peer        string
	channelId   string
	initiatorId string
}

type SwapOutInitAction struct{}

//todo validate data
func (a *SwapOutInitAction) Execute(services map[string]interface{}, data Data, eventCtx EventContext) EventType {
	cc := eventCtx.(*SwapCreationContext)
	swap := data.(*Swap)
	newSwap := NewSwap(SWAPTYPE_OUT, SWAPROLE_MAKER, cc.amount, cc.initiatorId, cc.peer, cc.channelId)
	*swap = *newSwap
	return Event_SwapOutSender_OnSwapOutRequestSent
}

type SwapOutCreatedAction struct{}

func (s *SwapOutCreatedAction) Execute(services map[string]interface{}, data Data, eventCtx EventContext) EventType {
	swap := data.(*Swap)
	messenger := services["messenger"].(Messenger)

	pubkey := swap.GetPrivkey().PubKey()
	swap.TakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())

	//todo correct message
	err := messenger.SendMessage(swap.PeerNodeId, "request")
	if err != nil {
		return Event_SwapOutSender_OnCancelSwapOut
	}
	return Event_SwapOutSender_OnSendSwapOutSucceed
}

type FeeRequestContext struct {
	FeeInvoice string
}

type FeeInvoiceReceivedAction struct{}

func (r *FeeInvoiceReceivedAction) Execute(services map[string]interface{}, data Data, eventCtx EventContext) EventType {
	swap := data.(*Swap)
	ctx := eventCtx.(*FeeRequestContext)
	swap.FeeInvoice = ctx.FeeInvoice
	ll := services["lightning"].(LightningClient)
	policy := services["policy"].(Policy)
	invoice, err := ll.DecodeInvoice(ctx.FeeInvoice)
	if err != nil {
		return Event_SwapOutReceiver_OnCancelInternal
	}
	// todo check peerId
	if !policy.ShouldPayFee(invoice.Amount, swap.PeerNodeId, swap.ChannelId) {
		return Event_SwapOutReceiver_OnCancelInternal
	}
	preimage, err := ll.PayInvoice(ctx.FeeInvoice)
	if err != nil {
		return Event_SwapOutReceiver_OnCancelInternal
	}
	swap.FeePreimage = preimage
	return Event_SwapOutSender_OnWaitInvoiceMsg
}

type TxBroadcastedContext struct {
	MakerPubkeyHash string
	ClaimInvoice    string
	TxId            string
	TxHex           string
	Cltv            int64
}
type SwapOutTxBroadCastedAction struct{}

func (t *SwapOutTxBroadCastedAction) Execute(services map[string]interface{}, data Data, eventCtx EventContext) EventType {
	swap := data.(*Swap)
	ctx := eventCtx.(*TxBroadcastedContext)

	swap.MakerPubkeyHash = ctx.MakerPubkeyHash
	swap.Payreq = ctx.ClaimInvoice
	swap.OpeningTxId = ctx.TxId
	swap.Cltv = ctx.Cltv
	swap.OpeningTxHex = ctx.TxHex

	lc := services["lightning"].(LightningClient)
	txWatcher := services["txwatcher"].(TxWatcher)

	invoice, err := lc.DecodeInvoice(swap.Payreq)
	if err != nil {
		return Event_SwapOutSender_OnAbortSwapInternal
	}

	swap.PHash = invoice.PHash

	// todo check policy

	txWatcher.AddTx(swap.Id, ctx.TxId, ctx.TxHex)
	return NoOp
}

type SwapOutTxConfirmedAction struct{}

func (p *SwapOutTxConfirmedAction) Execute(services map[string]interface{}, data Data, eventCtx EventContext) EventType {
	swap := data.(*Swap)

	lc := services["lightning"].(LightningClient)

	preimageString, err := lc.PayInvoice(swap.Payreq)
	if err != nil {
		return Event_SwapOutSender_OnAbortSwapInternal
	}
	swap.PreImage = preimageString
	return Event_SwapOutSender_OnClaimTxPreimage
}

type SwapOutClaimInvPaidAction struct{}

func (c *SwapOutClaimInvPaidAction) Execute(services map[string]interface{}, data Data, eventCtx EventContext) EventType {
	swap := data.(*Swap)

	node := services["node"].(Node)
	messenger := services["messenger"].(Messenger)

	preimage, err := lightning.MakePreimageFromStr(swap.PreImage)
	if err != nil {
		return RetryState
	}
	redeemScript, err := node.GetSwapScript(swap)
	if err != nil {
		return RetryState
	}

	blockheight, err := node.GetBlockHeight()
	if err != nil {
		return RetryState
	}

	address, err := node.GetAddress()
	if err != nil {
		return RetryState
	}
	outputScript, err := utils.Blech32ToScript(address, node.GetNetwork())
	if err != nil {
		return RetryState
	}
	//todo correct fee
	claimTxHex, err := node.CreatePreimageSpendingTransaction(&utils.SpendingParams{
		Signer:       swap.GetPrivkey(),
		OpeningTxHex: swap.OpeningTxHex,
		SwapAmount:   swap.Amount,
		FeeAmount:    node.GetFee(""),
		CurrentBlock: blockheight,
		Asset:        node.GetAsset(),
		OutputScript: outputScript,
		RedeemScript: redeemScript,
	}, preimage[:])
	if err != nil {
		return RetryState
	}

	claimId, err := node.SendRawTx(claimTxHex)
	if err != nil {
		return RetryState
	}
	swap.ClaimTxId = claimId

	//todo correct message
	err = messenger.SendMessage(swap.PeerNodeId, "claimed")
	if err != nil {
		return RetryState
	}
	return Event_SwapOutSender_FinishSwap
}

type SendSwapOutCancelAction struct{}

// todo correct message
func (c *SendSwapOutCancelAction) Execute(services map[string]interface{}, data Data, eventCtx EventContext) EventType {
	swap := data.(*Swap)
	messenger := services["messenger"].(Messenger)
	err := messenger.SendMessage(swap.PeerNodeId, "cancel")
	if err != nil {
		return RetryState
	}
	return Event_SwapOutSender_OnCancelSwapOut
}

type SwapOutAbortedAction struct{}

func (a *SwapOutAbortedAction) Execute(services map[string]interface{}, data Data, eventCtx EventContext) EventType {
	swap := data.(*Swap)

	messenger := services["messenger"].(Messenger)
	//todo correct message
	err := messenger.SendMessage(swap.PeerNodeId, "abort")
	if err != nil {
		return RetryState
	}
	return NoOp
}

type ClaimedContext struct {
	TxId string
}

type SwapOutClaimedCltvAction struct{}

func (s *SwapOutClaimedCltvAction) Execute(services map[string]interface{}, data Data, eventCtx EventContext) EventType {
	swap := data.(*Swap)
	ctx := eventCtx.(*ClaimedContext)
	swap.ClaimTxId = ctx.TxId
	return NoOp
}

type NoOpAction struct{}

func (n *NoOpAction) Execute(services map[string]interface{}, data Data, eventCtx EventContext) EventType {
	return NoOp
}

func newSwapOutSenderFSM(id string, store Store, services map[string]interface{}) *StateMachine {
	return &StateMachine{
		Id:       id,
		store:    store,
		services: services,
		States: States{
			Default: State{
				Events: Events{
					Event_SwapOutSender_OnSwapOutCreated: State_SwapOutSender_Init,
				},
			},
			State_SwapOutSender_Init: {
				Action: &SwapOutInitAction{},
				Events: Events{
					Event_SwapOutSender_OnSwapOutRequestSent: State_SwapOutSender_Created,
				},
			},
			State_SwapOutSender_Created: {
				Action: &SwapOutCreatedAction{},
				Events: Events{
					Event_SwapOutReceiver_OnCancelInternal:   State_SwapOutSender_Canceled,
					Event_SwapOutSender_OnSendSwapOutSucceed: State_SwapOutSender_RequestSent,
				},
			},
			State_SwapOutSender_RequestSent: {
				Action: &NoOpAction{},
				Events: Events{
					Event_SwapOutSender_OnCancelMsgReceived: State_SwapOutSender_SendCancel,
					Event_SwapOutSender_OnFeeInvReceived:    State_SwapOutSender_FeeInvoiceReceived,
				},
			},
			State_SwapOutSender_FeeInvoiceReceived: {
				Action: &FeeInvoiceReceivedAction{},
				Events: Events{
					Event_SwapOutReceiver_OnCancelInternal: State_SwapOutSender_SendCancel,
					Event_SwapOutSender_OnWaitInvoiceMsg:   State_SwapOutSender_FeeInvoicePaid,
				},
			},
			State_SwapOutSender_FeeInvoicePaid: {
				Action: &NoOpAction{},
				Events: Events{
					Event_SwapOutReceiver_OnCancelInternal: State_SwapOutSender_SendCancel,
					Event_SwapOutSender_OnTxOpenedMessage:  State_SwapOutSender_TxBroadcasted,
				},
			},
			State_SwapOutSender_SendCancel: {
				Action: &SendSwapOutCancelAction{},
				Events: Events{
					Event_SwapOutSender_OnCancelSwapOut: State_SwapOutSender_Canceled,
				},
			},
			State_SwapOutSender_Canceled: {
				Action: &NoOpAction{},
			},
			State_SwapOutSender_TxBroadcasted: {
				Action: &SwapOutTxBroadCastedAction{},
				Events: Events{
					Event_SwapOutSender_OnAbortSwapInternal: State_SwapOutSender_Aborted,
					Event_SwapOutSender_OnTxConfirmations:   State_SwapOutSender_TxConfirmed,
				},
			},
			State_SwapOutSender_TxConfirmed: {
				Action: &SwapOutTxConfirmedAction{},
				Events: Events{
					Event_SwapOutSender_OnAbortSwapInternal: State_SwapOutSender_Aborted,
					Event_SwapOutSender_OnClaimTxPreimage:   State_SwapOutSender_ClaimInvPaid,
				},
			},
			State_SwapOutSender_ClaimInvPaid: {
				Action: &SwapOutClaimInvPaidAction{},
				Events: Events{
					Event_SwapOutSender_FinishSwap: State_SwapOutSender_ClaimedPreimage,
					RetryState:                     State_SwapOutSender_ClaimInvPaid,
				},
			},
			State_SwapOutSender_ClaimedPreimage: {
				Action: &NoOpAction{},
			},
			State_SwapOutSender_Aborted: {
				Action: &SwapOutAbortedAction{},
				Events: Events{
					Event_SwapOutSender_OnCltvClaimMsgReceived: State_SwapOutSender_ClaimedCltv,
				},
			},
			State_SwapOutSender_ClaimedCltv: {
				Action: &SwapOutClaimedCltvAction{},
			},
		},
	}
}
