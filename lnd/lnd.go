package lnd

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"sync"
	"time"

	"github.com/elementsproject/peerswap/log"

	"github.com/elementsproject/peerswap/lightning"
	"github.com/elementsproject/peerswap/messages"
	"github.com/elementsproject/peerswap/onchain"
	"github.com/elementsproject/peerswap/poll"
	"github.com/elementsproject/peerswap/swap"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/invoicesrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	"github.com/lightningnetwork/lnd/lnrpc/walletrpc"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/macaroons"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"
)

type Lnd struct {
	lndClient      lnrpc.LightningClient
	walletClient   walletrpc.WalletKitClient
	routerClient   routerrpc.RouterClient
	invoicesClient invoicesrpc.InvoicesClient

	PollService    *poll.Service
	bitcoinOnChain *onchain.BitcoinOnChain

	cc  *grpc.ClientConn
	ctx context.Context

	messageHandler       []func(peerId string, msgType string, payload []byte) error
	paymentCallback      []func(swapId string, invoiceType swap.InvoiceType)
	invoiceSubscriptions map[string]interface{}
	pubkey               string

	sync.Mutex
}

func (l *Lnd) AddPaymentNotifier(swapId string, payreq string, invoiceType swap.InvoiceType) (alreadyPaid bool) {
	invoice, err := l.lndClient.DecodePayReq(l.ctx, &lnrpc.PayReqString{PayReq: payreq})
	if err != nil {
		log.Infof("decode invoice error")
		return false
	}
	rHash, err := hex.DecodeString(invoice.PaymentHash)
	if err != nil {
		log.Infof("decode rhash error")
		return false
	}
	lookup, err := l.lndClient.LookupInvoice(l.ctx, &lnrpc.PaymentHash{RHash: rHash})
	if err != nil && (lookup.State == lnrpc.Invoice_SETTLED && lookup.AmtPaidSat == invoice.NumSatoshis) {
		return true
	}
	// Check if service is already subscribed
	if HasInvoiceSubscribtion(l.invoiceSubscriptions, invoice.PaymentHash) {
		return false
	}

	go func() {

		// add subscribtion
		AddInvoiceSubscription(l, l.invoiceSubscriptions, invoice.PaymentHash)
		defer RemoveInvoiceSubscribtion(l, l.invoiceSubscriptions, invoice.PaymentHash)

		for {
			select {
			case <-l.ctx.Done():
				log.Debugf("context done")
				return
			default:
				stream, err := l.invoicesClient.SubscribeSingleInvoice(l.ctx, &invoicesrpc.SubscribeSingleInvoiceRequest{RHash: rHash})
				if err == nil {
					inv, err := stream.Recv()
					if err == nil {
						switch inv.State {
						case lnrpc.Invoice_SETTLED:
							for _, v := range l.paymentCallback {
								if inv.AmtPaidSat == invoice.NumSatoshis {
									go v(swapId, invoiceType)
								}
							}
							return
						case lnrpc.Invoice_CANCELED:
							return
						}
					}

				}
				time.Sleep(100 * time.Millisecond)

			}
		}
	}()
	return false
}

func (l *Lnd) DecodePayreq(payreq string) (paymentHash string, amountMsat uint64, err error) {
	decoded, err := l.lndClient.DecodePayReq(l.ctx, &lnrpc.PayReqString{PayReq: payreq})
	if err != nil {
		return "", 0, err
	}
	return decoded.PaymentHash, uint64(decoded.NumMsat), nil
}

func (l *Lnd) PayInvoice(payreq string) (preImage string, err error) {
	payres, err := l.lndClient.SendPaymentSync(l.ctx, &lnrpc.SendRequest{PaymentRequest: payreq})
	if err != nil {
		return "", nil
	}
	return hex.EncodeToString(payres.PaymentPreimage), nil
}

func (l *Lnd) CheckChannel(shortChannelId string, amountSat uint64) (*lnrpc.Channel, error) {
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

func (l *Lnd) GetPayreq(msatAmount uint64, preimageString string, swapId string, invoiceType swap.InvoiceType, expiry uint64) (string, error) {
	preimage, err := lightning.MakePreimageFromStr(preimageString)
	if err != nil {
		return "", err
	}

	payreq, err := l.lndClient.AddInvoice(l.ctx, &lnrpc.Invoice{
		ValueMsat:  int64(msatAmount),
		Memo:       fmt.Sprintf("%s_%s", swapId, invoiceType),
		RPreimage:  preimage[:],
		Expiry:     int64(expiry),
		CltvExpiry: 144,
	})
	if err != nil {
		return "", err
	}
	return payreq.PaymentRequest, nil
}

func (l *Lnd) AddPaymentCallback(f func(swapId string, invoiceType swap.InvoiceType)) {
	l.paymentCallback = append(l.paymentCallback, f)
}

func (l *Lnd) RebalancePayment(payreq string, channelId string) (preimage string, err error) {
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

func (l *Lnd) SendMessage(peerId string, message []byte, messageType int) error {
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

func (l *Lnd) AddMessageHandler(f func(peerId string, msgType string, payload []byte) error) {
	l.messageHandler = append(l.messageHandler, f)
}

func (l *Lnd) PrepareOpeningTransaction(address string, amount uint64) (txId string, txHex string, err error) {
	return "", "", nil
}

func (l *Lnd) StartListening() {
	go l.startListenMessages()
	go l.startListenPeerEvents()
}

func (l *Lnd) startListenMessages() {
	err := l.listenMessages()
	if err != nil {
		log.Infof("error listening on messages %v, restarting", err)
		l.startListenMessages()
	}
}

func (l *Lnd) startListenPeerEvents() {
	err := l.listenPeerEvents()
	if err != nil {
		log.Infof("error listening on peer events %v", err)
		l.startListenPeerEvents()
	}
}

func (l *Lnd) GetPeers() []string {
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

func (l *Lnd) listenMessages() error {
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

func (l *Lnd) listenPeerEvents() error {
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

func (l *Lnd) handleCustomMessage(msg *lnrpc.CustomMessage) error {
	peerId := hex.EncodeToString(msg.Peer)
	for _, v := range l.messageHandler {
		err := v(peerId, messages.MessageTypeToHexString(messages.MessageType(msg.Type)), msg.Data)
		if err != nil {
			log.Infof("\n Custom Message Handler err: %v", err)
		}
	}
	return nil
}

func AddInvoiceSubscription(lock sync.Locker, subMap map[string]interface{}, rHash string) {
	lock.Lock()
	defer lock.Unlock()
	subMap[rHash] = ""
}

func RemoveInvoiceSubscribtion(lock sync.Locker, subMap map[string]interface{}, rHash string) {
	lock.Lock()
	defer lock.Unlock()
	delete(subMap, rHash)
}

func HasInvoiceSubscribtion(subMap map[string]interface{}, rHash string) bool {
	_, ok := subMap[rHash]
	return ok
}

func NewLnd(ctx context.Context, tlsCertPath, macaroonPath, address string, chain *onchain.BitcoinOnChain) (*Lnd, error) {
	cc, err := getClientConnection(ctx, tlsCertPath, macaroonPath, address)
	if err != nil {
		return nil, err
	}
	lndClient := lnrpc.NewLightningClient(cc)
	walletClient := walletrpc.NewWalletKitClient(cc)
	routerClient := routerrpc.NewRouterClient(cc)
	invoicesClient := invoicesrpc.NewInvoicesClient(cc)

	gi, err := lndClient.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		return nil, err
	}
	return &Lnd{
		lndClient:            lndClient,
		walletClient:         walletClient,
		routerClient:         routerClient,
		invoicesClient:       invoicesClient,
		bitcoinOnChain:       chain,
		cc:                   cc,
		ctx:                  ctx,
		pubkey:               gi.IdentityPubkey,
		invoiceSubscriptions: make(map[string]interface{}),
	}, nil
}

func getClientConnection(ctx context.Context, tlsCertPath, macaroonPath, address string) (*grpc.ClientConn, error) {
	maxMsgRecvSize := grpc.MaxCallRecvMsgSize(1 * 1024 * 1024 * 500)

	creds, err := credentials.NewClientTLSFromFile(tlsCertPath, "")
	if err != nil {
		return nil, err
	}

	macBytes, err := ioutil.ReadFile(macaroonPath)
	if err != nil {
		return nil, err
	}

	mac := &macaroon.Macaroon{}
	if err := mac.UnmarshalBinary(macBytes); err != nil {
		return nil, err
	}

	cred, err := macaroons.NewMacaroonCredential(mac)
	if err != nil {
		return nil, err
	}

	if err := mac.UnmarshalBinary(macBytes); err != nil {
		return nil, err
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithBlock(),
		grpc.WithPerRPCCredentials(cred),
		grpc.WithDefaultCallOptions(maxMsgRecvSize),
	}
	conn, err := grpc.DialContext(ctx, address, opts...)
	if err != nil {
		return nil, err
	}
	return conn, nil

}

func LndShortChannelIdToCLShortChannelId(lndCI lnwire.ShortChannelID) string {
	return fmt.Sprintf("%dx%dx%d", lndCI.BlockHeight, lndCI.TxIndex, lndCI.TxPosition)
}
