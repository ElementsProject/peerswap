package testframework

type LightningNode interface {
	Address() (addr string)
	Id() (id string)

	// GetBtcBalanceSat returns the total amount of sats on the nodes
	// wallet.
	GetBtcBalanceSat() (sats uint64, err error)
	// GetChannelBalanceSat returns the confirmed balance of a channel.
	// scid is given clightning style i.e `100x0x1`.
	GetChannelBalanceSat(scid string) (sats uint64, err error)
	// GetScid returns the short channel id with a peer in clightning style
	// i.e. `100x0x1`.
	GetScid(peer LightningNode) (scid string, err error)

	Connect(peer LightningNode, waitForConnection bool) error
	FundWallet(sats uint64, mineBlock bool) (addr string, err error)
	OpenChannel(peer LightningNode, capacity uint64, connect, confirm, waitForChannelActive bool) (scid string, err error)

	IsBlockHeightSynced() (bool, error)
	IsChannelActive(scid string) (bool, error)
	IsConnected(peer LightningNode) (bool, error)

	AddInvoice(amtSat uint64, desc, label string) (payreq string, err error)
	PayInvoice(payreq string) error
	SendPay(bolt11, scid string) error

	// GetLatestInvoice returns the latest invoice from the stack of created
	// invoices.
	GetLatestInvoice() (payreq string, err error)
	GetMemoFromPayreq(payreq string) (memo string, err error)
}
