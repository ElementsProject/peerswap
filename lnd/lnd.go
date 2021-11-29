package lnd

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/sputn1ck/peerswap/messages"
	"github.com/sputn1ck/peerswap/onchain"
	"github.com/sputn1ck/peerswap/poll"
	"io/ioutil"
	"log"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	"github.com/lightningnetwork/lnd/lnrpc/walletrpc"
	"github.com/lightningnetwork/lnd/macaroons"
	"github.com/sputn1ck/peerswap/lightning"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"
)

type Lnd struct {
	lndClient    lnrpc.LightningClient
	walletClient walletrpc.WalletKitClient
	routerClient routerrpc.RouterClient

	PollService    *poll.Service
	bitcoinOnChain *onchain.BitcoinOnChain

	cc  *grpc.ClientConn
	ctx context.Context

	messageHandler  []func(peerId string, msgType string, payload []byte) error
	paymentCallback func(paymentLabel string)
	pubkey          string
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

func (l *Lnd) CheckChannel(shortChannelId string, amountSat uint64) error {
	res, err := l.lndClient.ListChannels(l.ctx, &lnrpc.ListChannelsRequest{ActiveOnly: true})
	if err != nil {
		return err
	}
	var channel *lnrpc.Channel
	for _, v := range res.Channels {
		channelShortId := lnwire.NewShortChanIDFromInt(v.ChanId)
		if lndShortChannelIdToCLShortChannelId(channelShortId) == shortChannelId {
			channel = v
			break
		}
	}
	if channel == nil {
		return errors.New("channel not found")
	}
	if channel.LocalBalance < int64(amountSat) {
		return errors.New("not enough outbound capacity to perform swapOut")
	}

	return nil
}

func (l *Lnd) GetPayreq(msatAmount uint64, preimageString string, label string) (string, error) {
	preimage, err := lightning.MakePreimageFromStr(preimageString)
	if err != nil {
		return "", err
	}
	preimageHash := preimage.Hash()
	payreq, err := l.lndClient.AddInvoice(l.ctx, &lnrpc.Invoice{
		ValueMsat: int64(msatAmount),
		Memo:      label,
		RPreimage: preimageHash[:],
	})
	if err != nil {
		return "", err
	}
	return payreq.PaymentRequest, nil
}

func (l *Lnd) AddPaymentCallback(f func(paymentLabel string)) {
	l.paymentCallback = f
}

func (l *Lnd) RebalancePayment(payreq string, channel string) (preimage string, err error) {
	decoded, err := l.lndClient.DecodePayReq(l.ctx, &lnrpc.PayReqString{PayReq: payreq})
	if err != nil {
		return "", err
	}
	err = l.CheckChannel(channel, uint64(decoded.NumSatoshis))
	if err != nil {
		return "", err
	}

	pkBytes, err := hex.DecodeString(decoded.Destination)
	if err != nil {
		return "", err
	}
	route, err := l.routerClient.BuildRoute(l.ctx, &routerrpc.BuildRouteRequest{
		AmtMsat:        decoded.NumMsat,
		OutgoingChanId: 0,
		HopPubkeys:     [][]byte{pkBytes},
	})
	rHash, err := hex.DecodeString(decoded.PaymentHash)
	if err != nil {
		return "", err
	}
	payRes, err := l.routerClient.SendToRouteV2(l.ctx, &routerrpc.SendToRouteRequest{
		PaymentHash: rHash,
		Route:       route.Route,
	})
	if err != nil {
		return "", err
	}
	if payRes.Failure != nil {
		return "", errors.New(fmt.Sprintf("payment failure %s", payRes.Failure.Code))
	}
	return hex.EncodeToString(payRes.Preimage), nil

}

func (l *Lnd) SendMessage(peerId string, message []byte, messageType int) error {
	peerBytes, err := hex.DecodeString(peerId)
	if err != nil {
		return err
	}

	log.Printf("sending message %s %s %v", peerId, hex.EncodeToString(message), messageType)
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

	go func() {
		err := l.listenMessages()
		if err != nil {
			log.Printf("error listening on messages %v", err)
		}
	}()
	go func() {
		err := l.listenPayments()
		if err != nil {
			log.Printf("error listening on payments %v", err)
		}
	}()
	go func() {
		err := l.listenPeerEvents()
		if err != nil {
			log.Printf("error listening on peer events %v", err)
		}
	}()
}

func (l *Lnd) GetPeers() []string {
	res, err := l.lndClient.ListPeers(l.ctx, &lnrpc.ListPeersRequest{})
	if err != nil {
		log.Printf("could not listpeers: %v", err)
		return nil
	}

	var peerlist []string
	for _, peer := range res.Peers {
		peerlist = append(peerlist, peer.PubKey)
	}
	return peerlist
}

func (l *Lnd) listenPayments() error {
	client, err := l.lndClient.SubscribeInvoices(l.ctx, &lnrpc.InvoiceSubscription{})
	if err != nil {
		return err
	}
	for {
		select {
		case _ = <-l.ctx.Done():
			return client.CloseSend()
		default:
			msg, err := client.Recv()
			if err != nil {
				return err
			}
			if msg.State == lnrpc.Invoice_SETTLED {
				l.paymentCallback(msg.Memo)
			}
		}
	}
}

func (l *Lnd) listenMessages() error {
	client, err := l.lndClient.SubscribeCustomMessages(l.ctx, &lnrpc.SubscribeCustomMessagesRequest{})
	if err != nil {
		return err
	}
	for {
		select {
		case _ = <-l.ctx.Done():
			return client.CloseSend()
		default:
			msg, err := client.Recv()
			if err != nil {
				return err
			}

			err = l.handleCustomMessage(msg)
			if err != nil {
				log.Printf("Error handling msg %v", err)
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
		case _ = <-l.ctx.Done():
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
			log.Printf("\n msghandler err: %v", err)
		}
	}
	return nil
}

func NewLnd(ctx context.Context, tlsCertPath, macaroonPath, address string, chain *onchain.BitcoinOnChain) (*Lnd, error) {
	cc, err := getClientConnection(ctx, tlsCertPath, macaroonPath, address)
	if err != nil {
		return nil, err
	}
	lndClient := lnrpc.NewLightningClient(cc)
	walletClient := walletrpc.NewWalletKitClient(cc)
	routerClient := routerrpc.NewRouterClient(cc)

	gi, err := lndClient.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		return nil, err
	}
	return &Lnd{
		lndClient:      lndClient,
		walletClient:   walletClient,
		routerClient:   routerClient,
		bitcoinOnChain: chain,
		cc:             cc,
		ctx:            ctx,
		pubkey:         gi.IdentityPubkey,
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

func lndShortChannelIdToCLShortChannelId(lndCI lnwire.ShortChannelID) string {
	return fmt.Sprintf("%dx%dx%d", lndCI.BlockHeight, lndCI.TxIndex, lndCI.TxPosition)
}
