package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/niftynei/glightning/glightning"
	"github.com/sputn1ck/sugarmama/lightning"
	"github.com/sputn1ck/sugarmama/liquid"
	"github.com/sputn1ck/sugarmama/swap"
	"log"
	"math/big"
	"os"
)

type ClightningClient struct {
	glightning *glightning.Lightning
	plugin     *glightning.Plugin

	msgHandlers []func(peerId string, messageType string, payload string) error
}

func NewClightningClient() (*ClightningClient, error) {
	cl := &ClightningClient{}
	cl.plugin = glightning.NewPlugin(cl.onInit)
	err := cl.plugin.RegisterHooks(&glightning.Hooks{
		CustomMsgReceived: cl.OnCustomMsg,
	})
	if err != nil {
		return nil, err
	}

	cl.glightning = glightning.NewLightning()

	var b big.Int
	b.Exp(big.NewInt(2), big.NewInt(112), nil)
	cl.plugin.AddNodeFeatures(b.Bytes())
	cl.plugin.SetDynamic(true)

	return cl, nil
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

	// If 'initialization' happened at the same time as the plugin starts,
	//   then the 'startup' will be true. Otherwise, you've been
	//   initialized by the 'dynamic' plugin command.
	//   Note that you have to opt-into dynamic startup.
	log.Printf("Is this initial node startup? %v\n", config.Startup)

	bopt, _ := plugin.GetBoolOption("bool_opt")
	iopt, _ := plugin.GetIntOption("int_opt")
	fopt, _ := plugin.IsOptionFlagged("flag_opt")

	log.Printf("the bool option is set to %t", bopt)
	log.Printf("the int option is set to %d", iopt)
	log.Printf("the flag option is set? %t", fopt)
}

func (c *ClightningClient) RegisterOptions() error {
	err := c.plugin.RegisterNewOption("db_path", "path to boltdb", "~/.liquid-loop/db")
	if err != nil {
		return err
	}
	err = c.plugin.RegisterNewOption("esplora_url", "url to esplora api", "")
	if err != nil {
		return err
	}
	return nil
}

func (c *ClightningClient) RegisterMethods(wallet lightning.WalletService, swaps *swap.Service, esplora *liquid.EsploraClient) error {
	loopIn := glightning.NewRpcMethod(&SwapOut{
		wallet:    wallet,
		pc:        c,
		lightning: c.glightning,
		swapper:   swaps,
	}, "Loop In")
	loopIn.Category = "liquid-loop"
	err := c.plugin.RegisterMethod(loopIn)
	if err != nil {
		return err
	}

	listSwaps := glightning.NewRpcMethod(&ListSwaps{
		swapper: swaps,
	}, "list swaps")
	listSwaps.Category = "liquid-loop"
	err = c.plugin.RegisterMethod(listSwaps)
	if err != nil {
		return err
	}

	getAddress := glightning.NewRpcMethod(&GetAddressMethod{
		wallet: wallet,
	}, "get new liquid address")
	getAddress.Category = "liquid-loop"
	err = c.plugin.RegisterMethod(getAddress)
	if err != nil {
		return err
	}

	getBalance := glightning.NewRpcMethod(&GetBalanceMethod{
		wallet: wallet,
	}, "get liquid balance")
	getBalance.Category = "liquid-loop"
	err = c.plugin.RegisterMethod(getBalance)
	if err != nil {
		return err
	}

	listUtxos := glightning.NewRpcMethod(&ListUtxosMethod{
		wallet: wallet,
	}, "list liquid utxos")
	listUtxos.Category = "liquid-loop"
	err = c.plugin.RegisterMethod(listUtxos)
	if err != nil {
		return err
	}

	devFaucet := glightning.NewRpcMethod(&DevFaucet{
		wallet:  wallet,
		esplora: esplora,
	}, "list liquid utxos")
	devFaucet.Category = "liquid-loop"
	err = c.plugin.RegisterMethod(devFaucet)
	if err != nil {
		return err
	}
	return nil
}
