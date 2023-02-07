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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_ClnConfig checks that the peerswap plugin does not accept
// peerswap config from cln config, exits and prints an error to the
// logs. It is sufficient to test with command line arguments only (no
// config file), as core lightning internally parses all config
// options to command line arguments on startup.
func Test_ClnConfig(t *testing.T) {
	t.Parallel()
	IsIntegrationTest(t)

	_, filename, _, _ := runtime.Caller(0)
	pathToPlugin := filepath.Join(filename, "..", "..", "out", "test-builds", "peerswap")
	testDir := t.TempDir()

	// Start bitcoin node
	bitcoind, err := testframework.NewBitcoinNode(testDir, 1)
	require.NoError(t, err)
	t.Cleanup(bitcoind.Kill)

	err = bitcoind.Run(true)
	require.NoError(t, err)

	// Setup core lightning node.
	lightningd, err := testframework.NewCLightningNode(testDir, bitcoind, 1)
	require.NoError(t, err)
	t.Cleanup(lightningd.Kill)

	policyPath := filepath.Join(lightningd.GetDataDir(), "..", "policy.conf")
	// Write policy file, accept all peers.
	os.WriteFile(
		policyPath,
		[]byte("accept_all_peers=1"),
		os.ModePerm,
	)

	// Add commandline arguments, especially peerswap related arguments.
	lightningd.AppendCmdLine([]string{
		fmt.Sprintf("--plugin=%s", pathToPlugin),
		fmt.Sprintf("--peerswap-policy-path=%s", policyPath),
	})

	// Start lightning daemon.
	err = lightningd.Run(true, false)
	require.NoError(t, err)

	err = lightningd.WaitForLog(
		"Setting config in core lightning config file is deprecated",
		30*time.Second,
	)
	assert.NoError(t, err)

	if t.Failed() {
		pprintFail(tailableProcess{
			p: lightningd.DaemonProcess,
		})
	}
}

// Test_ClnPluginConfigFile checks that the config is read from the data directory
func Test_ClnPluginConfigFile(t *testing.T) {
	t.Parallel()
	IsIntegrationTest(t)

	_, filename, _, _ := runtime.Caller(0)
	pathToPlugin := filepath.Join(filename, "..", "..", "out", "test-builds", "peerswap")
	testDir := t.TempDir()

	// Start bitcoin node
	bitcoind, err := testframework.NewBitcoinNode(testDir, 1)
	require.NoError(t, err)
	t.Cleanup(bitcoind.Kill)

	err = bitcoind.Run(true)
	require.NoError(t, err)

	// Setup core lightning node.
	lightningd, err := testframework.NewCLightningNode(testDir, bitcoind, 1)
	require.NoError(t, err)
	t.Cleanup(lightningd.Kill)

	peerswapConfig := ``

	configPath := filepath.Join(lightningd.GetDataDir(), "peerswap.conf")
	os.WriteFile(
		configPath,
		[]byte(peerswapConfig),
		os.ModePerm,
	)

	// Add commandline arguments, especially peerswap related arguments.
	lightningd.AppendCmdLine([]string{
		fmt.Sprintf("--plugin=%s", pathToPlugin),
	})

	// Start lightning daemon.
	err = lightningd.Run(true, false)
	require.NoError(t, err)

	err = lightningd.WaitForLog(
		"Waiting for cln to be synced",
		testframework.TIMEOUT,
	)
	assert.NoError(t, err)

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

	_, filename, _, _ := runtime.Caller(0)
	pathToPlugin := filepath.Join(filename, "..", "..", "out", "test-builds", "peerswap")
	testDir := t.TempDir()

	// Start bitcoin node
	bitcoind, err := testframework.NewBitcoinNode(testDir, 1)
	require.NoError(t, err)
	t.Cleanup(bitcoind.Kill)

	err = bitcoind.Run(true)
	require.NoError(t, err)

	// Setup core lightning node.
	lightningd, err := testframework.NewCLightningNode(testDir, bitcoind, 1)
	require.NoError(t, err)
	t.Cleanup(lightningd.Kill)

	// Add commandline arguments, especially peerswap related arguments.
	lightningd.AppendCmdLine([]string{
		fmt.Sprintf("--plugin=%s", pathToPlugin),
	})

	// Start lightning daemon.
	err = lightningd.Run(true, false)
	require.NoError(t, err)

	err = lightningd.WaitForLog(
		"Waiting for cln to be synced",
		testframework.TIMEOUT,
	)
	assert.NoError(t, err)

	if t.Failed() {
		pprintFail(tailableProcess{
			p: lightningd.DaemonProcess,
		})
	}
}

// Test_ClnPluginConfig_ElementsAuthCookie checks that peerswap can
// read the elements cookie file.
func Test_ClnPluginConfig_ElementsAuthCookie(t *testing.T) {
	t.Parallel()
	IsIntegrationTest(t)

	_, filename, _, _ := runtime.Caller(0)
	pathToPlugin := filepath.Join(filename, "..", "..", "out", "test-builds", "peerswap")
	testDir := t.TempDir()

	// Start bitcoin node
	bitcoind, err := testframework.NewBitcoinNode(testDir, 1)
	require.NoError(t, err)
	t.Cleanup(bitcoind.Kill)

	err = bitcoind.Run(true)
	require.NoError(t, err)

	// Start Elements node
	liquidd, err := testframework.NewLiquidNodeFromConfig(
		testDir,
		bitcoind,
		map[string]string{
			"listen":           "1",
			"debug":            "1",
			"fallbackfee":      "0.00001",
			"initialfreecoins": "2100000000000000",
			"validatepegin":    "0",
			"chain":            "regtest"},
		1,
	)
	require.NoError(t, err)

	err = liquidd.Run(true)
	t.Cleanup(func() {
		if t.Failed() {
			pprintFail(tailableProcess{
				p: liquidd.DaemonProcess,
			})
		}
	})

	// Setup core lightning node.
	lightningd, err := testframework.NewCLightningNode(testDir, bitcoind, 1)
	require.NoError(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			pprintFail(tailableProcess{
				p: lightningd.DaemonProcess,
			})
		}
	})
	t.Cleanup(lightningd.Kill)

	peerswapConfig := struct {
		Liquid struct {
			RpcPasswordFile string
			RpcPort         int
			Enabled         bool
		}
	}{
		Liquid: struct {
			RpcPasswordFile string
			RpcPort         int
			Enabled         bool
		}{
			RpcPasswordFile: filepath.Join(liquidd.DataDir, "regtest", ".cookie"),
			RpcPort:         liquidd.RpcPort,
			Enabled:         true,
		},
	}

	data, err := toml.Marshal(peerswapConfig)
	require.NoError(t, err)

	configPath := filepath.Join(lightningd.GetDataDir(), "peerswap.conf")
	os.WriteFile(
		configPath,
		data,
		os.ModePerm,
	)

	// Add commandline arguments, especially peerswap related arguments.
	lightningd.AppendCmdLine([]string{
		fmt.Sprintf("--plugin=%s", pathToPlugin),
	})

	// Start lightning daemon.
	err = lightningd.Run(true, false)
	require.NoError(t, err)

	err = lightningd.WaitForLog(
		"Liquid swaps enabled",
		testframework.TIMEOUT,
	)
	assert.NoError(t, err)
}
