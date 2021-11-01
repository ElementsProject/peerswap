package swap

import (
	"bytes"
	"testing"
)

func TestPrint(t *testing.T) {
	store := &requestedSwapsStoreMock{
		data: map[string][]RequestedSwap{
			"node1": {
				{
					Asset:           "l-btc",
					AmountMsat:      1000,
					Type:            SWAPTYPE_IN,
					RejectionReason: "asset not allowed",
				},
				{
					Asset:           "btc",
					AmountMsat:      1000,
					Type:            SWAPTYPE_IN,
					RejectionReason: "asset not allowed",
				},
				{
					Asset:           "btc",
					AmountMsat:      1000,
					Type:            SWAPTYPE_IN,
					RejectionReason: "asset not allowed",
				},
			},
			"node2": {
				{
					Asset:           "btc",
					AmountMsat:      1000,
					Type:            SWAPTYPE_OUT,
					RejectionReason: "asset not allowed",
				},
			},
		},
	}

	sp := NewRequestedSwapsPrinter(store)
	var b bytes.Buffer
	sp.Write(&b)
	got := b.String()
	want := `[
	{
		"node_id": "node1",
		"requests": {
			"swap in": {
				"btc": {
					"amount_msat": 2000,
					"n_requests": 2
				},
				"l-btc": {
					"amount_msat": 1000,
					"n_requests": 1
				}
			}
		}
	},
	{
		"node_id": "node2",
		"requests": {
			"swap out": {
				"btc": {
					"amount_msat": 1000,
					"n_requests": 1
				}
			}
		}
	}
]`

	if got != want {
		t.Errorf("sp.Write() = %q, want %q", got, want)
	}
}
