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
	State_SwapInReceiver_ClaimedCltv          StateType = "State_SwapInReceiver_ClaimedCltv"
	State_SwapInReceiver_ClaimedPreimage      StateType = "State_SwapInReceiver_ClaimedPreimage"

	Event_SwapInReceiver_OnRequestReceived    EventType = "Event_SwapInReceiver_OnRequestReceived"
	Event_SwapInReceiver_OnSwapCreated        EventType = "Event_SwapInReceiver_OnSwapCreated"
	Event_SwapInReceiver_OnAgreementSent      EventType = "Event_SwapInReceiver_OnAgreementSent"
	Event_SwapInReceiver_OnTxBroadcasted      EventType = "Event_SwapInReceiver_OnTxBroadcasted"
	Event_SwapInReceiver_OnOpeningTxConfirmed EventType = "Event_SwapInReceiver_OnOpeningTxConvirmed"
	Event_SwapInReceiver_OnClaimInvoicePaid   EventType = "Event_SwapInReceiver_OnClaimInvoicePaid"
	Event_SwapInReceiver_OnClaimedPreimage    EventType = "Event_SwapInReceiver_OnClaimedPreimage"
	Event_SwapInReceiver_OnClaimedCltv        EventType = "Event_SwapInReceiver_OnClaimedCltv"
)

type SwapInReceiverInitAction struct{}

func (s *SwapInReceiverInitAction) Execute(services *SwapServices, swap *Swap) EventType {
	newSwap := NewSwapFromRequest(swap.PeerNodeId, swap.Id, swap.Amount, swap.ChannelId, SWAPTYPE_IN)
	*swap = *newSwap

	pubkey := swap.GetPrivkey().PubKey()
	swap.Role = SWAPROLE_RECEIVER
	swap.TakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())

	return Event_SwapInReceiver_OnSwapCreated
}

type SwapInReceiverRequestReceivedAction struct{}

func (s *SwapInReceiverRequestReceivedAction) Execute(services *SwapServices, swap *Swap) EventType {
	response := &SwapInAgreementResponse{
		SwapId:          swap.Id,
		TakerPubkeyHash: swap.TakerPubkeyHash,
	}
	err := services.messenger.SendMessage(swap.PeerNodeId, response)
	if err != nil {
		return Event_ActionFailed
	}
	return Event_SwapInReceiver_OnAgreementSent
}

type SwapInReceiverOpeningTxBroadcastedAction struct{}

func (s *SwapInReceiverOpeningTxBroadcastedAction) Execute(services *SwapServices, swap *Swap) EventType {
	var invoice *lightning.Invoice

	invoice, swap.LastErr = services.lightning.DecodePayreq(swap.ClaimPayreq)
	if swap.LastErr != nil {
		return Event_ActionFailed
	}

	if invoice.Amount > (swap.Amount)*1000 {
		swap.LastErr = errors.New("invalid invoice price")
		return Event_ActionFailed
	}
	swap.ClaimPaymenHash = invoice.PHash

	services.txWatcher.AddConfirmationsTx(swap.Id, swap.OpeningTxId)

	return Event_Action_Success

}

type SwapInReceiverOpeningTxConfirmedAction struct{}

func (s *SwapInReceiverOpeningTxConfirmedAction) Execute(services *SwapServices, swap *Swap) EventType {
	var preimage string
	preimage, swap.LastErr = services.lightning.PayInvoice(swap.ClaimPayreq)
	if swap.LastErr != nil {
		return Event_ActionFailed
	}
	swap.ClaimPreimage = preimage

	return Event_SwapInReceiver_OnClaimInvoicePaid
}

type SwapInReceiverClaimInvoicePaidAction struct{}

func (s *SwapInReceiverClaimInvoicePaidAction) Execute(services *SwapServices, swap *Swap) EventType {
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
	return Event_SwapInReceiver_OnClaimedPreimage
}
func SwapInReceiverFSMFromStore(smData *StateMachine, services *SwapServices) *StateMachine {
	smData.swapServices = services
	smData.States = getSwapInReceiverStates()
	return smData
}

func newSwapInReceiverFSM(id string, services *SwapServices) *StateMachine {
	return &StateMachine{
		Id:           id,
		swapServices: services,
		Type:         SWAPTYPE_OUT,
		Role:         SWAPROLE_RECEIVER,
		States:       getSwapInReceiverStates(),
		Data:         &Swap{},
	}
}

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
				Event_SwapInReceiver_OnTxBroadcasted: State_SwapInReceiver_OpeningTxBroadcasted,
				Event_OnCancelReceived:               State_SwapCanceled,
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
			Action: &NoOpAction{},
			Events: Events{
				Event_SwapInReceiver_OnOpeningTxConfirmed: State_SwapInReceiver_OpeningTxConfirmed,
				Event_OnCancelReceived:                    State_SwapCanceled,
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
				Event_SwapInReceiver_OnClaimedPreimage: State_SwapInReceiver_ClaimedPreimage,
			},
		},
		State_SwapInReceiver_ClaimedCltv: {
			Action: &NoOpAction{},
		},
		State_SwapInReceiver_ClaimedPreimage: {
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
				Event_SwapInReceiver_OnClaimedCltv: State_SwapInReceiver_ClaimedCltv,
			},
		},
	}
}
