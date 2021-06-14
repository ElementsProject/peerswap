package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/niftynei/glightning/glightning"
	"github.com/sputn1ck/sugarmama"
	"github.com/sputn1ck/sugarmama/lightning"
	"github.com/sputn1ck/sugarmama/liquid"
	"github.com/sputn1ck/sugarmama/swap"
	"log"
	"math/big"
	"os"
	"path/filepath"
)

const (
	dbOption      = "db-path"
	esploraOption = "esplora-url"
)
type ClightningClient struct {
	glightning *glightning.Lightning
	plugin     *glightning.Plugin

	wallet lightning.WalletService
	swaps *swap.Service
	esplora *liquid.EsploraClient

	msgHandlers []func(peerId string, messageType string, payload string) error
	initChan chan interface{}
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

	cl.glightning = glightning.NewLightning()

	var b big.Int
	b.Exp(big.NewInt(2), big.NewInt(112), nil)
	cl.plugin.AddNodeFeatures(b.Bytes())
	cl.plugin.SetDynamic(true)

	cl.initChan = make(chan interface{})
	return cl,cl.initChan, nil
}

func (c *ClightningClient) Start() error {
	return c.plugin.Start(os.Stdin, os.Stdout)
}

func (c *ClightningClient) SendMessage(peerId string, message lightning.PeerMessage) error {
	messageBytes, err := json.Marshal(message)
	if err != nil {
		return err
	}
	msg := message.MessageType() + hex.EncodeToString(messageBytes)
	res, err := c.glightning.SendCustomMessage(peerId, msg)
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return errors.New(res.Message)
	}
	return nil
}

func (c *ClightningClient) AddMessageHandler(f func(peerId string, messageType string, payload string) error) error {
	c.msgHandlers = append(c.msgHandlers, f)
	return nil
}

func (c *ClightningClient) GetPayreq(amountMsat uint64, preImage string, label string) (string, error) {
	res, err := c.glightning.CreateInvoice(amountMsat, "liquid swap:"+label, "liquid swap", 3600, []string{}, preImage, false)
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
		log.Printf("got pay err: %s ", err.Error())
		return "", err
	}
	return res.PaymentPreimage, nil
}

func (c *ClightningClient) OnCustomMsg(event *glightning.CustomMsgReceivedEvent) (*glightning.CustomMsgReceivedResponse, error) {

	typeString := event.Payload[:4]
	payload := event.Payload[4:]
	log.Printf("new custom msg. peer: %s, messageType %s messageType payload: %s", event.PeerId, typeString, payload)
	for _, v := range c.msgHandlers {
		err := v(event.PeerId, typeString, payload)
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
	c.initChan<-true
}

func (c *ClightningClient) GetConfig() (*sugarmama.Config, error) {

	dbpath, err := c.plugin.GetOption(dbOption)
	if err != nil {
		return nil, err
	}
	if dbpath == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		dbpath = filepath.Join(wd,"swaps")
	}
	err = os.MkdirAll(dbpath,0700)
	if err != nil && err != os.ErrExist{
		return nil, err
	}
	dbpath = filepath.Join(dbpath,"db")
	esploraUrl, err := c.plugin.GetOption(esploraOption)
	if err != nil {
		return nil, err
	}
	if esploraUrl == "" {
		return nil, errors.New(fmt.Sprintf("%s need to be set", esploraOption))
	}

	return &sugarmama.Config{
		DbPath:     dbpath,
		EsploraUrl: esploraUrl,
	}, nil
}

func (c *ClightningClient) RegisterOptions() error {
	err := c.plugin.RegisterNewOption(dbOption, "path to boltdb", "")
	if err != nil {
		return err
	}
	err = c.plugin.RegisterNewOption(esploraOption, "url to esplora api", "")
	if err != nil {
		return err
	}
	return nil
}

func (c *ClightningClient) SetupClients(wallet lightning.WalletService, swaps *swap.Service, esplora *liquid.EsploraClient) {
	c.wallet = wallet
	c.swaps = swaps
	c.esplora = esplora
}
func (c *ClightningClient) RegisterMethods() error {
	loopIn := glightning.NewRpcMethod(&SwapOut{
		cl: c,
	}, "Loop In")
	loopIn.Category = "liquid-loop"
	err := c.plugin.RegisterMethod(loopIn)
	if err != nil {
		return err
	}

	listSwaps := glightning.NewRpcMethod(&ListSwaps{
		cl: c,
	}, "list swaps")
	listSwaps.Category = "liquid-loop"
	err = c.plugin.RegisterMethod(listSwaps)
	if err != nil {
		return err
	}

	getAddress := glightning.NewRpcMethod(&GetAddressMethod{
		cl: c,
	}, "get new liquid address")
	getAddress.Category = "liquid-loop"
	err = c.plugin.RegisterMethod(getAddress)
	if err != nil {
		return err
	}

	getBalance := glightning.NewRpcMethod(&GetBalanceMethod{
		cl: c,
	}, "get liquid balance")
	getBalance.Category = "liquid-loop"
	err = c.plugin.RegisterMethod(getBalance)
	if err != nil {
		return err
	}

	listUtxos := glightning.NewRpcMethod(&ListUtxosMethod{
		cl: c,
	}, "list liquid utxos")
	listUtxos.Category = "liquid-loop"
	err = c.plugin.RegisterMethod(listUtxos)
	if err != nil {
		return err
	}

	devFaucet := glightning.NewRpcMethod(&DevFaucet{
		cl: c,
	}, "add lbtc funds to wallet")
	devFaucet.Category = "liquid-loop"
	err = c.plugin.RegisterMethod(devFaucet)
	if err != nil {
		return err
	}
	return nil
}
