package fsm

type Messenger interface {
	SendMessage(peerId string, hexMsg string) error
}

type Policy interface {
	ShouldPayFee(feeAmount uint64, peerId, channelId string) bool
}
type LightningClient interface {
	DecodeInvoice(payreq string) (peerId string, amount uint64, err error)
	PayInvoice(payreq string) (preImage string, err error)
}
