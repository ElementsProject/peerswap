package main

import (
	"flag"
	"path/filepath"

	"github.com/sputn1ck/peerswap/swap"
)

func main() {
	dir := flag.String("dir", "./", "destination directory")
	flag.Parse()

	swap.SwapInReceiverStatesToMermaid(filepath.Join(*dir, "states_swapin_receiver.md"))
	swap.SwapOutReceiverStatesToMermaid(filepath.Join(*dir, "states_swapout_receiver.md"))
	swap.SwapInSenderStatesToMermaid(filepath.Join(*dir, "states_swapin_sender.md"))
	swap.SwapOutSenderStatesToMermaid(filepath.Join(*dir, "states_swapout_sender.md"))
}
