package swap

import (
	"encoding/hex"
	"errors"
	"github.com/sputn1ck/peerswap/lightning"
)

const (
	State_SwapInReceiver_Init                 StateType = "State_SwapInReceiver_Init"
	State_SwapInReceiver_RequestReceived      StateType = "State_SwapInReceiver_RequestReceived"
	State_SwapInReceiver_AgreementSent        StateType = "State_SwapInReceiver_AgreementSent"
	State_SwapInReceiver_OpeningTxBroadcasted StateType = "State_SwapInReceiver_OpeningTxBroadcasted"
	State_SwapInReceiver_WaitForConfirmations StateType = "State_SwapInReceiver_WaitForConfirmations"
	State_SwapInReceiver_OpeningTxConfirmed   StateType = "State_SwapInReceiver_OpeningTxConfirmed"
	State_SwapInReceiver_ClaimInvoicePaid     StateType = "State_SwapInReceiver_ClaimInvoicePaid"
	State_ClaimedCltv                         StateType = "State_ClaimedCltv"
	State_ClaimedPreimage                     StateType = "State_ClaimedPreimage"

	Event_SwapInReceiver_OnRequestReceived  EventType = "Event_SwapInReceiver_OnRequestReceived"
	Event_SwapInReceiver_OnSwapCreated      EventType = "Event_SwapInReceiver_OnSwapCreated"
	Event_SwapInReceiver_OnAgreementSent    EventType = "Event_SwapInReceiver_OnAgreementSent"
	Event_SwapInReceiver_OnClaimInvoicePaid EventType = "Event_SwapInReceiver_OnClaimInvoicePaid"
)

// todo check for policy / balance
// SwapInReceiverInitAction creates the swap-in process
type SwapInReceiverInitAction struct{}

func (s *SwapInReceiverInitAction) Execute(services *SwapServices, swap *SwapData) EventType {
	newSwap := NewSwapFromRequest(swap.PeerNodeId, swap.Id, swap.Amount, swap.ChannelId, SWAPTYPE_IN)
	*swap = *newSwap

	pubkey := swap.GetPrivkey().PubKey()
	swap.Role = SWAPROLE_RECEIVER
	swap.TakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())

	return Event_SwapInReceiver_OnSwapCreated
}

// SwapInReceiverRequestReceivedAction sends the agreement message to the peer
type SwapInReceiverRequestReceivedAction struct{}

func (s *SwapInReceiverRequestReceivedAction) Execute(services *SwapServices, swap *SwapData) EventType {
	response := &SwapInAgreementMessage{
		SwapId:          swap.Id,
		TakerPubkeyHash: swap.TakerPubkeyHash,
	}
	err := services.messenger.SendMessage(swap.PeerNodeId, response)
	if err != nil {
		return Event_ActionFailed
	}
	return Event_SwapInReceiver_OnAgreementSent
}

// SwapInReceiverOpeningTxBroadcastedAction checks if the
// invoice is correct and adss the transaction to the txwatcher
type SwapInReceiverOpeningTxBroadcastedAction struct{}

func (s *SwapInReceiverOpeningTxBroadcastedAction) Execute(services *SwapServices, swap *SwapData) EventType {
	var invoice *lightning.Invoice

	invoice, swap.LastErr = services.lightning.DecodePayreq(swap.ClaimInvoice)
	if swap.LastErr != nil {
		return Event_ActionFailed
	}

	if invoice.Amount > (swap.Amount)*1000 {
		swap.LastErr = errors.New("invalid invoice price")
		return Event_ActionFailed
	}
	swap.ClaimPaymentHash = invoice.PHash
	services.txWatcher.AddConfirmationsTx(swap.Id, swap.OpeningTxId)

	return Event_Action_Success

}

// SwapInWaitForConfirmationsAction adds the swap opening tx to the txwatcher
type SwapInWaitForConfirmationsAction struct{}

func (s *SwapInWaitForConfirmationsAction) Execute(services *SwapServices, swap *SwapData) EventType {
	services.txWatcher.AddConfirmationsTx(swap.Id, swap.OpeningTxId)
	return NoOp
}

// SwapInWaitForConfirmationsAction pays the claim invoice
type SwapInReceiverOpeningTxConfirmedAction struct{}

func (s *SwapInReceiverOpeningTxConfirmedAction) Execute(services *SwapServices, swap *SwapData) EventType {
	var preimage string
	preimage, swap.LastErr = services.lightning.PayInvoice(swap.ClaimInvoice)
	if swap.LastErr != nil {
		return Event_ActionFailed
	}
	swap.ClaimPreimage = preimage

	return Event_SwapInReceiver_OnClaimInvoicePaid
}

// SwapInWaitForConfirmationsAction spends the opening transaction to the nodes liquid wallet
type SwapInReceiverClaimInvoicePaidAction struct{}

func (s *SwapInReceiverClaimInvoicePaidAction) Execute(services *SwapServices, swap *SwapData) EventType {
	var claimTxHex string
	claimTxHex, swap.LastErr = CreatePreimageSpendingTransaction(services, swap)
	if swap.LastErr != nil {
		return Event_ActionFailed
	}

	var claimId string
	claimId, swap.LastErr = services.blockchain.SendRawTx(claimTxHex)
	if swap.LastErr != nil {
		return Event_ActionFailed
	}
	swap.ClaimTxId = claimId
	msg := &ClaimedMessage{
		SwapId:    swap.Id,
		ClaimType: CLAIMTYPE_PREIMAGE,
		ClaimTxId: claimId,
	}
	err := services.messenger.SendMessage(swap.PeerNodeId, msg)
	if err != nil {
		return Event_ActionFailed
	}
	return Event_OnClaimedPreimage
}

// swapInReceiverFromStore recovers a swap statemachine from the swap store
func swapInReceiverFromStore(smData *SwapStateMachine, services *SwapServices) *SwapStateMachine {
	smData.swapServices = services
	smData.States = getSwapInReceiverStates()
	return smData
}

// newSwapInReceiverFSM returns a new swap statemachine for a swap-in receiver
func newSwapInReceiverFSM(id string, services *SwapServices) *SwapStateMachine {
	return &SwapStateMachine{
		Id:           id,
		swapServices: services,
		Type:         SWAPTYPE_IN,
		Role:         SWAPROLE_RECEIVER,
		States:       getSwapInReceiverStates(),
		Data:         &SwapData{},
	}
}

// getSwapInReceiverStates returns the states for the swap-in receiver
func getSwapInReceiverStates() States {
	return States{
		Default: State{
			Events: Events{
				Event_SwapInReceiver_OnRequestReceived: State_SwapInReceiver_Init,
			},
		},
		State_SwapInReceiver_Init: {
			Action: &SwapInReceiverInitAction{},
			Events: Events{
				Event_SwapInReceiver_OnSwapCreated: State_SwapInReceiver_RequestReceived,
				Event_ActionFailed:                 State_SendCancel,
			},
		},
		State_SwapInReceiver_RequestReceived: {
			Action: &SwapInReceiverRequestReceivedAction{},
			Events: Events{
				Event_SwapInReceiver_OnAgreementSent: State_SwapInReceiver_AgreementSent,
				Event_ActionFailed:                   State_SendCancel,
			},
		},
		State_SwapInReceiver_AgreementSent: {
			Action: &NoOpAction{},
			Events: Events{
				Event_OnTxOpenedMessage: State_SwapInReceiver_OpeningTxBroadcasted,
				Event_OnCancelReceived:  State_SwapCanceled,
			},
		},
		State_SwapInReceiver_OpeningTxBroadcasted: {
			Action: &SwapInReceiverOpeningTxBroadcastedAction{},
			Events: Events{
				Event_Action_Success: State_SwapInReceiver_WaitForConfirmations,
				Event_ActionFailed:   State_SendCancel,
			},
		},
		State_SwapInReceiver_WaitForConfirmations: {
			Action: &SwapInWaitForConfirmationsAction{},
			Events: Events{
				Event_OnTxConfirmed:    State_SwapInReceiver_OpeningTxConfirmed,
				Event_OnCancelReceived: State_SwapCanceled,
			},
		},
		State_SwapInReceiver_OpeningTxConfirmed: {
			Action: &SwapInReceiverOpeningTxConfirmedAction{},
			Events: Events{
				Event_SwapInReceiver_OnClaimInvoicePaid: State_SwapInReceiver_ClaimInvoicePaid,
				Event_ActionFailed:                      State_SendCancel,
			},
		},
		State_SwapInReceiver_ClaimInvoicePaid: {
			Action: &SwapInReceiverClaimInvoicePaidAction{},
			Events: Events{
				Event_OnClaimedPreimage: State_ClaimedPreimage,
			},
		},
		State_ClaimedCltv: {
			Action: &NoOpAction{},
		},
		State_ClaimedPreimage: {
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
			Events: Events{
				Event_OnClaimedCltv: State_ClaimedCltv,
			},
		},
	}
}
