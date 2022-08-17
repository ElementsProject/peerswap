package lnd

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/elementsproject/peerswap/log"

	"github.com/elementsproject/peerswap/lightning"
	"github.com/elementsproject/peerswap/messages"
	"github.com/elementsproject/peerswap/onchain"
	"github.com/elementsproject/peerswap/poll"
	"github.com/elementsproject/peerswap/swap"
	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	"github.com/lightningnetwork/lnd/lnrpc/walletrpc"
	"github.com/lightningnetwork/lnd/lnwire"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

const (
	// defaultGrpcBackoffTime is the linear backoff time between failing grpc
	// calls (also server side stream) to the lnd node.
	defaultGrpcBackoffTime   = 1 * time.Second
	defaultGrpcBackoffJitter = 0.1

	// defaultMaxGrpcRetries is the amount of retries we take if the grpc
	// connection to the lnd node drops for some reason or if the resource is
	// exhausted. With the defaultGrpcBackoffTime and a linear backoff stratefgy
	// this leads to roughly 5h of retry time.
	defaultMaxGrpcRetries = 5
)

var (
	// defaultGrpcRetryCodes are the grpc status codes that are returned with an
	// error, on which we retry our call (and server side stream) to the lnd
	// node. The codes represent:
	// - Unavailable:	The service is currently unavailable. This is most
	//					likely a transient condition, which can be correctesd by
	//					retrying with a backoff. Note that it is not always safe
	//					to retry non-idempotent operations.
	//
	// - ResourceExhausted:	Some resource has been exhausted, perhaps a per-user
	//						quota, or perhaps the entire file system is out of
	//						space.
	defaultGrpcRetryCodes []codes.Code = []codes.Code{
		codes.Unavailable,
		codes.ResourceExhausted,
	}

	// defaultGrpcRetryCodesWithMsg are grpc status codes that must have a
	// matching message for us to retry. This is due to LND using a confusing
	// rpc error code on startup.
	// See: https://github.com/lightningnetwork/lnd/issues/6765
	//
	// This is also the reason that we need to use a fork of the
	// go-grpc-middleware "retry" to provide this optional check.
	defaultGrpcRetryCodesWithMsg []grpc_retry.CodeWithMsg = []grpc_retry.CodeWithMsg{
		{
			Code: codes.Unknown,
			Msg:  "the RPC server is in the process of starting up, but not yet ready to accept calls",
		},
		{
			Code: codes.Unknown,
			Msg:  "chain notifier RPC is still in the process of starting",
		},
	}
)

// Client combines multiple methods, functions and watchers to a service that
// is consumed by the swap service. Client fulfils the swap.LightningClient
// interface.
// TODO: Rework the swap.LightningClient interface to separate the watchers from
// the client service. This will make it easier to test and more modular to use.
type Client struct {
	lndClient    lnrpc.LightningClient
	walletClient walletrpc.WalletKitClient
	routerClient routerrpc.RouterClient

	PollService    *poll.Service
	bitcoinOnChain *onchain.BitcoinOnChain
	paymentWatcher *PaymentWatcher

	cc  *grpc.ClientConn
	ctx context.Context

	messageHandler       []func(peerId string, msgType string, payload []byte) error
	invoiceSubscriptions map[string]interface{}
	pubkey               string

	sync.Mutex
}

func NewClient(ctx context.Context, cc *grpc.ClientConn, paymentWatcher *PaymentWatcher, chain *onchain.BitcoinOnChain) (*Client, error) {
	// TODO: Refactor this module so that it becomes testable. At the moment a
	// LND client is always needed, so we can not provide mocked unit tests.

	lndClient := lnrpc.NewLightningClient(cc)
	walletClient := walletrpc.NewWalletKitClient(cc)
	routerClient := routerrpc.NewRouterClient(cc)

	gi, err := lndClient.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		return nil, err
	}
	return &Client{
		lndClient:            lndClient,
		walletClient:         walletClient,
		routerClient:         routerClient,
		paymentWatcher:       paymentWatcher,
		bitcoinOnChain:       chain,
		cc:                   cc,
		ctx:                  ctx,
		pubkey:               gi.IdentityPubkey,
		invoiceSubscriptions: make(map[string]interface{}),
	}, nil
}

func (l *Client) Stop() {
	l.paymentWatcher.Stop()
}

func (l *Client) AddPaymentNotifier(swapId string, payreq string, invoiceType swap.InvoiceType) {
	l.paymentWatcher.AddWaitForPayment(swapId, payreq, invoiceType)
}

func (l *Client) DecodePayreq(payreq string) (paymentHash string, amountMsat uint64, err error) {
	decoded, err := l.lndClient.DecodePayReq(l.ctx, &lnrpc.PayReqString{PayReq: payreq})
	if err != nil {
		return "", 0, err
	}
	return decoded.PaymentHash, uint64(decoded.NumMsat), nil
}

func (l *Client) PayInvoice(payreq string) (preImage string, err error) {
	payres, err := l.lndClient.SendPaymentSync(l.ctx, &lnrpc.SendRequest{PaymentRequest: payreq})
	if err != nil {
		return "", nil
	}
	return hex.EncodeToString(payres.PaymentPreimage), nil
}

func (l *Client) CheckChannel(shortChannelId string, amountSat uint64) (*lnrpc.Channel, error) {
	res, err := l.lndClient.ListChannels(l.ctx, &lnrpc.ListChannelsRequest{ActiveOnly: true})
	if err != nil {
		return nil, err
	}

	var channel *lnrpc.Channel
	for _, v := range res.Channels {
		channelShortId := lnwire.NewShortChanIDFromInt(v.ChanId)
		if channelShortId.String() == shortChannelId || LndShortChannelIdToCLShortChannelId(channelShortId) == shortChannelId {
			channel = v
			break
		}
	}
	if channel == nil {
		return nil, errors.New("channel not found")
	}
	if channel.LocalBalance < int64(amountSat) {
		return nil, errors.New("not enough outbound capacity to pay invoice")
	}

	return channel, nil
}

func (l *Client) GetPayreq(msatAmount uint64, preimageString string, swapId string, memo string, invoiceType swap.InvoiceType, expiry uint64) (string, error) {
	preimage, err := lightning.MakePreimageFromStr(preimageString)
	if err != nil {
		return "", err
	}

	payreq, err := l.lndClient.AddInvoice(l.ctx, &lnrpc.Invoice{
		ValueMsat:  int64(msatAmount),
		Memo:       memo,
		RPreimage:  preimage[:],
		Expiry:     int64(expiry),
		CltvExpiry: 144,
	})
	if err != nil {
		return "", err
	}
	return payreq.PaymentRequest, nil
}

func (l *Client) AddPaymentCallback(f func(swapId string, invoiceType swap.InvoiceType)) {
	l.paymentWatcher.AddPaymentCallback(f)
}

func (l *Client) PayInvoiceViaChannel(payreq, scid string) (preimage string, err error) {
	decoded, err := l.lndClient.DecodePayReq(l.ctx, &lnrpc.PayReqString{PayReq: payreq})
	if err != nil {
		return "", err
	}

	channel, err := l.CheckChannel(scid, uint64(decoded.NumSatoshis))
	if err != nil {
		return "", err
	}

	paymentStream, err := l.routerClient.SendPaymentV2(l.ctx, &routerrpc.SendPaymentRequest{
		PaymentRequest:  payreq,
		TimeoutSeconds:  30,
		OutgoingChanIds: []uint64{channel.ChanId},
		MaxParts:        1,
	})

	if err != nil {
		return "", err
	}

	for {
		res, err := paymentStream.Recv()
		if err != nil {
			return "", err
		}
		switch res.Status {
		case lnrpc.Payment_UNKNOWN:
			log.Debugf("PayInvoiceViaChannel: payment is unknown")
		case lnrpc.Payment_SUCCEEDED:
			return res.PaymentPreimage, nil
		case lnrpc.Payment_IN_FLIGHT:
			log.Debugf("PayInvoiceViaChannel: payment still in flight")
		case lnrpc.Payment_FAILED:
			return "", fmt.Errorf("payment failure %s", res.FailureReason)
		default:
			log.Debugf("PayInvoiceViaChannel: got unexpected payment status %d", res.Status)
		}
		time.Sleep(time.Millisecond * 100)
	}
}

func (l *Client) RebalancePayment(payreq string, channelId string) (preimage string, err error) {
	decoded, err := l.lndClient.DecodePayReq(l.ctx, &lnrpc.PayReqString{PayReq: payreq})
	if err != nil {
		return "", err
	}

	channel, err := l.CheckChannel(channelId, uint64(decoded.NumSatoshis))
	if err != nil {
		return "", err
	}

	paymentStream, err := l.routerClient.SendPaymentV2(l.ctx, &routerrpc.SendPaymentRequest{
		PaymentRequest:  payreq,
		TimeoutSeconds:  30,
		OutgoingChanIds: []uint64{channel.ChanId},
		MaxParts:        30,
	})

	if err != nil {
		return "", err
	}

	for {
		select {
		case <-l.ctx.Done():
			return "", errors.New("context done")
		default:
			res, err := paymentStream.Recv()
			if err != nil {
				return "", err
			}
			switch res.Status {
			case lnrpc.Payment_SUCCEEDED:
				return res.PaymentPreimage, nil
			case lnrpc.Payment_IN_FLIGHT:
				log.Debugf("payment in flight")
			case lnrpc.Payment_FAILED:
				return "", fmt.Errorf("payment failure %s", res.FailureReason)
			default:
				continue
			}
			time.Sleep(time.Millisecond * 10)
		}
	}
}

func (l *Client) SendMessage(peerId string, message []byte, messageType int) error {
	peerBytes, err := hex.DecodeString(peerId)
	if err != nil {
		return err
	}

	_, err = l.lndClient.SendCustomMessage(l.ctx, &lnrpc.SendCustomMessageRequest{
		Peer: peerBytes,
		Type: uint32(messageType),
		Data: message,
	})
	if err != nil {
		return err
	}
	return nil
}

func (l *Client) AddMessageHandler(f func(peerId string, msgType string, payload []byte) error) {
	l.messageHandler = append(l.messageHandler, f)
}

func (l *Client) PrepareOpeningTransaction(address string, amount uint64) (txId string, txHex string, err error) {
	return "", "", nil
}

func (l *Client) StartListening() {
	go l.startListenMessages()
	go l.startListenPeerEvents()
}

func (l *Client) startListenMessages() {
	err := l.listenMessages()
	if err != nil {
		log.Infof("error listening on messages %v, restarting", err)
		l.startListenMessages()
	}
}

func (l *Client) startListenPeerEvents() {
	err := l.listenPeerEvents()
	if err != nil {
		log.Infof("error listening on peer events %v", err)
		l.startListenPeerEvents()
	}
}

func (l *Client) GetPeers() []string {
	res, err := l.lndClient.ListPeers(l.ctx, &lnrpc.ListPeersRequest{})
	if err != nil {
		log.Debugf("could not listpeers: %v", err)
		return nil
	}

	var peerlist []string
	for _, peer := range res.Peers {
		peerlist = append(peerlist, peer.PubKey)
	}
	return peerlist
}

func (l *Client) listenMessages() error {
	client, err := l.lndClient.SubscribeCustomMessages(l.ctx, &lnrpc.SubscribeCustomMessagesRequest{})
	if err != nil {
		return err
	}
	for {
		select {
		case <-l.ctx.Done():
			return client.CloseSend()
		default:
			msg, err := client.Recv()
			if err != nil {
				return err
			}

			err = l.handleCustomMessage(msg)
			if err != nil {
				log.Infof("Error handling msg %v", err)
			}
		}
	}
}

func (l *Client) listenPeerEvents() error {
	client, err := l.lndClient.SubscribePeerEvents(l.ctx, &lnrpc.PeerEventSubscription{})
	if err != nil {
		return err
	}
	for {
		select {
		case <-l.ctx.Done():
			return client.CloseSend()
		default:
			msg, err := client.Recv()
			if err != nil {
				return err
			}
			if msg.Type == lnrpc.PeerEvent_PEER_ONLINE {
				if l.PollService != nil {
					l.PollService.Poll(msg.PubKey)
				}
			}
		}
	}
}

func (l *Client) handleCustomMessage(msg *lnrpc.CustomMessage) error {
	peerId := hex.EncodeToString(msg.Peer)
	for _, v := range l.messageHandler {
		err := v(peerId, messages.MessageTypeToHexString(messages.MessageType(msg.Type)), msg.Data)
		if err != nil {
			log.Infof("\n Custom Message Handler err: %v", err)
		}
	}
	return nil
}

func LndShortChannelIdToCLShortChannelId(lndCI lnwire.ShortChannelID) string {
	return fmt.Sprintf("%dx%dx%d", lndCI.BlockHeight, lndCI.TxIndex, lndCI.TxPosition)
}
