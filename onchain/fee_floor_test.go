package onchain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetermineFeeFloor_ModernVersion(t *testing.T) {
	floor, normalized := DetermineFeeFloor("/Satoshi:29.2.0/")
	require.Equal(t, ModernFeeFloorSatPerKw, floor)
	require.Equal(t, "29.2.0", normalized)
}

func TestDetermineFeeFloor_LegacyVersion(t *testing.T) {
	floor, normalized := DetermineFeeFloor("/Satoshi:29.1.1/")
	require.Equal(t, LegacyFeeFloorSatPerKw, floor)
	require.Equal(t, "29.1.1", normalized)
}

func TestDetermineFeeFloor_UnknownInput(t *testing.T) {
	floor, normalized := DetermineFeeFloor("custom")
	require.Equal(t, LegacyFeeFloorSatPerKw, floor)
	require.Equal(t, "", normalized)
}
