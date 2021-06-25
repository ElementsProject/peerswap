package fsm

import (
	"encoding/hex"
	"log"
)

const (
	Initial                StateType = "Init"
	SwapOutCreated         StateType = "SwapOutCreated"
	SwapOutRequestSent     StateType = "SwapOutRequestSent"
	FeeInvoiceReceived     StateType = "FeeInvoiceReceived"
	FeeInvoicePaid         StateType = "FeeInvoicePaid"
	SwapOutCanceled        StateType = "SwapOutCanceled"
	SwapOutFeeInvPaid      StateType = "SwapOutFeeInvoicePaid"
	SwapOutTxBroadcasted   StateType = "SwapOutTxBroadcasted"
	SwapOutTxConfirmed     StateType = "SwapOutTxConfirmed"
	SwapOutClaimInvPaid    StateType = "SwapOutClaimInvPaid"
	SwapOutClaimedPreimage StateType = "SwapOutClaimedPreimage"
	SwapOutAborted         StateType = "SwapOutAborted"
	SwapOutClaimedCltv     StateType = "SwapOutClaimedCltv"

	CreateSwapOut          EventType = "CreateSwapOut"
	SendSwapOutRequest     EventType = "SendSwapOutRequest"
	OnFeeInvReceived       EventType = "OnFeeInvoiceReceived"
	OnCancelMsgReceived    EventType = "OnCancelMsgReceived"
	CancelSwapOut          EventType = "CancelSwapOut"
	WaitInvoiceMsg         EventType = "WaitInvoiceMessage"
	OnTxOpenedMessage      EventType = "OnTxOpenededMsg"
	OnTxConfirmations      EventType = "OnTxConfirmations"
	PayClaimInvoice        EventType = "PayClaimInvoice"
	CancelPayment          EventType = "CancelPayment"
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

//todo correct message
func (s *SendSwapOutRequestAction) Execute(services map[string]interface{}, data Data, eventCtx EventContext) EventType {
	swap := data.(*Swap)
	messenger := services["messenger"].(Messenger)
	pubkey := swap.GetPrivkey().PubKey()
	swap.TakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())
	log.Printf("%s", swap.TakerPubkeyHash)
	err := messenger.SendMessage(swap.PeerNodeId, "yadadada")
	if err != nil {
		return CancelSwapOut
	}
	return NoOp
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
	peerId, fee, err := ll.DecodeInvoice(ctx.FeeInvoice)
	if err != nil {
		return CancelPayment
	}
	// todo check peerId
	if !policy.ShouldPayFee(fee, peerId, swap.ChannelId) {
		return CancelPayment
	}
	preimage, err := ll.PayInvoice(ctx.FeeInvoice)
	if err != nil {
		return CancelPayment
	}
	swap.FeePreimage = preimage
	return WaitInvoiceMsg
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
					CreateSwapOut: Initial,
				},
			},
			Initial: {
				Action: &CreateSwapAction{},
				Events: Events{
					CancelPayment:      SwapOutCanceled,
					SendSwapOutRequest: SwapOutCreated,
				},
			},
			SwapOutCreated: {
				Action: &SendSwapOutRequestAction{},
				Events: Events{
					CancelPayment:      SwapOutCanceled,
					SendSwapOutRequest: SwapOutRequestSent,
				},
			},
			SwapOutRequestSent: {
				Action: &NoOpAction{},
				Events: Events{
					CancelPayment:    SwapOutCanceled,
					OnFeeInvReceived: FeeInvoiceReceived,
				},
			},
			FeeInvoiceReceived: {
				Action: &PayFeeAction{},
				Events: Events{
					CancelPayment:  SwapOutCanceled,
					WaitInvoiceMsg: FeeInvoicePaid,
				},
			},
			FeeInvoicePaid: {
				Action: &NoOpAction{},
				Events: Events{
					CancelPayment:     SwapOutCanceled,
					OnTxOpenedMessage: SwapOutTxBroadcasted,
				},
			},
		},
	}
}
