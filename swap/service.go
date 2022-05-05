package swap

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/elementsproject/peerswap/log"

	"github.com/elementsproject/peerswap/messages"
)

const (
	PEERSWAP_PROTOCOL_VERSION = 2
	MINIMUM_SWAP_SIZE         = uint64(100000)
)

var (
	AllowedAssets       = []string{"btc", "lbtc"}
	ErrSwapDoesNotExist = errors.New("swap does not exist")
	ErrMinimumSwapSize  = fmt.Errorf("requiring minimum swap size of %v", MINIMUM_SWAP_SIZE)
)

type ErrUnknownSwapMessageType string

func (s ErrUnknownSwapMessageType) Error() string {
	return fmt.Sprintf("message type %s is unknown to peerswap", string(s))
}

type PeerNotAllowedError string

func (s PeerNotAllowedError) Error() string {
	return fmt.Sprintf("requests from peer %s are not allowed", string(s))
}

func ErrReceivedMessageFromUnexpectedPeer(peerId string, swapId *SwapId) error {
	return fmt.Errorf("received a message from an unexpected peer, peerId: %s, swapId: %s", peerId, swapId.String())
}

// SwapService contains the logic for swaps
type SwapService struct {
	swapServices *SwapServices

	activeSwaps       map[string]*SwapStateMachine
	BitcoinEnabled    bool
	LiquidEnabled     bool
	allowSwapRequests bool
	sync.RWMutex
}

func NewSwapService(services *SwapServices) *SwapService {
	return &SwapService{
		swapServices:      services,
		activeSwaps:       map[string]*SwapStateMachine{},
		LiquidEnabled:     services.liquidEnabled,
		BitcoinEnabled:    services.bitcoinEnabled,
		allowSwapRequests: true,
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

func (s *SwapService) HasActiveSwaps() (bool, error) {
	swaps, err := s.swapServices.swapStore.ListAll()
	if err != nil {
		return false, err
	}
	for _, swap := range swaps {
		if !swap.IsFinished() {
			return true, nil
		}
	}
	return false, nil
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
	if len(payload) > 100*1024 {
		return errors.New("Payload is unexpectedly large")
	}
	msgType, err := messages.HexStringToMessageType(msgTypeString)
	if err != nil {
		return err
	}
	msgBytes := []byte(payload)
	log.Debugf("[Messenger] From: %s got msgtype: %s payload: %s", peerId, msgTypeString, payload)
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
		err = s.OnSwapOutRequestReceived(msg.SwapId, peerId, msg)
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
		err = s.OnSwapInRequestReceived(msg.SwapId, peerId, msg)
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

		err = s.OnSwapInAgreementReceived(msg)
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

// todo move wallet and chain / channel validation logic here
// SwapOut starts a new swap out process
func (s *SwapService) SwapOut(peer string, chain string, channelId string, initiator string, amount uint64) (*SwapStateMachine, error) {
	if s.hasActiveSwapOnChannel(channelId) {
		return nil, fmt.Errorf("already has an active swap on channel")
	}

	if !s.allowSwapRequests {
		return nil, fmt.Errorf("peerswap set to reject all swaps")
	}

	if amount < MINIMUM_SWAP_SIZE {
		return nil, ErrMinimumSwapSize
	}

	swap := newSwapOutSenderFSM(s.swapServices, initiator, peer)
	s.AddActiveSwap(swap.Id, swap)

	var bitcoinNetwork string
	var elementsAsset string
	if chain == l_btc_chain {
		elementsAsset = s.swapServices.liquidWallet.GetAsset()
	} else if chain == btc_chain {
		bitcoinNetwork = s.swapServices.bitcoinWallet.GetNetwork()
	} else {
		return nil, errors.New("invalid chain")
	}

	request := &SwapOutRequestMessage{
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
		SwapId:          swap.SwapId,
		Asset:           elementsAsset,
		Network:         bitcoinNetwork,
		Scid:            channelId,
		Amount:          amount,
		Pubkey:          hex.EncodeToString(swap.Data.GetPrivkey().PubKey().SerializeCompressed()),
	}

	done, err := swap.SendEvent(Event_OnSwapOutStarted, request)
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
func (s *SwapService) SwapIn(peer string, chain string, channelId string, initiator string, amount uint64) (*SwapStateMachine, error) {
	if s.hasActiveSwapOnChannel(channelId) {
		return nil, fmt.Errorf("already has an active swap on channel")
	}
	if !s.allowSwapRequests {
		return nil, fmt.Errorf("peerswap set to reject all swaps")
	}

	if amount < MINIMUM_SWAP_SIZE {
		return nil, ErrMinimumSwapSize
	}

	var bitcoinNetwork string
	var elementsAsset string
	if chain == l_btc_chain {
		elementsAsset = s.swapServices.liquidWallet.GetAsset()
	} else if chain == btc_chain {
		bitcoinNetwork = s.swapServices.bitcoinWallet.GetNetwork()
	} else {
		return nil, errors.New("invalid chain")
	}
	swap := newSwapInSenderFSM(s.swapServices, initiator, peer)
	s.AddActiveSwap(swap.Id, swap)

	request := &SwapInRequestMessage{
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
		SwapId:          swap.SwapId,
		Asset:           elementsAsset,
		Network:         bitcoinNetwork,
		Scid:            channelId,
		Amount:          amount,
		Pubkey:          hex.EncodeToString(swap.Data.GetPrivkey().PubKey().SerializeCompressed()),
	}

	done, err := swap.SendEvent(Event_SwapInSender_OnSwapInRequested, request)
	if err != nil {
		return nil, err
	}
	if done {
		s.RemoveActiveSwap(swap.Id)
	}
	return swap, nil
}

// OnSwapInRequestReceived creates a new swap-in process and sends the event to the swap statemachine
func (s *SwapService) OnSwapInRequestReceived(swapId *SwapId, peerId string, message *SwapInRequestMessage) error {
	// check if a swap is already active on the channel
	if s.hasActiveSwapOnChannel(message.Scid) {
		return fmt.Errorf("already has an active swap on channel")
	}

	if !s.allowSwapRequests {
		return fmt.Errorf("rejecting all swaps")
	}
	swap := newSwapInReceiverFSM(swapId, s.swapServices, peerId)
	s.AddActiveSwap(swapId.String(), swap)

	done, err := swap.SendEvent(Event_SwapInReceiver_OnRequestReceived, message)
	if done {
		s.RemoveActiveSwap(swap.Id)
	}
	return err
}

// OnSwapInRequestReceived creates a new swap-out process and sends the event to the swap statemachine
func (s *SwapService) OnSwapOutRequestReceived(swapId *SwapId, peerId string, message *SwapOutRequestMessage) error {
	// check if a swap is already active on the channel
	if s.hasActiveSwapOnChannel(message.Scid) {
		return fmt.Errorf("already has an active swap on channel")
	}

	if !s.allowSwapRequests {
		return fmt.Errorf("rejecting all swaps")
	}
	swap := newSwapOutReceiverFSM(swapId, s.swapServices, peerId)

	s.AddActiveSwap(swapId.String(), swap)

	done, err := swap.SendEvent(Event_OnSwapOutRequestReceived, message)
	if err != nil {
		return err
	}
	if done {
		s.RemoveActiveSwap(swap.Id)
	}
	return nil
}

// OnSwapInAgreementReceived sends the agreementreceived event to the corresponding swap state machine
func (s *SwapService) OnSwapInAgreementReceived(msg *SwapInAgreementMessage) error {
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

// OnFeeInvoiceNotification sends the FeeInvoicePaid event to the corresponding swap state machine
func (s *SwapService) OnFeeInvoiceNotification(swapId *SwapId) error {
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

// OnClaimInvoiceNotification sends the ClaimInvoicePaid event to the corresponding swap state machine
func (s *SwapService) OnClaimInvoiceNotification(swapId *SwapId) error {
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
func (s *SwapService) OnPayment(swapIdStr string, invoiceType InvoiceType) {
	swapId, err := ParseSwapIdFromString(swapIdStr)
	if err != nil {
		log.Infof("parse swapId error")
		return
	}

	// Check for claim_ label
	switch invoiceType {
	case INVOICE_FEE:
		if err := s.OnFeeInvoiceNotification(swapId); err != nil {
			log.Infof("[SwapService] Error OnFeeInvoiceNotification: %v", err)
			return
		}
	case INVOICE_CLAIM:
		if err := s.OnClaimInvoiceNotification(swapId); err != nil {
			log.Infof("[SwapService] Error OnClaimInvoiceNotification: %v", err)
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

func (s *SwapService) ListActiveSwaps() ([]*SwapStateMachine, error) {
	swaps, err := s.swapServices.swapStore.ListAll()
	if err != nil {
		return nil, err
	}

	activeSwaps := []*SwapStateMachine{}

	for _, swap := range swaps {
		if swap.IsFinished() {
			continue
		}
		activeSwaps = append(activeSwaps, swap)
	}
	return activeSwaps, nil
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
		if swap.Data.GetScid() == channelId {
			return true
		}
	}

	return false
}

func (s *SwapService) SetAllowSwapRequests(allow bool) bool {
	s.allowSwapRequests = allow
	return s.allowSwapRequests
}

func (s *SwapService) GetAllowSwapRequests() bool {
	return s.allowSwapRequests
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
		if err == ErrSwapDoesNotExist {
			return
		}
		if err != nil {
			log.Debugf("[SwapService] timeout callback: %v", err)
			return
		}

		// Reset cancel func
		if swap != nil && swap.Data != nil {
			swap.Data.toCancel = nil
		}

		done, err := swap.SendEvent(Event_OnTimeout, nil)
		if err == ErrEventRejected {
			return
		}
		if err != nil {
			log.Debugf("[SwapService] SendEvent(): %v", err)
			return
		}

		if done {
			s.RemoveActiveSwap(swap.Id)
		}
	}
}
