package electrum

import (
	"context"

	"github.com/checksum0/go-electrum/electrum"
)

type RPC interface {
	SubscribeHeaders(ctx context.Context) (<-chan *electrum.SubscribeHeadersResult, error)
	GetHistory(ctx context.Context, scripthash string) ([]*electrum.GetMempoolResult, error)
	GetRawTransaction(ctx context.Context, txHash string) (string, error)
	BroadcastTransaction(ctx context.Context, rawTx string) (string, error)
	GetFee(ctx context.Context, target uint32) (float32, error)
	Ping(ctx context.Context) error
}
