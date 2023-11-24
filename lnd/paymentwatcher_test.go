package lnd

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/cmd/peerswaplnd"
	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/test"
	"github.com/elementsproject/peerswap/testframework"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

// TestPaymentWatcher_WatchPayment tests the basic functionality of the payment
// watcher. A callback is added to the watcher and a payment request is added to
// the watcher. We expect the callback to be called after the payment request
// was payed for.
func TestPaymentWatcher_WatchPayment(t *testing.T) {
	test.IsIntegrationTest(t)
	t.Parallel()

	// Setup bitcoind and lnd.
	tmpDir := t.TempDir()
	_, payer, payee, cc, err := paymentwatcherNodeSetup(t, tmpDir)
	if err != nil {
		t.Fatalf("Could not create lnd client connection: %v", err)
	}

	ctx := context.Background()

	// Create new payment watcher
	paymentwatcher, err := NewPaymentWatcher(ctx, cc)
	if err != nil {
		t.Fatalf("Could not create tx watcher: %v", err)
	}

	payreq, err := payee.AddInvoice(100000, "testpaymentwatcher_payinvoice", "")
	if err != nil {
		t.Fatalf("Could not add invoice: %v", err)
	}

	// Add a payment callback.
	var gotCallback bool
	paymentwatcher.AddPaymentCallback(func(swapId string, invoiceType swap.InvoiceType) {
		gotCallback = true
	})

	paymentwatcher.AddWaitForPayment("myswap", payreq, swap.INVOICE_FEE)

	err = testframework.WaitFor(func() bool {
		return gotCallback
	}, 5*time.Second)
	if err == nil {
		t.Fatalf("Failed waiting for confirmation callback being called: %v", err)
	}

	err = payer.PayInvoice(payreq)
	if err != nil {
		t.Fatalf("Failed PayInvoice(): %v", err)
	}

	err = testframework.WaitFor(func() bool {
		return gotCallback
	}, 50*time.Second)
	if err != nil {
		t.Fatalf("Failed waiting for confirmation callback being called: %v", err)
	}
}

// TestPaymentWatcher_WatchPayment_Reconnect tests that the payment watcher is
// still receiving payments after the node that the payment watcher is
// subscribed to was killed and restarted after 1s to 5s. We expect the callback
// to be called after the payment request was payed for.
func TestPaymentWatcher_WatchPayment_Reconnect(t *testing.T) {
	test.IsIntegrationTest(t)
	t.Parallel()

	// Setup bitcoind and lnd.
	tmpDir := t.TempDir()
	_, payer, payee, cc, err := paymentwatcherNodeSetup(t, tmpDir)
	if err != nil {
		t.Fatalf("Could not create lnd client connection: %v", err)
	}

	ctx := context.Background()

	// Create new payment watcher
	paymentwatcher, err := NewPaymentWatcher(ctx, cc)
	if err != nil {
		t.Fatalf("Could not create tx watcher: %v", err)
	}

	payreq, err := payee.AddInvoice(100000, "testpaymentwatcher_payinvoice", "")
	if err != nil {
		t.Fatalf("Could not add invoice: %v", err)
	}

	// Add a payment callback.
	var gotCallback bool
	paymentwatcher.AddPaymentCallback(func(swapId string, invoiceType swap.InvoiceType) {
		gotCallback = true
	})

	paymentwatcher.AddWaitForPayment("myswap", payreq, swap.INVOICE_FEE)

	// We expect that the callback was not called at this point. Then we kill the
	// payee side (the one with the payment watcher) and restart it in 1s to 5s.
	assert.False(t, gotCallback)
	payee.Kill()
	// Restart lnd
	n := rand.Intn(5) + 1
	time.Sleep(time.Duration(n) * time.Second)
	err = payee.Run(true, true)
	if err != nil {
		t.Fatalf("Failed Run(): %v", err)
	}

	// We still assert the callback was not called by now.
	assert.False(t, gotCallback)

	// We have to reconnect in order to pay the invoice.
	payer.Connect(payee, true)

	// Now we pay the invoice and check if the watcher is still active. If the
	// watcher is still active, the callback should be called.
	err = payer.PayInvoice(payreq)
	if err != nil {
		t.Fatalf("Failed PayInvoice(): %v", err)
	}
	err = testframework.WaitFor(func() bool {
		return gotCallback
	}, 50*time.Second)
	if err != nil {
		t.Fatalf("Failed waiting for confirmation callback being called: %v", err)
	}
}

// TestPaymentWatcher_WatchPayment_Reconnect_OnGracefulStop tests that the
// payment watcher is still receiving payments after the node that the payment
// watcher is subscribed to was gracefully shutdown and restarted after 1s to
// 5s. We expect the callback to be called after the payment request was payed
// for.
func TestPaymentWatcher_WatchPayment_Reconnect_OnGracefulStop(t *testing.T) {
	test.IsIntegrationTest(t)
	t.Parallel()

	// Setup bitcoind and lnd.
	tmpDir := t.TempDir()
	_, payer, payee, cc, err := paymentwatcherNodeSetup(t, tmpDir)
	if err != nil {
		t.Fatalf("Could not create lnd client connection: %v", err)
	}

	ctx := context.Background()

	// Create new payment watcher
	paymentwatcher, err := NewPaymentWatcher(ctx, cc)
	if err != nil {
		t.Fatalf("Could not create tx watcher: %v", err)
	}

	payreq, err := payee.AddInvoice(100000, "testpaymentwatcher_payinvoice", "")
	if err != nil {
		t.Fatalf("Could not add invoice: %v", err)
	}

	// Add a payment callback.
	var gotCallback bool
	paymentwatcher.AddPaymentCallback(func(swapId string, invoiceType swap.InvoiceType) {
		gotCallback = true
	})

	paymentwatcher.AddWaitForPayment("myswap", payreq, swap.INVOICE_FEE)

	// We expect that the callback was not called at this point. Then we kill the
	// payee side (the one with the payment watcher) and restart it in 1s to 5s.
	assert.False(t, gotCallback)
	_, err = payee.Rpc.StopDaemon(context.Background(), &lnrpc.StopRequest{})
	if err != nil {
		t.Fatalf("Failed StopDaemon(): %v", err)
	}
	// Restart lnd
	n := rand.Intn(5) + 1
	time.Sleep(time.Duration(n) * time.Second)
	err = payee.Run(true, true)
	if err != nil {
		t.Fatalf("Failed Run(): %v", err)
	}

	// We still assert the callback was not called by now.
	assert.False(t, gotCallback)

	// We have to reconnect in order to pay the invoice.
	payer.Connect(payee, true)

	// Now we pay the invoice and check if the watcher is still active. If the
	// watcher is still active, the callback should be called.
	err = payer.PayInvoice(payreq)
	if err != nil {
		t.Fatalf("Failed PayInvoice(): %v", err)
	}
	err = testframework.WaitFor(func() bool {
		return gotCallback
	}, 50*time.Second)
	if err != nil {
		t.Fatalf("Failed waiting for confirmation callback being called: %v", err)
	}
}

func paymentwatcherNodeSetup(t *testing.T, dir string) (
	bitcoind *testframework.BitcoinNode,
	payer *testframework.LndNode,
	payee *testframework.LndNode,
	cc *grpc.ClientConn, err error,
) {
	bitcoind, err = testframework.NewBitcoinNode(dir, 1)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("Can not create bitcoin node: %v", err)
	}

	payer, err = testframework.NewLndNode(dir, bitcoind, 1, nil)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("Can not create lnd node: %v", err)
	}

	payee, err = testframework.NewLndNode(dir, bitcoind, 1, nil)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("Can not create lnd node: %v", err)
	}

	if err := bitcoind.Run(true); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("Can not start bitcoind: %v", err)
	}
	t.Cleanup(bitcoind.Kill)

	if err := payer.Run(true, true); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("Can not start lnd: %v", err)
	}
	t.Cleanup(payer.Kill)

	if err := payee.Run(true, true); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("Can not start lnd: %v", err)
	}
	t.Cleanup(payee.Kill)

	// Create a client connection to the lnd node. And a new lnd client.
	cc, err = getClientConnectionForTests(
		context.Background(),
		&peerswaplnd.LndConfig{
			LndHost:      fmt.Sprintf("localhost:%d", payee.RpcPort),
			TlsCertPath:  payee.TlsPath,
			MacaroonPath: payee.MacaroonPath,
		},
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("Could not create lnd client connection: %v", err)
	}

	_, err = payer.OpenChannel(payee, uint64(math.Pow10(7)), 0, true, true, true)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("Could not open channel: %v", err)
	}

	return bitcoind, payer, payee, cc, nil
}
