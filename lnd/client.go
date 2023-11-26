package lnd

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/elementsproject/peerswap/log"

	"github.com/elementsproject/peerswap/lightning"
	"github.com/elementsproject/peerswap/onchain"
	"github.com/elementsproject/peerswap/swap"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	"github.com/lightningnetwork/lnd/lnrpc/walletrpc"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
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

// getMaxHtlcAmtMsat returns the maximum htlc amount in msat for a channel.
// If for some reason it cannot be retrieved, return 0.
func (l *Client) getMaxHtlcAmtMsat(chanId uint64, pubkey string) (uint64, error) {
	var maxHtlcAmtMsat uint64 = 0
	r, err := l.lndClient.GetChanInfo(context.Background(), &lnrpc.ChanInfoRequest{
		ChanId: chanId,
	})
	if err != nil {
		// Ignore err because channel graph information is not always set.
		return maxHtlcAmtMsat, nil
	}
	if r.Node1Pub == pubkey {
		maxHtlcAmtMsat = r.GetNode1Policy().GetMaxHtlcMsat()
	} else if r.Node2Pub == pubkey {
		maxHtlcAmtMsat = r.GetNode2Policy().GetMaxHtlcMsat()
	}
	return maxHtlcAmtMsat, nil
}

func min(x, y uint64) uint64 {
	if x < y {
		return x
	}
	return y
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
			if err = l.checkChannel(ch); err != nil {
				return 0, err
			}
			maxHtlcAmtMsat, err := l.getMaxHtlcAmtMsat(ch.ChanId, l.pubkey)
			if err != nil {
				return 0, err
			}
			spendable := (uint64(ch.GetLocalBalance()) -
				ch.GetLocalConstraints().GetChanReserveSat()*1000)
			// since the max htlc limit is not always set reliably,
			// the check is skipped if it is not set.
			if maxHtlcAmtMsat == 0 {
				return spendable, nil
			}
			return min(maxHtlcAmtMsat, spendable), nil

		}
	}
	return 0, fmt.Errorf("could not find a channel with scid: %s", scid)
}

// ReceivableMsat returns an estimate of the total we could receive through the
// channel with given scid.
func (l *Client) ReceivableMsat(scid string) (uint64, error) {
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
			if err = l.checkChannel(ch); err != nil {
				return 0, err
			}
			maxHtlcAmtMsat, err := l.getMaxHtlcAmtMsat(ch.ChanId, ch.GetRemotePubkey())
			if err != nil {
				return 0, err
			}
			receivable := (uint64(ch.GetRemoteBalance()) -
				ch.GetRemoteConstraints().GetChanReserveSat()*1000)
			// since the max htlc limit is not always set reliably,
			// the check is skipped if it is not set.
			if maxHtlcAmtMsat == 0 {
				return receivable, nil
			}
			return min(maxHtlcAmtMsat, receivable), nil
		}
	}
	return 0, fmt.Errorf("could not find a channel with scid: %s", scid)
}

// checkChannel checks that a channel channel peer is connected and that the
// channel is active.
func (l *Client) checkChannel(ch *lnrpc.Channel) error {
	if !ch.Active {
		return fmt.Errorf("channel not active")
	}
	if !l.isPeerConnected(ch.RemotePubkey) {
		return fmt.Errorf("peer is not connected")
	}
	return nil
}

// isPeerConnected returns `true` if the peer can be found in `listpeers`.
func (l *Client) isPeerConnected(pubkey string) bool {
	r, err := l.lndClient.ListPeers(context.Background(), &lnrpc.ListPeersRequest{})
	if err != nil {
		return false
	}
	for _, peer := range r.Peers {
		if peer.PubKey == pubkey {
			return true
		}
	}
	return false

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

// PayInvoiceViaChannel ensures that the invoice is payed via the direct channel
// to the peer. It takes the desired channel as the enforced route and uses the
// `SendToRouteSync` api for a direct payment via this route.
func (l *Client) PayInvoiceViaChannel(payreq, scid string) (preimage string, err error) {
	decoded, err := l.lndClient.DecodePayReq(l.ctx, &lnrpc.PayReqString{PayReq: payreq})
	if err != nil {
		return "", err
	}

	channel, err := l.CheckChannel(scid, uint64(decoded.NumSatoshis))
	if err != nil {
		return "", err
	}

	v, err := route.NewVertexFromStr(channel.GetRemotePubkey())
	if err != nil {
		return "", err
	}
	route, err := l.routerClient.BuildRoute(context.Background(), &routerrpc.BuildRouteRequest{
		AmtMsat:        decoded.NumMsat,
		FinalCltvDelta: int32(decoded.CltvExpiry),
		OutgoingChanId: channel.GetChanId(),
		HopPubkeys:     [][]byte{v[:]},
	})
	if err != nil {
		return "", err
	}
	if decoded.GetPaymentAddr() != nil {
		route.GetRoute().GetHops()[0].MppRecord = &lnrpc.MPPRecord{
			PaymentAddr:  decoded.GetPaymentAddr(),
			TotalAmtMsat: decoded.NumMsat,
		}
	}
	rHash, err := hex.DecodeString(decoded.PaymentHash)
	if err != nil {
		return "", err
	}
	res, err := l.lndClient.SendToRouteSync(context.Background(), &lnrpc.SendToRouteRequest{
		PaymentHash: rHash,
		Route:       route.GetRoute(),
	})
	if err != nil {
		return "", err
	}
	if res.PaymentError != "" {
		return "", fmt.Errorf("received payment error: %v", res.PaymentError)
	}
	return hex.EncodeToString(res.PaymentPreimage), nil
}

func (l *Client) RebalancePayment(payreq string, channelId string) (preimage string, err error) {
	return l.PayInvoiceViaChannel(payreq, channelId)
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

// ProbePayment trying to pay via a route with a random payment hash
// that the receiver doesn't have the preimage of.
// The receiver node aren't able to settle the payment.
// When the probe is successful, the receiver will return
// a incorrect_or_unknown_payment_details error to the sender.
func (l *Client) ProbePayment(scid string, amountMsat uint64) (bool, string, error) {
	chsRes, err := l.lndClient.ListChannels(context.Background(), &lnrpc.ListChannelsRequest{})
	if err != nil {
		return false, "", fmt.Errorf("ListChannels() %w", err)
	}
	var channel *lnrpc.Channel
	for _, ch := range chsRes.GetChannels() {
		channelShortId := lnwire.NewShortChanIDFromInt(ch.ChanId)
		if channelShortId.String() == lightning.Scid(scid).LndStyle() {
			channel = ch
		}
	}
	if channel.GetChanId() == 0 {
		return false, "", fmt.Errorf("could not find a channel with scid: %s", scid)
	}
	v, err := route.NewVertexFromStr(channel.GetRemotePubkey())
	if err != nil {
		return false, "", fmt.Errorf("NewVertexFromStr() %w", err)
	}

	route, err := l.routerClient.BuildRoute(context.Background(), &routerrpc.BuildRouteRequest{
		AmtMsat:        int64(amountMsat),
		FinalCltvDelta: 9,
		OutgoingChanId: channel.GetChanId(),
		HopPubkeys:     [][]byte{v[:]},
	})
	if err != nil {
		return false, "", fmt.Errorf("BuildRoute() %w", err)
	}
	preimage, err := lightning.GetPreimage()
	if err != nil {
		return false, "", fmt.Errorf("GetPreimage() %w", err)
	}
	pHash, err := hex.DecodeString(preimage.Hash().String())
	if err != nil {
		return false, "", fmt.Errorf("DecodeString() %w", err)
	}

	res2, err := l.lndClient.SendToRouteSync(context.Background(), &lnrpc.SendToRouteRequest{
		PaymentHash: pHash,
		Route:       route.GetRoute(),
	})
	if err != nil {
		return false, "", fmt.Errorf("SendToRouteSync() %w", err)
	}
	if !strings.Contains(res2.PaymentError, "IncorrectOrUnknownPaymentDetails") {
		log.Debugf("send pay would be failed. reason:%w", res2.PaymentError)
		return false, res2.PaymentError, nil
	}
	return true, "", nil
}

func LndShortChannelIdToCLShortChannelId(lndCI lnwire.ShortChannelID) string {
	return fmt.Sprintf("%dx%dx%d", lndCI.BlockHeight, lndCI.TxIndex, lndCI.TxPosition)
}
