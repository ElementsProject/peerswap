package messages

import "testing"

func TestPeerswapCustomMessageType(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		msgType string
		want    MessageType
		wantErr bool
	}{
		"swapinrequest":       {msgType: MessageTypeToHexString(MESSAGETYPE_SWAPINREQUEST), want: MESSAGETYPE_SWAPINREQUEST},
		"swapoutrequest":      {msgType: MessageTypeToHexString(MESSAGETYPE_SWAPOUTREQUEST), want: MESSAGETYPE_SWAPOUTREQUEST},
		"swapinagreement":     {msgType: MessageTypeToHexString(MESSAGETYPE_SWAPINAGREEMENT), want: MESSAGETYPE_SWAPINAGREEMENT},
		"swapoutagreement":    {msgType: MessageTypeToHexString(MESSAGETYPE_SWAPOUTAGREEMENT), want: MESSAGETYPE_SWAPOUTAGREEMENT},
		"openintxbroadcasted": {msgType: MessageTypeToHexString(MESSAGETYPE_OPENINGTXBROADCASTED), want: MESSAGETYPE_OPENINGTXBROADCASTED},
		"canceled":            {msgType: MessageTypeToHexString(MESSAGETYPE_CANCELED), want: MESSAGETYPE_CANCELED},
		"coopclose":           {msgType: MessageTypeToHexString(MESSAGETYPE_COOPCLOSE), want: MESSAGETYPE_COOPCLOSE},
		"poll":                {msgType: MessageTypeToHexString(MESSAGETYPE_POLL), want: MESSAGETYPE_POLL},
		"request_poll":        {msgType: MessageTypeToHexString(MESSAGETYPE_REQUEST_POLL), want: MESSAGETYPE_REQUEST_POLL},
		"invalid":             {msgType: "invalid", wantErr: true},
	}
	for name, tt := range tests {
		tt := tt
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := PeerswapCustomMessageType(tt.msgType)
			if (err != nil) != tt.wantErr {
				t.Errorf("PeerswapCustomMessageType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("PeerswapCustomMessageType() = %v, want %v", got, tt.want)
			}
		})
	}
}
