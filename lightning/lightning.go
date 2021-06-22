package lightning



type Invoice struct {
	PHash       string
	Amount      uint64
	Description string
}

type Swapper interface {
	StartSwapOut(peerNodeId string, channelId string, amount uint64) error
}
