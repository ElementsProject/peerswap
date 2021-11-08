package poll

import "github.com/sputn1ck/peerswap/messages"

type PollMessage struct {
	Version     uint64   `json:"version"`
	Assets      []string `json:"assets"`
	PeerAllowed bool     `json:"peer_allowed"`
}

func (PollMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_POLL
}

type RequestPollMessage struct {
	Version     uint64   `json:"version"`
	Assets      []string `json:"assets"`
	PeerAllowed bool     `json:"peer_allowed"`
}

func (RequestPollMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_REQUEST_POLL
}
