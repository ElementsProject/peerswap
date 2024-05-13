package lwk_test

import (
	"testing"

	"github.com/elementsproject/peerswap/lwk"
	"github.com/stretchr/testify/assert"
)

func TestSatPerKVByteFromFeeBTCPerKb(t *testing.T) {
	t.Parallel()
	t.Run("below minimum minimumSatPerByte", func(t *testing.T) {
		t.Parallel()
		var (
			txsize      int64 = 1000
			FeeBTCPerKb       = 0.0000001
		)
		got := lwk.SatPerKVByteFromFeeBTCPerKb(FeeBTCPerKb).GetFee(txsize)
		want := lwk.Satoshi(100)
		assert.Equal(t, want, got)
	})
	t.Run("above minimum minimumSatPerByte", func(t *testing.T) {
		t.Parallel()
		var (
			txsize      int64 = 1000
			FeeBTCPerKb       = 0.000002
		)
		got := lwk.SatPerKVByteFromFeeBTCPerKb(FeeBTCPerKb).GetFee(txsize)
		want := lwk.Satoshi(200)
		assert.Equal(t, want, got)
	})
}
