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
	State_SwapOutReceiver_ClaimInvoicePaid     StateType = "State_SwapOutReceiver_ClaimInvoicePaid"
	State_SwapOutReceiver_ClaimedPreimage      StateType = "State_SwapOutReceiver_ClaimedPreimage"
	State_SwapOutReceiver_SwapAborted          StateType = "State_SwapOutReceiver_Aborted"
	State_SwapOutReceiver_CltvPassed           StateType = "State_SwapOutReceiver_CltvPassed"
	State_SwapOutReceiver_ClaimedCltv          StateType = "State_SwapOutReceiver_ClaimedCltv"

	State_SwapOutSendCancel StateType = "State_SwapOutSendCancel"

	Event_SwapOutReceiver_OnSwapOutRequestReceived EventType = "Event_SwapOutReceiver_OnSwapOutRequestReceived"
	Event_SwapOutReceiver_OnSwapCreated            EventType = "Event_SwapOutReceiver_SwapCreated"

	Event_SwapOutReceiver_OnSendFeeInvoiceSuceeded EventType = "Event_SwapOutReceiver_SendFeeInvoiceSuceede"
	Event_SwapOutReceiver_OnFeeInvoicePaid         EventType = "Event_SwapOutReceiver_OnFeeInvoicePaid"
	Event_SwapOutReceiver_OnTxBroadcasted          EventType = "Event_SwapOutReceiver_TxBroadcasted"
	Event_SwapOutReceiver_OnClaimInvoicePaid       EventType = "Event_SwapOutReceiver_OnClaimInvoicePaid"
	Event_SwapOutReceiver_OnClaimMsgReceived       EventType = "Event_SwapOutReceiver_OnClaimMsgReceived"
	Event_SwapOutReceiver_OnCltvPassed             EventType = "Event_SwapOutReceiver_OnCltvPassed"
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

func (c *CreateSwapFromRequestContext) ApplyOnSwap(swap *Swap) {
	swap.Amount = c.amount
	swap.PeerNodeId = c.peer
	swap.ChannelId = c.channelId
	swap.Id = c.swapId
	swap.TakerPubkeyHash = c.takerPubkeyHash
}

type CreateSwapFromRequestAction struct{}

func (c *CreateSwapFromRequestAction) Execute(services *SwapServices, swap *Swap) EventType {
	newSwap := NewSwapFromRequest(swap.PeerNodeId, swap.Id, swap.Amount, swap.ChannelId, SWAPTYPE_OUT)
	newSwap.TakerPubkeyHash = swap.TakerPubkeyHash
	*swap = *newSwap

	//todo check balances

	pubkey := swap.GetPrivkey().PubKey()

	swap.Role = SWAPROLE_RECEIVER
	swap.MakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())
	// Generate Preimage
	preimage, err := lightning.GetPreimage()
	if err != nil {
		swap.LastErr = err
		return Event_SwapOutReceiver_OnCancelInternal
	}
	pHash := preimage.Hash()
	log.Printf("maker preimage: %s ", preimage.String())
	payreq, err := services.lightning.GetPayreq((swap.Amount)*1000, preimage.String(), "claim_"+swap.Id)
	if err != nil {
		swap.LastErr = err
		return Event_SwapOutReceiver_OnCancelInternal
	}

	swap.ClaimPayreq = payreq
	swap.ClaimPreimage = preimage.String()
	swap.ClaimPaymenHash = pHash.String()

	err = CreateOpeningTransaction(services, swap)
	if err != nil {
		swap.LastErr = err
		return Event_SwapOutReceiver_OnCancelInternal
	}

	fee, err := services.policy.GetMakerFee(swap.Amount, swap.OpeningTxFee)
	if err != nil {
		swap.LastErr = err
		return Event_SwapOutReceiver_OnCancelInternal
	}

	// Generate Preimage
	feepreimage, err := lightning.GetPreimage()
	if err != nil {
		swap.LastErr = err
		return Event_SwapOutReceiver_OnCancelInternal
	}
	feeInvoice, err := services.lightning.GetPayreq(fee*1000, feepreimage.String(), "fee_"+swap.Id)
	if err != nil {
		swap.LastErr = err
		return Event_SwapOutReceiver_OnCancelInternal
	}
	swap.FeeInvoice = feeInvoice
	return Event_SwapOutReceiver_OnSwapCreated
}

type SendFeeInvoiceAction struct{}

func (s *SendFeeInvoiceAction) Execute(services *SwapServices, swap *Swap) EventType {
	messenger := services.messenger

	msg := &FeeResponse{
		SwapId:  swap.Id,
		Invoice: swap.FeeInvoice,
	}
	err := messenger.SendMessage(swap.PeerNodeId, msg)
	if err != nil {
		swap.LastErr = err
		return Event_SwapOutReceiver_OnCancelInternal
	}
	return Event_SwapOutReceiver_OnSendFeeInvoiceSuceeded
}

type FeeInvoicePaidAction struct{}

// todo seperate into broadcast state and sendmessage state
func (b *FeeInvoicePaidAction) Execute(services *SwapServices, swap *Swap) EventType {

	node := services.blockchain
	txwatcher := services.txWatcher
	messenger := services.messenger

	finalizedTx, err := services.wallet.FinalizeTransaction(swap.OpeningTxUnpreparedHex)
	if err != nil {
		swap.LastErr = err
		return Event_SwapOutSender_OnCancelSwapOut
	}
	swap.OpeningTxHex = finalizedTx

	txId, err := node.SendRawTx(finalizedTx)
	if err != nil {
		swap.LastErr = err
		return Event_SwapOutSender_OnCancelSwapOut
	}

	swap.OpeningTxId = txId
	txwatcher.AddCltvTx(swap.Id, swap.Cltv)
	txwatcher.AddConfirmationsTx(swap.Id, txId)

	msg := &TxOpenedResponse{
		SwapId:          swap.Id,
		MakerPubkeyHash: swap.MakerPubkeyHash,
		Invoice:         swap.ClaimPayreq,
		TxId:            swap.OpeningTxId,
		TxHex:           swap.OpeningTxHex,
		Cltv:            swap.Cltv,
	}
	err = messenger.SendMessage(swap.PeerNodeId, msg)
	if err != nil {
		swap.LastErr = err
	}
	return Event_SwapOutReceiver_OnTxBroadcasted
}

type ClaimedPreimageAction struct{}

func (c *ClaimedPreimageAction) Execute(services *SwapServices, swap *Swap) EventType {
	return NoOp
}

type CltvPassedAction struct{}

// todo seperate into claim state and sendmessage state
func (c *CltvPassedAction) Execute(services *SwapServices, swap *Swap) EventType {

	blockchain := services.blockchain
	messenger := services.messenger

	claimTxHex, err := CreateCltvSpendingTransaction(services, swap)
	if err != nil {
		swap.LastErr = err
		return Event_OnRetry
	}

	claimId, err := blockchain.SendRawTx(claimTxHex)
	if err != nil {
		swap.LastErr = err
		return Event_OnRetry
	}
	swap.ClaimTxId = claimId
	msg := &ClaimedMessage{
		SwapId:    swap.Id,
		ClaimType: CLAIMTYPE_CLTV,
		ClaimTxId: claimId,
	}
	err = messenger.SendMessage(swap.PeerNodeId, msg)
	if err != nil {
		swap.LastErr = err
		return Event_OnRetry
	}
	return Event_SwapOutReceiver_OnCltvClaimed
}

type SendCancelAction struct{}

func (s *SendCancelAction) Execute(services *SwapServices, swap *Swap) EventType {
	log.Printf("[FSM] Canceling because of %s", swap.LastErr.Error())
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
	return Event_Action_Success
}

func SwapOutReceiverFSMFromStore(smData *StateMachine, services *SwapServices) *StateMachine {
	smData.swapServices = services
	smData.States = getSwapOutSenderStates()
	return smData
}

func newSwapOutReceiverFSM(id string, services *SwapServices) *StateMachine {
	return &StateMachine{
		Id:           id,
		swapServices: services,
		Type:         SWAPTYPE_OUT,
		Role:         SWAPROLE_RECEIVER,
		States:       getSwapOutReceiverStates(),
		Data:         &Swap{},
	}
}

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
				Event_OnCancelReceived:                 State_SwapOut_Canceled,
			},
		},
		State_SwapOutReceiver_FeeInvoicePaid: {
			Action: &FeeInvoicePaidAction{},
			Events: Events{
				Event_SwapOutReceiver_OnTxBroadcasted:  State_SwapOutReceiver_OpeningTxBroadcasted,
				Event_SwapOutReceiver_OnCancelInternal: State_SwapOutSendCancel,
			},
		},
		State_SwapOutReceiver_OpeningTxBroadcasted: {
			Action: &NoOpAction{},
			Events: Events{
				Event_SwapOutReceiver_OnClaimInvoicePaid: State_SwapOutReceiver_ClaimInvoicePaid,
				Event_OnCancelReceived:                   State_SwapOutReceiver_SwapAborted,
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
			Action: &ClaimedPreimageAction{},
		},
		State_SwapOutReceiver_SwapAborted: {
			Action: &NoOpAction{},
			Events: Events{
				Event_SwapOutReceiver_OnCltvPassed: State_SwapOutReceiver_CltvPassed,
			},
		},
		State_SwapOutReceiver_CltvPassed: {
			Action: &CltvPassedAction{},
			Events: Events{
				Event_SwapOutReceiver_OnCltvClaimed: State_SwapOutReceiver_ClaimedCltv,
			},
		},
		State_SwapOutReceiver_ClaimedCltv: {
			Action: &NoOpAction{},
		},
		State_SwapOutSendCancel: {
			Action: &SendCancelAction{},
			Events: Events{
				Event_Action_Success: State_SwapOut_Canceled,
			},
		},
		State_SwapOut_Canceled: {
			Action: &NoOpAction{},
		},
	}
}
