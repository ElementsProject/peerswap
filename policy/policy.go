package policy

import (
	"fmt"

	"github.com/jessevdk/go-flags"
)

const (
	// reserveTankMsat is the amount of msat that is
	// kept as a reserve in the onchain wallet and
	// can not be spent by incomming swap requests.
	defaultReserveTankMsat uint64 = 0
)

var (
	defaultPeerWhitelist = []string{}
)

// PolicyConfig will ensure that a swap request is only
// performed if the policy is matched. In this case
// this means that the requesting peer is part of
// the whitelist and that the tank reserve is
// respected.
type Policy struct {
	ReserveTankMsat uint64   `long:"reserve_tank_msat" description:"The amount of msats that are kept untouched on the onchain wallet for swap requests that are received." clightning_options:"ignore"`
	PeerWhitelist   []string `long:"whitelisted_peers" description:"A list of peers that are allowed to send swap requests to the node.x"`
}

func (p Policy) String() string {
	return fmt.Sprintf("reserve_tank_msat: %d\nwhitelisted_peers: %s", p.ReserveTankMsat, p.PeerWhitelist)
}

// GetTankReserve returns the amount of msats that
// should be keept in the wallet when receiving a
// peerswap request.
func (p Policy) GetTankReserve() uint64 {
	return p.ReserveTankMsat
}

// IsPeerAllowed returns if a peer or node is part
// of the whitelist.
func (p Policy) IsPeerAllowed(peer string) bool {
	for _, allowedPeer := range p.PeerWhitelist {
		if peer == allowedPeer {
			return true
		}
	}
	return false
}

func DefaultPolicy() Policy {
	return Policy{
		ReserveTankMsat: defaultReserveTankMsat,
		PeerWhitelist:   defaultPeerWhitelist,
	}
}

// CreatePolicy returns a policy based on a
// DefaultPolicy. If the path to the policy file
// (ini notation) is empty, the default policy is used.
func CreatePolicy(path string) (Policy, error) {
	policy := DefaultPolicy()

	if path == "" {
		return policy, nil
	}

	err := flags.IniParse(path, &policy)
	if err != nil {
		return policy, err
	}

	return policy, nil
}
