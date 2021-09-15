package swap

import (
	"bytes"
	"fmt"
	"os"
)

func writeMermaidFile(filename string, states States) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	var b bytes.Buffer
	fmt.Fprint(&b, "```mermaid\nstateDiagram-v2\n")

	counter := 0
	for state, edges := range states {
		if len(state) > 0 {
			fmt.Fprintf(&b, "%s\n", state)
		} else {
			state = "[*]"
		}
		for edge, target := range edges.Events {
			fmt.Fprintf(&b, "%s --> %s: %s\n", state, target, edge)
		}
		counter++
	}
	fmt.Fprint(&b, "```")
	f.Write(b.Bytes())

	return nil
}

func SwapInSenderStatesToMermaid(filename string) error {
	return writeMermaidFile(filename, getSwapInSenderStates())
}

func SwapInReceiverStatesToMermaid(filename string) error {
	return writeMermaidFile(filename, getSwapInReceiverStates())
}

func SwapOutSenderStatesToMermaid(filename string) error {
	return writeMermaidFile(filename, getSwapOutSenderStates())
}
func SwapOutReceiverStatesToMermaid(filename string) error {
	return writeMermaidFile(filename, getSwapOutReceiverStates())
}
