package clightning

// PeerswapClightningConfig contains relevant config params for peerswap
type PeerswapClightningConfig struct {
	DbPath                string
	LiquidRpcUser         string
	LiquidRpcPassword     string
	LiquidRpcPasswordFile string
	LiquidRpcHost         string
	LiquidRpcPort         uint
	LiquidRpcWallet       string
	PolicyPath            string

	LiquidEnabled bool
}
