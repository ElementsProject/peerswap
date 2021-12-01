package poll

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/sputn1ck/peerswap/messages"
)

const version uint64 = 0

type MessageSender interface {
	SendMessage(peerId string, message []byte, messageType int) error
}

type MessageReceiver interface {
	AddMessageHandler(func(peerId string, msgType string, payload string) error)
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
}

type Store interface {
	Update(peerId string, info PollInfo) error
	GetAll() (map[string]PollInfo, error)
	RemoveUnseen(olderThan time.Duration) error
}

type PollInfo struct {
	Assets      []string `json:"assets"`
	PeerAllowed bool
	LastSeen    time.Time
}
type Service struct {
	sync.RWMutex
	clock *time.Ticker
	ctx   context.Context
	done  context.CancelFunc

	assets         []string
	messenger      Messenger
	policy         Policy
	peers          PeerGetter
	store          Store
	removeDuration time.Duration
}

func NewService(tickDuration time.Duration, removeDuration time.Duration, store Store, messenger Messenger, policy Policy, peers PeerGetter, allowedAssets []string) *Service {
	clock := time.NewTicker(tickDuration)
	ctx, done := context.WithCancel(context.Background())
	s := &Service{
		clock:          clock,
		ctx:            ctx,
		done:           done,
		assets:         allowedAssets,
		messenger:      messenger,
		policy:         policy,
		peers:          peers,
		store:          store,
		removeDuration: removeDuration,
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
			case <-s.clock.C:
				// remove unseen
				s.store.RemoveUnseen(s.removeDuration)
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
		Version:     version,
		Assets:      s.assets,
		PeerAllowed: s.policy.IsPeerAllowed(peer),
	}

	msg, err := json.Marshal(poll)
	if err != nil {
		log.Printf("poll_service: could not marshal poll msg: %v", err)
		return
	}

	if err := s.messenger.SendMessage(peer, msg, int(poll.MessageType())); err != nil {
		log.Printf("poll_service: could not send poll msg: %v", err)
	}
}

func (s *Service) PollAllPeers() {
	log.Println("poll peers")
	for _, peer := range s.peers.GetPeers() {
		go s.Poll(peer)
	}
}

// RequestPoll sends the REUQEST_POLL message to a
// single peer.
func (s *Service) RequestPoll(peer string) {
	request := RequestPollMessage{
		Version:     version,
		Assets:      s.assets,
		PeerAllowed: s.policy.IsPeerAllowed(peer),
	}

	msg, err := json.Marshal(request)
	if err != nil {
		log.Printf("poll_service: could not marshal request_poll msg: %v", err)
		return
	}

	if err := s.messenger.SendMessage(peer, msg, int(request.MessageType())); err != nil {
		log.Printf("poll_service: could not send request_poll msg: %v", err)
	}
}

// RequestAllPeerPolls requests the poll message from
// every peer.
func (s *Service) RequestAllPeerPolls() {
	for _, peer := range s.peers.GetPeers() {
		go s.RequestPoll(peer)
	}
}

// MessageHandler checks for the incomming messages
// type and takes the incomming payload to update the
// store.
func (s *Service) MessageHandler(peerId string, msgType string, payload string) error {
	messageType, err := messages.HexStringToMessageType(msgType)
	if err != nil {
		return err
	}

	switch messageType {
	case messages.MESSAGETYPE_POLL:
		var msg PollMessage
		err = json.Unmarshal([]byte(payload), &msg)
		if err != nil {
			return err
		}
		s.store.Update(peerId, PollInfo{
			Assets:      msg.Assets,
			PeerAllowed: msg.PeerAllowed,
			LastSeen:    time.Now(),
		})
		return nil
	case messages.MESSAGETYPE_REQUEST_POLL:
		var msg RequestPollMessage
		err = json.Unmarshal([]byte(payload), &msg)
		if err != nil {
			return err
		}
		s.store.Update(peerId, PollInfo{
			Assets:      msg.Assets,
			PeerAllowed: msg.PeerAllowed,
			LastSeen:    time.Now(),
		})
		// Send a poll on request
		s.Poll(peerId)
		return nil
	default:
		return nil
	}
}

func (s *Service) GetPolls() (map[string]PollInfo, error) {
	return s.store.GetAll()
}
