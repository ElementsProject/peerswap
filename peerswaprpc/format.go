package peerswaprpc

import (
	"github.com/elementsproject/peerswap/policy"
	"google.golang.org/protobuf/encoding/protojson"
)

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

func (p *Policy) MarshalJSON() ([]byte, error) {
	return protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "",
		AllowPartial:    false,
		UseProtoNames:   true,
		UseEnumNumbers:  false,
		EmitUnpopulated: true,
		Resolver:        nil,
	}.Marshal(p)
}
