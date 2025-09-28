package test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/testframework"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

type logPrinter func(*testing.T, *testframework.DaemonProcess)

type HarnessBuilder struct {
	t                  *testing.T
	testDir            string
	peerswapPluginPath string
	peerswapdBinary    string

	bitcoind        *testframework.BitcoinNode
	bitcoindStarted bool
	clnSpecs        []*clnNodeSpec
	lndSpecs        []*lndNodeSpec
	peerswapSpecs   []*peerswapdSpec

	started bool
}

func NewHarnessBuilder(t *testing.T) *HarnessBuilder {
	t.Helper()

	_, filename, _, _ := runtime.Caller(0)
	base := filepath.Join(filepath.Dir(filename), "..", "out", "test-builds")

	return &HarnessBuilder{
		t:                  t,
		testDir:            makeTestDataDir(t),
		peerswapPluginPath: filepath.Join(base, "peerswap"),
		peerswapdBinary:    filepath.Join(base, "peerswapd"),
	}
}

func (b *HarnessBuilder) TestDir() string {
	return b.testDir
}

func (b *HarnessBuilder) Bitcoind() *testframework.BitcoinNode {
	if b.bitcoind != nil {
		return b.bitcoind
	}

	node, err := testframework.NewBitcoinNode(b.testDir, 1)
	require.NoError(b.t, err, "failed to create bitcoind")
	b.t.Cleanup(node.Kill)
	b.bitcoind = node
	return node
}

func (b *HarnessBuilder) EnsureBitcoindStarted() {
	b.t.Helper()

	if b.bitcoind == nil {
		b.Bitcoind()
	}

	if b.bitcoindStarted {
		return
	}

	require.NoError(b.t, b.bitcoind.Run(true), "failed to start bitcoind")
	b.bitcoindStarted = true
}

type clnNodeSpec struct {
	node           *testframework.CLightningNode
	extraArgs      []string
	policy         []byte
	config         []byte
	waitLog        string
	failurePrinter logPrinter
	checkExitLog   bool
}

type ClnOption func(*clnNodeSpec)

func WithClnPolicy(policy []byte) ClnOption {
	return func(spec *clnNodeSpec) {
		spec.policy = copyBytes(policy)
	}
}

func WithClnConfig(config []byte) ClnOption {
	return func(spec *clnNodeSpec) {
		spec.config = copyBytes(config)
	}
}

func WithClnExtraArgs(args ...string) ClnOption {
	return func(spec *clnNodeSpec) {
		spec.extraArgs = append(spec.extraArgs, args...)
	}
}

func WithClnWaitLog(log string) ClnOption {
	return func(spec *clnNodeSpec) {
		spec.waitLog = log
	}
}

func WithClnFailurePrinter(printer logPrinter) ClnOption {
	return func(spec *clnNodeSpec) {
		spec.failurePrinter = printer
	}
}

func WithoutClnExitCheck() ClnOption {
	return func(spec *clnNodeSpec) {
		spec.checkExitLog = false
	}
}

func (b *HarnessBuilder) AddCLightningNode(index int, opts ...ClnOption) *testframework.CLightningNode {
	b.t.Helper()

	node, err := testframework.NewCLightningNode(b.testDir, b.Bitcoind(), index)
	require.NoError(b.t, err, "failed to create c-lightning node")
	b.t.Cleanup(node.Kill)

	spec := &clnNodeSpec{
		node:           node,
		waitLog:        "peerswap initialized",
		failurePrinter: printFailedFiltered,
		checkExitLog:   true,
	}

	for _, opt := range opts {
		opt(spec)
	}

	if spec.policy == nil {
		spec.policy = defaultClnPolicy()
	}
	if spec.config == nil {
		spec.config = []byte{}
	}

	peerswapDir := filepath.Join(node.GetDataDir(), "peerswap")
	require.NoError(b.t, os.MkdirAll(peerswapDir, os.ModePerm), "failed to create cln peerswap dir")
	require.NoError(b.t, os.WriteFile(filepath.Join(peerswapDir, "policy.conf"), spec.policy, os.ModePerm), "failed to write cln policy")
	require.NoError(b.t, os.WriteFile(filepath.Join(peerswapDir, "peerswap.conf"), spec.config, os.ModePerm), "failed to write cln config")

	node.WithCmd("lightningd")
	args := append([]string{fmt.Sprintf("--plugin=%s", b.peerswapPluginPath)}, spec.extraArgs...)
	node.AppendCmdLine(args)

	if spec.failurePrinter != nil {
		b.t.Cleanup(func() { spec.failurePrinter(b.t, node.DaemonProcess) })
	}

	b.clnSpecs = append(b.clnSpecs, spec)
	return node
}

func (spec *clnNodeSpec) start() error {
	if err := spec.node.Run(true, true); err != nil {
		return err
	}

	waitLog := spec.waitLog
	if waitLog == "" {
		return nil
	}

	var g errgroup.Group
	g.Go(func() error {
		return spec.node.WaitForLog(waitLog, testframework.TIMEOUT)
	})

	if spec.checkExitLog {
		g.Go(func() error {
			err := spec.node.WaitForLog("Exited with error", 30*time.Second)
			if err == nil {
				return fmt.Errorf("lightningd exited with error")
			}
			return nil
		})
	}

	return g.Wait()
}

type lndNodeSpec struct {
	node           *testframework.LndNode
	extraConfig    map[string]string
	failurePrinter logPrinter
}

type LndOption func(*lndNodeSpec)

func WithLndExtraConfig(cfg map[string]string) LndOption {
	return func(spec *lndNodeSpec) {
		spec.extraConfig = copyStringMap(cfg)
	}
}

func WithLndFailurePrinter(printer logPrinter) LndOption {
	return func(spec *lndNodeSpec) {
		spec.failurePrinter = printer
	}
}

func (b *HarnessBuilder) AddLndNode(index int, opts ...LndOption) *testframework.LndNode {
	b.t.Helper()

	spec := &lndNodeSpec{
		extraConfig:    map[string]string{},
		failurePrinter: printFailed,
	}

	for _, opt := range opts {
		opt(spec)
	}

	node, err := testframework.NewLndNode(b.testDir, b.Bitcoind(), index, spec.extraConfig)
	require.NoError(b.t, err, "failed to create lnd node")
	b.t.Cleanup(node.Kill)

	if spec.failurePrinter != nil {
		printer := spec.failurePrinter
		b.t.Cleanup(func() { printer(b.t, node.DaemonProcess) })
	}

	spec.node = node
	b.lndSpecs = append(b.lndSpecs, spec)
	return node
}

func (spec *lndNodeSpec) start() error {
	return spec.node.Run(true, true)
}

type peerswapdSpec struct {
	daemon         *PeerSwapd
	waitForReady   bool
	waitLog        string
	failurePrinter logPrinter
}

type PeerSwapdOption func(*peerswapdSpec)

func WithPeerSwapdWaitForReady(wait bool) PeerSwapdOption {
	return func(spec *peerswapdSpec) {
		spec.waitForReady = wait
	}
}

func WithPeerSwapdWaitLog(log string) PeerSwapdOption {
	return func(spec *peerswapdSpec) {
		spec.waitLog = log
	}
}

func WithPeerSwapdFailurePrinter(printer logPrinter) PeerSwapdOption {
	return func(spec *peerswapdSpec) {
		spec.failurePrinter = printer
	}
}

func (b *HarnessBuilder) AddPeerSwapd(index int, lnd *testframework.LndNode, extraConfig map[string]string, opts ...PeerSwapdOption) *PeerSwapd {
	b.t.Helper()

	spec := &peerswapdSpec{
		waitForReady:   true,
		waitLog:        "peerswapd grpc listening on",
		failurePrinter: printFailed,
	}

	for _, opt := range opts {
		opt(spec)
	}

	lndCfg := &LndConfig{
		LndHost:      fmt.Sprintf("localhost:%d", lnd.RpcPort),
		TlsPath:      lnd.TlsPath,
		MacaroonPath: lnd.MacaroonPath,
	}

	daemon, err := NewPeerSwapd(b.testDir, b.peerswapdBinary, lndCfg, extraConfig, index)
	require.NoError(b.t, err, "failed to create peerswapd")
	b.t.Cleanup(daemon.Kill)

	if spec.failurePrinter != nil {
		printer := spec.failurePrinter
		b.t.Cleanup(func() { printer(b.t, daemon.DaemonProcess) })
	}

	spec.daemon = daemon
	b.peerswapSpecs = append(b.peerswapSpecs, spec)
	return daemon
}

func (spec *peerswapdSpec) start() error {
	if err := spec.daemon.Run(spec.waitForReady); err != nil {
		return err
	}

	if spec.waitLog != "" {
		if err := spec.daemon.WaitForLog(spec.waitLog, testframework.TIMEOUT); err != nil {
			return err
		}
	}

	return nil
}

func (b *HarnessBuilder) Start() {
	b.t.Helper()

	if b.started {
		return
	}

	b.EnsureBitcoindStarted()

	for _, spec := range b.clnSpecs {
		require.NoError(b.t, spec.start(), "failed to start cln node")
	}

	for _, spec := range b.lndSpecs {
		require.NoError(b.t, spec.start(), "failed to start lnd node")
	}

	for _, spec := range b.peerswapSpecs {
		require.NoError(b.t, spec.start(), "failed to start peerswapd")
	}

	b.started = true
}

func defaultClnPolicy() []byte {
	return []byte("accept_all_peers=1\nmin_swap_amount_msat=1\n")
}

func copyBytes(in []byte) []byte {
	if in == nil {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}

func copyStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
