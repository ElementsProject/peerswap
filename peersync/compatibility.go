package peersync

// HasCompatiblePeer reports whether peersync knows a peer with the given ID
// whose advertised capability matches the local protocol version.
func (ps *PeerSync) HasCompatiblePeer(peerID string) bool {
	if ps == nil || ps.store == nil {
		return false
	}

	id, err := NewPeerID(peerID)
	if err != nil {
		return false
	}

	peer, err := ps.store.GetPeerState(id)
	if err != nil || peer == nil {
		return false
	}

	return peer.IsCompatibleWith(ps.version)
}

// CompatiblePeers returns all peers with a capability compatible with the local
// protocol version, keyed by their stringified IDs.
func (ps *PeerSync) CompatiblePeers() (map[string]*Peer, error) {
	result := make(map[string]*Peer)
	if ps == nil || ps.store == nil {
		return result, nil
	}

	peers, err := ps.store.GetAllPeerStates()
	if err != nil {
		return nil, err
	}

	for _, peer := range peers {
		if peer != nil && peer.IsCompatibleWith(ps.version) {
			result[peer.ID().String()] = peer
		}
	}

	return result, nil
}
