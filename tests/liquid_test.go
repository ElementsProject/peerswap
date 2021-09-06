// +build docker

package tests

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/btcec"
	"github.com/sputn1ck/glightning/gelements"
	"github.com/sputn1ck/peerswap/lightning"
	"github.com/sputn1ck/peerswap/utils"
	wallet "github.com/sputn1ck/peerswap/wallet"
	"github.com/vulpemventures/go-elements/elementsutil"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/transaction"
	"testing"
)

const (
	LiquidPort = 18884
)

var lbtc = append(
	[]byte{0x01},
	elementsutil.ReverseBytes(h2b(network.Regtest.AssetID))...,
)

var (
	regtestOpReturnAddress = "ert1qfkht0df45q00kzyayagw6vqhfhe8ve7z7wecm0xsrkgmyulewlzqumq3ep"
)

//func Test_AssetSwap(t *testing.T) {
//	testSetup, err := NewTestSetup()
//	if err != nil {
//		t.Fatal(err)
//	}
//	//eCLi := gbitcoin.NewBitcoin("admin1","123","")
//	ecli := gelements.NewElements("admin1", "123")
//	t.Log("new ecli")
//	err = ecli.StartUp("http://localhost", LiquidPort)
//	if err != nil {
//		t.Fatalf("error testing rpc wallet %v", err)
//	}
//	walletCli, err := wallet.NewRpcWallet(ecli, "assettest")
//	if err != nil {
//		t.Fatalf("err creating wallet %v", err)
//	}
//	err = testSetup.FaucetWallet(walletCli, 1)
//	if err != nil {
//		t.Fatal(err)
//	}
//	addr, err := walletCli.GetAddress()
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	mintTx,assetId, err := mint(addr, 10000, "USDT", "USDT")
//	if err != nil {
//		t.Fatal(err)
//	}
//	var assetBytes = append(
//		[]byte{0x01},
//		elementsutil.ReverseBytes(h2b(assetId))...,
//	)
//	t.Logf("minttx %s", mintTx)
//	blockCount, err := ecli.GetBlockHeight()
//	if err != nil {
//		t.Fatalf("error testing rpc wallet %v", err)
//	}
//	// Generate Preimage
//	var preimage lightning.Preimage
//
//	if _, err := rand.Read(preimage[:]); err != nil {
//		t.Fatal(err)
//	}
//	pHash := preimage.Hash()
//
//	alicePrivkey := getRandomPrivkey()
//	bobPrivkey := getRandomPrivkey()
//
//	redeemScript, err := utils.GetOpeningTxScript(alicePrivkey.PubKey().SerializeCompressed(), bobPrivkey.PubKey().SerializeCompressed(), pHash[:], int64(blockCount+1))
//	if err != nil {
//		t.Fatalf("error creating opening tx: %v", err)
//	}
//	//dummyPayment := payment.FromPublicKey(alicePrivkey.PubKey(), &network.Regtest, nil)
//	scriptPubKey := []byte{0x00, 0x20}
//	witnessProgram := sha256.Sum256(redeemScript)
//	scriptPubKey = append(scriptPubKey, witnessProgram[:]...)
//
//	paymentaddr, err := utils.CreateOpeningAddress(redeemScript)
//	if err != nil {
//		t.Fatal(err)
//	}
//	t.Logf("addr %s", paymentaddr)
//	redeemPayment, _ := payment.FromScript(scriptPubKey, &network.Regtest, nil)
//	sats, err := elementsutil.SatoshiToElementsValue(10000)
//	if err != nil {
//		t.Log(err)
//	}
//
//
//	output := transaction.NewTxOutput(assetBytes, sats, redeemPayment.WitnessScript)
//	//feeoutput, _ := utils.GetFeeOutput(1000, &network.Regtest)
//	tx := transaction.NewTx(2)
//	tx.Outputs = append(tx.Outputs, output)
//	t.Logf("len outputs %v", len(tx.Outputs))
//	txHex, err := tx.ToHex()
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	fundedTx, err := ecli.FundRawTx(txHex)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//
//	unblinded, err := ecli.BlindRawTransaction(fundedTx.TxString)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//
//	finalized, err := ecli.SignRawTransactionWithWallet(unblinded)
//	if err != nil {
//		t.Fatal(err)
//	}
//	txid, err := testSetup.BroadcastAndGenerateN(finalized.Hex, 1)
//	if err != nil {
//		t.Fatalf("error testing rpc wallet %v", err)
//	}
//	t.Logf("tx %s", txid)
//
//	// create output for redeemtransaction
//
//	mockTx, err := getRandomTransaction(walletCli, ecli, 5000)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	newAddr, err := ecli.GetNewAddress(int(gelements.Bech32))
//	if err != nil {
//		t.Fatalf("error testing rpc wallet %v", err)
//	}
//
//	blechScript, err := utils.Blech32ToScript(newAddr, &network.Regtest)
//	if err != nil {
//		t.Fatalf("error creating blechscript %v", err)
//	}
//
//	firstTx, err := transaction.NewTxFromHex(finalized.Hex)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//
//	vout, err := utils.FindVout(firstTx.Outputs, redeemScript)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	txHash := firstTx.TxHash()
//	spendingInput := transaction.NewTxInput(txHash[:], vout)
//	spendingInput.Sequence = 0
//	spendingSatsBytes, _ := elementsutil.SatoshiToElementsValue(10000)
//
//
//	spendingOutput := transaction.NewTxOutput(assetBytes, spendingSatsBytes[:], blechScript)
//
//
//	mockTx.Inputs = append(mockTx.Inputs, spendingInput)
//	mockTx.Outputs = append(mockTx.Outputs, spendingOutput)
//
//
//
//	sigHash := mockTx.HashForWitnessV0(
//		0,
//		redeemScript[:],
//		spendingSatsBytes,
//		txscript.SigHashAll,
//	)
//
//	sig, err := alicePrivkey.Sign(sigHash[:])
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	mockTx.Inputs[len(mockTx.Inputs) - 1].Witness = getPreimageWitness(sig.Serialize(), preimage[:], redeemScript)
//
//
//	spendingTxHex, err := mockTx.ToHex()
//	if err != nil {
//		t.Fatal(err)
//	}
//
//
//
//	unblinded, err = ecli.UnblindRawtransaction(spendingTxHex)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//
//	finalized, err = ecli.SignRawTransactionWithWallet(unblinded)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	spendingTxId, err := testSetup.BroadcastAndGenerateN(finalized.Hex, 1)
//	if err != nil {
//		t.Fatalf("error testing rpc wallet %v", err)
//	}
//	t.Logf("spending txId %s", spendingTxId)
//}
//
//func getRandomTransaction(wallet2 wallet.Wallet,elements *gelements.Elements, upperFeeBound uint64) (*transaction.Transaction, error) {
//	addr, err := wallet2.GetAddress()
//	if err != nil {
//		return nil, err
//	}
//	script, err := utils.Blech32ToScript(addr, &network.Regtest)
//	if err != nil {
//		return nil, err
//	}
//	val, _ := elementsutil.SatoshiToElementsValue(upperFeeBound)
//	txOutput := transaction.NewTxOutput(lbtc, val,script)
//	tx := transaction.NewTx(2)
//	tx.Outputs = append(tx.Outputs, txOutput)
//	txHex, err := tx.ToHex()
//	if err != nil {
//		return nil, err
//	}
//	res, err := elements.FundRawTx(txHex)
//	if err != nil {
//		return nil, err
//	}
//
//	return transaction.NewTxFromHex(res.TxString)
//}
//
//func getPreimageWitness(signature, preimage, redeemScript []byte) [][]byte {
//	sigWithHashType := append(signature, byte(txscript.SigHashAll))
//	witness := make([][]byte, 0)
//	witness = append(witness, preimage[:])
//	witness = append(witness, sigWithHashType)
//	witness = append(witness, redeemScript)
//	return witness
//}
//
//func mint(address string, quantity int, name string, ticker string) (string, string, error) {
//	baseUrl, err := apiBaseUrl()
//	if err != nil {
//		return "", "", err
//	}
//
//	url := fmt.Sprintf("%s/mint", baseUrl)
//	payload := map[string]interface{}{
//		"address":  address,
//		"quantity": quantity,
//		"name":     name,
//		"ticker":   ticker,
//	}
//	body, _ := json.Marshal(payload)
//
//	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
//	if err != nil {
//		return "", "", err
//	}
//
//	data, err := ioutil.ReadAll(resp.Body)
//	if err != nil {
//		return "", "", err
//	}
//
//	if res := string(data); len(res) <= 0 || strings.Contains(res, "sendtoaddress") {
//		return "", "", fmt.Errorf("cannot fund address with minted asset: %s", res)
//	}
//
//	respBody := map[string]interface{}{}
//	if err := json.Unmarshal(data, &respBody); err != nil {
//		return "", "", err
//	}
//	return respBody["txId"].(string), respBody["asset"].(string), nil
//}
//func apiBaseUrl() (string, error) {
//	return "http://localhost:3001", nil
//}

func Test_FeeEstimation(t *testing.T) {

	util := &utils.Utility{}
	testSetup, err := NewTestSetup()
	if err != nil {
		t.Fatal(err)
	}
	//eCLi := gbitcoin.NewBitcoin("admin1","123","")
	ecli := gelements.NewElements("admin1", "123")
	t.Log("new ecli")
	err = ecli.StartUp("http://localhost", LiquidPort)
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}
	walletCli, err := wallet.NewRpcWallet(ecli, newWalletId())
	if err != nil {
		t.Fatalf("err creating wallet %v", err)
	}
	err = testSetup.FaucetWallet(walletCli, 1)
	if err != nil {
		t.Fatal(err)
	}
	blockCount, err := ecli.GetBlockHeight()
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}
	// Generate Preimage
	var preimage lightning.Preimage

	if _, err := rand.Read(preimage[:]); err != nil {
		t.Fatal(err)
	}
	pHash := preimage.Hash()

	alicePrivkey := getRandomPrivkey()
	bobPrivkey := getRandomPrivkey()

	redeemScript, err := utils.GetOpeningTxScript(alicePrivkey.PubKey().SerializeCompressed(), bobPrivkey.PubKey().SerializeCompressed(), pHash[:], int64(blockCount+1))
	if err != nil {
		t.Fatalf("error creating opening tx: %v", err)
	}
	tx, err := utils.CreateOpeningTransaction(redeemScript, lbtc, 10000)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("len outputs %v", len(tx.Outputs))
	txHex, err := tx.ToHex()
	if err != nil {
		t.Fatal(err)
	}

	fundedTx, err := ecli.FundRawTx(txHex)
	if err != nil {
		t.Fatal(err)
	}
	err = util.CheckTransactionValidity(fundedTx.TxString, 10000, redeemScript)
	if err != nil {
		t.Fatalf("error checking txValidty %v", err)
	}
	tx, err = transaction.NewTxFromHex(fundedTx.TxString)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("size: %v, fee: %f, num outputs %v, num inputs %v, ", tx.VirtualSize(), fundedTx.Fee*100000000, len(tx.Outputs), len(tx.Inputs))
	for i, o := range tx.Outputs {
		sats, err := elementsutil.ElementsToSatoshiValue(o.Value)
		if err != nil {
			t.Log(err)
		}

		t.Logf("output %v %v %v %v", i, o.Nonce, len(o.Nonce), sats)
	}

	unblinded, err := ecli.BlindRawTransaction(fundedTx.TxString)
	if err != nil {
		t.Fatal(err)
	}

	tx, err = transaction.NewTxFromHex(unblinded)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("size: %v, fee: %f, num outputs %v, num inputs %v, ", tx.VirtualSize(), fundedTx.Fee*100000000, len(tx.Outputs), len(tx.Inputs))
	for i, o := range tx.Outputs {
		sats, err := elementsutil.ElementsToSatoshiValue(o.Value)
		if err != nil {
			t.Log(err)
		}

		t.Logf("output %v %v %v %v", i, o.Nonce, len(o.Nonce), sats)
	}
	finalized, err := ecli.SignRawTransactionWithWallet(unblinded)
	if err != nil {
		t.Fatal(err)
	}
	txid, err := testSetup.BroadcastAndGenerateN(finalized.Hex, 10)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("tx %s", txid)

	rawTxHex, err := getRawTx(ecli, txid, finalized.Hex, redeemScript)
	if err != nil {
		t.Fatal(err)
	}
	// create output for redeemtransaction
	newAddr, err := ecli.GetNewAddress(int(gelements.Bech32))
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}
	blechScript, err := utils.Blech32ToScript(newAddr, &network.Regtest)
	if err != nil {
		t.Fatalf("error creating blechscript %v", err)
	}

	spendingTx, sigHash, err := util.CreateSpendingTransaction(rawTxHex, 10000, 500, 0, lbtc, redeemScript, blechScript)
	if err != nil {
		t.Fatal(err)
	}

	sig, err := alicePrivkey.Sign(sigHash[:])
	if err != nil {
		t.Fatal(err)
	}
	spendingTx.Inputs[0].Witness = util.GetPreimageWitness(sig.Serialize(), preimage[:], redeemScript)
	spendingTxHex, err := spendingTx.ToHex()
	if err != nil {
		t.Fatal(err)
	}
	spendingTxId, err := testSetup.BroadcastAndGenerateN(spendingTxHex, 1)
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}
	t.Logf("spending txId %s", spendingTxId)

}

//func Test_RpcWalletPreimage(t *testing.T) {
//	testSetup, err := NewTestSetup()
//	if err != nil {
//		t.Fatal(err)
//	}
//	//eCLi := gbitcoin.NewBitcoin("admin1","123","")
//	ecli := gelements.NewElements("admin1", "123")
//	t.Log("new ecli")
//	err = ecli.StartUp("http://localhost", LiquidPort)
//	if err != nil {
//		t.Fatalf("error testing rpc wallet %v", err)
//	}
//	walletCli, err := wallet.NewRpcWallet(ecli, newWalletId())
//	if err != nil {
//		t.Fatalf("err creating wallet %v", err)
//	}
//	err = testSetup.FaucetCli(ecli, 1)
//	if err != nil {
//		t.Fatalf("err fnding wallet %v", err)
//	}
//	blockCount, err := ecli.GetBlockHeight()
//	if err != nil {
//		t.Fatalf("error testing rpc wallet %v", err)
//	}
//	t.Logf("blockcount %v", blockCount)
//
//	// Generate Preimage
//	var preimage lightning.Preimage
//
//	if _, err := rand.Read(preimage[:]); err != nil {
//		t.Fatal(err)
//	}
//	pHash := preimage.Hash()
//
//	alicePrivkey := getRandomPrivkey()
//	bobPrivkey := getRandomPrivkey()
//
//	redeemScript, err := utils.GetOpeningTxScript(alicePrivkey.PubKey().SerializeCompressed(), bobPrivkey.PubKey().SerializeCompressed(), pHash[:], int64(blockCount+1))
//	if err != nil {
//		t.Fatalf("error creating opening tx: %v", err)
//	}
//	openingTxAddr, err := utils.CreateOpeningAddress(redeemScript)
//	if err != nil {
//		t.Fatalf("error creating opening tx: %v", err)
//	}
//	txId, err := walletCli.SendToAddress(openingTxAddr, 10000)
//	if err != nil {
//		t.Fatalf("error testing rpc wallet %v", err)
//	}
//	t.Logf("txId %s", txId)
//
//	_, err = ecli.GenerateToAddress(regtestOpReturnAddress, 1)
//	if err != nil {
//		t.Fatal(err)
//	}
//	// create output for redeemtransaction
//	newAddr, err := ecli.GetNewAddress(int(gelements.Bech32))
//	if err != nil {
//		t.Fatalf("error testing rpc wallet %v", err)
//	}
//	blechScript, err := utils.Blech32ToScript(newAddr, &network.Regtest)
//	if err != nil {
//		t.Fatalf("error creating blechscript %v", err)
//	}
//
//	rawTx, err := ecli.GetRawtransaction(txId)
//	if err != nil {
//		t.Fatalf("error testing rpc wallet %v", err)
//	}
//
//	util := &utils.Utility{}
//	spendingTx, sigHash, err := util.CreateSpendingTransaction(rawTx, 10000, 500, 0, lbtc, redeemScript, blechScript)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	sig, err := alicePrivkey.Sign(sigHash[:])
//	if err != nil {
//		t.Fatal(err)
//	}
//	spendingTx.Inputs[0].Witness = util.GetPreimageWitness(sig.Serialize(), preimage[:], redeemScript)
//	spendingTxHex, err := spendingTx.ToHex()
//	if err != nil {
//		t.Fatal(err)
//	}
//	spendingTxId, err := testSetup.BroadcastAndGenerateN(spendingTxHex, 1)
//	if err != nil {
//		t.Fatalf("error testing rpc wallet %v", err)
//	}
//	t.Logf("spending txId %s", spendingTxId)
//
//	// generate a blocks
//	_, err = ecli.GenerateToAddress(regtestOpReturnAddress, 1)
//	if err != nil {
//		t.Fatalf("error testing rpc wallet %v", err)
//	}
//
//}
//func Test_RpcWalletCltv(t *testing.T) {
//	testSetup, err := NewTestSetup()
//	if err != nil {
//		t.Fatal(err)
//	}
//	//eCLi := gbitcoin.NewBitcoin("admin1","123","")
//	ecli := gelements.NewElements("admin1", "123")
//	t.Log("new ecli")
//	err = ecli.StartUp("http://localhost", LiquidPort)
//	if err != nil {
//		t.Fatalf("error testing rpc wallet %v", err)
//	}
//	walletCli, err := wallet.NewRpcWallet(ecli, newWalletId())
//	if err != nil {
//		t.Fatalf("err creating wallet %v", err)
//	}
//	err = testSetup.FaucetCli(ecli, 1)
//	if err != nil {
//		t.Fatalf("err fnding wallet %v", err)
//	}
//	blockCount, err := ecli.GetBlockHeight()
//	if err != nil {
//		t.Fatalf("error testing rpc wallet %v", err)
//	}
//	t.Logf("blockcount %v", blockCount)
//
//	// Generate Preimage
//	var preimage lightning.Preimage
//
//	if _, err := rand.Read(preimage[:]); err != nil {
//		t.Fatal(err)
//	}
//	pHash := preimage.Hash()
//
//	alicePrivkey := getRandomPrivkey()
//	bobPrivkey := getRandomPrivkey()
//
//	redeemScript, err := utils.GetOpeningTxScript(alicePrivkey.PubKey().SerializeCompressed(), bobPrivkey.PubKey().SerializeCompressed(), pHash[:], int64(blockCount+5))
//	if err != nil {
//		t.Fatalf("error creating opening tx: %v", err)
//	}
//	openingTxAddr, err := utils.CreateOpeningAddress(redeemScript)
//	if err != nil {
//		t.Fatalf("error creating opening tx: %v", err)
//	}
//	txId, err := walletCli.SendToAddress(openingTxAddr, 10000)
//	if err != nil {
//		t.Fatalf("error testing rpc wallet %v", err)
//	}
//	t.Logf("txId %s", txId)
//
//	_, err = ecli.GenerateToAddress(regtestOpReturnAddress, 5)
//	if err != nil {
//		t.Fatal(err)
//	}
//	// create output for redeemtransaction
//	newAddr, err := ecli.GetNewAddress(int(gelements.Bech32))
//	if err != nil {
//		t.Fatalf("error testing rpc wallet %v", err)
//	}
//	blechScript, err := utils.Blech32ToScript(newAddr, &network.Regtest)
//	if err != nil {
//		t.Fatalf("error creating blechscript %v", err)
//	}
//
//	rawTx, err := getRawTx(ecli, )
//	blockCount, err = ecli.GetBlockHeight()
//	if err != nil {
//		t.Fatalf("error testing rpc wallet %v", err)
//	}
//	util := &utils.Utility{}
//	spendingTx, sigHash, err := util.CreateSpendingTransaction(rawTx, 10000, 500, blockCount, lbtc, redeemScript, blechScript)
//	if err != nil {
//		t.Fatal(err)
//	}
//	sig, err := bobPrivkey.Sign(sigHash[:])
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	spendingTx.Inputs[0].Witness = util.GetCltvWitness(sig.Serialize(), redeemScript)
//	spendingTxHex, err := spendingTx.ToHex()
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	spendingTxId, err := testSetup.BroadcastAndGenerateN(spendingTxHex, 1)
//	if err != nil {
//		t.Fatalf("error testing rpc wallet %v", err)
//	}
//	t.Logf("spending txId %s", spendingTxId)
//
//	// generate a blocks
//	_, err = ecli.GenerateToAddress(regtestOpReturnAddress, 1)
//	if err != nil {
//		t.Fatalf("error testing rpc wallet %v", err)
//	}
//
//}

func getRandomPrivkey() *btcec.PrivateKey {
	privkey, err := btcec.NewPrivateKey(btcec.S256())
	if err != nil {
		return nil
	}
	return privkey
}

type TestSetup struct {
	Elcli *gelements.Elements
}

func NewTestSetup() (*TestSetup, error) {
	walletCli := gelements.NewElements("admin1", "123")
	err := walletCli.StartUp("http://localhost", LiquidPort)
	if err != nil {
		return nil, err
	}
	return &TestSetup{Elcli: walletCli}, nil
}

func (t *TestSetup) FaucetCli(walletCli *gelements.Elements, amount float64) error {
	addr, err := walletCli.GetNewAddress(0)
	if err != nil {
		return err
	}
	return t.Faucet(addr, amount)
}
func (t *TestSetup) FaucetWallet(wallet wallet.Wallet, amount float64) error {
	addr, err := wallet.GetAddress()
	if err != nil {
		return err
	}
	return t.Faucet(addr, amount)
}

func (t *TestSetup) Faucet(address string, amount float64) error {

	_, err := t.Elcli.SendToAddress(address, fmt.Sprintf("%f", amount))
	if err != nil {
		return err
	}
	return t.GenerateBlock(1)
}

func (t *TestSetup) GenerateBlock(n uint) error {
	_, err := t.Elcli.GenerateToAddress(regtestOpReturnAddress, n)
	return err
}

func (t *TestSetup) BroadcastAndGenerateN(txHex string, nBlocks uint) (string, error) {
	txId, err := t.Elcli.SendRawTx(txHex)
	if err != nil {
		return "", err
	}
	err = t.GenerateBlock(nBlocks)
	if err != nil {
		return "", err
	}
	return txId, nil

}

func newWalletId() string {
	idBytes := make([]byte, 32)
	_, _ = rand.Read(idBytes[:])
	return hex.EncodeToString(idBytes)
}
func h2b(str string) []byte {
	buf, _ := hex.DecodeString(str)
	return buf
}

func getRawTx(ecli *gelements.Elements, txid, txHex string, redeemScript []byte) (string, error) {
	vout, err := utils.VoutFromTxHex(txHex, redeemScript)
	if err != nil {
		return "", err
	}
	txOut, err := ecli.GetTxOut(txid, vout)
	if err != nil {
		return "", err
	}
	blockheight, err := ecli.GetBlockHeight()
	if err != nil {
		return "", err
	}
	blockhash, err := ecli.GetBlockHash(uint32(blockheight) - txOut.Confirmations + 1)
	if err != nil {
		return "", err
	}
	rawTxHex, err := ecli.GetRawtransactionWithBlockHash(txid, blockhash)
	if err != nil {
		return "", err
	}
	return rawTxHex, nil
}
