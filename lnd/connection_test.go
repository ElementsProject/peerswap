package lnd

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	testgrpc "google.golang.org/grpc/test/grpc_testing"
	testpb "google.golang.org/grpc/test/grpc_testing"
)

func TestWaitForReady_OnStop(t *testing.T) {
	t.Parallel()

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("net.Listen() failed: %v", err)
	}
	defer l.Close()

	grpcServerExitDone := &sync.WaitGroup{}
	s, _ := newTestServer(t, l, grpcServerExitDone)
	cc, err := grpc.Dial(l.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("grpc.Dial() failed: %v", err)
	}

	// Expecting waitForReady to return without an error as the state should be
	// READY.
	err = WaitForReady(cc, 10*time.Second)
	if err != nil {
		t.Fatalf("waitForReady failed: %v", err)
	}

	// Stop the server to drop the connection.
	s.Stop()
	grpcServerExitDone.Wait()
	// Wait for connection to drop.
	cc.WaitForStateChange(context.Background(), connectivity.Ready)

	wg := sync.WaitGroup{}
	wg.Add(1)
	var wfErr error
	// We expect waitForReady to return without an error detecting the fixed
	// connection once we spin up the server again.
	go func() {
		defer wg.Done()
		wfErr = WaitForReady(cc, 10*time.Second)
	}()

	// Restart the server on the same network and address.
	l, err = net.Listen(l.Addr().Network(), l.Addr().String())
	if err != nil {
		t.Errorf("net.Listen() failed: %v", err)
	}
	defer l.Close()
	s, _ = newTestServer(t, l, grpcServerExitDone)

	// Wait here until go routine above returns indicating that waitForReady has
	// returned due to the state being READY again or timeout. We are expecting
	// no timeout error.
	wg.Wait()
	if wfErr != nil {
		t.Fatalf("waitForReady failed: %v", wfErr)
	}

	s.Stop()
	grpcServerExitDone.Wait()
}

func TestWaitForReady_OnGracefulStop(t *testing.T) {
	t.Parallel()

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("net.Listen() failed: %v", err)
	}
	defer l.Close()

	grpcServerExitDone := &sync.WaitGroup{}
	s, _ := newTestServer(t, l, grpcServerExitDone)
	cc, err := grpc.Dial(l.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("grpc.Dial() failed: %v", err)
	}

	// Expecting waitForReady to return without an error as the state should be
	// READY.
	err = WaitForReady(cc, 10*time.Second)
	if err != nil {
		t.Fatalf("waitForReady failed: %v", err)
	}

	// Stop the server to drop the connection.
	s.GracefulStop()
	grpcServerExitDone.Wait()
	// Wait for connection to drop.
	cc.WaitForStateChange(context.Background(), connectivity.Ready)

	wg := sync.WaitGroup{}
	wg.Add(1)
	var wfErr error
	// We expect waitForReady to return without an error detecting the fixed
	// connection once we spin up the server again.
	go func() {
		defer wg.Done()
		wfErr = WaitForReady(cc, 10*time.Second)
	}()

	// Restart the server on the same network and address.
	l, err = net.Listen(l.Addr().Network(), l.Addr().String())
	if err != nil {
		t.Errorf("net.Listen() failed: %v", err)
	}
	defer l.Close()
	s, _ = newTestServer(t, l, grpcServerExitDone)

	// Wait here until go routine above returns indicating that waitForReady has
	// returned due to the state being READY again or timeout. We are expecting
	// no timeout error.
	wg.Wait()
	if wfErr != nil {
		t.Fatalf("waitForReady failed: %v", wfErr)
	}

	s.Stop()
	grpcServerExitDone.Wait()
}

func TestResubscribeToStream(t *testing.T) {
	t.Skip("This test is flaky and we do not strictly need it.")
	t.Parallel()

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("net.Listen() failed: %v", err)
	}
	defer l.Close()

	grpcServerExitDone := &sync.WaitGroup{}
	s, ts := newTestServer(t, l, grpcServerExitDone)

	cc, err := grpc.Dial(l.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("grpc.Dial() failed: %v", err)
	}
	client := testgrpc.NewTestServiceClient(cc)

	wg := sync.WaitGroup{}
	wg.Add(1)

	resCh := make(chan bool)
	errs := make(chan error, 1)
	// This is an example implementation of a service that is subscribed to a
	// stream.
	go func() {
		defer wg.Done()

		// Subscribe to stream.
		stream, err := client.StreamingOutputCall(
			context.Background(),
			&testpb.StreamingOutputCallRequest{},
		)
		if err != nil {
			t.Logf("StreamingOutputCall() failed: %v", err)
			errs <- err
			return
		}

		for {
			_, err := stream.Recv()
			if err == io.EOF {
				// If we receive an io.EOF error this means that the stream has
				// been closed by the server.
				t.Log("Server closed connection")
				return
			}
			if err != nil {
				// If we received a different error than an io.EOF error this is
				// an indication that the server has crashed or been stopped. If
				// this is the case we want to wait until we can reconnect.
				// After we are reconnected to the server, we open a new stream.
				t.Logf("Got an error: %v\n", err)
				errs <- err

				WaitForReady(cc, 20*time.Second)

				// Subscibe to new stream.
				stream, err = client.StreamingOutputCall(
					context.Background(),
					&testpb.StreamingOutputCallRequest{},
				)
				if err != nil {
					t.Logf("StreamingOutputCall() failed: %v", err)
					errs <- err
				}
				continue
			}
			// We got a response from the server: handle response.
			resCh <- true

		}
	}()

	// Check if stream is receiving the response.
	ts.sendStreamingOutput()
	<-resCh

	// Stop server to break the grpc connection. Expecting a connection error
	// from the server.
	s.Stop()
	grpcServerExitDone.Wait()
	<-errs

	// Restart server.
	l, err = net.Listen(l.Addr().Network(), l.Addr().String())
	if err != nil {
		t.Errorf("net.Listen() failed: %v", err)
	}
	defer l.Close()
	s, ts = newTestServer(t, l, grpcServerExitDone)

	// The example service should reconnect and resubscribe to the stream. We
	// are expecting that the service is receiving a response.
	ts.sendStreamingOutput()
	<-resCh

	// Stop the stream and wait for example service go routine to finish.
	ts.stopStreamingOutput()
	wg.Wait()
	s.Stop()
	grpcServerExitDone.Wait()
}

// testServer is a minimalistic grpc server mock that allows us to send to a
// stream as well as closing a stream on demand.
type testServer struct {
	t *testing.T
	testpb.UnimplementedTestServiceServer

	sendOutputCh chan bool
	stop         chan bool
}

func newTestServer(t *testing.T, l net.Listener, wg *sync.WaitGroup) (*grpc.Server, *testServer) {
	s := grpc.NewServer()
	ts := &testServer{
		t:            t,
		sendOutputCh: make(chan bool),
		stop:         make(chan bool),
	}
	testgrpc.RegisterTestServiceServer(s, ts)
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := s.Serve(l)
		if err != nil {
			t.Logf("s.Serve() failed: %v", err)
		}
	}()
	return s, ts
}

func (t testServer) EmptyCall(context.Context, *testpb.Empty) (*testpb.Empty, error) {
	t.t.Log("EmptyCall was called")
	return &testpb.Empty{}, nil
}

// StreamingOutputCall sends a message to the stream when triggered by the
// sendOutputCh channel.
func (t testServer) StreamingOutputCall(req *testpb.StreamingOutputCallRequest, srv testpb.TestService_StreamingOutputCallServer) error {
	for {
		select {
		case <-t.sendOutputCh:
			go func() {
				if err := srv.Send(
					&testpb.StreamingOutputCallResponse{
						Payload: &testpb.Payload{
							Type: testpb.PayloadType_RANDOM,
							Body: []byte("whatever"),
						},
					},
				); err != nil {
					t.t.Fatalf("Send() failed: %v", err)
				}
			}()
		case <-t.stop:
			return nil
		}
	}
}

// sendStreamingOutput triggers the server to send a message to the stream.
func (t *testServer) sendStreamingOutput() {
	t.sendOutputCh <- true
}

// stopStreamingOutput stops a stream gracefully.
func (t *testServer) stopStreamingOutput() {
	t.stop <- true
}
