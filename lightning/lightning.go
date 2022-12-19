package lightning

import "strings"

// Invoice defines the neccessary parts for decodepayreq
type Invoice struct {
	PHash       string
	Amount      uint64
	Description string
}

type Scid string

// ClnStyle returns the `short_channel_id` divided by 'x'
func (s Scid) ClnStyle() string {
	return strings.ReplaceAll(string(s), ":", "x")
}

// LndStyle returns the `short_channel_id` divided by ':'
func (s Scid) LndStyle() string {
	return strings.ReplaceAll(string(s), "x", ":")
}
