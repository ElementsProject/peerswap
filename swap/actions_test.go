package swap

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckPremiumAmount_Execute(t *testing.T) {
	t.Parallel()
	next := &NoOpAction{}
	v := CheckPremiumAmount{
		next: next,
	}
	tests := map[string]struct {
		swap *SwapData
		want EventType
	}{
		"swap in premium amount is within limit": {
			swap: &SwapData{
				SwapInAgreement: &SwapInAgreementMessage{
					Premium: 100,
				},
				SwapInRequest: &SwapInRequestMessage{
					PremiumLimit: 200,
				},
			},
			want: NoOp,
		},
		"swap in premium amount exceeds limit": {
			swap: &SwapData{
				SwapInAgreement: &SwapInAgreementMessage{
					Premium: 300,
				},
				SwapInRequest: &SwapInRequestMessage{
					PremiumLimit: 200,
				},
			},
			want: Event_ActionFailed,
		},
		"swap out premium amount is within limit": {
			swap: &SwapData{
				SwapOutAgreement: &SwapOutAgreementMessage{
					Premium: 100,
				},
				SwapOutRequest: &SwapOutRequestMessage{
					PremiumLimit: 200,
				},
			},
			want: NoOp,
		},
		"swap out premium amount exceeds limit": {
			swap: &SwapData{
				SwapOutAgreement: &SwapOutAgreementMessage{
					Premium: 300,
				},
				SwapOutRequest: &SwapOutRequestMessage{
					PremiumLimit: 200,
				},
			},
			want: Event_ActionFailed,
		},
	}

	for name, tt := range tests {
		tt := tt
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := v.Execute(nil, tt.swap)
			assert.Equal(t, tt.want, got, "Event type should match")
		})
	}
}
