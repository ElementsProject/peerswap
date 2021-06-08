package lightning

import (
	"encoding/hex"
	"errors"
	"github.com/niftynei/glightning/glightning"
	"log"
	"math/big"
	"os"
)

type ClightningClient struct {
	glightning *glightning.Lightning
	plugin     *glightning.Plugin

	msgHandlers []func(peerId string, message string) error
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

func (c *ClightningClient) SendMessage(peerId string, message []byte) error {
	msg := customMsgType + hex.EncodeToString(message)
	res, err := c.glightning.SendCustomMessage(peerId, msg)
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return errors.New(res.Message)
	}
	return nil
}

func (c *ClightningClient) AddMessageHandler(f func(peerId string, message string) error) error {
	c.msgHandlers = append(c.msgHandlers, f)
	return nil
}

func (c *ClightningClient) GetPayreq(amount uint64, preImage, pHash string) (string, error) {
	panic("implement me")
}

func (c *ClightningClient) DecodePayreq(payreq string) (*Invoice, error) {
	panic("implement me")
}

func (c *ClightningClient) PayInvoice(payreq string) (preimage string, err error) {
	panic("implement me")
}

func (c *ClightningClient) OnCustomMsg(event *glightning.CustomMsgReceivedEvent) (*glightning.CustomMsgReceivedResponse, error) {
	log.Printf("new custom msg. peer: %s, payload: %s", event.PeerId, event.Payload)
	for _, v := range c.msgHandlers {
		err := v(event.PeerId, event.Payload)
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

func (c *ClightningClient) RegisterMethods(wallet WalletService) error {
	loopIn := glightning.NewRpcMethod(&LoopIn{
		wallet: wallet,
		pc:     c,
	}, "Loop In")
	loopIn.Category = "liquid-loop"
	err := c.plugin.RegisterMethod(loopIn)
	if err != nil {
		return err
	}

	getAddress := glightning.NewRpcMethod(&GetAddressMethod{
		wallet: wallet,
	}, "get new liquid address")
	loopIn.Category = "liquid-loop"
	err = c.plugin.RegisterMethod(getAddress)
	if err != nil {
		return err
	}

	getBalance := glightning.NewRpcMethod(&GetBalanceMethod{
		wallet: wallet,
	}, "get liquid balance")
	loopIn.Category = "liquid-loop"
	err = c.plugin.RegisterMethod(getBalance)
	if err != nil {
		return err
	}

	listUtxos := glightning.NewRpcMethod(&ListUtxosMethod{
		wallet: wallet,
	}, "list liquid utxos")
	loopIn.Category = "liquid-loop"
	err = c.plugin.RegisterMethod(listUtxos)
	if err != nil {
		return err
	}
	return nil
}
