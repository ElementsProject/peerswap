package clightning

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/sputn1ck/glightning/gbitcoin"
	"github.com/sputn1ck/peerswap/onchain"

	"github.com/sputn1ck/glightning/gelements"
	"github.com/sputn1ck/glightning/glightning"
	"github.com/sputn1ck/glightning/jrpc2"
	"github.com/sputn1ck/peerswap/lightning"
	"github.com/sputn1ck/peerswap/messages"
	"github.com/sputn1ck/peerswap/poll"
	"github.com/sputn1ck/peerswap/swap"
	"github.com/sputn1ck/peerswap/wallet"
)

var methods = []peerswaprpcMethod{
	&ListNodes{},
	&ListPeers{},
	&LiquidSendToAddress{},
	&GetSwap{},
	&ResendLastMessage{},
}

var devmethods = []peerswaprpcMethod{}

const (
	featureBit = 69

	paymentSplitterMsat = 1000000000
)

// ClightningClient is the main driver behind c-lightnings plugins system
// it handles rpc calls and messages
type ClightningClient struct {
	glightning *glightning.Lightning
	plugin     *glightning.Plugin

	liquidWallet   wallet.Wallet
	swaps          *swap.SwapService
	requestedSwaps *swap.RequestedSwapsPrinter
	policy         PolicyReloader
	pollService    *poll.Service

	Gelements *gelements.Elements

	gbitcoin       *gbitcoin.Bitcoin
	bitcoinChain   *onchain.BitcoinOnChain
	bitcoinNetwork *chaincfg.Params

	msgHandlers          []func(peerId string, messageType string, payload []byte) error
	paymentSubscriptions []func(paymentLabel string)
	initChan             chan interface{}
	nodeId               string
	hexToIdMap           map[string]string
}

// NewClightningClient returns a new clightning cl and channel which get closed when the plugin is initialized
func NewClightningClient() (*ClightningClient, <-chan interface{}, error) {
	cl := &ClightningClient{}
	cl.plugin = glightning.NewPlugin(cl.onInit)
	err := cl.plugin.RegisterHooks(&glightning.Hooks{
		CustomMsgReceived: cl.OnCustomMsg,
	})
	if err != nil {
		return nil, nil, err
	}
	cl.plugin.SubscribeInvoicePaid(cl.OnPayment)
	cl.plugin.SubscribeConnect(cl.OnConnect)

	cl.glightning = glightning.NewLightning()

	b := big.NewInt(0)
	b = b.Exp(big.NewInt(2), big.NewInt(featureBit), nil)
	cl.plugin.AddNodeFeatures(b.Bytes())
	cl.plugin.SetDynamic(true)
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
func (cl *ClightningClient) SetupClients(liquidWallet wallet.Wallet,
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

// OnPayment gets called by clightnings hooks
func (cl *ClightningClient) OnPayment(payment *glightning.Payment) {
	for _, v := range cl.paymentSubscriptions {
		v(payment.Label)
	}
}

// Start starts the plugin
func (cl *ClightningClient) Start() error {
	return cl.plugin.Start(os.Stdin, os.Stdout)
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
		log.Printf("[Messenger] error decoding payload %v", err)
	}
	for _, v := range cl.msgHandlers {
		err := v(event.PeerId, typeString, payloadDecoded)
		if err != nil {
			log.Printf("\n msghandler err: %v", err)
		}
	}
	return event.Continue(), nil
}

// AddMessageHandler adds a listener for incoming peermessages
func (cl *ClightningClient) AddMessageHandler(f func(peerId string, msgType string, payload []byte) error) {
	cl.msgHandlers = append(cl.msgHandlers, f)
}

// AddPaymentCallback adds a callback when a payment was paid
func (cl *ClightningClient) AddPaymentCallback(f func(paymentLabel string)) {
	cl.paymentSubscriptions = append(cl.paymentSubscriptions, f)
}

// GetPayreq returns a Bolt11 Invoice
func (cl *ClightningClient) GetPayreq(amountMsat uint64, preImage string, label string, expiry uint64) (string, error) {
	res, err := cl.glightning.CreateInvoice(amountMsat, label, "liquid swap", uint32(expiry), []string{}, preImage, false)
	if err != nil {
		return "", err
	}
	return res.Bolt11, nil
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
	if Bolt11.MilliSatoshis > 4000000000 {
		preimage, err = cl.MppPayment(payreq, channel, Bolt11)
		if err != nil {
			return "", err
		}
	} else {
		label := randomString()
		_, err = cl.SendPayChannel(payreq, Bolt11, Bolt11.MilliSatoshis, channel, label, 0)
		if err != nil {
			return "", err
		}
		res, err := cl.glightning.WaitSendPay(Bolt11.PaymentHash, 30)
		if err != nil {
			return "", err
		}
		preimage = res.PaymentPreimage
	}
	return preimage, nil
}

// MppPayment splits the payment in parts and waits for the payments to finish
func (cl *ClightningClient) MppPayment(payreq string, channel string, Bolt11 *glightning.DecodedBolt11) (string, error) {
	label := randomString()
	var preimage string

	splits := Bolt11.MilliSatoshis / paymentSplitterMsat

	log.Printf("millisats: %v splitter: %v, splits: %v", Bolt11.MilliSatoshis, paymentSplitterMsat, splits)
	var i uint64
	for i = 1; i < splits+1; i++ {
		split := i
		_, err := cl.SendPayChannel(payreq, Bolt11, paymentSplitterMsat, channel, fmt.Sprintf("%s%v", label, i), split)
		if err != nil {
			return "", err
		}
	}
	remainingSats := Bolt11.MilliSatoshis - splits*paymentSplitterMsat
	if remainingSats > 0 {
		split := i
		_, err := cl.SendPayChannel(payreq, Bolt11, remainingSats, channel, fmt.Sprintf("%s%v", label, i), split)
		if err != nil {
			return "", err
		}
	} else {
		i--
	}
	res, err := cl.glightning.WaitSendPayPart(Bolt11.PaymentHash, 30, i)
	if err != nil {
		return "", err
	}
	preimage = res.PaymentPreimage
	return preimage, nil
}

// SendPayChannel sends a payment through a specific channel
func (cl *ClightningClient) SendPayChannel(payreq string, bolt11 *glightning.DecodedBolt11, amountMsat uint64, channel string, label string, partId uint64) (string, error) {

	satString := fmt.Sprintf("%smsat", strconv.FormatUint(amountMsat, 10))
	res, err := cl.glightning.SendPay(
		[]glightning.RouteHop{
			{
				Id:             bolt11.Payee,
				ShortChannelId: channel,
				MilliSatoshi:   amountMsat,
				AmountMsat:     satString,
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
		return "", err
	}
	return res.PaymentPreimage, nil
}

func (cl *ClightningClient) PeerRunsPeerSwap(peerid string) error {
	// get polls
	polls, err := cl.pollService.GetPolls()
	if err != nil {
		return err
	}
	peers, err := cl.glightning.ListPeers()
	if err != nil {
		return err
	}

	if _, ok := polls[peerid]; !ok {
		return errors.New("peer does not run peerswap")
	}

	for _, peer := range peers {
		if peer.Id == peerid && peer.Connected {
			return nil
		}
	}
	return errors.New("peer is not connected")
}

// This is called after the plugin starts up successfully
func (cl *ClightningClient) onInit(plugin *glightning.Plugin, options map[string]glightning.Option, config *glightning.Config) {
	log.Printf("successfully init'd! %s\n", config.RpcFile)
	cl.glightning.StartUp(config.RpcFile, config.LightningDir)

	getInfo, err := cl.glightning.GetInfo()
	if err != nil {
		log.Fatalf("getinfo err %v", err)
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
	swapIn := glightning.NewRpcMethod(&SwapIn{
		cl: cl,
	}, "swap In")
	swapIn.Category = "peerswap"
	err := cl.plugin.RegisterMethod(swapIn)
	if err != nil {
		return err
	}

	swapOut := glightning.NewRpcMethod(&SwapOut{
		cl: cl,
	}, "swap out")
	swapOut.Category = "peerswap"
	err = cl.plugin.RegisterMethod(swapOut)
	if err != nil {
		return err
	}

	listSwaps := glightning.NewRpcMethod(&ListSwaps{
		cl: cl,
	}, "list swaps")
	listSwaps.Category = "peerswap"
	err = cl.plugin.RegisterMethod(listSwaps)
	if err != nil {
		return err
	}

	getAddress := glightning.NewRpcMethod(&LiquidGetAddress{
		cl: cl,
	}, "get new liquid address")
	getAddress.Category = "peerswap"
	err = cl.plugin.RegisterMethod(getAddress)
	if err != nil {
		return err
	}

	getBalance := glightning.NewRpcMethod(&LiquidGetBalance{
		cl: cl,
	}, "get liquid liquidWallet balance")
	getBalance.Category = "peerswap"
	err = cl.plugin.RegisterMethod(getBalance)
	if err != nil {
		return err
	}

	long := `If the policy file has changed, reload the policy
	from the file specified in the config. Overrides the default
	config, so fields that are not set are interpreted as default.`
	reloadPolicyFile := &glightning.RpcMethod{
		Method: &ReloadPolicyFile{
			cl:   cl,
			name: "peerswap-reload-policy",
		},
		Desc:     "Reload the policy file.",
		LongDesc: long,
		Category: "peerswap",
	}
	err = cl.plugin.RegisterMethod(reloadPolicyFile)
	if err != nil {
		return err
	}

	listRequestedSwapsMethod := &GetRequestedSwaps{
		cl:   cl,
		name: "peerswap-listswaprequests",
	}
	listRequestedSwaps := &glightning.RpcMethod{
		Method:   listRequestedSwapsMethod,
		Desc:     listRequestedSwapsMethod.Description(),
		LongDesc: listRequestedSwapsMethod.LongDescription(),
		Category: "peerswap",
	}
	err = cl.plugin.RegisterMethod(listRequestedSwaps)
	if err != nil {
		return err
	}

	for _, v := range methods {
		method := v.Get(cl)
		glightningMethod := glightning.NewRpcMethod(method, "dev")
		glightningMethod.Category = "peerswap"
		glightningMethod.Desc = v.Description()
		glightningMethod.LongDesc = v.LongDescription()
		err = cl.plugin.RegisterMethod(glightningMethod)
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
		err = cl.plugin.RegisterMethod(glightningMethod)
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
		log.Printf("could not listpeers: %v", err)
		return nil
	}

	var peerlist []string
	for _, peer := range peers {
		peerlist = append(peerlist, peer.Id)
	}
	return peerlist
}
