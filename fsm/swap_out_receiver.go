package fsm

import (
	"encoding/hex"
	"github.com/sputn1ck/peerswap/lightning"
	"log"
)

const (
	State_SwapOutReceiver_Init                 StateType = "State_SwapOutReceiver_Init"
	State_SwapOutReceiver_RequestReceived      StateType = "State_SwapOutReceiver_RequestReceived"
	State_SwapOutReceiver_FeeInvoiceSent       StateType = "State_SwapOutReceiver_FeeInvoiceSent"
	State_SwapOutReceiver_FeeInvoicePaid       StateType = "State_SwapOutReceiver_FeeInvoicePaid"
	State_SwapOutReceiver_OpeningTxBroadcasted StateType = "State_SwapOutReceiver_OpeningTxBroadcasted"
	State_SwapOutReceiver_ClaimInvoicePaid     StateType = "State_SwapOutReceiver_ClaimInvoicePaid"
	State_SwapOutReceiver_ClaimedPreimage      StateType = "State_SwapOutReceiver_ClaimedPreimage"
	State_SwapOutReceiver_SwapAborted          StateType = "State_SwapOutReceiver_Aborted"
	State_SwapOutReceiver_CltvPassed           StateType = "State_SwapOutReceiver_CltvPassed"
	State_SwapOutReceiver_ClaimedCltv          StateType = "State_SwapOutReceiver_ClaimedCltv"

	State_SwapOutCanceled   StateType = "State_SwapOutCanceled"
	State_SwapOutSendCancel StateType = "State_SwapOutSendCancel"

	Event_SwapOutReceiver_OnSwapCreated EventType = "Event_SwapOutReceiver_SwapCreated"

	Event_SwapOutReceiver_OnSendFeeInvoiceSuceeded EventType = "Event_SwapOutReceiver_SendFeeInvoiceSuceede"
	Event_SwapOutReceiver_OnFeeInvoicePaid         EventType = "Event_SwapOutReceiver_OnFeeInvoicePaid"
	Event_SwapOutReceiver_OnTxBroadcasted          EventType = "Event_SwapOutReceiver_TxBroadcasted"
	Event_SwapOutReceiver_OnClaimInvoicePaid       EventType = "Event_SwapOutReceiver_OnClaimInvoicePaid"
	Event_SwapOutReceiver_OnClaimMsgReceived       EventType = "Event_SwapOutReceiver_OnClaimMsgReceived"
	Event_SwapOutReceiver_OnAbortMsgReceived       EventType = "Event_SwapOutReceiver_OnAbortMsgReceived"
	Event_SwapOutReceiver_OnCltvPassed             EventType = "Event_SwapOutReceiver_OnCltvPassed"
	Event_SwapOutReceiver_OnCltvClaimed            EventType = "Event_SwapOutReceiver_OnCltvClaimed"

	Event_SwapOutReceiver_OnCancelReceived EventType = "Event_SwapOutReceiver_OnCancelReceived"
	Event_SwapOutReceiver_OnCancelInternal EventType = "Event_SwapOutReceiver_OnCancelInternal"
)

type CreateSwapFromRequestContext struct {
	amount          uint64
	peer            string
	channelId       string
	swapId          string
	takerPubkeyHash string
}
type CreateSwapFromRequestAction struct{}

func (c *CreateSwapFromRequestAction) Execute(services map[string]interface{}, data Data, eventCtx EventContext) EventType {
	request := eventCtx.(*CreateSwapFromRequestContext)
	swap := data.(*Swap)
	newSwap := NewSwapFromRequest(request.peer, request.swapId, request.amount, request.channelId, SWAPTYPE_OUT)
	*swap = *newSwap

	ll := services["lightning"].(LightningClient)
	policy := services["policy"].(Policy)

	node := services["node"].(Node)
	//todo check balances

	pubkey := swap.GetPrivkey().PubKey()

	swap.Role = SWAPROLE_MAKER
	swap.TakerPubkeyHash = request.takerPubkeyHash
	swap.MakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())
	// Generate Preimage
	preimage, err := lightning.GetPreimage()
	if err != nil {
		return Event_SwapOutSender_OnCancelSwapOut
	}
	pHash := preimage.Hash()
	log.Printf("maker preimage: %s ", preimage.String())
	payreq, err := ll.GetPayreq((request.amount)*1000, preimage.String(), "redeem_"+swap.Id)
	if err != nil {
		return Event_SwapOutSender_OnCancelSwapOut
	}

	swap.Payreq = payreq
	swap.PreImage = preimage.String()
	swap.PHash = pHash.String()
	err = node.CreateOpeningTransaction(swap)
	if err != nil {
		return Event_SwapOutSender_OnCancelSwapOut
	}

	fee, err := policy.GetMakerFee(request.amount, swap.OpeningTxFee)
	if err != nil {
		return Event_SwapOutSender_OnCancelSwapOut
	}

	// Generate Preimage
	feepreimage, err := lightning.GetPreimage()
	if err != nil {
		return Event_SwapOutSender_OnCancelSwapOut
	}
	feeInvoice, err := ll.GetPayreq(fee*1000, feepreimage.String(), "fee_"+swap.Id)
	if err != nil {
		return Event_SwapOutSender_OnCancelSwapOut
	}
	swap.FeeInvoice = feeInvoice
	return Event_SwapOutReceiver_OnSwapCreated
}

type SendFeeInvoiceAction struct{}

func (s *SendFeeInvoiceAction) Execute(services map[string]interface{}, data Data, eventCtx EventContext) EventType {
	messenger := services["messenger"].(Messenger)
	swap := data.(*Swap)

	err := messenger.SendMessage(swap.PeerNodeId, "feeinvoice")
	if err != nil {
		return Event_SwapOutSender_OnCancelSwapOut
	}
	return Event_SwapOutReceiver_OnSendFeeInvoiceSuceeded
}

func newSwapOutReceiverFSM(id string, store Store, services map[string]interface{}) *StateMachine {
	return &StateMachine{
		Id:       id,
		store:    store,
		services: services,
		States: States{
			Default: State{
				Events: Events{
					Event_SwapOutSender_OnSwapOutCreated: State_SwapOutReceiver_Init,
				},
			},
			State_SwapOutReceiver_Init: {
				Action: &CreateSwapFromRequestAction{},
				Events: Events{
					Event_SwapOutReceiver_OnSwapCreated:    State_SwapOutSender_Created,
					Event_SwapOutReceiver_OnCancelInternal: State_SwapOutSendCancel,
				},
			},
			State_SwapOutReceiver_RequestReceived: {
				Action: &SendFeeInvoiceAction{},
				Events: Events{
					Event_SwapOutReceiver_OnCancelInternal:         State_SwapOutSendCancel,
					Event_SwapOutReceiver_OnSendFeeInvoiceSuceeded: State_SwapOutReceiver_FeeInvoiceSent,
				},
			},
			State_SwapOutReceiver_FeeInvoiceSent: {
				Action: &NoOpAction{},
				Events: Events{
					Event_SwapOutReceiver_OnFeeInvoicePaid: State_SwapOutReceiver_FeeInvoicePaid,
					Event_SwapOutReceiver_OnCancelReceived: State_SwapOutCanceled,
				},
			},
			State_SwapOutReceiver_FeeInvoicePaid: {
				Action: &NoOpAction{},
				Events: Events{
					Event_SwapOutReceiver_OnTxBroadcasted:  State_SwapOutReceiver_OpeningTxBroadcasted,
					Event_SwapOutReceiver_OnCancelInternal: State_SwapOutSendCancel,
				},
			},
			State_SwapOutReceiver_OpeningTxBroadcasted: {
				Action: &NoOpAction{},
				Events: Events{
					Event_SwapOutReceiver_OnClaimInvoicePaid: State_SwapOutReceiver_ClaimInvoicePaid,
					Event_SwapOutReceiver_OnAbortMsgReceived: State_SwapOutReceiver_SwapAborted,
					Event_SwapOutReceiver_OnCltvPassed:       State_SwapOutReceiver_CltvPassed,
				},
			},
			State_SwapOutReceiver_ClaimInvoicePaid: {
				Action: &NoOpAction{},
				Events: Events{
					Event_SwapOutReceiver_OnClaimMsgReceived: State_SwapOutReceiver_ClaimedPreimage,
				},
			},
			State_SwapOutReceiver_ClaimedPreimage: {
				Action: &NoOpAction{},
			},
			State_SwapOutReceiver_SwapAborted: {
				Action: &NoOpAction{},
				Events: Events{
					Event_SwapOutReceiver_OnCltvPassed: State_SwapOutReceiver_CltvPassed,
				},
			},
			State_SwapOutReceiver_CltvPassed: {
				Action: &NoOpAction{},
				Events: Events{
					Event_SwapOutReceiver_OnCltvClaimed: State_SwapOutReceiver_ClaimedCltv,
				},
			},
			State_SwapOutReceiver_ClaimedCltv: {
				Action: &NoOpAction{},
			},
		},
	}
}
