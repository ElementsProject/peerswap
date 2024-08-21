package lwk

import (
	"errors"
	"testing"
)

func TestParseRPCError(t *testing.T) {
	t.Parallel()
	testCases := map[string]struct {
		err          error
		expectedCode int
		expectedMsg  string
		wantErr      bool
	}{
		"Valid RPC error": {
			err:          errors.New("sendrawtransaction RPC error: {\"code\":-26,\"message\":\"min relay fee not met\"}"),
			expectedCode: -26,
			expectedMsg:  "min relay fee not met",
			wantErr:      false,
		},
		"Invalid JSON payload": {

			err:          errors.New("RPC error: {invalid json}"),
			expectedCode: 0,
			expectedMsg:  "",
			wantErr:      true,
		},
		"No RPC error pattern": {
			err:          errors.New("Some other error"),
			expectedCode: 0,
			expectedMsg:  "",
			wantErr:      true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rpcErr, err := parseRPCError(tc.err)
			if (err != nil) != tc.wantErr {
				t.Errorf("wantErr: %v, got error: %v", tc.wantErr, err)
			}
			if err == nil {
				if rpcErr.Code != tc.expectedCode {
					t.Errorf("expected code: %d, got: %d", tc.expectedCode, rpcErr.Code)
				}
				if rpcErr.Message != tc.expectedMsg {
					t.Errorf("expected message: %s, got: %s", tc.expectedMsg, rpcErr.Message)
				}
			}
		})
	}
}
