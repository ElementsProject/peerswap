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
	"github.com/elementsproject/peerswap/onchain"
	"github.com/elementsproject/peerswap/swap"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	"github.com/lightningnetwork/lnd/lnrpc/walletrpc"
	"github.com/lightningnetwork/lnd/lnwire"
	"google.golang.org/grpc"
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

	bitcoinOnChain  *onchain.BitcoinOnChain
	paymentWatcher  *PaymentWatcher
	messageListener *MessageListener

	cc  *grpc.ClientConn
	ctx context.Context

	invoiceSubscriptions map[string]interface{}
	pubkey               string

	sync.Mutex
}

func NewClient(
	ctx context.Context,
	cc *grpc.ClientConn,
	paymentWatcher *PaymentWatcher,
	messageListener *MessageListener,
	chain *onchain.BitcoinOnChain,
) (*Client, error) {
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
		messageListener:      messageListener,
		bitcoinOnChain:       chain,
		cc:                   cc,
		ctx:                  ctx,
		pubkey:               gi.IdentityPubkey,
		invoiceSubscriptions: make(map[string]interface{}),
	}, nil
}

// CanSpend has no functionality, it is just here to fulfill the interface.
func (l *Client) CanSpend(amtMsat uint64) error {
	return nil
}

// SpendableMsat returns an estimate of the total we could send through the
// channel with given scid.
func (l *Client) SpendableMsat(scid string) (uint64, error) {
	s := lightning.Scid(scid)
	r, err := l.lndClient.ListChannels(context.Background(), &lnrpc.ListChannelsRequest{
		ActiveOnly:   false,
		InactiveOnly: false,
		PublicOnly:   false,
		PrivateOnly:  false,
	})
	if err != nil {
		return 0, err
	}
	for _, ch := range r.Channels {
		channelShortId := lnwire.NewShortChanIDFromInt(ch.ChanId)
		if channelShortId.String() == s.LndStyle() {
			return uint64(ch.LocalBalance * 1000), nil
		}
	}
	return 0, fmt.Errorf("could not find a channel with scid: %s", scid)
}

// Implementation returns the name of the lightning network client
// implementation.
func (l *Client) Implementation() string {
	return "LND"
}

func (l *Client) StartListening() error {
	return l.messageListener.Start()
}

func (l *Client) AddPaymentNotifier(swapId string, payreq string, invoiceType swap.InvoiceType) {
	l.paymentWatcher.AddWaitForPayment(swapId, payreq, invoiceType)
}

func (l *Client) DecodePayreq(payreq string) (paymentHash string, amountMsat uint64, expiry int64, err error) {
	decoded, err := l.lndClient.DecodePayReq(l.ctx, &lnrpc.PayReqString{PayReq: payreq})
	if err != nil {
		return "", 0, 0, err
	}
	return decoded.PaymentHash, uint64(decoded.NumMsat), decoded.CltvExpiry, nil
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

func (l *Client) GetPayreq(msatAmount uint64, preimageString string, swapId string, memo string, invoiceType swap.InvoiceType, expirySeconds, expiryCltv uint64) (string, error) {
	preimage, err := lightning.MakePreimageFromStr(preimageString)
	if err != nil {
		return "", err
	}

	payreq, err := l.lndClient.AddInvoice(l.ctx, &lnrpc.Invoice{
		ValueMsat:  int64(msatAmount),
		Memo:       memo,
		RPreimage:  preimage[:],
		Expiry:     int64(expirySeconds),
		CltvExpiry: expiryCltv,
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
		CltvLimit:       int32(decoded.Expiry),
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
	l.messageListener.AddMessageHandler(f)
}

func (l *Client) PrepareOpeningTransaction(address string, amount uint64) (txId string, txHex string, err error) {
	return "", "", nil
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

func LndShortChannelIdToCLShortChannelId(lndCI lnwire.ShortChannelID) string {
	return fmt.Sprintf("%dx%dx%d", lndCI.BlockHeight, lndCI.TxIndex, lndCI.TxPosition)
}
