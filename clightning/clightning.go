package clightning

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	log2 "log"
	"os"
	"time"

	"github.com/elementsproject/peerswap/log"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/elementsproject/glightning/gbitcoin"
	"github.com/elementsproject/peerswap/onchain"

	"github.com/elementsproject/glightning/gelements"
	"github.com/elementsproject/glightning/glightning"
	"github.com/elementsproject/glightning/jrpc2"
	"github.com/elementsproject/peerswap/lightning"
	"github.com/elementsproject/peerswap/messages"
	"github.com/elementsproject/peerswap/poll"
	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/wallet"
)

// ClnMaxPaymentSizeMsat is the max amount in msat that core-lightning will send
// in a single htlc if `large-channels` are not enabled. The amount is
// 2^32 msat.
//
// FIXME: This should be removed some time soon in cln and we can remove it here
// then also.
const ClnMaxPaymentSizeMsat uint64 = 4294967296

var methods = []peerswaprpcMethod{
	//&ListNodes{}, we disable finding nodes with the featurebit for now, as you would only find clightning nodes
	&ListPeers{},
	&LiquidSendToAddress{},
	&GetSwap{},
	&ListActiveSwaps{},
	&AllowSwapRequests{},
	&AddPeer{},
	&RemovePeer{},
	&AddSuspiciousPeer{},
	&RemoveSuspiciousPeer{},
	&SwapIn{},
	&SwapOut{},
	&ListSwaps{},
	&LiquidGetAddress{},
	&LiquidGetBalance{},
	&ReloadPolicyFile{},
	&GetRequestedSwaps{},
	&ListConfig{},
}

var devmethods = []peerswaprpcMethod{}

const featureBit = 69

var ErrWaitingForReady = fmt.Errorf("peerswap is still in the process of starting up")

type SendPayPartWaiter interface {
	SendPayPartAndWait(paymentRequest string, bolt11 *glightning.DecodedBolt11, amountMsat uint64, channel string, label string, partId uint64) (*glightning.SendPayFields, error)
}

// ClightningClient is the main driver behind c-lightnings plugins system
// it handles rpc calls and messages
type ClightningClient struct {
	version    string
	glightning *glightning.Lightning
	Plugin     *glightning.Plugin

	liquidWallet   *wallet.ElementsRpcWallet
	swaps          *swap.SwapService
	requestedSwaps *swap.RequestedSwapsPrinter
	policy         PolicyReloader
	pollService    *poll.Service

	Gelements *gelements.Elements

	gbitcoin       *gbitcoin.Bitcoin
	bitcoinChain   *onchain.BitcoinOnChain
	bitcoinNetwork *chaincfg.Params

	msgHandlers     []func(peerId string, messageType string, payload []byte) error
	paymenthandlers []func(swapId string, invoiceType swap.InvoiceType)
	initChan        chan interface{}
	nodeId          string
	hexToIdMap      map[string]string

	ctx context.Context

	isReady bool

	peerswapConfig PeerswapClightningConfig
}

// Version returns the version of the core-lightning node, as reported
// by `getinfo`.
func (cl *ClightningClient) Version() string {
	return cl.version
}

func (cl *ClightningClient) SetReady() {
	cl.isReady = true
}

func (cl *ClightningClient) AddPaymentCallback(f func(swapId string, invoiceType swap.InvoiceType)) {
	cl.paymenthandlers = append(cl.paymenthandlers, f)
}

func (cl *ClightningClient) AddPaymentNotifier(swapId string, payreq string, invoiceType swap.InvoiceType) {
	go func() {
		// WaitInvoice is a blocking call that returns as soon as an invoice is
		// either paid or expired.
		res, err := cl.glightning.WaitInvoice(getLabel(swapId, invoiceType))
		if err != nil {
			log.Infof("[Payment Notifier] Error %v, swap %s", err, swapId)
			return
		}

		switch res.Status {
		case "paid":
			for _, handler := range cl.paymenthandlers {
				go handler(swapId, invoiceType)
			}
			return
		case "expired":
			return
		default:
			log.Debugf("Payment notifier received an unexpected status: %v", res.Status)
			return
		}
	}()
}

// NewClightningClient returns a new clightning cl and channel which get closed when the Plugin is initialized
func NewClightningClient(ctx context.Context) (*ClightningClient, <-chan interface{}, error) {
	cl := &ClightningClient{ctx: ctx}
	cl.Plugin = glightning.NewPlugin(cl.onInit)
	err := cl.Plugin.RegisterHooks(&glightning.Hooks{
		CustomMsgReceived: cl.OnCustomMsg,
	})
	if err != nil {
		return nil, nil, err
	}
	cl.Plugin.SubscribeConnect(cl.OnConnect)

	cl.glightning = glightning.NewLightning()
	cl.glightning.SetTimeout(40)

	// we disable feature bit for now as lnd does not support it anyway
	//b := big.NewInt(0)
	//b = b.Exp(big.NewInt(2), big.NewInt(featureBit), nil)
	//cl.Plugin.AddNodeFeatures(b.Bytes())
	cl.Plugin.SetDynamic(true)
	cl.initChan = make(chan interface{})
	cl.hexToIdMap = make(map[string]string)
	return cl, cl.initChan, nil
}

// CanSpend checks if an `amtMsat` can be spend. It returns an error if the
// amount is larger than the `ClnMaxPaymentSizeMsat` if the option
// `--large-channels` is missing or set to false.
func (cl *ClightningClient) CanSpend(amtMsat uint64) error {
	if amtMsat > ClnMaxPaymentSizeMsat {
		var has_large_channel bool
		cfg, err := cl.glightning.ListConfigs()
		if err != nil {
			return err
		}
		if _, ok := cfg["large-channels"]; ok {
			// Found the config option, read field
			var lc struct {
				LargeChannels bool `json:"large-channels"`
			}

			jstring, _ := json.Marshal(cfg)
			json.Unmarshal(jstring, &lc)
			has_large_channel = lc.LargeChannels
		}

		if !has_large_channel {
			return fmt.Errorf("swap amount is %d: need to enable option '--large-channels' to swap amounts larger than 2^32 msat", amtMsat)
		}
	}
	return nil
}

// Implementation returns the name of the lightning network client
// implementation.
func (cl *ClightningClient) Implementation() string {
	return "CLN"
}

// ListPeerChannelsRequest is a glightning jrpc2 method to call
// `listpeerchannels`.
type ListPeerChannelsRequest struct {
	// Supplying id will filter the results to only return channel data that
	// match id, if one exists.
	Id string `json:"id,omitempty"`
}

func (r ListPeerChannelsRequest) Name() string {
	return "listpeerchannels"
}

// ListPeerChannelsResponse is the response to the `listpeerchannels` rpc
// method. This struct is incomplete, see
// `https://docs.corelightning.org/reference/lightning-listpeerchannels` for a
// full list of fields that are returned.
type ListPeerChannelsResponse struct {
	Channels []PeerChannel `json:"channels"`
}

type PeerChannel struct {
	PeerId           string            `json:"peer_id"`
	PeerConnected    bool              `json:"peer_connected"`
	State            string            `json:"state"`
	ShortChannelId   string            `json:"short_channel_id,omitempty"`
	TotalMsat        glightning.Amount `json:"total_msat,omitempty"`
	ToUsMsat         glightning.Amount `json:"to_us_msat,omitempty"`
	ReceivableMsat   glightning.Amount `json:"receivable_msat,omitempty"`
	SpendableMsat    glightning.Amount `json:"spendable_msat,omitempty"`
	TheirReserveMsat glightning.Amount `json:"their_reserve_msat,omitempty"`
	OurReserveMsat   glightning.Amount `json:"our_reserve_msat,omitempty"`
}

func (ch *PeerChannel) GetSpendableMsat() uint64 {
	if ch.SpendableMsat.MSat() > 0 {
		return ch.SpendableMsat.MSat()
	} else {
		return ch.ToUsMsat.MSat() - ch.OurReserveMsat.MSat()
	}
}

func (ch *PeerChannel) GetReceivableMsat() uint64 {
	if ch.ReceivableMsat.MSat() > 0 {
		return ch.ReceivableMsat.MSat()
	} else {
		return ch.TotalMsat.MSat() - ch.ToUsMsat.MSat() - ch.TheirReserveMsat.MSat()
	}
}

func (cl *ClightningClient) getMaxHtlcAmtMsat(scid, nodeId string) (uint64, error) {
	var htlcMaximumMilliSatoshis uint64
	chgs, err := cl.glightning.GetChannel(scid)
	if err != nil {
		return htlcMaximumMilliSatoshis, nil
	}
	for _, c := range chgs {
		if c.Source == nodeId {
			htlcMaximumMilliSatoshis = c.HtlcMaximumMilliSatoshis.MSat()
		}
	}
	return htlcMaximumMilliSatoshis, nil
}

func min(x, y uint64) uint64 {
	if x < y {
		return x
	}
	return y
}

// SpendableMsat returns an estimate of the total we could send through the
// channel with given scid. Falls back to the owned amount in the channel.
func (cl *ClightningClient) SpendableMsat(scid string) (uint64, error) {
	scid = lightning.Scid(scid).ClnStyle()
	var res ListPeerChannelsResponse
	err := cl.glightning.Request(ListPeerChannelsRequest{}, &res)
	if err != nil {
		return 0, err
	}
	for _, ch := range res.Channels {
		if ch.ShortChannelId == scid {
			if err = cl.checkChannel(ch); err != nil {
				return 0, err
			}
			maxHtlcAmtMsat, err := cl.getMaxHtlcAmtMsat(scid, cl.nodeId)
			if err != nil {
				return 0, err
			}
			// since the max htlc limit is not always set reliably,
			// the check is skipped if it is not set.
			if maxHtlcAmtMsat == 0 {
				return ch.GetSpendableMsat(), nil
			}
			return min(maxHtlcAmtMsat, ch.GetSpendableMsat()), nil
		}
	}
	return 0, fmt.Errorf("could not find a channel with scid: %s", scid)
}

// ReceivableMsat returns an estimate of the total we could receive through the
// channel with given scid.
func (cl *ClightningClient) ReceivableMsat(scid string) (uint64, error) {
	scid = lightning.Scid(scid).ClnStyle()
	var res ListPeerChannelsResponse
	err := cl.glightning.Request(ListPeerChannelsRequest{}, &res)
	if err != nil {
		return 0, err
	}
	for _, ch := range res.Channels {
		if ch.ShortChannelId == scid {
			if err = cl.checkChannel(ch); err != nil {
				return 0, err
			}
			maxHtlcAmtMsat, err := cl.getMaxHtlcAmtMsat(scid, ch.PeerId)
			if err != nil {
				return 0, err
			}
			// since the max htlc limit is not always set reliably,
			// the check is skipped if it is not set.
			if maxHtlcAmtMsat == 0 {
				return ch.GetReceivableMsat(), nil
			}
			return min(maxHtlcAmtMsat, ch.GetReceivableMsat()), nil
		}
	}
	return 0, fmt.Errorf("could not find a channel with scid: %s", scid)
}

// checkChannel performs a set of sanity checks id the channel is eligible for
// a swap of amtSat
func (cl *ClightningClient) checkChannel(ch PeerChannel) error {
	if !ch.PeerConnected {
		return fmt.Errorf("channel peer is not connected")
	}
	if ch.State != "CHANNELD_NORMAL" {
		return fmt.Errorf("channel not in normal operation mode: %s", ch.State)
	}
	return nil
}

// GetNodeId returns the lightning nodes pubkey
func (cl *ClightningClient) GetNodeId() string {
	return cl.nodeId
}

// GetPreimage returns a random preimage
func (cl *ClightningClient) GetPreimage() (lightning.Preimage, error) {
	var preimage lightning.Preimage

	if _, err := rand.Read(preimage[:]); err != nil {
		return preimage, err
	}
	return preimage, nil
}

// SetupClients injects the required services
func (cl *ClightningClient) SetupClients(liquidWallet *wallet.ElementsRpcWallet,
	swaps *swap.SwapService,
	policy PolicyReloader, requestedSwaps *swap.RequestedSwapsPrinter, elements *gelements.Elements,
	bitcoin *gbitcoin.Bitcoin, bitcoinChain *onchain.BitcoinOnChain, pollService *poll.Service) {
	cl.liquidWallet = liquidWallet
	cl.requestedSwaps = requestedSwaps
	cl.swaps = swaps
	cl.Gelements = elements
	cl.policy = policy
	cl.gbitcoin = bitcoin
	cl.pollService = pollService
	cl.bitcoinChain = bitcoinChain
	if cl.bitcoinChain != nil {
		cl.bitcoinNetwork = bitcoinChain.GetChain()
	}
}

// SetPeerswapConfig injects the peerswap config that is used during runtime.
// This config is just used for a console print.
func (cl *ClightningClient) SetPeerswapConfig(config *Config) {
	cl.peerswapConfig = PeerswapClightningConfig{
		BitcoinRpcUser:         config.Bitcoin.RpcUser,
		BitcoinRpcPassword:     config.Bitcoin.RpcPassword,
		BitcoinRpcPasswordFile: config.Bitcoin.RpcPasswordFile,
		BitcoinRpcHost:         config.Bitcoin.RpcHost,
		BitcoinRpcPort:         config.Bitcoin.RpcPort,
		BitcoinCookieFilePath:  config.Bitcoin.RpcPasswordFile,
		LiquidRpcUser:          config.Liquid.RpcUser,
		LiquidRpcPassword:      config.Liquid.RpcPassword,
		LiquidRpcPasswordFile:  config.Liquid.RpcPasswordFile,
		LiquidRpcHost:          config.Liquid.RpcHost,
		LiquidRpcPort:          config.Liquid.RpcPort,
		LiquidRpcWallet:        config.Liquid.RpcWallet,
		LiquidDisabled:         *config.Liquid.LiquidSwaps,
		PeerswapDir:            config.PeerswapDir,
	}
}

func (cl *ClightningClient) GetLightningRpc() *glightning.Lightning {
	return cl.glightning
}

// Start starts the Plugin
func (cl *ClightningClient) Start() error {
	return cl.Plugin.Start(os.Stdin, os.Stdout)
}

// SendMessage sends a hexmessage to a peer
func (cl *ClightningClient) SendMessage(peerId string, message []byte, messageType int) error {
	msg := messages.MessageTypeToHexString(messages.MessageType(messageType)) + hex.EncodeToString(message)
	res, err := cl.glightning.SendCustomMessage(peerId, msg)
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return errors.New(res.Message)
	}
	return nil
}

// OnCustomMsg is the hook that c-lightning calls
func (cl *ClightningClient) OnCustomMsg(event *glightning.CustomMsgReceivedEvent) (*glightning.CustomMsgReceivedResponse, error) {
	typeString := event.Payload[:4]
	payload := event.Payload[4:]
	payloadDecoded, err := hex.DecodeString(payload)
	if err != nil {
		log.Debugf("[Messenger] error decoding payload %v", err)
		return event.Continue(), nil
	}
	for _, v := range cl.msgHandlers {
		err := v(event.PeerId, typeString, payloadDecoded)
		// We silence logging on AlreadyExistsErrors as this is just spammy
		// and we already log that we received a message of the same type
		// earlier.
		if err != nil && !errors.Is(err, swap.AlreadyExistsError) {
			log.Debugf("\n msghandler err: %v", err)
		}
	}
	return event.Continue(), nil
}

// AddMessageHandler adds a listener for incoming peermessages
func (cl *ClightningClient) AddMessageHandler(f func(peerId string, msgType string, payload []byte) error) {
	cl.msgHandlers = append(cl.msgHandlers, f)
}

// GetPayreq returns a Bolt11 Invoice
func (cl *ClightningClient) GetPayreq(amountMsat uint64, preImage string, swapId string, memo string, invoiceType swap.InvoiceType, expirySeconds, expiryCltv uint64) (string, error) {
	res, err := cl.glightning.CreateInvoiceWithCltvExpiry(amountMsat, getLabel(swapId, invoiceType), memo, uint32(expirySeconds), []string{}, preImage, false, uint32(expiryCltv))
	if err != nil {
		return "", err
	}
	return res.Bolt11, nil
}

func getLabel(swapId string, invoiceType swap.InvoiceType) string {
	return fmt.Sprintf("%s_%s", swapId, invoiceType)
}

// DecodePayreq decodes a Bolt11 Invoice
func (cl *ClightningClient) DecodePayreq(payreq string) (paymentHash string, amountMsat uint64, expiry int64, err error) {
	res, err := cl.glightning.DecodeBolt11(payreq)
	if err != nil {
		return "", 0, 0, err
	}
	return res.PaymentHash, res.AmountMsat.MSat(), int64(res.MinFinalCltvExpiry), nil
}

// PayInvoice tries to pay a Bolt11 Invoice
func (cl *ClightningClient) PayInvoice(payreq string) (preimage string, err error) {
	res, err := cl.glightning.Pay(&glightning.PayRequest{Bolt11: payreq})
	if err != nil {
		return "", err
	}
	return res.PaymentPreimage, nil
}

// PayInvoiceViaChannel ensures that the invoice is payed via the direct channel
// to the peer. It takes the desired channel as the enforced route and uses the
// `sendpay` api for a direct payment via this route.
func (cl *ClightningClient) PayInvoiceViaChannel(payreq string, scid string) (preimage string, err error) {
	bolt11, err := cl.glightning.DecodeBolt11(payreq)
	if err != nil {
		return "", err
	}

	label := randomString()

	// We have to ensure that the `short_channel_id` is divided by `x`es.
	cid := lightning.Scid(scid)
	scid = cid.ClnStyle()

	_, err = cl.glightning.SendPay(
		[]glightning.RouteHop{
			{
				Id:             bolt11.Payee,
				ShortChannelId: scid,
				AmountMsat:     bolt11.AmountMsat,
				Delay:          uint32(bolt11.MinFinalCltvExpiry + 1),
				Direction:      0,
			},
		},
		bolt11.PaymentHash,
		label,
		bolt11.AmountMsat.MSat(),
		payreq,
		bolt11.PaymentSecret,
		0,
	)
	if err != nil {
		return "", err
	}

	res, err := cl.glightning.WaitSendPay(bolt11.PaymentHash, 0)
	if err != nil {
		return "", err
	}

	preimage = res.PaymentPreimage
	return preimage, nil
}

// RebalancePayment handles the lightning payment that should re-balance the
// channel.
func (cl *ClightningClient) RebalancePayment(payreq string, channel string) (preimage string, err error) {
	return cl.PayInvoiceViaChannel(payreq, channel)
}

// isPeerConnected returns true if the peer is connected to the cln node.
func (cl *ClightningClient) isPeerConnected(nodeId string) bool {
	peer, err := cl.glightning.GetPeer(nodeId)
	if err != nil {
		log.Infof("Could not get peer: %v", err)
		return false
	}
	return peer.Connected
}

// peerRunsPeerSwap returns true if the peer with peerId is listed in the
// pollService.
func (cl *ClightningClient) peerRunsPeerSwap(peerId string) bool {
	pollInfo, err := cl.pollService.GetPollFrom(peerId)
	if err == nil && pollInfo != nil {
		return true
	}
	return false
}

// This is called after the Plugin starts up successfully
func (cl *ClightningClient) onInit(plugin *glightning.Plugin, options map[string]glightning.Option, config *glightning.Config) {
	cl.glightning.StartUp(config.RpcFile, config.LightningDir)

	getInfo, err := cl.glightning.GetInfo()
	if err != nil {
		log2.Fatalf("getinfo err %v", err)
	}
	cl.nodeId = getInfo.Id
	cl.version = getInfo.Version
	cl.initChan <- true
}

// OnConnect is called after the connect event. The
// handler sends out a poll to the peer it connected
// to.
func (cl *ClightningClient) OnConnect(connectEvent *glightning.ConnectEvent) {
	go func() {
		for {
			time.Sleep(10 * time.Second)
			if cl.pollService != nil {
				cl.pollService.RequestPoll(connectEvent.PeerId)
				return
			}
		}
	}()
}

// RegisterMethods registeres rpc methods to c-lightning
func (cl *ClightningClient) RegisterMethods() error {
	for _, v := range methods {
		method := v.Get(cl)
		glightningMethod := &glightning.RpcMethod{
			Method:   method,
			Desc:     v.Description(),
			LongDesc: v.LongDescription(),
			Category: "peerswap",
		}
		err := cl.Plugin.RegisterMethod(glightningMethod)
		if err != nil {
			return err
		}
	}
	for _, v := range devmethods {
		method := v.Get(cl)
		glightningMethod := glightning.NewRpcMethod(method, "dev")
		glightningMethod.Category = "peerswap"
		glightningMethod.Desc = v.Description()
		glightningMethod.LongDesc = v.LongDescription()
		err := cl.Plugin.RegisterMethod(glightningMethod)
		if err != nil {
			return err
		}
	}
	return nil
}

type peerswaprpcMethod interface {
	Get(*ClightningClient) jrpc2.ServerMethod
	Description() string
	LongDescription() string
}

func (cl *ClightningClient) GetPeers() []string {
	peers, err := cl.glightning.ListPeers()
	if err != nil {
		log.Debugf("could not listpeers: %v", err)
		return nil
	}

	var peerlist []string
	for _, peer := range peers {
		if peer.Connected {
			peerlist = append(peerlist, peer.Id)
		}
	}
	return peerlist
}

// ProbePayment trying to pay via a route with a random payment hash
// that the receiver doesn't have the preimage of.
// The receiver node aren't able to settle the payment.
// When the probe is successful, the receiver will return
// a incorrect_or_unknown_payment_details error to the sender.
func (cl *ClightningClient) ProbePayment(scid string, amountMsat uint64) (bool, string, error) {
	var res ListPeerChannelsResponse
	err := cl.glightning.Request(ListPeerChannelsRequest{}, &res)
	if err != nil {
		return false, "", fmt.Errorf("ListPeerChannelsRequest() %w", err)
	}
	var channel PeerChannel
	for _, ch := range res.Channels {
		if ch.ShortChannelId == lightning.Scid(scid).ClnStyle() {
			if err := cl.checkChannel(ch); err != nil {
				return false, "", err
			}
			channel = ch
		}
	}

	route, err := cl.glightning.GetRoute(channel.PeerId, amountMsat, 1, 0, cl.nodeId, 0, nil, 1)
	if err != nil {
		return false, "", fmt.Errorf("GetRoute() %w", err)
	}
	preimage, err := lightning.GetPreimage()
	if err != nil {
		return false, "", fmt.Errorf("GetPreimage() %w", err)
	}
	paymentHash := preimage.Hash().String()
	_, err = cl.glightning.SendPay(
		route,
		paymentHash,
		"",
		amountMsat,
		"",
		"",
		0,
	)
	if err != nil {
		return false, "", fmt.Errorf("SendPay() %w", err)
	}
	_, err = cl.glightning.WaitSendPay(paymentHash, 0)
	if err != nil {
		pe, ok := err.(*glightning.PaymentError)
		if !ok {
			return false, "", fmt.Errorf("WaitSendPay() %w", err)
		}
		faiCodeWireIncorrectOrUnknownPaymentDetails := 203
		if pe.RpcError.Code != faiCodeWireIncorrectOrUnknownPaymentDetails {
			log.Debugf("send pay would be failed. reason:%w", err)
			return false, pe.Error(), nil
		}
	}
	return true, "", nil
}

type Glightninglogger struct {
	plugin *glightning.Plugin
}

func NewGlightninglogger(plugin *glightning.Plugin) *Glightninglogger {
	return &Glightninglogger{plugin: plugin}
}

func (g *Glightninglogger) Infof(format string, v ...interface{}) {
	g.plugin.Log(fmt.Sprintf(format, v...), glightning.Info)
}

func (g *Glightninglogger) Debugf(format string, v ...interface{}) {
	g.plugin.Log(fmt.Sprintf(format, v...), glightning.Debug)
}
