package clightning

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"strconv"

	"github.com/sputn1ck/glightning/gelements"
	"github.com/sputn1ck/glightning/glightning"
	"github.com/sputn1ck/glightning/jrpc2"
	"github.com/sputn1ck/peerswap"
	"github.com/sputn1ck/peerswap/lightning"
	"github.com/sputn1ck/peerswap/swap"
	"github.com/sputn1ck/peerswap/wallet"
)

var methods = []peerswaprpcMethod{
	&ListNodes{},
	&ListPeers{},
	&SendToAddressMethod{},
	&GetSwap{},
}

var devmethods = []peerswaprpcMethod{}

const (
	dbOption            = "peerswap-db-path"
	rpcHostOption       = "peerswap-liquid-rpchost"
	rpcPortOption       = "peerswap-liquid-rpcport"
	rpcUserOption       = "peerswap-liquid-rpcuser"
	rpcPasswordOption   = "peerswap-liquid-rpcpassword"
	rpcWalletOption     = "peerswap-liquid-rpcwallet"
	liquidNetworkOption = "peerswap-liquid-network"
	policyPathOption    = "peerswap-policy-path"

	featureBit = 69

	paymentSplitterMsat = 1000000000
)

// ClightningClient is the main driver behind c-lightnings plugins system
// it handles rpc calls and messages
type ClightningClient struct {
	glightning *glightning.Lightning
	plugin     *glightning.Plugin

	wallet wallet.Wallet
	swaps  *swap.SwapService

	Gelements *gelements.Elements

	msgHandlers          []func(peerId string, messageType string, payload string) error
	paymentSubscriptions []func(payment *glightning.Payment)
	initChan             chan interface{}
	nodeId               string
}

// CheckChannel checks if a channel is eligable for a swap
func (c *ClightningClient) CheckChannel(channelId string, amount uint64) error {
	funds, err := c.glightning.ListFunds()
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

	if fundingChannels.ChannelSatoshi < amount {
		return errors.New("not enough outbound capacity to perform swapOut")
	}
	if !fundingChannels.Connected {
		return errors.New("fundingChannels is not connected")
	}
	return nil
}

// GetNodeId returns the lightning nodes pubkey
func (c *ClightningClient) GetNodeId() string {
	return c.nodeId
}

// GetPreimage returns a random preimage
func (c *ClightningClient) GetPreimage() (lightning.Preimage, error) {
	var preimage lightning.Preimage

	if _, err := rand.Read(preimage[:]); err != nil {
		return preimage, err
	}
	return preimage, nil
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

	cl.glightning = glightning.NewLightning()

	b := big.NewInt(0)
	b = b.Exp(big.NewInt(2), big.NewInt(featureBit), nil)
	cl.plugin.AddNodeFeatures(b.Bytes())
	cl.plugin.SetDynamic(true)
	cl.initChan = make(chan interface{})
	return cl, cl.initChan, nil
}

func (c *ClightningClient) GetLightningRpc() *glightning.Lightning {
	return c.glightning
}

// OnPayment gets called by clightnings hooks
func (c *ClightningClient) OnPayment(payment *glightning.Payment) {
	for _, v := range c.paymentSubscriptions {
		v(payment)
	}
}

// Start starts the plugin
func (c *ClightningClient) Start() error {
	return c.plugin.Start(os.Stdin, os.Stdout)
}

// SendMessage sends a hexmessage to a peer
func (c *ClightningClient) SendMessage(peerId string, message swap.PeerMessage) error {
	messageBytes, err := json.Marshal(message)
	if err != nil {
		return err
	}
	msg := swap.MessageTypeToHexString(message.MessageType()) + hex.EncodeToString(messageBytes)
	res, err := c.glightning.SendCustomMessage(peerId, msg)
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return errors.New(res.Message)
	}
	return nil
}

// AddMessageHandler adds a listener for incoming peermessages
func (c *ClightningClient) AddMessageHandler(f func(peerId string, msgType string, payload string) error) {
	c.msgHandlers = append(c.msgHandlers, f)
}

// AddPaymentCallback adds a callback when a payment was paid
func (c *ClightningClient) AddPaymentCallback(f func(*glightning.Payment)) {
	c.paymentSubscriptions = append(c.paymentSubscriptions, f)
}

// GetPayreq returns a Bolt11 Invoice
func (c *ClightningClient) GetPayreq(amountMsat uint64, preImage string, label string) (string, error) {
	res, err := c.glightning.CreateInvoice(amountMsat, label, "liquid swap", 3600, []string{}, preImage, false)
	if err != nil {
		return "", err
	}
	return res.Bolt11, nil
}

// DecodePayreq decodes a Bolt11 Invoice
func (c *ClightningClient) DecodePayreq(payreq string) (*lightning.Invoice, error) {
	res, err := c.glightning.DecodeBolt11(payreq)
	if err != nil {
		return nil, err
	}
	return &lightning.Invoice{
		Description: res.Description,
		PHash:       res.PaymentHash,
		Amount:      res.MilliSatoshis,
	}, nil
}

// PayInvoice tries to pay a Bolt11 Invoice
func (c *ClightningClient) PayInvoice(payreq string) (preimage string, err error) {
	res, err := c.glightning.Pay(&glightning.PayRequest{Bolt11: payreq})
	if err != nil {
		return "", err
	}
	return res.PaymentPreimage, nil
}

// RebalancePayment handles the lightning payment that should rebalance the channel
// if the payment is larger than 4mm sats it forces a mpp payment through the channel
func (c *ClightningClient) RebalancePayment(payreq string, channel string) (preimage string, err error) {
	Bolt11, err := c.glightning.DecodeBolt11(payreq)
	if err != nil {
		return "", err
	}
	err = c.CheckChannel(channel, Bolt11.MilliSatoshis/1000)
	if err != nil {
		return "", err
	}
	if Bolt11.MilliSatoshis > 4000000000 {
		preimage, err = c.MppPayment(payreq, channel, Bolt11)
		if err != nil {
			return "", err
		}
	} else {
		label := randomString()
		_, err = c.SendPayChannel(payreq, Bolt11, Bolt11.MilliSatoshis, channel, label, 0)
		if err != nil {
			return "", err
		}
		res, err := c.glightning.WaitSendPay(Bolt11.PaymentHash, 30)
		if err != nil {
			return "", err
		}
		preimage = res.PaymentPreimage
	}
	return preimage, nil
}

// MppPayment splits the payment in parts and waits for the payments to finish
func (c *ClightningClient) MppPayment(payreq string, channel string, Bolt11 *glightning.DecodedBolt11) (string, error) {
	label := randomString()
	var preimage string

	splits := Bolt11.MilliSatoshis / paymentSplitterMsat
	log.Printf("millisats: %v splitter: %v, splits: %v", Bolt11.MilliSatoshis, paymentSplitterMsat, splits)
	var i uint64
	for i = 1; i < splits+1; i++ {
		split := i
		_, err := c.SendPayChannel(payreq, Bolt11, paymentSplitterMsat, channel, fmt.Sprintf("%s%v", label, i), split)
		if err != nil {
			return "", err
		}
	}
	remainingSats := Bolt11.MilliSatoshis - splits*paymentSplitterMsat
	if remainingSats > 0 {
		split := i
		_, err := c.SendPayChannel(payreq, Bolt11, remainingSats, channel, fmt.Sprintf("%s%v", label, i), split)
		if err != nil {
			return "", err
		}
	} else {
		i--
	}
	res, err := c.glightning.WaitSendPayPart(Bolt11.PaymentHash, 30, i)
	if err != nil {
		return "", err
	}
	preimage = res.PaymentPreimage
	return preimage, nil
}

// SendPayChannel sends a payment through a specific channel
func (c *ClightningClient) SendPayChannel(payreq string, bolt11 *glightning.DecodedBolt11, amountMsat uint64, channel string, label string, partId uint64) (string, error) {

	satString := fmt.Sprintf("%smsat", strconv.FormatUint(amountMsat, 10))
	res, err := c.glightning.SendPay(
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

	log.Printf("message %s", res.Message)

	return res.PaymentPreimage, nil
}

// OnCustomMsg is the hook that c-lightning calls
func (c *ClightningClient) OnCustomMsg(event *glightning.CustomMsgReceivedEvent) (*glightning.CustomMsgReceivedResponse, error) {
	typeString := event.Payload[:4]
	payload := event.Payload[4:]
	payloadDecoded, err := hex.DecodeString(payload)
	if err != nil {
		log.Printf("[Messenger] error decoding payload %v", err)
	}
	for _, v := range c.msgHandlers {
		err := v(event.PeerId, typeString, string(payloadDecoded))
		if err != nil {
			log.Printf("\n msghandler err: %v", err)
		}
	}
	return event.Continue(), nil
}

// This is called after the plugin starts up successfully
func (c *ClightningClient) onInit(plugin *glightning.Plugin, options map[string]glightning.Option, config *glightning.Config) {
	log.Printf("successfully init'd! %s\n", config.RpcFile)
	c.glightning.StartUp(config.RpcFile, config.LightningDir)

	getInfo, err := c.glightning.GetInfo()
	if err != nil {
		log.Fatalf("getinfo err %v", err)
	}
	c.nodeId = getInfo.Id
	c.initChan <- true
}

// GetConfig returns the peerswap config
func (c *ClightningClient) GetConfig() (*peerswap.Config, error) {

	dbpath, err := c.plugin.GetOption(dbOption)
	if err != nil {
		return nil, err
	}
	if dbpath == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		dbpath = filepath.Join(wd, "swaps")
	}
	err = os.MkdirAll(dbpath, 0700)
	if err != nil && err != os.ErrExist {
		return nil, err
	}
	rpcHost, err := c.plugin.GetOption(rpcHostOption)
	if err != nil {
		return nil, err
	}
	if rpcHost == "" {
		return nil, errors.New(fmt.Sprintf("%s need to be set", rpcHostOption))
	}
	rpcPortString, err := c.plugin.GetOption(rpcPortOption)
	if err != nil {
		return nil, err
	}
	if rpcPortString == "" {
		return nil, errors.New(fmt.Sprintf("%s need to be set", rpcPortOption))
	}
	rpcPort, err := strconv.Atoi(rpcPortString)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("%s is not an int", rpcPortOption))
	}
	rpcUser, err := c.plugin.GetOption(rpcUserOption)
	if err != nil {
		return nil, err
	}
	if rpcUser == "" {
		return nil, errors.New(fmt.Sprintf("%s need to be set", rpcUserOption))
	}
	rpcPass, err := c.plugin.GetOption(rpcPasswordOption)
	if err != nil {
		return nil, err
	}
	if rpcPass == "" {
		return nil, errors.New(fmt.Sprintf("%s need to be set", rpcPasswordOption))
	}
	liquidNetwork, err := c.plugin.GetOption(liquidNetworkOption)
	if err != nil {
		return nil, err
	}
	rpcWallet, err := c.plugin.GetOption(rpcWalletOption)
	if err != nil {
		return nil, err
	}
	if rpcWallet == "dev_test" {
		idBytes := make([]byte, 8)
		_, _ = rand.Read(idBytes[:])
		rpcWallet = hex.EncodeToString(idBytes)
	}

	// get policy path
	policyPath, err := c.plugin.GetOption(policyPathOption)
	if err != nil {
		return nil, err
	}

	return &peerswap.Config{
		DbPath:              dbpath,
		LiquidRpcHost:       rpcHost,
		LiquidRpcPort:       uint(rpcPort),
		LiquidRpcUser:       rpcUser,
		LiquidRpcPassword:   rpcPass,
		LiquidNetworkString: liquidNetwork,
		LiquidRpcWallet:     rpcWallet,
		PolicyPath:          policyPath,
	}, nil
}

// RegisterOptions adds options to clightning
func (c *ClightningClient) RegisterOptions() error {
	err := c.plugin.RegisterNewOption(dbOption, "path to boltdb", "")
	if err != nil {
		return err
	}
	err = c.plugin.RegisterNewOption(rpcHostOption, "elementsd rpchost", "")
	if err != nil {
		return err
	}
	err = c.plugin.RegisterNewOption(rpcPortOption, "elementsd rpcport", "")
	if err != nil {
		return err
	}
	err = c.plugin.RegisterNewOption(rpcUserOption, "elementsd rpcuser", "")
	if err != nil {
		return err
	}
	err = c.plugin.RegisterNewOption(rpcPasswordOption, "elementsd rpcpassword", "")
	if err != nil {
		return err
	}
	err = c.plugin.RegisterNewOption(liquidNetworkOption, "liquid-network", "regtest")
	if err != nil {
		return err
	}
	err = c.plugin.RegisterNewOption(rpcWalletOption, "liquid-rpcwallet", "swap")
	if err != nil {
		return err
	}

	// register policy options
	err = c.plugin.RegisterNewOption(policyPathOption, "Path to the policy file. If empty the default policy is used", "")
	if err != nil {
		return err
	}
	return nil
}

// SetupClients injects the required services
func (c *ClightningClient) SetupClients(wallet wallet.Wallet, swaps *swap.SwapService, elements *gelements.Elements) {
	c.wallet = wallet
	c.swaps = swaps
	c.Gelements = elements
}

// RegisterMethods registeres rpc methods to c-lightning
func (c *ClightningClient) RegisterMethods() error {
	swapIn := glightning.NewRpcMethod(&SwapIn{
		cl: c,
	}, "swap In")
	swapIn.Category = "peerswap"
	err := c.plugin.RegisterMethod(swapIn)
	if err != nil {
		return err
	}

	swapOut := glightning.NewRpcMethod(&SwapOut{
		cl: c,
	}, "swap out")
	swapIn.Category = "peerswap"
	err = c.plugin.RegisterMethod(swapOut)
	if err != nil {
		return err
	}

	listSwaps := glightning.NewRpcMethod(&ListSwaps{
		cl: c,
	}, "list swaps")
	swapIn.Category = "peerswap"
	err = c.plugin.RegisterMethod(listSwaps)
	if err != nil {
		return err
	}

	getAddress := glightning.NewRpcMethod(&GetAddressMethod{
		cl: c,
	}, "get new liquid address")
	swapIn.Category = "peerswap"
	err = c.plugin.RegisterMethod(getAddress)
	if err != nil {
		return err
	}

	getBalance := glightning.NewRpcMethod(&GetBalanceMethod{
		cl: c,
	}, "get liquid wallet balance")
	swapIn.Category = "peerswap"
	err = c.plugin.RegisterMethod(getBalance)
	if err != nil {
		return err
	}

	for _, v := range methods {
		method := v.Get(c)
		glightningMethod := glightning.NewRpcMethod(method, "dev")
		glightningMethod.Category = "peerswap"
		glightningMethod.Desc = v.Description()
		glightningMethod.LongDesc = v.LongDescription()
		err = c.plugin.RegisterMethod(glightningMethod)
		if err != nil {
			return err
		}
	}
	for _, v := range devmethods {
		method := v.Get(c)
		glightningMethod := glightning.NewRpcMethod(method, "dev")
		glightningMethod.Category = "peerswap"
		glightningMethod.Desc = v.Description()
		glightningMethod.LongDesc = v.LongDescription()
		err = c.plugin.RegisterMethod(glightningMethod)
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
