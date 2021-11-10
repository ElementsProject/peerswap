package messages

import (
	"strconv"
	"testing"
)

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
		{"t1", args{MessageTypeToHexString(BASE_MESSAGE_TYPE - 1)}, false, true},
		{"t2", args{MessageTypeToHexString(BASE_MESSAGE_TYPE)}, true, false},
		{"t3", args{MessageTypeToHexString(MESSAGETYPE_SWAPINAGREEMENT)}, true, false},
		{"t4", args{MessageTypeToHexString(UPPER_MESSAGE_BOUND - 1)}, true, false},
		{"t5", args{MessageTypeToHexString(UPPER_MESSAGE_BOUND)}, false, true},
		{"t6", args{MessageTypeToHexString(UPPER_MESSAGE_BOUND + 1)}, false, false},
		{"t7", args{"z"}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := inRangeStr(tt.args.msg)
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

func TestHexStringToMessageType(t *testing.T) {
	type args struct {
		msgType string
	}
	tests := []struct {
		name    string
		args    args
		want    MessageType
		wantErr bool
	}{
		{"t1", args{MessageTypeToHexString(MESSAGETYPE_SWAPINREQUEST)}, MESSAGETYPE_SWAPINREQUEST, false},
		{"t2", args{MessageTypeToHexString(BASE_MESSAGE_TYPE + 1)}, 0, true},
		{"t3", args{MessageTypeToHexString(UPPER_MESSAGE_BOUND + 1)}, 0, true},
		{"t4", args{"z"}, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := HexStringToMessageType(tt.args.msgType)
			if (err != nil) != tt.wantErr {
				t.Errorf("HexStringToMessageType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("HexStringToMessageType() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func inRangeStr(msg string) (bool, error) {
	msgInt, err := strconv.ParseInt(msg, 16, 64)
	if err != nil {
		return false, err
	}
	return InRange(MessageType(msgInt))
}
