package peersync

import (
	"encoding/json"
	"sort"
)

const (
	channelAdjacencySchemaVersion = 1
	defaultChannelAdjacencyLimit  = 20
)

// ChannelAdjacency is an optional, advisory advertisement embedded in peersync
// poll messages.
//
// Domain definition:
//   - It represents a (possibly truncated) adjacency list of the sender's LN
//     channels to immediate neighbors.
//   - It is intended only as a *hint* for 2-hop discovery (u→m→v). It must not
//     be trusted: data may be stale or incorrect, and swaps still use explicit
//     2-hop discovery during negotiation.
//   - By default we only advertise public + active channels and we do not
//     include balance information for privacy.
type ChannelAdjacency struct {
	// SchemaVersion is the version of this nested schema. This is not the
	// PeerSwap protocol version from the outer poll message.
	SchemaVersion int `json:"schema_version,omitempty"`

	// PublicChannelsOnly indicates that only public channels were included.
	// Public channels are those that are announced in the public LN graph.
	PublicChannelsOnly bool `json:"public_channels_only,omitempty"`

	// MaxNeighbors is the maximum number of neighbor nodes included in Neighbors.
	// If the sender has more neighbors, Neighbors will be truncated.
	MaxNeighbors int `json:"max_neighbors,omitempty"`

	// Truncated indicates whether Neighbors was truncated due to MaxNeighbors.
	Truncated bool `json:"truncated,omitempty"`

	// Neighbors is the set of adjacent neighbor nodes (remote node pubkeys).
	Neighbors []ChannelAdjacencyNeighbor `json:"neighbors,omitempty"`
}

// ChannelAdjacencyNeighbor represents a single adjacent node and the channels
// connecting the sender to that node.
type ChannelAdjacencyNeighbor struct {
	// NodeID is the neighbor node's pubkey as a 33-byte compressed secp256k1
	// public key, hex-encoded (66 hex characters).
	NodeID string `json:"node_id,omitempty"`

	// Channels describes one or more channels between the sender and NodeID.
	Channels []ChannelAdjacencyChannel `json:"channels,omitempty"`
}

// ChannelAdjacencyChannel describes a single channel edge from the sender to
// a neighbor.
type ChannelAdjacencyChannel struct {
	// ChannelID is an implementation-specific numeric channel id, if available.
	// For LND, this corresponds to lnrpc.Channel.chan_id (a packed short_channel_id).
	// For backends that do not expose a stable numeric id, this may be 0.
	ChannelID uint64 `json:"channel_id,omitempty"`

	// ShortChannelID is the BOLT-07 short_channel_id formatted as "blockxtransactionxoutput",
	// for example "1x2x3".
	ShortChannelID string `json:"short_channel_id,omitempty"`

	// Active indicates whether the channel is currently usable for forwarding
	// (from the sender's local perspective).
	Active bool `json:"active,omitempty"`
}

func buildChannelAdjacency(channels []Channel) *ChannelAdjacency {
	byNode := make(map[string][]ChannelAdjacencyChannel)
	for _, ch := range channels {
		if !ch.Public || !ch.Active {
			continue
		}
		if ch.ShortChannelID == "" {
			continue
		}
		nodeID := ch.Peer.String()
		if nodeID == "" {
			continue
		}
		byNode[nodeID] = append(byNode[nodeID], ChannelAdjacencyChannel{
			ChannelID:      ch.ChannelID,
			ShortChannelID: ch.ShortChannelID,
			Active:         ch.Active,
		})
	}

	if len(byNode) == 0 {
		return nil
	}

	keys := make([]string, 0, len(byNode))
	for k := range byNode {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	maxNeighbors := defaultChannelAdjacencyLimit
	truncated := maxNeighbors > 0 && len(keys) > maxNeighbors
	if truncated {
		keys = keys[:maxNeighbors]
	}

	neighbors := make([]ChannelAdjacencyNeighbor, 0, len(keys))
	for _, nodeID := range keys {
		chans := byNode[nodeID]
		sort.Slice(chans, func(i, j int) bool {
			return chans[i].ShortChannelID < chans[j].ShortChannelID
		})
		neighbors = append(neighbors, ChannelAdjacencyNeighbor{
			NodeID:   nodeID,
			Channels: chans,
		})
	}

	if len(neighbors) == 0 {
		return nil
	}

	return &ChannelAdjacency{
		SchemaVersion:      channelAdjacencySchemaVersion,
		PublicChannelsOnly: true,
		MaxNeighbors:       maxNeighbors,
		Truncated:          truncated,
		Neighbors:          neighbors,
	}
}

func cloneChannelAdjacency(ad *ChannelAdjacency) *ChannelAdjacency {
	if ad == nil {
		return nil
	}

	out := &ChannelAdjacency{
		SchemaVersion:      ad.SchemaVersion,
		PublicChannelsOnly: ad.PublicChannelsOnly,
		MaxNeighbors:       ad.MaxNeighbors,
		Truncated:          ad.Truncated,
	}
	if len(ad.Neighbors) == 0 {
		return out
	}

	out.Neighbors = make([]ChannelAdjacencyNeighbor, len(ad.Neighbors))
	for i, neighbor := range ad.Neighbors {
		out.Neighbors[i].NodeID = neighbor.NodeID
		if len(neighbor.Channels) == 0 {
			continue
		}
		out.Neighbors[i].Channels = make([]ChannelAdjacencyChannel, len(neighbor.Channels))
		copy(out.Neighbors[i].Channels, neighbor.Channels)
	}

	return out
}

// UnmarshalJSON supports legacy field names from earlier iterations of this
// optional extension.
func (a *ChannelAdjacency) UnmarshalJSON(data []byte) error {
	// New schema keys.
	type schemaV1 struct {
		SchemaVersion      int                        `json:"schema_version,omitempty"`
		PublicChannelsOnly bool                       `json:"public_channels_only,omitempty"`
		MaxNeighbors       int                        `json:"max_neighbors,omitempty"`
		Truncated          bool                       `json:"truncated,omitempty"`
		Neighbors          []ChannelAdjacencyNeighbor `json:"neighbors,omitempty"`
	}

	// Legacy schema keys (2hop.md initial draft).
	type legacySchema struct {
		V          int                        `json:"v,omitempty"`
		PublicOnly bool                       `json:"public_only,omitempty"`
		Limit      int                        `json:"limit,omitempty"`
		Entries    []ChannelAdjacencyNeighbor `json:"entries,omitempty"`
	}

	var v1 schemaV1
	if err := json.Unmarshal(data, &v1); err != nil {
		return err
	}

	if v1.SchemaVersion != 0 || v1.MaxNeighbors != 0 || v1.Truncated || v1.PublicChannelsOnly || len(v1.Neighbors) > 0 {
		a.SchemaVersion = v1.SchemaVersion
		a.PublicChannelsOnly = v1.PublicChannelsOnly
		a.MaxNeighbors = v1.MaxNeighbors
		a.Truncated = v1.Truncated
		a.Neighbors = v1.Neighbors
		return nil
	}

	var legacy legacySchema
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}

	a.SchemaVersion = legacy.V
	a.PublicChannelsOnly = legacy.PublicOnly
	a.MaxNeighbors = legacy.Limit
	a.Neighbors = legacy.Entries
	a.Truncated = false
	return nil
}
