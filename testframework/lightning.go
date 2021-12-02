package testframework

type LightningNode interface {
	Address() (addr string)
	Id() (id string)

	GetBtcBalanceSat() (sats uint64, err error)
	GetScid(peer LightningNode) (scid string, err error)

	Connect(peer LightningNode, waitForConnection bool) error
	FundWallet(sats uint64, mineBlock bool) (addr string, err error)
	OpenChannel(peer LightningNode, capcity uint64, connect, confirm, waitForChannelActive bool) (scid string, err error)

	IsBlockHeightSynced() (bool, error)
	IsChannelActive(scid string) (bool, error)
	IsConnected(peer LightningNode) (bool, error)
}
