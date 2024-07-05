package testframework

import (
	"fmt"
	"net/url"
)

type LWK struct {
	Process *DaemonProcess
	RPCURL  *url.URL
}

func NewLWK(testDir string, id int, electrs *Electrs) (*LWK, error) {
	rpcPort, err := GetFreePort()
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", rpcPort))
	if err != nil {
		return nil, err
	}
	cmdLine := []string{
		"lwk_cli",
		"--network=regtest",
		fmt.Sprintf("--addr=%s", u.Host),
		"server",
		"start",
		fmt.Sprintf("--electrum-url=%s", electrs.RPCURL.Host),
		fmt.Sprintf("--datadir=%s", testDir),
	}
	return &LWK{
		Process: NewDaemonProcess(cmdLine, fmt.Sprintf("lwk-%d", id)),
		RPCURL:  u,
	}, nil
}
