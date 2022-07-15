package messages

import (
	"sync"
	"time"

	"github.com/elementsproject/peerswap/log"
)

type Messenger interface {
	SendMessage(peerId string, message []byte, messageType int) error
}

type StoppableMessenger interface {
	Messenger
	Stop()
}

type RedundantMessenger struct {
	messenger Messenger
	ticker    time.Ticker
	stop      chan struct{}
}

func NewRedundantMessenger(messenger Messenger, retryTime time.Duration) *RedundantMessenger {
	return &RedundantMessenger{
		messenger: messenger,
		ticker:    *time.NewTicker(retryTime),
		stop:      make(chan struct{}),
	}
}

func (s *RedundantMessenger) SendMessage(peerId string, message []byte, messageType int) error {
	log.Debugf("[RedundantSender]\tstart sending messages of type %d to %s\n", messageType, peerId)

	// Send one time before we go loop the send, so that we do not have to wait for the ticker.
	err := s.messenger.SendMessage(peerId, message, messageType)
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case <-s.ticker.C:
				err := s.messenger.SendMessage(peerId, message, messageType)
				if err != nil {
					log.Debugf("[RedundantSender]\tSendMessageWithRetry: %v\n", err)
				}
			case <-s.stop:
				log.Debugf("[RedundantSender]\tstop sending messages of type %d to %s\n", messageType, peerId)
				return
			}
		}
	}()

	// This function returns an error to fulfil the Messenger interface.
	return nil
}

func (s *RedundantMessenger) Stop() {
	close(s.stop)
}

type Manager struct {
	sync.Mutex
	messengers map[string]StoppableMessenger
}

func NewManager() *Manager {
	return &Manager{messengers: map[string]StoppableMessenger{}}
}

func (m *Manager) AddSender(id string, messenger StoppableMessenger) error {
	m.Lock()
	defer m.Unlock()
	if _, ok := m.messengers[id]; ok {
		return ErrAlreadyHasASender(id)
	}
	m.messengers[id] = messenger
	return nil
}

func (m *Manager) RemoveSender(id string) {
	m.Lock()
	defer m.Unlock()
	if sender, ok := m.messengers[id]; ok {
		sender.Stop()
	}
	delete(m.messengers, id)
}
