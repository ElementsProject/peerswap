package lnd

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/swap"
	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const recoveredPreimage = "preimage"

type claimRecoveryServer struct {
	routerrpc.UnimplementedRouterServer
	code codes.Code
}

func (s *claimRecoveryServer) TrackPaymentV2(
	*routerrpc.TrackPaymentRequest,
	routerrpc.Router_TrackPaymentV2Server,
) error {
	return status.Error(s.code, "payment lookup result")
}

func TestRecoverClaimPaymentPreservesStatusThroughRetryInterceptor(t *testing.T) {
	for _, code := range []codes.Code{codes.NotFound, codes.Unknown} {
		t.Run(code.String(), func(t *testing.T) {
			listener := bufconn.Listen(1024 * 1024)
			server := grpc.NewServer()
			routerrpc.RegisterRouterServer(server, &claimRecoveryServer{code: code})
			serveErr := make(chan error, 1)
			go func() { serveErr <- server.Serve(listener) }()
			t.Cleanup(func() {
				server.Stop()
				if err := <-serveErr; err != nil {
					t.Errorf("gRPC server failed: %v", err)
				}
			})

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			conn, err := grpc.DialContext(ctx, "passthrough:///recovery",
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
					return listener.DialContext(ctx)
				}),
				grpc.WithStreamInterceptor(grpc_retry.StreamClientInterceptor(
					grpc_retry.WithAlwaysRetry(),
					grpc_retry.WithMax(defaultMaxGrpcRetries),
					grpc_retry.WithBackoff(func(uint) time.Duration { return defaultGrpcBackoffTime }),
				)),
			)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := conn.Close(); err != nil {
					t.Error(err)
				}
			})
			client := &Client{ctx: ctx, lndClient: &recoveryLightning{}, routerClient: routerrpc.NewRouterClient(conn)}
			preimage, err := client.RecoverClaimPayment("invoice")
			var wantErr error
			if code == codes.NotFound {
				wantErr = swap.ErrClaimPaymentNotFound
			} else if status.Code(err) != code {
				t.Fatalf("expected original RPC status %s, got %v", code, err)
			}
			assertRecoveryOutcome(t, preimage, err, false, wantErr)
			if ctx.Err() != nil {
				t.Fatalf("status lookup waited for the retry deadline: %v", ctx.Err())
			}
		})
	}
}

type recoveryLightning struct{ lnrpc.LightningClient }

func (*recoveryLightning) DecodePayReq(
	context.Context,
	*lnrpc.PayReqString,
	...grpc.CallOption,
) (*lnrpc.PayReq, error) {
	return &lnrpc.PayReq{PaymentHash: strings.Repeat("ab", 32)}, nil
}

type recoveryRouter struct {
	routerrpc.RouterClient
	stream  *recoveryPaymentStream
	err     error
	request *routerrpc.TrackPaymentRequest
	ctx     context.Context
}

func (r *recoveryRouter) TrackPaymentV2(
	ctx context.Context,
	request *routerrpc.TrackPaymentRequest,
	_ ...grpc.CallOption,
) (routerrpc.Router_TrackPaymentV2Client, error) {
	r.request = request
	r.ctx = ctx
	if r.stream != nil {
		r.stream.ctx = ctx
	}
	return r.stream, r.err
}

type recoveryPaymentStream struct {
	routerrpc.Router_TrackPaymentV2Client
	updates []*lnrpc.Payment
	err     error
	ctx     context.Context
	block   bool
}

func (s *recoveryPaymentStream) Recv() (*lnrpc.Payment, error) {
	if s.block {
		<-s.ctx.Done()
		return nil, s.ctx.Err()
	}
	if len(s.updates) == 0 {
		if s.err != nil {
			return nil, s.err
		}
		return nil, io.EOF
	}
	payment := s.updates[0]
	s.updates = s.updates[1:]
	return payment, nil
}

func TestRecoverClaimPayment(t *testing.T) {
	for _, test := range []struct {
		name     string
		updates  []*lnrpc.Payment
		trackErr error
		recvErr  error
		wantErr  error
		wantPaid bool
	}{
		{name: "missing on stream creation", trackErr: status.Error(codes.NotFound, "missing"), wantErr: swap.ErrClaimPaymentNotFound},
		{name: "missing on receive", recvErr: status.Error(codes.NotFound, "missing"), wantErr: swap.ErrClaimPaymentNotFound},
		{name: "failed", updates: []*lnrpc.Payment{{Status: lnrpc.Payment_FAILED}}, wantErr: swap.ErrClaimPaymentFailed},
		{name: "succeeded", updates: []*lnrpc.Payment{{Status: lnrpc.Payment_SUCCEEDED, PaymentPreimage: recoveredPreimage}}, wantPaid: true},
		{name: "in flight snapshot", updates: []*lnrpc.Payment{{Status: lnrpc.Payment_IN_FLIGHT}, {Status: lnrpc.Payment_SUCCEEDED, PaymentPreimage: recoveredPreimage}}},
		{name: "initiated snapshot", updates: []*lnrpc.Payment{{Status: lnrpc.Payment_INITIATED}, {Status: lnrpc.Payment_SUCCEEDED, PaymentPreimage: recoveredPreimage}}},
		{name: "in flight then connection lost", updates: []*lnrpc.Payment{{Status: lnrpc.Payment_IN_FLIGHT}}, recvErr: status.Error(codes.Unavailable, "disconnected")},
		{name: "tracking unavailable", trackErr: status.Error(codes.Unavailable, "disconnected")},
		{name: "success without preimage", updates: []*lnrpc.Payment{{Status: lnrpc.Payment_SUCCEEDED}}},
		{name: "unknown", updates: []*lnrpc.Payment{{Status: lnrpc.Payment_PaymentStatus(99)}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := &recoveryRouter{
				stream: &recoveryPaymentStream{updates: test.updates, err: test.recvErr},
				err:    test.trackErr,
			}
			client := &Client{ctx: context.Background(), lndClient: &recoveryLightning{}, routerClient: router}
			preimage, err := client.RecoverClaimPayment("invoice")
			assertRecoveryOutcome(t, preimage, err, test.wantPaid, test.wantErr)
			if router.request == nil || len(router.request.PaymentHash) != 32 || router.request.NoInflightUpdates {
				t.Fatalf("unexpected tracking request: %+v", router.request)
			}
			assertRecoveryQueryClosed(t, router.ctx)
		})
	}
}

func assertRecoveryQueryClosed(t *testing.T, ctx context.Context) {
	t.Helper()
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 10*time.Second {
		t.Fatal("recovery query must have a bounded deadline")
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("recovery must close its tracking stream after the snapshot: %v", ctx.Err())
	}
}

func TestRecoverClaimPaymentRechecksPendingSnapshot(t *testing.T) {
	router := &recoveryRouter{stream: &recoveryPaymentStream{updates: []*lnrpc.Payment{
		{Status: lnrpc.Payment_IN_FLIGHT},
		{Status: lnrpc.Payment_SUCCEEDED, PaymentPreimage: recoveredPreimage},
	}}}
	client := &Client{ctx: context.Background(), lndClient: &recoveryLightning{}, routerClient: router}
	preimage, err := client.RecoverClaimPayment("invoice")
	assertRecoveryOutcome(t, preimage, err, false, nil)
	preimage, err = client.RecoverClaimPayment("invoice")
	assertRecoveryOutcome(t, preimage, err, true, nil)
}

func TestRecoverClaimPaymentUnresponsiveStream(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	router := &recoveryRouter{stream: &recoveryPaymentStream{block: true}}
	client := &Client{ctx: ctx, lndClient: &recoveryLightning{}, routerClient: router}
	preimage, err := client.RecoverClaimPayment("invoice")
	assertRecoveryOutcome(t, preimage, err, false, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected bounded query failure, got %v", err)
	}
}

func assertRecoveryOutcome(t *testing.T, preimage string, err error, wantPaid bool, wantErr error) {
	t.Helper()
	switch {
	case wantPaid:
		if err != nil || preimage != recoveredPreimage {
			t.Fatalf("expected recovered preimage, got %q, %v", preimage, err)
		}
	case wantErr != nil:
		if !errors.Is(err, wantErr) {
			t.Fatalf("expected %v, got %v", wantErr, err)
		}
	default:
		if err == nil || errors.Is(err, swap.ErrClaimPaymentNotFound) || errors.Is(err, swap.ErrClaimPaymentFailed) {
			t.Fatalf("unresolved payment must not authorize key release: %v", err)
		}
	}
}
