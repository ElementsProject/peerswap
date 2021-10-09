package policy

import (
	"fmt"
	"io"
	"os"

	"github.com/jessevdk/go-flags"
)

const (
	// ReserveOnchainMsat is the amount of msat that is
	// kept as a reserve in the onchain wallet and
	// can not be spent by incomming swap requests.
	defaultReserveOnchainMsat uint64 = 0
	defaultAcceptAllPeers            = false
)

var (
	defaultPeerAllowlist = []string{}
)

// Error definitions
var ErrNoPolicyFile error = fmt.Errorf("no policy file given")

type ErrCreatePolicy string

func (e ErrCreatePolicy) Error() string {
	return fmt.Sprintf("policy could not be created: %v", string(e))
}

type ErrReloadPolicy string

func (e ErrReloadPolicy) Error() string {
	return fmt.Sprintf("policy could not be reloaded: %v", string(e))
}

// PolicyConfig will ensure that a swap request is
// only performed if the policy is matched. In this
// case this means that the requesting peer is part
// of the allowlist and that the tank reserve is
// respected.
type Policy struct {
	path string

	ReserveOnchainMsat uint64   `long:"reserve_onchain_msat" description:"The amount of msats that are kept untouched on the onchain wallet for swap requests that are received." clightning_options:"ignore"`
	PeerAllowlist      []string `long:"allowlisted_peers" description:"A list of peers that are allowed to send swap requests to the node."`
	AcceptAllPeers     bool     `long:"accept_all_peers" description:"Use with caution! If set, the peer allowlist is ignored and all incomming swap requests are allowed"`
}

func (p *Policy) String() string {
	str := fmt.Sprintf("reserve_onchain_msat: %d\nallowlisted_peers: %s\naccept_all_peers: %t\n", p.ReserveOnchainMsat, p.PeerAllowlist, p.AcceptAllPeers)
	if p.AcceptAllPeers {
		return fmt.Sprintf("%sCAUTION: Accept all incomming swap requests", str)
	}
	return str
}

// GetReserveOnchainMsat returns the amount of msats
// that should be keept in the wallet when receiving
// a peerswap request.
func (p *Policy) GetReserveOnchainMsat() uint64 {
	return p.ReserveOnchainMsat
}

// IsPeerAllowed returns if a peer or node is part of
// the allowlist.
func (p *Policy) IsPeerAllowed(peer string) bool {
	if p.AcceptAllPeers {
		return true
	}
	for _, allowedPeer := range p.PeerAllowlist {
		if peer == allowedPeer {
			return true
		}
	}
	return false
}

// ReloadFile reloads and and sets the policy
// to the policy file. This might be becaus the
// policy file changed and the runtime should use the
// new policy.
func (p *Policy) ReloadFile() error {
	if p.path == "" {
		return ErrNoPolicyFile
	}
	path := p.path

	file, err := os.Open(p.path)
	if err != nil {
		return ErrReloadPolicy(err.Error())
	}
	defer file.Close()

	err = p.reload(file)
	if err != nil {
		return err
	}

	p.path = path
	return nil
}

func (p *Policy) reload(r io.Reader) error {
	newp, err := create(r)
	if err != nil {
		return err
	}

	*p = *newp
	return nil
}

// DefaultPolicy returns a policy struct made from
// the default values.
func DefaultPolicy() *Policy {
	return &Policy{
		ReserveOnchainMsat: defaultReserveOnchainMsat,
		PeerAllowlist:      defaultPeerAllowlist,
		AcceptAllPeers:     defaultAcceptAllPeers,
	}
}

// CreateFromFile returns a policy based on a
// DefaultPolicy and a ini file with a policy
// configuration. If the path to the policy file (ini
// notation) is empty, the default policy is used.
func CreateFromFile(path string) (*Policy, error) {
	if path == "" {
		return DefaultPolicy(), nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, ErrCreatePolicy(err.Error())
	}
	defer file.Close()

	policy, err := create(file)
	if err != nil {
		return nil, err
	}

	policy.path = path
	return policy, nil
}

// Create returns a policy based on a DefaultPolicy.
func create(r io.Reader) (*Policy, error) {
	policy := DefaultPolicy()

	err := flags.NewIniParser(flags.NewParser(policy, 0)).Parse(r)
	if err != nil {
		return nil, ErrCreatePolicy(err.Error())
	}

	return policy, nil
}
