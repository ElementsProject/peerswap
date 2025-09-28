package test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/clightning"
	"github.com/elementsproject/peerswap/peerswaprpc"
	"github.com/elementsproject/peerswap/testframework"
	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"
)

const (
	BitcoinCsv      = 1008
	BitcoinConfirms = 3

	LiquidCsv      = 60
	LiquidConfirms = 2
)

// makeTestDataDir creates a temporary directory for test data with proper cleanup.
// It uses os.MkdirTemp() instead of t.TempDir() to avoid problems with long unix
// socket paths. See https://github.com/golang/go/issues/62614.
func makeTestDataDir(t *testing.T) string {
	// 1. Check for custom test directory from environment
	if baseDir := os.Getenv("PEERSWAP_TEST_DIR"); baseDir != "" {
		testDir := filepath.Join(baseDir, fmt.Sprintf("t%d", time.Now().UnixNano()))
		err := os.MkdirAll(testDir, 0755)
		require.NoError(t, err, "failed to create test dir in PEERSWAP_TEST_DIR")
		t.Cleanup(func() { os.RemoveAll(testDir) })
		return testDir
	}

	// 2. Try to use /tmp/ps/ for shorter paths
	shortBase := "/tmp/ps"
	if err := os.MkdirAll(shortBase, 0755); err == nil {
		// Use process ID and timestamp for uniqueness
		testDir := filepath.Join(shortBase, fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano()%1000000))
		if err := os.MkdirAll(testDir, 0755); err == nil {
			t.Cleanup(func() { os.RemoveAll(testDir) })
			return testDir
		}
	}

	// 3. Fallback to standard temp directory with short prefix
	tempDir, err := os.MkdirTemp("", "ps-")
	require.NoError(t, err, "os.MkdirTemp failed")
	t.Cleanup(func() { os.RemoveAll(tempDir) })
	return tempDir
}

func mustToml(t testing.TB, v interface{}) []byte {
	t.Helper()

	data, err := toml.Marshal(v)
	require.NoError(t, err)
	return data
}

type fundingNode string

const (
	FUNDER_LND fundingNode = "lnd"
	FUNDER_CLN fundingNode = "cln"
)

func clnclnSetup(t *testing.T, fundAmt uint64) (*testframework.BitcoinNode, []*testframework.CLightningNode, string) {
	return clnclnSetupWithConfig(t, fundAmt, 0, nil, true, nil)
}

func clnclnSetupWithConfig(t *testing.T, fundAmt, pushAmt uint64, clnConf []string, waitForActiveChannel bool, policyConf []byte) (*testframework.BitcoinNode, []*testframework.CLightningNode, string) {
	if len(clnConf) == 0 {
		clnConf = []string{
			"--dev-bitcoind-poll=1",
			"--dev-fast-gossip",
			"--large-channels",
		}
	}
	if policyConf == nil {
		policyConf = defaultClnPolicy()
	}

	builder := NewHarnessBuilder(t)
	bitcoind := builder.Bitcoind()

	var lightningds []*testframework.CLightningNode
	for i := 1; i <= 2; i++ {
		node := builder.AddCLightningNode(i,
			WithClnPolicy(policyConf),
			WithClnExtraArgs(clnConf...),
		)
		lightningds = append(lightningds, node)
	}

	builder.Start()

	scid, err := lightningds[0].OpenChannel(lightningds[1], fundAmt, pushAmt, true, true, waitForActiveChannel)
	require.NoError(t, err)

	_, err = lightningds[1].FundWallet(fundAmt, true)
	require.NoError(t, err)

	err = syncPoll(&clnPollableNode{lightningds[0]}, &clnPollableNode{lightningds[1]})
	require.NoError(t, err)

	return bitcoind, lightningds, scid
}

func lndlndSetup(t *testing.T, fundAmt uint64) (*testframework.BitcoinNode, []*testframework.LndNode, []*PeerSwapd, string) {
	builder := NewHarnessBuilder(t)
	bitcoind := builder.Bitcoind()

	extraConfig := map[string]string{"protocol.wumbo-channels": "true"}

	var lightningds []*testframework.LndNode
	for i := 1; i <= 2; i++ {
		lightningds = append(lightningds, builder.AddLndNode(i, WithLndExtraConfig(extraConfig)))
	}

	var peerswapds []*PeerSwapd
	for i, lightningd := range lightningds {
		peerswapds = append(peerswapds, builder.AddPeerSwapd(i+1, lightningd, nil))
	}

	builder.Start()

	scid, err := lightningds[0].OpenChannel(lightningds[1], fundAmt, 0, true, true, true)
	require.NoError(t, err)

	_, err = lightningds[1].FundWallet(10*fundAmt, true)
	require.NoError(t, err)

	err = syncPoll(&peerswapPollableNode{peerswapds[0], lightningds[0].Id()}, &peerswapPollableNode{peerswapds[1], lightningds[1].Id()})
	require.NoError(t, err)

	return bitcoind, lightningds, peerswapds, scid
}

func mixedSetup(t *testing.T, fundAmt uint64, funder fundingNode) (*testframework.BitcoinNode, []testframework.LightningNode, *PeerSwapd, string) {
	builder := NewHarnessBuilder(t)
	bitcoind := builder.Bitcoind()

	defaultPolicy := []byte("accept_all_peers=1\n")
	cln := builder.AddCLightningNode(1,
		WithClnPolicy(defaultPolicy),
		WithClnExtraArgs("--dev-bitcoind-poll=1", "--dev-fast-gossip", "--large-channels"),
	)
	extraConfig := map[string]string{"protocol.wumbo-channels": "true"}
	lnd := builder.AddLndNode(1, WithLndExtraConfig(extraConfig))
	peerswapd := builder.AddPeerSwapd(1, lnd, nil)

	builder.Start()

	var lightningds []testframework.LightningNode
	switch funder {
	case FUNDER_CLN:
		lightningds = append(lightningds, cln, lnd)
	case FUNDER_LND:
		lightningds = append(lightningds, lnd, cln)
	default:
		t.Fatalf("unknown fundingNode %s", funder)
	}

	scid, err := lightningds[0].OpenChannel(lightningds[1], fundAmt, 0, true, true, true)
	require.NoError(t, err)

	_, err = lightningds[1].FundWallet(10*fundAmt, true)
	require.NoError(t, err)

	err = syncPoll(&clnPollableNode{cln}, &peerswapPollableNode{peerswapd, lnd.Id()})
	require.NoError(t, err)

	return bitcoind, lightningds, peerswapd, scid
}

func clnclnElementsSetup(t *testing.T, fundAmt uint64) (*testframework.BitcoinNode, *testframework.LiquidNode, []*CLightningNodeWithLiquid, string) {
	builder := NewHarnessBuilder(t)
	bitcoind := builder.Bitcoind()

	liquidd, err := testframework.NewLiquidNode(builder.TestDir(), bitcoind, 1)
	if err != nil {
		t.Fatal("error creating liquidd node", err)
	}
	t.Cleanup(liquidd.Kill)

	var lightningds []*testframework.CLightningNode
	for i := 1; i <= 2; i++ {
		walletName := fmt.Sprintf("swap%d", i)
		cfg := mustToml(t, struct {
			Liquid struct {
				RpcUser     string
				RpcPassword string
				RpcHost     string
				RpcPort     uint
				RpcWallet   string
				Enabled     bool
			}
		}{
			Liquid: struct {
				RpcUser     string
				RpcPassword string
				RpcHost     string
				RpcPort     uint
				RpcWallet   string
				Enabled     bool
			}{
				RpcUser:     liquidd.RpcUser,
				RpcPassword: liquidd.RpcPassword,
				RpcHost:     "http://127.0.0.1",
				RpcPort:     uint(liquidd.RpcPort),
				RpcWallet:   walletName,
				Enabled:     true,
			},
		})

		node := builder.AddCLightningNode(i,
			WithClnPolicy([]byte("accept_all_peers=1\n")),
			WithClnConfig(cfg),
			WithClnExtraArgs("--dev-bitcoind-poll=1", "--dev-fast-gossip", "--large-channels"),
		)
		lightningds = append(lightningds, node)
	}

	builder.EnsureBitcoindStarted()
	require.NoError(t, liquidd.Run(true))

	builder.Start()

	for _, lightningd := range lightningds {
		var result peerswaprpc.GetAddressResponse
		require.NoError(t, lightningd.Rpc.Request(&clightning.LiquidGetAddress{}, &result))
		require.NoError(t, liquidd.GenerateBlocks(20))
		_, err = liquidd.Rpc.Call("sendtoaddress", result.Address, 10., "", "", false, false, 1, "UNSET")
		require.NoError(t, err)
	}

	_, err = liquidd.Rpc.Call("generatetoaddress", 1, testframework.LBTC_BURN)
	require.NoError(t, err)

	scid, err := lightningds[0].OpenChannel(lightningds[1], fundAmt, 0, true, true, true)
	if err != nil {
		t.Fatalf("lightingds[0].OpenChannel() %v", err)
	}

	var result interface{}
	require.NoError(t, lightningds[0].Rpc.Request(&clightning.ReloadPolicyFile{}, &result))
	require.NoError(t, lightningds[1].Rpc.Request(&clightning.ReloadPolicyFile{}, &result))

	require.NoError(t, syncPoll(&clnPollableNode{lightningds[0]}, &clnPollableNode{lightningds[1]}))

	return bitcoind, liquidd, []*CLightningNodeWithLiquid{{lightningds[0]}, {lightningds[1]}}, scid
}

func lndlndElementsSetup(t *testing.T, fundAmt uint64) (*testframework.BitcoinNode, *testframework.LiquidNode, []*LndNodeWithLiquid, []*PeerSwapd, string) {
	builder := NewHarnessBuilder(t)
	bitcoind := builder.Bitcoind()

	liquidd, err := testframework.NewLiquidNode(builder.TestDir(), bitcoind, 1)
	if err != nil {
		t.Fatal("error creating liquidd node", err)
	}
	t.Cleanup(liquidd.Kill)

	var (
		lightningds []*testframework.LndNode
		peerswapds  []*PeerSwapd
	)

	extraLnd := map[string]string{"protocol.wumbo-channels": "true"}
	for i := 1; i <= 2; i++ {
		lnd := builder.AddLndNode(i, WithLndExtraConfig(extraLnd), WithLndFailurePrinter(printFailedFiltered))
		lightningds = append(lightningds, lnd)

		extraConfig := map[string]string{
			"elementsd.rpcuser":   liquidd.RpcUser,
			"elementsd.rpcpass":   liquidd.RpcPassword,
			"elementsd.rpchost":   "http://127.0.0.1",
			"elementsd.rpcport":   fmt.Sprintf("%d", liquidd.RpcPort),
			"elementsd.rpcwallet": fmt.Sprintf("swap-test-wallet-%d", i),
		}

		peerswapds = append(peerswapds, builder.AddPeerSwapd(i+1, lnd, extraConfig))
	}

	builder.EnsureBitcoindStarted()
	require.NoError(t, liquidd.Run(true))

	builder.Start()

	// Give liquid funds to nodes to have something to swap.
	for _, peerswapd := range peerswapds {
		r, err := peerswapd.PeerswapClient.LiquidGetAddress(context.Background(), &peerswaprpc.GetAddressRequest{})
		require.NoError(t, err)
		require.NoError(t, liquidd.GenerateBlocks(20))
		_, err = liquidd.Rpc.Call("sendtoaddress", r.Address, 10., "", "", false, false, 1, "UNSET")
		require.NoError(t, err)
	}

	// Lock txs.
	_, err = liquidd.Rpc.Call("generatetoaddress", 1, testframework.LBTC_BURN)
	require.NoError(t, err)

	// Setup channel ([0] fundAmt(10^7) ---- 0 [1])
	scid, err := lightningds[0].OpenChannel(lightningds[1], fundAmt, 0, true, true, true)
	if err != nil {
		t.Fatalf("lightingds[0].OpenChannel() %v", err)
	}

	// Give btc to node [1] in order to initiate swap-in.
	_, err = lightningds[1].FundWallet(10*fundAmt, true)
	if err != nil {
		t.Fatalf("lightningds[1].FundWallet() %v", err)
	}

	syncPoll(&peerswapPollableNode{peerswapds[0], lightningds[0].Id()}, &peerswapPollableNode{peerswapds[1], lightningds[1].Id()})
	return bitcoind, liquidd, []*LndNodeWithLiquid{{lightningds[0], peerswapds[0]}, {lightningds[1], peerswapds[1]}}, peerswapds, scid
}

func mixedElementsSetup(t *testing.T, fundAmt uint64, funder fundingNode) (*testframework.BitcoinNode, *testframework.LiquidNode, []testframework.LightningNode, *PeerSwapd, string) {
	builder := NewHarnessBuilder(t)
	bitcoind := builder.Bitcoind()

	liquidd, err := testframework.NewLiquidNode(builder.TestDir(), bitcoind, 1)
	if err != nil {
		t.Fatal("error creating liquidd node", err)
	}
	t.Cleanup(liquidd.Kill)

	clnConfig := mustToml(t, struct {
		Liquid struct {
			RpcUser     string
			RpcPassword string
			RpcHost     string
			RpcPort     uint
			RpcWallet   string
		}
	}{
		Liquid: struct {
			RpcUser     string
			RpcPassword string
			RpcHost     string
			RpcPort     uint
			RpcWallet   string
		}{
			RpcUser:     liquidd.RpcUser,
			RpcPassword: liquidd.RpcPassword,
			RpcHost:     "http://127.0.0.1",
			RpcPort:     uint(liquidd.RpcPort),
			RpcWallet:   "cln-test-wallet-1",
		},
	})

	cln := builder.AddCLightningNode(1,
		WithClnPolicy([]byte("accept_all_peers=1\n")),
		WithClnConfig(clnConfig),
		WithClnExtraArgs("--dev-bitcoind-poll=1", "--dev-fast-gossip", "--large-channels"),
	)

	extraConfigLnd := map[string]string{"protocol.wumbo-channels": "true"}
	lnd := builder.AddLndNode(1, WithLndExtraConfig(extraConfigLnd))

	peerswapd := builder.AddPeerSwapd(1, lnd, map[string]string{
		"elementsd.rpcuser":   liquidd.RpcUser,
		"elementsd.rpcpass":   liquidd.RpcPassword,
		"elementsd.rpchost":   "http://127.0.0.1",
		"elementsd.rpcport":   fmt.Sprintf("%d", liquidd.RpcPort),
		"elementsd.rpcwallet": "swap-test-wallet-1",
	}, WithPeerSwapdFailurePrinter(printFailed))

	builder.EnsureBitcoindStarted()
	require.NoError(t, liquidd.Run(true))

	builder.Start()

	var lar peerswaprpc.GetAddressResponse
	require.NoError(t, cln.Rpc.Request(&clightning.LiquidGetAddress{}, &lar))
	require.NoError(t, liquidd.GenerateBlocks(20))
	_, err = liquidd.Rpc.Call("sendtoaddress", lar.GetAddress(), 10., "", "", false, false, 1, "UNSET")
	require.NoError(t, err)

	r, err := peerswapd.PeerswapClient.LiquidGetAddress(context.Background(), &peerswaprpc.GetAddressRequest{})
	require.NoError(t, err)
	require.NoError(t, liquidd.GenerateBlocks(20))
	_, err = liquidd.Rpc.Call("sendtoaddress", r.Address, 10., "", "", false, false, 1, "UNSET")
	require.NoError(t, err)

	_, err = liquidd.Rpc.Call("generatetoaddress", 1, testframework.LBTC_BURN)
	require.NoError(t, err)

	var lightningds []testframework.LightningNode
	switch funder {
	case FUNDER_CLN:
		lightningds = append(lightningds, &CLightningNodeWithLiquid{cln})
		lightningds = append(lightningds, &LndNodeWithLiquid{lnd, peerswapd})
	case FUNDER_LND:
		lightningds = append(lightningds, &LndNodeWithLiquid{lnd, peerswapd})
		lightningds = append(lightningds, &CLightningNodeWithLiquid{cln})
	default:
		t.Fatalf("unknown fundingNode %s", funder)
	}

	scid, err := lightningds[0].OpenChannel(lightningds[1], fundAmt, 0, true, true, true)
	if err != nil {
		t.Fatalf("cln.OpenChannel() %v", err)
	}

	require.NoError(t, syncPoll(&clnPollableNode{cln}, &peerswapPollableNode{peerswapd, lnd.Id()}))

	return bitcoind, liquidd, lightningds, peerswapd, scid
}

type CLightningNodeWithLiquid struct {
	*testframework.CLightningNode
}

func (n *CLightningNodeWithLiquid) GetBtcBalanceSat() (uint64, error) {
	var response peerswaprpc.GetBalanceResponse
	err := n.Rpc.Request(&clightning.LiquidGetBalance{}, &response)
	if err != nil {
		return 0, err
	}
	return response.GetSatAmount(), nil
}

type LndNodeWithLiquid struct {
	*testframework.LndNode
	ps *PeerSwapd
}

func (n *LndNodeWithLiquid) GetBtcBalanceSat() (uint64, error) {
	r, err := n.ps.PeerswapClient.LiquidGetBalance(context.Background(), &peerswaprpc.GetBalanceRequest{})
	if err != nil {
		return 0, err
	}
	return r.SatAmount, nil
}

func clnclnLWKSetup(t *testing.T, fundAmt uint64) (
	*testframework.BitcoinNode,
	*testframework.LiquidNode,
	[]*CLightningNodeWithLiquid,
	string,
	*testframework.Electrs,
	*testframework.LWK,
) {
	builder := NewHarnessBuilder(t)
	bitcoind := builder.Bitcoind()

	liquidd, err := testframework.NewLiquidNode(builder.TestDir(), bitcoind, 1)
	if err != nil {
		t.Fatal("error creating liquidd node", err)
	}
	t.Cleanup(liquidd.Kill)

	electrsd, err := testframework.NewElectrs(builder.TestDir(), 1, liquidd)
	if err != nil {
		t.Fatal("error creating electrsd node", err)
	}
	t.Cleanup(electrsd.Process.Kill)

	lwk, err := testframework.NewLWK(builder.TestDir(), 1, electrsd)
	if err != nil {
		t.Fatal("error creating electrsd node", err)
	}
	t.Cleanup(lwk.Process.Kill)

	var lightningds []*testframework.CLightningNode
	for i := 1; i <= 2; i++ {
		walletName := fmt.Sprintf("swap%d", i)
		cfg := mustToml(t, struct {
			LWK struct {
				SignerName       string
				WalletName       string
				LWKEndpoint      string
				ElectrumEndpoint string
				Network          string
				LiquidSwaps      bool
			}
		}{
			LWK: struct {
				SignerName       string
				WalletName       string
				LWKEndpoint      string
				ElectrumEndpoint string
				Network          string
				LiquidSwaps      bool
			}{
				SignerName:       walletName + "-" + "signer",
				WalletName:       walletName,
				LWKEndpoint:      lwk.RPCURL.String(),
				ElectrumEndpoint: electrsd.RPCURL.String(),
				Network:          "liquid-regtest",
				LiquidSwaps:      true,
			},
		})

		node := builder.AddCLightningNode(i,
			WithClnPolicy([]byte("accept_all_peers=1\n")),
			WithClnConfig(cfg),
			WithClnExtraArgs("--dev-bitcoind-poll=1", "--dev-fast-gossip", "--large-channels"),
		)
		lightningds = append(lightningds, node)
	}

	builder.EnsureBitcoindStarted()
	require.NoError(t, liquidd.Run(true))

	ctx, cancel := context.WithTimeout(context.Background(), testframework.TIMEOUT)
	defer cancel()
	require.NoError(t, electrsd.Run(ctx))
	lwk.Process.Run()

	builder.Start()

	for _, lightningd := range lightningds {
		var result peerswaprpc.GetAddressResponse
		require.NoError(t, lightningd.Rpc.Request(&clightning.LiquidGetAddress{}, &result))
		require.NoError(t, liquidd.GenerateBlocks(20))
		_, err = liquidd.Rpc.Call("sendtoaddress", result.Address, 10., "", "", false, false, 1, "UNSET")
		require.NoError(t, err)
		require.NoError(t,
			testframework.WaitFor(func() bool {
				var balance peerswaprpc.GetBalanceResponse
				require.NoError(t, lightningd.Rpc.Request(&clightning.LiquidGetBalance{}, &balance))
				return balance.GetSatAmount() >= 1000000000
			}, testframework.TIMEOUT))
	}

	_, err = liquidd.Rpc.Call("generatetoaddress", 1, testframework.LBTC_BURN)
	require.NoError(t, err)
	require.NoError(t, liquidd.GenerateBlocks(20))

	scid, err := lightningds[0].OpenChannel(lightningds[1], fundAmt, 0, true, true, true)
	if err != nil {
		t.Fatalf("lightingds[0].OpenChannel() %v", err)
	}

	var result interface{}
	require.NoError(t, lightningds[0].Rpc.Request(&clightning.ReloadPolicyFile{}, &result))
	require.NoError(t, lightningds[1].Rpc.Request(&clightning.ReloadPolicyFile{}, &result))

	require.NoError(t, syncPoll(&clnPollableNode{lightningds[0]}, &clnPollableNode{lightningds[1]}))

	return bitcoind, liquidd, []*CLightningNodeWithLiquid{{lightningds[0]}, {lightningds[1]}}, scid, electrsd, lwk
}

func lndlndLWKSetup(t *testing.T, fundAmt uint64) (
	*testframework.BitcoinNode,
	*testframework.LiquidNode,
	[]*LndNodeWithLiquid,
	[]*PeerSwapd,
	string,
	*testframework.Electrs,
	*testframework.LWK,
) {
	builder := NewHarnessBuilder(t)
	bitcoind := builder.Bitcoind()

	liquidd, err := testframework.NewLiquidNode(builder.TestDir(), bitcoind, 1)
	if err != nil {
		t.Fatal("error creating liquidd node", err)
	}
	t.Cleanup(liquidd.Kill)

	electrsd, err := testframework.NewElectrs(builder.TestDir(), 1, liquidd)
	if err != nil {
		t.Fatal("error creating electrsd node", err)
	}
	t.Cleanup(electrsd.Process.Kill)

	lwk, err := testframework.NewLWK(builder.TestDir(), 1, electrsd)
	if err != nil {
		t.Fatal("error creating lwk node", err)
	}
	t.Cleanup(lwk.Process.Kill)

	var (
		lightningds []*testframework.LndNode
		peerswapds  []*PeerSwapd
	)

	extraConfig := map[string]string{"protocol.wumbo-channels": "true"}
	for i := 1; i <= 2; i++ {
		lnd := builder.AddLndNode(i, WithLndExtraConfig(extraConfig), WithLndFailurePrinter(printFailedFiltered))
		lightningds = append(lightningds, lnd)

		walletName := fmt.Sprintf("swap%d", i)
		extra := map[string]string{
			"lwk.signername":       walletName + "-" + "signer",
			"lwk.walletname":       walletName,
			"lwk.lwkendpoint":      lwk.RPCURL.String(),
			"lwk.elementsendpoint": electrsd.RPCURL.String(),
			"lwk.network":          "liquid-regtest",
			"lwk.liquidswaps":      "true",
		}

		peerswapds = append(peerswapds, builder.AddPeerSwapd(i+1, lnd, extra))
	}

	builder.EnsureBitcoindStarted()
	require.NoError(t, liquidd.Run(true))

	ctx, cancel := context.WithTimeout(context.Background(), testframework.TIMEOUT)
	defer cancel()
	require.NoError(t, electrsd.Run(ctx))
	lwk.Process.Run()

	builder.Start()

	for _, peerswapd := range peerswapds {
		r, err := peerswapd.PeerswapClient.LiquidGetAddress(context.Background(), &peerswaprpc.GetAddressRequest{})
		require.NoError(t, err)
		_, err = liquidd.Rpc.Call("sendtoaddress", r.Address, 10., "", "", false, false, 1, "UNSET")
		require.NoError(t, err)
		require.NoError(t, liquidd.GenerateBlocks(20))
		require.NoError(t,
			testframework.WaitFor(func() bool {
				b, err := peerswapd.PeerswapClient.LiquidGetBalance(ctx, &peerswaprpc.GetBalanceRequest{})
				require.NoError(t, err)
				return b.GetSatAmount() >= 1000000000
			}, testframework.TIMEOUT))
	}

	_, err = liquidd.Rpc.Call("generatetoaddress", 1, testframework.LBTC_BURN)
	require.NoError(t, err)

	scid, err := lightningds[0].OpenChannel(lightningds[1], fundAmt, 0, true, true, true)
	if err != nil {
		t.Fatalf("lightingds[0].OpenChannel() %v", err)
	}

	_, err = lightningds[1].FundWallet(10*fundAmt, true)
	if err != nil {
		t.Fatalf("lightningds[1].FundWallet() %v", err)
	}

	require.NoError(t, syncPoll(&peerswapPollableNode{peerswapds[0], lightningds[0].Id()}, &peerswapPollableNode{peerswapds[1], lightningds[1].Id()}))

	return bitcoind, liquidd, []*LndNodeWithLiquid{{lightningds[0], peerswapds[0]}, {lightningds[1], peerswapds[1]}}, peerswapds, scid, electrsd, lwk
}

func mixedLWKSetup(t *testing.T, fundAmt uint64, funder fundingNode) (
	*testframework.BitcoinNode,
	*testframework.LiquidNode,
	[]testframework.LightningNode,
	*PeerSwapd,
	string,
	*testframework.Electrs,
	*testframework.LWK,
) {
	builder := NewHarnessBuilder(t)
	bitcoind := builder.Bitcoind()

	liquidd, err := testframework.NewLiquidNode(builder.TestDir(), bitcoind, 1)
	if err != nil {
		t.Fatal("error creating liquidd node", err)
	}
	t.Cleanup(liquidd.Kill)

	electrsd, err := testframework.NewElectrs(builder.TestDir(), 1, liquidd)
	if err != nil {
		t.Fatal("error creating electrsd node", err)
	}
	t.Cleanup(electrsd.Process.Kill)

	lwk, err := testframework.NewLWK(builder.TestDir(), 1, electrsd)
	if err != nil {
		t.Fatal("error creating lwk node", err)
	}
	t.Cleanup(lwk.Process.Kill)

	clnCfg := mustToml(t, struct {
		LWK struct {
			SignerName       string
			WalletName       string
			LWKEndpoint      string
			ElectrumEndpoint string
			Network          string
			LiquidSwaps      bool
		}
	}{
		LWK: struct {
			SignerName       string
			WalletName       string
			LWKEndpoint      string
			ElectrumEndpoint string
			Network          string
			LiquidSwaps      bool
		}{
			SignerName:       "cln-test-wallet-1-signer",
			WalletName:       "cln-test-wallet-1",
			LWKEndpoint:      lwk.RPCURL.String(),
			ElectrumEndpoint: electrsd.RPCURL.String(),
			Network:          "liquid-regtest",
			LiquidSwaps:      true,
		},
	})

	cln := builder.AddCLightningNode(1,
		WithClnPolicy([]byte("accept_all_peers=1\n")),
		WithClnConfig(clnCfg),
		WithClnExtraArgs("--dev-bitcoind-poll=1", "--dev-fast-gossip", "--large-channels"),
	)

	lnd := builder.AddLndNode(1, WithLndExtraConfig(map[string]string{"protocol.wumbo-channels": "true"}))

	peerswapd := builder.AddPeerSwapd(1, lnd, map[string]string{
		"lwk.signername":       "lnd-test-wallet-1-signer",
		"lwk.walletname":       "lnd-test-wallet-1",
		"lwk.lwkendpoint":      lwk.RPCURL.String(),
		"lwk.elementsendpoint": electrsd.RPCURL.String(),
		"lwk.network":          "liquid-regtest",
		"lwk.liquidswaps":      "true",
	}, WithPeerSwapdFailurePrinter(printFailed))

	builder.EnsureBitcoindStarted()
	require.NoError(t, liquidd.Run(true))

	ctx, cancel := context.WithTimeout(context.Background(), testframework.TIMEOUT)
	defer cancel()
	require.NoError(t, electrsd.Run(ctx))
	lwk.Process.Run()

	builder.Start()

	var lar peerswaprpc.GetAddressResponse
	require.NoError(t, cln.Rpc.Request(&clightning.LiquidGetAddress{}, &lar))
	require.NoError(t, liquidd.GenerateBlocks(20))
	_, err = liquidd.Rpc.Call("sendtoaddress", lar.GetAddress(), 10., "", "", false, false, 1, "UNSET")
	require.NoError(t, err)
	require.NoError(t,
		testframework.WaitFor(func() bool {
			var balance peerswaprpc.GetBalanceResponse
			require.NoError(t, cln.Rpc.Request(&clightning.LiquidGetBalance{}, &balance))
			return balance.GetSatAmount() >= 1000000000
		}, testframework.TIMEOUT))

	r, err := peerswapd.PeerswapClient.LiquidGetAddress(context.Background(), &peerswaprpc.GetAddressRequest{})
	require.NoError(t, err)
	require.NoError(t, liquidd.GenerateBlocks(20))
	_, err = liquidd.Rpc.Call("sendtoaddress", r.Address, 10., "", "", false, false, 1, "UNSET")
	require.NoError(t, err)
	require.NoError(t,
		testframework.WaitFor(func() bool {
			b, err := peerswapd.PeerswapClient.LiquidGetBalance(context.Background(), &peerswaprpc.GetBalanceRequest{})
			require.NoError(t, err)
			return b.GetSatAmount() >= 1000000000
		}, testframework.TIMEOUT))

	_, err = liquidd.Rpc.Call("generatetoaddress", 1, testframework.LBTC_BURN)
	require.NoError(t, err)

	var lightningds []testframework.LightningNode
	switch funder {
	case FUNDER_CLN:
		lightningds = append(lightningds, &CLightningNodeWithLiquid{cln})
		lightningds = append(lightningds, &LndNodeWithLiquid{lnd, peerswapd})
	case FUNDER_LND:
		lightningds = append(lightningds, &LndNodeWithLiquid{lnd, peerswapd})
		lightningds = append(lightningds, &CLightningNodeWithLiquid{cln})
	default:
		t.Fatalf("unknown fundingNode %s", funder)
	}

	scid, err := lightningds[0].OpenChannel(lightningds[1], fundAmt, 0, true, true, true)
	if err != nil {
		t.Fatalf("cln.OpenChannel() %v", err)
	}

	require.NoError(t, syncPoll(&clnPollableNode{cln}, &peerswapPollableNode{peerswapd, lnd.Id()}))

	return bitcoind, liquidd, lightningds, peerswapd, scid, electrsd, lwk
}

func clnclnLWKLiquidSetup(t *testing.T, fundAmt uint64) (
	*testframework.BitcoinNode,
	*testframework.LiquidNode,
	[]*CLightningNodeWithLiquid,
	string,
	*testframework.Electrs,
	*testframework.LWK,
) {
	builder := NewHarnessBuilder(t)
	bitcoind := builder.Bitcoind()

	liquidd, err := testframework.NewLiquidNode(builder.TestDir(), bitcoind, 1)
	if err != nil {
		t.Fatal("error creating liquidd node", err)
	}
	t.Cleanup(liquidd.Kill)

	electrsd, err := testframework.NewElectrs(builder.TestDir(), 1, liquidd)
	if err != nil {
		t.Fatal("error creating electrsd node", err)
	}
	t.Cleanup(electrsd.Process.Kill)

	lwk, err := testframework.NewLWK(builder.TestDir(), 1, electrsd)
	if err != nil {
		t.Fatal("error creating lwk node", err)
	}
	t.Cleanup(lwk.Process.Kill)

	liquidCfg := mustToml(t, struct {
		Liquid struct {
			RpcUser     string
			RpcPassword string
			RpcHost     string
			RpcPort     uint
			RpcWallet   string
		}
	}{
		Liquid: struct {
			RpcUser     string
			RpcPassword string
			RpcHost     string
			RpcPort     uint
			RpcWallet   string
		}{
			RpcUser:     liquidd.RpcUser,
			RpcPassword: liquidd.RpcPassword,
			RpcHost:     "http://127.0.0.1",
			RpcPort:     uint(liquidd.RpcPort),
			RpcWallet:   "swapElements",
		},
	})

	lwkCfg := mustToml(t, struct {
		LWK struct {
			SignerName       string
			WalletName       string
			LWKEndpoint      string
			ElectrumEndpoint string
			Network          string
			LiquidSwaps      bool
		}
	}{
		LWK: struct {
			SignerName       string
			WalletName       string
			LWKEndpoint      string
			ElectrumEndpoint string
			Network          string
			LiquidSwaps      bool
		}{
			SignerName:       "swapLWK-signer",
			WalletName:       "swapLWK",
			LWKEndpoint:      lwk.RPCURL.String(),
			ElectrumEndpoint: electrsd.RPCURL.String(),
			Network:          "liquid-regtest",
			LiquidSwaps:      true,
		},
	})

	nodeLiquid := builder.AddCLightningNode(1,
		WithClnPolicy([]byte("accept_all_peers=1\n")),
		WithClnConfig(liquidCfg),
		WithClnExtraArgs("--dev-bitcoind-poll=1", "--dev-fast-gossip", "--large-channels"),
	)

	nodeLWK := builder.AddCLightningNode(2,
		WithClnPolicy([]byte("accept_all_peers=1\n")),
		WithClnConfig(lwkCfg),
		WithClnExtraArgs("--dev-bitcoind-poll=1", "--dev-fast-gossip", "--large-channels"),
	)

	lightningds := []*testframework.CLightningNode{nodeLiquid, nodeLWK}

	builder.EnsureBitcoindStarted()
	require.NoError(t, liquidd.Run(true))

	ctx, cancel := context.WithTimeout(context.Background(), testframework.TIMEOUT)
	defer cancel()
	require.NoError(t, electrsd.Run(ctx))
	lwk.Process.Run()

	builder.Start()

	for _, node := range lightningds {
		var addr peerswaprpc.GetAddressResponse
		require.NoError(t, node.Rpc.Request(&clightning.LiquidGetAddress{}, &addr))
		_, err = liquidd.Rpc.Call("sendtoaddress", addr.GetAddress(), 10., "", "", false, false, 1, "UNSET")
		require.NoError(t, err)
		require.NoError(t, liquidd.GenerateBlocks(20))
		require.NoError(t,
			testframework.WaitFor(func() bool {
				var balance peerswaprpc.GetBalanceResponse
				require.NoError(t, node.Rpc.Request(&clightning.LiquidGetBalance{}, &balance))
				return balance.GetSatAmount() >= 1000000000
			}, testframework.TIMEOUT))
	}

	_, err = liquidd.Rpc.Call("generatetoaddress", 1, testframework.LBTC_BURN)
	require.NoError(t, err)
	require.NoError(t, liquidd.GenerateBlocks(20))

	scid, err := lightningds[0].OpenChannel(lightningds[1], fundAmt, 0, true, true, true)
	if err != nil {
		t.Fatalf("lightingds[0].OpenChannel() %v", err)
	}

	var result interface{}
	require.NoError(t, lightningds[0].Rpc.Request(&clightning.ReloadPolicyFile{}, &result))
	require.NoError(t, lightningds[1].Rpc.Request(&clightning.ReloadPolicyFile{}, &result))

	require.NoError(t, syncPoll(&clnPollableNode{lightningds[0]}, &clnPollableNode{lightningds[1]}))

	return bitcoind, liquidd, []*CLightningNodeWithLiquid{{lightningds[0]}, {lightningds[1]}}, scid, electrsd, lwk
}

func clnSingleElementsSetup(t *testing.T, elementsConfig map[string]string) (*testframework.BitcoinNode, *testframework.LiquidNode, *testframework.CLightningNode) {
	builder := NewHarnessBuilder(t)
	bitcoind := builder.Bitcoind()
	builder.EnsureBitcoindStarted()

	liquidd, err := testframework.NewLiquidNodeFromConfig(builder.TestDir(), bitcoind, elementsConfig, 1)
	if err != nil {
		t.Fatalf("error creating liquidd node: %v", err)
	}
	t.Cleanup(liquidd.Kill)

	require.NoError(t, liquidd.Run(true))

	cfg := mustToml(t, struct {
		Liquid struct {
			RpcUser     string
			RpcPassword string
			RpcHost     string
			RpcPort     uint
			RpcWallet   string
			Enabled     bool
		}
	}{
		Liquid: struct {
			RpcUser     string
			RpcPassword string
			RpcHost     string
			RpcPort     uint
			RpcWallet   string
			Enabled     bool
		}{
			RpcUser:     liquidd.RpcUser,
			RpcPassword: liquidd.RpcPassword,
			RpcHost:     "http://127.0.0.1",
			RpcPort:     uint(liquidd.RpcPort),
			RpcWallet:   "swap1",
			Enabled:     true,
		},
	})

	cln := builder.AddCLightningNode(1,
		WithClnPolicy([]byte("accept_all_peers=1\n")),
		WithClnConfig(cfg),
		WithClnExtraArgs("--dev-bitcoind-poll=1", "--dev-fast-gossip", "--large-channels"),
	)

	return bitcoind, liquidd, cln
}
