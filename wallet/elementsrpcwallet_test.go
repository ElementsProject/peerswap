package wallet

import "testing"

func TestSatsToBTCString(t *testing.T) {
	cases := []struct {
		sats uint64
		want string
	}{
		{0, "0.00000000"},
		{1, "0.00000001"},
		{99_999_999, "0.99999999"},
		{100_000_000, "1.00000000"},
		{123_456_789, "1.23456789"},
		// Values that lose precision through float64 formatting.
		{210_000_000_000_000, "2100000.00000000"},
		{12_345, "0.00012345"},
		{100_000_001, "1.00000001"},
	}
	for _, c := range cases {
		if got := SatsToBTCString(c.sats); got != c.want {
			t.Errorf("SatsToBTCString(%d) = %q, want %q", c.sats, got, c.want)
		}
	}
}
