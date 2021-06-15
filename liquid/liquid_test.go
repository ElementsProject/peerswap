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
	"github.com/sputn1ck/sugarmama/lightning"
	"github.com/sputn1ck/sugarmama/wallet"
	"github.com/stretchr/testify/assert"
	"github.com/vulpemventures/go-elements/payment"
	"github.com/vulpemventures/go-elements/pset"
	"github.com/vulpemventures/go-elements/transaction"
	"io/ioutil"
	"log"
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
var (
	alicePrivkey = "b5ca71cc0ea0587fc40b3650dfb12c1e50fece3b88593b223679aea733c55605"
	esplora      = NewEsploraClient("http://localhost:3001")
)

func Test_Wallet(t *testing.T) {
	//privkeyBytes, err := hex.DecodeString(alicePrivkey)
	//if err != nil {
	//	t.Fatal(err)
	//}
	//privkey,_ := btcec.PrivKeyFromBytes(btcec.S256(), privkeyBytes)

	walletStore := &wallet.DummyWalletStore{}
	walletStore.Initialize()
	walletService := wallet.NewLiquiddWallet(walletStore, esplora, &network.Regtest)
	addresses, err := walletStore.ListAddresses()
	if err != nil {
		t.Fatal(err)
	}

	nextBalanceChan := make(chan uint64)
	balance, err := waitBalanceChange(walletService, nextBalanceChan)
	if err != nil {
		t.Fatal(err)
	}

	_, err = faucet(addresses[0], 1)
	if err != nil {
		t.Fatal(err)
	}
	balance = <-nextBalanceChan
	wantBalance := uint64(100000000)
	if !assert.Equal(t, wantBalance, balance) {
		t.Fatalf("balance wanted: %v, got %v \n", wantBalance, balance)
	}
	balance, err = waitBalanceChange(walletService, nextBalanceChan)
	if err != nil {
		t.Fatal(err)
	}
	_, err = faucet(addresses[0], 1)
	if err != nil {
		t.Fatal(err)
	}
	balance = <-nextBalanceChan
	wantBalance = uint64(200000000)

	if !assert.Equal(t, wantBalance, balance) {
		t.Fatalf("balance wanted: %v, got %v \n", wantBalance, balance)
	}

}
func Test_InputStuff(t *testing.T) {

	// Generating Bob Keys and Address

	bobStore := &wallet.DummyWalletStore{}
	bobStore.Initialize()
	bobWallet := wallet.NewLiquiddWallet(bobStore, esplora, &network.Regtest)

	privkeyBob, err := bobStore.LoadPrivKey()
	if err != nil {
		t.Fatal(err)
	}

	pubkeyBob := privkeyBob.PubKey()
	p2pkhBob := payment.FromPublicKey(pubkeyBob, &network.Regtest, nil)
	addressBob, _ := p2pkhBob.PubKeyHash()

	balanceChan := make(chan uint64)
	_, err = waitBalanceChange(bobWallet, balanceChan)
	if err != nil {
		t.Fatal(err)
	}
	// Fund Bob address with LBTC.
	if _, err := faucet(addressBob, 1); err != nil {
		t.Fatal(err)
	}

	<-balanceChan

	// Retrieve bob utxos.
	satsToSpend := uint64(60000000)
	fee := uint64(500)
	utxosBob, change, err := bobWallet.GetUtxos(satsToSpend)
	if err != nil {
		t.Fatal(err)
	}
	// First Transaction
	// 1 Input
	txInputHashBob := elementsutil.ReverseBytes(h2b(utxosBob[0].TxId))
	txInputIndexBob := utxosBob[0].VOut
	txInputBob := transaction.NewTxInput(txInputHashBob, txInputIndexBob)
	txinputs, err := esplora.WalletUtxosToTxInputs(utxosBob)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(txInputBob.Hash, txinputs[0].Hash) != 0 {
		t.Fatal("txinputs not equal")
	}
	// 3 outputs Script, Change, fee
	// Fees
	feeValue, _ := elementsutil.SatoshiToElementsValue(fee)
	feeScript := []byte{}
	feeOutput := transaction.NewTxOutput(lbtc, feeValue, feeScript)

	// Change from/to Bob
	changeScriptBob := p2pkhBob.Script
	changeValueBob, _ := elementsutil.SatoshiToElementsValue(change - fee)
	changeOutputBob := transaction.NewTxOutput(lbtc, changeValueBob[:], changeScriptBob)

	// P2WSH script
	// miniscript: or(and(pk(A),sha256(H)),pk(B))
	redeemScript, err := GetOpeningTxScript([]byte("gude"), pubkeyBob.SerializeCompressed(), []byte("gude")[:], 10)
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
	swapInValue, _ := elementsutil.SatoshiToElementsValue(satsToSpend)
	output := transaction.NewTxOutput(lbtc, swapInValue, redeemPayment.WitnessScript)

	// Create a new pset
	//inputs := []*transaction.TxInput{txinputs...}
	outputs := []*transaction.TxOutput{output, changeOutputBob, feeOutput}
	p, err := pset.New(txinputs, outputs, 2, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Add sighash type and witness utxo to the partial input.
	updater, err := pset.NewUpdater(p)
	if err != nil {
		t.Fatal(err)
	}

	bobspendingTxHash, err := fetchTx(utxosBob[0].TxId)
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
	_, err = esplora.BroadcastTransaction(txHex)
	if err != nil {
		t.Fatal(err)
	}
}
func Test_swap_TimelockCase(t *testing.T) {
	locktime := 5
	// Generate Preimage
	var preimage lightning.Preimage

	if _, err := rand.Read(preimage[:]); err != nil {
		t.Fatal(err)
	}
	pHash := preimage.Hash()

	// Generating Alices Keys and Address

	aliceStore := &wallet.DummyWalletStore{}
	aliceStore.Initialize()

	privkeyAlice, err := aliceStore.LoadPrivKey()
	if err != nil {
		t.Fatal(err)
	}
	pubkeyAlice := privkeyAlice.PubKey()
	p2pkhAlice := payment.FromPublicKey(pubkeyAlice, &network.Regtest, nil)
	adressAlice, _ := p2pkhAlice.PubKeyHash()

	// Generating Bob Keys and Address

	bobStore := &wallet.DummyWalletStore{}
	bobStore.Initialize()
	bobWallet := wallet.NewLiquiddWallet(bobStore, esplora, &network.Regtest)

	privkeyBob, err := bobStore.LoadPrivKey()
	if err != nil {
		t.Fatal(err)
	}

	pubkeyBob := privkeyBob.PubKey()
	p2pkhBob := payment.FromPublicKey(pubkeyBob, &network.Regtest, nil)
	addressBob, _ := p2pkhBob.PubKeyHash()

	nextBalanceChan := make(chan uint64)
	_, err = waitBalanceChange(bobWallet, nextBalanceChan)
	if err != nil {
		t.Fatal(err)
	}

	// Fund Bob address with LBTC.
	if _, err := faucet(addressBob, 1); err != nil {
		t.Fatal(err)
	}
	bobStartingBalance := <-nextBalanceChan

	// Retrieve bob utxos.
	satsToSpend := uint64(60000000)
	fee := uint64(500)
	utxosBob, change, err := bobWallet.GetUtxos(satsToSpend)
	if err != nil {
		t.Fatal(err)
	}
	// First Transaction
	// 1 Input
	//txInputHashBob := elementsutil.ReverseBytes(h2b(utxosBob[0].TxId))
	//txInputIndexBob := utxosBob[0].VOut
	//txInputBob := transaction.NewTxInput(txInputHashBob, txInputIndexBob)
	txinputs, err := esplora.WalletUtxosToTxInputs(utxosBob)
	if err != nil {
		t.Fatal(err)
	}
	// 3 outputs Script, Change, fee
	// Fees
	feeValue, _ := elementsutil.SatoshiToElementsValue(fee)
	feeScript := []byte{}
	feeOutput := transaction.NewTxOutput(lbtc, feeValue, feeScript)

	// Change from/to Bob
	changeScriptBob := p2pkhBob.Script
	changeValueBob, _ := elementsutil.SatoshiToElementsValue(change - fee)
	changeOutputBob := transaction.NewTxOutput(lbtc, changeValueBob, changeScriptBob)

	// P2WSH script
	// miniscript: or(and(pk(A),sha256(H)),pk(B))
	blockHeight, err := getBestBlock()
	if err != nil {
		t.Fatal(err)
	}

	spendingBlocktimeHeight := int64(blockHeight + locktime)
	redeemScript, err := GetOpeningTxScript(pubkeyAlice.SerializeCompressed(), pubkeyBob.SerializeCompressed(), pHash[:], spendingBlocktimeHeight)
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
	swapInValue, _ := elementsutil.SatoshiToElementsValue(satsToSpend)
	output := transaction.NewTxOutput(lbtc, swapInValue, redeemPayment.WitnessScript)

	// Create a new pset
	//inputs := []*transaction.TxInput{txinputs...}
	outputs := []*transaction.TxOutput{output, changeOutputBob, feeOutput}
	p, err := pset.New(txinputs, outputs, 2, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Add sighash type and witness utxo to the partial input.
	updater, err := pset.NewUpdater(p)
	if err != nil {
		t.Fatal(err)
	}

	bobspendingTxHash, err := fetchTx(utxosBob[0].TxId)
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
	if err = signTransaction(p, prvKeys, scripts, false, nil); err != nil {
		t.Fatal(err)
	}

	// Finalize the partial transaction.
	if err = pset.FinalizeAll(p); err != nil {
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
	nextBlockChan := make(chan int)
	waitNextBlock(nextBlockChan)
	tx, err := esplora.BroadcastTransaction(txHex)
	if err != nil {
		t.Fatal(err)
	}
	<-nextBlockChan
	t.Log(finalTx.WitnessHash())
	t.Log(finalTx.TxHash())
	t.Log(tx)

	// let some block pass
	//err = generate(uint(locktime))
	//if err != nil {
	//	t.Fatal(err)
	//}

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
		Locktime: uint32(spendingBlocktimeHeight),
		Inputs:   []*transaction.TxInput{spendingInput},
		Outputs:  []*transaction.TxOutput{spendingOutput, feeOutput},
	}

	var sigHash [32]byte

	sigHash = spendingTx.HashForWitnessV0(
		0,
		redeemScript[:],
		swapInValue,
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
	spendingTx.Inputs[0].Witness = witness

	spendingTxHex, err := spendingTx.ToHex()
	if err != nil {
		t.Fatal(err)
	}

	t.Log(spendingTxHex)
	t.Log(spendingTx.Locktime)
	t.Log(spendingTxHex)



	for i := 0; i < locktime - 2; i++ {
		_,err = faucet(adressAlice,1)
		if err != nil {
			t.Fatal(err)
		}
	}
	res, err := esplora.BroadcastTransaction(spendingTxHex)
	if err != nil  && !strings.Contains(err.Error(), "non-final"){
		t.Fatalf("expected locktime error, got: %v", err)

	}
	_,err = faucet(adressAlice,1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = waitBalanceChange(bobWallet, nextBalanceChan)
	if err != nil {
		t.Fatal(err)
	}
	res, err = esplora.BroadcastTransaction(spendingTxHex)
	if err != nil  {
		t.Fatalf("expected no error: %v", err)
	}
	bobBalance := <-nextBalanceChan

	t.Log(res)
	t.Logf("bob startingBalance %v:", bobStartingBalance)
	expected := bobStartingBalance - uint64(2*500)
	if !assert.Equal(t, expected, bobBalance) {
		t.Fatalf("balance incorrenct got: %v, expected %v", bobBalance, expected)
	}
}
func Test_Swap_PreimageClaim(t *testing.T) {
	// Generate Preimage
	var preimage lightning.Preimage

	if _, err := rand.Read(preimage[:]); err != nil {
		t.Fatal(err)
	}
	pHash := preimage.Hash()

	// Generating Alices Keys and Address

	aliceStore := &wallet.DummyWalletStore{

	}
	aliceStore.Initialize()
	aliceWallet := wallet.NewLiquiddWallet(aliceStore, esplora, &network.Regtest)

	privkeyAlice, err := aliceWallet.GetPrivKey()
	if err != nil {
		t.Fatal(err)
	}
	pubkeyAlice := privkeyAlice.PubKey()
	p2pkhAlice := payment.FromPublicKey(pubkeyAlice, &network.Regtest, nil)
	_, _ = p2pkhAlice.PubKeyHash()

	// Generating Bob Keys and Address

	bobStore := &wallet.DummyWalletStore{}
	bobStore.Initialize()
	bobWallet := wallet.NewLiquiddWallet(bobStore, esplora, &network.Regtest)

	privkeyBob, err := bobStore.LoadPrivKey()
	if err != nil {
		t.Fatal(err)
	}

	pubkeyBob := privkeyBob.PubKey()
	p2pkhBob := payment.FromPublicKey(pubkeyBob, &network.Regtest, nil)
	addressBob, _ := p2pkhBob.PubKeyHash()

	nextBalanceChan := make(chan uint64)
	_, err = waitBalanceChange(bobWallet, nextBalanceChan)
	if err != nil {
		t.Fatal(err)
	}

	// Fund Bob address with LBTC.
	if _, err := faucet(addressBob, 1); err != nil {
		t.Fatal(err)
	}
	<-nextBalanceChan

	// Retrieve bob utxos.
	satsToSpend := uint64(60000000)
	fee := uint64(500)
	utxosBob, change, err := bobWallet.GetUtxos(satsToSpend)
	if err != nil {
		t.Fatal(err)
	}
	// First Transaction
	// 1 Input
	//txInputHashBob := elementsutil.ReverseBytes(h2b(utxosBob[0].TxId))
	//txInputIndexBob := utxosBob[0].VOut
	//txInputBob := transaction.NewTxInput(txInputHashBob, txInputIndexBob)
	txinputs, err := esplora.WalletUtxosToTxInputs(utxosBob)
	if err != nil {
		t.Fatal(err)
	}
	// 3 outputs Script, Change, fee
	// Fees
	feeValue, _ := elementsutil.SatoshiToElementsValue(fee)
	feeScript := []byte{}
	feeOutput := transaction.NewTxOutput(lbtc, feeValue, feeScript)

	// Change from/to Bob
	changeScriptBob := p2pkhBob.Script
	changeValueBob, _ := elementsutil.SatoshiToElementsValue(change - fee)
	changeOutputBob := transaction.NewTxOutput(lbtc, changeValueBob, changeScriptBob)

	// P2WSH script
	// miniscript: or(and(pk(A),sha256(H)),pk(B))
	redeemScript, err := GetOpeningTxScript(pubkeyAlice.SerializeCompressed(), pubkeyBob.SerializeCompressed(), pHash[:], 10)
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
	swapInValue, _ := elementsutil.SatoshiToElementsValue(satsToSpend)
	output := transaction.NewTxOutput(lbtc, swapInValue, redeemPayment.WitnessScript)

	// Create a new pset
	//inputs := []*transaction.TxInput{txinputs...}
	outputs := []*transaction.TxOutput{output, changeOutputBob, feeOutput}
	p, err := pset.New(txinputs, outputs, 2, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Add sighash type and witness utxo to the partial input.
	updater, err := pset.NewUpdater(p)
	if err != nil {
		t.Fatal(err)
	}

	bobspendingTxHash, err := fetchTx(utxosBob[0].TxId)
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
	if err = signTransaction(p, prvKeys, scripts, false, nil); err != nil {
		t.Fatal(err)
	}

	// Finalize the partial transaction.
	if err = pset.FinalizeAll(p); err != nil {
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
	nextBlockChan := make(chan int)
	waitNextBlock(nextBlockChan)
	tx, err := esplora.BroadcastTransaction(txHex)
	if err != nil {
		t.Fatal(err)
	}
	<-nextBlockChan
	t.Log(tx)

	// second transaction
	firstTxHash := finalTx.WitnessHash()
	spendingInput := transaction.NewTxInput(firstTxHash[:], 0)
	spendingSatsBytes, _ := elementsutil.SatoshiToElementsValue(satsToSpend - 500)
	spendingOutput := transaction.NewTxOutput(lbtc, spendingSatsBytes, p2pkhAlice.Script)

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
		redeemScript,
		swapInValue,
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
	witness = append(witness, sigWithHashType)
	//witness = append(witness, []byte{})
	witness = append(witness, redeemScript)
	spendingTx.Inputs[0].Witness = witness

	spendingTxHex, err := spendingTx.ToHex()
	if err != nil {
		t.Fatal(err)
	}

	t.Log(spendingTxHex)
	_, err = waitBalanceChange(aliceWallet, nextBalanceChan)
	if err != nil {
		t.Fatal(err)
	}

	res, err := esplora.BroadcastTransaction(spendingTxHex)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(res)
	aliceBalance := <-nextBalanceChan
	expected := satsToSpend - uint64(500)
	if !assert.Equal(t, expected, aliceBalance) {
		t.Fatalf("balance incorrenct got: %v, expected %v", aliceBalance, expected)
	}
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

func faucet(address string, amount float64) (string,  error) {
	nextBlockChan := make(chan int)
	waitNextBlock(nextBlockChan)

	url := fmt.Sprintf("%s/faucet", "http://localhost:3001")
	payload := map[string]string{"address": address, "amount": fmt.Sprintf("%v", amount)}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return "",  err
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "",  err
	}
	if res := string(data); len(res) <= 0 || strings.Contains(res, "sendtoaddress") {
		return "", fmt.Errorf("cannot fund address with faucet: %s", res)
	}

	respBody := map[string]string{}
	if err := json.Unmarshal(data, &respBody); err != nil {
		return "", err
	}


	<-nextBlockChan
	return respBody["txId"], nil
}

func waitNextBlock(nextBlockChan chan int)  {
	timeOut := time.After(10*time.Second)
	bestBlock, err := getBestBlock()
	if err != nil {
		return
	}
	go func() {
		for {
			select {
			case <-timeOut:
				close(nextBlockChan)
				return
			default:
				nextBlock, err := getBestBlock()
				if err != nil {
					log.Printf("error getting bext block %v",err)
					return
				}
				if nextBlock > bestBlock {
					nextBlockChan <- nextBlock
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

}


func waitBalanceChange(walletService *wallet.LiquiddWallet,newBalanceChan chan uint64) (uint64, error){
	timeOut := time.After(10*time.Second)
	startBalance, err := walletService.GetBalance()
	if err != nil {
		return 0, err
	}
	log.Printf("starting balance %v", startBalance)
	go func() {
		for {
			select {
			case <-timeOut:
				close(newBalanceChan)
				return
			default:
				nextBalance, err := walletService.GetBalance()
				if err != nil {
					log.Fatalf("next balance error: %v", err)
					return
				}
				if startBalance != nextBalance {
					log.Printf("next balance %v", nextBalance)
					newBalanceChan <- nextBalance
					return
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
	return startBalance, nil
}

func getBestBlock() (int, error) {
	res, err := esplora.GetBlockHeight()
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

func Test_Esplora(t *testing.T) {
	client := NewEsploraClient("http://localhost:3001")

	bestBlock, err := client.GetBlockHeight()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("\n \n \n %v", bestBlock)
}


func fetchTx(txId string) (string, error) {
	baseUrl := "http://localhost:3001"
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

func b2h(buf []byte) string {
	return hex.EncodeToString(buf)
}
