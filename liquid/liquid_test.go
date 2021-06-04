package liquid

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/txscript"
	"github.com/sputn1ck/liquid-loop/gelements"
	"github.com/sputn1ck/liquid-loop/lightning"
	"github.com/vulpemventures/go-elements/payment"
	"github.com/vulpemventures/go-elements/pset"
	"github.com/vulpemventures/go-elements/transaction"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/vulpemventures/go-elements/elementsutil"
	"github.com/vulpemventures/go-elements/network"

	"testing"
)

var lbtc = append(
	[]byte{0x01},
	elementsutil.ReverseBytes(h2b(network.Regtest.AssetID))...,
)


func Test_Loop_TimelockCase(t *testing.T) {
	var preimage lightning.Preimage

	if _, err := rand.Read(preimage[:]); err != nil {
		t.Fatal(err)
	}
	pHash := preimage.Hash()

	// Generating Alices Keys and Address
	privkeyAlice, err := btcec.NewPrivateKey(btcec.S256())
	if err != nil {
		t.Fatal(err)
	}
	pubkeyAlice := privkeyAlice.PubKey()
	p2pkhAlice := payment.FromPublicKey(pubkeyAlice, &network.Regtest, nil)
	_, _ = p2pkhAlice.PubKeyHash()


	// Generating Bob Keys and Address
	privkeyBob, err := btcec.NewPrivateKey(btcec.S256())
	if err != nil {
		t.Fatal(err)
	}

	pubkeyBob := privkeyBob.PubKey()
	p2pkhBob := payment.FromPublicKey(pubkeyBob, &network.Regtest, nil)
	addressBob, _ := p2pkhBob.PubKeyHash()

	// Fund Bob address with LBTC.
	if _, err := faucet(addressBob); err != nil {
		t.Fatal(err)
	}

	// Retrieve Bob utxos.
	utxosBob, err := unspents(addressBob)
	if err != nil {
		t.Fatal(err)
	}

	// First Transaction
	// 1 Input
	txInputHashBob := elementsutil.ReverseBytes(h2b(utxosBob[0]["txid"].(string)))
	txInputIndexBob := uint32(utxosBob[0]["vout"].(float64))
	txInputBob := transaction.NewTxInput(txInputHashBob, txInputIndexBob)

	// 3 outputs Script, Change, fee
	// Fees
	feeValue, _ := elementsutil.SatoshiToElementsValue(500)
	feeScript := []byte{}
	feeOutput := transaction.NewTxOutput(lbtc, feeValue, feeScript)

	// Change from/to Bob
	changeScriptBob := p2pkhBob.Script
	changeValueBob, _ := elementsutil.SatoshiToElementsValue(39999500)
	changeOutputBob := transaction.NewTxOutput(lbtc, changeValueBob[:], changeScriptBob)

	// calc cltv
	locktime := 5
	blockHeight, err := getBestBlock()
	if err != nil {
		t.Fatal(err)
	}
	spendingBlockHeight := int64(blockHeight + locktime)
	// P2WSH script
	// miniscript: or(and(pk(A),sha256(H)),and(pk(B), after(100)))
	script := txscript.NewScriptBuilder().
		AddData(pubkeyAlice.SerializeCompressed()).
		AddOp(txscript.OP_CHECKSIG).
		AddOp(txscript.OP_NOTIF).
		AddData(pubkeyBob.SerializeCompressed()).
		AddOp(txscript.OP_CHECKSIGVERIFY).
		AddInt64(spendingBlockHeight).
		AddOp(txscript.OP_CHECKLOCKTIMEVERIFY).
		AddOp(txscript.OP_ELSE).
		AddOp(txscript.OP_SIZE).
		AddData(h2b("20")).
		AddOp(txscript.OP_EQUALVERIFY).
		AddOp(txscript.OP_SHA256).
		AddData(pHash[:]).
		AddOp(txscript.OP_EQUAL).
		AddOp(txscript.OP_ENDIF)

	redeemScript, err := script.Script()
	if err != nil {
		t.Fatal(err)
	}

	redeemPayment, err := payment.FromPayment(&payment.Payment{
		Script:  redeemScript,
		Network: &network.Regtest,
	})
	if err != nil {
		t.Fatal(err)
	}
	satsToSpend := uint64(60000000)
	loopInValue, _ := elementsutil.SatoshiToElementsValue(satsToSpend)
	output := transaction.NewTxOutput(lbtc, loopInValue, redeemPayment.WitnessScript)

	// Create a new pset
	inputs := []*transaction.TxInput{txInputBob}
	outputs := []*transaction.TxOutput{output, changeOutputBob, feeOutput}
	p, err := pset.New(inputs, outputs, 2, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Add sighash type and witness utxo to the partial input.
	updater, err := pset.NewUpdater(p)
	if err != nil {
		t.Fatal(err)
	}

	bobspendingTxHash, err := fetchTx(utxosBob[0]["txid"].(string))
	if err != nil {
		t.Fatal(err)
	}
	bobFaucetTx, _ := transaction.NewTxFromHex(bobspendingTxHash)

	err = updater.AddInNonWitnessUtxo(bobFaucetTx, 0)
	if err != nil {
		t.Fatal(err)
	}

	prvKeys := []*btcec.PrivateKey{privkeyBob}
	scripts := [][]byte{p2pkhBob.Script}
	if err := signTransaction(p, prvKeys, scripts, false, nil); err != nil {
		t.Fatal(err)
	}

	// Finalize the partial transaction.
	if err := pset.FinalizeAll(p); err != nil {
		t.Fatal(err)
	}
	// Extract the final signed transaction from the Pset wrapper.
	finalTx, err := pset.Extract(p)
	if err != nil {
		t.Fatal(err)
	}
	// Serialize the transaction and try to broadcast.
	txHex, err := finalTx.ToHex()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := broadcast(txHex)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(finalTx.WitnessHash())
	t.Log(finalTx.TxHash())
	t.Log(tx)

	// let some block pass
	err = generate(uint(locktime))
	if err != nil {
		t.Fatal(err)
	}

	blockHeight, err = getBestBlock()
	if err != nil {
		t.Fatal(err)
	}
	// second transaction
	firstTxHash := finalTx.WitnessHash()
	spendingInput := transaction.NewTxInput(firstTxHash[:], 0)
	spendingInput.Sequence = 0
	spendingSatsBytes, _ := elementsutil.SatoshiToElementsValue(satsToSpend - 500)
	spendingOutput := transaction.NewTxOutput(lbtc, spendingSatsBytes[:], p2pkhBob.Script)

	spendingTx := &transaction.Transaction{
		Version:  2,
		Flag:     0,
		Locktime: uint32(spendingBlockHeight),
		Inputs:   []*transaction.TxInput{spendingInput},
		Outputs:  []*transaction.TxOutput{spendingOutput, feeOutput},
	}

	var sigHash [32]byte

	sigHash = spendingTx.HashForWitnessV0(
		0,
		redeemScript[:],
		loopInValue,
		txscript.SigHashAll,
	)
	sig, err := privkeyBob.Sign(sigHash[:])
	if err != nil {
		t.Fatal(err)
	}
	sigWithHashType := append(sig.Serialize(), byte(txscript.SigHashAll))
	witness := make([][]byte, 0)

	witness = append(witness, sigWithHashType[:])
	witness = append(witness, []byte{})
	witness = append(witness, redeemScript)
	spendingTx.Inputs[0].Witness =  witness


	spendingTxHex, err := spendingTx.ToHex()
	if err != nil {
		t.Fatal(err)
	}

	t.Log(spendingTxHex)
	t.Log(spendingTx.Locktime)
	_, err = broadcast(spendingTxHex)
	if err != nil  {
		t.Fatal(err)
	}
}
func Test_Loop_PreimageClaim(t *testing.T) {
	var preimage lightning.Preimage

	if _, err := rand.Read(preimage[:]); err != nil {
		t.Fatal(err)
	}
	pHash := preimage.Hash()

	// Generating Alices Keys and Address
	privkeyAlice, err := btcec.NewPrivateKey(btcec.S256())
	if err != nil {
		t.Fatal(err)
	}
	pubkeyAlice := privkeyAlice.PubKey()
	p2pkhAlice := payment.FromPublicKey(pubkeyAlice, &network.Regtest, nil)
	_, _ = p2pkhAlice.PubKeyHash()


	// Generating Bob Keys and Address
	privkeyBob, err := btcec.NewPrivateKey(btcec.S256())
	if err != nil {
		t.Fatal(err)
	}

	pubkeyBob := privkeyBob.PubKey()
	p2pkhBob := payment.FromPublicKey(pubkeyBob, &network.Regtest, nil)
	addressBob, _ := p2pkhBob.PubKeyHash()

	// Fund Bob address with LBTC.
	if _, err := faucet(addressBob); err != nil {
		t.Fatal(err)
	}

	// Retrieve Alice utxos.
	utxosBob, err := unspents(addressBob)
	if err != nil {
		t.Fatal(err)
	}

	// First Transaction
	// 1 Input
	txInputHashBob := elementsutil.ReverseBytes(h2b(utxosBob[0]["txid"].(string)))
	txInputIndexBob := uint32(utxosBob[0]["vout"].(float64))
	txInputBob := transaction.NewTxInput(txInputHashBob, txInputIndexBob)

	// 3 outputs Script, Change, fee
	// Fees
	feeValue, _ := elementsutil.SatoshiToElementsValue(500)
	feeScript := []byte{}
	feeOutput := transaction.NewTxOutput(lbtc, feeValue, feeScript)

	// Change from/to Bob
	changeScriptBob := p2pkhBob.Script
	changeValueBob, _ := elementsutil.SatoshiToElementsValue(39999500)
	changeOutputBob := transaction.NewTxOutput(lbtc, changeValueBob[:], changeScriptBob)

	// P2WSH script
	// miniscript: or(and(pk(A),sha256(H)),pk(B))
	script := txscript.NewScriptBuilder().
		AddData(pubkeyAlice.SerializeCompressed()).
		AddOp(txscript.OP_CHECKSIG).
		AddOp(txscript.OP_NOTIF).
		AddData(pubkeyBob.SerializeCompressed()).
		AddOp(txscript.OP_CHECKSIG).
		AddOp(txscript.OP_ELSE).
		AddOp(txscript.OP_SIZE).
		AddData(h2b("20")).
		AddOp(txscript.OP_EQUALVERIFY).
		AddOp(txscript.OP_SHA256).
		AddData(pHash[:]).
		AddOp(txscript.OP_EQUAL).
		AddOp(txscript.OP_ENDIF)

	redeemScript, err := script.Script()
	if err != nil {
		t.Fatal(err)
	}

	redeemPayment, err := payment.FromPayment(&payment.Payment{
		Script:  redeemScript,
		Network: &network.Regtest,

	})
	if err != nil {
		t.Fatal(err)
	}
	satsToSpend := uint64(60000000)
	loopInValue, _ := elementsutil.SatoshiToElementsValue(satsToSpend)
	output := transaction.NewTxOutput(lbtc, loopInValue, redeemPayment.WitnessScript)

	// Create a new pset
	inputs := []*transaction.TxInput{txInputBob}
	outputs := []*transaction.TxOutput{output, changeOutputBob, feeOutput}
	p, err := pset.New(inputs, outputs, 2, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Add sighash type and witness utxo to the partial input.
	updater, err := pset.NewUpdater(p)
	if err != nil {
		t.Fatal(err)
	}

	bobspendingTxHash, err := fetchTx(utxosBob[0]["txid"].(string))
	if err != nil {
		t.Fatal(err)
	}
	bobFaucetTx, _ := transaction.NewTxFromHex(bobspendingTxHash)

	err = updater.AddInNonWitnessUtxo(bobFaucetTx, 0)
	if err != nil {
		t.Fatal(err)
	}

	prvKeys := []*btcec.PrivateKey{privkeyBob}
	scripts := [][]byte{p2pkhBob.Script}
	if err := signTransaction(p, prvKeys, scripts, false, nil); err != nil {
		t.Fatal(err)
	}

	// Finalize the partial transaction.
	if err := pset.FinalizeAll(p); err != nil {
		t.Fatal(err)
	}
	// Extract the final signed transaction from the Pset wrapper.
	finalTx, err := pset.Extract(p)
	if err != nil {
		t.Fatal(err)
	}
	// Serialize the transaction and try to broadcast.
	txHex, err := finalTx.ToHex()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := broadcast(txHex)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(tx)

	// second transaction
	firstTxHash := finalTx.WitnessHash()
	spendingInput := transaction.NewTxInput(firstTxHash[:], 0)
	spendingSatsBytes, _ := elementsutil.SatoshiToElementsValue(satsToSpend - 500)
	spendingOutput := transaction.NewTxOutput(lbtc, spendingSatsBytes[:], p2pkhAlice.Script)

	spendingTx := &transaction.Transaction{
		Version:  2,
		Flag:     0,
		Locktime: 0,
		Inputs:   []*transaction.TxInput{spendingInput},
		Outputs:  []*transaction.TxOutput{spendingOutput, feeOutput},
	}

	var sigHash [32]byte

	sigHash = spendingTx.HashForWitnessV0(
		0,
		redeemScript[:],
		loopInValue,
		txscript.SigHashAll,
	)
	//sig, err := privkeyBob.Sign(sigHash[:])
	//if err != nil {
	//	t.Fatal(err)
	//}
	sig, err := privkeyAlice.Sign(sigHash[:])
	if err != nil {
		t.Fatal(err)
	}
	sigWithHashType := append(sig.Serialize(), byte(txscript.SigHashAll))
	witness := make([][]byte, 0)

	witness = append(witness, preimage[:])
	witness = append(witness, sigWithHashType[:])
	//witness = append(witness, []byte{})
	witness = append(witness, redeemScript)
	spendingTx.Inputs[0].Witness =  witness


	spendingTxHex, err := spendingTx.ToHex()
	if err != nil {
		t.Fatal(err)
	}

	t.Log(spendingTxHex)

	res, err := broadcast(spendingTxHex)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(res)
}





func signTransaction(
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

func faucet(address string) (string, error) {
	baseURL, err := apiBaseUrl()
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/faucet", baseURL)
	payload := map[string]string{"address": address}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if res := string(data); len(res) <= 0 || strings.Contains(res, "sendtoaddress") {
		return "", fmt.Errorf("cannot fund address with faucet: %s", res)
	}

	respBody := map[string]string{}
	if err := json.Unmarshal(data, &respBody); err != nil {
		return "", err
	}

	return respBody["txId"], nil
}

func getBestBlock() (int, error) {
	elements := gelements.NewElements("admin1","123")
	elements.StartUp("http://localhost", 7041)

	res, err := elements.GetBlockCount()
	if err != nil {
		return 0, err
	}
	return res, nil
}

func Test_BestBlock(t *testing.T) {
	bestblock, err := getBestBlock()
	if err != nil {
		t.Fatal(err)
	}
	t.Log(bestblock)
}


func unspents(address string) ([]map[string]interface{}, error) {
	getUtxos := func(address string) ([]interface{}, error) {
		baseUrl, err := apiBaseUrl()
		if err != nil {
			return nil, err
		}
		url := fmt.Sprintf("%s/address/%s/utxo", baseUrl, address)
		resp, err := http.Get(url)
		if err != nil {
			return nil, err
		}
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		var respBody interface{}
		if err := json.Unmarshal(data, &respBody); err != nil {
			return nil, err
		}
		return respBody.([]interface{}), nil
	}

	utxos := []map[string]interface{}{}
	for len(utxos) <= 0 {
		time.Sleep(1 * time.Second)
		u, err := getUtxos(address)
		if err != nil {
			return nil, err
		}
		for _, unspent := range u {
			utxo := unspent.(map[string]interface{})
			utxos = append(utxos, utxo)
		}
	}

	return utxos, nil
}

func broadcast(txHex string) (string, error) {
	//elements := gelements.NewElements("admin1","123")
	//elements.StartUp("http://localhost", 7041)
	//
	//res, err := elements.SendRawTx(txHex)
	//if err != nil {
	//	return "",err
	//}
	//return res, nil
	baseUrl, err := apiBaseUrl()
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/tx", baseUrl)

	resp, err := http.Post(url, "text/plain", strings.NewReader(txHex))
	if err != nil {
		return "", err
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	res := string(data)
	if len(res) <= 0 || strings.Contains(res, "sendrawtransaction") {
		return "", fmt.Errorf("failed to broadcast tx: %s", res)
	}
	return res, nil
}

func generate(numBlocks uint) ( error) {
	elements := gelements.NewElements("admin1","123")
	elements.StartUp("http://localhost", 7041)

	_, err := elements.GenerateToAddress("XYYena4XzRaexwmqv6HbDQgjfT7sEkx2y9", numBlocks)
	if err != nil {
		return err
	}
	return nil
}

func fetchTx(txId string) (string, error) {
	baseUrl, err := apiBaseUrl()
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/tx/%s/hex", baseUrl, txId)

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func apiBaseUrl() (string, error) {
	return "http://localhost:3001", nil
}

func b2h(buf []byte) string {
	return hex.EncodeToString(buf)
}

