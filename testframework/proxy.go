package testframework

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/elementsproject/glightning/glightning"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	"github.com/lightningnetwork/lnd/macaroons"
	"github.com/ybbus/jsonrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"

	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
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

type RpcProxy struct {
	rpcHost    string
	rpcPort    int
	configFile string
	serviceURL *url.URL
	authHeader []byte

	Rpc jsonrpc.RPCClient
}

func NewRpcProxy(configFile string) (*RpcProxy, error) {

	conf, err := ReadConfig(configFile)
	if err != nil {
		return nil, fmt.Errorf("ReadConfig() %w", err)
	}

	var rpcPort int
	if port, ok := conf["rpcport"]; ok {
		portInt, err := strconv.Atoi(port)
		if err != nil {
			return nil, fmt.Errorf("could not convert string to int %w", err)
		}
		rpcPort = portInt
	} else {
		return nil, fmt.Errorf("rpcport not found in config %s", configFile)
	}

	rpcHost := "localhost"
	if host, ok := conf["rpchost"]; ok {
		rpcHost = host
	}

	serviceRawURL := fmt.Sprintf("%s://%s:%d", "http", rpcHost, rpcPort)
	serviceURL, err := url.Parse(serviceRawURL)
	if err != nil {
		return nil, fmt.Errorf("url.Parse() %w", err)
	}

	var auth string
	if pass, ok := conf["rpcpassword"]; ok {
		if user, ok := conf["rpcuser"]; ok {
			auth = fmt.Sprintf("%s:%s", user, pass)
		} else {
			return nil, fmt.Errorf("rpcuser not found in config %s", configFile)
		}
	} else {
		// Assume cookie file.
		cookiePath := filepath.Join(filepath.Dir(configFile), "liquidregtest", ".cookie")
		authByte, err := os.ReadFile(cookiePath)
		auth = string(authByte)
		if err != nil {
			return nil, fmt.Errorf("can not read .cookie file at %s", cookiePath)
		}
	}
	if auth == "" {
		return nil, fmt.Errorf("no .cookie file found and no rpcpasssword found in config file %s", configFile)
	}

	auth64 := base64.RawURLEncoding.EncodeToString([]byte(auth))
	authHeader := append([]byte("Basic "), []byte(auth64)...)

	rpcClient := jsonrpc.NewClientWithOpts(serviceURL.String(), &jsonrpc.RPCClientOpts{
		CustomHeaders: map[string]string{
			"Authorization": string(authHeader),
		},
	})

	return &RpcProxy{
		rpcHost:    rpcHost,
		rpcPort:    rpcPort,
		configFile: configFile,
		serviceURL: serviceURL,
		authHeader: authHeader,
		Rpc:        rpcClient,
	}, nil
}

func (p *RpcProxy) Call(method string, parameters ...any) (*jsonrpc.RPCResponse, error) {
	return p.Rpc.Call(method, parameters...)
}

func (p *RpcProxy) UpdateServiceUrl(url string) {
	p.Rpc = jsonrpc.NewClientWithOpts(url, &jsonrpc.RPCClientOpts{
		CustomHeaders: map[string]string{
			"Authorization": string(p.authHeader),
		},
	})
}

type CLightningProxy struct {
	Rpc            *glightning.Lightning
	socketFileName string
	dataDir        string
}

func NewCLightningProxy(socketFileName, dataDir string) (*CLightningProxy, error) {
	lcli := glightning.NewLightning()
	lcli.SetTimeout(uint(TIMEOUT.Seconds()))

	return &CLightningProxy{
		Rpc:            lcli,
		socketFileName: socketFileName,
		dataDir:        dataDir,
	}, nil
}

func (p *CLightningProxy) StartProxy() error {
	return p.Rpc.StartUp(p.socketFileName, p.dataDir)
}

type LndRpcClient struct {
	Rpc   lnrpc.LightningClient
	RpcV2 routerrpc.RouterClient
	conn  *grpc.ClientConn
}

func NewLndRpcClient(host, certPath, macaroonPath string, options ...grpc.DialOption) (*LndRpcClient, error) {
	creds, err := credentials.NewClientTLSFromFile(certPath, "")
	if err != nil {
		return nil, fmt.Errorf("NewClientTLSFromFile() %w", err)
	}

	macBytes, err := os.ReadFile(macaroonPath)
	if err != nil {
		return nil, fmt.Errorf("ReadFile() %w", err)
	}

	mac := &macaroon.Macaroon{}
	if err := mac.UnmarshalBinary(macBytes); err != nil {
		return nil, fmt.Errorf("UnmarshalBinary() %w", err)
	}

	cred, err := macaroons.NewMacaroonCredential(mac)
	if err != nil {
		return nil, fmt.Errorf("NewMacaroonCredential() %w", err)
	}

	maxMsgRecvSize := grpc.MaxCallRecvMsgSize(1 * 1024 * 1024 * 500)
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithPerRPCCredentials(cred),
		grpc.WithDefaultCallOptions(maxMsgRecvSize),
		grpc.WithBlock(),
	}
	opts = append(opts, options...)

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

	interceptorOpts := []grpc.DialOption{
		grpc.WithStreamInterceptor(grpc_retry.StreamClientInterceptor(
			retryOptions...,
		)),
		grpc.WithUnaryInterceptor(grpc_retry.UnaryClientInterceptor(
			retryOptions...,
		)),
	}
	opts = append(opts, interceptorOpts...)

	ctx, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()

	conn, err := grpc.DialContext(ctx, host, opts...)
	if err != nil {
		return nil, fmt.Errorf("NewMacaroonCredential() %w", err)
	}

	lnRpc := lnrpc.NewLightningClient(conn)
	routerRpc := routerrpc.NewRouterClient(conn)
	return &LndRpcClient{
		Rpc:   lnRpc,
		RpcV2: routerRpc,
		conn:  conn,
	}, nil
}
