package testframework

type LightningNode interface {
	Address() (addr string)
	Id() (id string)

	// GetBtcBalanceSat returns the total amount of sats on the nodes
	// wallet.
	GetBtcBalanceSat() (sats uint64, err error)
	// GetScid returns the short channel id with a peer in clightning style
	// i.e. `100x0x1`.
	GetScid(peer LightningNode) (scid string, err error)

	Connect(peer LightningNode, waitForConnection bool) error
	FundWallet(sats uint64, mineBlock bool) (addr string, err error)
	OpenChannel(peer LightningNode, capacity uint64, connect, confirm, waitForChannelActive bool) (scid string, err error)

	IsBlockHeightSynced() (bool, error)
	IsChannelActive(scid string) (bool, error)
	IsConnected(peer LightningNode) (bool, error)
}
