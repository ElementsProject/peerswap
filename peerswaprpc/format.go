package peerswaprpc

import "github.com/elementsproject/peerswap/policy"

func GetPolicyMessage(p policy.Policy) *Policy {
	return &Policy{
		ReserveOnchainMsat: p.ReserveOnchainMsat,
		MinSwapAmountMsat:  p.MinSwapAmountMsat,
		AcceptAllPeers:     p.AcceptAllPeers,
		AllowNewSwaps:      p.AllowNewSwaps,
		AllowlistedPeers:   p.PeerAllowlist,
		SuspiciousPeerList: p.SuspiciousPeerList,
	}
}
