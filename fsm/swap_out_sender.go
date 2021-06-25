package fsm

import (
	"encoding/hex"
	"github.com/sputn1ck/peerswap/lightning"
	"github.com/sputn1ck/peerswap/utils"
)

const (
	SwapOutInit            StateType = "Init"
	SwapOutCreated         StateType = "SwapOutCreated"
	SwapOutRequestSent     StateType = "SwapOutRequestSent"
	FeeInvoiceReceived     StateType = "FeeInvoiceReceived"
	FeeInvoicePaid         StateType = "FeeInvoicePaid"
	SwapOutTxBroadcasted   StateType = "SwapOutTxBroadcasted"
	SwapOutTxConfirmed     StateType = "SwapOutTxConfirmed"
	SwapOutClaimInvPaid    StateType = "SwapOutClaimInvPaid"
	SwapOutClaimedPreimage StateType = "SwapOutClaimedPreimage"
	SwapOutClaimedCltv     StateType = "SwapOutClaimedCltv"

	SwapOutCancelInternal StateType = "SwapOutCancelInternal"
	SwapOutCanceled       StateType = "SwapOutCanceled"
	SwapOutAborted        StateType = "SwapOutAborted"

	CreateSwapOut       EventType = "CreateSwapOut"
	SendSwapOutRequest  EventType = "SendSwapOutRequest"
	SendSwapOutSucceed  EventType = "SendSwapOutSucceed"
	OnFeeInvReceived    EventType = "OnFeeInvoiceReceived"
	OnCancelMsgReceived EventType = "OnCancelMsgReceived"
	CancelSwapOut       EventType = "CancelSwapOut"
	WaitInvoiceMsg      EventType = "WaitInvoiceMessage"
	OnTxOpenedMessage   EventType = "OnTxOpenededMsg"

	OnTxConfirmations EventType = "OnTxConfirmations"
	CancelPayment     EventType = "CancelPayment"
	FinishSwap        EventType = "FinishSwap"
	// todo retrystate? failstate? refundstate?
	RetryState             EventType = "RetryState"
	AbortSwap              EventType = "AbortSwap"
	ClaimTxPreimage        EventType = "ClaimTxPreimage"
	OnCltvClaimMsgReceived EventType = "OnCltvClaimMsgReceived"
)

type SwapCreationContext struct {
	amount      uint64
	peer        string
	channelId   string
	initiatorId string
}

type CreateSwapAction struct{}

//todo validate data
func (a *CreateSwapAction) Execute(services map[string]interface{}, data Data, eventCtx EventContext) EventType {
	cc := eventCtx.(*SwapCreationContext)
	swap := data.(*Swap)
	newSwap := NewSwap(SWAPTYPE_OUT, SWAPROLE_MAKER, cc.amount, cc.initiatorId, cc.peer, cc.channelId)
	*swap = *newSwap
	return SendSwapOutRequest
}

type SendSwapOutRequestAction struct{}

func (s *SendSwapOutRequestAction) Execute(services map[string]interface{}, data Data, eventCtx EventContext) EventType {
	swap := data.(*Swap)
	messenger := services["messenger"].(Messenger)

	pubkey := swap.GetPrivkey().PubKey()
	swap.TakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())

	//todo correct message
	err := messenger.SendMessage(swap.PeerNodeId, "request")
	if err != nil {
		return CancelSwapOut
	}
	return SendSwapOutSucceed
}

type FeeRequestContext struct {
	FeeInvoice string
}

type PayFeeAction struct{}

func (r *PayFeeAction) Execute(services map[string]interface{}, data Data, eventCtx EventContext) EventType {
	swap := data.(*Swap)
	ctx := eventCtx.(*FeeRequestContext)
	swap.FeeInvoice = ctx.FeeInvoice
	ll := services["lightning"].(LightningClient)
	policy := services["policy"].(Policy)
	invoice, err := ll.DecodeInvoice(ctx.FeeInvoice)
	if err != nil {
		return CancelPayment
	}
	// todo check peerId
	if !policy.ShouldPayFee(invoice.Amount, swap.PeerNodeId, swap.ChannelId) {
		return CancelPayment
	}
	preimage, err := ll.PayInvoice(ctx.FeeInvoice)
	if err != nil {
		return CancelPayment
	}
	swap.FeePreimage = preimage
	return WaitInvoiceMsg
}

type TxBroadcastedContext struct {
	MakerPubkeyHash string
	ClaimInvoice    string
	TxId            string
	TxHex           string
	Cltv            int64
}
type TxBroadcastedAction struct{}

func (t *TxBroadcastedAction) Execute(services map[string]interface{}, data Data, eventCtx EventContext) EventType {
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
		return AbortSwap
	}

	swap.PHash = invoice.PHash

	// todo check policy

	txWatcher.AddTx(swap.Id, ctx.TxId, ctx.TxHex)
	return NoOp
}

type PayClaimInvoiceAction struct{}

func (p *PayClaimInvoiceAction) Execute(services map[string]interface{}, data Data, eventCtx EventContext) EventType {
	swap := data.(*Swap)

	lc := services["lightning"].(LightningClient)

	preimageString, err := lc.PayInvoice(swap.Payreq)
	if err != nil {
		return AbortSwap
	}
	swap.PreImage = preimageString
	return ClaimTxPreimage
}

type ClaimTxPreimageAction struct{}

func (c *ClaimTxPreimageAction) Execute(services map[string]interface{}, data Data, eventCtx EventContext) EventType {
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
	return FinishSwap
}

type SendCancelSwapAction struct{}

// todo correct message
func (c *SendCancelSwapAction) Execute(services map[string]interface{}, data Data, eventCtx EventContext) EventType {
	swap := data.(*Swap)
	messenger := services["messenger"].(Messenger)
	err := messenger.SendMessage(swap.PeerNodeId, "cancel")
	if err != nil {
		return RetryState
	}
	return CancelSwapOut
}

type AbortSwapAction struct{}

func (a *AbortSwapAction) Execute(services map[string]interface{}, data Data, eventCtx EventContext) EventType {
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
					CreateSwapOut: SwapOutInit,
				},
			},
			SwapOutInit: {
				Action: &CreateSwapAction{},
				Events: Events{
					SendSwapOutRequest: SwapOutCreated,
				},
			},
			SwapOutCreated: {
				Action: &SendSwapOutRequestAction{},
				Events: Events{
					CancelPayment:      SwapOutCanceled,
					SendSwapOutSucceed: SwapOutRequestSent,
				},
			},
			SwapOutRequestSent: {
				Action: &NoOpAction{},
				Events: Events{
					OnCancelMsgReceived: SwapOutCancelInternal,
					OnFeeInvReceived:    FeeInvoiceReceived,
				},
			},
			FeeInvoiceReceived: {
				Action: &PayFeeAction{},
				Events: Events{
					CancelPayment:  SwapOutCancelInternal,
					WaitInvoiceMsg: FeeInvoicePaid,
				},
			},
			FeeInvoicePaid: {
				Action: &NoOpAction{},
				Events: Events{
					CancelPayment:     SwapOutCancelInternal,
					OnTxOpenedMessage: SwapOutTxBroadcasted,
				},
			},
			SwapOutCancelInternal: {
				Action: &SendCancelSwapAction{},
				Events: Events{
					CancelSwapOut: SwapOutCanceled,
				},
			},
			SwapOutCanceled: {
				Action: &NoOpAction{},
			},
			SwapOutTxBroadcasted: {
				Action: &TxBroadcastedAction{},
				Events: Events{
					AbortSwap:         SwapOutAborted,
					OnTxConfirmations: SwapOutTxConfirmed,
				},
			},
			SwapOutTxConfirmed: {
				Action: &PayClaimInvoiceAction{},
				Events: Events{
					AbortSwap:       SwapOutAborted,
					ClaimTxPreimage: SwapOutClaimInvPaid,
				},
			},
			SwapOutClaimInvPaid: {
				Action: &ClaimTxPreimageAction{},
				Events: Events{
					FinishSwap: SwapOutClaimedPreimage,
					RetryState: SwapOutClaimInvPaid,
				},
			},
			SwapOutClaimedPreimage: {
				Action: &NoOpAction{},
			},
			SwapOutAborted: {
				Action: &AbortSwapAction{},
				Events: Events{
					OnCltvClaimMsgReceived: SwapOutClaimedCltv,
				},
			},
			SwapOutClaimedCltv: {
				Action: &SwapOutClaimedCltvAction{},
			},
		},
	}
}
