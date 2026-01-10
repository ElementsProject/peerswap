package test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/elementsproject/peerswap/onchain"
	"github.com/elementsproject/peerswap/testframework"
)

func TestBitcoinFeeFloorMatchesNodeVersion(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	builder := NewHarnessBuilder(t)
	bitcoind := builder.Bitcoind()
	builder.EnsureBitcoindStarted()

	var networkInfo struct {
		Subversion string `json:"subversion"`
	}
	r, err := bitcoind.Call("getnetworkinfo")
	requireNoError(t, err)
	requireNoError(t, r.GetObject(&networkInfo))
	requireNew(t).NotEmpty(networkInfo.Subversion, "bitcoind subversion should be present")

	expectedFloor, normalizedVersion := onchain.DetermineFeeFloor(networkInfo.Subversion)
	requireNew(t).NotEmpty(normalizedVersion, "bitcoind version should be parseable")

	cln := builder.AddCLightningNode(1,
		WithClnExtraArgs("--dev-bitcoind-poll=1", "--dev-fast-gossip", "--large-channels"),
	)

	builder.Start()

	pattern := fmt.Sprintf(
		"Detected Bitcoin Core version %s, using fee floor %d sat/kw",
		regexp.QuoteMeta(normalizedVersion),
		expectedFloor,
	)
	err = cln.WaitForLog(pattern, testframework.TIMEOUT)
	requireNoError(t, err, "peerswap should log fee floor derived from bitcoind version")
}
