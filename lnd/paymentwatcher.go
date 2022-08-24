package lnd

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"sync"

	"github.com/elementsproject/peerswap/log"
	"github.com/elementsproject/peerswap/swap"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/invoicesrpc"
	"google.golang.org/grpc"
)

type PaymentWatcher struct {
	sync.Mutex
	wg sync.WaitGroup

	invoicesrpcClient invoicesrpc.InvoicesClient
	lnrpcClient       lnrpc.LightningClient

	paymentCallback func(string, swap.InvoiceType)
	paymentWatchers map[string]bool

	ctx    context.Context
	cancel context.CancelFunc
}

func NewPaymentWatcher(ctx context.Context, cc *grpc.ClientConn) (*PaymentWatcher, error) {
	invoicesrpcClient := invoicesrpc.NewInvoicesClient(cc)
	lnrpcClient := lnrpc.NewLightningClient(cc)

	// Check that service is reachable.
	_, err := lnrpcClient.GetInfo(context.TODO(), &lnrpc.GetInfoRequest{})
	if err != nil {
		return nil, fmt.Errorf(
			"unable to reach out to lnd for GetInfo(): %v", err,
		)
	}

	ctx, cancel := context.WithCancel(ctx)

	return &PaymentWatcher{
		invoicesrpcClient: invoicesrpcClient,
		lnrpcClient:       lnrpcClient,
		paymentWatchers:   make(map[string]bool),
		ctx:               ctx,
		cancel:            cancel,
	}, nil
}

func (p *PaymentWatcher) Stop() error {
	p.cancel()
	log.Infof("[PaymentWatcher] Stop called, cancel context and wait for subscriptions to close")
	p.wg.Wait()
	return nil
}

func (p *PaymentWatcher) AddWaitForPayment(swapId string, payreq string, invoiceType swap.InvoiceType) {
	p.Lock()
	if _, ok := p.paymentWatchers[payreq]; ok {
		log.Debugf("[PaymentWatcher] Swap: %s: Tried to resubscribe to payment watcher for payreq %s", swapId, payreq)
		p.Unlock()
		return
	}
	log.Debugf("[PaymentWatcher] Swap: %s: Add new payment watcher for payreq %s of type \"%s\"", swapId, payreq, invoiceType)
	p.paymentWatchers[payreq] = true
	p.Unlock()

	ctx, cancel := context.WithCancel(context.TODO())
	invoice, err := p.lnrpcClient.DecodePayReq(ctx, &lnrpc.PayReqString{PayReq: payreq})
	if err != nil {
		log.Infof("[PaymentWatcher] Swap: %s: Could not decode invoice: %v", swapId, err)
		cancel()
		return
	}

	rHash, err := hex.DecodeString(invoice.PaymentHash)
	if err != nil {
		log.Infof("[PaymentWatcher] Swap: %s: Could not decode payment hash error: %v", swapId, err)
		cancel()
		return
	}

	stream, err := p.invoicesrpcClient.SubscribeSingleInvoice(
		ctx,
		&invoicesrpc.SubscribeSingleInvoiceRequest{
			RHash: rHash,
		},
	)
	if err != nil {
		log.Infof("[PaymentWatcher] Swap: %s: Could not subscribe to invoice: %v", swapId, err)
		cancel()
		return
	}

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer func() {
			p.Lock()
			delete(p.paymentWatchers, payreq)
			p.Unlock()
		}()
		defer cancel()

		for {
			res, err := stream.Recv()
			if err == io.EOF {
				// Stream was closed by the server.
				return
			}
			if err != nil {
				// TODO: better error handling.
				log.Infof("[PaymentWatcher] Swap: %s: Could not read from stream: %v", swapId, err)
				return
			}

			switch res.State {
			case lnrpc.Invoice_SETTLED:
				if p.paymentCallback == nil {
					log.Infof("[PaymentWatcher] Swap: %s: paymentCallback was nil", swapId)
					return
				}
				log.Debugf("[PaymentWatcher] Swap: %s: Calling paymentCallback", swapId)
				p.paymentCallback(swapId, invoiceType)
				return

			case lnrpc.Invoice_CANCELED:
				log.Infof("[PaymentWatcher] Swap: %s: Invoice %v of type: %v was canceled", swapId, payreq, invoiceType)
				return

			case lnrpc.Invoice_OPEN:
				continue

			default:
				log.Debugf("[PaymentWatcher] Swap: %s: Got unknown invoice state: %s", swapId, res.State)
			}
		}
	}()
}

func (p *PaymentWatcher) AddPaymentCallback(f func(swapId string, invoiceType swap.InvoiceType)) {
	p.Lock()
	defer p.Unlock()
	p.paymentCallback = f
}

// func (p *PaymentWatcher)
