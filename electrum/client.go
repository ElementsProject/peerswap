package electrum

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
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

func (c *electrumClient) Reboot(ctx context.Context) error {
	c.client.Shutdown()
	client, err := newClient(ctx, c.endpoint, c.isTLS)
	if err != nil {
		return err
	}
	c.client = client
	return nil
}

func (c *electrumClient) SubscribeHeaders(ctx context.Context) (<-chan *electrum.SubscribeHeadersResult, error) {
	return c.client.SubscribeHeaders(ctx)
}

func (c *electrumClient) GetHistory(ctx context.Context, scripthash string) ([]*electrum.GetMempoolResult, error) {
	if err := c.reconnect(ctx); err != nil {
		return nil, err
	}
	return c.client.GetHistory(ctx, scripthash)
}

// GetRawTransaction retrieves the raw transaction data for a given transaction
// and handles retries in case of a
// "missing transaction" error. It uses an exponential backoff strategy for
// retries, with a maximum of 10 retries. This is a temporary workaround for
// an issue where a missing transaction error occurs even when the UTXO exists.
// If the issue persists, the backoff strategy may need adjustment.
func (c *electrumClient) GetRawTransaction(ctx context.Context, txHash string) (string, error) {
	var rawTx string

	err := retryWithBackoff(func() error {
		if err := c.reconnect(ctx); err != nil {
			return err
		}
		var innerErr error
		rawTx, innerErr = c.client.GetRawTransaction(ctx, txHash)
		return innerErr
	})

	return rawTx, err
}

func retryWithBackoff(operation func() error) error {
	const maxRetries = 10
	const maxElapsedTime = 2 * time.Minute

	backoffStrategy := backoff.NewExponentialBackOff()
	backoffStrategy.MaxElapsedTime = maxElapsedTime

	return backoff.Retry(func() error {
		err := operation()
		if err != nil {
			log.Infof("Error during operation: %v", err)
			if strings.Contains(err.Error(), "missing transaction") {
				log.Infof("Retrying due to missing transaction error: %v", err)
				return err
			}
			return backoff.Permanent(fmt.Errorf("permanent error: %w", err))
		}
		return nil
	}, backoff.WithMaxRetries(backoffStrategy, uint64(maxRetries)))
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

func (c *electrumClient) Ping(ctx context.Context) error {
	return c.reconnect(ctx)
}
