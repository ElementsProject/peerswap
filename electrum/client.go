package electrum

import (
	"context"
	"crypto/tls"

	"github.com/checksum0/go-electrum/electrum"
	"github.com/elementsproject/peerswap/log"
)

type electrumClient struct {
	client   *electrum.Client
	endpoint string
	isTLS    bool
}

func NewElectrumClient(ctx context.Context, endpoint string, isTLS bool) (RPC, error) {
	ec, err := newClient(ctx, endpoint, isTLS)
	if err != nil {
		return nil, err
	}
	client := &electrumClient{
		client:   ec,
		endpoint: endpoint,
		isTLS:    isTLS,
	}
	return client, nil
}

// reconnect reconnects to the electrum server if the connection is lost.
func (c *electrumClient) reconnect(ctx context.Context) error {
	if err := c.client.Ping(ctx); err != nil {
		log.Infof("failed to ping electrum server: %v", err)
		log.Infof("reconnecting to electrum server")
		client, err := newClient(ctx, c.endpoint, c.isTLS)
		if err != nil {
			return err
		}
		c.client = client
	}
	return nil
}

func newClient(ctx context.Context, endpoint string, isTLS bool) (*electrum.Client, error) {
	if isTLS {
		return electrum.NewClientSSL(ctx, endpoint, &tls.Config{
			MinVersion: tls.VersionTLS12,
		})
	}
	return electrum.NewClientTCP(ctx, endpoint)
}

func (c *electrumClient) SubscribeHeaders(ctx context.Context) (<-chan *electrum.SubscribeHeadersResult, error) {
	if err := c.reconnect(ctx); err != nil {
		return nil, err
	}
	return c.client.SubscribeHeaders(ctx)
}

func (c *electrumClient) GetHistory(ctx context.Context, scripthash string) ([]*electrum.GetMempoolResult, error) {
	if err := c.reconnect(ctx); err != nil {
		return nil, err
	}
	return c.client.GetHistory(ctx, scripthash)
}

func (c *electrumClient) GetRawTransaction(ctx context.Context, txHash string) (string, error) {
	if err := c.reconnect(ctx); err != nil {
		return "", err
	}
	return c.client.GetRawTransaction(ctx, txHash)
}

func (c *electrumClient) BroadcastTransaction(ctx context.Context, rawTx string) (string, error) {
	if err := c.reconnect(ctx); err != nil {
		return "", err
	}
	return c.client.BroadcastTransaction(ctx, rawTx)
}

func (c *electrumClient) GetFee(ctx context.Context, target uint32) (float32, error) {
	if err := c.reconnect(ctx); err != nil {
		return 0, err
	}
	return c.client.GetFee(ctx, target)
}
