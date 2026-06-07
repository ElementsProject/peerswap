package wallet

import (
	"testing"
)

func TestSatsToAmountString(t *testing.T) {
	tests := []struct {
		name string
		sats uint64
		want string
	}{
		{
			name: "zero",
			sats: 0,
			want: "0.00000000",
		},
		{
			name: "single sat",
			sats: 1,
			want: "0.00000001",
		},
		{
			name: "issue example keeps sat precision",
			sats: 123456142,
			want: "1.23456142",
		},
		{
			name: "whole bitcoin keeps decimal places",
			sats: satsPerBitcoin,
			want: "1.00000000",
		},
		{
			name: "max uint64",
			sats: ^uint64(0),
			want: "184467440737.09551615",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := satsToAmountString(tt.sats); got != tt.want {
				t.Fatalf("satsToAmountString(%d) = %q, want %q", tt.sats, got, tt.want)
			}
		})
	}
}
