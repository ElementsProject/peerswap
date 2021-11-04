package poll

type PollMessage struct {
	Version     uint64   `json:"version"`
	Assets      []string `json:"assets"`
	PeerAllowed bool     `json:"peer_allowed"`
}
