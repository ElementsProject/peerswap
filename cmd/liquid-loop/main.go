package main

import (
	"errors"
	"fmt"
	"github.com/sputn1ck/liquid-loop/glightning"
	"github.com/sputn1ck/liquid-loop/jrpc2"
	"log"
	"math/big"
	"os"
)


var lightning *glightning.Lightning
var plugin *glightning.Plugin

// ok, let's try plugging this into c-lightning
func main() {
	plugin = glightning.NewPlugin(onInit)
	lightning = glightning.NewLightning()

	var b big.Int
	b.Exp(big.NewInt(2), big.NewInt(112), nil)
	plugin.AddNodeFeatures(b.Bytes())
	plugin.SetDynamic(true)

	registerOptions(plugin)
	registerMethods(plugin)
	
	err := plugin.Start(os.Stdin, os.Stdout)
	if err != nil {
		log.Fatal(err)
	}
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

func registerMethods(p *glightning.Plugin) {
	rpcHello := glightning.NewRpcMethod(&LoopIn{}, "Loop In")
	rpcHello.LongDesc = `Send a message! Name is set
by the 'name' option, passed in at startup `
	rpcHello.Category = "utility"
	p.RegisterMethod(rpcHello)

}

type LoopIn struct {
	Amt int64 `json:"amt"`
	Peer string `json:"peer"`
}

func (l *LoopIn) New() interface{} {
	return &LoopIn{}
}

func (l *LoopIn) Name() string {
	return "loop-in"
}

func (l *LoopIn) Call() (jrpc2.Result, error) {
	if l.Amt <= 0 {
		return nil, errors.New("Missing required amount parameter")
	}
	if l.Peer == "" {
		return nil, errors.New("Missing required peer parameter")
	}
	hops := []glightning.OnionMessageHop{
		{Id:     l.Peer,},
	}
	res, err := lightning.SendOnionMessage(hops)
	return fmt.Sprintf("loop-in called! %v %v %s %s %v", l.Amt, l.Peer, res, err), nil
}