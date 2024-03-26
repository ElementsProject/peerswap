package electrum

import (
	"context"

	"github.com/checksum0/go-electrum/electrum"
)

type RPC interface {
	SubscribeHeaders(ctx context.Context) (<-chan *electrum.SubscribeHeadersResult, error)
	GetHistory(ctx context.Context, scripthash string) ([]*electrum.GetMempoolResult, error)
	GetRawTransaction(ctx context.Context, txHash string) (string, error)
}
