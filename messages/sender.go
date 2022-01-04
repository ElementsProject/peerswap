package messages

import (
	"log"
	"sync"
	"time"
)

type Messenger interface {
	SendMessage(peerId string, message []byte, messageType int) error
}

type RedundantSender struct {
	messenger Messenger
	ticker    time.Ticker
	stop      chan struct{}
}

func NewRedundantSender(messenger Messenger, retryTime time.Duration) *RedundantSender {
	return &RedundantSender{
		messenger: messenger,
		ticker:    *time.NewTicker(retryTime),
		stop:      make(chan struct{}),
	}
}

func (s *RedundantSender) SendMessageWithRetry(peerId string, message []byte, messageType int) {
	log.Printf("[RedundantSender]\tstart sending messages of type %d to %s\n", messageType, peerId)
	go func() {
		for {
			select {
			case <-s.ticker.C:
				err := s.messenger.SendMessage(peerId, message, messageType)
				if err != nil {
					log.Printf("[RedundantSender]\tSendMessageWithRetry: %v\n", err)
				}
			case <-s.stop:
				log.Printf("[RedundantSender]\tstop sending messages of type %d to %s\n", messageType, peerId)
				return
			}
		}
	}()
}

func (s *RedundantSender) Stop() {
	close(s.stop)
}

type SenderManager struct {
	sync.Mutex
	sender map[string]*RedundantSender
}

func (m *SenderManager) AddSender(id string, sender *RedundantSender) error {
	m.Lock()
	defer m.Unlock()
	if _, ok := m.sender[id]; ok {
		return ErrAlreadyHasASender(id)
	}
	m.sender[id] = sender
	return nil
}

func (m *SenderManager) RemoveSender(id string) {
	m.Lock()
	defer m.Unlock()
	if sender, ok := m.sender[id]; ok {
		sender.Stop()
	}
	delete(m.sender, id)
}
