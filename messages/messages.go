package messages

// Message needs to have a MessageType.
type Message interface {
	MessageType() int64
}
