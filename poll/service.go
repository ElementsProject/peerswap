package poll

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/elementsproject/peerswap/log"
	policy "github.com/elementsproject/peerswap/policy"
	"github.com/elementsproject/peerswap/swap"

	"github.com/elementsproject/peerswap/messages"
)

type PollNotFoundErr string

func (p PollNotFoundErr) Error() string {
	return fmt.Sprintf("poll for node %s not found", string(p))
}

type MessageSender interface {
	SendMessage(peerId string, message []byte, messageType int) error
}

type MessageReceiver interface {
	AddMessageHandler(func(peerId string, msgType string, payload []byte) error)
}

type Messenger interface {
	MessageSender
	MessageReceiver
}

type PeerGetter interface {
	GetPeers() []string
}

type Policy interface {
	IsPeerAllowed(peerId string) bool
	GetPremiumRate(peerID string, k policy.PremiumRateKind) int64
}

type Store interface {
	Update(peerId string, info PollInfo) error
	GetAll() (map[string]PollInfo, error)
	RemoveUnseen(now time.Time, olderThan time.Duration) error
}

type PollInfo struct {
	ProtocolVersion           uint64   `json:"version"`
	Assets                    []string `json:"assets"`
	BTCSwapInPremiumRatePPM   int64    `json:"btc_swap_in_premium_rate_ppm"`
	BTCSwapOutPremiumRatePPM  int64    `json:"btc_swap_out_premium_rate_ppm"`
	LBTCSwapInPremiumRatePPM  int64    `json:"lbtc_swap_in_premium_rate_ppm"`
	LBTCSwapOutPremiumRatePPM int64    `json:"lbtc_swap_out_premium_rate_ppm"`
	PeerAllowed               bool
	LastSeen                  time.Time
}
type Service struct {
	sync.RWMutex
	clock *time.Ticker
	ctx   context.Context
	done  context.CancelFunc

	assets           []string
	messenger        Messenger
	policy           Policy
	peers            PeerGetter
	store            Store
	tmpStore         map[string]string
	removeDuration   time.Duration
	loggedDisconnect map[string]struct{}
}

func NewService(tickDuration time.Duration, removeDuration time.Duration, store Store, messenger Messenger, policy Policy, peers PeerGetter, allowedAssets []string) *Service {
	clock := time.NewTicker(tickDuration)
	ctx, done := context.WithCancel(context.Background())
	s := &Service{
		clock:            clock,
		ctx:              ctx,
		done:             done,
		assets:           allowedAssets,
		messenger:        messenger,
		policy:           policy,
		peers:            peers,
		store:            store,
		tmpStore:         make(map[string]string),
		removeDuration:   removeDuration,
		loggedDisconnect: make(map[string]struct{}),
	}

	s.messenger.AddMessageHandler(s.MessageHandler)
	return s
}

// Start the poll message loop and send the poll
// messages on every tick.
func (s *Service) Start() {
	// Request fresh polls from all peers on startup
	s.RequestAllPeerPolls()
	// Start poll loop
	go func() {
		for {
			select {
			case now := <-s.clock.C:
				// remove unseen
				s.store.RemoveUnseen(now, s.removeDuration)
				// poll
				s.PollAllPeers()
			case <-s.ctx.Done():
				return
			}
		}
	}()
}

func (s *Service) Stop() {
	s.clock.Stop()
	s.done()
}

// Poll sends the POLL message to a single peer.
func (s *Service) Poll(peer string) {
	poll := PollMessage{
		Version:                   swap.PEERSWAP_PROTOCOL_VERSION,
		Assets:                    s.assets,
		PeerAllowed:               s.policy.IsPeerAllowed(peer),
		BTCSwapInPremiumRatePPM:   s.policy.GetPremiumRate(peer, policy.BtcSwapIn),
		BTCSwapOutPremiumRatePPM:  s.policy.GetPremiumRate(peer, policy.BtcSwapOut),
		LBTCSwapInPremiumRatePPM:  s.policy.GetPremiumRate(peer, policy.LbtcSwapIn),
		LBTCSwapOutPremiumRatePPM: s.policy.GetPremiumRate(peer, policy.LbtcSwapOut),
	}

	msg, err := json.Marshal(poll)
	if err != nil {
		log.Debugf("poll_service: could not marshal poll msg: %v", err)
		return
	}
	// Send the request poll message
	// the time when the poll message was received can be known by `last seen` of ListPeerswapPeers,
	// which provides some indication of the freshness of the connection with the peer.
	// Therefore, it is not necessary to handle error of each failed custom message attempt.
	_ = s.messenger.SendMessage(peer, msg, int(poll.MessageType()))
}

func (s *Service) PollAllPeers() {
	for _, peer := range s.peers.GetPeers() {
		go s.Poll(peer)
	}
}

// RequestPoll sends the REUQEST_POLL message to a
// single peer.
func (s *Service) RequestPoll(peer string) {
	request := RequestPollMessage{
		Version:                   swap.PEERSWAP_PROTOCOL_VERSION,
		Assets:                    s.assets,
		PeerAllowed:               s.policy.IsPeerAllowed(peer),
		BTCSwapInPremiumRatePPM:   s.policy.GetPremiumRate(peer, policy.BtcSwapIn),
		BTCSwapOutPremiumRatePPM:  s.policy.GetPremiumRate(peer, policy.BtcSwapOut),
		LBTCSwapInPremiumRatePPM:  s.policy.GetPremiumRate(peer, policy.LbtcSwapIn),
		LBTCSwapOutPremiumRatePPM: s.policy.GetPremiumRate(peer, policy.LbtcSwapOut),
	}

	msg, err := json.Marshal(request)
	if err != nil {
		log.Debugf("poll_service: could not marshal request_poll msg: %v", err)
		return
	}

	// Send the request poll message
	// the time when the poll message was received can be known by `last seen` of ListPeerswapPeers,
	// which provides some indication of the freshness of the connection with the peer.
	// Therefore, it is not necessary to handle error of each failed custom message attempt.
	_ = s.messenger.SendMessage(peer, msg, int(request.MessageType()))
}

// RequestAllPeerPolls requests the poll message from
// every peer.
func (s *Service) RequestAllPeerPolls() {
	for _, peer := range s.peers.GetPeers() {
		go s.RequestPoll(peer)
	}
}

// MessageHandler checks for the incoming messages
// type and takes the incoming payload to update the
// store.
func (s *Service) MessageHandler(peerID, msgType string, payload []byte) error {
	messageType, err := messages.PeerswapCustomMessageType(msgType)
	if err != nil {
		// Check for specific errors: even message type or message out of range
		// message type that peerswap is not interested in.
		if errors.Is(err, &messages.ErrNotPeerswapCustomMessage{}) {
			// These errors are expected and can be handled gracefully
			return nil
		}
		return err
	}

	switch messageType {
	case messages.MESSAGETYPE_POLL:
		var msg PollMessage
		if jerr := json.Unmarshal(payload, &msg); jerr != nil {
			return jerr
		}
		if serr := s.store.Update(peerID, PollInfo{
			ProtocolVersion:           msg.Version,
			Assets:                    msg.Assets,
			PeerAllowed:               msg.PeerAllowed,
			BTCSwapInPremiumRatePPM:   s.policy.GetPremiumRate(peerID, policy.BtcSwapIn),
			BTCSwapOutPremiumRatePPM:  s.policy.GetPremiumRate(peerID, policy.BtcSwapOut),
			LBTCSwapInPremiumRatePPM:  s.policy.GetPremiumRate(peerID, policy.LbtcSwapIn),
			LBTCSwapOutPremiumRatePPM: s.policy.GetPremiumRate(peerID, policy.LbtcSwapOut),
			LastSeen:                  time.Now(),
		}); serr != nil {
			return serr
		}
		if ti, ok := s.tmpStore[peerID]; ok {
			if ti == string(payload) {
				return nil
			}
		}
		if msg.Version != swap.PEERSWAP_PROTOCOL_VERSION {
			log.Debugf("Received poll from INCOMPATIBLE peer %s: %s", peerID, string(payload))
		} else {
			log.Debugf("Received poll from peer %s: %s", peerID, string(payload))
		}
		s.tmpStore[peerID] = string(payload)
		return nil
	case messages.MESSAGETYPE_REQUEST_POLL:
		var msg RequestPollMessage
		if jerr := json.Unmarshal(payload, &msg); jerr != nil {
			return jerr
		}
		if serr := s.store.Update(peerID, PollInfo{
			ProtocolVersion:           msg.Version,
			Assets:                    msg.Assets,
			PeerAllowed:               msg.PeerAllowed,
			BTCSwapInPremiumRatePPM:   s.policy.GetPremiumRate(peerID, policy.BtcSwapIn),
			BTCSwapOutPremiumRatePPM:  s.policy.GetPremiumRate(peerID, policy.BtcSwapOut),
			LBTCSwapInPremiumRatePPM:  s.policy.GetPremiumRate(peerID, policy.LbtcSwapIn),
			LBTCSwapOutPremiumRatePPM: s.policy.GetPremiumRate(peerID, policy.LbtcSwapOut),
			LastSeen:                  time.Now(),
		}); serr != nil {
			return serr
		}
		// Send a poll on request
		s.Poll(peerID)
		if ti, ok := s.tmpStore[peerID]; ok {
			if ti == string(payload) {
				return nil
			}
		}
		if msg.Version != swap.PEERSWAP_PROTOCOL_VERSION {
			log.Debugf("Received poll from INCOMPATIBLE peer %s: %s", peerID, string(payload))
		} else {
			log.Debugf("Received poll from peer %s: %s", peerID, string(payload))
		}
		s.tmpStore[peerID] = string(payload)
		return nil
	default:
		return nil
	}
}

func (s *Service) GetPolls() (map[string]PollInfo, error) {
	return s.store.GetAll()
}

// GetCompatiblePolls returns all polls from peers that are running the same
// protocol version.
func (s *Service) GetCompatiblePolls() (map[string]PollInfo, error) {
	var compPeers = make(map[string]PollInfo)
	peers, err := s.store.GetAll()
	if err != nil {
		return nil, err
	}
	for id, p := range peers {
		if p.ProtocolVersion == swap.PEERSWAP_PROTOCOL_VERSION {
			compPeers[id] = p
		}
	}
	return compPeers, nil
}

// GetPollFrom returns the PollInfo for a single peer with peerId. Returns a
// PollNotFoundErr if no PollInfo for the peer is present.
func (s *Service) GetPollFrom(peerId string) (*PollInfo, error) {
	polls, err := s.store.GetAll()
	if err != nil {
		return nil, err
	}

	if poll, ok := polls[peerId]; ok {
		return &poll, nil
	}

	return nil, PollNotFoundErr(peerId)
}
