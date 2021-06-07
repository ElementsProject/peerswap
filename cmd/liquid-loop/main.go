package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/btcec"
	"github.com/niftynei/glightning/glightning"
	"github.com/niftynei/glightning/jrpc2"
	"github.com/sputn1ck/liquid-loop/liquid"
	"github.com/sputn1ck/liquid-loop/wallet"
	"log"
	"math/big"
	"os"
)
const (
	dataType = "aaff"
)

var (
	lightning *glightning.Lightning
	plugin *glightning.Plugin
	walletService *wallet.LiquiddWallet
	esplora *liquid.EsploraClient
	walletStore wallet.WalletStore
)


// ok, let's try plugging this into c-lightning
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}

}
func run() error {
	err := SetupPlugin()
	if err != nil {
		return err
	}
	err = SetupServices()
	if err != nil {
		return err
	}
	err = plugin.Start(os.Stdin, os.Stdout)
	if err != nil {
		return err
	}
	return nil
}
func SetupServices() error {
	privkey, err := btcec.NewPrivateKey(btcec.S256())
	if err != nil {
		return err
	}
	esplora = liquid.NewEsploraClient("http://localhost:3001")
	walletStore = &wallet.DummyWalletStore{PrivKey: privkey}
	walletService = &wallet.LiquiddWallet{Store: walletStore, Blockchain: esplora}
	blockheight, err := esplora.GetBlockHeight()
	if err != nil {
		return err
	}
	log.Printf("BLOCKHEIGHT!!! %v", blockheight)
	return nil
}
func SetupPlugin() error {
	// Setup Plugin
	plugin = glightning.NewPlugin(onInit)
	err := plugin.RegisterHooks(&glightning.Hooks{
		CustomMsgReceived: OnCustomMsg,
	})
	if err != nil {
		log.Fatal(err)
	}
	lightning = glightning.NewLightning()

	var b big.Int
	b.Exp(big.NewInt(2), big.NewInt(112), nil)
	plugin.AddNodeFeatures(b.Bytes())
	plugin.SetDynamic(true)

	registerOptions(plugin)
	registerMethods(plugin)

	return nil
}

func OnCustomMsg(event *glightning.CustomMsgReceivedEvent) (*glightning.CustomMsgReceivedResponse, error) {
	log.Printf("new custom msg. peer: %s, payload: %s", event.PeerId, event.Payload)
	return event.Continue(), nil
}

// This is called after the plugin starts up successfully
func onInit(plugin *glightning.Plugin, options map[string]glightning.Option, config *glightning.Config) {
	log.Printf("successfully init'd! %s\n", config.RpcFile)

	// Here's how you'd use the config's lightning-dir to
	//   start up an RPC client for the node.
	lightning.StartUp(config.RpcFile, config.LightningDir)
	channels, _ := lightning.ListChannels()
	log.Printf("You know about %d channels", len(channels))

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

func registerOptions(p *glightning.Plugin) {
	p.RegisterNewOption("db_path", "path to boltdb", "~/.liquid-loop/db")
	p.RegisterNewOption("liquid_url", "url to liquid daemon", "")
}

type GetAddressMethod struct {}

func (g *GetAddressMethod) New() interface{} {
	return &GetAddressMethod{}
}

func (g *GetAddressMethod) Name() string {
	return "liquid-wallet-getaddresss"
}

func (g *GetAddressMethod) Call() (jrpc2.Result, error) {
	res, err := walletService.ListAddresses()
	if err != nil {
		return nil, err
	}
	return res, nil
}

type GetBalanceMethod struct {}

func (g *GetBalanceMethod) Name() string {
	return "liquid-wallet-getbalance"
}

func (g *GetBalanceMethod) New() interface{} {
	return &GetBalanceMethod{}
}

func (g *GetBalanceMethod) Call() (jrpc2.Result, error) {
	res, err := walletService.GetBalance()
	if err != nil {
		return nil,err
	}
	return res, nil
}

type ListUtxosMethod struct {

}

func (l *ListUtxosMethod) Name() string {
	return "liquid-wallet-listutxos"
}

func (l *ListUtxosMethod) New() interface{} {
	return &ListUtxosMethod{}
}

func (l *ListUtxosMethod) Call() (jrpc2.Result, error) {
	utxos, err := walletService.ListUtxos()
	if err != nil {
		return nil,err
	}
	return utxos, nil
}


type LoopIn struct {
	Amt int64 `json:"amt"`
	Peer string `json:"peer"`
}

type LoopInData struct {
	Amt int64 `json:"amt"`
	Msg string `json:"msg"`
}

func (l *LoopIn) New() interface{} {
	return &LoopIn{}
}

func (l *LoopIn) Name() string {
	return "loop-in"
}

func (l *LoopIn) Call() (jrpc2.Result, error) {
	if l.Amt <= 0 {
		return nil, errors.New("Missing required amt parameter")
	}
	if l.Peer == "" {
		return nil, errors.New("Missing required peer parameter")
	}
	bytes, err := json.Marshal(&LoopInData{
		Amt: l.Amt,
		Msg: "Gudee",
	})
	if err != nil {
		return nil, err
	}
	msg := dataType + hex.EncodeToString(bytes)
	res, err := lightning.SendCustomMessage(l.Peer, msg)
	if err != nil {
		return nil, err
	}
	return fmt.Sprintf("loop-in called! %v %s %s %v %v", l.Amt, l.Peer, msg, res, err), nil
}

func registerMethods(p *glightning.Plugin) {
	loopIn := glightning.NewRpcMethod(&LoopIn{}, "Loop In")
	loopIn.Category = "liquid-loop"
	p.RegisterMethod(loopIn)

	getAddress := glightning.NewRpcMethod(&GetAddressMethod{}, "get new liquid address")
	loopIn.Category = "liquid-loop"
	p.RegisterMethod(getAddress)

	getBalance := glightning.NewRpcMethod(&GetBalanceMethod{}, "get liquid balance")
	loopIn.Category = "liquid-loop"
	p.RegisterMethod(getBalance)

	listUtxos := glightning.NewRpcMethod(&ListUtxosMethod{}, "list liquid utxos")
	loopIn.Category = "liquid-loop"
	p.RegisterMethod(listUtxos)

}
func constructError(err error) *jrpc2.RpcError {
	// todo: specify return data?
	return &jrpc2.RpcError{
		Code:    -1,
		Message: err.Error(),
	}
}

func constructRes(msg string) jrpc2.Result {
	result := &struct {
		Result string `json:"hi"`
		// If you want the result returned to be 'simply' formatted
		// return a field called "format-hint" set to `FormatSimple`
		FormatHint string `json:"format-hint,omitempty"`
	}{
		Result:     fmt.Sprintf("\n\tHowdy %s!\n\n", msg),
		FormatHint: glightning.FormatSimple,
	}
	return result
}



