package main

import (
	"flag"
	"path/filepath"

	"github.com/sputn1ck/peerswap/swap"
)

func main() {
	dir := flag.String("dir", "./", "destination directory")
	flag.Parse()

	swap.SwapInSenderStatesToMermaid(filepath.Join(*dir, "swap-in-sender-states.md"))
	swap.SwapInReceiverStatesToMermaid(filepath.Join(*dir, "swap-in-receiver-states.md"))
	swap.SwapOutSenderStatesToMermaid(filepath.Join(*dir, "swap-out-sender-states.md"))
	swap.SwapOutReceiverStatesToMermaid(filepath.Join(*dir, "swap-out-receiver-states.md"))
}
