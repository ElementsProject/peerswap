package test

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/elementsproject/peerswap/peerswaprpc"
	"github.com/elementsproject/peerswap/testframework"
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

func NewPeerSwapd(testDir string, pathToPeerswapPlugin string, lndConfig *LndConfig, extraConfig map[string]string, id int) (*PeerSwapd, error) {
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
		"datadir":          dataDir,
		"host":             fmt.Sprintf("localhost:%v", rpcPort),
		"resthost":         "",
	}

	for k, v := range extraConfig {
		peerswapConfig[k] = v
	}

	configFile := filepath.Join(dataDir, "peerswap.conf")

	testframework.WriteConfig(configFile, peerswapConfig, nil, "")

	policyConfig := map[string]string{
		"accept_all_peers": "true",
	}

	policyFile := filepath.Join(dataDir, "policy.conf")
	testframework.WriteConfig(policyFile, policyConfig, nil, "")

	cmdLine := []string{
		pathToPeerswapPlugin,
		fmt.Sprintf("--configfile=%s", configFile),
		fmt.Sprintf("--policyfile=%s", policyFile),
	}

	return &PeerSwapd{
		DaemonProcess: testframework.NewDaemonProcess(cmdLine, fmt.Sprintf("peerswapd-%v", id)),
		DataDir:       dataDir,
		RpcPort:       rpcPort,
	}, nil
}

func (p *PeerSwapd) Run(waitForReady bool) error {
	p.DaemonProcess.Run()

	if waitForReady {
		err := p.WaitForLog("listening on", testframework.TIMEOUT)
		if err != nil {
			return err
		}
	}

	psClient, clientConn, err := getPeerswapClient(p.RpcPort)
	if err != nil {
		return err
	}
	p.clientConn = clientConn
	p.PeerswapClient = psClient

	return nil
}

func (p *PeerSwapd) Stop() {
	if p.PeerswapClient != nil {
		_, _ = p.PeerswapClient.Stop(context.Background(), &peerswaprpc.Empty{})
	}
	if p.clientConn != nil {
		if err := p.clientConn.Close(); err != nil {
			log.Println("ERROR CLOSE CONNECTION TO PEERSWAPCLIENT", err.Error())
		}
	}
}

func (p *PeerSwapd) Kill() {
	p.Stop()
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
		grpc.WithBlock(),
	}

	conn, err := grpc.Dial(address, opts...)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to RPC server: %v",
			err)
	}

	return conn, nil
}
