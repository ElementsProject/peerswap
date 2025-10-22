package test

import (
	"os"
	"testing"

	"github.com/elementsproject/peerswap/testframework"
)

// DumpOption configures which processes to include in a failure dump.
type DumpOption func(*[]tailableProcess)

// DumpOnFailure registers a t.Cleanup that tails logs from the configured
// processes only when the test fails. Use With* options to add processes.
func DumpOnFailure(t *testing.T, opts ...DumpOption) {
	t.Helper()

	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		var processes []tailableProcess
		for _, opt := range opts {
			opt(&processes)
		}
		if len(processes) == 0 {
			return
		}
		pprintFail(processes...)
	})
}

// WithBitcoin includes bitcoind logs in the failure dump.
func WithBitcoin(bitcoind *testframework.BitcoinNode) DumpOption {
	return func(ps *[]tailableProcess) {
		if bitcoind == nil {
			return
		}
		*ps = append(*ps, tailableProcess{p: bitcoind.DaemonProcess, lines: defaultLines})
	}
}

// WithLiquid includes liquidd logs in the failure dump.
func WithLiquid(liquidd *testframework.LiquidNode) DumpOption {
	return func(ps *[]tailableProcess) {
		if liquidd == nil {
			return
		}
		*ps = append(*ps, tailableProcess{p: liquidd.DaemonProcess, lines: defaultLines})
	}
}

// WithElectrs includes electrs logs in the failure dump.
func WithElectrs(e *testframework.Electrs) DumpOption {
	return func(ps *[]tailableProcess) {
		if e == nil {
			return
		}
		*ps = append(*ps, tailableProcess{p: e.Process, lines: defaultLines})
	}
}

// WithLWK includes LWK logs in the failure dump.
func WithLWK(lwk *testframework.LWK) DumpOption {
	return func(ps *[]tailableProcess) {
		if lwk == nil {
			return
		}
		*ps = append(*ps, tailableProcess{p: lwk.Process, lines: defaultLines})
	}
}

// WithPeerSwapd includes peerswapd logs in the failure dump.
func WithPeerSwapd(p *PeerSwapd) DumpOption {
	return func(ps *[]tailableProcess) {
		if p == nil || p.DaemonProcess == nil {
			return
		}
		*ps = append(*ps, tailableProcess{p: p.DaemonProcess, lines: defaultLines})
	}
}

// WithPeerSwapds includes multiple peerswapd processes in the failure dump.
func WithPeerSwapds(peers ...*PeerSwapd) DumpOption {
	return func(ps *[]tailableProcess) {
		for _, p := range peers {
			if p == nil || p.DaemonProcess == nil {
				continue
			}
			*ps = append(*ps, tailableProcess{p: p.DaemonProcess, lines: defaultLines})
		}
	}
}

// WithCLightningNodes includes C-Lightning node logs. If filters is provided,
// it is applied per-node index; otherwise PEERSWAP_TEST_FILTER is used.
func WithCLightningNodes(nodes []*CLightningNodeWithLiquid, filters []string) DumpOption {
	return func(ps *[]tailableProcess) {
		if len(nodes) == 0 {
			return
		}
		defaultFilter := os.Getenv("PEERSWAP_TEST_FILTER")
		for i, n := range nodes {
			if n == nil {
				continue
			}
			filter := defaultFilter
			if len(filters) > i {
				filter = filters[i]
			}
			*ps = append(*ps, tailableProcess{
				p:      n.DaemonProcess,
				filter: filter,
				lines:  defaultLines,
			})
		}
	}
}

// WithLightningNodes includes mixed CLN/LND node logs. CLN nodes are filtered
// by PEERSWAP_TEST_FILTER; LND nodes are included without filter.
func WithLightningNodes(lightningds []testframework.LightningNode) DumpOption {
	return func(ps *[]tailableProcess) {
		if len(lightningds) == 0 {
			return
		}
		filter := os.Getenv("PEERSWAP_TEST_FILTER")
		for _, node := range lightningds {
			switch n := node.(type) {
			case *CLightningNodeWithLiquid:
				*ps = append(*ps, tailableProcess{
					p:      n.DaemonProcess,
					filter: filter,
					lines:  defaultLines,
				})
			case *LndNodeWithLiquid:
				*ps = append(*ps, tailableProcess{
					p:     n.DaemonProcess,
					lines: defaultLines,
				})
			case *testframework.CLightningNode:
				*ps = append(*ps, tailableProcess{
					p:      n.DaemonProcess,
					filter: filter,
					lines:  defaultLines,
				})
			case *testframework.LndNode:
				*ps = append(*ps, tailableProcess{
					p:     n.DaemonProcess,
					lines: defaultLines,
				})
			default:
				// Ignore unknown types to keep helper generic.
			}
		}
	}
}

// WithCLightnings includes Core Lightning nodes (bitcoin) with a shared filter.
func WithCLightnings(nodes []*testframework.CLightningNode) DumpOption {
	return func(ps *[]tailableProcess) {
		if len(nodes) == 0 {
			return
		}
		filter := os.Getenv("PEERSWAP_TEST_FILTER")
		for _, n := range nodes {
			if n == nil {
				continue
			}
			*ps = append(*ps, tailableProcess{p: n.DaemonProcess, filter: filter, lines: defaultLines})
		}
	}
}

// WithLnds includes LND nodes (bitcoin) without filter.
func WithLnds(nodes []*testframework.LndNode) DumpOption {
	return func(ps *[]tailableProcess) {
		if len(nodes) == 0 {
			return
		}
		for _, n := range nodes {
			if n == nil {
				continue
			}
			*ps = append(*ps, tailableProcess{p: n.DaemonProcess, lines: defaultLines})
		}
	}
}

// WithLndNodesWithLiquid includes LND nodes configured for Liquid (LndNodeWithLiquid).
func WithLndNodesWithLiquid(nodes []*LndNodeWithLiquid) DumpOption {
	return func(ps *[]tailableProcess) {
		if len(nodes) == 0 {
			return
		}
		for _, n := range nodes {
			if n == nil {
				continue
			}
			*ps = append(*ps, tailableProcess{p: n.DaemonProcess, lines: defaultLines})
		}
	}
}
