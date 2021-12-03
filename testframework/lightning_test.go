package testframework

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	"github.com/stretchr/testify/require"
)

func TestOpenChannelLndLnd(t *testing.T) {
	testDir := t.TempDir()

	// Settings
	// Inital channel capacity
	var fundAmt = uint64(math.Pow(10, 7))

	// Setup nodes (1 bitcoind, 2 lnd)
	bitcoind, err := NewBitcoinNode(testDir, 1)
	if err != nil {
		t.Fatalf("could not create bitcoind %v", err)
	}
	t.Cleanup(bitcoind.Kill)

	var lightningds []*LndNode
	for i := 1; i <= 2; i++ {
		lightningd, err := NewLndNode(testDir, bitcoind, i)
		if err != nil {
			t.Fatalf("could not create liquidd %v", err)
		}
		t.Cleanup(lightningd.Kill)

		lightningds = append(lightningds, lightningd)
	}

	// Start nodes
	err = bitcoind.Run(true)
	if err != nil {
		t.Fatalf("bitcoind.Run() got err %v", err)
	}

	for _, lightningd := range lightningds {
		err = lightningd.Run(true, true)
		if err != nil {
			t.Fatalf("lightningd.Run() got err %v", err)
		}
	}

	// Setup channel ([0] fundAmt(10^7) ---- 0 [1])
	_, err = lightningds[0].OpenChannel(lightningds[1], fundAmt, true, true, true)
	if err != nil {
		t.Fatalf("lightingds[0].OpenChannel() %v", err)
	}

	// Pay Invoice
	ir, err := lightningds[1].Rpc.AddInvoice(context.Background(), &lnrpc.Invoice{Value: 10000})
	require.NoError(t, err)

	pstream, err := lightningds[0].RpcV2.SendPaymentV2(context.Background(), &routerrpc.SendPaymentRequest{PaymentRequest: ir.PaymentRequest, TimeoutSeconds: int32(TIMEOUT.Seconds())})
	require.NoError(t, err)

	// Wait for settelment.
	err = WaitForWithErr(func() (bool, error) {
		u, err := pstream.Recv()
		if err == nil {
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("Recv() %w", err)
		}

		return u.Status == lnrpc.Payment_SUCCEEDED, nil
	}, TIMEOUT)
	require.NoError(t, err)
}

func TestOpenChannelLndCln(t *testing.T) {
	testDir := t.TempDir()

	// Settings
	// Inital channel capacity
	var fundAmt = uint64(math.Pow(10, 7))

	// Setup nodes (1 bitcoind, 2 lnd)
	bitcoind, err := NewBitcoinNode(testDir, 1)
	if err != nil {
		t.Fatalf("could not create bitcoind %v", err)
	}
	t.Cleanup(bitcoind.Kill)

	lnd, err := NewLndNode(testDir, bitcoind, 1)
	if err != nil {
		t.Fatalf("could not create lnd %v", err)
	}
	t.Cleanup(lnd.Kill)

	cln, err := NewCLightningNode(testDir, bitcoind, 1)
	if err != nil {
		t.Fatalf("could not create cln %v", err)
	}
	t.Cleanup(cln.Kill)

	// Start nodes
	err = bitcoind.Run(true)
	if err != nil {
		t.Fatalf("bitcoind.Run() got err %v", err)
	}

	err = lnd.Run(true, true)
	if err != nil {
		t.Fatalf("lightningd.Run() got err %v", err)
	}

	err = cln.Run(true, true)
	if err != nil {
		t.Fatalf("lightningd.Run() got err %v", err)
	}

	// Setup channel ([0] fundAmt(10^7) ---- 0 [1])
	_, err = lnd.OpenChannel(cln, fundAmt, true, true, true)
	if err != nil {
		t.Fatalf("lnd.OpenChannel() %v", err)
	}

	// Pay Invoice
	invoice, err := cln.Rpc.Invoice(10000000, "mylabel", "mydescr")
	require.NoError(t, err)

	pstream, err := lnd.RpcV2.SendPaymentV2(context.Background(), &routerrpc.SendPaymentRequest{PaymentRequest: invoice.Bolt11, TimeoutSeconds: int32(TIMEOUT.Seconds())})
	require.NoError(t, err)

	// Wait for settelment.
	err = WaitForWithErr(func() (bool, error) {
		u, err := pstream.Recv()
		if err != nil {
			return false, fmt.Errorf("Recv() %w", err)
		}

		return u.Status == lnrpc.Payment_SUCCEEDED, nil
	}, TIMEOUT)
	require.NoError(t, err)
}
