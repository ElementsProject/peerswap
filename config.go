package peerswap

// Config contains relevant config params for peerswap
type Config struct {
	DbPath      string
	RpcUser     string
	RpcPassword string
	RpcHost     string
	RpcPort     uint
	RpcWallet   string
	Network     string
}
