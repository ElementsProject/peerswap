package swap

import "testing"

func TestInRange(t *testing.T) {
	type args struct {
		msg string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{"t1", args{"a455"}, true, false},
		{"t2", args{"a456"}, false, false},
		{"t3", args{"a461"}, true, false},
		{"t4", args{"a463"}, false, false},
		{"t5", args{"z"}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := InRange(tt.args.msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("InRange() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("InRange() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHexStrToMsgType(t *testing.T) {
	type args struct {
		msgType string
	}
	tests := []struct {
		name    string
		args    args
		want    MessageType
		wantErr bool
	}{
		{"t1", args{"a455"}, MESSAGETYPE_SWAPINREQUEST, false},
		{"t2", args{"a456"}, 0, true},
		{"t3", args{"a461"}, MESSAGETYPE_CLAIMED, false},
		{"t4", args{"a463"}, 0, true},
		{"t5", args{"z"}, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := HexStrToMsgType(tt.args.msgType)
			if (err != nil) != tt.wantErr {
				t.Errorf("HexStrToMsgType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("HexStrToMsgType() got = %v, want %v", got, tt.want)
			}
		})
	}
}
