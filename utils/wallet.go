package utils

import (
	"github.com/btcsuite/btcd/txscript"
	"github.com/vulpemventures/go-elements/address"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/payment"
)

// Blech32ToScript returns an elements script from a Blech32 Address
func Blech32ToScript(blech32Addr string, network *network.Network) ([]byte, error) {
	blechAddr, err := address.FromBlech32(blech32Addr)
	if err != nil {
		return nil, err
	}
	blechscript, err := txscript.NewScriptBuilder().AddOp(txscript.OP_0).AddData(blechAddr.Program).Script()
	if err != nil {
		return nil, err
	}
	blechPayment, err := payment.FromScript(blechscript[:], network, nil)
	if err != nil {
		return nil, err
	}
	return blechPayment.WitnessScript, nil
}
