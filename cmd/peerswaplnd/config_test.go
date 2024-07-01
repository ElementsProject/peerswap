package peerswaplnd_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/elementsproject/peerswap/cmd/peerswaplnd"
	"github.com/stretchr/testify/assert"
	"github.com/vulpemventures/go-elements/network"
)

func TestLWKFromIniFileConfig(t *testing.T) {
	t.Parallel()
	t.Run("valid ini config", func(t *testing.T) {
		t.Parallel()
		file := `
		lwk.signername=signername
		lwk.walletname=walletname
		lwk.lwkendpoint=http://localhost:32110
		lwk.network=liquid
		lwk.liquidswaps=true
		`

		filePath := filepath.Join(t.TempDir(), "peerswap.conf")
		assert.NoError(t, os.WriteFile(filePath, []byte(file), fs.ModePerm))
		got, err := peerswaplnd.LWKFromIniFileConfig(filePath)
		if err != nil {
			t.Errorf("LWKConfigFromToml() error = %v", err)
			return
		}
		assert.Equal(t, got.GetChain(), &network.Liquid)
		assert.Equal(t, got.GetElectrumEndpoint(), "blockstream.info:995")
		assert.Equal(t, got.GetLWKEndpoint(), "http://localhost:32110")
		assert.Equal(t, got.GetLiquidSwaps(), true)
		assert.Equal(t, got.GetNetwork(), "liquid")
	})
	t.Run("default ini config", func(t *testing.T) {
		t.Parallel()
		file := `
		lwk.network=liquid-testnet
		lwk.liquidswaps=true
		`

		filePath := filepath.Join(t.TempDir(), "peerswap.conf")
		assert.NoError(t, os.WriteFile(filePath, []byte(file), fs.ModePerm))
		got, err := peerswaplnd.LWKFromIniFileConfig(filePath)
		if err != nil {
			t.Errorf("LWKConfigFromToml() error = %v", err)
			return
		}
		assert.Equal(t, got.GetChain(), &network.Testnet)
		assert.Equal(t, got.GetElectrumEndpoint(), "blockstream.info:465")
		assert.Equal(t, got.GetLWKEndpoint(), "http://localhost:32111")
		assert.Equal(t, got.GetLiquidSwaps(), true)
		assert.Equal(t, got.GetNetwork(), "liquid-testnet")
	})
}
