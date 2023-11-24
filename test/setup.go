package test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

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

type fundingNode string

const (
	FUNDER_LND fundingNode = "lnd"
	FUNDER_CLN fundingNode = "cln"
)

func clnclnSetup(t *testing.T, fundAmt uint64) (*testframework.BitcoinNode, []*testframework.CLightningNode, string) {
	return clnclnSetupWithConfig(t, fundAmt, 0, []string{
		"--dev-bitcoind-poll=1",
		"--dev-fast-gossip",
		"--large-channels",
	})
}

func clnclnSetupWithConfig(t *testing.T, fundAmt, pushAmt uint64, clnConf []string) (*testframework.BitcoinNode, []*testframework.CLightningNode, string) {
	// Get PeerSwap plugin path and test dir
	_, filename, _, _ := runtime.Caller(0)
	pathToPlugin := filepath.Join(filename, "..", "..", "out", "test-builds", "peerswap")
	testDir := t.TempDir()

	// Setup nodes (1 bitcoind, 2 lightningd)
	bitcoind, err := testframework.NewBitcoinNode(testDir, 1)
	if err != nil {
		t.Fatalf("could not create bitcoind %v", err)
	}
	t.Cleanup(bitcoind.Kill)

	var lightningds []*testframework.CLightningNode
	for i := 1; i <= 2; i++ {
		lightningd, err := testframework.NewCLightningNode(testDir, bitcoind, i)
		if err != nil {
			t.Fatalf("could not create liquidd %v", err)
		}
		t.Cleanup(lightningd.Kill)
		defer printFailedFiltered(t, lightningd.DaemonProcess)

		// Create policy file and accept all peers
		err = os.MkdirAll(filepath.Join(lightningd.GetDataDir(), "peerswap"), os.ModePerm)
		if err != nil {
			t.Fatal("could not create dir", err)
		}
		err = os.WriteFile(filepath.Join(lightningd.GetDataDir(), "peerswap", "policy.conf"),
			[]byte("accept_all_peers=1\nmin_swap_amount_msat=1\n"), os.ModePerm)
		if err != nil {
			t.Fatal("could not create policy file", err)
		}

		// Create config file
		peerswapConfig := ``

		configPath := filepath.Join(lightningd.GetDataDir(), "peerswap", "peerswap.conf")
		os.WriteFile(
			configPath,
			[]byte(peerswapConfig),
			os.ModePerm,
		)

		// Use lightningd with dev flags enabled
		lightningd.WithCmd("lightningd-dev")

		// Add plugin to cmd line options
		lightningd.AppendCmdLine(append([]string{fmt.Sprint("--plugin=", pathToPlugin)}, clnConf...))

		lightningds = append(lightningds, lightningd)
	}

	// Start nodes
	err = bitcoind.Run(true)
	if err != nil {
		t.Fatalf("bitcoind.Run() got err %v", err)
	}

	for _, lightningd := range lightningds {
		err = lightningd.Run(true, true)
		if err != nil {
			t.Fatalf("lightningd.Run() got err %v", err)
		}
		err = lightningd.WaitForLog("peerswap initialized", testframework.TIMEOUT)
		if err != nil {
			t.Fatalf("lightningd.WaitForLog() got err %v", err)
		}
	}

	// Setup channel ([0] fundAmt(10^7) ---- 0 [1])
	scid, err := lightningds[0].OpenChannel(lightningds[1], fundAmt, pushAmt, true, true, true)
	if err != nil {
		t.Fatalf("lightingds[0].OpenChannel() %v", err)
	}

	// Give btc to node [1] in order to initiate swap-in.
	_, err = lightningds[1].FundWallet(fundAmt, true)
	if err != nil {
		t.Fatalf("ListPeers %v", err)
	}

	_ = syncPoll(&clnPollableNode{lightningds[0]}, &clnPollableNode{lightningds[1]})

	return bitcoind, lightningds, scid
}

func lndlndSetup(t *testing.T, fundAmt uint64) (*testframework.BitcoinNode, []*testframework.LndNode, []*PeerSwapd, string) {
	// Get PeerSwap plugin path and test dir
	_, filename, _, _ := runtime.Caller(0)
	pathToPlugin := filepath.Join(filename, "..", "..", "out", "test-builds", "peerswapd")
	testDir := t.TempDir()

	// Setup nodes (1 bitcoind, 2 lightningd, 2 peerswapd)
	bitcoind, err := testframework.NewBitcoinNode(testDir, 1)
	if err != nil {
		t.Fatalf("could not create bitcoind %v", err)
	}
	t.Cleanup(bitcoind.Kill)

	var lightningds []*testframework.LndNode
	for i := 1; i <= 2; i++ {
		extraConfig := map[string]string{"protocol.wumbo-channels": "true"}
		lightningd, err := testframework.NewLndNode(testDir, bitcoind, i, extraConfig)
		if err != nil {
			t.Fatalf("could not create liquidd %v", err)
		}
		t.Cleanup(lightningd.Kill)

		lightningds = append(lightningds, lightningd)
	}

	var peerswapds []*PeerSwapd
	for i, lightningd := range lightningds {
		peerswapd, err := NewPeerSwapd(testDir, pathToPlugin, &LndConfig{LndHost: fmt.Sprintf("localhost:%d", lightningd.RpcPort), TlsPath: lightningd.TlsPath, MacaroonPath: lightningd.MacaroonPath}, nil, i+1)
		if err != nil {
			t.Fatalf("could not create peerswapd %v", err)
		}
		t.Cleanup(peerswapd.Kill)
		defer printFailed(t, lightningd.DaemonProcess)

		peerswapds = append(peerswapds, peerswapd)
	}

	// Start nodes
	err = bitcoind.Run(true)
	if err != nil {
		t.Fatalf("bitcoind.Run() got err %v", err)
	}

	for _, lightningd := range lightningds {
		err = lightningd.Run(true, true)
		if err != nil {
			t.Fatalf("lightningd.Run() got err %v", err)
		}
	}

	for _, peerswapd := range peerswapds {
		err = peerswapd.Run(true)
		if err != nil {
			t.Fatalf("peerswapd.Run() got err %v", err)
		}
		err = peerswapd.WaitForLog("peerswapd grpc listening on", testframework.TIMEOUT)
		if err != nil {
			t.Fatalf("peerswapd.WaitForLog() got err %v", err)
		}
	}

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
	return bitcoind, lightningds, peerswapds, scid
}

func mixedSetup(t *testing.T, fundAmt uint64, funder fundingNode) (*testframework.BitcoinNode, []testframework.LightningNode, *PeerSwapd, string) {
	// Get PeerSwap plugin path and test dir
	_, filename, _, _ := runtime.Caller(0)
	peerswapdPath := filepath.Join(filename, "..", "..", "out", "test-builds", "peerswapd")
	peerswapPluginPath := filepath.Join(filename, "..", "..", "out", "test-builds", "peerswap")
	testDir := t.TempDir()

	// Setup nodes (1 bitcoind, 1 cln, 1 lnd, 1 peerswapd)
	bitcoind, err := testframework.NewBitcoinNode(testDir, 1)
	if err != nil {
		t.Fatalf("could not create bitcoind %v", err)
	}
	t.Cleanup(bitcoind.Kill)

	// cln
	cln, err := testframework.NewCLightningNode(testDir, bitcoind, 1)
	if err != nil {
		t.Fatalf("could not create cln %v", err)
	}
	t.Cleanup(cln.Kill)
	defer printFailedFiltered(t, cln.DaemonProcess)

	// Create policy file and accept all peers
	err = os.MkdirAll(filepath.Join(cln.GetDataDir(), "peerswap"), os.ModePerm)
	if err != nil {
		t.Fatal("could not create dir", err)
	}
	err = os.WriteFile(filepath.Join(cln.GetDataDir(), "peerswap", "policy.conf"), []byte("accept_all_peers=1\n"), os.ModePerm)
	if err != nil {
		t.Fatal("could not create policy file", err)
	}

	// Create config file
	peerswapConfig := ``

	configPath := filepath.Join(cln.GetDataDir(), "peerswap", "peerswap.conf")
	os.WriteFile(
		configPath,
		[]byte(peerswapConfig),
		os.ModePerm,
	)

	// Use lightningd with dev flags enabled
	cln.WithCmd("lightningd-dev")

	// Add plugin to cmd line options
	cln.AppendCmdLine([]string{
		"--dev-bitcoind-poll=1",
		"--dev-fast-gossip",
		"--large-channels",
		fmt.Sprint("--plugin=", peerswapPluginPath),
	})

	// lnd
	extraConfig := map[string]string{"protocol.wumbo-channels": "true"}
	lnd, err := testframework.NewLndNode(testDir, bitcoind, 1, extraConfig)
	if err != nil {
		t.Fatalf("could not create lnd %v", err)
	}
	t.Cleanup(lnd.Kill)

	// peerswapd
	peerswapd, err := NewPeerSwapd(testDir, peerswapdPath, &LndConfig{LndHost: fmt.Sprintf("localhost:%d", lnd.RpcPort), TlsPath: lnd.TlsPath, MacaroonPath: lnd.MacaroonPath}, nil, 1)
	if err != nil {
		t.Fatalf("could not create peerswapd %v", err)
	}
	t.Cleanup(peerswapd.Kill)
	defer printFailed(t, peerswapd.DaemonProcess)

	// Start nodes
	err = bitcoind.Run(true)
	if err != nil {
		t.Fatalf("bitcoind.Run() got err %v", err)
	}

	err = cln.Run(true, true)
	if err != nil {
		t.Fatalf("cln.Run() got err %v", err)
	}
	err = cln.WaitForLog("peerswap initialized", testframework.TIMEOUT)
	if err != nil {
		t.Fatalf("cln.WaitForLog() got err %v", err)
	}

	err = lnd.Run(true, true)
	if err != nil {
		t.Fatalf("lnd.Run() got err %v", err)
	}

	err = peerswapd.Run(true)
	if err != nil {
		t.Fatalf("peerswapd.Run() got err %v", err)
	}
	err = peerswapd.WaitForLog("peerswapd grpc listening on", testframework.TIMEOUT)
	if err != nil {
		t.Fatalf("perrswapd.WaitForLog() got err %v", err)
	}

	var lightningds []testframework.LightningNode
	switch funder {
	case FUNDER_CLN:
		lightningds = append(lightningds, cln)
		lightningds = append(lightningds, lnd)

	case FUNDER_LND:
		lightningds = append(lightningds, lnd)
		lightningds = append(lightningds, cln)
	default:
		t.Fatalf("unknown fundingNode %s", funder)
	}

	// Setup channel ([0] fundAmt(10^7) ---- 0 [1])
	scid, err := lightningds[0].OpenChannel(lightningds[1], fundAmt, 0, true, true, true)
	if err != nil {
		t.Fatalf("lightningds[0].OpenChannel() %v", err)
	}
	_, err = lightningds[1].FundWallet(10*fundAmt, true)
	if err != nil {
		t.Fatalf("lightningds[1].FundWallet() %v", err)
	}

	syncPoll(&clnPollableNode{cln}, &peerswapPollableNode{peerswapd, lnd.Id()})

	return bitcoind, lightningds, peerswapd, scid
}

func clnclnElementsSetup(t *testing.T, fundAmt uint64) (*testframework.BitcoinNode, *testframework.LiquidNode, []*CLightningNodeWithLiquid, string) {
	/// Get PeerSwap plugin path and test dir
	_, filename, _, _ := runtime.Caller(0)
	pathToPlugin := filepath.Join(filename, "..", "..", "out", "test-builds", "peerswap")
	testDir := t.TempDir()

	// Setup nodes (1 bitcoind, 1 liquidd, 2 lightningd)
	bitcoind, err := testframework.NewBitcoinNode(testDir, 1)
	if err != nil {
		t.Fatalf("could not create bitcoind %v", err)
	}
	t.Cleanup(bitcoind.Kill)

	liquidd, err := testframework.NewLiquidNode(testDir, bitcoind, 1)
	if err != nil {
		t.Fatal("error creating liquidd node", err)
	}
	t.Cleanup(liquidd.Kill)

	var lightningds []*testframework.CLightningNode
	for i := 1; i <= 2; i++ {
		lightningd, err := testframework.NewCLightningNode(testDir, bitcoind, i)
		if err != nil {
			t.Fatalf("could not create liquidd %v", err)
		}
		t.Cleanup(lightningd.Kill)
		defer printFailedFiltered(t, lightningd.DaemonProcess)

		// Create policy file and accept all peers
		err = os.MkdirAll(filepath.Join(lightningd.GetDataDir(), "peerswap"), os.ModePerm)
		if err != nil {
			t.Fatal("could not create dir", err)
		}
		err = os.WriteFile(filepath.Join(lightningd.GetDataDir(), "peerswap", "policy.conf"), []byte("accept_all_peers=1\n"), os.ModePerm)
		if err != nil {
			t.Fatal("could not create policy file", err)
		}

		// Set wallet name
		walletName := fmt.Sprintf("swap%d", i)

		// Create config file
		fileConf := struct {
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
		}
		data, err := toml.Marshal(fileConf)
		require.NoError(t, err)

		configPath := filepath.Join(lightningd.GetDataDir(), "peerswap", "peerswap.conf")
		os.WriteFile(
			configPath,
			data,
			os.ModePerm,
		)

		// Use lightningd with dev flags enabled
		lightningd.WithCmd("lightningd-dev")

		// Add plugin to cmd line options
		lightningd.AppendCmdLine([]string{
			"--dev-bitcoind-poll=1",
			"--dev-fast-gossip",
			"--large-channels",
			fmt.Sprint("--plugin=", pathToPlugin),
		})

		lightningds = append(lightningds, lightningd)
	}

	// Start nodes
	err = bitcoind.Run(true)
	if err != nil {
		t.Fatalf("bitcoind.Run() got err %v", err)
	}

	err = liquidd.Run(true)
	if err != nil {
		t.Fatalf("Run() got err %v", err)
	}

	for _, lightningd := range lightningds {
		err = lightningd.Run(true, true)
		if err != nil {
			t.Fatalf("lightningd.Run() got err %v", err)
		}
		err = lightningd.WaitForLog("peerswap initialized", testframework.TIMEOUT)
		if err != nil {
			t.Fatalf("lightningd.WaitForLog() got err %v", err)
		}
	}

	// Give liquid funds to nodes to have something to swap.
	for _, lightningd := range lightningds {
		var result clightning.GetAddressResponse
		lightningd.Rpc.Request(&clightning.LiquidGetAddress{}, &result)
		_ = liquidd.GenerateBlocks(20)
		_, err = liquidd.Rpc.Call("sendtoaddress", result.LiquidAddress, 10., "", "", false, false, 1, "UNSET")
		require.NoError(t, err)
	}

	// Lock txs.
	_, err = liquidd.Rpc.Call("generatetoaddress", 1, testframework.LBTC_BURN)
	require.NoError(t, err)

	// Setup channel ([0] fundAmt(10^7) ---- 0 [1]).
	scid, err := lightningds[0].OpenChannel(lightningds[1], fundAmt, 0, true, true, true)
	if err != nil {
		t.Fatalf("lightingds[0].OpenChannel() %v", err)
	}

	// Sync peer polling
	var result interface{}
	err = lightningds[0].Rpc.Request(&clightning.ReloadPolicyFile{}, &result)
	if err != nil {
		t.Fatalf("ListPeers %v", err)
	}
	err = lightningds[1].Rpc.Request(&clightning.ReloadPolicyFile{}, &result)
	if err != nil {
		t.Fatalf("ListPeers %v", err)
	}

	syncPoll(&clnPollableNode{lightningds[0]}, &clnPollableNode{lightningds[1]})

	return bitcoind, liquidd, []*CLightningNodeWithLiquid{{lightningds[0]}, {lightningds[1]}}, scid
}

func lndlndElementsSetup(t *testing.T, fundAmt uint64) (*testframework.BitcoinNode, *testframework.LiquidNode, []*LndNodeWithLiquid, []*PeerSwapd, string) {
	// Get PeerSwap plugin path and test dir
	_, filename, _, _ := runtime.Caller(0)
	pathToPlugin := filepath.Join(filename, "..", "..", "out", "test-builds", "peerswapd")
	testDir := t.TempDir()

	// Setup nodes (1 bitcoind, 1 liquidd, 2 lightningd, 2 peerswapd)
	bitcoind, err := testframework.NewBitcoinNode(testDir, 1)
	if err != nil {
		t.Fatalf("could not create bitcoind %v", err)
	}
	t.Cleanup(bitcoind.Kill)

	liquidd, err := testframework.NewLiquidNode(testDir, bitcoind, 1)
	if err != nil {
		t.Fatal("error creating liquidd node", err)
	}
	t.Cleanup(liquidd.Kill)

	var lightningds []*testframework.LndNode
	for i := 1; i <= 2; i++ {
		extraConfig := map[string]string{"protocol.wumbo-channels": "true"}
		lightningd, err := testframework.NewLndNode(testDir, bitcoind, i, extraConfig)
		if err != nil {
			t.Fatalf("could not create liquidd %v", err)
		}
		t.Cleanup(lightningd.Kill)
		defer printFailedFiltered(t, lightningd.DaemonProcess)

		lightningds = append(lightningds, lightningd)
	}

	var peerswapds []*PeerSwapd
	for i, lightningd := range lightningds {
		extraConfig := map[string]string{
			"elementsd.rpcuser":   liquidd.RpcUser,
			"elementsd.rpcpass":   liquidd.RpcPassword,
			"elementsd.rpchost":   "http://127.0.0.1",
			"elementsd.rpcport":   fmt.Sprintf("%d", liquidd.RpcPort),
			"elementsd.rpcwallet": fmt.Sprintf("swap-test-wallet-%d", i),
		}

		peerswapd, err := NewPeerSwapd(testDir, pathToPlugin, &LndConfig{LndHost: fmt.Sprintf("localhost:%d", lightningd.RpcPort), TlsPath: lightningd.TlsPath, MacaroonPath: lightningd.MacaroonPath}, extraConfig, i+1)
		if err != nil {
			t.Fatalf("could not create peerswapd %v", err)
		}
		t.Cleanup(peerswapd.Kill)

		// Create policy file and accept all peers
		err = os.WriteFile(filepath.Join(peerswapd.DataDir, "..", "policy.conf"), []byte("accept_all_peers=1\n"), os.ModePerm)
		if err != nil {
			t.Fatal("could not create policy file", err)
		}

		peerswapds = append(peerswapds, peerswapd)
	}

	// Start nodes
	err = bitcoind.Run(true)
	if err != nil {
		t.Fatalf("bitcoind.Run() got err %v", err)
	}

	err = liquidd.Run(true)
	if err != nil {
		t.Fatalf("Run() got err %v", err)
	}

	for _, lightningd := range lightningds {
		err = lightningd.Run(true, true)
		if err != nil {
			t.Fatalf("lightningd.Run() got err %v", err)
		}
	}

	for _, peerswapd := range peerswapds {
		err = peerswapd.Run(true)
		if err != nil {
			t.Fatalf("peerswapd.Run() got err %v", err)
		}
		err = peerswapd.WaitForLog("peerswapd grpc listening on", testframework.TIMEOUT)
		if err != nil {
			t.Fatalf("peerswapd.WaitForLog() got err %v", err)
		}
	}

	// Give liquid funds to nodes to have something to swap.
	for _, peerswapd := range peerswapds {
		r, err := peerswapd.PeerswapClient.LiquidGetAddress(context.Background(), &peerswaprpc.GetAddressRequest{})
		require.NoError(t, err)
		_ = liquidd.GenerateBlocks(20)
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
	// Get PeerSwap plugin path and test dir
	_, filename, _, _ := runtime.Caller(0)
	peerswapdPath := filepath.Join(filename, "..", "..", "out", "test-builds", "peerswapd")
	peerswapPluginPath := filepath.Join(filename, "..", "..", "out", "test-builds", "peerswap")
	testDir := t.TempDir()

	// Setup nodes (1 bitcoind, 1 liquid, 1 cln, 1 lnd, 1 peerswapd)
	bitcoind, err := testframework.NewBitcoinNode(testDir, 1)
	if err != nil {
		t.Fatalf("could not create bitcoind %v", err)
	}
	t.Cleanup(bitcoind.Kill)

	liquidd, err := testframework.NewLiquidNode(testDir, bitcoind, 1)
	if err != nil {
		t.Fatal("error creating liquidd node", err)
	}
	t.Cleanup(liquidd.Kill)

	// cln
	cln, err := testframework.NewCLightningNode(testDir, bitcoind, 1)
	if err != nil {
		t.Fatalf("could not create cln %v", err)
	}
	t.Cleanup(cln.Kill)
	defer printFailedFiltered(t, cln.DaemonProcess)

	// Create policy file and accept all peers
	err = os.MkdirAll(filepath.Join(cln.GetDataDir(), "peerswap"), os.ModePerm)
	if err != nil {
		t.Fatal("could not create dir", err)
	}
	err = os.WriteFile(filepath.Join(cln.GetDataDir(), "peerswap", "policy.conf"), []byte("accept_all_peers=1\n"), os.ModePerm)
	if err != nil {
		t.Fatal("could not create policy file", err)
	}

	walletName := "cln-test-wallet-1"

	// Create config file
	fileConf := struct {
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
			RpcWallet:   walletName,
		},
	}
	data, err := toml.Marshal(fileConf)
	require.NoError(t, err)

	configPath := filepath.Join(cln.GetDataDir(), "peerswap", "peerswap.conf")
	os.WriteFile(
		configPath,
		data,
		os.ModePerm,
	)

	// Use lightningd with dev flags enabled
	cln.WithCmd("lightningd-dev")

	// Add plugin to cmd line options
	cln.AppendCmdLine([]string{
		"--dev-bitcoind-poll=1",
		"--dev-fast-gossip",
		"--large-channels",
		fmt.Sprint("--plugin=", peerswapPluginPath),
	})

	// lnd
	extraConfigLnd := map[string]string{"protocol.wumbo-channels": "true"}
	lnd, err := testframework.NewLndNode(testDir, bitcoind, 1, extraConfigLnd)
	if err != nil {
		t.Fatalf("could not create lnd %v", err)
	}
	t.Cleanup(lnd.Kill)

	// peerswapd
	extraConfig := map[string]string{
		"elementsd.rpcuser":   liquidd.RpcUser,
		"elementsd.rpcpass":   liquidd.RpcPassword,
		"elementsd.rpchost":   "http://127.0.0.1",
		"elementsd.rpcport":   fmt.Sprintf("%d", liquidd.RpcPort),
		"elementsd.rpcwallet": "swap-test-wallet-1",
	}

	peerswapd, err := NewPeerSwapd(testDir, peerswapdPath, &LndConfig{LndHost: fmt.Sprintf("localhost:%d", lnd.RpcPort), TlsPath: lnd.TlsPath, MacaroonPath: lnd.MacaroonPath}, extraConfig, 1)
	if err != nil {
		t.Fatalf("could not create peerswapd %v", err)
	}
	t.Cleanup(peerswapd.Kill)
	defer printFailed(t, peerswapd.DaemonProcess)

	// Start nodes
	err = bitcoind.Run(true)
	if err != nil {
		t.Fatalf("bitcoind.Run() got err %v", err)
	}

	err = liquidd.Run(true)
	if err != nil {
		t.Fatalf("Run() got err %v", err)
	}

	err = cln.Run(true, true)
	if err != nil {
		t.Fatalf("cln.Run() got err %v", err)
	}
	err = cln.WaitForLog("peerswap initialized", testframework.TIMEOUT)
	if err != nil {
		t.Fatalf("cln.WaitForLog() got err %v", err)
	}

	err = lnd.Run(true, true)
	if err != nil {
		t.Fatalf("lnd.Run() got err %v", err)
	}

	err = peerswapd.Run(true)
	if err != nil {
		t.Fatalf("peerswapd.Run() got err %v", err)
	}
	err = peerswapd.WaitForLog("peerswapd grpc listening on", testframework.TIMEOUT)
	if err != nil {
		t.Fatalf("peerswapd.WaitForLog() got err %v", err)
	}

	// Give liquid funds to nodes to have something to swap.
	var lar clightning.GetAddressResponse
	cln.Rpc.Request(&clightning.LiquidGetAddress{}, &lar)
	_ = liquidd.GenerateBlocks(20)
	_, err = liquidd.Rpc.Call("sendtoaddress", lar.LiquidAddress, 10., "", "", false, false, 1, "UNSET")
	require.NoError(t, err)

	r, err := peerswapd.PeerswapClient.LiquidGetAddress(context.Background(), &peerswaprpc.GetAddressRequest{})
	require.NoError(t, err)
	_ = liquidd.GenerateBlocks(20)
	_, err = liquidd.Rpc.Call("sendtoaddress", r.Address, 10., "", "", false, false, 1, "UNSET")
	require.NoError(t, err)

	// Lock txs.
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

	// Setup channel ([0] fundAmt(10^7) ---- 0 [1])
	scid, err := lightningds[0].OpenChannel(lightningds[1], fundAmt, 0, true, true, true)
	if err != nil {
		t.Fatalf("cln.OpenChannel() %v", err)
	}

	syncPoll(&clnPollableNode{cln}, &peerswapPollableNode{peerswapd, lnd.Id()})
	return bitcoind, liquidd, lightningds, peerswapd, scid
}

type CLightningNodeWithLiquid struct {
	*testframework.CLightningNode
}

func (n *CLightningNodeWithLiquid) GetBtcBalanceSat() (uint64, error) {
	var response clightning.GetBalanceResponse
	err := n.Rpc.Request(&clightning.LiquidGetBalance{}, &response)
	if err != nil {
		return 0, err
	}
	return response.LiquidBalance, nil
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
