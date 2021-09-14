package policy

import (
	"path/filepath"
	"testing"

	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func Test_ReadFromSamplePolicy(t *testing.T) {
	p := DefaultPolicy()

	parser := flags.NewParser(&p, flags.PrintErrors)
	iniParser := flags.NewIniParser(parser)

	err := iniParser.ParseFile(filepath.Join("..", "sample_policy.conf"))
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), p.ReserveOnchainMsat)
	assert.Equal(t, []string{"peer1", "peer2", "peer3"}, p.PeerWhitelist)
}

func Test_Policy_IsAllowedPeer(t *testing.T) {
	const peer = "my-peer"

	p := DefaultPolicy()
	isAllowed := p.IsPeerAllowed(peer)
	assert.False(t, isAllowed)

	// Allowed peer returns true
	p = Policy{
		PeerWhitelist: []string{peer},
	}
	isAllowed = p.IsPeerAllowed(peer)
	assert.True(t, isAllowed)
}
