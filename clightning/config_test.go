package clightning

import (
	"io/fs"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseConfigFile(t *testing.T) {
	conf := `
	dbpath="dbpath"
	policypath="policypath"

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
	enabled=true
	`

	dir := t.TempDir()
	fp := filepath.Join(dir, "peerswap.conf")
	ioutil.WriteFile(fp, []byte(conf), fs.ModePerm)

	actual, err := parseConfigFromFile(fp)
	if err != nil {
		t.Fatalf("ERROR: %v", err)
	}

	expected := &PeerswapClightningConfig{
		DbPath:                 "dbpath",
		LiquidRpcHost:          "rpchost",
		LiquidRpcPort:          1234,
		LiquidRpcUser:          "rpcuser",
		LiquidRpcPassword:      "rpcpassword",
		LiquidRpcPasswordFile:  "rpcpasswordfile",
		LiquidRpcWallet:        "rpcwallet",
		LiquidEnabled:          true,
		BitcoinRpcHost:         "rpchost",
		BitcoinRpcPort:         1234,
		BitcoinRpcUser:         "rpcuser",
		BitcoinRpcPassword:     "rpcpassword",
		BitcoinRpcPasswordFile: "rpcpasswordfile",
		BitcoinCookieFilePath:  "cookiefilepath",
		PolicyPath:             "policypath",
	}

	assert.EqualValues(t, expected, actual)
}
