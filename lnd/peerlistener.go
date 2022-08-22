package lnd

import (
	"context"
	"fmt"
	"sync"

	"github.com/elementsproject/peerswap/log"
	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"github.com/lightningnetwork/lnd/lnrpc"
	"google.golang.org/grpc"
)

type PeerListener struct {
	sync.Mutex
	wg sync.WaitGroup

	ctx    context.Context
	cancel context.CancelFunc

	lnrpcClient lnrpc.LightningClient
	// handlers holds a map that connects the lnrpc.PeerEvent__EventType to the
	// handler functions that should be called if this lnrpc.PeerEvent_EventType
	// is received.
	handlers map[int32][]func(peerId string)
}

func NewPeerListener(ctx context.Context, cc *grpc.ClientConn) (*PeerListener, error) {
	lnrpcClient := lnrpc.NewLightningClient(cc)

	// Check that service is available
	_, err := lnrpcClient.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		return nil, fmt.Errorf("unable to reach out to lnd for GetInfo(): %v", err)
	}

	ctx, cancel := context.WithCancel(ctx)

	pl := &PeerListener{
		ctx:         ctx,
		cancel:      cancel,
		lnrpcClient: lnrpcClient,
		handlers:    make(map[int32][]func(peerId string)),
	}

	if err := pl.start(); err != nil {
		return nil, err
	}

	return pl, nil
}

func (p *PeerListener) start() error {
	stream, err := p.lnrpcClient.SubscribePeerEvents(
		p.ctx,
		&lnrpc.PeerEventSubscription{},
		grpc_retry.WithIgnoreEOF(),
	)
	if err != nil {
		return err
	}

	log.Infof("[PeerListener]: Start listening for peer events")

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		for {
			evt, err := stream.Recv()
			// if err == io.EOF {
			// log.Infof("[PeerListener]: Stream closed")
			// return
			// }
			if err != nil {
				log.Infof("[PeerListener]: Stream closed with err: %v", err)
				return
			}

			log.Debugf("[PeerListener]: %s %s", evt.Type, evt.PubKey)
			p.Lock()
			for _, handler := range p.handlers[int32(evt.Type)] {
				go handler(evt.PubKey)
			}
			p.Unlock()
		}
	}()

	return nil
}

func (p *PeerListener) Stop() {
	p.cancel()
	p.wg.Wait()
}

func (p *PeerListener) AddHandler(evt lnrpc.PeerEvent_EventType, f func(string)) error {
	p.Lock()
	defer p.Unlock()

	switch evt {
	case lnrpc.PeerEvent_PEER_ONLINE, lnrpc.PeerEvent_PEER_OFFLINE:
		p.handlers[int32(evt)] = append(p.handlers[int32(evt)], f)
		return nil
	}
	return fmt.Errorf("unknown peer event type: %s", evt)
}
