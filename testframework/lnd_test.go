package testframework

import (
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsLndConnectPeerStartupError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "grpc unavailable",
			err:  status.Error(codes.Unavailable, "connection refused"),
			want: true,
		},
		{
			name: "lnd still starting",
			err: status.Error(
				codes.Unknown,
				"server is still in the process of starting",
			),
			want: true,
		},
		{
			name: "lnd starting up",
			err: status.Error(
				codes.Unknown,
				"server is in the process of starting up, but not yet ready to accept calls",
			),
			want: true,
		},
		{
			name: "unrelated unknown",
			err:  status.Error(codes.Unknown, "peer already connected"),
			want: false,
		},
		{
			name: "plain error",
			err:  errors.New("server is still in the process of starting"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := isLndConnectPeerStartupError(test.err)
			if got != test.want {
				t.Fatalf("isLndConnectPeerStartupError() = %v, want %v", got, test.want)
			}
		})
	}
}
