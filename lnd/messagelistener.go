package lnd

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"sync"

	"github.com/elementsproject/peerswap/log"
	"github.com/elementsproject/peerswap/messages"
	"github.com/lightningnetwork/lnd/lnrpc"
	"google.golang.org/grpc"
)

type MessageListener struct {
	sync.Mutex
	wg sync.WaitGroup

	ctx    context.Context
	cancel context.CancelFunc

	lnrpcClient lnrpc.LightningClient
	handlers    []func(peerId string, msgType string, payload []byte) error
}

func NewMessageListener(ctx context.Context, cc *grpc.ClientConn) (*MessageListener, error) {
	lnrpcClient := lnrpc.NewLightningClient(cc)

	// Check that service is available
	_, err := lnrpcClient.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		return nil, fmt.Errorf(
			"[MsgListener]: Unable to reach out to lnd for GetInfo(): %v", err,
		)
	}

	ctx, cancel := context.WithCancel(ctx)

	return &MessageListener{
		lnrpcClient: lnrpcClient,
		ctx:         ctx,
		cancel:      cancel,
	}, nil
}

func (m *MessageListener) Start() error {
	stream, err := m.lnrpcClient.SubscribeCustomMessages(m.ctx, &lnrpc.SubscribeCustomMessagesRequest{})
	if err != nil {
		return err
	}

	log.Infof("[MsgListener]: Start listening for custom messages")

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				log.Infof("[MsgListener]: Stream closed")
				return
			}
			if err != nil {
				log.Infof("[MsgListener]: Stream closed with err: %v", err)
				return
			}

			peerId := hex.EncodeToString(msg.Peer)
			log.Debugf("[MsgListener]: Received custom message type %s from %s", messages.MessageTypeToHexString(messages.MessageType(msg.Type)), peerId)

			m.Lock()
			for _, handler := range m.handlers {
				err := handler(peerId, messages.MessageTypeToHexString(messages.MessageType(msg.Type)), msg.Data)
				if err != nil {
					log.Infof("[MsgListener]: Handler failed: %v", err)
				}
			}
			m.Unlock()
		}
	}()

	return nil
}

func (m *MessageListener) Stop() {
	m.cancel()
	m.wg.Wait()
}

func (m *MessageListener) AddMessageHandler(f func(peerId string, msgType string, payload []byte) error) {
	m.Lock()
	defer m.Unlock()
	m.handlers = append(m.handlers, f)
}
