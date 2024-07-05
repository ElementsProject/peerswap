package lwk_test

import (
	"testing"

	"github.com/elementsproject/peerswap/lwk"
	"github.com/stretchr/testify/assert"
)

func TestSatPerKVByteFromFeeBTCPerKb(t *testing.T) {
	t.Parallel()
	t.Run("above minimumSatPerByte", func(t *testing.T) {
		t.Parallel()
		var (
			FeeBTCPerKb = 0.0001
		)
		got := lwk.SatPerVByteFromFeeBTCPerKb(FeeBTCPerKb)
		assert.Equal(t, 10.0, float64(got))
	})
	t.Run("below minimumSatPerByte", func(t *testing.T) {
		t.Parallel()
		var (
			FeeBTCPerKb = 0.0000002
		)
		got := lwk.SatPerVByteFromFeeBTCPerKb(FeeBTCPerKb)
		assert.Equal(t, 0.1, float64(got))
	})
}

func TestGetFee(t *testing.T) {
	t.Parallel()
	t.Run("above minimumSatPerByte", func(t *testing.T) {
		t.Parallel()
		var (
			txsize      int64 = 250
			FeeBTCPerKb       = 0.0001
		)
		got := lwk.SatPerVByteFromFeeBTCPerKb(FeeBTCPerKb).GetFee(txsize)
		want := lwk.Satoshi(2500)
		assert.Equal(t, want, got)
	})
	t.Run("above minimum minimumSatPerByte", func(t *testing.T) {
		t.Parallel()
		var (
			txsize      int64 = 1000
			FeeBTCPerKb       = 0.0000002
		)
		got := lwk.SatPerVByteFromFeeBTCPerKb(FeeBTCPerKb).GetFee(txsize)
		want := lwk.Satoshi(100)
		assert.Equal(t, want, got)
	})
}
