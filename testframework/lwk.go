package testframework

import (
	"fmt"
	"net/url"
)

type LWK struct {
	Process *DaemonProcess
}

func NewLWK(testDir string, id int, electrs *Electrs) (*Electrs, error) {
	rpcPort, err := GetFreePort()
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(fmt.Sprintf("127.0.0.1:%d", rpcPort))
	if err != nil {
		return nil, err
	}
	cmdLine := []string{
		"lwk_cli",
		"--network=regtest",
		fmt.Sprintf("--addr=%s", u.String()),
		"server",
		"start",
		fmt.Sprintf("--electrum-url=%s", electrs.rpcURL.String()),
	}
	return &Electrs{
		Process: NewDaemonProcess(cmdLine, fmt.Sprintf("lwk-%d", id)),
	}, nil
}
