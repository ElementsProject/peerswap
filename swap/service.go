package swap

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/sputn1ck/glightning/glightning"
)

const (
	PEERSWAP_PROTOCOL_VERSION = 1
)

var (
	AllowedAssets       = []string{"btc", "l-btc"}
	ErrSwapDoesNotExist = errors.New("swap does not exist")
)

// SwapService contains the logic for swaps
type SwapService struct {
	swapServices *SwapServices

	activeSwaps    map[string]*SwapStateMachine
	BitcoinEnabled bool
	LiquidEnabled  bool

	sync.Mutex
}

func NewSwapService(swapStore Store, enableLiquid bool, liquidChainService Onchain, enableBitcoin bool, bitcoinChainService Onchain, lightning LightningClient, messenger Messenger, policy Policy) *SwapService {

	services := NewSwapServices(
		swapStore,
		lightning,
		messenger,
		policy,
		nil,
		enableBitcoin,
		bitcoinChainService,
		enableLiquid,
		liquidChainService,
	)

	return &SwapService{
		swapServices:   services,
		activeSwaps:    map[string]*SwapStateMachine{},
		LiquidEnabled:  enableLiquid,
		BitcoinEnabled: enableBitcoin,
	}
}

// Start adds callback to the messenger, txwatcher services and lightning client
func (s *SwapService) Start() error {
	s.swapServices.messenger.AddMessageHandler(s.OnMessageReceived)

	if s.LiquidEnabled {
		s.swapServices.liquidOnchain.AddConfirmationCallback(s.OnTxConfirmed)
		s.swapServices.liquidOnchain.AddCltvCallback(s.OnCltvPassed)
	}
	if s.BitcoinEnabled {
		s.swapServices.bitcoinOnchain.AddConfirmationCallback(s.OnTxConfirmed)
		s.swapServices.bitcoinOnchain.AddCltvCallback(s.OnCltvPassed)
	}

	s.swapServices.lightning.AddPaymentCallback(s.OnPayment)

	return nil
}

// RecoverSwaps tries to recover swaps that are not yet finished
func (s *SwapService) RecoverSwaps() error {
	swaps, err := s.swapServices.swapStore.ListAll()
	if err != nil {
		return err
	}
	for _, swap := range swaps {
		if swap.IsFinished() {
			continue
		}
		if swap.Type == SWAPTYPE_IN && swap.Role == SWAPROLE_SENDER {
			swap = swapInSenderFromStore(swap, s.swapServices)
		} else if swap.Type == SWAPTYPE_IN && swap.Role == SWAPROLE_RECEIVER {
			swap = swapInReceiverFromStore(swap, s.swapServices)

		} else if swap.Type == SWAPTYPE_OUT && swap.Role == SWAPROLE_SENDER {
			swap = swapOutSenderFromStore(swap, s.swapServices)

		} else if swap.Type == SWAPTYPE_OUT && swap.Role == SWAPROLE_RECEIVER {
			swap = swapOutReceiverFromStore(swap, s.swapServices)
		}

		onchain, err := s.getOnchainAsset(swap.Data.Asset)
		if err != nil {
			return err
		}
		s.swapServices.onchain = onchain

		s.AddActiveSwap(swap.Id, swap)

		done, err := swap.Recover()
		if err != nil {
			return err
		}

		if done {
			s.RemoveActiveSwap(swap.Id)
		}
	}
	return nil
}

// OnMessageReceived handles incoming valid peermessages
func (s *SwapService) OnMessageReceived(peerId string, msgTypeString string, payload string) error {
	msgType, err := HexStrToMsgType(msgTypeString)
	if err != nil {
		return err
	}
	msgBytes := []byte(payload)
	log.Printf("[Messenger] From: %s got msgtype: %s payload: %s", peerId, msgTypeString, payload)
	switch msgType {
	case MESSAGETYPE_SWAPOUTREQUEST:
		var msg *SwapOutRequest
		err := json.Unmarshal(msgBytes, &msg)
		if err != nil {
			return err
		}
		err = s.OnSwapOutRequestReceived(peerId, msg.Asset, msg.ChannelId, msg.SwapId, msg.TakerPubkeyHash, msg.Amount, msg.ProtocolVersion)
		if err != nil {
			return err
		}
	case MESSAGETYPE_FEERESPONSE:
		var msg *FeeMessage
		err := json.Unmarshal(msgBytes, &msg)
		if err != nil {
			return err
		}
		err = s.OnFeeInvoiceReceived(msg.SwapId, msg.Invoice)
		if err != nil {
			return err
		}
	case MESSAGETYPE_TXOPENEDRESPONSE:
		var msg *TxOpenedMessage
		err := json.Unmarshal(msgBytes, &msg)
		if err != nil {
			return err
		}
		err = s.OnTxOpenedMessage(msg.SwapId, msg.MakerPubkeyHash, msg.Invoice, msg.TxId, msg.Cltv)
		if err != nil {
			return err
		}
	case MESSAGETYPE_CANCELED:
		var msg *CancelMessage
		err := json.Unmarshal(msgBytes, &msg)
		if err != nil {
			return err
		}
		err = s.OnCancelReceived(msg.SwapId)
		if err != nil {
			return err
		}
	case MESSAGETYPE_SWAPINREQUEST:
		var msg *SwapInRequest
		err := json.Unmarshal(msgBytes, &msg)
		if err != nil {
			return err
		}
		err = s.OnSwapInRequestReceived(peerId, msg.Asset, msg.ChannelId, msg.SwapId, msg.Amount, msg.ProtocolVersion)
		if err != nil {
			return err
		}
	case MESSAGETYPE_SWAPINAGREEMENT:
		var msg *SwapInAgreementMessage
		err := json.Unmarshal(msgBytes, &msg)
		if err != nil {
			return err
		}
		err = s.OnAgreementReceived(msg)
		if err != nil {
			return err
		}
	case MESSAGETYPE_CLAIMED:
		var msg *ClaimedMessage
		err := json.Unmarshal(msgBytes, &msg)
		if err != nil {
			return err
		}
		if msg.ClaimType == CLAIMTYPE_CLTV {
			err = s.OnCltvClaimMessageReceived(msg.SwapId, msg.ClaimTxId)
		} else if msg.ClaimType == CLAIMTYPE_PREIMAGE {
			err = s.OnPreimageClaimMessageReceived(msg.SwapId, msg.ClaimTxId)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// OnTxConfirmed sends the txconfirmed event to the corresponding swap
func (s *SwapService) OnTxConfirmed(swapId string) error {
	swap, err := s.GetActiveSwap(swapId)
	if err != nil {
		return err
	}
	done, err := swap.SendEvent(Event_OnTxConfirmed, nil)
	if err == ErrEventRejected {
		return nil
	} else if err != nil {
		return err
	}
	if done {
		s.RemoveActiveSwap(swap.Id)
	}
	return nil
}

// OnCltvPassed sends the cltvpassed event to the corresponding swap
func (s *SwapService) OnCltvPassed(swapId string) error {
	swap, err := s.GetActiveSwap(swapId)
	if err != nil {
		return err
	}
	done, err := swap.SendEvent(Event_OnCltvPassed, nil)
	if err == ErrEventRejected {
		return nil
	} else if err != nil {
		return err
	}
	if done {
		s.RemoveActiveSwap(swap.Id)
	}
	return nil
}

// todo check prerequisites
// SwapOut starts a new swap out process
func (s *SwapService) SwapOut(peer string, asset string, channelId string, initiator string, amount uint64) (*SwapStateMachine, error) {
	if s.hasActiveSwapOnChannel(channelId) {
		return nil, fmt.Errorf("already has an active swap on channel")
	}

	onchain, err := s.getOnchainAsset(asset)
	if err != nil {
		return nil, err
	}
	s.swapServices.onchain = onchain

	log.Printf("[SwapService] Start swapping out: peer: %s chanId: %s initiator: %s amount %v", peer, channelId, initiator, amount)
	swap := newSwapOutSenderFSM(s.swapServices)
	s.AddActiveSwap(swap.Id, swap)
	done, err := swap.SendEvent(Event_SwapOutSender_OnSwapOutRequested, &SwapCreationContext{
		amount:          amount,
		asset:           asset,
		initiatorId:     initiator,
		peer:            peer,
		channelId:       channelId,
		swapId:          swap.Id,
		protocolversion: PEERSWAP_PROTOCOL_VERSION,
	})
	if err != nil {
		return nil, err
	}
	if done {
		s.RemoveActiveSwap(swap.Id)
	}
	return swap, nil
}

// todo check prerequisites
// SwapIn starts a new swap in process
func (s *SwapService) SwapIn(peer string, asset string, channelId string, initiator string, amount uint64) (*SwapStateMachine, error) {
	if s.hasActiveSwapOnChannel(channelId) {
		return nil, fmt.Errorf("already has an active swap on channel")
	}

	onchain, err := s.getOnchainAsset(asset)
	if err != nil {
		return nil, err
	}
	s.swapServices.onchain = onchain

	swap := newSwapInSenderFSM(s.swapServices)
	s.AddActiveSwap(swap.Id, swap)
	done, err := swap.SendEvent(Event_SwapInSender_OnSwapInRequested, &SwapCreationContext{
		amount:          amount,
		asset:           asset,
		initiatorId:     initiator,
		peer:            peer,
		channelId:       channelId,
		swapId:          swap.Id,
		protocolversion: PEERSWAP_PROTOCOL_VERSION,
	})
	if err != nil {
		return nil, err
	}
	if done {
		s.RemoveActiveSwap(swap.Id)
	}
	return swap, nil
}

// OnSwapInRequestReceived creates a new swap-in process and sends the event to the swap statemachine
func (s *SwapService) OnSwapInRequestReceived(peer, asset, channelId, swapId string, amount, protocolversion uint64) error {
	if s.hasActiveSwapOnChannel(channelId) {
		return fmt.Errorf("already has an active swap on channel")
	}

	onchain, err := s.getOnchainAsset(asset)
	if err != nil {
		return err
	}
	s.swapServices.onchain = onchain

	swap := newSwapInReceiverFSM(swapId, s.swapServices)
	s.AddActiveSwap(swapId, swap)

	done, err := swap.SendEvent(Event_SwapInReceiver_OnRequestReceived, &CreateSwapFromRequestContext{
		amount:          amount,
		asset:           asset,
		peer:            peer,
		channelId:       channelId,
		swapId:          swapId,
		protocolversion: protocolversion,
	})
	if done {
		s.RemoveActiveSwap(swap.Id)
	}
	return err
}

// OnSwapInRequestReceived creates a new swap-out process and sends the event to the swap statemachine
func (s *SwapService) OnSwapOutRequestReceived(peer, asset, channelId, swapId, takerPubkeyHash string, amount, protocolversion uint64) error {
	if s.hasActiveSwapOnChannel(channelId) {
		return fmt.Errorf("already has an active swap on channel")
	}

	onchain, err := s.getOnchainAsset(asset)
	if err != nil {
		return err
	}
	s.swapServices.onchain = onchain

	swap := newSwapOutReceiverFSM(swapId, s.swapServices)
	s.AddActiveSwap(swapId, swap)
	done, err := swap.SendEvent(Event_SwapOutReceiver_OnSwapOutRequestReceived, &CreateSwapFromRequestContext{
		amount:          amount,
		asset:           asset,
		peer:            peer,
		channelId:       channelId,
		swapId:          swapId,
		takerPubkeyHash: takerPubkeyHash,
		protocolversion: protocolversion,
	})
	if err != nil {
		return err
	}
	if done {
		s.RemoveActiveSwap(swap.Id)
	}
	return nil
}

// OnAgreementReceived sends the agreementreceived event to the corresponding swap state machine
func (s *SwapService) OnAgreementReceived(msg *SwapInAgreementMessage) error {
	swap, err := s.GetActiveSwap(msg.SwapId)
	if err != nil {
		return err
	}
	done, err := swap.SendEvent(Event_SwapInSender_OnAgreementReceived, msg)
	if err != nil {
		return err
	}
	if done {
		s.RemoveActiveSwap(swap.Id)
	}
	return nil
}

// OnFeeInvoiceReceived sends the FeeInvoiceReceived event to the corresponding swap state machine
func (s *SwapService) OnFeeInvoiceReceived(swapId, feeInvoice string) error {
	swap, err := s.GetActiveSwap(swapId)
	if err != nil {
		return err
	}
	done, err := swap.SendEvent(Event_SwapOutSender_OnFeeInvReceived, &FeeMessage{Invoice: feeInvoice})
	if err != nil {
		return err
	}
	if done {
		s.RemoveActiveSwap(swap.Id)
	}
	return nil
}

// OnFeeInvoicePaid sends the FeeInvoicePaid event to the corresponding swap state machine
func (s *SwapService) OnFeeInvoicePaid(swapId string) error {
	swap, err := s.GetActiveSwap(swapId)
	if err != nil {
		return err
	}
	done, err := swap.SendEvent(Event_SwapOutReceiver_OnFeeInvoicePaid, nil)
	if err != nil {
		return err
	}
	if done {
		s.RemoveActiveSwap(swap.Id)
	}
	return nil
}

// OnClaimInvoicePaid sends the ClaimInvoicePaid event to the corresponding swap state machine
func (s *SwapService) OnClaimInvoicePaid(swapId string) error {
	swap, err := s.GetActiveSwap(swapId)
	if err != nil {
		return err
	}
	done, err := swap.SendEvent(Event_OnClaimInvoicePaid, nil)
	if err != nil {
		return err
	}
	if done {
		s.RemoveActiveSwap(swap.Id)
	}
	return nil
}

// OnPreimageClaimMessageReceived sends the ClaimedPreimage event to the corresponding swap state machine
func (s *SwapService) OnPreimageClaimMessageReceived(swapId string, txId string) error {
	swap, err := s.GetActiveSwap(swapId)
	if err != nil {
		return err
	}
	done, err := swap.SendEvent(Event_OnClaimedPreimage, &ClaimedMessage{ClaimTxId: txId})
	if err != nil {
		return err
	}
	if done {
		s.RemoveActiveSwap(swap.Id)
	}
	return nil
}

// OnCltvClaimMessageReceived sends the ClaimedCltv event to the corresponding swap state machine
func (s *SwapService) OnCltvClaimMessageReceived(swapId string, txId string) error {
	swap, err := s.GetActiveSwap(swapId)
	if err != nil {
		return err
	}
	done, err := swap.SendEvent(Event_OnClaimedCltv, &ClaimedMessage{ClaimTxId: txId})
	if err != nil {
		return err
	}
	if done {
		s.RemoveActiveSwap(swap.Id)
	}
	return nil
}

// OnTxOpenedMessage sends the TxOpenedMessage event to the corresponding swap state machine
func (s *SwapService) OnTxOpenedMessage(swapId, makerPubkeyHash, claimInvoice, txId string, cltv int64) error {
	swap, err := s.GetActiveSwap(swapId)
	if err != nil {
		return err
	}
	done, err := swap.SendEvent(Event_OnTxOpenedMessage, &TxOpenedMessage{
		SwapId:          swap.Id,
		MakerPubkeyHash: makerPubkeyHash,
		Invoice:         claimInvoice,
		TxId:            txId,
		Cltv:            cltv,
	})
	if err != nil {
		return err
	}
	if done {
		s.RemoveActiveSwap(swap.Id)
	}
	return nil
}

// OnTxOpenedMessage sends the TxConfirmed event to the corresponding swap state machine
func (s *SwapService) SenderOnTxConfirmed(swapId string) error {
	swap, err := s.GetActiveSwap(swapId)
	if err != nil {
		return err
	}
	done, err := swap.SendEvent(Event_OnTxConfirmed, nil)
	if err != nil {
		return err
	}
	if done {
		s.RemoveActiveSwap(swap.Id)
	}
	s.RemoveActiveSwap(swap.Id)
	return nil
}

// OnPayment handles incoming payments and if it corresponds to a claim or
// fee invoice passes the dater to the corresponding function
func (s *SwapService) OnPayment(payment *glightning.Payment) {
	// check if feelabel
	var swapId string
	var err error
	if strings.Contains(payment.Label, "claim_") && len(payment.Label) == (len("claim_")+64) {
		log.Printf("[SwapService] New claim payment received %s", payment.Label)
		swapId = payment.Label[6:]
		err = s.OnClaimInvoicePaid(swapId)
	} else if strings.Contains(payment.Label, "fee_") && len(payment.Label) == (len("fee_")+64) {
		log.Printf("[SwapService] New fee payment received %s", payment.Label)
		swapId = payment.Label[4:]
		err = s.OnFeeInvoicePaid(swapId)
	} else {
		return
	}

	if err != nil {
		log.Printf("error handling onfeeinvoice paid %v", err)
		return
	}
}

// OnCancelReceived sends the CancelReceived event to the corresponding swap state machine
func (s *SwapService) OnCancelReceived(swapId string) error {
	swap, err := s.GetActiveSwap(swapId)
	if err != nil {
		return err
	}
	done, err := swap.SendEvent(Event_OnCancelReceived, nil)
	if err != nil {
		return err
	}
	if done {
		s.RemoveActiveSwap(swap.Id)
	}
	return nil
}

// ListSwaps returns all swaps stored
func (s *SwapService) ListSwaps() ([]*SwapStateMachine, error) {
	return s.swapServices.swapStore.ListAll()
}

// ListSwapsByPeer only returns the swaps that are done with a specific peer
func (s *SwapService) ListSwapsByPeer(peer string) ([]*SwapStateMachine, error) {
	return s.swapServices.swapStore.ListAllByPeer(peer)
}

func (s *SwapService) GetSwap(swapId string) (*SwapStateMachine, error) {
	return s.swapServices.swapStore.GetData(swapId)
}

// AddActiveSwap adds a swap to the active swaps
func (s *SwapService) AddActiveSwap(swapId string, swap *SwapStateMachine) {
	// todo: why does this function take a swapId if we have a swap struct containing the swapId?
	s.Lock()
	defer s.Unlock()
	s.activeSwaps[swapId] = swap
}

// GetActiveSwap returns the active swap, or an error if it does not exist
func (s *SwapService) GetActiveSwap(swapId string) (*SwapStateMachine, error) {
	if swap, ok := s.activeSwaps[swapId]; ok {
		return swap, nil
	}
	return nil, ErrSwapDoesNotExist
}

// RemoveActiveSwap removes a swap from the active swap map
func (s *SwapService) RemoveActiveSwap(swapId string) {
	s.Lock()
	defer s.Unlock()
	delete(s.activeSwaps, swapId)
}

func (s *SwapService) hasActiveSwapOnChannel(channelId string) bool {
	s.Lock()
	defer s.Unlock()
	for _, swap := range s.activeSwaps {
		if swap.Data.ChannelId == channelId {
			return true
		}
	}

	return false
}

type WrongAssetError string

func (e WrongAssetError) Error() string {
	return fmt.Sprintf("unallowed asset: %s", string(e))
}

func (s *SwapService) getOnchainAsset(asset string) (Onchain, error) {
	if asset == "" {
		return nil, fmt.Errorf("missing asset")
	}
	if asset == "btc" {
		return s.swapServices.bitcoinOnchain, nil
	}
	if asset == "l-btc" {
		return s.swapServices.liquidOnchain, nil
	}
	return nil, WrongAssetError(asset)
}
