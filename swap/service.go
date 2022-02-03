package swap

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/sputn1ck/peerswap/messages"
)

const (
	PEERSWAP_PROTOCOL_VERSION = 2
)

var (
	AllowedAssets       = []string{"btc", "l-btc"}
	ErrSwapDoesNotExist = errors.New("swap does not exist")
)

type ErrUnknownSwapMessageType string

func (s ErrUnknownSwapMessageType) Error() string {
	return fmt.Sprintf("message type %s is unknown to peerswap", string(s))
}

type PeerNotAllowedError string

func (s PeerNotAllowedError) Error() string {
	log.Printf("unalowed request from non-allowlist peer: %s", string(s))
	return fmt.Sprintf("requests from peer %s are not allowed", string(s))
}

func ErrReceivedMessageFromUnexpectedPeer(peerId string, swapId *SwapId) error {
	return fmt.Errorf("received a message from an unexpected peer, peerId: %s, swapId: %s", peerId, swapId.String())
}

// SwapService contains the logic for swaps
type SwapService struct {
	swapServices *SwapServices

	activeSwaps    map[string]*SwapStateMachine
	BitcoinEnabled bool
	LiquidEnabled  bool

	sync.RWMutex
}

func NewSwapService(services *SwapServices) *SwapService {
	return &SwapService{
		swapServices:   services,
		activeSwaps:    map[string]*SwapStateMachine{},
		LiquidEnabled:  services.liquidEnabled,
		BitcoinEnabled: services.bitcoinEnabled,
	}
}

// Start adds callback to the messenger, txwatcher services and lightning client
func (s *SwapService) Start() error {
	s.swapServices.toService = newTimeOutService(s.createTimeoutCallback)
	s.swapServices.messenger.AddMessageHandler(s.OnMessageReceived)

	if s.LiquidEnabled {
		s.swapServices.liquidTxWatcher.AddConfirmationCallback(s.OnTxConfirmed)
		s.swapServices.liquidTxWatcher.AddCsvCallback(s.OnCsvPassed)
	}
	if s.BitcoinEnabled {
		s.swapServices.bitcoinTxWatcher.AddConfirmationCallback(s.OnTxConfirmed)
		s.swapServices.bitcoinTxWatcher.AddCsvCallback(s.OnCsvPassed)
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
func (s *SwapService) OnMessageReceived(peerId string, msgTypeString string, payload []byte) error {
	msgType, err := messages.HexStringToMessageType(msgTypeString)
	if err != nil {
		return err
	}
	msgBytes := []byte(payload)
	log.Printf("[Messenger] From: %s got msgtype: %s payload: %s", peerId, msgTypeString, payload)
	switch msgType {
	default:
		// Do nothing here, as it will spam the cln log.
		return nil
	case messages.MESSAGETYPE_SWAPOUTREQUEST:
		var msg *SwapOutRequestMessage
		err := json.Unmarshal(msgBytes, &msg)
		if err != nil {
			return err
		}
		err = s.OnSwapOutRequestReceived(msg.SwapId, peerId, msg.Asset, msg.Network, msg.Scid, msg.Pubkey, msg.Amount, msg.ProtocolVersion)
		if err != nil {
			return err
		}
	case messages.MESSAGETYPE_SWAPOUTAGREEMENT:
		var msg *SwapOutAgreementMessage
		err := json.Unmarshal(msgBytes, &msg)
		if err != nil {
			return err
		}

		// Check if sender is expected swap partner peer.
		ok, err := s.isMessageSenderExpectedPeer(peerId, msg.SwapId)
		if err != nil {
			return err
		}
		if !ok {
			return ErrReceivedMessageFromUnexpectedPeer(peerId, msg.SwapId)
		}

		err = s.OnSwapOutAgreementReceived(msg)
		if err != nil {
			return err
		}
	case messages.MESSAGETYPE_OPENINGTXBROADCASTED:
		var msg *OpeningTxBroadcastedMessage
		err := json.Unmarshal(msgBytes, &msg)
		if err != nil {
			return err
		}

		// Check if sender is expected swap partner peer.
		ok, err := s.isMessageSenderExpectedPeer(peerId, msg.SwapId)
		if err != nil {
			return err
		}
		if !ok {
			return ErrReceivedMessageFromUnexpectedPeer(peerId, msg.SwapId)
		}

		err = s.OnTxOpenedMessage(msg)
		if err != nil {
			return err
		}
	case messages.MESSAGETYPE_CANCELED:
		var msg *CancelMessage
		err := json.Unmarshal(msgBytes, &msg)
		if err != nil {
			return err
		}

		// Check if sender is expected swap partner peer.
		ok, err := s.isMessageSenderExpectedPeer(peerId, msg.SwapId)
		if err != nil {
			return err
		}
		if !ok {
			return ErrReceivedMessageFromUnexpectedPeer(peerId, msg.SwapId)
		}

		err = s.OnCancelReceived(msg.SwapId, msg)
		if err != nil {
			return err
		}
	case messages.MESSAGETYPE_SWAPINREQUEST:
		var msg *SwapInRequestMessage
		err := json.Unmarshal(msgBytes, &msg)
		if err != nil {
			return err
		}
		err = s.OnSwapInRequestReceived(peerId, msg.Pubkey, msg.Asset, msg.Network, msg.Scid, msg.SwapId, msg.Amount, msg.ProtocolVersion)
		if err != nil {
			return err
		}
	case messages.MESSAGETYPE_SWAPINAGREEMENT:
		var msg *SwapInAgreementMessage
		err := json.Unmarshal(msgBytes, &msg)
		if err != nil {
			return err
		}

		// Check if sender is expected swap partner peer.
		ok, err := s.isMessageSenderExpectedPeer(peerId, msg.SwapId)
		if err != nil {
			return err
		}
		if !ok {
			return ErrReceivedMessageFromUnexpectedPeer(peerId, msg.SwapId)
		}

		err = s.OnAgreementReceived(msg)
		if err != nil {
			return err
		}
	case messages.MESSAGETYPE_COOPCLOSE:
		var msg *CoopCloseMessage
		err := json.Unmarshal(msgBytes, &msg)
		if err != nil {
			return err
		}

		// Check if sender is expected swap partner peer.
		ok, err := s.isMessageSenderExpectedPeer(peerId, msg.SwapId)
		if err != nil {
			return err
		}
		if !ok {
			return ErrReceivedMessageFromUnexpectedPeer(peerId, msg.SwapId)
		}

		err = s.OnCoopCloseReceived(msg.SwapId, msg)
		if err != nil {
			return err
		}
	}
	return nil
}

// OnTxConfirmed sends the txconfirmed event to the corresponding swap
func (s *SwapService) OnTxConfirmed(swapId string, txHex string) error {
	swap, err := s.GetActiveSwap(swapId)
	if err != nil {
		return err
	}
	// todo move to eventctx
	swap.Data.OpeningTxHex = txHex
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

// OnCsvPassed sends the csvpassed event to the corresponding swap
func (s *SwapService) OnCsvPassed(swapId string) error {
	swap, err := s.GetActiveSwap(swapId)
	if err != nil {
		return err
	}
	done, err := swap.SendEvent(Event_OnCsvPassed, nil)
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

// SwapOut starts a new swap out process
func (s *SwapService) SwapOut(peer string, asset string, channelId string, initiator string, amount uint64) (*SwapStateMachine, error) {
	if s.hasActiveSwapOnChannel(channelId) {
		return nil, fmt.Errorf("already has an active swap on channel")
	}
	var bitcoinNetwork string
	var elementsAsset string
	if asset == l_btc_asset {
		elementsAsset = s.swapServices.liquidWallet.GetAsset()
	} else if asset == btc_asset {
		bitcoinNetwork = s.swapServices.bitcoinWallet.GetNetwork()
	}
	log.Printf("[SwapService] Start swapping out: peer: %s chanId: %s initiator: %s amount %v", peer, channelId, initiator, amount)
	swap := newSwapOutSenderFSM(s.swapServices)
	s.AddActiveSwap(swap.Id, swap)
	done, err := swap.SendEvent(Event_OnSwapOutStarted, &SwapCreationContext{
		amount:          amount,
		asset:           asset,
		initiatorId:     initiator,
		peer:            peer,
		channelId:       channelId,
		swapId:          swap.SwapId,
		bitcoinNetwork:  bitcoinNetwork,
		elementsAsset:   elementsAsset,
		id:              swap.Id,
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
	var bitcoinNetwork string
	var elementsAsset string
	if asset == l_btc_asset {
		elementsAsset = s.swapServices.liquidWallet.GetAsset()
	} else if asset == btc_asset {
		bitcoinNetwork = s.swapServices.bitcoinWallet.GetNetwork()
	}
	swap := newSwapInSenderFSM(s.swapServices)
	s.AddActiveSwap(swap.Id, swap)
	done, err := swap.SendEvent(Event_SwapInSender_OnSwapInRequested, &SwapCreationContext{
		amount:          amount,
		asset:           asset,
		initiatorId:     initiator,
		peer:            peer,
		channelId:       channelId,
		swapId:          swap.SwapId,
		id:              swap.Id,
		bitcoinNetwork:  bitcoinNetwork,
		elementsAsset:   elementsAsset,
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
func (s *SwapService) OnSwapInRequestReceived(peer, makerPubkey, elementsAsset, bitcoinNetwork, channelId string, swapId *SwapId, amount, protocolversion uint64) error {
	// check if a swap is already active on the channel
	if s.hasActiveSwapOnChannel(channelId) {
		return fmt.Errorf("already has an active swap on channel")
	}

	swap := newSwapInReceiverFSM(swapId, s.swapServices)
	s.AddActiveSwap(swapId.String(), swap)

	done, err := swap.SendEvent(Event_SwapInReceiver_OnRequestReceived, &CreateSwapFromRequestContext{
		amount:          amount,
		elementsAsset:   elementsAsset,
		bitcoinNetwork:  bitcoinNetwork,
		peer:            peer,
		channelId:       channelId,
		swapId:          swapId,
		id:              swapId.String(),
		protocolversion: protocolversion,
		makerPubkey:     makerPubkey,
	})
	if done {
		s.RemoveActiveSwap(swap.Id)
	}
	return err
}

// OnSwapInRequestReceived creates a new swap-out process and sends the event to the swap statemachine
func (s *SwapService) OnSwapOutRequestReceived(swapId *SwapId, peer, elementsAsset, bitcoinNetwork, channelId, takerPubkeyHash string, amount, protocolversion uint64) error {
	// check if a swap is already active on the channel
	if s.hasActiveSwapOnChannel(channelId) {
		return fmt.Errorf("already has an active swap on channel")
	}

	swap := newSwapOutReceiverFSM(swapId, s.swapServices)
	s.AddActiveSwap(swapId.String(), swap)
	done, err := swap.SendEvent(Event_OnSwapOutRequestReceived, &CreateSwapFromRequestContext{
		amount:          amount,
		elementsAsset:   elementsAsset,
		bitcoinNetwork:  bitcoinNetwork,
		peer:            peer,
		channelId:       channelId,
		swapId:          swapId,
		id:              swapId.String(),
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
	swap, err := s.GetActiveSwap(msg.SwapId.String())
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

// OnSwapOutAgreementReceived sends the FeeInvoiceReceived event to the corresponding swap state machine
func (s *SwapService) OnSwapOutAgreementReceived(message *SwapOutAgreementMessage) error {
	swap, err := s.GetActiveSwap(message.SwapId.String())
	if err != nil {
		return err
	}
	done, err := swap.SendEvent(Event_OnFeeInvoiceReceived, message)
	if err != nil {
		return err
	}
	if done {
		s.RemoveActiveSwap(swap.Id)
	}
	return nil
}

// OnFeeInvoicePaid sends the FeeInvoicePaid event to the corresponding swap state machine
func (s *SwapService) OnFeeInvoicePaid(swapId *SwapId) error {
	swap, err := s.GetActiveSwap(swapId.String())
	if err != nil {
		return err
	}
	done, err := swap.SendEvent(Event_OnFeeInvoicePaid, nil)
	if err != nil {
		return err
	}
	if done {
		s.RemoveActiveSwap(swap.Id)
	}
	return nil
}

// OnClaimInvoicePaid sends the ClaimInvoicePaid event to the corresponding swap state machine
func (s *SwapService) OnClaimInvoicePaid(swapId *SwapId) error {
	swap, err := s.GetActiveSwap(swapId.String())
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

// OnTxOpenedMessage sends the TxOpenedMessage event to the corresponding swap state machine
func (s *SwapService) OnTxOpenedMessage(message *OpeningTxBroadcastedMessage) error {
	swap, err := s.GetActiveSwap(message.SwapId.String())
	if err != nil {
		return err
	}
	done, err := swap.SendEvent(Event_OnTxOpenedMessage, message)
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

const PaymentLabelSeparator = "_"

func getPaymentLabel(description string) string {
	parts := strings.SplitN(description, PaymentLabelSeparator, 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[0]
}

// OnPayment handles incoming payments and if it corresponds to a claim or
// fee invoice passes the dater to the corresponding function
func (s *SwapService) OnPayment(description string) {
	// Check for claim_ label
	switch getPaymentLabel(description) {
	case "fee":
		swapId, err := ParseSwapIdFromString(description[4:])
		if err != nil {
			log.Printf("[SwapService] Error parsing id from invoice: %v", err)
			return
		}
		if err := s.OnFeeInvoicePaid(swapId); err != nil {
			log.Printf("[SwapService] Error OnFeeInvoicePaid: %v", err)
			return
		}
	case "claim":
		swapId, err := ParseSwapIdFromString(description[6:])
		if err != nil {
			log.Printf("[SwapService] Error parsing id from invoice: %v", err)
			return
		}
		if err := s.OnClaimInvoicePaid(swapId); err != nil {
			log.Printf("[SwapService] Error OnClaimInvoicePaid: %v", err)
			return
		}
	default:
		return
	}
}

// OnCancelReceived sends the CancelReceived event to the corresponding swap state machine
func (s *SwapService) OnCancelReceived(swapId *SwapId, cancelMsg *CancelMessage) error {
	swap, err := s.GetActiveSwap(swapId.String())
	if err != nil {
		return err
	}
	done, err := swap.SendEvent(Event_OnCancelReceived, cancelMsg)
	if err != nil {
		return err
	}
	if done {
		s.RemoveActiveSwap(swap.Id)
	}
	return nil
}

// OnCoopCloseReceived sends the CoopMessage event to the corresponding swap state mahcine
func (s *SwapService) OnCoopCloseReceived(swapId *SwapId, coopCloseMessage *CoopCloseMessage) error {
	swap, err := s.GetActiveSwap(swapId.String())
	if err != nil {
		return err
	}
	done, err := swap.SendEvent(Event_OnCoopCloseReceived, coopCloseMessage)
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

func (s *SwapService) ResendLastMessage(swapId string) error {
	swap, err := s.GetActiveSwap(swapId)
	if err != nil {
		return err
	}
	action := &SendMessageAction{}
	event := action.Execute(s.swapServices, swap.Data)
	if event == Event_ActionFailed {
		return swap.Data.LastErr
	}
	return nil
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
	s.RLock()
	defer s.RUnlock()
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
	s.RLock()
	defer s.RUnlock()
	for _, swap := range s.activeSwaps {
		if swap.Data.Scid == channelId {
			return true
		}
	}

	return false
}

type WrongAssetError string

func (e WrongAssetError) Error() string {
	return fmt.Sprintf("unallowed asset: %s", string(e))
}

// isMessageSenderExpectedPeer returns true if the senderId matches the
// PeerNodeId of the swap, false if not.
func (s *SwapService) isMessageSenderExpectedPeer(senderId string, swapId *SwapId) (bool, error) {
	swap, err := s.GetActiveSwap(swapId.String())
	if err != nil {
		return false, err
	}
	return swap.Data.PeerNodeId == senderId, nil
}

func (s *SwapService) createTimeoutCallback(swapId string) func() {
	return func() {
		swap, err := s.GetActiveSwap(swapId)
		if err != nil {
			log.Printf("[SwapService]\ttimeout callback: %v", err)
			return
		}

		// Reset cancel func
		swap.Data.toCancel = nil

		done, err := swap.SendEvent(Event_OnTimeout, nil)
		if err != nil {
			log.Printf("[SwapService]\tSendEvent(): %v", err)
			return
		}

		if done {
			s.RemoveActiveSwap(swap.Id)
		}
	}
}
