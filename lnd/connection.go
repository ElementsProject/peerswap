package lnd

import (
	"context"
	"fmt"
	"time"

	"github.com/elementsproject/peerswap/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/status"
)

// WaitForReady checks on the status of a grpc client connection. We wait until
// the connection is READY or until timeout. Is a blocking call. Returns an
// error on timeout.
func WaitForReady(conn *grpc.ClientConn, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	state := conn.GetState()
	if state == connectivity.Ready {
		return nil
	}

	log.Debugf("Waiting for client connection to be READY: current state: %s", state)

	for {
		ok := conn.WaitForStateChange(ctx, state)
		if !ok {
			return fmt.Errorf("waiting for client connection to be READY: timeout")
		}
		state = conn.GetState()
		log.Debugf("Waiting for client connection to be READY: state changed: %s", state)
		if state == connectivity.Ready {
			return nil
		}
	}
}

// IsContextError returns true if the error is of grpc error type Canceled or
// DeadlineExceeded. A ContextError always indicates that a grpc stream was
// closed by the client side.
func IsContextError(err error) bool {
	code := status.Code(err)
	return code == codes.DeadlineExceeded || code == codes.Canceled
}
