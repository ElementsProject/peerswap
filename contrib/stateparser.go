package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/elementsproject/peerswap/swap"
)

func main() {
	out := flag.String("out", "", "outfile")
	stateMachine := flag.String("fsm", "", "the swap state machine to parse")
	flag.Parse()

	if filepath.Ext(*out) != ".md" {
		fmt.Println("Wrong argument: out must be a .md file")
		os.Exit(1)
	}

	fp, err := filepath.Abs(*out)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	switch *stateMachine {
	case "swap_in_sender":
		swap.SwapInSenderStatesToMermaid(fp)
	case "swap_in_receiver":
		swap.SwapInReceiverStatesToMermaid(fp)
	case "swap_out_sender":
		swap.SwapOutSenderStatesToMermaid(fp)
	case "swap_out_receiver":
		swap.SwapOutReceiverStatesToMermaid(fp)
	default:
		fmt.Println("Missing or wrong argument: fsm must be one of:")
		fmt.Println("\tswap_in_sender")
		fmt.Println("\tswap_in_receiver")
		fmt.Println("\tswap_out_sender")
		fmt.Println("\tswap_out_receiver")
		os.Exit(1)
	}
}
