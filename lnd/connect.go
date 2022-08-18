package lnd

import (
	"context"
	"io/ioutil"
	"time"

	"github.com/elementsproject/peerswap/cmd/peerswaplnd"
	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"github.com/lightningnetwork/lnd/macaroons"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"
)

const (
	// defaultGrpcBackoffTime is the linear backoff time between failing grpc
	// calls (also server side stream) to the lnd node.
	defaultGrpcBackoffTime   = 1 * time.Second
	defaultGrpcBackoffJitter = 0.1

	// defaultMaxGrpcRetries is the amount of retries we take if the grpc
	// connection to the lnd node drops for some reason or if the resource is
	// exhausted. With the defaultGrpcBackoffTime and a linear backoff stratefgy
	// this leads to roughly 5h of retry time.
	defaultMaxGrpcRetries = 5
)

var (
	// defaultGrpcRetryCodes are the grpc status codes that are returned with an
	// error, on which we retry our call (and server side stream) to the lnd
	// node. The codes represent:
	// - Unavailable:	The service is currently unavailable. This is most
	//					likely a transient condition, which can be correctesd by
	//					retrying with a backoff. Note that it is not always safe
	//					to retry non-idempotent operations.
	//
	// - ResourceExhausted:	Some resource has been exhausted, perhaps a per-user
	//						quota, or perhaps the entire file system is out of
	//						space.
	defaultGrpcRetryCodes []codes.Code = []codes.Code{
		codes.Unavailable,
		codes.ResourceExhausted,
	}

	// defaultGrpcRetryCodesWithMsg are grpc status codes that must have a
	// matching message for us to retry. This is due to LND using a confusing
	// rpc error code on startup.
	// See: https://github.com/lightningnetwork/lnd/issues/6765
	//
	// This is also the reason that we need to use a fork of the
	// go-grpc-middleware "retry" to provide this optional check.
	defaultGrpcRetryCodesWithMsg []grpc_retry.CodeWithMsg = []grpc_retry.CodeWithMsg{
		{
			Code: codes.Unknown,
			Msg:  "the RPC server is in the process of starting up, but not yet ready to accept calls",
		},
		{
			Code: codes.Unknown,
			Msg:  "server is in the process of starting up, but not yet ready to accept calls",
		},
		{
			Code: codes.Unknown,
			Msg:  "chain notifier RPC is still in the process of starting",
		},
	}
)

func GetClientConnection(ctx context.Context, cfg *peerswaplnd.LndConfig) (*grpc.ClientConn, error) {
	creds, err := credentials.NewClientTLSFromFile(cfg.TlsCertPath, "")
	if err != nil {
		return nil, err
	}
	macBytes, err := ioutil.ReadFile(cfg.MacaroonPath)
	if err != nil {
		return nil, err
	}
	mac := &macaroon.Macaroon{}
	if err := mac.UnmarshalBinary(macBytes); err != nil {
		return nil, err
	}
	cred, err := macaroons.NewMacaroonCredential(mac)
	if err != nil {
		return nil, err
	}
	maxMsgRecvSize := grpc.MaxCallRecvMsgSize(1 * 1024 * 1024 * 500)

	retryOptions := []grpc_retry.CallOption{
		grpc_retry.WithBackoff(
			grpc_retry.BackoffExponentialWithJitter(
				defaultGrpcBackoffTime,
				defaultGrpcBackoffJitter,
			),
		),
		grpc_retry.WithCodes(defaultGrpcRetryCodes...),
		grpc_retry.WithCodesAndMatchingMessage(defaultGrpcRetryCodesWithMsg...),
		grpc_retry.WithMax(defaultMaxGrpcRetries),
	}
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithBlock(),
		grpc.WithPerRPCCredentials(cred),
		grpc.WithDefaultCallOptions(maxMsgRecvSize),
		grpc.WithStreamInterceptor(grpc_retry.StreamClientInterceptor(
			retryOptions...,
		)),
		grpc.WithUnaryInterceptor(grpc_retry.UnaryClientInterceptor(
			retryOptions...,
		)),
	}
	conn, err := grpc.DialContext(ctx, cfg.LndHost, opts...)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
