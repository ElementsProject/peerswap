package policy

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jessevdk/go-flags"
)

const (
	// ReserveOnchainMsat is the amount of msat that is
	// kept as a reserve in the onchain wallet and
	// can not be spent by incoming swap requests.
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
		return fmt.Sprintf("%sCAUTION: Accept all incoming swap requests", str)
	}
	return str
}

func (p *Policy) Get() Policy {
	return Policy{
		ReserveOnchainMsat: p.ReserveOnchainMsat,
		PeerAllowlist:      p.PeerAllowlist,
		AcceptAllPeers:     p.AcceptAllPeers,
	}
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

// AddToAllowlist adds a peer to the policy file in runtime
func (p *Policy) AddToAllowlist(pubkey string) error {
	for _, v := range p.PeerAllowlist {
		if v == pubkey {
			return errors.New("peer is already whitelisted")
		}
	}
	if p.path == "" {
		return ErrNoPolicyFile
	}
	err := addLineToFile(p.path, fmt.Sprintf("allowlisted_peers=%s", pubkey))
	if err != nil {
		return err
	}
	return p.ReloadFile()
}

func addLineToFile(filePath, line string) error {
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0660)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(line + "\n")
	if err != nil {
		return err
	}
	return nil
}

func (p *Policy) RemoveFromAllowlist(pubkey string) error {
	var peerPk string
	for _, v := range p.PeerAllowlist {
		if v == pubkey {
			peerPk = v
			break
		}
	}
	if peerPk == "" {
		return fmt.Errorf("peer %s is not in allowlist", pubkey)
	}
	if p.path == "" {
		return ErrNoPolicyFile
	}
	err := removeLineFromFile(p.path, fmt.Sprintf("allowlisted_peers=%s", pubkey))
	if err != nil {
		return err
	}
	return p.ReloadFile()

}

func removeLineFromFile(filePath, line string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	var bs []byte
	buf := bytes.NewBuffer(bs)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if scanner.Text() != line {
			_, err := buf.Write(scanner.Bytes())
			if err != nil {
				return err
			}
			_, err = buf.WriteString("\n")
			if err != nil {
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	err = os.WriteFile(filePath, buf.Bytes(), 0660)
	if err != nil {
		return err
	}
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

	policyPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(filepath.Dir(policyPath), 0755)
	if err != nil {
		return nil, ErrCreatePolicy(err.Error())
	}

	file, err := os.OpenFile(policyPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, ErrCreatePolicy(err.Error())
	}
	defer file.Close()

	policy, err := create(file)
	if err != nil {
		return nil, err
	}

	policy.path = policyPath
	return policy, nil
}

// Create returns a policy based on a DefaultPolicy.
func create(r io.Reader) (*Policy, error) {
	policy := DefaultPolicy()
	err := flags.NewIniParser(flags.NewParser(policy, flags.Default|flags.IgnoreUnknown)).Parse(r)
	if err != nil {
		return nil, ErrCreatePolicy(err.Error())
	}

	return policy, nil
}
