package elements

import (
	"strings"
	"time"

	"github.com/elementsproject/glightning/gelements"
	"github.com/elementsproject/peerswap/log"
)

type ElementsClientBuilder struct {
}

func NewClient(rpcUser, rpcPassword, rpcHost string, rpcPort uint) (*gelements.Elements, error) {
	c := gelements.NewElements(rpcUser, rpcPassword)

	var backoff int64 = 1
	for {
		err := c.StartUp(rpcHost, rpcPort)
		if err != nil {
			log.Infof("Could not connect to elements: %s", err.Error())
			// Check if error starts with -28 indicating, that we can not connect
			// to elementsd as it is starting up and not ready yet.
			if strings.HasPrefix(strings.TrimSpace(err.Error()), "-28") {
				// wait a bit and try again.
				time.Sleep(time.Duration(backoff*10) * time.Second)
				backoff *= 2
				continue
			}
			// Other errors fail.
			return nil, err
		}
		break
	}

	backoff = 1
	for {
		info, err := c.GetChainInfo()
		if err != nil {
			return nil, err
		}
		if info.VerificationProgress < 1. {
			// Waiting for block verification to catch up.
			log.Infof("Elementsd still syncing, progress: %d", info.VerificationProgress)
			time.Sleep(time.Duration(backoff*10) * time.Second)
			backoff *= 2
			continue
		}
		return c, nil
	}
}
