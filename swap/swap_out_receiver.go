package swap

import (
	"encoding/hex"
	"errors"
	"log"

	"github.com/btcsuite/btcd/btcec"

	"github.com/sputn1ck/peerswap/lightning"
)

// todo every send message should be it's own state / action, if msg sending fails, tx will be broadcasted again / error occurs
// or make the sender a more sophisticated program which tries resending...
const ()

type CreateSwapFromRequestContext struct {
	amount          uint64
	elementsAsset   string
	bitcoinNetwork  string
	peer            string
	channelId       string
	swapId          *SwapId
	id              string
	takerPubkeyHash string
	protocolversion uint64
	makerPubkey     string
}

func (c *CreateSwapFromRequestContext) ApplyOnSwap(swap *SwapData) {
	swap.Amount = c.amount
	swap.ElementsAsset = c.elementsAsset
	swap.BitcoinNetwork = c.bitcoinNetwork
	swap.PeerNodeId = c.peer
	swap.Scid = c.channelId
	swap.Id = c.id
	swap.SwapId = c.swapId
	swap.TakerPubkeyHash = c.takerPubkeyHash
	swap.MakerPubkeyHash = c.makerPubkey
	swap.ProtocolVersion = c.protocolversion
}

// CreateSwapFromRequestAction creates the swap-out process and prepares the opening transaction
type CreateSwapFromRequestAction struct{}

func (c *CreateSwapFromRequestAction) Execute(services *SwapServices, swap *SwapData) EventType {
	if swap.ElementsAsset != "" && swap.BitcoinNetwork == "" {
		swap.Chain = l_btc_asset
	} else if swap.ElementsAsset == "" && swap.BitcoinNetwork != "" {
		swap.Chain = btc_asset
	} else {
		swap.LastErr = errors.New("malformed request")
		swap.CancelMessage = "malformed request"
		services.requestedSwapsStore.Add(swap.PeerNodeId, RequestedSwap{
			Asset:           swap.Chain,
			AmountSat:       swap.Amount,
			Type:            swap.Type,
			RejectionReason: swap.CancelMessage,
		})
		return swap.HandleError(errors.New(swap.CancelMessage))
	}
	if swap.Chain == l_btc_asset && !services.liquidEnabled {
		swap.LastErr = errors.New("l-btc swaps are not supported")
		swap.CancelMessage = "l-btc swaps are not supported"
		services.requestedSwapsStore.Add(swap.PeerNodeId, RequestedSwap{
			Asset:           swap.Chain,
			AmountSat:       swap.Amount,
			Type:            swap.Type,
			RejectionReason: swap.CancelMessage,
		})
		return swap.HandleError(errors.New(swap.CancelMessage))
	}

	if swap.Chain == btc_asset && !services.bitcoinEnabled {
		swap.LastErr = errors.New("btc swaps are not supported")
		swap.CancelMessage = "btc swaps are not supported"
		services.requestedSwapsStore.Add(swap.PeerNodeId, RequestedSwap{
			Asset:           swap.Chain,
			AmountSat:       swap.Amount,
			Type:            swap.Type,
			RejectionReason: swap.CancelMessage,
		})
		return swap.HandleError(errors.New(swap.CancelMessage))
	}

	if swap.ProtocolVersion != PEERSWAP_PROTOCOL_VERSION {
		swap.CancelMessage = "incompatible peerswap version"
		services.requestedSwapsStore.Add(swap.PeerNodeId, RequestedSwap{
			Asset:           swap.Chain,
			AmountSat:       swap.Amount,
			Type:            swap.Type,
			RejectionReason: swap.CancelMessage,
		})
		return swap.HandleError(errors.New(swap.CancelMessage))
	}
	_, wallet, _, err := services.getOnChainServices(swap.Chain)
	if err != nil {
		return swap.HandleError(err)
	}
	if swap.ElementsAsset != "" && swap.ElementsAsset != wallet.GetAsset() {
		swap.CancelMessage = "invalid liquid asset"
		services.requestedSwapsStore.Add(swap.PeerNodeId, RequestedSwap{
			Asset:           swap.Chain,
			AmountSat:       swap.Amount,
			Type:            swap.Type,
			RejectionReason: swap.CancelMessage,
		})
		return swap.HandleError(errors.New(swap.CancelMessage))
	}
	if swap.BitcoinNetwork != "" && swap.BitcoinNetwork != wallet.GetNetwork() {
		swap.CancelMessage = "invalid bitcoin network"
		services.requestedSwapsStore.Add(swap.PeerNodeId, RequestedSwap{
			Asset:           swap.Chain,
			AmountSat:       swap.Amount,
			Type:            swap.Type,
			RejectionReason: swap.CancelMessage,
		})
		return swap.HandleError(errors.New(swap.CancelMessage))
	}

	newSwap := NewSwapFromRequest(swap.Id, swap.SwapId, swap.Chain, swap.ElementsAsset, swap.BitcoinNetwork, swap.PeerNodeId, swap.Amount, swap.Scid, SWAPTYPE_OUT, swap.ProtocolVersion)
	newSwap.TakerPubkeyHash = swap.TakerPubkeyHash
	*swap = *newSwap

	if !services.policy.IsPeerAllowed(swap.PeerNodeId) {
		swap.CancelMessage = "peer not allowed to request swaps"
		services.requestedSwapsStore.Add(swap.PeerNodeId, RequestedSwap{
			Asset:           swap.Chain,
			AmountSat:       swap.Amount,
			Type:            swap.Type,
			RejectionReason: swap.CancelMessage,
		})
		return swap.HandleError(errors.New(swap.CancelMessage))
	}
	//todo check balance/policy if we want to create the swap
	pubkey := swap.GetPrivkey().PubKey()

	swap.Role = SWAPROLE_RECEIVER
	swap.MakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())
	// Generate Preimage
	preimage, err := lightning.GetPreimage()
	if err != nil {
		return swap.HandleError(err)
	}
	pHash := preimage.Hash()
	log.Printf("maker preimage: %s ", preimage.String())
	expiry := uint64(3600)
	if swap.Chain == "btc" {
		expiry = 3600 * 24
	}
	payreq, err := services.lightning.GetPayreq((swap.Amount)*1000, preimage.String(), "claim_"+swap.Id, expiry)
	if err != nil {
		return swap.HandleError(err)
	}

	swap.ClaimInvoice = payreq
	swap.ClaimPreimage = preimage.String()
	swap.ClaimPaymentHash = pHash.String()

	err = CreateOpeningTransaction(services, swap)
	if err != nil {
		swap.LastErr = err
		return swap.HandleError(err)
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
		return swap.HandleError(err)
	}
	feeInvoice, err := services.lightning.GetPayreq(feeSat*1000, feepreimage.String(), "fee_"+swap.Id, 600)
	if err != nil {
		return swap.HandleError(err)
	}
	swap.FeeInvoice = feeInvoice

	nextMessage, nextMessageType, err := MarshalPeerswapMessage(&SwapOutAgreementMessage{
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
		SwapId:          swap.SwapId,
		Pubkey:          swap.MakerPubkeyHash,
		Payreq:          swap.FeeInvoice,
	})
	if err != nil {
		return swap.HandleError(err)
	}
	swap.NextMessage = nextMessage
	swap.NextMessageType = nextMessageType

	return Event_ActionSucceeded
}

// BroadCastOpeningTxAction finalizes and broadcasts the opening transaction
type BroadCastOpeningTxAction struct{}

func (b *BroadCastOpeningTxAction) Execute(services *SwapServices, swap *SwapData) EventType {
	txWatcher, wallet, _, err := services.getOnChainServices(swap.Chain)
	if err != nil {
		return swap.HandleError(err)
	}
	txId, finalizedTx, err := wallet.BroadcastOpeningTx(swap.OpeningTxUnpreparedHex)
	if err != nil {
		return swap.HandleError(err)
	}

	swap.OpeningTxHex = finalizedTx
	swap.OpeningTxId = txId

	startingHeight, err := txWatcher.GetBlockHeight()
	if err != nil {
		return swap.HandleError(err)
	}
	swap.StartingBlockHeight = startingHeight

	nextMessage, nextMessageType, err := MarshalPeerswapMessage(&OpeningTxBroadcastedMessage{
		SwapId:      swap.SwapId,
		Payreq:      swap.ClaimInvoice,
		TxId:        txId,
		ScriptOut:   swap.OpeningTxVout,
		BlindingKey: swap.BlindingKeyHex,
	})
	if err != nil {
		return swap.HandleError(err)
	}
	swap.NextMessage = nextMessage
	swap.NextMessageType = nextMessageType

	return Event_ActionSucceeded
}

// ClaimSwapTransactionWithCsv spends the opening transaction with a signature
type ClaimSwapTransactionWithCsv struct{}

func (c *ClaimSwapTransactionWithCsv) Execute(services *SwapServices, swap *SwapData) EventType {
	err := CreateCsvSpendingTransaction(services, swap)
	if err != nil {
		swap.HandleError(err)
		return Event_OnRetry
	}
	return Event_ActionSucceeded
}

// ClaimSwapTransactionWithCsv spends the opening transaction with maker and taker Signatures
type ClaimSwapTransactionCoop struct{}

func (c *ClaimSwapTransactionCoop) Execute(services *SwapServices, swap *SwapData) EventType {
	_, wallet, _, err := services.getOnChainServices(swap.Chain)
	if err != nil {
		return swap.HandleError(err)
	}
	openingParams := &OpeningParams{
		TakerPubkeyHash:  swap.TakerPubkeyHash,
		MakerPubkeyHash:  swap.MakerPubkeyHash,
		ClaimPaymentHash: swap.ClaimPaymentHash,
		Amount:           swap.Amount,
	}

	takerKeyBytes, err := hex.DecodeString(swap.TakerPrivkey)
	if err != nil {
		return swap.HandleError(err)
	}
	takerKey, _ := btcec.PrivKeyFromBytes(btcec.S256(), takerKeyBytes)
	makerKey, _ := btcec.PrivKeyFromBytes(btcec.S256(), swap.PrivkeyBytes)

	claimParams := &ClaimParams{
		Signer:       makerKey,
		OpeningTxHex: swap.OpeningTxHex,
	}
	if swap.Chain == l_btc_asset {
		err = SetBlindingParams(swap, openingParams)
		if err != nil {
			return swap.HandleError(err)
		}
	}
	txId, _, err := wallet.CreateCoopSpendingTransaction(openingParams, claimParams, takerKey)
	if err != nil {
		return swap.HandleError(err)
	}
	swap.ClaimTxId = txId
	return Event_ActionSucceeded
}

// SendCancelAction sends a cancel message to the swap peer
type SendCancelAction struct{}

func (s *SendCancelAction) Execute(services *SwapServices, swap *SwapData) EventType {
	if swap.LastErr != nil {
		log.Printf("[FSM] Canceling because of %s", swap.LastErr.Error())
	}
	messenger := services.messenger

	msgBytes, msgType, err := MarshalPeerswapMessage(&CancelMessage{
		SwapId:  swap.SwapId,
		Message: swap.CancelMessage,
	})
	if err != nil {
		return swap.HandleError(err)
	}

	err = messenger.SendMessage(swap.PeerNodeId, msgBytes, msgType)
	if err != nil {
		return swap.HandleError(err)
	}
	return Event_ActionSucceeded
}

// TakerSendPrivkeyAction builds the sighash to send the maker for cooperatively closing the swap
type TakerSendPrivkeyAction struct{}

func (s *TakerSendPrivkeyAction) Execute(services *SwapServices, swap *SwapData) EventType {
	privkeystring := hex.EncodeToString(swap.PrivkeyBytes)
	nextMessage, nextMessageType, err := MarshalPeerswapMessage(&CoopCloseMessage{
		SwapId:  swap.SwapId,
		Message: swap.CancelMessage,
		Privkey: privkeystring,
	})
	if err != nil {
		return swap.HandleError(err)
	}
	swap.NextMessage = nextMessage
	swap.NextMessageType = nextMessageType

	return Event_ActionSucceeded
}

// swapOutReceiverFromStore recovers a swap statemachine from the swap store
func swapOutReceiverFromStore(smData *SwapStateMachine, services *SwapServices) *SwapStateMachine {
	smData.swapServices = services
	smData.States = getSwapOutReceiverStates()
	return smData
}

// newSwapOutReceiverFSM returns a new swap statemachine for a swap-out receiver
func newSwapOutReceiverFSM(swapId *SwapId, services *SwapServices) *SwapStateMachine {
	return &SwapStateMachine{
		Id:           swapId.String(),
		SwapId:       swapId,
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
			Action: &SendMessageWithRetryAction{},
			Events: Events{
				Event_ActionSucceeded: State_SwapOutReceiver_AwaitClaimInvoicePayment,
				Event_ActionFailed:    State_SwapOutReceiver_SwapAborted,
			},
		},
		State_SwapOutReceiver_AwaitClaimInvoicePayment: {
			Action: &AwaitCsvAction{},
			Events: Events{
				Event_OnClaimInvoicePaid:  State_ClaimedPreimage,
				Event_OnCancelReceived:    State_SwapOutReceiver_ClaimSwapCsv,
				Event_OnCoopCloseReceived: State_SwapOutReceiver_ClaimSwapCoop,
				Event_OnCsvPassed:         State_SwapOutReceiver_ClaimSwapCsv,
			},
		},
		State_SwapOutReceiver_ClaimSwapCoop: {
			Action: &StopSendMessageWithRetryWrapperAction{next: &ClaimSwapTransactionCoop{}},
			Events: Events{
				Event_ActionSucceeded: State_ClaimedCoop,
				Event_ActionFailed:    State_SwapOutReceiver_SwapAborted,
			},
		},
		State_SwapOutReceiver_SwapAborted: {
			Action: &AwaitCsvAction{},
			Events: Events{
				Event_OnCsvPassed: State_SwapOutReceiver_ClaimSwapCsv,
			},
		},
		State_SwapOutReceiver_ClaimSwapCsv: {
			Action: &StopSendMessageWithRetryWrapperAction{next: &ClaimSwapTransactionWithCsv{}},
			Events: Events{
				Event_ActionSucceeded: State_ClaimedCsv,
				Event_OnRetry:         State_SwapOutReceiver_ClaimSwapCsv,
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
		State_ClaimedCsv: {
			Action: &NoOpDoneAction{},
		},
		State_ClaimedPreimage: {
			Action: &NoOpDoneAction{},
		},
		State_ClaimedCoop: {
			Action: &NoOpDoneAction{},
		},
	}
}
