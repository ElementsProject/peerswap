package onchain

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/btcsuite/btcd/btcutil"
)

const (
	modernFeeFloorMajor = 29
	modernFeeFloorMinor = 2

	// LegacyFeeFloorSatPerKw enforces ~1 sat/vB minimums for older nodes.
	LegacyFeeFloorSatPerKw btcutil.Amount = 253
	// ModernFeeFloorSatPerKw matches the v29.2+ mempoolminfee default (0.1 sat/vB).
	ModernFeeFloorSatPerKw btcutil.Amount = 25
)

var bitcoinVersionPattern = regexp.MustCompile(`(\d+)(?:\.(\d+))?(?:\.(\d+))?`)

// DetermineFeeFloor inspects the provided Bitcoin Core version string (as
// returned by getnetworkinfo subversion) and picks the default fee floor. It
// returns both the chosen floor and the normalized semantic version string.
func DetermineFeeFloor(versionString string) (btcutil.Amount, string) {
	version := normalizeBitcoinVersion(versionString)
	if version == nil {
		return LegacyFeeFloorSatPerKw, ""
	}

	if version.major > modernFeeFloorMajor || (version.major == modernFeeFloorMajor && version.minor >= modernFeeFloorMinor) {
		return ModernFeeFloorSatPerKw, version.String()
	}

	return LegacyFeeFloorSatPerKw, version.String()
}

type bitcoinVersion struct {
	major int
	minor int
	patch int
}

func (v *bitcoinVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch)
}

func normalizeBitcoinVersion(input string) *bitcoinVersion {
	matches := bitcoinVersionPattern.FindStringSubmatch(input)
	if matches == nil {
		return nil
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return nil
	}

	minor := parseVersionSegment(matches, 2)
	patch := parseVersionSegment(matches, 3)

	return &bitcoinVersion{major: major, minor: minor, patch: patch}
}

func parseVersionSegment(matches []string, idx int) int {
	if len(matches) <= idx || matches[idx] == "" {
		return 0
	}

	value, err := strconv.Atoi(matches[idx])
	if err != nil {
		return 0
	}

	return value
}
