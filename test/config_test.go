package test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/testframework"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_ClnConfig checks that the peerswap plugin does not accept
// peerswap config from cln config, exits and prints an error to the
// logs. It is sufficient test with command line arguments only (no
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
