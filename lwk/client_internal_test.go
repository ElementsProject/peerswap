package lwk

import (
	"encoding/json"
	"testing"

	"github.com/elementsproject/glightning/jrpc2"
)

func TestSupportedRPCVersion(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{version: "0.18.0", want: true},
		{version: "0.18.1", want: true},
		{version: "0.8.0", want: false},
		{version: "0.19.0", want: false},
		{version: "invalid", want: false},
	}

	for _, test := range tests {
		t.Run(test.version, func(t *testing.T) {
			wallet := &LWKRpcWallet{lwkVersion: test.version}
			if got := wallet.IsSupportedVersion(); got != test.want {
				t.Fatalf("IsSupportedVersion() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestSendRequestOmitsRemovedFields(t *testing.T) {
	feeRate := 100.0
	payload, err := json.Marshal(&jrpc2.Request{
		Method: &sendRequest{
			Addressees: []*unvalidatedAddressee{
				{
					Address: "ert1qexample",
					Satoshi: 1000,
				},
			},
			FeeRate:    &feeRate,
			WalletName: "peerswap",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var request struct {
		Params map[string]json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(payload, &request); err != nil {
		t.Fatal(err)
	}
	if _, ok := request.Params["enable_ct_discount"]; ok {
		t.Fatal("send request includes removed enable_ct_discount field")
	}
	for _, field := range []string{"addressees", "fee_rate", "name"} {
		if _, ok := request.Params[field]; !ok {
			t.Errorf("send request is missing %q", field)
		}
	}
}
