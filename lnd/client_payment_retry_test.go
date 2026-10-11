package lnd

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const paymentRetryPeer = "peer"

type paymentRetryServer struct {
	routerrpc.UnimplementedRouterServer
	sendInFlight bool
	sends        atomic.Int32
}

type paymentRetryLightningServer struct {
	lnrpc.UnimplementedLightningServer
}

func (*paymentRetryLightningServer) DecodePayReq(context.Context, *lnrpc.PayReqString) (*lnrpc.PayReq, error) {
	return &lnrpc.PayReq{Destination: paymentRetryPeer, NumSatoshis: 1, NumMsat: 1_000, CltvExpiry: 503}, nil
}

func (*paymentRetryLightningServer) ListChannels(
	context.Context,
	*lnrpc.ListChannelsRequest,
) (*lnrpc.ListChannelsResponse, error) {
	return &lnrpc.ListChannelsResponse{Channels: []*lnrpc.Channel{
		{ChanId: 42, RemotePubkey: paymentRetryPeer, LocalBalance: 1_000},
	}}, nil
}

func (s *paymentRetryServer) SendPaymentV2(
	_ *routerrpc.SendPaymentRequest,
	stream routerrpc.Router_SendPaymentV2Server,
) error {
	if s.sends.Add(1) == 1 {
		if s.sendInFlight {
			if err := stream.Send(&lnrpc.Payment{Status: lnrpc.Payment_IN_FLIGHT}); err != nil {
				return err
			}
		}
		return status.Error(codes.Unavailable, "payment stream interrupted")
	}
	return stream.Send(&lnrpc.Payment{Status: lnrpc.Payment_SUCCEEDED, PaymentPreimage: recoveredPreimage})
}

func newPaymentRetryClient(t *testing.T, sendInFlight bool) (*Client, *paymentRetryServer) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	backend := &paymentRetryServer{sendInFlight: sendInFlight}
	lnrpc.RegisterLightningServer(server, &paymentRetryLightningServer{})
	routerrpc.RegisterRouterServer(server, backend)
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		if err := <-serveErr; err != nil {
			t.Errorf("gRPC server failed: %v", err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	conn, err := grpc.DialContext(ctx, "passthrough:///payment-retry",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithStreamInterceptor(grpc_retry.StreamClientInterceptor(
			grpc_retry.WithAlwaysRetry(),
			grpc_retry.WithMax(3),
			grpc_retry.WithBackoff(func(uint) time.Duration { return time.Millisecond }),
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
	return &Client{
		ctx:          ctx,
		lndClient:    lnrpc.NewLightningClient(conn),
		routerClient: routerrpc.NewRouterClient(conn),
	}, backend
}

func TestClaimPaymentDoesNotRetryInterruptedStream(t *testing.T) {
	for _, stage := range []struct {
		name         string
		sendInFlight bool
	}{
		{name: "before first update"},
		{name: "after in flight update", sendInFlight: true},
	} {
		for _, payment := range []struct {
			name      string
			pay       func(*Client) (string, error)
			wantSends int32
		}{
			{"claim", func(c *Client) (string, error) { return c.RebalancePayment("invoice", "0:0:42", 506) }, 1},
			{"fee", func(c *Client) (string, error) { return c.PayInvoiceViaChannel("invoice", "0:0:42") }, 2},
			{"ordinary", func(c *Client) (string, error) { return c.PayInvoice("invoice") }, 2},
		} {
			t.Run(stage.name+"/"+payment.name, func(t *testing.T) {
				client, backend := newPaymentRetryClient(t, stage.sendInFlight)
				preimage, err := payment.pay(client)
				if got := backend.sends.Load(); got != payment.wantSends {
					t.Fatalf("SendPaymentV2 calls: got %d, want %d", got, payment.wantSends)
				}
				if payment.wantSends == 1 {
					if status.Code(err) != codes.Unavailable || preimage != "" {
						t.Fatalf("claim must return the stream failure for status recovery, got %q, %v", preimage, err)
					}
				} else if err != nil || preimage != recoveredPreimage {
					t.Fatalf("expected existing retry behavior to succeed, got %q, %v", preimage, err)
				}
			})
		}
	}
}
