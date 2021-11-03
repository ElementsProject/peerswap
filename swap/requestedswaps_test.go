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
					AmountSat:       10000,
					Type:            SWAPTYPE_IN,
					RejectionReason: "asset not allowed",
				},
				{
					Asset:           "btc",
					AmountSat:       10000,
					Type:            SWAPTYPE_IN,
					RejectionReason: "asset not allowed",
				},
				{
					Asset:           "btc",
					AmountSat:       10000,
					Type:            SWAPTYPE_IN,
					RejectionReason: "asset not allowed",
				},
			},
			"node2": {
				{
					Asset:           "btc",
					AmountSat:       10000,
					Type:            SWAPTYPE_OUT,
					RejectionReason: "asset not allowed",
				},
				{
					Asset:           "btc",
					AmountSat:       10000,
					Type:            SWAPTYPE_IN,
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
			"swap_in": {
				"btc": {
					"total_amount_sat": 20000,
					"n_requests": 2
				},
				"l-btc": {
					"total_amount_sat": 10000,
					"n_requests": 1
				}
			}
		}
	},
	{
		"node_id": "node2",
		"requests": {
			"swap_in": {
				"btc": {
					"total_amount_sat": 10000,
					"n_requests": 1
				}
			},
			"swap_out": {
				"btc": {
					"total_amount_sat": 10000,
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
