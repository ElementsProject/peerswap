package peerswap

import "github.com/vulpemventures/go-elements/network"

// Config contains relevant config params for peerswap
type Config struct {
	DbPath              string
	LiquidRpcUser       string
	LiquidRpcPassword   string
	LiquidRpcPasswordFile string
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
	Bech32:       "tex",
	Blech32:      "tlq",
	HDPublicKey:  [4]byte{0x04, 0x35, 0x87, 0xcf},
	HDPrivateKey: [4]byte{0x04, 0x35, 0x83, 0x94},
	PubKeyHash:   235,
	ScriptHash:   75,
	Wif:          0xef,
	Confidential: 4,
	AssetID:      "144c654344aa716d6f3abcc1ca90e5641e4e2a7f633bc09fe3baf64585819a49",
}
