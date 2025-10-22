package scenario

import (
	"fmt"
	"math"

	"github.com/elementsproject/peerswap/premium"
	"github.com/elementsproject/peerswap/swap"
)

// Expectations captures the numeric inputs required to derive swap assertions.
type Expectations struct {
	SwapAmt            uint64
	SwapType           swap.SwapType
	SwapInPremiumRate  int64
	SwapOutPremiumRate int64

	OrigTakerChannel uint64
	OrigMakerChannel uint64
	OrigTakerWallet  uint64
	OrigMakerWallet  uint64
}

// Premium returns the premium (sat) to apply for the scenario's swap type.
func (e Expectations) Premium() int64 {
	switch e.SwapType {
	case swap.SWAPTYPE_IN:
		return premium.NewPPM(e.SwapInPremiumRate).Compute(e.SwapAmt)
	case swap.SWAPTYPE_OUT:
		return premium.NewPPM(e.SwapOutPremiumRate).Compute(e.SwapAmt)
	default:
		return 0
	}
}

// TakerChannelAfterPreimageClaim returns the expected satoshis on the taker side.
func (e Expectations) TakerChannelAfterPreimageClaim(invoiceFee uint64) float64 {
	balance := safeUint64ToInt64(e.OrigTakerChannel) -
		safeUint64ToInt64(e.SwapAmt) -
		safeUint64ToInt64(invoiceFee)
	if e.SwapType == swap.SWAPTYPE_OUT {
		balance -= e.Premium()
	}
	return float64(balance)
}

// MakerChannelAfterPreimageClaim returns the expected satoshis on the maker side.
func (e Expectations) MakerChannelAfterPreimageClaim(invoiceFee uint64) float64 {
	balance := safeUint64ToInt64(e.OrigMakerChannel) +
		safeUint64ToInt64(e.SwapAmt) +
		safeUint64ToInt64(invoiceFee)
	if e.SwapType == swap.SWAPTYPE_OUT {
		balance += e.Premium()
	}
	return float64(balance)
}

// TakerWalletAfterPreimageClaim returns the expected wallet balance for the taker.
func (e Expectations) TakerWalletAfterPreimageClaim(claimFee uint64) uint64 {
	balance := safeUint64ToInt64(e.OrigTakerWallet) -
		safeUint64ToInt64(claimFee) +
		safeUint64ToInt64(e.SwapAmt)
	if e.SwapType == swap.SWAPTYPE_IN {
		balance += e.Premium()
	}
	return safeInt64ToUint64(balance)
}

// MakerWalletAfterPreimageClaim returns the expected wallet balance for the maker.
func (e Expectations) MakerWalletAfterPreimageClaim(commitFee uint64) uint64 {
	balance := safeUint64ToInt64(e.OrigMakerWallet) -
		safeUint64ToInt64(commitFee) -
		safeUint64ToInt64(e.SwapAmt)
	if e.SwapType == swap.SWAPTYPE_IN {
		balance -= e.Premium()
	}
	return safeInt64ToUint64(balance)
}

// TakerWalletUnchanged reports the taker's wallet when no on-chain movement is expected.
func (e Expectations) TakerWalletUnchanged() uint64 {
	return e.OrigTakerWallet
}

// MakerWalletAfterFees returns the maker wallet after paying commit + claim fees.
func (e Expectations) MakerWalletAfterFees(commitFee, claimFee uint64) uint64 {
	return e.OrigMakerWallet - commitFee - claimFee
}

// MakerChannelAfterCsv returns the maker channel balance once a CSV claim succeeds.
func (e Expectations) MakerChannelAfterCsv(premiumAmt uint64) float64 {
	return float64(e.OrigMakerChannel + premiumAmt)
}

func safeUint64ToInt64(value uint64) int64 {
	if value > math.MaxInt64 {
		panic(fmt.Sprintf("value %d exceeds max int64", value))
	}
	return int64(value)
}

func safeInt64ToUint64(value int64) uint64 {
	if value < 0 {
		panic(fmt.Sprintf("value %d is negative", value))
	}
	return uint64(value)
}
