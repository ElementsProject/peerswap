package peerswap

import "github.com/vulpemventures/go-elements/network"

// Config contains relevant config params for peerswap
type Config struct {
	DbPath              string
	LiquidRpcUser       string
	LiquidRpcPassword   string
	LiquidRpcHost       string
	LiquidRpcPort       uint
	LiquidRpcWallet     string
	LiquidNetworkString string
	PolicyPath          string

	LiquidEnabled bool
	LiquidNetwork *network.Network
}

// Test defines the network parameters for the liquid test network.
var Testnet = network.Network{
	Name:         "testnet",
	Bech32:       "ert",
	Blech32:      "el",
	HDPublicKey:  [4]byte{0x04, 0x35, 0x87, 0xcf},
	HDPrivateKey: [4]byte{0x04, 0x35, 0x83, 0x94},
	PubKeyHash:   235,
	ScriptHash:   75,
	Wif:          0xef,
	Confidential: 4,
	AssetID:      "5d8629bf58c7f98e90e171a81058ce543418f0dc16e8459367773552b067f3f3",
}
