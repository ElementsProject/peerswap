package clightning

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sputn1ck/glightning/gelements"
	"github.com/sputn1ck/glightning/glightning"
	"github.com/sputn1ck/glightning/jrpc2"
	"github.com/sputn1ck/peerswap"
	"github.com/sputn1ck/peerswap/blockchain"
	"github.com/sputn1ck/peerswap/lightning"
	"github.com/sputn1ck/peerswap/swap"
	"github.com/sputn1ck/peerswap/wallet"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
)

var methods []peerswaprpcMethod

const (
	dbOption          = "peerswap-db-path"
	rpcHostOption     = "peerswap-liquid-rpchost"
	rpcPortOption     = "peerswap-liquid-rpcport"
	rpcUserOption     = "peerswap-liquid-rpcuser"
	rpcPasswordOption = "peerswap-liquid-rpcpassword"

	rpcWalletOption = "peerswap-liquid-rpcwallet"

	liquidNetworkOption = "peerswap-liquid-network"
)

type ClightningClient struct {
	glightning *glightning.Lightning
	plugin     *glightning.Plugin

	wallet     wallet.Wallet
	swaps      *swap.SwapService
	blockchain blockchain.Blockchain

	Gelements *gelements.Elements

	msgHandlers          []func(peerId string, messageType string, payload string) error
	paymentSubscriptions []func(payment *glightning.Payment)
	initChan             chan interface{}
	nodeId               string
}

func (c *ClightningClient) CheckChannel(channelId string, amount uint64) (bool, error) {
	funds, err := c.glightning.ListFunds()
	if err != nil {
		return false, err
	}
	var fundingChannels *glightning.FundingChannel
	for _, v := range funds.Channels {
		if v.ShortChannelId == channelId {
			fundingChannels = v
			break
		}
	}
	if fundingChannels == nil {
		return false, errors.New("fundingChannels not found")
	}

	if fundingChannels.ChannelSatoshi < amount {
		return false, errors.New("not enough outbound capacity to perform swapOut")
	}
	if !fundingChannels.Connected {
		return false, errors.New("fundingChannels is not connected")
	}
	return true, nil
}

func (c *ClightningClient) GetNodeId() string {
	return c.nodeId
}

func (c *ClightningClient) GetPreimage() (lightning.Preimage, error) {
	var preimage lightning.Preimage

	if _, err := rand.Read(preimage[:]); err != nil {
		return preimage, err
	}
	return preimage, nil
}

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

	var b big.Int
	b.Exp(big.NewInt(2), big.NewInt(112), nil)
	cl.plugin.AddNodeFeatures(b.Bytes())
	cl.plugin.SetDynamic(true)

	cl.initChan = make(chan interface{})
	return cl, cl.initChan, nil
}

func (c *ClightningClient) OnPayment(payment *glightning.Payment) {
	for _, v := range c.paymentSubscriptions {
		v(payment)
	}
}

func (c *ClightningClient) Start() error {
	return c.plugin.Start(os.Stdin, os.Stdout)
}

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

func (c *ClightningClient) AddMessageHandler(f func(peerId string, msgType string, payload string) error) {
	c.msgHandlers = append(c.msgHandlers, f)
}

func (c *ClightningClient) AddPaymentCallback(f func(*glightning.Payment)) {
	c.paymentSubscriptions = append(c.paymentSubscriptions, f)
}

func (c *ClightningClient) GetPayreq(amountMsat uint64, preImage string, label string) (string, error) {
	res, err := c.glightning.CreateInvoice(amountMsat, label, "liquid swap", 3600, []string{}, preImage, false)
	if err != nil {
		return "", err
	}
	return res.Bolt11, nil
}

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

func (c *ClightningClient) PayInvoice(payreq string) (preimage string, err error) {
	res, err := c.glightning.Pay(&glightning.PayRequest{Bolt11: payreq})
	if err != nil {
		return "", err
	}
	return res.PaymentPreimage, nil
}

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

	return &peerswap.Config{
		DbPath:      dbpath,
		RpcHost:     rpcHost,
		RpcPort:     uint(rpcPort),
		RpcUser:     rpcUser,
		RpcPassword: rpcPass,
		Network:     liquidNetwork,
		RpcWallet:   rpcWallet,
	}, nil
}

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
	return nil
}

func (c *ClightningClient) SetupClients(wallet wallet.Wallet, swaps *swap.SwapService, blockchain blockchain.Blockchain, elements *gelements.Elements) {
	c.wallet = wallet
	c.swaps = swaps
	c.blockchain = blockchain
	c.Gelements = elements
}
func (c *ClightningClient) RegisterMethods() error {
	swapIn := glightning.NewRpcMethod(&SwapIn{
		cl: c,
	}, "swap In")
	swapIn.Category = "liquid-swap"
	err := c.plugin.RegisterMethod(swapIn)
	if err != nil {
		return err
	}

	swapOut := glightning.NewRpcMethod(&SwapOut{
		cl: c,
	}, "swap out")
	swapOut.Category = "liquid-swap"
	err = c.plugin.RegisterMethod(swapOut)
	if err != nil {
		return err
	}

	listSwaps := glightning.NewRpcMethod(&ListSwaps{
		cl: c,
	}, "list swaps")
	listSwaps.Category = "liquid-swap"
	err = c.plugin.RegisterMethod(listSwaps)
	if err != nil {
		return err
	}

	getAddress := glightning.NewRpcMethod(&GetAddressMethod{
		cl: c,
	}, "get new liquid address")
	getAddress.Category = "liquid-swap"
	err = c.plugin.RegisterMethod(getAddress)
	if err != nil {
		return err
	}

	getBalance := glightning.NewRpcMethod(&GetBalanceMethod{
		cl: c,
	}, "get liquid wallet balance")
	getBalance.Category = "liquid-swap"
	err = c.plugin.RegisterMethod(getBalance)
	if err != nil {
		return err
	}

	for _, v := range methods {
		method := v.Get(c)
		glightningMethod := glightning.NewRpcMethod(method, "dev")
		glightningMethod.Category = "liquid-swap-dev"
		err = c.plugin.RegisterMethod(glightningMethod)
		if err != nil {
			return err
		}
	}
	return nil
}

type peerswaprpcMethod interface {
	Get(*ClightningClient) jrpc2.ServerMethod
}
