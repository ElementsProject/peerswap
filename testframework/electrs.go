package testframework

import (
	"context"
	"fmt"
	"net/url"

	"github.com/checksum0/go-electrum/electrum"
)

type Electrs struct {
	Process *DaemonProcess
	rpcURL  *url.URL
}

func NewElectrs(testDir string, id int, elements *LiquidNode) (*Electrs, error) {
	rpcPort, err := GetFreePort()
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(fmt.Sprintf("127.0.0.1:%d", rpcPort))
	if err != nil {
		return nil, err
	}
	cmdLine := []string{
		"electrs",
		"-v",
		"--network=liquidregtest",
		fmt.Sprintf("daemon-rpc-addr=%s:%d", elements.rpcHost, elements.rpcPort),
		fmt.Sprintf("electrum-rpc-addr=:%s", u.String()),
		fmt.Sprintf("cookie=%s", elements.RpcUser+":"+elements.RpcPassword),
		fmt.Sprintf("daemon-dir=%s", elements.DataDir),
		"- --jsonrpc-import",
	}
	return &Electrs{
		Process: NewDaemonProcess(cmdLine, fmt.Sprintf("electrs-%d", id)),
		rpcURL:  u,
	}, nil
}

func (e *Electrs) Run(ctx context.Context) error {
	e.Process.Run()
	ec, err := electrum.NewClientTCP(ctx, e.rpcURL.String())
	if err != nil {
		return err
	}
	return ec.Ping(ctx)
}
