package messages

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedundantSender_SendMessageWithRetry(t *testing.T) {
	type fields struct {
		messenger Messenger
		retryTime time.Duration
	}
	type args struct {
		peerId      string
		message     []byte
		messageType int
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewRedundantSender(tt.fields.messenger, tt.fields.retryTime)
			s.SendMessageWithRetry(tt.args.peerId, tt.args.message, tt.args.messageType)
		})
	}
}

func TestRedundantSender_Stop(t *testing.T) {
	type fields struct {
		messenger Messenger
		ticker    time.Ticker
		stop      chan struct{}
	}
	tests := []struct {
		name        string
		fields      fields
		shouldPanic bool
	}{
		{
			name:        "nil channel",
			fields:      fields{messenger: &MessengerStub{}, ticker: *time.NewTicker(time.Second), stop: nil},
			shouldPanic: true,
		},
		{
			name:        "non nil channel",
			fields:      fields{messenger: &MessengerStub{}, ticker: *time.NewTicker(time.Second), stop: make(chan struct{})},
			shouldPanic: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldPanic {
				defer func() {
					require.NotNil(t, recover())
				}()
			}
			s := &RedundantSender{
				messenger: tt.fields.messenger,
				ticker:    tt.fields.ticker,
				stop:      tt.fields.stop,
			}
			s.Stop()
			v, ok := <-tt.fields.stop
			t.Log(v, ok)
		})
	}
}

func TestSenderManager_AddSender(t *testing.T) {
	t.Run("add multiple sender (different id)", func(t *testing.T) {
		m := &SenderManager{sender: map[string]*RedundantSender{}}

		// Add first sender
		err := m.AddSender("my_id", NewRedundantSender(&MessengerStub{}, 1*time.Second))
		assert.Nil(t, err)

		// Add second sender, different id
		err = m.AddSender("other_id", NewRedundantSender(&MessengerStub{}, 1*time.Second))
		assert.Nil(t, err)

		// Len should be 2
		assert.Len(t, m.sender, 2)
	})

	t.Run("add multiple sender (same id)", func(t *testing.T) {
		m := &SenderManager{sender: map[string]*RedundantSender{}}

		// Add first sender
		err := m.AddSender("my_id", NewRedundantSender(&MessengerStub{}, 1*time.Second))
		assert.Nil(t, err)

		// Add second sender, same id, should fail
		expectedError := ErrAlreadyHasASender("")
		err = m.AddSender("my_id", NewRedundantSender(&MessengerStub{}, 1*time.Second))
		assert.ErrorAs(t, err, &expectedError)

		// Len should be 1
		assert.Len(t, m.sender, 1)
	})
}

func TestSendMessageWithRetry(t *testing.T) {
	tRetry := 10 * time.Millisecond
	tWait := 50 * time.Millisecond

	msgr := &MessengerStub{}
	rs := NewRedundantSender(msgr, tRetry)

	rs.SendMessageWithRetry("peer_id", []byte("canceled"), int(MESSAGETYPE_CANCELED))
	time.Sleep(tWait)
	rs.Stop()

	// Should have sent multiple messages.
	time.Sleep(tWait)
	nMsgs := msgr.Called()
	assert.Greater(t, nMsgs, 1)

	// Check it is not sending anymore.
	time.Sleep(tWait)
	assert.Equal(t, nMsgs, msgr.Called())
}

type MessengerStub struct {
	sync.Mutex
	called int
}

func (s *MessengerStub) SendMessage(peerId string, message []byte, messageType int) error {
	s.Lock()
	defer s.Unlock()
	s.called++
	return nil
}

func (s *MessengerStub) Called() int {
	s.Lock()
	defer s.Unlock()
	return s.called
}
