package swap

import (
	"encoding/hex"
	"github.com/sputn1ck/peerswap/lightning"
	"log"
)

// todo every send message should be it's own state / action, if msg sending fails, tx will be broadcasted again / error occurs
// or make the sender a more sophisticated program which tries resending...
const (
	State_SwapOutReceiver_Init                 StateType = "State_SwapOutReceiver_Init"
	State_SwapOutReceiver_RequestReceived      StateType = "State_SwapOutReceiver_RequestReceived"
	State_SwapOutReceiver_FeeInvoiceSent       StateType = "State_SwapOutReceiver_FeeInvoiceSent"
	State_SwapOutReceiver_FeeInvoicePaid       StateType = "State_SwapOutReceiver_FeeInvoicePaid"
	State_SwapOutReceiver_OpeningTxBroadcasted StateType = "State_SwapOutReceiver_OpeningTxBroadcasted"
	State_SwapOutReceiver_TxMsgSent            StateType = "State_SwapOutReceiver_TxMsgSent"
	State_SwapOutReceiver_ClaimInvoicePaid     StateType = "State_SwapOutReceiver_ClaimInvoicePaid"
	State_SwapOutReceiver_SwapAborted          StateType = "State_SwapOutReceiver_Aborted"
	State_SwapOutReceiver_CltvPassed           StateType = "State_SwapOutReceiver_CltvPassed"
	State_SwapOutReceiver_TxClaimed            StateType = "State_SwapOutReceiver_TxClaimed"

	Event_SwapOutReceiver_OnSwapOutRequestReceived EventType = "Event_SwapOutReceiver_OnSwapOutRequestReceived"
	Event_SwapOutReceiver_OnSwapCreated            EventType = "Event_SwapOutReceiver_SwapCreated"

	Event_SwapOutReceiver_OnSendFeeInvoiceSuceeded EventType = "Event_SwapOutReceiver_SendFeeInvoiceSuceede"
	Event_SwapOutReceiver_OnFeeInvoicePaid         EventType = "Event_SwapOutReceiver_OnFeeInvoicePaid"
	Event_SwapOutReceiver_OnTxBroadcasted          EventType = "Event_SwapOutReceiver_TxBroadcasted"
	Event_OnClaimInvoicePaid                       EventType = "Event_OnClaimInvoicePaid"
	Event_OnClaimedPreimage                        EventType = "Event_OnClaimedPreimage"
	Event_OnCltvPassed                             EventType = "Event_OnCltvPassed"
	Event_SwapOutReceiver_OnCltvClaimed            EventType = "Event_SwapOutReceiver_OnCltvClaimed"

	Event_OnCancelReceived                 EventType = "Event_OnCancelReceived"
	Event_SwapOutReceiver_OnCancelInternal EventType = "Event_SwapOutReceiver_OnCancelInternal"

	Event_Action_Success EventType = "Event_Action_Success"
)

type CreateSwapFromRequestContext struct {
	amount          uint64
	peer            string
	channelId       string
	swapId          string
	takerPubkeyHash string
}

func (c *CreateSwapFromRequestContext) ApplyOnSwap(swap *SwapData) {
	swap.Amount = c.amount
	swap.PeerNodeId = c.peer
	swap.ChannelId = c.channelId
	swap.Id = c.swapId
	swap.TakerPubkeyHash = c.takerPubkeyHash
}

// CreateSwapFromRequestAction creates the swap-out process and prepares the opening transaction
type CreateSwapFromRequestAction struct{}

func (c *CreateSwapFromRequestAction) Execute(services *SwapServices, swap *SwapData) EventType {
	newSwap := NewSwapFromRequest(swap.PeerNodeId, swap.Id, swap.Amount, swap.ChannelId, SWAPTYPE_OUT)
	newSwap.TakerPubkeyHash = swap.TakerPubkeyHash
	*swap = *newSwap

	//todo check balance/policy if we want to create the swap

	pubkey := swap.GetPrivkey().PubKey()

	swap.Role = SWAPROLE_RECEIVER
	swap.MakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())
	// Generate Preimage
	preimage, err := lightning.GetPreimage()
	if err != nil {

		return Event_SwapOutReceiver_OnCancelInternal
	}
	pHash := preimage.Hash()
	log.Printf("maker preimage: %s ", preimage.String())
	payreq, err := services.lightning.GetPayreq((swap.Amount)*1000, preimage.String(), "claim_"+swap.Id)
	if err != nil {

		return Event_SwapOutReceiver_OnCancelInternal
	}

	swap.ClaimInvoice = payreq
	swap.ClaimPreimage = preimage.String()
	swap.ClaimPaymentHash = pHash.String()

	err = CreateOpeningTransaction(services, swap)
	if err != nil {
		return Event_SwapOutReceiver_OnCancelInternal
	}

	feeSat, err := services.policy.GetMakerFee(swap.Amount, swap.OpeningTxFee)
	if err != nil {

		return Event_SwapOutReceiver_OnCancelInternal
	}

	// Generate Preimage
	feepreimage, err := lightning.GetPreimage()
	if err != nil {

		return Event_SwapOutReceiver_OnCancelInternal
	}
	feeInvoice, err := services.lightning.GetPayreq(feeSat*1000, feepreimage.String(), "fee_"+swap.Id)
	if err != nil {

		return Event_SwapOutReceiver_OnCancelInternal
	}
	swap.FeeInvoice = feeInvoice
	return Event_SwapOutReceiver_OnSwapCreated
}

// SendFeeInvoiceAction sends the fee invoice to the swap peer
type SendFeeInvoiceAction struct{}

func (s *SendFeeInvoiceAction) Execute(services *SwapServices, swap *SwapData) EventType {
	messenger := services.messenger

	msg := &FeeMessage{
		SwapId:  swap.Id,
		Invoice: swap.FeeInvoice,
	}
	err := messenger.SendMessage(swap.PeerNodeId, msg)
	if err != nil {

		return Event_SwapOutReceiver_OnCancelInternal
	}
	return Event_SwapOutReceiver_OnSendFeeInvoiceSuceeded
}

// FeeInvoicePaidAction finalizes and broadcasts the opening transaction
type FeeInvoicePaidAction struct{}

func (b *FeeInvoicePaidAction) Execute(services *SwapServices, swap *SwapData) EventType {
	txId, finalizedTx, err := services.onchain.BroadcastOpeningTx(swap.OpeningTxUnpreparedHex)
	if err != nil {
		return Event_SwapOutSender_OnCancelSwapOut
	}

	swap.OpeningTxHex = finalizedTx
	swap.OpeningTxId = txId

	return Event_SwapOutReceiver_OnTxBroadcasted
}

// SwapOutReceiverOpeningTxBroadcastedAction sends the TxOpenedMessage to the peer
type SwapOutReceiverOpeningTxBroadcastedAction struct{}

func (s *SwapOutReceiverOpeningTxBroadcastedAction) Execute(services *SwapServices, swap *SwapData) EventType {
	err := services.onchain.AddWaitForCltvTx(swap.Id, swap.OpeningTxId, uint64(swap.Cltv))
	if err != nil {
		return swap.HandleError(err)
	}
	msg := &TxOpenedMessage{
		SwapId:          swap.Id,
		MakerPubkeyHash: swap.MakerPubkeyHash,
		Invoice:         swap.ClaimInvoice,
		TxId:            swap.OpeningTxId,
		Cltv:            swap.Cltv,
	}
	err = services.messenger.SendMessage(swap.PeerNodeId, msg)
	if err != nil {
		return swap.HandleError(err)
	}
	return Event_Action_Success
}

type ClaimedPreimageAction struct{}

func (c *ClaimedPreimageAction) Execute(services *SwapServices, swap *SwapData) EventType {
	return NoOp
}

// CltvPassedAction spends the opening transaction with a signature
type CltvPassedAction struct{}

func (c *CltvPassedAction) Execute(services *SwapServices, swap *SwapData) EventType {
	err := CreateCltvSpendingTransaction(services, swap)
	if err != nil {
		swap.HandleError(err)
		return Event_OnRetry
	}
	return Event_SwapOutReceiver_OnCltvClaimed
}

type CltvTxClaimedAction struct{}

func (c *CltvTxClaimedAction) Execute(services *SwapServices, swap *SwapData) EventType {
	msg := &ClaimedMessage{
		SwapId:    swap.Id,
		ClaimType: CLAIMTYPE_CLTV,
		ClaimTxId: swap.ClaimTxId,
	}
	err := services.messenger.SendMessage(swap.PeerNodeId, msg)
	if err != nil {
		return swap.HandleError(err)
	}
	return Event_Action_Success
}

// SendCancelAction sends a cancel message to the swap peer
type SendCancelAction struct{}

func (s *SendCancelAction) Execute(services *SwapServices, swap *SwapData) EventType {
	if swap.LastErr != nil {
		log.Printf("[FSM] Canceling because of %s", swap.LastErr.Error())
	}
	messenger := services.messenger
	msg := &CancelMessage{
		SwapId: swap.Id,
		Error:  swap.CancelMessage,
	}
	err := messenger.SendMessage(swap.PeerNodeId, msg)
	if err != nil {

		return Event_OnRetry
	}
	return Event_Action_Success
}

// swapOutReceiverFromStore recovers a swap statemachine from the swap store
func swapOutReceiverFromStore(smData *SwapStateMachine, services *SwapServices) *SwapStateMachine {
	smData.swapServices = services
	smData.States = getSwapOutReceiverStates()
	return smData
}

// newSwapOutReceiverFSM returns a new swap statemachine for a swap-out receiver
func newSwapOutReceiverFSM(id string, services *SwapServices) *SwapStateMachine {
	return &SwapStateMachine{
		Id:           id,
		swapServices: services,
		Type:         SWAPTYPE_OUT,
		Role:         SWAPROLE_RECEIVER,
		States:       getSwapOutReceiverStates(),
		Data:         &SwapData{},
	}
}

// getSwapOutReceiverStates returns the states for the swap-out receiver
func getSwapOutReceiverStates() States {
	return States{
		Default: State{
			Events: Events{
				Event_SwapOutReceiver_OnSwapOutRequestReceived: State_SwapOutReceiver_Init,
			},
		},
		State_SwapOutReceiver_Init: {
			Action: &CreateSwapFromRequestAction{},
			Events: Events{
				Event_SwapOutReceiver_OnSwapCreated:    State_SwapOutReceiver_RequestReceived,
				Event_SwapOutReceiver_OnCancelInternal: State_SendCancel,
			},
		},
		State_SwapOutReceiver_RequestReceived: {
			Action: &SendFeeInvoiceAction{},
			Events: Events{
				Event_SwapOutReceiver_OnCancelInternal:         State_SendCancel,
				Event_SwapOutReceiver_OnSendFeeInvoiceSuceeded: State_SwapOutReceiver_FeeInvoiceSent,
			},
		},
		State_SwapOutReceiver_FeeInvoiceSent: {
			Action: &NoOpAction{},
			Events: Events{
				Event_SwapOutReceiver_OnFeeInvoicePaid: State_SwapOutReceiver_FeeInvoicePaid,
				Event_OnCancelReceived:                 State_SwapOut_Canceled,
			},
		},
		State_SwapOutReceiver_FeeInvoicePaid: {
			Action: &FeeInvoicePaidAction{},
			Events: Events{
				Event_SwapOutReceiver_OnTxBroadcasted:  State_SwapOutReceiver_OpeningTxBroadcasted,
				Event_SwapOutReceiver_OnCancelInternal: State_SendCancel,
			},
		},
		State_SwapOutReceiver_OpeningTxBroadcasted: {
			Action: &SwapOutReceiverOpeningTxBroadcastedAction{},
			Events: Events{
				Event_Action_Success: State_SwapOutReceiver_TxMsgSent,
				Event_ActionFailed:   State_SwapOutReceiver_OpeningTxBroadcasted,
			},
		},
		State_SwapOutReceiver_TxMsgSent: {
			Action: &WaitCltvAction{},
			Events: Events{
				Event_OnClaimInvoicePaid: State_SwapOutReceiver_ClaimInvoicePaid,
				Event_OnCancelReceived:   State_SwapOutReceiver_SwapAborted,
				Event_OnCltvPassed:       State_SwapOutReceiver_CltvPassed,
			},
		},
		State_SwapOutReceiver_ClaimInvoicePaid: {
			Action: &NoOpAction{},
			Events: Events{
				Event_OnClaimedPreimage: State_ClaimedPreimage,
			},
		},
		State_ClaimedPreimage: {
			Action: &ClaimedPreimageAction{},
		},
		State_SwapOutReceiver_SwapAborted: {
			Action: &NoOpAction{},
			Events: Events{
				Event_OnCltvPassed: State_SwapOutReceiver_CltvPassed,
			},
		},
		State_SwapOutReceiver_CltvPassed: {
			Action: &CltvPassedAction{},
			Events: Events{
				Event_SwapOutReceiver_OnCltvClaimed: State_SwapOutReceiver_TxClaimed,
				Event_OnRetry:                       State_SwapOutReceiver_CltvPassed,
			},
		},
		State_SwapOutReceiver_TxClaimed: {
			Action: &CltvTxClaimedAction{},
			Events: Events{
				Event_Action_Success: State_ClaimedCltv,
				Event_ActionFailed:   State_ClaimedCltv,
			},
		},
		State_ClaimedCltv: {
			Action: &NoOpAction{},
		},
		State_SendCancel: {
			Action: &SendCancelAction{},
			Events: Events{
				Event_Action_Success: State_SwapOut_Canceled,
			},
		},
		State_SwapOut_Canceled: {
			Action: &CancelAction{},
		},
	}
}
