package elements

import (
	"fmt"
	"strings"
	"time"

	"github.com/elementsproject/glightning/gelements"
	"github.com/elementsproject/peerswap/log"
)

const (
	liquidMainnetChain = "liquidv1"

	minLiquidMainnetElementsVersion           = 230301
	minLiquidMainnetElementsVersionString     = "23.3.1"
	recommendedLiquidMainnetElementsVersion   = "23.3.3"
	unsupportedLiquidMainnetElementsErrorText = "unsupported elementsd version for Liquid mainnet"
)

type ElementsClientBuilder struct {
}

func NewClient(rpcUser, rpcPassword, rpcHost, RpcPasswordFile string, rpcPort uint) (*gelements.Elements, error) {
	c := gelements.NewElements(rpcUser, rpcPassword, RpcPasswordFile)

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
	versionChecked := false
	for {
		info, err := c.GetChainInfo()
		if err != nil {
			return nil, err
		}
		if !versionChecked {
			if err := checkLiquidMainnetElementsVersion(c, info.Chain); err != nil {
				return nil, err
			}
			versionChecked = true
		}
		if info.VerificationProgress < 1. {
			// Waiting for block verification to catch up.
			log.Infof("Elementsd still syncing, progress: %f", info.VerificationProgress)
			time.Sleep(time.Duration(backoff*10) * time.Second)
			backoff *= 2
			continue
		}
		return c, nil
	}
}

func checkLiquidMainnetElementsVersion(c *gelements.Elements, chain string) error {
	if chain != liquidMainnetChain {
		return nil
	}
	info, err := c.GetNetworkInfo()
	if err != nil {
		return err
	}
	return validateLiquidMainnetElementsVersion(chain, info.Version, info.Subversion)
}

func validateLiquidMainnetElementsVersion(chain string, version int, subversion string) error {
	if chain != liquidMainnetChain || version >= minLiquidMainnetElementsVersion {
		return nil
	}

	return fmt.Errorf(
		"%s: Liquid mainnet requires elementsd %s or newer after ELIP 203; "+
			"detected version %s (%s). Upgrade to Elements %s or newer, preferably %s",
		unsupportedLiquidMainnetElementsErrorText,
		minLiquidMainnetElementsVersionString,
		formatElementsVersion(version),
		subversion,
		minLiquidMainnetElementsVersionString,
		recommendedLiquidMainnetElementsVersion,
	)
}

func formatElementsVersion(version int) string {
	return fmt.Sprintf("%d.%d.%d", version/10000, version/100%100, version%100)
}
