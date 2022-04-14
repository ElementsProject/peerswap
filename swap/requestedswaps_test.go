package swap

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrint(t *testing.T) {
	want := map[string][]RequestedSwap{
		"node1": {
			{
				Asset:           "lbtc",
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
	}

	store := &requestedSwapsStoreMock{
		data: want,
	}
	sp := NewRequestedSwapsPrinter(store)
	got, err := sp.store.GetAll()
	if err != nil {
		t.Fatalf("GetAll(): %v", err)
	}

	eq := reflect.DeepEqual(want, got)
	assert.True(t, eq)
}
