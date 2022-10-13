package clightning

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	log2 "log"
	"math"
	"os"
	"strings"
	"sync"
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
}

var devmethods = []peerswaprpcMethod{}

const featureBit = 69

var maxPaymentSizeMsat = uint64(math.Pow(2, 32))

var ErrWaitingForReady = fmt.Errorf("peerswap is still in the process of starting up")

type SendPayPartWaiter interface {
	SendPayPartAndWait(paymentRequest string, bolt11 *glightning.DecodedBolt11, amountMsat uint64, channel string, label string, partId uint64) (*glightning.SendPayFields, error)
}

// ClightningClient is the main driver behind c-lightnings plugins system
// it handles rpc calls and messages
type ClightningClient struct {
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
			log.Debugf("[Payment Notifier] Error %v", err)
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

	// we disable feature bit for now as lnd does not support it anyway
	//b := big.NewInt(0)
	//b = b.Exp(big.NewInt(2), big.NewInt(featureBit), nil)
	//cl.Plugin.AddNodeFeatures(b.Bytes())
	cl.Plugin.SetDynamic(true)
	cl.initChan = make(chan interface{})
	cl.hexToIdMap = make(map[string]string)
	return cl, cl.initChan, nil
}

// CheckChannel checks if a channel is eligable for a swap
func (cl *ClightningClient) CheckChannel(channelId string, amountSat uint64) error {
	funds, err := cl.glightning.ListFunds()
	if err != nil {
		return err
	}

	var fundingChannels *glightning.FundingChannel
	for _, v := range funds.Channels {
		if v.ShortChannelId == channelId {
			fundingChannels = v
			break
		}
	}
	if fundingChannels == nil {
		return errors.New("fundingChannels not found")
	}

	if fundingChannels.ChannelSatoshi < amountSat {
		return errors.New("not enough outbound capacity to perform swapOut")
	}
	if !fundingChannels.Connected {
		return errors.New("fundingChannels is not connected")
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
		if err != nil {
			log.Debugf("\n msghandler err: %v", err)
			return event.Continue(), nil
		}
	}
	return event.Continue(), nil
}

// AddMessageHandler adds a listener for incoming peermessages
func (cl *ClightningClient) AddMessageHandler(f func(peerId string, msgType string, payload []byte) error) {
	cl.msgHandlers = append(cl.msgHandlers, f)
}

// GetPayreq returns a Bolt11 Invoice
func (cl *ClightningClient) GetPayreq(amountMsat uint64, preImage string, swapId string, memo string, invoiceType swap.InvoiceType, expiry uint64) (string, error) {
	res, err := cl.glightning.CreateInvoice(amountMsat, getLabel(swapId, invoiceType), memo, uint32(expiry), []string{}, preImage, false)
	if err != nil {
		return "", err
	}
	return res.Bolt11, nil
}

func getLabel(swapId string, invoiceType swap.InvoiceType) string {
	return fmt.Sprintf("%s_%s", swapId, invoiceType)
}

// DecodePayreq decodes a Bolt11 Invoice
func (cl *ClightningClient) DecodePayreq(payreq string) (paymentHash string, amountMsat uint64, err error) {
	res, err := cl.glightning.DecodeBolt11(payreq)
	if err != nil {
		return "", 0, err
	}
	return res.PaymentHash, res.MilliSatoshis, nil
}

// PayInvoice tries to pay a Bolt11 Invoice
func (cl *ClightningClient) PayInvoice(payreq string) (preimage string, err error) {
	res, err := cl.glightning.Pay(&glightning.PayRequest{Bolt11: payreq})
	if err != nil {
		return "", err
	}
	return res.PaymentPreimage, nil
}

// PayInvoiceViaChannel ensures that the invoice is payed via the direct
// channel to the peer.
func (cl *ClightningClient) PayInvoiceViaChannel(payreq string, scid string) (preimage string, err error) {
	bolt11, err := cl.glightning.DecodeBolt11(payreq)
	if err != nil {
		return "", err
	}

	label := randomString()
	_, err = cl.SendPayPart(payreq, bolt11, bolt11.MilliSatoshis, scid, label, 0)
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

// RebalancePayment handles the lightning payment that should rebalance the channel
// if the payment is larger than 4mm sats it forces a mpp payment through the channel
func (cl *ClightningClient) RebalancePayment(payreq string, channel string) (preimage string, err error) {
	Bolt11, err := cl.glightning.DecodeBolt11(payreq)
	if err != nil {
		return "", err
	}
	if !strings.Contains(channel, "x") {
		channel = strings.Replace(channel, ":", "x", -1)
	}
	err = cl.CheckChannel(channel, Bolt11.MilliSatoshis/1000)
	if err != nil {
		return "", err
	}

	// If we exceed the maximum msat amount for a single payment we split them
	// up and use MPPs.
	if Bolt11.MilliSatoshis > maxPaymentSizeMsat {
		preimage, err = MppPayment(cl, payreq, channel, Bolt11)
		if err != nil {
			return "", err
		}
	} else {
		preimage, err = cl.PayInvoiceViaChannel(payreq, channel)
		if err != nil {
			return "", err
		}
	}
	return preimage, nil
}

func (cl *ClightningClient) SendPayPartAndWait(paymentRequest string, bolt11 *glightning.DecodedBolt11, amountMsat uint64, channel string, label string, partId uint64) (*glightning.SendPayFields, error) {
	_, err := cl.SendPayPart(paymentRequest, bolt11, amountMsat, channel, label, partId)
	if err != nil {
		return nil, err
	}
	return cl.glightning.WaitSendPayPart(bolt11.PaymentHash, 0, partId)
}

// MppPayment splits the payment in parts and waits for the payments to finish.
// We split in 10 parts as this will always result in a set of payments that
// dont have a "rest" and are all of the exact same size. They match the total
// amount that we want to transfer. As we only send over a direct channel to a
// direct peer we also dont need to optimize on a small number of subpayments.
func MppPayment(spw SendPayPartWaiter, payreq string, channel string, bolt11 *glightning.DecodedBolt11) (string, error) {
	wg := new(sync.WaitGroup)

	var numPayments uint64 = 10
	var partId uint64
	var res *glightning.SendPayFields
	var err error
	for partId = 1; partId < numPayments+1; partId++ {
		wg.Add(1)
		go func(partId uint64) {
			defer wg.Done()
			log.Debugf("Sending part %d/%d", partId, numPayments)
			res, err = spw.SendPayPartAndWait(payreq, bolt11, bolt11.MilliSatoshis/numPayments, channel, randomString(), partId)
			if err != nil {
				log.Debugf("Could not complete MPP: %v", err)
			}
		}(partId)
	}
	wg.Wait()

	if err != nil {
		return "", err
	}

	return res.PaymentPreimage, nil
}

// SendPayPart sends a payment through a specific channel. If the partId is not 0
// it sends only the part of the payment that is set on amountMsat. the final
// amount is read from the bolt11.
func (cl *ClightningClient) SendPayPart(payreq string, bolt11 *glightning.DecodedBolt11, amountMsat uint64, channel string, label string, partId uint64) (*glightning.SendPayResult, error) {
	res, err := cl.glightning.SendPay(
		[]glightning.RouteHop{
			{
				Id:             bolt11.Payee,
				ShortChannelId: channel,
				MilliSatoshi:   amountMsat,
				AmountMsat:     fmt.Sprintf("%dmsat", amountMsat),
				Delay:          uint(bolt11.MinFinalCltvExpiry + 1),
				Direction:      0,
			},
		},
		bolt11.PaymentHash,
		label,
		&bolt11.MilliSatoshis,
		payreq,
		bolt11.PaymentSecret,
		partId,
	)
	if err != nil {
		return nil, err
	}
	return res, nil
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
		peerlist = append(peerlist, peer.Id)
	}
	return peerlist
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
