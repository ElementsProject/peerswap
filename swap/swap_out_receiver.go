package swap

import (
	"encoding/hex"
	"errors"
	"log"

	"github.com/sputn1ck/peerswap/lightning"
)

// todo every send message should be it's own state / action, if msg sending fails, tx will be broadcasted again / error occurs
// or make the sender a more sophisticated program which tries resending...
const ()

type CreateSwapFromRequestContext struct {
	amount          uint64
	asset           string
	peer            string
	channelId       string
	swapId          string
	takerPubkeyHash string
	protocolversion uint64
}

func (c *CreateSwapFromRequestContext) ApplyOnSwap(swap *SwapData) {
	swap.Amount = c.amount
	swap.Asset = c.asset
	swap.PeerNodeId = c.peer
	swap.ChannelId = c.channelId
	swap.Id = c.swapId
	swap.TakerPubkeyHash = c.takerPubkeyHash
	swap.ProtocolVersion = c.protocolversion
}

// CreateSwapFromRequestAction creates the swap-out process and prepares the opening transaction
type CreateSwapFromRequestAction struct{}

func (c *CreateSwapFromRequestAction) Execute(services *SwapServices, swap *SwapData) EventType {
	if swap.Asset == "l-btc" && !services.liquidEnabled {
		swap.LastErr = errors.New("l-btc swaps are not supported")
		swap.CancelMessage = "l-btc swaps are not supported"
		return Event_ActionFailed
	}
	if swap.Asset == "btc" && !services.bitcoinEnabled {
		swap.LastErr = errors.New("btc swaps are not supported")
		swap.CancelMessage = "btc swaps are not supported"
		return Event_ActionFailed
	}

	newSwap := NewSwapFromRequest(swap.PeerNodeId, swap.Asset, swap.Id, swap.Amount, swap.ChannelId, SWAPTYPE_OUT, swap.ProtocolVersion)
	newSwap.TakerPubkeyHash = swap.TakerPubkeyHash
	*swap = *newSwap

	if !services.policy.IsPeerAllowed(swap.PeerNodeId) {
		swap.CancelMessage = "peer not allowed to request swaps"
		return Event_ActionFailed
	}
	//todo check balance/policy if we want to create the swap
	pubkey := swap.GetPrivkey().PubKey()

	swap.Role = SWAPROLE_RECEIVER
	swap.MakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())
	// Generate Preimage
	preimage, err := lightning.GetPreimage()
	if err != nil {
		return Event_ActionFailed
	}
	pHash := preimage.Hash()
	log.Printf("maker preimage: %s ", preimage.String())
	payreq, err := services.lightning.GetPayreq((swap.Amount)*1000, preimage.String(), "claim_"+swap.Id)
	if err != nil {
		return Event_ActionFailed
	}

	swap.ClaimInvoice = payreq
	swap.ClaimPreimage = preimage.String()
	swap.ClaimPaymentHash = pHash.String()

	err = CreateOpeningTransaction(services, swap)
	if err != nil {
		swap.LastErr = err
		return Event_ActionFailed
	}

	/*
		 feeSat, err := services.policy.GetMakerFee(swap.Amount, swap.OpeningTxFee)
		 if err != nil {

		return Event_ActionFailed
		}
	*/
	feeSat := swap.OpeningTxFee

	// Generate Preimage
	feepreimage, err := lightning.GetPreimage()
	if err != nil {

		return Event_ActionFailed
	}
	feeInvoice, err := services.lightning.GetPayreq(feeSat*1000, feepreimage.String(), "fee_"+swap.Id)
	if err != nil {

		return Event_ActionFailed
	}
	swap.FeeInvoice = feeInvoice

	swap.NextMessage = FeeMessage{
		SwapId:  swap.Id,
		Invoice: swap.FeeInvoice,
	}
	return Event_ActionSucceeded
}

// BroadCastOpeningTxAction finalizes and broadcasts the opening transaction
type BroadCastOpeningTxAction struct{}

func (b *BroadCastOpeningTxAction) Execute(services *SwapServices, swap *SwapData) EventType {
	onchain, err := services.getOnchainAsset(swap.Asset)
	if err != nil {
		return Event_ActionFailed
	}
	txId, finalizedTx, err := onchain.BroadcastOpeningTx(swap.OpeningTxUnpreparedHex)
	if err != nil {
		return Event_ActionFailed
	}

	swap.OpeningTxHex = finalizedTx
	swap.OpeningTxId = txId

	// todo this will never throw an error

	swap.NextMessage = TxOpenedMessage{
		SwapId:          swap.Id,
		MakerPubkeyHash: swap.MakerPubkeyHash,
		Invoice:         swap.ClaimInvoice,
		TxId:            swap.OpeningTxId,
		Cltv:            swap.Cltv,
	}

	return Event_ActionSucceeded
}

// ClaimSwapTransactionWithCltv spends the opening transaction with a signature
type ClaimSwapTransactionWithCltv struct{}

func (c *ClaimSwapTransactionWithCltv) Execute(services *SwapServices, swap *SwapData) EventType {
	err := CreateCltvSpendingTransaction(services, swap)
	if err != nil {
		swap.HandleError(err)
		return Event_OnRetry
	}
	return Event_ActionSucceeded
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
		return Event_ActionFailed
	}
	return Event_ActionSucceeded
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
				Event_OnSwapOutRequestReceived: State_SwapOutReceiver_CreateSwap,
			},
		},
		State_SwapOutReceiver_CreateSwap: {
			Action: &CreateSwapFromRequestAction{},
			Events: Events{
				Event_ActionSucceeded: State_SwapOutReceiver_SendFeeInvoice,
				Event_ActionFailed:    State_SendCancel,
			},
		},
		State_SwapOutReceiver_SendFeeInvoice: {
			Action: &SendMessageAction{},
			Events: Events{
				Event_ActionFailed:    State_SendCancel,
				Event_ActionSucceeded: State_SwapOutReceiver_AwaitFeeInvoicePayment,
			},
		},
		State_SwapOutReceiver_AwaitFeeInvoicePayment: {
			Action: &NoOpAction{},
			Events: Events{
				Event_OnFeeInvoicePaid: State_SwapOutReceiver_BroadcastOpeningTx,
				Event_OnCancelReceived: State_SwapCanceled,
			},
		},
		State_SwapOutReceiver_BroadcastOpeningTx: {
			Action: &BroadCastOpeningTxAction{},
			Events: Events{
				Event_ActionSucceeded: State_SwapOutReceiver_SendTxBroadcastedMessage,
				Event_ActionFailed:    State_SendCancel,
			},
		},
		State_SwapOutReceiver_SendTxBroadcastedMessage: {
			Action: &SendMessageAction{},
			Events: Events{
				Event_ActionSucceeded: State_SwapOutReceiver_AwaitClaimInvoicePayment,
				Event_ActionFailed:    State_SwapOutReceiver_SwapAborted,
			},
		},
		State_SwapOutReceiver_AwaitClaimInvoicePayment: {
			Action: &AwaitCltvAction{},
			Events: Events{
				Event_OnClaimInvoicePaid: State_ClaimedPreimage,
				// todo this will be coop close
				Event_OnCancelReceived: State_SwapOutReceiver_SwapAborted,
				Event_OnCltvPassed:     State_SwapOutReceiver_ClaimSwap,
			},
		},
		State_SwapOutReceiver_SwapAborted: {
			Action: &AwaitCltvAction{},
			Events: Events{
				Event_OnCltvPassed: State_SwapOutReceiver_ClaimSwap,
			},
		},
		State_SwapOutReceiver_ClaimSwap: {
			Action: &ClaimSwapTransactionWithCltv{},
			Events: Events{
				Event_ActionSucceeded: State_ClaimedCltv,
				Event_OnRetry:         State_SwapOutReceiver_ClaimSwap,
			},
		},
		State_SendCancel: {
			Action: &SendCancelAction{},
			Events: Events{
				Event_ActionSucceeded: State_SwapCanceled,
				Event_ActionFailed:    State_SwapCanceled,
			},
		},
		State_SwapCanceled: {
			Action: &CancelAction{},
		},
		State_ClaimedCltv: {
			Action: &NoOpDoneAction{},
		},
		State_ClaimedPreimage: {
			Action: &NoOpDoneAction{},
		},
	}
}
