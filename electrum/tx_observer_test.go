package electrum

import (
	"context"
	"math"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	goelectrum "github.com/checksum0/go-electrum/electrum"
	"github.com/elementsproject/peerswap/onchain"
	"github.com/elementsproject/peerswap/swap"
)

type observerRPC struct {
	history  []*goelectrum.GetMempoolResult
	rawTx    string
	rawCalls int
}

func testScriptPubKey(t *testing.T) scriptPubKey {
	t.Helper()
	script, err := NewScriptPubKey([]byte{
		0x00, 0x20,
		0xec, 0x6f, 0x7a, 0x5a, 0xa8, 0xf2, 0xb1, 0x0c,
		0xa5, 0x15, 0x04, 0x52, 0x3a, 0x60, 0xd4, 0x03,
		0x06, 0xf6, 0x96, 0xcd, 0x06, 0xf6, 0x96, 0xcd,
		0x06, 0xf6, 0x96, 0xcd, 0x06, 0xf6, 0x96, 0xcd,
	})
	if err != nil {
		t.Fatalf("NewScriptPubKey() error = %v", err)
	}
	return script
}

func (r *observerRPC) SubscribeHeaders(context.Context) (<-chan *goelectrum.SubscribeHeadersResult, error) {
	return nil, nil
}

func (r *observerRPC) GetHistory(context.Context, string) ([]*goelectrum.GetMempoolResult, error) {
	return r.history, nil
}

func (r *observerRPC) GetRawTransaction(context.Context, string) (string, error) {
	r.rawCalls++
	return r.rawTx, nil
}

func (r *observerRPC) BroadcastTransaction(context.Context, string) (string, error) {
	return "", nil
}

func (r *observerRPC) GetFee(context.Context, uint32) (float32, error) {
	return 0, nil
}

func (r *observerRPC) Ping(context.Context) error {
	return nil
}

func (r *observerRPC) Reboot(context.Context) error {
	return nil
}

func checkExpectedError(t *testing.T, err error, wantErr bool) {
	t.Helper()
	if wantErr {
		if err == nil {
			t.Fatal("expected an error")
		}
		return
	}
	if err != nil {
		t.Fatalf("unexpected error = %v", err)
	}
}

func checkOpeningObserverResult(
	t *testing.T,
	called bool,
	wantCallback bool,
	callbackCalls int,
	rawCalls int,
) {
	t.Helper()
	if called != wantCallback {
		t.Errorf("Callback() called = %v, want %v", called, wantCallback)
	}
	wantCalls := 0
	if wantCallback {
		wantCalls = 1
	}
	if callbackCalls != wantCalls || rawCalls != wantCalls {
		t.Errorf(
			"callback calls = %d, raw transaction calls = %d; want %d each",
			callbackCalls, rawCalls, wantCalls,
		)
	}
}

func TestHasConfirmations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		txHeight BlockHeight
		tip      BlockHeight
		required uint32
		want     bool
		wantErr  bool
	}{
		{name: "negative unconfirmed height", txHeight: -1, tip: 100, required: 2},
		{name: "zero unconfirmed height", txHeight: 0, tip: 100, required: 2},
		{name: "invalid tip", txHeight: 1, tip: 0, required: 2, wantErr: true},
		{name: "future transaction height", txHeight: 101, tip: 100, required: 2, wantErr: true},
		{name: "one confirmation", txHeight: 100, tip: 100, required: 2},
		{name: "exactly two confirmations", txHeight: 100, tip: 101, required: 2, want: true},
		{name: "CSV one block early", txHeight: 100, tip: 158, required: onchain.LiquidCsv},
		{name: "exact CSV boundary", txHeight: 100, tip: 159, required: onchain.LiquidCsv, want: true},
		{name: "max int32 one confirmation", txHeight: math.MaxInt32, tip: math.MaxInt32, required: 2},
		{name: "max int32 exact boundary", txHeight: math.MaxInt32 - 1, tip: math.MaxInt32, required: 2, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := hasConfirmations(tt.txHeight, tt.tip, tt.required)
			if tt.wantErr {
				if err == nil {
					t.Fatal("hasConfirmations() expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("hasConfirmations() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("hasConfirmations() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestObserveOpeningTXConfirmationBoundary(t *testing.T) {
	t.Parallel()
	txID, err := chainhash.NewHashFromStr("01")
	if err != nil {
		t.Fatalf("NewHashFromStr() error = %v", err)
	}
	script := testScriptPubKey(t)

	tests := []struct {
		name         string
		heights      []int32
		tip          BlockHeight
		wantCallback bool
		wantErr      bool
	}{
		{name: "height minus one", heights: []int32{-1}, tip: 100},
		{name: "height zero", heights: []int32{0}, tip: 100},
		{name: "future height", heights: []int32{101}, tip: 100, wantErr: true},
		{name: "one confirmation", heights: []int32{100}, tip: 100},
		{name: "two confirmations", heights: []int32{100}, tip: 101, wantCallback: true},
		{name: "max int32 one confirmation", heights: []int32{math.MaxInt32}, tip: math.MaxInt32},
		{
			name:         "max int32 two confirmations",
			heights:      []int32{math.MaxInt32 - 1},
			tip:          math.MaxInt32,
			wantCallback: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			history := make([]*goelectrum.GetMempoolResult, 0, len(tt.heights))
			for _, height := range tt.heights {
				history = append(history, &goelectrum.GetMempoolResult{
					Hash:   txID.String(),
					Height: height,
				})
			}
			rpc := &observerRPC{history: history, rawTx: "raw-opening-transaction"}
			callbackCalls := 0
			observer := NewObserveOpeningTX(
				*swap.NewSwapId(), txID, script, rpc,
				func(string, string, error) error {
					callbackCalls++
					return nil
				},
			)

			called, err := observer.Callback(context.Background(), tt.tip)
			checkExpectedError(t, err, tt.wantErr)
			checkOpeningObserverResult(
				t, called, tt.wantCallback, callbackCalls, rpc.rawCalls,
			)
		})
	}
}

func TestObserveCSVTXConfirmationBoundary(t *testing.T) {
	t.Parallel()
	txID, err := chainhash.NewHashFromStr("01")
	if err != nil {
		t.Fatalf("NewHashFromStr() error = %v", err)
	}
	script := testScriptPubKey(t)
	rpc := &observerRPC{history: []*goelectrum.GetMempoolResult{{
		Hash:   txID.String(),
		Height: 100,
	}}}
	callbackCalls := 0
	observer := NewobserveCSVTX(
		*swap.NewSwapId(), txID, script, rpc,
		func(string) error {
			callbackCalls++
			return nil
		},
	)

	called, err := observer.Callback(context.Background(), 158)
	if err != nil {
		t.Fatalf("Callback() one block early error = %v", err)
	}
	if called || callbackCalls != 0 {
		t.Errorf("Callback() one block early called = %v, calls = %d", called, callbackCalls)
	}

	called, err = observer.Callback(context.Background(), 159)
	if err != nil {
		t.Fatalf("Callback() at boundary error = %v", err)
	}
	if !called || callbackCalls != 1 {
		t.Errorf("Callback() at boundary called = %v, calls = %d", called, callbackCalls)
	}
}
