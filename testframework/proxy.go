package testframework

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"strconv"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/macaroons"
	"github.com/sputn1ck/glightning/glightning"
	"github.com/ybbus/jsonrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"
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

	var rpcPass string
	if pass, ok := conf["rpcpassword"]; ok {
		rpcPass = pass
	} else {
		return nil, fmt.Errorf("rpcpassword not found in config %s", configFile)
	}

	var rpcUser string
	if user, ok := conf["rpcuser"]; ok {
		rpcUser = user
	} else {
		return nil, fmt.Errorf("rpcuser not found in config %s", configFile)
	}

	authPair := fmt.Sprintf("%s:%s", rpcUser, rpcPass)
	authPairb64 := base64.RawURLEncoding.EncodeToString([]byte(authPair))
	authHeader := []byte("Basic ")
	authHeader = append(authHeader, []byte(authPairb64)...)

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

func (p *RpcProxy) Call(method string, parameters ...interface{}) (*jsonrpc.RPCResponse, error) {
	log.Println(p.Rpc, method)
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
	Rpc  lnrpc.LightningClient
	conn *grpc.ClientConn
}

func NewLndRpcClient(host, certPath, macaroonPath string, options ...grpc.DialOption) (*LndRpcClient, error) {
	creds, err := credentials.NewClientTLSFromFile(certPath, "")
	if err != nil {
		return nil, fmt.Errorf("NewClientTLSFromFile() %w", err)
	}

	macBytes, err := ioutil.ReadFile(macaroonPath)
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

	ctx, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()

	conn, err := grpc.DialContext(ctx, host, opts...)
	if err != nil {
		return nil, fmt.Errorf("NewMacaroonCredential() %w", err)
	}

	lncli := lnrpc.NewLightningClient(conn)
	return &LndRpcClient{
		Rpc:  lncli,
		conn: conn,
	}, nil
}
