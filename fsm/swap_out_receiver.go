package fsm

import (
	"encoding/hex"
	"github.com/sputn1ck/peerswap/lightning"
	"github.com/sputn1ck/peerswap/utils"
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
type CreateSwapFromRequestAction struct{}

func (c *CreateSwapFromRequestAction) Execute(services *SwapServices, data Data, eventCtx EventContext) EventType {
	request := eventCtx.(*CreateSwapFromRequestContext)
	swap := data.(*Swap)
	newSwap := NewSwapFromRequest(request.peer, request.swapId, request.amount, request.channelId, SWAPTYPE_OUT)
	*swap = *newSwap

	//todo check balances

	pubkey := swap.GetPrivkey().PubKey()

	swap.Role = SWAPROLE_SENDER
	swap.TakerPubkeyHash = request.takerPubkeyHash
	swap.MakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())
	// Generate Preimage
	preimage, err := lightning.GetPreimage()
	if err != nil {
		return Event_SwapOutReceiver_OnCancelInternal
	}
	pHash := preimage.Hash()
	log.Printf("maker preimage: %s ", preimage.String())
	payreq, err := services.lightning.GetPayreq((request.amount)*1000, preimage.String(), "redeem_"+swap.Id)
	if err != nil {
		return Event_SwapOutReceiver_OnCancelInternal
	}

	swap.Payreq = payreq
	swap.PreImage = preimage.String()
	swap.PHash = pHash.String()
	err = services.node.CreateOpeningTransaction(swap)
	if err != nil {
		return Event_SwapOutReceiver_OnCancelInternal
	}

	fee, err := services.policy.GetMakerFee(request.amount, swap.OpeningTxFee)
	if err != nil {
		return Event_SwapOutReceiver_OnCancelInternal
	}

	// Generate Preimage
	feepreimage, err := lightning.GetPreimage()
	if err != nil {
		return Event_SwapOutReceiver_OnCancelInternal
	}
	feeInvoice, err := services.lightning.GetPayreq(fee*1000, feepreimage.String(), "fee_"+swap.Id)
	if err != nil {
		return Event_SwapOutReceiver_OnCancelInternal
	}
	swap.FeeInvoice = feeInvoice
	return Event_SwapOutReceiver_OnSwapCreated
}

type SendFeeInvoiceAction struct{}

func (s *SendFeeInvoiceAction) Execute(services *SwapServices, data Data, eventCtx EventContext) EventType {
	messenger := services.messenger
	swap := data.(*Swap)

	msg := &FeeResponse{
		SwapId:  swap.Id,
		Invoice: swap.FeeInvoice,
	}
	err := messenger.SendMessage(swap.PeerNodeId, msg)
	if err != nil {
		return Event_SwapOutReceiver_OnCancelInternal
	}
	return Event_SwapOutReceiver_OnSendFeeInvoiceSuceeded
}

type FeeInvoicePaidAction struct{}

func (b *FeeInvoicePaidAction) Execute(services *SwapServices, data Data, eventCtx EventContext) EventType {
	swap := data.(*Swap)

	node := services.node
	txwatcher := services.txWatcher
	messenger := services.messenger

	txId, err := node.FinalizeAndBroadcastFundedTransaction(swap.OpeningTxUnpreparedHex)
	if err != nil {
		return Event_SwapOutSender_OnCancelSwapOut
	}

	swap.OpeningTxId = txId
	txwatcher.AddTx(swap.Id, txId, swap.OpeningTxHex, swap.Cltv)

	msg := &TxOpenedResponse{
		SwapId:          swap.Id,
		MakerPubkeyHash: swap.MakerPubkeyHash,
		Invoice:         swap.Payreq,
		TxId:            txId,
		TxHex:           swap.OpeningTxHex,
		Cltv:            swap.Cltv,
	}
	err = messenger.SendMessage(swap.PeerNodeId, msg)
	if err != nil {
		return Event_OnRetry
	}
	return Event_SwapOutReceiver_OnTxBroadcasted
}

type ClaimedPreimageAction struct{}

func (c *ClaimedPreimageAction) Execute(services *SwapServices, data Data, eventCtx EventContext) EventType {
	swap := data.(*Swap)
	ctx := eventCtx.(*ClaimedContext)
	swap.ClaimTxId = ctx.TxId
	return NoOp
}

type CltvPassedAction struct{}

func (c *CltvPassedAction) Execute(services *SwapServices, data Data, eventCtx EventContext) EventType {
	swap := data.(*Swap)

	node := services.node
	messenger := services.messenger

	redeemScript, err := node.GetSwapScript(swap)
	if err != nil {
		return Event_OnRetry
	}

	blockheight, err := node.GetBlockHeight()
	if err != nil {
		return Event_OnRetry
	}

	address, err := node.GetAddress()
	if err != nil {
		return Event_OnRetry
	}

	outputScript, err := utils.Blech32ToScript(address, node.GetNetwork())
	if err != nil {
		return Event_OnRetry
	}

	claimTxHex, err := utils.CreateCltvSpendingTransaction(&utils.SpendingParams{
		Signer:       swap.GetPrivkey(),
		OpeningTxHex: swap.OpeningTxHex,
		SwapAmount:   swap.Amount,
		// todo correct fee hex blabla
		FeeAmount:    node.GetFee(""),
		CurrentBlock: blockheight,
		Asset:        node.GetAsset(),
		OutputScript: outputScript,
		RedeemScript: redeemScript,
	})

	claimId, err := node.SendRawTx(claimTxHex)
	if err != nil {
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
		return Event_OnRetry
	}
	return Event_SwapOutReceiver_OnCltvClaimed
}

type SendCancelAction struct{}

func (s *SendCancelAction) Execute(services *SwapServices, data Data, eventCtx EventContext) EventType {
	swap := data.(*Swap)
	messenger := services.messenger
	msg := &CancelResponse{
		SwapId: swap.Id,
		Error:  swap.CancelMessage,
	}
	err := messenger.SendMessage(swap.PeerNodeId, msg)
	if err != nil {
		return Event_OnRetry
	}
	return Event_Action_Success
}

func newSwapOutReceiverFSM(id string, store Store, services *SwapServices) *StateMachine {
	return &StateMachine{
		Id:           id,
		store:        store,
		swapServices: services,
		Type:         SWAPTYPE_OUT,
		Role:         SWAPROLE_RECEIVER,
		States: States{
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
		},
	}
}
