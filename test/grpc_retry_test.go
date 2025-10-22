package test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/cmd/peerswaplnd"
	"github.com/elementsproject/peerswap/lightning"
	peerswaplndinternal "github.com/elementsproject/peerswap/lnd"
	"github.com/elementsproject/peerswap/testframework"
	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"github.com/lightningnetwork/lnd/lnrpc"
	"google.golang.org/grpc/codes"
)

func Test_GrpcRetryRequest(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	// Setup bitcoind and lnd.
	tmpDir := makeTestDataDir(t)
	bitcoind, err := testframework.NewBitcoinNode(tmpDir, 1)
	if err != nil {
		t.Fatalf("Can not create bitcoin node: %v", err)
	}

	lnd, err := testframework.NewLndNode(tmpDir, bitcoind, 1, nil)
	if err != nil {
		t.Fatalf("Can not create lnd node: %v", err)
	}

	if err := bitcoind.Run(true); err != nil {
		t.Fatalf("Can not start bitcoind: %v", err)
	}
	t.Cleanup(bitcoind.Kill)

	if err := lnd.Run(true, true); err != nil {
		t.Fatalf("Can not start lnd: %v", err)
	}
	t.Cleanup(lnd.Kill)

	// Create a client connection to the lnd node. And a new lnd client.
    cc, err := peerswaplndinternal.GetClientConnectionShortBackoff(
        context.Background(),
        &peerswaplnd.LndConfig{
            LndHost:      fmt.Sprintf("localhost:%d", lnd.RpcPort),
            TlsCertPath:  lnd.TlsPath,
            MacaroonPath: lnd.MacaroonPath,
        },
    )
	if err != nil {
		t.Fatalf("Could not create lnd client connection: %v", err)
	}
	lnrpcClient := lnrpc.NewLightningClient(cc)

	// Assert that getinfo returns no error.
	_, err = lnrpcClient.GetInfo(context.Background(), &lnrpc.GetInfoRequest{})
	if err != nil {
		t.Fatalf("GetInfo() failed: %v", err)
	}

	// We now kill the lnd node and fire a request in the go routine. We wait a
	// random time between 1 and 6 seconds and restart the node. We expect the
	// call to retry and return with no error.
	lnd.Kill()

	wg := sync.WaitGroup{}
	wg.Add(1)

	var fErr error
	go func() {
		defer wg.Done()
		_, fErr = lnrpcClient.GetInfo(context.Background(), &lnrpc.GetInfoRequest{})
	}()

	const restartDelay = 2 * time.Second
	time.Sleep(restartDelay)
	if err := lnd.Run(true, true); err != nil {
		t.Fatalf("Cannot restart lnd: %v", err)
	}

	// Wait for GetInfo to return
	wg.Wait()
	assertNoError(t, fErr)
}

func Test_GrpcReconnectStream(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	// Setup bitcoind and lnd.
	tmpDir := makeTestDataDir(t)
	bitcoind, err := testframework.NewBitcoinNode(tmpDir, 1)
	if err != nil {
		t.Fatalf("Can not create bitcoin node: %v", err)
	}

	lnd, err := testframework.NewLndNode(tmpDir, bitcoind, 1, nil)
	if err != nil {
		t.Fatalf("Can not create lnd node: %v", err)
	}

	if err := bitcoind.Run(true); err != nil {
		t.Fatalf("Can not start bitcoind: %v", err)
	}
	t.Cleanup(bitcoind.Kill)

	if err := lnd.Run(true, true); err != nil {
		t.Fatalf("Can not start lnd: %v", err)
	}
	t.Cleanup(lnd.Kill)

	// Create a client connection to the lnd node. And a new lnd client.
    cc, err := peerswaplndinternal.GetClientConnectionShortBackoff(
        context.Background(),
        &peerswaplnd.LndConfig{
            LndHost:      fmt.Sprintf("localhost:%d", lnd.RpcPort),
            TlsCertPath:  lnd.TlsPath,
            MacaroonPath: lnd.MacaroonPath,
        },
    )
	if err != nil {
		t.Fatalf("Could not create lnd client connection: %v", err)
	}
	lnrpcClient := lnrpc.NewLightningClient(cc)

	// Add some invoices to prepopulate the database. We can only subscribe to
	// invoices with an AddIndex > 1 if we want to return the invoices that we
	// missed again.
	for i := range 10 {
		preimage, _ := lightning.GetPreimage()
		_, err = lnrpcClient.AddInvoice(
			context.Background(),
			&lnrpc.Invoice{
				ValueMsat: int64(1000000) + int64(i),
				Memo:      fmt.Sprintf("memo-%d", i),
				RPreimage: preimage[:],
			},
		)
		if err != nil {
			t.Fatalf("AddInvoice() failed: %v", err)
		}
	}

	// Subscribe to invoices
	wg := sync.WaitGroup{}
	wg.Add(1)

	var fErr error
	var results []*lnrpc.Invoice
	go func() {
		defer wg.Done()

		var stream lnrpc.Lightning_SubscribeInvoicesClient
		stream, fErr = lnrpcClient.SubscribeInvoices(
			context.Background(),
			&lnrpc.InvoiceSubscription{AddIndex: 1},
			grpc_retry.WithMax(10),
			grpc_retry.WithCodesAndMatchingMessage(grpc_retry.CodeWithMsg{
				Code: codes.Unknown,
				Msg:  "the RPC server is in the process of starting up, but not yet ready to accept calls",
			}),
		)
		if fErr != nil {
			return
		}

		var index uint64 = 0
		for {
			if len(results) >= 11 {
				return
			}

			var r *lnrpc.Invoice
			r, fErr = stream.Recv()
			if errors.Is(fErr, io.EOF) {
				return
			} else if fErr != nil {
				return
			}
			if r.AddIndex > index {
				results = append(results, r)
				index = r.AddIndex
			}
		}
	}()

	preimage, _ := lightning.GetPreimage()
	_, err = lnrpcClient.AddInvoice(
		context.Background(),
		&lnrpc.Invoice{
			ValueMsat: int64(1000000),
			Memo:      "mymemo",
			RPreimage: preimage[:],
		},
	)
	if err != nil {
		t.Fatalf("AddInvoice() failed: %v", err)
	}

	err = testframework.WaitFor(func() bool {
		return len(results) > 0
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("WaitFor() failed: %v", err)
	}

	// We now kill the lnd node and wait a random time between 1s and 6s before
	// we restart the node. We expect the stream to resubscribe and return with
	// no error.
	lnd.Kill()

	time.Sleep(2 * time.Second)
	if err := lnd.Run(true, true); err != nil {
		t.Fatalf("Cannot restart lnd: %v", err)
	}

	preimage, _ = lightning.GetPreimage()
	_, err = lnrpcClient.AddInvoice(
		context.Background(),
		&lnrpc.Invoice{
			ValueMsat: int64(1000000),
			Memo:      "lastmemo",
			RPreimage: preimage[:],
		},
		grpc_retry.WithMax(5),
		grpc_retry.WithCodesAndMatchingMessage(grpc_retry.CodeWithMsg{
			Code: codes.Unknown,
			Msg:  "the RPC server is in the process of starting up, but not yet ready to accept calls",
		}),
	)
	if err != nil {
		t.Fatalf("AddInvoice() failed: %v", err)
	}

	wg.Wait()
	assertNoError(t, fErr)
}
