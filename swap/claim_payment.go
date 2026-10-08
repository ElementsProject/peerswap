package swap

import "errors"

// These errors are the only recovery outcomes that establish that no outgoing
// claim HTLC can still settle. RPC errors and unknown payment states must never
// authorize handing the taker's cooperative-close key to the peer.
var (
	ErrClaimPaymentNotFound = errors.New("claim payment was not found")
	ErrClaimPaymentFailed   = errors.New("claim payment already failed")
)
