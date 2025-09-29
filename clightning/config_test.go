package clightning

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ReadFromFile(t *testing.T) {
	conf := `
	[Bitcoin]
	rpcuser="rpcuser"
	rpcpassword="rpcpassword"
	rpcpasswordfile="rpcpasswordfile"
	rpchost="rpchost"
	rpcport=1234
	cookiefilepath="cookiefilepath"

	[Liquid]
	rpcuser="rpcuser"
	rpcpassword="rpcpassword"
	rpcpasswordfile="rpcpasswordfile"
	rpchost="rpchost"
	rpcport=1234
	rpcwallet="rpcwallet"
	`

	dir := t.TempDir()
	fp := filepath.Join(dir, "peerswap.conf")
	_ = os.WriteFile(fp, []byte(conf), fs.ModePerm)

	c := &Config{PeerswapDir: dir, Bitcoin: &BitcoinConf{}, Liquid: &LiquidConf{}}
	actual, err := ReadFromFile()(c)
	if err != nil {
		t.Fatalf("ERROR: %v", err)
	}

	expected := &Config{
		PeerswapDir: dir,
		Bitcoin: &BitcoinConf{
			RpcUser:         "rpcuser",
			RpcPassword:     "rpcpassword",
			RpcPasswordFile: "rpcpasswordfile",
			RpcHost:         "rpchost",
			RpcPort:         1234,
			Network:         "",
			DataDir:         "",
		},
		Liquid: &LiquidConf{
			RpcUser:         "rpcuser",
			RpcPassword:     "rpcpassword",
			RpcPasswordFile: "rpcpasswordfile",
			RpcHost:         "rpchost",
			RpcPort:         1234,
			RpcWallet:       "rpcwallet",
			Network:         "",
			DataDir:         "",
			LiquidSwaps:     nil,
		},
	}

	assert.EqualValues(t, expected, actual)
}

func Test_ReadFromFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "peerswap.conf")
	_ = os.WriteFile(fp, []byte{}, fs.ModePerm)

	c := &Config{PeerswapDir: dir, Bitcoin: &BitcoinConf{}, Liquid: &LiquidConf{}}
	actual, err := ReadFromFile()(c)
	if err != nil {
		t.Fatalf("ERROR: %v", err)
	}

	expected := &Config{
		PeerswapDir: dir,
		Bitcoin:     &BitcoinConf{},
		Liquid:      &LiquidConf{},
	}

	assert.EqualValues(t, expected, actual)
}
