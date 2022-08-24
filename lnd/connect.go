package lnd

import (
	"context"
	"io/ioutil"
	"time"

	"github.com/elementsproject/peerswap/cmd/peerswaplnd"
	"github.com/elementsproject/peerswap/log"
	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"github.com/lightningnetwork/lnd/macaroons"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"

	internal_log "log"
)

const (
	// defaultGrpcBackoffTime is the linear back off time between failing grpc
	// calls (also server side stream) to the lnd node.
	defaultGrpcBackoffTime = 30 * time.Second

	// defaultMaxGrpcRetries is the amount of retries we take if the grpc
	// connection to the lnd node drops for some reason or if the resource is
	// exhausted. With the defaultGrpcBackoffTime and a constant back off
	// strategy this leads to 10 hours of retry.
	defaultMaxGrpcRetries = 1200
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

	debugLogger := internal_log.New(log.NewDebugLogger(), "[grpc_conn]: ", 0)
	retryOptions := []grpc_retry.CallOption{
		grpc_retry.WithBackoff(func(_ uint) time.Duration {
			return defaultGrpcBackoffTime
		}),
		grpc_retry.WithAlwaysRetry(),
		grpc_retry.WithMax(defaultMaxGrpcRetries),
		grpc_retry.WithLogger(debugLogger),
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

func getClientConnectionForTests(ctx context.Context, cfg *peerswaplnd.LndConfig) (*grpc.ClientConn, error) {
	// testGrpcBackoffTime is used for the test
	testGrpcBackoffTime := 500 * time.Millisecond
	testMaxGrpcRetries := 1000

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

	debugLogger := internal_log.New(log.NewDebugLogger(), "[grpc_conn]: ", 0)
	retryOptions := []grpc_retry.CallOption{
		grpc_retry.WithBackoff(func(_ uint) time.Duration {
			return testGrpcBackoffTime
		}),
		grpc_retry.WithAlwaysRetry(),
		grpc_retry.WithMax(uint(testMaxGrpcRetries)),
		grpc_retry.WithLogger(debugLogger),
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
