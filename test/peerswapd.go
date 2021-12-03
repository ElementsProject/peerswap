package test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/sputn1ck/peerswap/peerswaprpc"
	"github.com/sputn1ck/peerswap/testframework"
	"google.golang.org/grpc"
)

type PeerSwapd struct {
	*testframework.DaemonProcess

	PeerswapClient peerswaprpc.PeerSwapClient
	clientConn     *grpc.ClientConn

	RpcPort int
	DataDir string
}

type LndConfig struct {
	LndHost      string
	TlsPath      string
	MacaroonPath string
}

func NewPeerSwapd(testDir string, lndConfig *LndConfig, extraConfig map[string]string, id int) (*PeerSwapd, error) {
	rpcPort, err := testframework.GetFreePort()
	if err != nil {
		return nil, fmt.Errorf("getFreePort() %w", err)
	}

	rngDirExtension, err := testframework.GenerateRandomString(5)
	if err != nil {
		return nil, fmt.Errorf("generateRandomString(5) %w", err)
	}

	dataDir := filepath.Join(testDir, fmt.Sprintf("peerswap-%s", rngDirExtension))

	err = os.MkdirAll(dataDir, os.ModeDir|os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("os.MkdirAll() %w", err)
	}

	peerswapConfig := map[string]string{
		"network":          "regtest",
		"lnd.tlscertpath":  lndConfig.TlsPath,
		"lnd.macaroonpath": lndConfig.MacaroonPath,
		"lnd.host":         lndConfig.LndHost,
		"accept_all_peers": "true",
		"datadir":          dataDir,
		"host":             fmt.Sprintf("localhost:%v", rpcPort),
	}

	for k, v := range extraConfig {
		peerswapConfig[k] = v
	}

	configFile := filepath.Join(dataDir, "peerswap.conf")

	testframework.WriteConfig(configFile, peerswapConfig, nil, "")

	// Get PeerSwapd plugin path and test dir
	_, filename, _, _ := runtime.Caller(0)
	pathToPlugin := filepath.Join(filename, "..", "..", "peerswapd")
	cmdLine := []string{
		pathToPlugin,
		fmt.Sprintf("--configfile=%s", configFile),
	}

	return &PeerSwapd{
		DaemonProcess: testframework.NewDaemonProcess(cmdLine, fmt.Sprintf("peerswapd-%v", id)),
		DataDir:       dataDir,
		RpcPort:       rpcPort,
	}, nil
}

func (p *PeerSwapd) Run() error {
	p.DaemonProcess.Run()

	err := p.WaitForLog("Listening on", testframework.TIMEOUT)
	if err != nil {
		return err
	}

	psClient, clientConn, err := getPeerswapClient(p.RpcPort)
	if err != nil {
		return err
	}
	p.clientConn = clientConn
	p.PeerswapClient = psClient

	return nil
}

func (p *PeerSwapd) Kill() {
	if p.PeerswapClient != nil {
		_, _ = p.PeerswapClient.Stop(context.Background(), &peerswaprpc.Empty{})
	}
	if p.clientConn != nil {
		p.clientConn.Close()
	}

	p.DaemonProcess.Kill()
}

func getPeerswapClient(rpcPort int) (peerswaprpc.PeerSwapClient, *grpc.ClientConn, error) {
	conn, err := getClientConn(fmt.Sprintf("localhost:%v", rpcPort))
	if err != nil {
		return nil, nil, err
	}

	psClient := peerswaprpc.NewPeerSwapClient(conn)

	return psClient, conn, nil
}

func getClientConn(address string) (*grpc.ClientConn, error) {

	maxMsgRecvSize := grpc.MaxCallRecvMsgSize(1 * 1024 * 1024 * 200)
	opts := []grpc.DialOption{
		grpc.WithDefaultCallOptions(maxMsgRecvSize),
		grpc.WithInsecure(),
	}

	conn, err := grpc.Dial(address, opts...)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to RPC server: %v",
			err)
	}

	return conn, nil
}
