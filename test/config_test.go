package test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/testframework"
	"github.com/pelletier/go-toml/v2"
)

// Test_ClnConfig checks that the peerswap plugin does not accept
// peerswap config from cln config, exits and prints an error to the
// logs. It is sufficient to test with command line arguments only (no
// config file), as core lightning internally parses all config
// options to command line arguments on startup.
func Test_ClnConfig(t *testing.T) {
	t.Parallel()
	IsIntegrationTest(t)

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	pathToPlugin := filepath.Join(filename, "..", "..", "out", "test-builds", "peerswap")
	testDir := makeTestDataDir(t)

	// Start bitcoin node
	bitcoind, err := testframework.NewBitcoinNode(testDir, 1)
	requireNoError(t, err)
	t.Cleanup(bitcoind.Kill)

	err = bitcoind.Run(true)
	requireNoError(t, err)

	// Setup core lightning node.
	lightningd, err := testframework.NewCLightningNode(testDir, bitcoind, 1)
	requireNoError(t, err)
	t.Cleanup(lightningd.Kill)

	// Dump lightningd logs on failure.
	DumpOnFailure(t, WithCLightnings([]*testframework.CLightningNode{lightningd}))

	policyPath := filepath.Join(lightningd.GetDataDir(), "..", "policy.conf")
	// Write policy file, accept all peers.
	err = os.WriteFile(
		policyPath,
		[]byte("accept_all_peers=1"),
		0o600,
	)
	requireNoError(t, err)

	// Add commandline arguments, especially peerswap related arguments.
	lightningd.AppendCmdLine([]string{
		fmt.Sprintf("--plugin=%s", pathToPlugin),
		fmt.Sprintf("--peerswap-policy-path=%s", policyPath),
	})

	// Start lightning daemon.
	err = lightningd.Run(true, false)
	requireNoError(t, err)

	err = lightningd.WaitForLog(
		"Setting config in core lightning config file is deprecated",
		30*time.Second,
	)
	assertNoError(t, err)

	// Failure dump handled by DumpOnFailure
}

// Test_ClnPluginConfigFile checks that the config is read from the data directory.
func Test_ClnPluginConfigFile(t *testing.T) {
	t.Parallel()
	IsIntegrationTest(t)

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	pathToPlugin := filepath.Join(filename, "..", "..", "out", "test-builds", "peerswap")
	testDir := makeTestDataDir(t)

	// Start bitcoin node
	bitcoind, err := testframework.NewBitcoinNode(testDir, 1)
	requireNoError(t, err)
	t.Cleanup(bitcoind.Kill)

	err = bitcoind.Run(true)
	requireNoError(t, err)

	// Setup core lightning node.
	lightningd, err := testframework.NewCLightningNode(testDir, bitcoind, 1)
	requireNoError(t, err)
	t.Cleanup(lightningd.Kill)

	peerswapConfig := ``

	configPath := filepath.Join(lightningd.GetDataDir(), "peerswap.conf")
	err = os.WriteFile(
		configPath,
		[]byte(peerswapConfig),
		0o600,
	)
	requireNoError(t, err)

	// Add commandline arguments, especially peerswap related arguments.
	lightningd.AppendCmdLine([]string{
		fmt.Sprintf("--plugin=%s", pathToPlugin),
	})

	// Start lightning daemon.
	err = lightningd.Run(true, false)
	requireNoError(t, err)

	err = lightningd.WaitForLog(
		"Waiting for cln to be synced",
		testframework.TIMEOUT,
	)
	assertNoError(t, err)

	if t.Failed() {
		pprintFail(tailableProcess{
			p: lightningd.DaemonProcess,
		})
	}
}

// Test_ClnPluginConfigFile_DoesNotExist checks that peerswap is able
// to start if no config file is provided.
func Test_ClnPluginConfigFile_DoesNotExist(t *testing.T) {
	t.Parallel()
	IsIntegrationTest(t)

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	pathToPlugin := filepath.Join(filename, "..", "..", "out", "test-builds", "peerswap")
	testDir := makeTestDataDir(t)

	// Start bitcoin node
	bitcoind, err := testframework.NewBitcoinNode(testDir, 1)
	requireNoError(t, err)
	t.Cleanup(bitcoind.Kill)

	err = bitcoind.Run(true)
	requireNoError(t, err)

	// Setup core lightning node.
	lightningd, err := testframework.NewCLightningNode(testDir, bitcoind, 1)
	requireNoError(t, err)
	t.Cleanup(lightningd.Kill)

	// Dump lightningd logs on failure.
	DumpOnFailure(t, WithCLightnings([]*testframework.CLightningNode{lightningd}))

	// Add commandline arguments, especially peerswap related arguments.
	lightningd.AppendCmdLine([]string{
		fmt.Sprintf("--plugin=%s", pathToPlugin),
	})

	// Start lightning daemon.
	err = lightningd.Run(true, false)
	requireNoError(t, err)

	err = lightningd.WaitForLog(
		"Waiting for cln to be synced",
		testframework.TIMEOUT,
	)
	assertNoError(t, err)

	// Failure dump handled by DumpOnFailure
}

// Test_ClnPluginConfig_ElementsAuthCookie checks that peerswap can
// read the elements cookie file.
func Test_ClnPluginConfig_ElementsAuthCookie(t *testing.T) {
	t.Parallel()
	IsIntegrationTest(t)

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	pathToPlugin := filepath.Join(filename, "..", "..", "out", "test-builds", "peerswap")
	testDir := makeTestDataDir(t)

	// Start bitcoin node
	bitcoind, err := testframework.NewBitcoinNode(testDir, 1)
	requireNoError(t, err)
	t.Cleanup(bitcoind.Kill)

	err = bitcoind.Run(true)
	requireNoError(t, err)

	// Start Elements node
	liquidd, err := testframework.NewLiquidNodeFromConfig(
		testDir,
		bitcoind,
		map[string]string{
			"listen":           "1",
			"debug":            "1",
			"fallbackfee":      "0.00001",
			"initialfreecoins": "2100000000000000",
			"creatediscountct": "1",
			"validatepegin":    "0",
			"chain":            "liquidregtest",
		},
		1,
	)
	requireNoError(t, err)

	err = liquidd.Run(true)
	requireNoError(t, err)

	// Setup core lightning node.
	lightningd, err := testframework.NewCLightningNode(testDir, bitcoind, 1)
	requireNoError(t, err)
	t.Cleanup(lightningd.Kill)

	// Dump both liquidd and lightningd logs on failure.
	DumpOnFailure(t, WithLiquid(liquidd), WithCLightnings([]*testframework.CLightningNode{lightningd}))

	peerswapConfig := struct {
		Liquid struct {
			RpcPasswordFile string
			RpcPort         int
		}
	}{
		Liquid: struct {
			RpcPasswordFile string
			RpcPort         int
		}{
			RpcPasswordFile: filepath.Join(liquidd.DataDir, "liquidregtest", ".cookie"),
			RpcPort:         liquidd.RpcPort,
		},
	}

	data, err := toml.Marshal(peerswapConfig)
	requireNoError(t, err)

	err = os.MkdirAll(filepath.Join(lightningd.GetDataDir(), "peerswap"), os.ModePerm)
	requireNoError(t, err)
	configPath := filepath.Join(lightningd.GetDataDir(), "peerswap", "peerswap.conf")
	err = os.WriteFile(
		configPath,
		data,
		0o600,
	)
	requireNoError(t, err)

	// Add commandline arguments, especially peerswap related arguments.
	lightningd.AppendCmdLine([]string{
		fmt.Sprintf("--plugin=%s", pathToPlugin),
	})

	// Start lightning daemon.
	err = lightningd.Run(true, false)
	requireNoError(t, err)

	err = lightningd.WaitForLog(
		"Liquid swaps enabled",
		testframework.TIMEOUT,
	)
	assertNoError(t, err)
}

// Test_ClnPluginConfig_DisableLiquid checks that liquid can be disabled by
// setting:
// ```
// [liquid]
// disabled=true
// ```
// in the plugin config file.
func Test_ClnPluginConfig_DisableLiquid(t *testing.T) {
	t.Parallel()
	IsIntegrationTest(t)

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	pathToPlugin := filepath.Join(filename, "..", "..", "out", "test-builds", "peerswap")
	testDir := makeTestDataDir(t)

	// Start bitcoin node
	bitcoind, err := testframework.NewBitcoinNode(testDir, 1)
	requireNoError(t, err)
	t.Cleanup(bitcoind.Kill)

	err = bitcoind.Run(true)
	requireNoError(t, err)

	// Start Elements node
	liquidd, err := testframework.NewLiquidNodeFromConfig(
		testDir,
		bitcoind,
		map[string]string{
			"listen":           "1",
			"debug":            "1",
			"fallbackfee":      "0.00001",
			"creatediscountct": "1",
			"initialfreecoins": "2100000000000000",
			"validatepegin":    "0",
			"chain":            "liquidregtest",
		},
		1,
	)
	requireNoError(t, err)

	err = liquidd.Run(true)
	requireNoError(t, err)

	// Setup core lightning node.
	lightningd, err := testframework.NewCLightningNode(testDir, bitcoind, 1)
	requireNoError(t, err)
	t.Cleanup(lightningd.Kill)

	// Dump both liquidd and lightningd logs on failure.
	DumpOnFailure(t, WithLiquid(liquidd), WithCLightnings([]*testframework.CLightningNode{lightningd}))

	peerswapConfig := struct {
		Liquid struct {
			RpcPasswordFile string
			RpcPort         int
			Disabled        bool
		}
	}{
		Liquid: struct {
			RpcPasswordFile string
			RpcPort         int
			Disabled        bool
		}{
			RpcPasswordFile: filepath.Join(liquidd.DataDir, "liquidregtest", ".cookie"),
			RpcPort:         liquidd.RpcPort,
			Disabled:        true,
		},
	}

	data, err := toml.Marshal(peerswapConfig)
	requireNoError(t, err)

	configPath := filepath.Join(lightningd.GetDataDir(), "peerswap.conf")
	err = os.WriteFile(
		configPath,
		data,
		0o600,
	)
	requireNoError(t, err)

	// Add commandline arguments, especially peerswap related arguments.
	lightningd.AppendCmdLine([]string{
		fmt.Sprintf("--plugin=%s", pathToPlugin),
	})

	// Start lightning daemon.
	err = lightningd.Run(true, false)
	requireNoError(t, err)

	err = lightningd.WaitForLog(
		"Liquid swaps disabled",
		testframework.TIMEOUT,
	)
	assertNoError(t, err)
}
