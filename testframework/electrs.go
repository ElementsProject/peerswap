package testframework

import (
	"context"
	"fmt"
	"net/url"

	"github.com/cenkalti/backoff/v4"
	"github.com/checksum0/go-electrum/electrum"
)

type Electrs struct {
	Process *DaemonProcess
	RPCURL  *url.URL
}

func NewElectrs(testDir string, id int, elements *LiquidNode) (*Electrs, error) {
	rpcPort, err := GetFreePort()
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(fmt.Sprintf("tcp://127.0.0.1:%d", rpcPort))
	if err != nil {
		return nil, err
	}
	monitoringPort, err := GetFreePort()
	if err != nil {
		return nil, err
	}
	cmdLine := []string{
		"electrs",
		"-v",
		"--network=liquidregtest",
		fmt.Sprintf("--daemon-rpc-addr=127.0.0.1:%d", elements.RpcPort),
		fmt.Sprintf("--electrum-rpc-addr=%s", u.Host),
		fmt.Sprintf("--cookie=%s", elements.RpcUser+":"+elements.RpcPassword),
		fmt.Sprintf("--daemon-dir=%s", elements.DataDir),
		fmt.Sprintf("--monitoring-addr=127.0.0.1:%d", monitoringPort),
		fmt.Sprintf("--db-dir=%s", testDir),

		"--jsonrpc-import",
	}
	return &Electrs{
		Process: NewDaemonProcess(cmdLine, fmt.Sprintf("electrs-%d", id)),
		RPCURL:  u,
	}, nil
}

func (e *Electrs) Run(ctx context.Context) error {
	e.Process.Run()
	return backoff.Retry(func() error {
		ec, err := electrum.NewClientTCP(ctx, e.RPCURL.Host)
		if err != nil {
			return err
		}
		return ec.Ping(ctx)
	}, backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 5))
}
