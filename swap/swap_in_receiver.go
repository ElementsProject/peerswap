package swap

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
)

// todo check for policy / balance
// SwapInReceiverInitAction creates the swap-in process
type SwapInReceiverInitAction struct{}

func (s *SwapInReceiverInitAction) Execute(services *SwapServices, swap *SwapData) EventType {
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
	newSwap := NewSwapFromRequest(swap.Id, swap.SwapId, swap.Chain, swap.ElementsAsset, swap.BitcoinNetwork, swap.PeerNodeId, swap.Amount, swap.Scid, SWAPTYPE_IN, swap.ProtocolVersion)
	newSwap.MakerPubkeyHash = swap.MakerPubkeyHash
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

	pubkey := swap.GetPrivkey().PubKey()
	swap.Role = SWAPROLE_RECEIVER
	swap.TakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())

	nextMessage, nextMessageType, err := MarshalPeerswapMessage(&SwapInAgreementMessage{
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
		SwapId:          swap.SwapId,
		Pubkey:          swap.TakerPubkeyHash,
		// todo: set premium
		Premium: 0,
	})
	if err != nil {
		return swap.HandleError(err)
	}
	swap.NextMessage = nextMessage
	swap.NextMessageType = nextMessageType
	return Event_ActionSucceeded
}

func MarshalPeerswapMessage(msg PeerMessage) ([]byte, int, error) {
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return nil, 0, err
	}
	return msgBytes, int(msg.MessageType()), nil
}

func (s *SwapData) HandleError(err error) EventType {
	s.LastErr = err
	if err != nil {
		s.LastErrString = err.Error()
	}
	if s.CancelMessage == "" {
		s.CancelMessage = s.LastErrString
	}
	log.Printf("swap error: %v", err)
	return Event_ActionFailed
}

// ClaimSwapTransactionWithPreimageAction spends the opening transaction to the nodes liquid wallet
type ClaimSwapTransactionWithPreimageAction struct{}

// todo this is very critical
func (s *ClaimSwapTransactionWithPreimageAction) Execute(services *SwapServices, swap *SwapData) EventType {
	err := CreatePreimageSpendingTransaction(services, swap)
	if err != nil {
		log.Printf("error claiming tx with preimage %v", err)
		return Event_OnRetry
	}
	return Event_ActionSucceeded
}

type SetStartingBlockHeightAction struct{}

func (s *SetStartingBlockHeightAction) Execute(services *SwapServices, swap *SwapData) EventType {
	onchain, _, _, err := services.getOnChainServices(swap.Chain)
	if err != nil {
		return swap.HandleError(err)
	}
	blockheight, err := onchain.GetBlockHeight()
	if err != nil {
		return swap.HandleError(err)
	}
	swap.StartingBlockHeight = blockheight
	return NoOp
}

type CancelAction struct{}

func (c *CancelAction) Execute(services *SwapServices, swap *SwapData) EventType {
	if swap.LastErr != nil {
		swap.LastErrString = swap.LastErr.Error()
	}
	return Event_Done
}

// swapInReceiverFromStore recovers a swap statemachine from the swap store
func swapInReceiverFromStore(smData *SwapStateMachine, services *SwapServices) *SwapStateMachine {
	smData.swapServices = services
	smData.States = getSwapInReceiverStates()
	return smData
}

// newSwapInReceiverFSM returns a new swap statemachine for a swap-in receiver
func newSwapInReceiverFSM(swapId *SwapId, services *SwapServices) *SwapStateMachine {
	return &SwapStateMachine{
		Id:           swapId.String(),
		SwapId:       swapId,
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
				Event_SwapInReceiver_OnRequestReceived: State_SwapInReceiver_CreateSwap,
			},
		},
		State_SwapInReceiver_CreateSwap: {
			Action: &SwapInReceiverInitAction{},
			Events: Events{
				Event_ActionSucceeded: State_SwapInReceiver_SendAgreement,
				Event_ActionFailed:    State_SendCancel,
			},
		},
		State_SwapInReceiver_SendAgreement: {
			Action: &SendMessageAction{},
			Events: Events{
				Event_ActionSucceeded: State_SwapInReceiver_AwaitTxBroadcastedMessage,
				Event_ActionFailed:    State_SendCancel,
			},
		},
		State_SwapInReceiver_AwaitTxBroadcastedMessage: {
			Action: &SetStartingBlockHeightAction{},
			Events: Events{
				Event_OnTxOpenedMessage: State_SwapInReceiver_AwaitTxConfirmation,
				Event_OnCancelReceived:  State_SwapCanceled,
				Event_ActionFailed:      State_SendCancel,
			},
		},
		State_SwapInReceiver_AwaitTxConfirmation: {
			Action: &AwaitTxConfirmationAction{},
			Events: Events{
				Event_OnTxConfirmed:    State_SwapInReceiver_ValidateTxAndPayClaimInvoice,
				Event_ActionFailed:     State_SendCancel,
				Event_OnCancelReceived: State_SwapInReceiver_SendPrivkey,
			},
		},
		State_SwapInReceiver_ValidateTxAndPayClaimInvoice: {
			Action: &ValidateTxAndPayClaimInvoiceAction{},
			Events: Events{
				Event_ActionSucceeded: State_SwapInReceiver_ClaimSwap,
				Event_ActionFailed:    State_SwapInReceiver_SendPrivkey,
			},
		},
		State_SwapInReceiver_SendPrivkey: {
			Action: &TakerSendPrivkeyAction{},
			Events: Events{
				Event_ActionFailed:    State_SendCancel,
				Event_ActionSucceeded: State_SwapInReceiver_SendCoopClose,
			},
		},
		State_SwapInReceiver_SendCoopClose: {
			Action: &SendMessageAction{},
			Events: Events{
				Event_ActionSucceeded: State_ClaimedCoop,
				Event_ActionFailed:    State_SendCancel,
			},
		},
		State_SwapInReceiver_ClaimSwap: {
			Action: &ClaimSwapTransactionWithPreimageAction{},
			Events: Events{
				Event_ActionSucceeded: State_ClaimedPreimage,
				Event_OnRetry:         State_SwapInReceiver_ClaimSwap,
			},
		},
		State_ClaimedPreimage: {
			Action: &NoOpDoneAction{},
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
		State_ClaimedCoop: {
			Action: &NoOpDoneAction{},
		},
	}
}
