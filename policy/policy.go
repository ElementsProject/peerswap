package policy

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/jessevdk/go-flags"
)

const (
	// ReserveOnchainMsat is the amount of msat that is
	// kept as a reserve in the onchain wallet and
	// can not be spent by incoming swap requests.
	defaultReserveOnchainMsat uint64 = 0
	defaultAcceptAllPeers            = false

	// defaultMinSwapAmount is the default of the minimum in msat that is needed
	// to perform a swap. We need this lower boundary as it is uneconomical to
	// swap small amounts.
	defaultMinSwapAmountMsat uint64 = 100000000
)

// Global Mutex
var mu = sync.Mutex{}

var (
	defaultPeerAllowlist = []string{}

	// defaultSuspiciousPeerList is the default set of suspicious peers.
	defaultSuspiciousPeerList = []string{}

	// defaultAllowNewSwaps is true as we want to allow performing swaps per
	// default.
	defaultAllowNewSwaps = true
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

type ErrNotAValidPublicKey string

func (e ErrNotAValidPublicKey) Error() string {
	return fmt.Sprintf("%s is not a valid public key", string(e))
}

// PolicyConfig will ensure that a swap request is
// only performed if the policy is matched. In this
// case this means that the requesting peer is part
// of the allowlist and that the tank reserve is
// respected.
type Policy struct {
	path string

	ReserveOnchainMsat uint64   `json:"reserve_onchain_msat" long:"reserve_onchain_msat" description:"The amount of msats that are kept untouched on the onchain wallet for swap requests that are received." clightning_options:"ignore"`
	PeerAllowlist      []string `json:"allowlisted_peers" long:"allowlisted_peers" description:"A list of peers that are allowed to send swap requests to the node."`
	SuspiciousPeerList []string `json:"suspicious_peers" long:"suspicious_peers" description:"A list of peers that acted suspicious and are not allowed to request swaps."`
	AcceptAllPeers     bool     `json:"accept_all_peers" long:"accept_all_peers" description:"Use with caution! If set, the peer allowlist is ignored and all incoming swap requests are allowed"`

	// MinSwapAmountMsat is the minimum swap amount in msat that is needed to
	// perform a swap. Below this amount it might be uneconomical to do a swap
	// due to the on-chain costs.
	MinSwapAmountMsat uint64 `json:"min_swap_amount_msat" long:"min_swap_amount_msat" description:"The minimum amount in msat that is needed to perform a swap."`

	// AllowNewSwaps can be used to disallow any new swaps. This can be useful
	// when we want to upgrade the node and do not want to allow for any new
	// swap request from the peer or the node operator.
	AllowNewSwaps bool `json:"allow_new_swaps" long:"allow_new_swaps" description:"If set to false, disables all swap requests, defaults to true."`
}

func (p *Policy) String() string {
	str := fmt.Sprintf(
		"allow_new_swaps: %t\n"+
			"min_swap_amount_msat: %d\n"+
			"reserve_onchain_msat: %d\n"+
			"allowlisted_peers: %s\n"+
			"accept_all_peers: %t\n"+
			"suspicious_peers: %s\n",
		p.AllowNewSwaps,
		p.MinSwapAmountMsat,
		p.ReserveOnchainMsat,
		p.PeerAllowlist,
		p.AcceptAllPeers,
		p.SuspiciousPeerList,
	)
	return str
}

func (p *Policy) Get() Policy {
	mu.Lock()
	defer mu.Unlock()

	return Policy{
		ReserveOnchainMsat: p.ReserveOnchainMsat,
		PeerAllowlist:      p.PeerAllowlist,
		SuspiciousPeerList: p.SuspiciousPeerList,
		AcceptAllPeers:     p.AcceptAllPeers,
		MinSwapAmountMsat:  p.MinSwapAmountMsat,
		AllowNewSwaps:      p.AllowNewSwaps,
	}
}

// GetReserveOnchainMsat returns the amount of msats
// that should be keept in the wallet when receiving
// a peerswap request.
func (p *Policy) GetReserveOnchainMsat() uint64 {
	mu.Lock()
	defer mu.Unlock()
	return p.ReserveOnchainMsat
}

// GetMinSwapAmountMsat returns the minimum swap amount in msat that is needed
// to perform a swap.
func (p *Policy) GetMinSwapAmountMsat() uint64 {
	mu.Lock()
	defer mu.Unlock()
	return p.MinSwapAmountMsat
}

// NewSwapsAllowed returns the boolean value of AllowNewSwaps.
func (p *Policy) NewSwapsAllowed() bool {
	return p.AllowNewSwaps
}

// IsPeerAllowed returns if a peer or node is part of
// the allowlist.
func (p *Policy) IsPeerAllowed(peer string) bool {
	mu.Lock()
	defer mu.Unlock()
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

// IsPeerSuspicious returns true if the peer is on the list of suspicious peers.
func (p *Policy) IsPeerSuspicious(peer string) bool {
	mu.Lock()
	defer mu.Unlock()
	for _, suspiciousPeer := range p.SuspiciousPeerList {
		if peer == suspiciousPeer {
			return true
		}
	}
	return false
}

// ReloadFile reloads and and sets the policy
// to the policy file. This might be because the
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

// DisableSwaps sets the AllowNewSwaps field to false. This persists in the
// policy.conf.
func (p *Policy) DisableSwaps() error {
	mu.Lock()
	defer mu.Unlock()

	if !p.AllowNewSwaps {
		return nil
	}

	err := removeLineFromFile(p.path, "allow_new_swaps=true")
	if err != nil {
		return err
	}
	err = addLineToFile(p.path, "allow_new_swaps=false")
	if err != nil {
		return err
	}

	return p.ReloadFile()
}

// EnableSwaps sets the AllowNewSwaps field to true. This persists in the
// policy.conf.
func (p *Policy) EnableSwaps() error {
	mu.Lock()
	defer mu.Unlock()

	if p.AllowNewSwaps {
		return nil
	}

	err := removeLineFromFile(p.path, "allow_new_swaps=false")
	if err != nil {
		return err
	}
	err = addLineToFile(p.path, "allow_new_swaps=true")
	if err != nil {
		return err
	}

	return p.ReloadFile()
}

// AddToAllowlist adds a peer to the policy file in runtime. The pubkey is
// expected to be hex encoded.
func (p *Policy) AddToAllowlist(pubkey string) error {
	mu.Lock()
	defer mu.Unlock()

	for _, v := range p.PeerAllowlist {
		if v == pubkey {
			return errors.New("peer is already whitelisted")
		}
	}
	if p.path == "" {
		return ErrNoPolicyFile
	}

	ok, err := isValidPubkey(pubkey)
	if !ok {
		return err
	}

	err = addLineToFile(p.path, fmt.Sprintf("allowlisted_peers=%s", pubkey))
	if err != nil {
		return err
	}
	return p.ReloadFile()
}

// AddToSuspiciousPeerList adds a peer as a suspicious peer to the policy file
// in runtime. The pubkey is expected to be hex encoded.
func (p *Policy) AddToSuspiciousPeerList(pubkey string) error {
	mu.Lock()
	defer mu.Unlock()

	for _, v := range p.SuspiciousPeerList {
		if v == pubkey {
			return errors.New("peer is already marked as suspicious")
		}
	}
	if p.path == "" {
		return ErrNoPolicyFile
	}

	ok, err := isValidPubkey(pubkey)
	if !ok {
		return err
	}

	err = addLineToFile(p.path, fmt.Sprintf("suspicious_peers=%s", pubkey))
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

// RemoveFromAllowlist removes the pubkey of a node from the policy
// allowlisted_peers list. If a pubkey is removed from the allowlist the node
// corresponding to the pubkey is no longer allowed to request swaps. The pubkey
// is expected to be hex encoded.
func (p *Policy) RemoveFromAllowlist(pubkey string) error {
	mu.Lock()
	defer mu.Unlock()

	ok, err := isValidPubkey(pubkey)
	if !ok {
		return err
	}

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
	err = removeLineFromFile(p.path, fmt.Sprintf("allowlisted_peers=%s", pubkey))
	if err != nil {
		return err
	}
	return p.ReloadFile()
}

// RemoveFromSuspiciousPeerList removes the pubkey of a node from the policy
// suspicious_peers list. The pubkey is expected to be hex encoded.
func (p *Policy) RemoveFromSuspiciousPeerList(pubkey string) error {
	mu.Lock()
	defer mu.Unlock()

	ok, err := isValidPubkey(pubkey)
	if !ok {
		return err
	}

	var peerPk string
	for _, v := range p.SuspiciousPeerList {
		if v == pubkey {
			peerPk = v
			break
		}
	}
	if peerPk == "" {
		return fmt.Errorf("peer %s is not in suspicious peer list", pubkey)
	}
	if p.path == "" {
		return ErrNoPolicyFile
	}
	err = removeLineFromFile(p.path, fmt.Sprintf("suspicious_peers=%s", pubkey))
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
		SuspiciousPeerList: defaultSuspiciousPeerList,
		AcceptAllPeers:     defaultAcceptAllPeers,
		MinSwapAmountMsat:  defaultMinSwapAmountMsat,
		AllowNewSwaps:      defaultAllowNewSwaps,
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

// isValidPubkey validates that the pubkey is 66 bytes hex encoded.
func isValidPubkey(pubkey string) (bool, error) {
	matched, err := regexp.MatchString("^[0-9a-f]{66}?\\z", pubkey)
	if err != nil {
		return false, err
	}
	if !matched {
		return false, ErrNotAValidPublicKey(pubkey)
	}
	return true, nil
}
