package swap

type BasicPolicy struct {

}

// return basic tx fee
func (b *BasicPolicy) GetMakerFee(swapValue uint64, swapFee uint64) (uint64, error) {
	return swapFee, nil
}

// always agree to swap in
func (b *BasicPolicy) GetSwapInAgreement(swapValue uint64) (bool, error) {
	return true, nil
}

// always pay fee
func (b *BasicPolicy) CheckSwapOutFee(fee uint64, chanId string) (bool, error) {
	return true,nil
}

