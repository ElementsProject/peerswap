package policy

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Create(t *testing.T) {
	// check if all variables are set
	// check default variables

	policy, err := create(strings.NewReader(""))
	assert.NoError(t, err)
	assert.EqualValues(t, &Policy{
		ReserveOnchainMsat: defaultReserveOnchainMsat,
		PeerWhitelist:      defaultPeerWhitelist,
		AcceptAllPeers:     defaultAcceptAllPeers,
	}, policy)

	peer1 := "123"
	peer2 := "345"
	accept := true
	var acceptInt int8
	if accept {
		acceptInt = 1
	}

	conf := fmt.Sprintf("accept_all_peers=%d\nwhitelisted_peers=%s\nwhitelisted_peers=%s", acceptInt, peer1, peer2)

	policy2, err := create(strings.NewReader(conf))
	assert.NoError(t, err)
	assert.EqualValues(t, &Policy{
		ReserveOnchainMsat: defaultReserveOnchainMsat,
		PeerWhitelist:      []string{peer1, peer2},
		AcceptAllPeers:     accept,
	}, policy2)
}

func Test_Reload(t *testing.T) {
	peer1 := "123"
	peer2 := "345"
	accept := true
	var acceptInt int8
	if accept {
		acceptInt = 1
	}

	conf := fmt.Sprintf("accept_all_peers=%d\nwhitelisted_peers=%s\nwhitelisted_peers=%s", acceptInt, peer1, peer2)

	policy, err := create(strings.NewReader(conf))
	assert.NoError(t, err)
	assert.EqualValues(t, &Policy{
		ReserveOnchainMsat: defaultReserveOnchainMsat,
		PeerWhitelist:      []string{peer1, peer2},
		AcceptAllPeers:     accept,
	}, policy)

	newPeer := "new_peer"
	newConf := fmt.Sprintf("whitelisted_peers=%s", newPeer)

	err = policy.reload(strings.NewReader(newConf))
	assert.NoError(t, err)
	assert.EqualValues(t, &Policy{
		ReserveOnchainMsat: defaultReserveOnchainMsat,
		PeerWhitelist:      []string{newPeer},
		AcceptAllPeers:     defaultAcceptAllPeers,
	}, policy)
}

func Test_Reload_NoOverrideOnError(t *testing.T) {
	peer1 := "123"
	peer2 := "345"
	accept := true
	var acceptInt int8
	if accept {
		acceptInt = 1
	}

	conf := fmt.Sprintf("accept_all_peers=%d\nwhitelisted_peers=%s\nwhitelisted_peers=%s", acceptInt, peer1, peer2)

	policy, err := create(strings.NewReader(conf))
	assert.NoError(t, err)
	assert.EqualValues(t, &Policy{
		ReserveOnchainMsat: defaultReserveOnchainMsat,
		PeerWhitelist:      []string{peer1, peer2},
		AcceptAllPeers:     accept,
	}, policy)

	// copy policy
	oldPolicy := &Policy{}
	*oldPolicy = *policy

	// Malformed config string
	newConf := "this_is_unknown:3"

	err = policy.reload(strings.NewReader(newConf))
	assert.Error(t, err)

	// assert policy did not change
	assert.EqualValues(t, oldPolicy, policy)
}

func Test_IsPeerAllowed_Whitelist(t *testing.T) {
	// is on whitelist and not
	peer1 := "peer1"
	peer2 := "peer2"

	// peer1 is whitelisted, peer2 not
	conf := fmt.Sprintf("whitelisted_peers=%s", peer1)

	policy, err := create(strings.NewReader(conf))
	assert.NoError(t, err)
	assert.True(t, policy.IsPeerAllowed(peer1))
	assert.False(t, policy.IsPeerAllowed(peer2))

	// accept all peers

}

func Test_IsPeerAllowed_AcceptAll(t *testing.T) {
	// all incomming requests should be allowed
	conf := "accept_all_peers=1"

	policy, err := create(strings.NewReader(conf))
	assert.NoError(t, err)
	assert.True(t, policy.IsPeerAllowed("some_peer"))
	assert.True(t, policy.IsPeerAllowed("some_other_peer"))

	// accept all peers

}
