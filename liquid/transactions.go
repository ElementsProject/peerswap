package liquid

import (
	"encoding/hex"
	"errors"
	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/txscript"
	"github.com/vulpemventures/go-elements/elementsutil"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/pset"
	"github.com/vulpemventures/go-elements/transaction"
)

var LBTC = append(
	[]byte{0x01},
	elementsutil.ReverseBytes(h2b(network.Regtest.AssetID))...,
)

func GetFeeOutput(fee uint64) (*transaction.TxOutput, error){
	feeValue, err := elementsutil.SatoshiToElementsValue(fee)
	if err != nil {
		return nil, err
	}
	feeScript := []byte{}
	feeOutput := transaction.NewTxOutput(LBTC, feeValue, feeScript)
	return feeOutput, nil
}


// GetOpeningTxScript returns the script for the opening transaction of a swap,
// where the taker is the peer paying the invoice and the maker the peer providing the lbtc
func GetOpeningTxScript(takerPubkey *btcec.PublicKey, makerPubkey *btcec.PublicKey, pHash []byte, locktimeHeight int64) ([]byte, error) {
	script := txscript.NewScriptBuilder().
		AddData(takerPubkey.SerializeCompressed()).
		AddOp(txscript.OP_CHECKSIG).
		AddOp(txscript.OP_NOTIF).
		AddData(makerPubkey.SerializeCompressed()).
		AddOp(txscript.OP_CHECKSIGVERIFY).
		AddInt64(locktimeHeight).
		AddOp(txscript.OP_CHECKLOCKTIMEVERIFY).
		AddOp(txscript.OP_ELSE).
		AddOp(txscript.OP_SIZE).
		AddData(h2b("20")).
		AddOp(txscript.OP_EQUALVERIFY).
		AddOp(txscript.OP_SHA256).
		AddData(pHash[:]).
		AddOp(txscript.OP_EQUAL).
		AddOp(txscript.OP_ENDIF)
	return script.Script()
}
type signOpts struct {
	pubkeyScript []byte
	script       []byte
}

func SignTransaction(
	p *pset.Pset,
	privKeys []*btcec.PrivateKey,
	scripts [][]byte,
	forWitness bool,
	opts *signOpts,
) error {
	updater, err := pset.NewUpdater(p)
	if err != nil {
		return err
	}

	for i, in := range p.Inputs {
		if err := updater.AddInSighashType(txscript.SigHashAll, i); err != nil {
			return err
		}

		var prevout *transaction.TxOutput
		if in.WitnessUtxo != nil {
			prevout = in.WitnessUtxo
		} else {
			prevout = in.NonWitnessUtxo.Outputs[p.UnsignedTx.Inputs[i].Index]
		}
		prvkey := privKeys[i]
		pubkey := prvkey.PubKey()
		script := scripts[i]

		var sigHash [32]byte
		if forWitness {
			sigHash = p.UnsignedTx.HashForWitnessV0(
				i,
				script,
				prevout.Value,
				txscript.SigHashAll,
			)
		} else {
			sigHash, err = p.UnsignedTx.HashForSignature(i, script, txscript.SigHashAll)
			if err != nil {
				return err
			}
		}

		sig, err := prvkey.Sign(sigHash[:])
		if err != nil {
			return err
		}
		sigWithHashType := append(sig.Serialize(), byte(txscript.SigHashAll))

		var witPubkeyScript []byte
		var witScript []byte
		if opts != nil {
			witPubkeyScript = opts.pubkeyScript
			witScript = opts.script
		}

		if _, err := updater.Sign(
			i,
			sigWithHashType,
			pubkey.SerializeCompressed(),
			witPubkeyScript,
			witScript,
		); err != nil {
			return err
		}
	}

	valid, err := p.ValidateAllSignatures()
	if err != nil {
		return err
	}
	if !valid {
		return errors.New("invalid signatures")
	}

	return nil
}

func h2b(str string) []byte {
	buf, _ := hex.DecodeString(str)
	return buf
}