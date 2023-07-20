package poll

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/elementsproject/peerswap/log"
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

type Policy interface {
	IsPeerAllowed(peerId string) bool
}

type Store interface {
	Update(peerId string, info PollInfo) error
	GetAll() (map[string]PollInfo, error)
	RemoveUnseen(now time.Time, olderThan time.Duration) error
}

type PollInfo struct {
	ProtocolVersion uint64   `json:"version"`
	Assets          []string `json:"assets"`
	PeerAllowed     bool
	LastSeen        time.Time
}
type Service struct {
	sync.RWMutex
	clock *time.Ticker
	ctx   context.Context
	done  context.CancelFunc

	assets           []string
	messenger        Messenger
	policy           Policy
	store            Store
	tmpStore         map[string]string
	removeDuration   time.Duration
	loggedDisconnect map[string]struct{}

	// pollDelta is the duration between two consecutive messages to
	// be sent to a peer.
	pollDelta time.Duration

	// peerList is a list of all known peers to the node. This
	// includes peers that don't support peerswap.
	peerList  map[string]struct{}
	pollQueue pollQueue
}

func NewService(tickDuration time.Duration, removeDuration time.Duration, store Store, messenger Messenger, policy Policy, allowedAssets []string) *Service {
	clock := time.NewTicker(tickDuration)
	ctx, done := context.WithCancel(context.Background())
	s := &Service{
		clock:            clock,
		ctx:              ctx,
		done:             done,
		assets:           allowedAssets,
		messenger:        messenger,
		policy:           policy,
		store:            store,
		tmpStore:         make(map[string]string),
		removeDuration:   removeDuration,
		loggedDisconnect: make(map[string]struct{}),
		pollDelta:        1 * time.Hour,
		peerList:         make(map[string]struct{}),
		pollQueue:        pollQueue{},
	}

	s.messenger.AddMessageHandler(s.MessageHandler)
	return s
}

// Start the poll message loop and send the poll
// messages on every tick.
func (s *Service) Start() {
	go func() {
		for {
			select {
			case now := <-s.clock.C:
				// On every tick we check if we need to send out the next poll
				// message. This is determined by a timestamp telling us when to
				// send out the next message. Therefore we peek into the next
				// element of our queue.
				next, ok := s.pollQueue.Peek()

				if ok && now.After(next.ts) {
					// We passed the time for the next peer to be polled.
					_, _ = s.pollQueue.Dequeue()
					s.Poll(next.peer)
					_ = s.store.RemoveUnseen(now, s.removeDuration)

					// If the peer is still in our list, we re-enqueue the peer
					// so that we sent a poll message again when the newly set
					// timestamp passes.
					s.RLock()
					if _, ok := s.peerList[next.peer]; ok {
						s.pollQueue.Enqueue(nextPeer{ts: now.Add(s.pollDelta), peer: next.peer})
					}
					s.RUnlock()
				}
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
		Version:     swap.PEERSWAP_PROTOCOL_VERSION,
		Assets:      s.assets,
		PeerAllowed: s.policy.IsPeerAllowed(peer),
	}

	msg, err := json.Marshal(poll)
	if err != nil {
		log.Debugf("poll_service: could not marshal poll msg: %v", err)
		return
	}

	s.sendMessage(peer, msg, int(poll.MessageType()))
}

// RequestPoll sends the REUQEST_POLL message to a
// single peer.
func (s *Service) RequestPoll(peer string) {
	request := RequestPollMessage{
		Version:     swap.PEERSWAP_PROTOCOL_VERSION,
		Assets:      s.assets,
		PeerAllowed: s.policy.IsPeerAllowed(peer),
	}

	msg, err := json.Marshal(request)
	if err != nil {
		log.Debugf("poll_service: could not marshal request_poll msg: %v", err)
		return
	}

	s.sendMessage(peer, msg, int(request.MessageType()))
}

// MessageHandler checks for the incoming messages
// type and takes the incoming payload to update the
// store.
func (s *Service) MessageHandler(peerId string, msgType string, payload []byte) error {
	messageType, err := messages.HexStringToMessageType(msgType)
	if err != nil {
		return err
	}

	switch messageType {
	case messages.MESSAGETYPE_POLL:
		var msg PollMessage
		err = json.Unmarshal(payload, &msg)
		if err != nil {
			return err
		}
		s.store.Update(peerId, PollInfo{
			ProtocolVersion: msg.Version,
			Assets:          msg.Assets,
			PeerAllowed:     msg.PeerAllowed,
			LastSeen:        time.Now(),
		})
		if ti, ok := s.tmpStore[peerId]; ok {
			if ti == string(payload) {
				return nil
			}
		}
		if msg.Version != swap.PEERSWAP_PROTOCOL_VERSION {
			log.Debugf("Received poll from INCOMPATIBLE peer %s: %s", peerId, string(payload))
		} else {
			log.Debugf("Received poll from peer %s: %s", peerId, string(payload))
		}
		s.tmpStore[peerId] = string(payload)
		return nil
	case messages.MESSAGETYPE_REQUEST_POLL:
		var msg RequestPollMessage
		err = json.Unmarshal([]byte(payload), &msg)
		if err != nil {
			return err
		}
		s.store.Update(peerId, PollInfo{
			ProtocolVersion: msg.Version,
			Assets:          msg.Assets,
			PeerAllowed:     msg.PeerAllowed,
			LastSeen:        time.Now(),
		})
		// Send a poll on request
		s.Poll(peerId)
		if ti, ok := s.tmpStore[peerId]; ok {
			if ti == string(payload) {
				return nil
			}
		}
		if msg.Version != swap.PEERSWAP_PROTOCOL_VERSION {
			log.Debugf("Received poll from INCOMPATIBLE peer %s: %s", peerId, string(payload))
		} else {
			log.Debugf("Received poll from peer %s: %s", peerId, string(payload))
		}
		s.tmpStore[peerId] = string(payload)
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

func (s *Service) sendMessage(peer string, msg []byte, msgType int) {
	if err := s.messenger.SendMessage(peer, msg, msgType); err != nil {
		s.Lock()
		defer s.Unlock()
		// Only log message if not already logged an error on this peer. Mostly
		// these errors will deal with disconnected peers so there is no need to
		// continue logging if the peer is 'still' disconnected.
		if _, seen := s.loggedDisconnect[peer]; !seen {
			log.Debugf("poll_service: could not send msg to %s: %v", peer, err)
			s.loggedDisconnect[peer] = struct{}{}
		} else {
			s.Lock()
			defer s.Unlock()
			// Message could be sent. Release peer from `loggedDisconnect`.
			delete(s.loggedDisconnect, peer)
		}
	}
}

// AddPeer adds a new peer to the polling service. This includes adding the
// peer to the peer list and the message queue. It also sends out the initial
// poll.
func (s *Service) AddPeer(peer string) {
	s.Lock()
	defer s.Unlock()
	if _, ok := s.peerList[peer]; !ok {
		s.peerList[peer] = struct{}{}
		s.Poll(peer)
		s.pollQueue.Enqueue(nextPeer{ts: time.Now().Add(s.pollDelta), peer: peer})
	}
}

// RemovePeer removes a peer from the peer list. A peer that is not on the peer
// list will not be enqueued again, the next time a poll is sent.
func (s *Service) RemovePeer(peer string) {
	s.Lock()
	defer s.Unlock()
	delete(s.peerList, peer)
}
