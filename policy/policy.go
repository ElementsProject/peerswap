package policy

type SimplePolicy struct {
	premium uint64
}

// todo correct fee
func (s *SimplePolicy) ShouldPayFee(swapAmount, feeAmount uint64, peerId, channelId string) bool {
	return true
}

func (s *SimplePolicy) GetMakerFee(swapValue uint64, swapFee uint64) (uint64, error) {
	return swapFee + s.premium, nil
}
