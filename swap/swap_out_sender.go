package swap

import (
	"encoding/hex"
	"log"
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
	State_SwapOut_Canceled         StateType = "State_SwapOut_Canceled"

	Event_SwapOutSender_OnSwapOutCreated     EventType = "Event_SwapOutSender_OnSwapOutCreated"
	Event_SwapOutSender_OnSwapCreated        EventType = "Event_SwapOutSender_OnSwapCreated"
	Event_SwapOutSender_OnSendSwapOutSucceed EventType = "Event_SwapOutSender_OnSendSwapOutSucceed"
	Event_SwapOutSender_OnFeeInvReceived     EventType = "Event_SwapOutSender_OnFeeInvoiceReceived"
	Event_SwapOutSender_OnCancelSwapOut      EventType = "Event_SwapOutSender_OnCancelSwapOut"
	Event_SwapOutSender_OnFeeInvoicePaid     EventType = "Event_SwapOutSender_WaitInvoiceMessage"
	Event_SwapOutSender_OnTxOpenedMessage    EventType = "Event_SwapOutSender_OnTxOpenededMsg"

	Event_SwapOutSender_OnTxConfirmations EventType = "Event_SwapOutSender_OnTxConfirmations"
	Event_SwapOutSender_FinishSwap        EventType = "Event_SwapOutSender_FinishSwap"
	// todo retrystate? failstate? refundstate?
	Event_OnRetry                              EventType = "Event_OnRetry"
	Event_OnRecover                            EventType = "Event_OnRecover"
	Event_SwapOutSender_OnAbortSwapInternal    EventType = "Event_SwapOutSender_OnAbortSwapInternal"
	Event_SwapOutSender_OnClaimTxPreimage      EventType = "Event_SwapOutSender_OnClaimTxPreimage"
	Event_SwapOutSender_OnCltvClaimMsgReceived EventType = "Event_SwapOutSender_OnCltvClaimMsgReceived"
)

type SwapCreationContext struct {
	swapId      string
	amount      uint64
	peer        string
	channelId   string
	initiatorId string
}

func (c *SwapCreationContext) ApplyOnSwap(swap *Swap) {
	swap.Amount = c.amount
	swap.PeerNodeId = c.peer
	swap.ChannelId = c.channelId
	swap.Id = c.swapId
	swap.InitiatorNodeId = c.initiatorId
}

type SwapOutInitAction struct{}

//todo validate data
func (a *SwapOutInitAction) Execute(services *SwapServices, swap *Swap) EventType {
	newSwap := NewSwap(swap.Id, SWAPTYPE_OUT, SWAPROLE_SENDER, swap.Amount, swap.InitiatorNodeId, swap.PeerNodeId, swap.ChannelId)
	*swap = *newSwap
	return Event_SwapOutSender_OnSwapCreated
}

type SwapOutCreatedAction struct{}

func (s *SwapOutCreatedAction) Execute(services *SwapServices, swap *Swap) EventType {
	messenger := services.messenger

	pubkey := swap.GetPrivkey().PubKey()
	swap.TakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())

	//todo correct message
	msg := &SwapOutRequest{
		SwapId:          swap.Id,
		ChannelId:       swap.ChannelId,
		Amount:          swap.Amount,
		TakerPubkeyHash: swap.TakerPubkeyHash,
	}
	err := messenger.SendMessage(swap.PeerNodeId, msg)
	if err != nil {
		swap.LastErr = err
		return Event_SwapOutSender_OnCancelSwapOut
	}
	return Event_SwapOutSender_OnSendSwapOutSucceed
}

type FeeRequestContext struct {
	FeeInvoice string
}

func (f *FeeRequestContext) ApplyOnSwap(swap *Swap) {
	swap.FeeInvoice = f.FeeInvoice
}

type FeeInvoiceReceivedAction struct{}

func (r *FeeInvoiceReceivedAction) Execute(services *SwapServices, swap *Swap) EventType {
	ll := services.lightning
	policy := services.policy
	invoice, err := ll.DecodePayreq(swap.FeeInvoice)
	if err != nil {
		swap.LastErr = err
		log.Printf("error decoding %v", err)
		return Event_SwapOutReceiver_OnCancelInternal
	}
	// todo check peerId
	if !policy.ShouldPayFee(swap.Amount, invoice.Amount, swap.PeerNodeId, swap.ChannelId) {
		swap.LastErr = err

		log.Printf("won't pay fee %v", err)
		return Event_SwapOutReceiver_OnCancelInternal
	}
	preimage, err := ll.PayInvoice(swap.FeeInvoice)
	if err != nil {
		swap.LastErr = err
		log.Printf("error paying out %v", err)
		return Event_SwapOutReceiver_OnCancelInternal
	}
	swap.FeePreimage = preimage
	return Event_SwapOutSender_OnFeeInvoicePaid
}

type TxBroadcastedContext struct {
	MakerPubkeyHash string
	ClaimInvoice    string
	TxId            string
	TxHex           string
	Cltv            int64
}

func (t *TxBroadcastedContext) ApplyOnSwap(swap *Swap) {
	swap.MakerPubkeyHash = t.MakerPubkeyHash
	swap.ClaimPayreq = t.ClaimInvoice
	swap.OpeningTxId = t.TxId
	swap.OpeningTxHex = t.TxHex
	swap.Cltv = t.Cltv
}

type SwapOutTxBroadCastedAction struct{}

func (t *SwapOutTxBroadCastedAction) Execute(services *SwapServices, swap *Swap) EventType {
	lc := services.lightning
	txWatcher := services.txWatcher

	invoice, err := lc.DecodePayreq(swap.ClaimPayreq)
	if err != nil {
		swap.LastErr = err
		return Event_SwapOutSender_OnAbortSwapInternal
	}

	swap.ClaimPaymenHash = invoice.PHash

	// todo check policy

	txWatcher.AddConfirmationsTx(swap.Id, swap.OpeningTxId)
	return NoOp
}

type SwapOutTxConfirmedAction struct{}

func (p *SwapOutTxConfirmedAction) Execute(services *SwapServices, swap *Swap) EventType {

	lc := services.lightning

	preimageString, err := lc.PayInvoice(swap.ClaimPayreq)
	if err != nil {
		swap.LastErr = err
		return Event_SwapOutSender_OnAbortSwapInternal
	}
	swap.ClaimPreimage = preimageString
	return Event_SwapOutSender_OnClaimTxPreimage
}

type SwapOutClaimInvPaidAction struct{}

func (c *SwapOutClaimInvPaidAction) Execute(services *SwapServices, swap *Swap) EventType {

	node := services.blockchain
	messenger := services.messenger

	claimTxHex, err := CreatePreimageSpendingTransaction(services, swap)
	if err != nil {
		log.Printf("error creating spending tx %v", err)
		swap.LastErr = err
		return Event_OnRetry
	}

	claimId, err := node.SendRawTx(claimTxHex)
	if err != nil {
		swap.LastErr = err
		log.Printf("error sendiong raw tx %v", err)
		return Event_OnRetry
	}
	swap.ClaimTxId = claimId

	//todo correct message
	msg := &ClaimedMessage{
		SwapId:    swap.Id,
		ClaimType: CLAIMTYPE_PREIMAGE,
		ClaimTxId: claimId,
	}
	err = messenger.SendMessage(swap.PeerNodeId, msg)
	if err != nil {
		swap.LastErr = err
		log.Printf("error sending message tx %v", err)
		return Event_OnRetry
	}
	return Event_SwapOutSender_FinishSwap
}

type SendSwapOutCancelAction struct{}

// todo correct message
func (c *SendSwapOutCancelAction) Execute(services *SwapServices, swap *Swap) EventType {

	log.Printf("[FSM] Canceling because of %v", swap.LastErr)
	messenger := services.messenger
	msg := &CancelResponse{
		SwapId: swap.Id,
		Error:  swap.CancelMessage,
	}
	err := messenger.SendMessage(swap.PeerNodeId, msg)
	if err != nil {
		swap.LastErr = err
		return Event_OnRetry
	}
	return Event_SwapOutSender_OnCancelSwapOut
}

type SwapOutAbortedAction struct{}

func (a *SwapOutAbortedAction) Execute(services *SwapServices, swap *Swap) EventType {

	log.Printf("[FSM] Aborting because of %v", swap.LastErr)
	messenger := services.messenger
	//todo correct message
	msg := &CancelResponse{
		SwapId: swap.Id,
		Error:  swap.CancelMessage,
	}
	err := messenger.SendMessage(swap.PeerNodeId, msg)
	if err != nil {
		swap.LastErr = err
		return Event_OnRetry
	}
	return NoOp
}

type ClaimedContext struct {
	TxId string
}

func (c *ClaimedContext) ApplyOnSwap(swap *Swap) {
	swap.ClaimTxId = c.TxId
}

type SwapOutClaimedCltvAction struct{}

func (s *SwapOutClaimedCltvAction) Execute(services *SwapServices, swap *Swap) EventType {

	return NoOp
}

type NoOpAction struct{}

func (n *NoOpAction) Execute(services *SwapServices, swap *Swap) EventType {
	return NoOp
}

func swapOutSenderFromStore(smData *StateMachine, services *SwapServices) *StateMachine {
	smData.swapServices = services
	smData.States = getSwapOutSenderStates()
	return smData
}

func newSwapOutSenderFSM(services *SwapServices) *StateMachine {
	return &StateMachine{
		Id:           newSwapId(),
		swapServices: services,
		Type:         SWAPTYPE_OUT,
		Role:         SWAPROLE_SENDER,
		States:       getSwapOutSenderStates(),
		Data:         &Swap{},
	}
}

func getSwapOutSenderStates() States {
	return States{
		Default: State{
			Events: Events{
				Event_SwapOutSender_OnSwapOutCreated: State_SwapOutSender_Init,
			},
		},
		State_SwapOutSender_Init: {
			Action: &SwapOutInitAction{},
			Events: Events{
				Event_SwapOutSender_OnSwapCreated: State_SwapOutSender_Created,
			},
		},
		State_SwapOutSender_Created: {
			Action: &SwapOutCreatedAction{},
			Events: Events{
				Event_SwapOutReceiver_OnCancelInternal:   State_SwapOut_Canceled,
				Event_SwapOutSender_OnSendSwapOutSucceed: State_SwapOutSender_RequestSent,
			},
		},
		State_SwapOutSender_RequestSent: {
			Action: &NoOpAction{},
			Events: Events{
				Event_OnCancelReceived:               State_SwapOut_Canceled,
				Event_SwapOutSender_OnFeeInvReceived: State_SwapOutSender_FeeInvoiceReceived,
			},
		},
		State_SwapOutSender_FeeInvoiceReceived: {
			Action: &FeeInvoiceReceivedAction{},
			Events: Events{
				Event_SwapOutReceiver_OnCancelInternal: State_SwapOutSender_SendCancel,
				Event_SwapOutSender_OnFeeInvoicePaid:   State_SwapOutSender_FeeInvoicePaid,
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
				Event_SwapOutSender_OnCancelSwapOut: State_SwapOut_Canceled,
			},
		},
		State_SwapOut_Canceled: {
			Action: &NoOpAction{},
			Events: Events{
				Event_SwapOutSender_OnCltvClaimMsgReceived: State_SwapOutSender_ClaimedCltv,
			},
		},
		State_SwapOutSender_TxBroadcasted: {
			Action: &SwapOutTxBroadCastedAction{},
			Events: Events{
				Event_SwapOutSender_OnAbortSwapInternal: State_SwapOutSender_SendCancel,
				Event_SwapOutSender_OnTxConfirmations:   State_SwapOutSender_TxConfirmed,
			},
		},
		State_SwapOutSender_TxConfirmed: {
			Action: &SwapOutTxConfirmedAction{},
			Events: Events{
				Event_SwapOutSender_OnAbortSwapInternal: State_SwapOutSender_SendCancel,
				Event_SwapOutSender_OnClaimTxPreimage:   State_SwapOutSender_ClaimInvPaid,
			},
		},
		State_SwapOutSender_ClaimInvPaid: {
			Action: &SwapOutClaimInvPaidAction{},
			Events: Events{
				Event_SwapOutSender_FinishSwap: State_SwapOutSender_ClaimedPreimage,
				Event_OnRetry:                  State_SwapOutSender_ClaimInvPaid,
			},
		},
		State_SwapOutSender_ClaimedPreimage: {
			Action: &NoOpAction{},
		},
		State_SwapOutSender_ClaimedCltv: {
			Action: &SwapOutClaimedCltvAction{},
		},
	}
}
