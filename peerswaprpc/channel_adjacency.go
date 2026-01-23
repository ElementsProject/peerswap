package peerswaprpc

import "github.com/elementsproject/peerswap/peersync"

// ChannelAdjacencyFromPeerState converts the optional channel_adjacency metadata
// stored in peersync.Peer into the RPC representation.
func ChannelAdjacencyFromPeerState(peer *peersync.Peer) *ChannelAdjacency {
	if peer == nil {
		return nil
	}
	return ChannelAdjacencyFromPeerSync(peer.ChannelAdjacency())
}

// ChannelAdjacencyFromPeerSync converts peersync ChannelAdjacency hints into the
// peerswaprpc type used by API and CLI outputs.
func ChannelAdjacencyFromPeerSync(ad *peersync.ChannelAdjacency) *ChannelAdjacency {
	if ad == nil {
		return nil
	}

	out := &ChannelAdjacency{
		SchemaVersion:      int32(ad.SchemaVersion),
		PublicChannelsOnly: ad.PublicChannelsOnly,
		MaxNeighbors:       int32(ad.MaxNeighbors),
		Truncated:          ad.Truncated,
	}

	if len(ad.Neighbors) == 0 {
		return out
	}

	out.Neighbors = make([]*ChannelAdjacencyNeighbor, 0, len(ad.Neighbors))
	for _, neighbor := range ad.Neighbors {
		rpcNeighbor := &ChannelAdjacencyNeighbor{
			NodeId: neighbor.NodeID,
		}

		if len(neighbor.Channels) > 0 {
			rpcNeighbor.Channels = make([]*ChannelAdjacencyChannel, 0, len(neighbor.Channels))
			for _, ch := range neighbor.Channels {
				rpcNeighbor.Channels = append(rpcNeighbor.Channels, &ChannelAdjacencyChannel{
					ChannelId:      ch.ChannelID,
					ShortChannelId: ch.ShortChannelID,
					Active:         ch.Active,
				})
			}
		}

		out.Neighbors = append(out.Neighbors, rpcNeighbor)
	}

	return out
}
