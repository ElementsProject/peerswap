package clightning

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/elementsproject/glightning/glightning"
	"github.com/elementsproject/glightning/jrpc2"
)

func TestSwapCommandUsage(t *testing.T) {
	for _, method := range []jrpc2.ServerMethod{&SwapIn{}, &SwapOut{}} {
		t.Run(method.Name(), func(t *testing.T) {
			data, err := json.Marshal(&glightning.RpcMethod{Method: method})
			if err != nil {
				t.Fatal(err)
			}
			var metadata struct {
				Usage string `json:"usage"`
			}
			if err := json.Unmarshal(data, &metadata); err != nil {
				t.Fatal(err)
			}
			const want = "short_channel_id amt_sat asset premium_rate_limit_ppm force"
			if metadata.Usage != want {
				t.Fatalf("usage = %q, want %q", metadata.Usage, want)
			}
		})
	}
}

func TestSwapCommandParams(t *testing.T) {
	for _, premium := range []int64{1250, -1250} {
		for _, want := range []jrpc2.ServerMethod{
			&SwapIn{ShortChannelId: "1x2x3", SatAmt: 100000, Asset: "btc", PremiumLimitRatePPM: premium, Force: true},
			&SwapOut{ShortChannelId: "1x2x3", SatAmt: 100000, Asset: "btc", PremiumLimitRatePPM: premium, Force: true},
		} {
			for _, tc := range []struct {
				name   string
				params string
			}{
				{"named", fmt.Sprintf(`{"short_channel_id":"1x2x3","amt_sat":100000,"asset":"btc","premium_rate_limit_ppm":%d,"force":true}`, premium)},
				{"positional", fmt.Sprintf(`["1x2x3",100000,"btc",%d,true]`, premium)},
			} {
				t.Run(fmt.Sprintf("%s/%s/%d", want.Name(), tc.name, premium), func(t *testing.T) {
					got := want.New().(jrpc2.ServerMethod)
					var params interface{}
					if err := json.Unmarshal([]byte(tc.params), &params); err != nil {
						t.Fatal(err)
					}
					var err error
					switch params := params.(type) {
					case map[string]interface{}:
						err = jrpc2.ParseNamedParams(got, params)
					case []interface{}:
						err = jrpc2.ParseParamArray(got, params)
					default:
						t.Fatalf("unexpected params type %T", params)
					}
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("parsed command = %+v, want %+v", got, want)
					}
				})
			}
		}
	}
}
