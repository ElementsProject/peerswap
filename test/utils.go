package test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/clightning"
	"github.com/elementsproject/peerswap/peerswaprpc"
	"github.com/elementsproject/peerswap/testframework"
)

const defaultLines = 1000

func IsIntegrationTest(t *testing.T) {
	t.Helper()

	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set env RUN_INTEGRATION_TESTS=1 to run this test")
	}
}

func OverrideLinesFromEnvVar(lines int) int {
	if slines, ok := os.LookupEnv("PS_LOG_LINES"); ok {
		n, err := strconv.Atoi(slines)
		if err != nil {
			return lines
		}
		return n
	}
	return lines
}

type tailableProcess struct {
	p      *testframework.DaemonProcess
	lines  int
	filter string
}

func pprintFail(fps ...tailableProcess) {
	fmt.Printf("\n============================== FAILURE ==============================\n\n")
	for _, fp := range fps {
		if fp.p == nil {
			continue
		}
		fmt.Printf("+++++++++++++++++++++++++++++ %s (StdOut) +++++++++++++++++++++++++++++\n", fp.p.Prefix())
		fmt.Printf("%s\n", fp.p.StdOut.Tail(OverrideLinesFromEnvVar(fp.lines), fp.filter))
		if fp.p.StdErr.String() != "" {
			fmt.Printf("+++++++++++++++++++++++++++++ %s (StdErr) +++++++++++++++++++++++++++++\n", fp.p.Prefix())
			fmt.Printf("%s\n", fp.p.StdErr.String())
		}
		fmt.Printf("+++++++++++++++++++++++++++++ %s (End) +++++++++++++++++++++++++++++\n", fp.p.Prefix())
		fmt.Printf("\n")
	}
}

func printFailedFiltered(t *testing.T, process *testframework.DaemonProcess) {
	t.Helper()

	if t.Failed() {
		filter := os.Getenv("PEERSWAP_TEST_FILTER")
		pprintFail(
			tailableProcess{
				p:      process,
				filter: filter,
				lines:  3000,
			},
		)
	}
}

func printFailed(t *testing.T, process *testframework.DaemonProcess) {
	t.Helper()

	if t.Failed() {
		filter := os.Getenv("PEERSWAP_TEST_FILTER")
		pprintFail(
			tailableProcess{
				p:      process,
				filter: filter,
				lines:  defaultLines,
			},
		)
	}
}

type ChainNode interface {
	GenerateBlocks(b int) error
	ReturnAsset() string
}

type pollableNode interface {
	ID() string
	TriggerPoll() error
	AwaitPollFrom(node pollableNode) error
}

type clnPollableNode struct {
	*testframework.CLightningNode
}

func (n *clnPollableNode) ID() string {
	return n.Id()
}

func (n *clnPollableNode) TriggerPoll() error {
	var result any
	err := n.Rpc.Request(&clightning.ReloadPolicyFile{}, &result)
	if err != nil {
		return err
	}
	return nil
}

func (n *clnPollableNode) AwaitPollFrom(node pollableNode) error {
	return n.WaitForLog("Received poll from peer "+node.ID(), testframework.TIMEOUT)
}

type peerswapPollableNode struct {
	*PeerSwapd
	peerID string
}

func (n *peerswapPollableNode) ID() string {
	return n.peerID
}

func (n *peerswapPollableNode) TriggerPoll() error {
	_, err := n.PeerswapClient.ReloadPolicyFile(context.Background(), &peerswaprpc.ReloadPolicyFileRequest{})
	if err != nil {
		return err
	}
	return nil
}

func (n *peerswapPollableNode) AwaitPollFrom(node pollableNode) error {
	return n.WaitForLog("Received poll from peer "+node.ID(), testframework.TIMEOUT)
}

func syncPoll(a, b pollableNode) error {
	if err := a.TriggerPoll(); err != nil {
		return fmt.Errorf("TriggerPoll() (a) %w", err)
	}
	if err := b.TriggerPoll(); err != nil {
		return fmt.Errorf("TriggerPoll() (b) %w", err)
	}

	if err := a.AwaitPollFrom(b); err != nil {
		return fmt.Errorf("AwaitPollFrom() (ab) %w", err)
	}

	if err := b.AwaitPollFrom(a); err != nil {
		return fmt.Errorf("AwaitPollFrom() (ba) %w", err)
	}

	return nil
}

func waitForBlockheightSync(t *testing.T, timeout time.Duration, nodes ...testframework.LightningNode) {
	t.Helper()

	for _, node := range nodes {
		err := testframework.WaitFor(func() bool {
			ok, err := node.IsBlockHeightSynced()
			requireNoError(t, err)
			return ok
		}, timeout)
		requireNoError(t, err)
	}
}

func waitForTxInMempool(t *testing.T, chainRPC *testframework.RpcProxy, timeout time.Duration) (uint64, error) {
	t.Helper()

	var satFee uint64
	err := testframework.WaitFor(func() bool {
		var mempool map[string]struct {
			Fees struct {
				Base float64 `json:"base"`
			} `json:"fees"`
		}
		jsonR, err := chainRPC.Call("getrawmempool", true)
		requireNoError(t, err)

		err = jsonR.GetObject(&mempool)
		requireNoError(t, err)

		if len(mempool) == 1 {
			for _, tx := range mempool {
				satFee = uint64(tx.Fees.Base * 100000000)
				return true
			}
		}
		return false
	}, timeout)
	return satFee, err
}
