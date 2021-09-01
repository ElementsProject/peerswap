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

	State_SendCancel       StateType = "State_SendCancel"
	State_SwapOut_Canceled StateType = "State_SwapOut_Canceled"

	Event_SwapOutSender_OnSwapOutRequested   EventType = "Event_SwapOutSender_OnSwapOutRequested"
	Event_SwapOutSender_OnSwapCreated        EventType = "Event_SwapOutSender_OnSwapCreated"
	Event_SwapOutSender_OnSendSwapOutSucceed EventType = "Event_SwapOutSender_OnSendSwapOutSucceed"
	Event_SwapOutSender_OnFeeInvReceived     EventType = "Event_SwapOutSender_OnFeeInvoiceReceived"
	Event_SwapOutSender_OnCancelSwapOut      EventType = "Event_SwapOutSender_OnCancelSwapOut"
	Event_SwapOutSender_OnFeeInvoicePaid     EventType = "Event_SwapOutSender_WaitInvoiceMessage"

	Event_OnTxOpenedMessage        EventType = "Event_OnTxOpenedMessage"
	Event_OnTxConfirmed            EventType = "Event_OnTxConfirmed"
	Event_SwapOutSender_FinishSwap EventType = "Event_SwapOutSender_FinishSwap"
	// todo retrystate? failstate? refundstate?
	Event_OnRetry                           EventType = "Event_OnRetry"
	Event_OnRecover                         EventType = "Event_OnRecover"
	Event_SwapOutSender_OnAbortSwapInternal EventType = "Event_SwapOutSender_OnAbortSwapInternal"
	Event_SwapOutSender_OnClaimTxPreimage   EventType = "Event_SwapOutSender_OnClaimTxPreimage"
	Event_OnClaimedCltv                     EventType = "Event_OnClaimedCltv"
)

type SwapCreationContext struct {
	swapId          string
	asset           string
	amount          uint64
	peer            string
	channelId       string
	initiatorId     string
	protocolversion uint64
}

func (c *SwapCreationContext) ApplyOnSwap(swap *SwapData) {
	swap.Amount = c.amount
	swap.PeerNodeId = c.peer
	swap.ChannelId = c.channelId
	swap.Asset = c.asset
	swap.Id = c.swapId
	swap.InitiatorNodeId = c.initiatorId
	swap.ProtocolVersion = c.protocolversion
}

// SwapInSenderInitAction creates the swap data
type SwapOutInitAction struct{}

//todo validate data
func (a *SwapOutInitAction) Execute(services *SwapServices, swap *SwapData) EventType {
	newSwap := NewSwap(swap.Id, swap.Asset, SWAPTYPE_OUT, SWAPROLE_SENDER, swap.Amount, swap.InitiatorNodeId, swap.PeerNodeId, swap.ChannelId, swap.ProtocolVersion)
	*swap = *newSwap
	return Event_SwapOutSender_OnSwapCreated
}

// SwapOutCreatedAction sends the request to the swap peer
type SwapOutCreatedAction struct{}

func (s *SwapOutCreatedAction) Execute(services *SwapServices, swap *SwapData) EventType {
	messenger := services.messenger

	pubkey := swap.GetPrivkey().PubKey()
	swap.TakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())

	msg := &SwapOutRequest{
		SwapId:          swap.Id,
		ChannelId:       swap.ChannelId,
		Amount:          swap.Amount,
		TakerPubkeyHash: swap.TakerPubkeyHash,
		ProtocolVersion: swap.ProtocolVersion,
		Asset:           swap.Asset,
	}
	err := messenger.SendMessage(swap.PeerNodeId, msg)
	if err != nil {

		return Event_SwapOutSender_OnCancelSwapOut
	}
	return Event_SwapOutSender_OnSendSwapOutSucceed
}

// FeeInvoiceReceivedAction checks the feeinvoice and pays it
type FeeInvoiceReceivedAction struct{}

func (r *FeeInvoiceReceivedAction) Execute(services *SwapServices, swap *SwapData) EventType {
	ll := services.lightning
	policy := services.policy
	invoice, err := ll.DecodePayreq(swap.FeeInvoice)
	if err != nil {

		log.Printf("error decoding %v", err)
		return Event_SwapOutReceiver_OnCancelInternal
	}
	swap.OpeningTxFee = invoice.Amount / 1000
	// todo check peerId
	if !policy.ShouldPayFee(swap.Amount, invoice.Amount, swap.PeerNodeId, swap.ChannelId) {

		log.Printf("won't pay fee %v", err)
		return Event_SwapOutReceiver_OnCancelInternal
	}
	preimage, err := ll.PayInvoice(swap.FeeInvoice)
	if err != nil {

		log.Printf("error paying out %v", err)
		return Event_SwapOutReceiver_OnCancelInternal
	}
	swap.FeePreimage = preimage
	return Event_SwapOutSender_OnFeeInvoicePaid
}

// SwapOutTxBroadCastedAction  checks the claim invoice and adds the transaction to the txwatcher
type SwapOutTxBroadCastedAction struct{}

func (t *SwapOutTxBroadCastedAction) Execute(services *SwapServices, swap *SwapData) EventType {
	lc := services.lightning

	invoice, err := lc.DecodePayreq(swap.ClaimInvoice)
	if err != nil {
		return Event_SwapOutSender_OnAbortSwapInternal
	}

	swap.ClaimPaymentHash = invoice.PHash

	// todo check policy

	err = services.onchain.AddWaitForConfirmationTx(swap.Id, swap.OpeningTxId)
	if err != nil {
		return Event_SwapOutSender_OnAbortSwapInternal
	}
	return NoOp
}

// SwapOutTxConfirmedAction pays the claim invoice
type SwapOutTxConfirmedAction struct{}

func (p *SwapOutTxConfirmedAction) Execute(services *SwapServices, swap *SwapData) EventType {
	lc := services.lightning
	ok, err := services.onchain.ValidateTx(swap.GetOpeningParams(), swap.Cltv, swap.OpeningTxId)
	if err != nil {
		return Event_SwapOutSender_OnAbortSwapInternal
	}
	if !ok {
		return Event_SwapOutSender_OnAbortSwapInternal
	}
	preimageString, err := lc.RebalancePayment(swap.ClaimInvoice, swap.ChannelId)
	if err != nil {
		return Event_SwapOutSender_OnAbortSwapInternal
	}
	swap.ClaimPreimage = preimageString
	return Event_SwapOutSender_OnClaimTxPreimage
}

// SwapOutTxConfirmedAction spends the opening transaction to the liquid wallet
type SwapOutClaimInvPaidAction struct{}

func (c *SwapOutClaimInvPaidAction) Execute(services *SwapServices, swap *SwapData) EventType {
	err := CreatePreimageSpendingTransaction(services, swap)
	if err != nil {
		log.Printf("error creating spending tx %v", err)
		swap.HandleError(err)
		return Event_OnRetry
	}

	//todo correct message
	msg := &ClaimedMessage{
		SwapId:    swap.Id,
		ClaimType: CLAIMTYPE_PREIMAGE,
		ClaimTxId: swap.ClaimTxId,
	}
	err = services.messenger.SendMessage(swap.PeerNodeId, msg)
	if err != nil {
		log.Printf("error sending message tx %v", err)
		swap.HandleError(err)
		return Event_OnRetry
	}
	return Event_SwapOutSender_FinishSwap
}

type NoOpAction struct{}

func (n *NoOpAction) Execute(services *SwapServices, swap *SwapData) EventType {
	return NoOp
}

// swapOutSenderFromStore recovers a swap statemachine from the swap store
func swapOutSenderFromStore(smData *SwapStateMachine, services *SwapServices) *SwapStateMachine {
	smData.swapServices = services
	smData.States = getSwapOutSenderStates()
	return smData
}

// newSwapOutSenderFSM returns a new swap statemachine for a swap-out sender
func newSwapOutSenderFSM(services *SwapServices) *SwapStateMachine {
	return &SwapStateMachine{
		Id:           newSwapId(),
		swapServices: services,
		Type:         SWAPTYPE_OUT,
		Role:         SWAPROLE_SENDER,
		States:       getSwapOutSenderStates(),
		Data:         &SwapData{},
	}
}

// getSwapOutSenderStates returns the states for the swap-out sender
func getSwapOutSenderStates() States {
	return States{
		Default: State{
			Events: Events{
				Event_SwapOutSender_OnSwapOutRequested: State_SwapOutSender_Init,
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
				Event_SwapOutSender_OnCancelSwapOut:      State_SwapOut_Canceled,
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
				Event_SwapOutReceiver_OnCancelInternal: State_SendCancel,
				Event_SwapOutSender_OnFeeInvoicePaid:   State_SwapOutSender_FeeInvoicePaid,
			},
		},
		State_SwapOutSender_FeeInvoicePaid: {
			Action: &NoOpAction{},
			Events: Events{
				Event_SwapOutReceiver_OnCancelInternal: State_SendCancel,
				Event_OnTxOpenedMessage:                State_SwapOutSender_TxBroadcasted,
			},
		},
		State_SendCancel: {
			Action: &SendCancelAction{},
			Events: Events{
				Event_Action_Success: State_SwapOut_Canceled,
				Event_OnRetry:        State_SendCancel,
			},
		},
		State_SwapOut_Canceled: {
			Action: &CancelAction{},
			Events: Events{
				Event_OnClaimedCltv: State_ClaimedCltv,
			},
		},
		State_SwapOutSender_TxBroadcasted: {
			Action: &SwapOutTxBroadCastedAction{},
			Events: Events{
				Event_SwapOutSender_OnAbortSwapInternal: State_SendCancel,
				Event_OnTxConfirmed:                     State_SwapOutSender_TxConfirmed,
			},
		},
		State_SwapOutSender_TxConfirmed: {
			Action: &SwapOutTxConfirmedAction{},
			Events: Events{
				Event_SwapOutSender_OnAbortSwapInternal: State_SendCancel,
				Event_SwapOutSender_OnClaimTxPreimage:   State_SwapOutSender_ClaimInvPaid,
			},
		},
		State_SwapOutSender_ClaimInvPaid: {
			Action: &SwapOutClaimInvPaidAction{},
			Events: Events{
				Event_SwapOutSender_FinishSwap: State_ClaimedPreimage,
				Event_OnRetry:                  State_SwapOutSender_ClaimInvPaid,
			},
		},
		State_ClaimedPreimage: {
			Action: &NoOpAction{},
		},
		State_ClaimedCltv: {
			Action: &NoOpAction{},
		},
	}
}
