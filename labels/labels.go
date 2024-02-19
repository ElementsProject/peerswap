package labels

import "fmt"

const (
	// peerswapLabelPattern is the pattern that peerswap uses to label on-chain transactions.
	peerswapLabelPattern = "peerswap -- %s(swap id=%s)"
	// opening is the label used for the opening transaction.
	opening = "Opening"
	// claimByInvoice is the label used for the claim by invoice transaction.
	claimByInvoice = "ClaimByInvoice"
	// claimByCoop is the label used for the claim by cooperative close transaction.
	claimByCoop = "ClaimByCoop"
	// ClaimByCsv is the label used for the claim by CSV transaction.
	claimByCsv = "ClaimByCsv"
)

// Opening returns the label used for the opening transaction.
func Opening(swapID string) string {
	return fmt.Sprintf(peerswapLabelPattern, opening, swapID)
}

// ClaimByInvoice returns the label used for the claim by invoice transaction.
func ClaimByInvoice(swapID string) string {
	return fmt.Sprintf(peerswapLabelPattern, claimByInvoice, swapID)
}

// ClaimByCoop returns the label used for the claim by cooperative close transaction.
func ClaimByCoop(swapID string) string {
	return fmt.Sprintf(peerswapLabelPattern, claimByCoop, swapID)
}

// ClaimByCsv returns the label used for the claim by CSV transaction.
func ClaimByCsv(swapID string) string {
	return fmt.Sprintf(peerswapLabelPattern, claimByCsv, swapID)
}
