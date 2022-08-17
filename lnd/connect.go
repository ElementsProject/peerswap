package lnd

import (
	"context"
	"io/ioutil"

	"github.com/elementsproject/peerswap/cmd/peerswaplnd"
	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"github.com/lightningnetwork/lnd/macaroons"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"
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
