package swap

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPeerswapMessages_TwoHopJSONIsOptional(t *testing.T) {
	t.Parallel()

	swapID := strings.Repeat("00", 32)
	pubkey := "02" + strings.Repeat("00", 32)
	intermediaryPubkey := "03" + strings.Repeat("11", 32)

	t.Run("swap_in_request without twohop", func(t *testing.T) {
		t.Parallel()

		raw := `{
			"protocol_version": 5,
			"swap_id": "` + swapID + `",
			"asset": "",
			"network": "regtest",
			"scid": "1x2x3",
			"amount": 100,
			"pubkey": "` + pubkey + `",
			"acceptable_premium": 0
		}`

		var msg SwapInRequestMessage
		require.NoError(t, json.Unmarshal([]byte(raw), &msg))
		assert.Nil(t, msg.TwoHop)
		require.NoError(t, msg.Validate(&SwapData{}))
	})

	t.Run("swap_in_request with twohop", func(t *testing.T) {
		t.Parallel()

		raw := `{
			"protocol_version": 5,
			"swap_id": "` + swapID + `",
			"asset": "",
			"network": "regtest",
			"scid": "1x2x3",
			"amount": 100,
			"pubkey": "` + pubkey + `",
			"acceptable_premium": 0,
			"twohop": {
				"intermediary_pubkey": "` + intermediaryPubkey + `"
			}
		}`

		var msg SwapInRequestMessage
		require.NoError(t, json.Unmarshal([]byte(raw), &msg))
		require.NotNil(t, msg.TwoHop)
		assert.Equal(t, intermediaryPubkey, msg.TwoHop.IntermediaryPubkey)
		require.NoError(t, msg.Validate(&SwapData{}))
	})

	t.Run("swap_out_request without twohop", func(t *testing.T) {
		t.Parallel()

		raw := `{
			"protocol_version": 5,
			"swap_id": "` + swapID + `",
			"asset": "",
			"network": "regtest",
			"scid": "1x2x3",
			"amount": 100,
			"pubkey": "` + pubkey + `",
			"acceptable_premium": 0
		}`

		var msg SwapOutRequestMessage
		require.NoError(t, json.Unmarshal([]byte(raw), &msg))
		assert.Nil(t, msg.TwoHop)
		require.NoError(t, msg.Validate(&SwapData{}))
	})

	t.Run("swap_out_request with twohop", func(t *testing.T) {
		t.Parallel()

		raw := `{
			"protocol_version": 5,
			"swap_id": "` + swapID + `",
			"asset": "",
			"network": "regtest",
			"scid": "1x2x3",
			"amount": 100,
			"pubkey": "` + pubkey + `",
			"acceptable_premium": 0,
			"twohop": {
				"intermediary_pubkey": "` + intermediaryPubkey + `"
			}
		}`

		var msg SwapOutRequestMessage
		require.NoError(t, json.Unmarshal([]byte(raw), &msg))
		require.NotNil(t, msg.TwoHop)
		assert.Equal(t, intermediaryPubkey, msg.TwoHop.IntermediaryPubkey)
		require.NoError(t, msg.Validate(&SwapData{}))
	})

	t.Run("swap_in_agreement with twohop.incoming_scid", func(t *testing.T) {
		t.Parallel()

		raw := `{
			"protocol_version": 5,
			"swap_id": "` + swapID + `",
			"pubkey": "` + pubkey + `",
			"premium": 10,
			"twohop": {
				"incoming_scid": "4x5x6"
			}
		}`

		var msg SwapInAgreementMessage
		require.NoError(t, json.Unmarshal([]byte(raw), &msg))
		require.NotNil(t, msg.TwoHop)
		assert.Equal(t, "4x5x6", msg.TwoHop.IncomingScid)
		require.NoError(t, msg.Validate(&SwapData{}))
	})

	t.Run("swap_out_agreement with twohop.incoming_scid", func(t *testing.T) {
		t.Parallel()

		raw := `{
			"protocol_version": 5,
			"swap_id": "` + swapID + `",
			"pubkey": "` + pubkey + `",
			"Payreq": "lnbc1...",
			"premium": 10,
			"twohop": {
				"incoming_scid": "4x5x6"
			}
		}`

		var msg SwapOutAgreementMessage
		require.NoError(t, json.Unmarshal([]byte(raw), &msg))
		require.NotNil(t, msg.TwoHop)
		assert.Equal(t, "4x5x6", msg.TwoHop.IncomingScid)
		require.NoError(t, msg.Validate(&SwapData{}))
	})

	t.Run("marshal omits twohop when nil", func(t *testing.T) {
		t.Parallel()

		msg := SwapOutRequestMessage{TwoHop: nil}
		b, err := json.Marshal(msg)
		require.NoError(t, err)
		assert.NotContains(t, string(b), "twohop")
	})
}

func TestPeerswapMessages_TwoHopValidation(t *testing.T) {
	t.Parallel()

	swapID, err := ParseSwapIdFromString(strings.Repeat("00", 32))
	require.NoError(t, err)

	pubkey := "02" + strings.Repeat("00", 32)

	t.Run("request rejects invalid intermediary_pubkey", func(t *testing.T) {
		t.Parallel()

		msg := SwapOutRequestMessage{
			ProtocolVersion: 5,
			SwapId:          swapID,
			Asset:           "",
			Network:         "regtest",
			Scid:            "1x2x3",
			Amount:          1,
			Pubkey:          pubkey,
			TwoHop: &TwoHop{
				IntermediaryPubkey: "00",
			},
		}
		require.Error(t, msg.Validate(&SwapData{}))
	})

	t.Run("agreement rejects invalid incoming_scid", func(t *testing.T) {
		t.Parallel()

		msg := SwapOutAgreementMessage{
			ProtocolVersion: 5,
			SwapId:          swapID,
			Pubkey:          pubkey,
			TwoHop: &TwoHop{
				IncomingScid: "not-a-scid",
			},
		}
		require.Error(t, msg.Validate(&SwapData{}))
	})
}
