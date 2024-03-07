package policy

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testPubKey = "02a427b2f7284fe185216dc9a60689104ee6f785eb2d636d3786ab46e5cbd9f12d"

func Test_Create(t *testing.T) {
	// check if all variables are set
	// check default variables

	policy, err := create(strings.NewReader(""))
	assert.NoError(t, err)
	assert.EqualValues(t, &Policy{
		ReserveOnchainMsat:    defaultReserveOnchainMsat,
		PeerAllowlist:         defaultPeerAllowlist,
		SuspiciousPeerList:    defaultSuspiciousPeerList,
		AcceptAllPeers:        defaultAcceptAllPeers,
		MinSwapAmountMsat:     defaultMinSwapAmountMsat,
		AllowNewSwaps:         defaultAllowNewSwaps,
		SwapInPremiumRatePPM:  defaultSwapInPremiumRatePPM,
		SwapOutPremiumRatePPM: defaultSwapOutPremiumRatePPM,
	}, policy)

	peer1 := "123"
	peer2 := "345"
	accept := true
	var acceptInt int8
	if accept {
		acceptInt = 1
	}

	conf := fmt.Sprintf(
		"accept_all_peers=%d\n"+
			"allowlisted_peers=%s\n"+
			"allowlisted_peers=%s\n"+
			"suspicious_peers=%s\n"+
			"suspicious_peers=%s",
		acceptInt,
		peer1,
		peer2,
		peer1,
		peer2,
	)

	policy2, err := create(strings.NewReader(conf))
	assert.NoError(t, err)
	assert.EqualValues(t, &Policy{
		ReserveOnchainMsat:    defaultReserveOnchainMsat,
		PeerAllowlist:         []string{peer1, peer2},
		SuspiciousPeerList:    []string{peer1, peer2},
		AcceptAllPeers:        accept,
		MinSwapAmountMsat:     defaultMinSwapAmountMsat,
		AllowNewSwaps:         defaultAllowNewSwaps,
		SwapInPremiumRatePPM:  defaultSwapInPremiumRatePPM,
		SwapOutPremiumRatePPM: defaultSwapOutPremiumRatePPM,
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

	conf := fmt.Sprintf("accept_all_peers=%d\nallowlisted_peers=%s\nallowlisted_peers=%s", acceptInt, peer1, peer2)

	policy, err := create(strings.NewReader(conf))
	assert.NoError(t, err)
	assert.EqualValues(t, &Policy{
		ReserveOnchainMsat:    defaultReserveOnchainMsat,
		PeerAllowlist:         []string{peer1, peer2},
		SuspiciousPeerList:    defaultSuspiciousPeerList,
		AcceptAllPeers:        accept,
		MinSwapAmountMsat:     defaultMinSwapAmountMsat,
		AllowNewSwaps:         defaultAllowNewSwaps,
		SwapInPremiumRatePPM:  defaultSwapInPremiumRatePPM,
		SwapOutPremiumRatePPM: defaultSwapOutPremiumRatePPM,
	}, policy)

	newPeer := "new_peer"
	newConf := fmt.Sprintf("allowlisted_peers=%s\nsuspicious_peers=%s", newPeer, newPeer)

	err = policy.reload(strings.NewReader(newConf))
	assert.NoError(t, err)
	assert.EqualValues(t, &Policy{
		ReserveOnchainMsat:    defaultReserveOnchainMsat,
		PeerAllowlist:         []string{newPeer},
		SuspiciousPeerList:    []string{newPeer},
		AcceptAllPeers:        defaultAcceptAllPeers,
		MinSwapAmountMsat:     defaultMinSwapAmountMsat,
		AllowNewSwaps:         defaultAllowNewSwaps,
		SwapInPremiumRatePPM:  defaultSwapInPremiumRatePPM,
		SwapOutPremiumRatePPM: defaultSwapOutPremiumRatePPM,
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

	conf := fmt.Sprintf("accept_all_peers=%d\nallowlisted_peers=%s\nallowlisted_peers=%s", acceptInt, peer1, peer2)

	policy, err := create(strings.NewReader(conf))
	assert.NoError(t, err)
	assert.EqualValues(t, &Policy{
		ReserveOnchainMsat:    defaultReserveOnchainMsat,
		PeerAllowlist:         []string{peer1, peer2},
		SuspiciousPeerList:    defaultSuspiciousPeerList,
		AcceptAllPeers:        accept,
		MinSwapAmountMsat:     defaultMinSwapAmountMsat,
		AllowNewSwaps:         defaultAllowNewSwaps,
		SwapInPremiumRatePPM:  defaultSwapInPremiumRatePPM,
		SwapOutPremiumRatePPM: defaultSwapOutPremiumRatePPM,
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

func Test_AddRemovePeer_Runtime(t *testing.T) {
	var pubkeys []string
	for i := 0; i < 4; i++ {
		pubkeys = append(pubkeys, randomPubKeyHex())
	}

	policyFilePath := path.Join(t.TempDir(), "policy.conf")
	file, err := os.Create(policyFilePath)
	assert.NoError(t, err)

	err = file.Close()
	assert.NoError(t, err)

	policy, err := CreateFromFile(policyFilePath)
	assert.NoError(t, err)

	err = policy.AddToAllowlist(pubkeys[0])
	assert.NoError(t, err)
	err = policy.AddToSuspiciousPeerList(pubkeys[1])
	assert.NoError(t, err)

	policyFile, err := ioutil.ReadFile(policyFilePath)
	assert.NoError(t, err)
	assert.Equal(
		t,
		fmt.Sprintf("allowlisted_peers=%s\nsuspicious_peers=%s\n", pubkeys[0], pubkeys[1]),
		string(policyFile),
	)

	err = policy.AddToAllowlist(pubkeys[2])
	assert.NoError(t, err)
	err = policy.RemoveFromAllowlist(pubkeys[0])
	assert.NoError(t, err)

	err = policy.AddToSuspiciousPeerList(pubkeys[3])
	assert.NoError(t, err)
	err = policy.RemoveFromSuspiciousPeerList(pubkeys[1])
	assert.NoError(t, err)

	policyFile, err = ioutil.ReadFile(policyFilePath)
	assert.NoError(t, err)
	assert.Equal(
		t,
		fmt.Sprintf("allowlisted_peers=%s\nsuspicious_peers=%s\n", pubkeys[2], pubkeys[3]),
		string(policyFile),
	)
}

func Test_AddRemovePeer_Runtime_ConcurrentWrite(t *testing.T) {
	const N_CONC_W = 500

	policyFilePath := path.Join(t.TempDir(), "policy.conf")
	file, err := os.Create(policyFilePath)
	if err != nil {
		t.Fatalf("Failed Create(): %v", err)
	}

	err = file.Close()
	if err != nil {
		t.Fatalf("Failed Close(): %v", err)
	}

	policy, err := CreateFromFile(policyFilePath)
	assert.NoError(t, err)

	var expectedPeers []string
	for i := 0; i < N_CONC_W; i++ {
		expectedPeers = append(expectedPeers, randomPubKeyHex())
	}

	wg := &sync.WaitGroup{}
	wg.Add(3 * N_CONC_W)
	for i := 0; i < N_CONC_W; i++ {
		pubkey := expectedPeers[i]

		go func() {
			_ = policy.GetReserveOnchainMsat()
			_ = policy.GetMinSwapAmountMsat()
			_ = policy.IsPeerAllowed("abc")
			_ = policy.IsPeerSuspicious("abc")
			wg.Done()
		}()
		go func(s string) {
			ierr := policy.AddToSuspiciousPeerList(s)
			assert.NoError(t, ierr)
			wg.Done()
		}(pubkey)
		go func(s string) {
			ierr := policy.AddToAllowlist(s)
			assert.NoError(t, ierr)
			wg.Done()
		}(pubkey)

	}

	wg.Wait()

	assert.ElementsMatch(t, expectedPeers, policy.PeerAllowlist)
	assert.ElementsMatch(t, expectedPeers, policy.SuspiciousPeerList)

}

func Test_IsPeerAllowed_Allowlist(t *testing.T) {
	// is on allowlist and not
	peer1 := "peer1"
	peer2 := "peer2"

	// peer1 is allowlisted, peer2 not
	conf := fmt.Sprintf("allowlisted_peers=%s", peer1)

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

func Test_CreateFile(t *testing.T) {
	confPath := filepath.Join(t.TempDir(), "peerswap.conf")

	policy, err := CreateFromFile(confPath)
	require.NoError(t, err)

	fileInfo, err := os.Stat(confPath)
	require.NoError(t, err)

	assert.Equal(t, "peerswap.conf", fileInfo.Name())

	err = policy.AddToAllowlist(testPubKey)
	require.NoError(t, err)
}

func Test_isValidPubkey(t *testing.T) {
	tests := []struct {
		desc     string
		arg      string
		isOk     bool
		hasError bool
	}{
		{
			desc:     "valid",
			arg:      "02a427b2f7284fe185216dc9a60689104ee6f785eb2d636d3786ab46e5cbd9f12d",
			isOk:     true,
			hasError: false,
		},
		{
			desc:     "too_long",
			arg:      "02a427b2f7284fe185216dc9a60689104ee6f785eb2d636d3786ab46e5cbd9f12d12",
			isOk:     false,
			hasError: true,
		},
		{
			desc:     "too_short",
			arg:      "02a427b2f7284fe185216dc9a60689104ee6f785eb2d636d3786ab46e5cbd9f1",
			isOk:     false,
			hasError: true,
		},
		{
			desc:     "wrong_char",
			arg:      "02a427h2f7284fe185216dc9a60689104ee6f785eb2d636d3786ab46e5cbd9f1",
			isOk:     false,
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ok, err := isValidPubkey(tt.arg)
			assert.Equal(t, tt.isOk, ok)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if t.Failed() {
				fmt.Println(err)
			}
		})
	}
}

func Test_AllowSwapRequests(t *testing.T) {
	policyFilePath := path.Join(t.TempDir(), "policy.conf")
	file, err := os.Create(policyFilePath)
	assert.NoError(t, err)

	err = file.Close()
	assert.NoError(t, err)

	policy, err := CreateFromFile(policyFilePath)
	assert.NoError(t, err)

	// Write the disallow to the policy file.
	err = policy.DisableSwaps()
	assert.NoError(t, err)
	assert.False(t, policy.NewSwapsAllowed())

	policyFile, _ := ioutil.ReadFile(policyFilePath)
	assert.Equal(
		t,
		"allow_new_swaps=false\n",
		string(policyFile))

	// Write allow_new_swaps to policy file.
	err = policy.EnableSwaps()
	assert.NoError(t, err)
	assert.True(t, policy.NewSwapsAllowed())

	policyFile, _ = ioutil.ReadFile(policyFilePath)
	assert.Equal(
		t,
		"allow_new_swaps=true\n",
		string(policyFile))
}

func randomPubKeyHex() string {
	var b = make([]byte, 33)
	rand.Read(b)
	return hex.EncodeToString(b)
}
