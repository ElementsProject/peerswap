package peerswap

import "github.com/vulpemventures/go-elements/network"

// Config contains relevant config params for peerswap
type Config struct {
	DbPath                string
	LiquidRpcUser         string
	LiquidRpcPassword     string
	LiquidRpcPasswordFile string
	LiquidRpcHost         string
	LiquidRpcPort         uint
	LiquidRpcWallet       string
	LiquidNetworkString   string
	PolicyPath            string

	LiquidEnabled bool
	LiquidNetwork *network.Network
}
