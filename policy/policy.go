package policy

// SimplePolicy allows the user to add a simple premium to an swap
type SimplePolicy struct {
	premium uint64
}

// todo correct fee
// ShouldPayFee decides if a FeeInvoice should be paid
func (s *SimplePolicy) ShouldPayFee(swapAmount, feeAmount uint64, peerId, channelId string) bool {
	return true
}

// GetMakerFee returns the fee for the swap with an added premium
func (s *SimplePolicy) GetMakerFee(swapValue uint64, swapFee uint64) (uint64, error) {
	return swapFee + s.premium, nil
}
