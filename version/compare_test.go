package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompareVersionStrings(t *testing.T) {
	tests := []struct {
		description    string
		a              string
		b              string
		expectedReturn bool
		expectedErr    error
	}{
		{
			description:    "a equals b",
			a:              "1.1.1",
			b:              "1.1.1",
			expectedReturn: true,
			expectedErr:    nil,
		},
		{
			description:    "a lower b",
			a:              "1.0.1",
			b:              "1.1.1",
			expectedReturn: false,
			expectedErr:    nil,
		},
		{
			description:    "a higher b",
			a:              "1.2.1",
			b:              "1.1.1",
			expectedReturn: true,
			expectedErr:    nil,
		},
		{
			description:    "a shorter b",
			a:              "2.1",
			b:              "1.1.1",
			expectedReturn: true,
			expectedErr:    nil,
		},
		{
			description:    "a longer b",
			a:              "1.1.1.0",
			b:              "1.1.1",
			expectedReturn: true,
			expectedErr:    nil,
		},
		{
			description:    "with chars",
			a:              "v1.1.1",
			b:              "1.1.1-beta",
			expectedReturn: true,
			expectedErr:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			t.Parallel()
			actualReturn, actualErr := CompareVersionStrings(tt.a, tt.b)
			assert.EqualValues(t, tt.expectedReturn, actualReturn)
			assert.ErrorIs(t, actualErr, tt.expectedErr)
		})
	}
}
