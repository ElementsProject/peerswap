package poll

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type MessengerMock struct {
	peersReceived   []string
	msgReceived     [][]byte
	msgTypeReceived []int
	called          uint
}

func (m *MessengerMock) SendMessage(peerId string, message []byte, messageType int) error {
	m.called++
	m.peersReceived = append(m.peersReceived, peerId)
	m.msgReceived = append(m.msgReceived, message)
	m.msgTypeReceived = append(m.msgTypeReceived, messageType)
	return nil
}

type PeerGetterMock struct {
	peers  []string
	called int
}

func (m *PeerGetterMock) GetPeers() []string {
	m.called++
	return m.peers
}

type PolicyMock struct {
	allowList []bool
	called    uint
}

func (m *PolicyMock) IsPeerAllowed(peerId string) bool {
	m.called++
	return m.allowList[m.called-1]
}

func TestService(t *testing.T) {
	messenger := &MessengerMock{}
	policy := &PolicyMock{allowList: []bool{true, false}}
	peerGetter := &PeerGetterMock{
		peers: []string{"peer1", "peer2"},
	}
	assets := []string{"asset1", "asset2", "asset3"}
	ps := NewPollService(500*time.Millisecond, messenger, policy, peerGetter, assets)
	for _, peer := range peerGetter.peers {
		ps.Poll(peer)
	}

	assert.Len(t, messenger.peersReceived, len(peerGetter.peers))
	assert.ElementsMatch(t, messenger.peersReceived, peerGetter.peers)

	var msgs []PollMessage
	for _, msgByte := range messenger.msgReceived {
		var msg PollMessage
		json.Unmarshal(msgByte, &msg)
		msgs = append(msgs, msg)
	}

	for i, isAllowed := range policy.allowList {
		assert.Equal(t, isAllowed, msgs[i].PeerAllowed)
		assert.Equal(t, msgs[i].Assets, assets)
	}
}
