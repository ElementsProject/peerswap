//go:build misc
// +build misc

package misc_tests

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"testing"

	"github.com/btcsuite/btcd/btcec"
	"github.com/elementsproject/peerswap/onchain"
	"github.com/elementsproject/peerswap/swap"
	"github.com/vulpemventures/go-elements/pset"
	"github.com/vulpemventures/go-elements/transaction"

	"github.com/elementsproject/glightning/gelements"
	"github.com/elementsproject/peerswap/lightning"
	wallet "github.com/elementsproject/peerswap/wallet"
	"github.com/vulpemventures/go-elements/elementsutil"
	"github.com/vulpemventures/go-elements/network"
)

const (
	LiquidPort = 18884
)

var lbtc = append(
	[]byte{0x01},
	elementsutil.ReverseBytes(h2b(network.Regtest.AssetID))...,
)

var (
	regtestOpReturnAddress = "el1qqtsk9kggwmrwzpjs74mf7hxpuzqetqzx3clapl79rrtvuchahtu0e2fqlt3sxw2z0enu29v7jtsm3x542llg54anv85gp9h04"
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

//func Test_FeeEstimation(t *testing.T) {
//
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
//	err = testSetup.FaucetWallet(walletCli, 1)
//	if err != nil {
//		t.Fatal(err)
//	}
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
//	redeemScript, err := utils.GetOpeningTxScript(alicePrivkey.PubKey().SerializeCompressed(), bobPrivkey.PubKey().SerializeCompressed(), pHash[:], uint32(100))
//	if err != nil {
//		t.Fatalf("error creating opening tx: %v", err)
//	}
//	tx, err := utils.CreateOpeningTransaction(redeemScript, lbtc, 10000)
//	if err != nil {
//		t.Fatal(err)
//	}
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
//	err = util.CheckTransactionValidity(fundedTx.TxString, 10000, redeemScript)
//	if err != nil {
//		t.Fatalf("error checking txValidty %v", err)
//	}
//	tx, err = transaction.NewTxFromHex(fundedTx.TxString)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	t.Logf("size: %v, fee: %f, num outputs %v, num inputs %v, ", tx.VirtualSize(), fundedTx.Fee*100000000, len(tx.Outputs), len(tx.Inputs))
//	for i, o := range tx.Outputs {
//		sats, err := elementsutil.ElementsToSatoshiValue(o.Value)
//		if err != nil {
//			t.Log(err)
//		}
//
//		t.Logf("output %v %v %v %v", i, o.Nonce, len(o.Nonce), sats)
//	}
//
//	unblinded, err := ecli.BlindRawTransaction(fundedTx.TxString)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	tx, err = transaction.NewTxFromHex(unblinded)
//	if err != nil {
//		t.Fatal(err)
//	}
//	t.Logf("size: %v, fee: %f, num outputs %v, num inputs %v, ", tx.VirtualSize(), fundedTx.Fee*100000000, len(tx.Outputs), len(tx.Inputs))
//	for i, o := range tx.Outputs {
//		sats, err := elementsutil.ElementsToSatoshiValue(o.Value)
//		if err != nil {
//			t.Log(err)
//		}
//
//		t.Logf("output %v %v %v %v", i, o.Nonce, len(o.Nonce), sats)
//	}
//	finalized, err := ecli.SignRawTransactionWithWallet(unblinded)
//	if err != nil {
//		t.Fatal(err)
//	}
//	txid, err := testSetup.BroadcastAndGenerateN(finalized.Hex, 10)
//	if err != nil {
//		t.Fatal(err)
//	}
//	t.Logf("tx %s", txid)
//
//	rawTxHex, err := getRawTx(ecli, txid, finalized.Hex, redeemScript)
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
//	spendingTx, sigHash, err := util.CreateSpendingTransaction(rawTxHex, 10000, 500, 0, lbtc, redeemScript, blechScript)
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
//}

func Test_RpcWalletPreimage(t *testing.T) {
	testSetup, err := NewTestSetup()
	if err != nil {
		t.Fatal(err)
	}

	err = testSetup.FaucetCli(testSetup.walletCli, 1)
	if err != nil {
		t.Fatalf("err funding wallet %v", err)
	}

	// Generate Preimage
	var preimage lightning.Preimage

	if _, err := rand.Read(preimage[:]); err != nil {
		t.Fatal(err)
	}
	pHash := preimage.Hash()

	// generate keys

	alicePrivkey := getRandomPrivkey()
	bobPrivkey := getRandomPrivkey()
	blindingKey := getRandomPrivkey()

	//nonceKey := getRandomPrivkey()
	//nonceHash,err := confidential.NonceHash(nonceKey.PubKey().SerializeCompressed(), nonceKey.Serialize())
	//if err != nil {
	//	t.Fatalf("error creating nonce hash: %v", err)
	//}

	// create opening params

	openingParams := &swap.OpeningParams{
		TakerPubkey:      hex.EncodeToString(alicePrivkey.PubKey().SerializeCompressed()),
		MakerPubkey:      hex.EncodeToString(bobPrivkey.PubKey().SerializeCompressed()),
		ClaimPaymentHash: hex.EncodeToString(pHash[:]),
		Amount:           100000,
		BlindingKey:      blindingKey,
	}

	claimParams := &swap.ClaimParams{
		Preimage: hex.EncodeToString(preimage[:]),
		Signer:   alicePrivkey,
	}

	testSetup.onchain.AddBlindingRandomFactors(claimParams)

	//openingTxAddr, err := testSetup.onchain.CreateBlindedOpeningAddress(redeemScript, blindingKey.PubKey())

	// create opening transaction
	unprepTxHex, _, _, err := testSetup.onchain.CreateOpeningTransaction(openingParams)
	if err != nil {
		t.Fatalf("error creating opening tx: %v", err)
	}

	txId, txHex, err := testSetup.onchain.BroadcastOpeningTx(unprepTxHex)
	if err != nil {
		t.Fatalf("error creating opening tx: %v", err)
	}

	log.Printf("opening txid %s", txId)

	_, err = testSetup.Elcli.GenerateToAddress(regtestOpReturnAddress, 1)
	if err != nil {
		t.Fatal(err)
	}
	claimParams.OpeningTxHex = txHex

	spendingtxId, _, err := testSetup.onchain.CreatePreimageSpendingTransaction(openingParams, claimParams)
	if err != nil {
		t.Fatal(err)
	}

	log.Printf("spending txid %s", spendingtxId)
}

func Test_RpcWalletCsv(t *testing.T) {
	testSetup, err := NewTestSetup()
	if err != nil {
		t.Fatal(err)
	}

	err = testSetup.FaucetCli(testSetup.walletCli, 1)
	if err != nil {
		t.Fatalf("err funding wallet %v", err)
	}

	// Generate Preimage
	var preimage lightning.Preimage

	if _, err := rand.Read(preimage[:]); err != nil {
		t.Fatal(err)
	}
	pHash := preimage.Hash()

	// generate keys

	alicePrivkey := getRandomPrivkey()
	bobPrivkey := getRandomPrivkey()
	blindingKey := getRandomPrivkey()

	//nonceKey := getRandomPrivkey()
	//nonceHash,err := confidential.NonceHash(nonceKey.PubKey().SerializeCompressed(), nonceKey.Serialize())
	//if err != nil {
	//	t.Fatalf("error creating nonce hash: %v", err)
	//}

	// create opening params

	openingParams := &swap.OpeningParams{
		TakerPubkey:      hex.EncodeToString(alicePrivkey.PubKey().SerializeCompressed()),
		MakerPubkey:      hex.EncodeToString(bobPrivkey.PubKey().SerializeCompressed()),
		ClaimPaymentHash: hex.EncodeToString(pHash[:]),
		Amount:           100000,
		BlindingKey:      blindingKey,
	}

	claimParams := &swap.ClaimParams{
		Preimage: hex.EncodeToString(preimage[:]),
		Signer:   bobPrivkey,
	}

	testSetup.onchain.AddBlindingRandomFactors(claimParams)

	//openingTxAddr, err := testSetup.onchain.CreateBlindedOpeningAddress(redeemScript, blindingKey.PubKey())

	// create opening transaction
	unprepTxHex, _, _, err := testSetup.onchain.CreateOpeningTransaction(openingParams)
	if err != nil {
		t.Fatalf("error creating opening tx: %v", err)
	}

	txId, txHex, err := testSetup.onchain.BroadcastOpeningTx(unprepTxHex)
	if err != nil {
		t.Fatalf("error creating opening tx: %v", err)
	}

	log.Printf("opening txid %s", txId)

	_, err = testSetup.Elcli.GenerateToAddress(regtestOpReturnAddress, 60)
	if err != nil {
		t.Fatal(err)
	}
	claimParams.OpeningTxHex = txHex

	spendingtxId, _, err := testSetup.onchain.CreateCsvSpendingTransaction(openingParams, claimParams)
	if err != nil {
		t.Fatal(err)
	}

	log.Printf("spending txid %s", spendingtxId)
}

func Test_RpcWalletCoop(t *testing.T) {
	testSetup, err := NewTestSetup()
	if err != nil {
		t.Fatal(err)
	}

	err = testSetup.FaucetCli(testSetup.walletCli, 1)
	if err != nil {
		t.Fatalf("err funding wallet %v", err)
	}

	// Generate Preimage
	var preimage lightning.Preimage

	if _, err := rand.Read(preimage[:]); err != nil {
		t.Fatal(err)
	}
	pHash := preimage.Hash()

	// generate keys

	alicePrivkey := getRandomPrivkey()
	bobPrivkey := getRandomPrivkey()
	blindingKey := getRandomPrivkey()

	//nonceKey := getRandomPrivkey()
	//nonceHash,err := confidential.NonceHash(nonceKey.PubKey().SerializeCompressed(), nonceKey.Serialize())
	//if err != nil {
	//	t.Fatalf("error creating nonce hash: %v", err)
	//}

	// create opening params

	openingParams := &swap.OpeningParams{
		TakerPubkey:      hex.EncodeToString(alicePrivkey.PubKey().SerializeCompressed()),
		MakerPubkey:      hex.EncodeToString(bobPrivkey.PubKey().SerializeCompressed()),
		ClaimPaymentHash: hex.EncodeToString(pHash[:]),
		Amount:           100000,
		BlindingKey:      blindingKey,
	}

	claimParams := &swap.ClaimParams{
		Preimage: hex.EncodeToString(preimage[:]),
		Signer:   alicePrivkey,
	}

	//openingTxAddr, err := testSetup.onchain.CreateBlindedOpeningAddress(redeemScript, blindingKey.PubKey())

	// create opening transaction
	unprepTxHex, _, _, err := testSetup.onchain.CreateOpeningTransaction(openingParams)
	if err != nil {
		t.Fatalf("error creating opening tx: %v", err)
	}

	txId, txHex, err := testSetup.onchain.BroadcastOpeningTx(unprepTxHex)
	if err != nil {
		t.Fatalf("error creating opening tx: %v", err)
	}

	log.Printf("opening txid %s", txId)

	_, err = testSetup.Elcli.GenerateToAddress(regtestOpReturnAddress, 1)
	if err != nil {
		t.Fatal(err)
	}
	claimParams.OpeningTxHex = txHex

	addr, _ := testSetup.wallet.GetAddress()
	fee := 500

	signature, err := testSetup.onchain.TakerCreateCoopSigHash(openingParams, claimParams, addr, 500)
	if err != nil {
		t.Fatal(err)
	}
	claimParams.Signer = bobPrivkey
	log.Printf("abf %x ", claimParams.OutputAssetBlindingFactor)
	spendingtxId, _, err := testSetup.onchain.CreateCooperativeSpendingTransaction(openingParams, claimParams, addr, signature, uint64(fee))
	if err != nil {
		t.Fatal(err)
	}

	log.Printf("spending txid %s", spendingtxId)
}

/*
	// create tx struct from opening tx hex
	openingtx, err := transaction.NewTxFromHex(txHex)
	if err != nil {
		t.Fatalf("error building openingTx %v", err)
	}

	claimParams.OpeningTxHex = txHex

	log.Printf("opening txId %s", txId)

	redeemAddr, err := testSetup.wallet.GetAddress()
	if err != nil {
		t.Fatal(err)
	}

	redeemScript, err := onchain.ParamsToTxScript(openingParams, onchain.LiquidCsv)
	if err != nil {
		t.Fatal(err)
	}

	vout, err := testSetup.onchain.FindVout(openingtx.Outputs, redeemScript)
	if err != nil {
		t.Fatal(err)
	}

	// unblind output
	ubRes, err := confidential.UnblindOutputWithKey(openingtx.Outputs[vout], openingParams.BlindingKey.Serialize())
	if err != nil {
		t.Fatal(err)
	}

	// todo muss ins protocol
	if !bytes.Equal(ubRes.Asset, lbtc) {

	}

	//check output amounts
	if ubRes.Value != openingParams.Amount {
		t.Fatal("values wrong")
	}

	fee := uint64(500)
	outputValue := ubRes.Value - fee

	// generate output blinding factors
	outputAbf, _ := rngesus.Next()
	if hex.EncodeToString(outputAbf[:]) != "510c805559239d589cae7cd36c8453f3d02925270321db76608588329d382516" {
		t.Fatal("rngesus not working")
	}

	finalVbfArgs := confidential.FinalValueBlindingFactorArgs{
		InValues:      []uint64{ubRes.Value},
		OutValues:     []uint64{outputValue},
		InGenerators:  [][]byte{ubRes.AssetBlindingFactor},
		OutGenerators: [][]byte{outputAbf[:]},
		InFactors:     [][]byte{ubRes.ValueBlindingFactor},
		OutFactors:    [][]byte{},
	}

	outputVbf, err := confidential.FinalValueBlindingFactor(finalVbfArgs)
	if err != nil {
		t.Fatal(err)
	}

	// get asset commitment
	assetcommitment, err := confidential.AssetCommitment(ubRes.Asset, outputAbf[:])
	if err != nil {
		t.Fatal(err)
	}

	valueCommitment, err := confidential.ValueCommitment(outputValue, assetcommitment[:], outputVbf[:])
	if err != nil {
		t.Fatal(err)
	}

	seed, _ := rngesus.Next()
	if hex.EncodeToString(seed[:]) != "00f762e0efaf015be580dbfbae1d894b3c6477738b47be41c3baba7bb9249ce9" {
		t.Fatal("rngesus not working")
	}

	surjectionProofArgs := confidential.SurjectionProofArgs{
		OutputAsset:               ubRes.Asset,
		OutputAssetBlindingFactor: outputAbf[:],
		InputAssets:               [][]byte{ubRes.Asset},
		InputAssetBlindingFactors: [][]byte{ubRes.AssetBlindingFactor},
		Seed:                      seed[:],
	}

	surjectionProof, ok := confidential.SurjectionProof(surjectionProofArgs)
	if !ok {
		t.Fatal(pset.ErrGenerateSurjectionProof)
	}

	log.Printf("surjection proof size: %v", len(surjectionProof))

	confOutputScript, err := address.ToOutputScript(redeemAddr)
	if err != nil {
		t.Fatal(err)
	}

	confAddr, err := address.FromConfidential(redeemAddr)
	if err != nil {
		t.Fatal(err)
	}

	// create new transaction
	spendingTx := transaction.NewTx(2)

	// add input
	txHash := openingtx.TxHash()
	swapInput := transaction.NewTxInput(txHash[:], vout)
	spendingTx.Inputs = []*transaction.TxInput{swapInput}

	// create nonce (shared secret)
	// ephemeralPrivKey, err := btcec.NewPrivateKey(btcec.S256())
	// if err != nil {
	// 	t.Fatal(err)
	// }
	ephemeralPrivKey := blindingKey
	outputNonce := ephemeralPrivKey.PubKey()

	nonce, err := confidential.NonceHash(confAddr.BlindingKey, ephemeralPrivKey.Serialize())
	if err != nil {
		t.Fatal(err)
	}

	// build rangeproof
	rangeProofArgs := confidential.RangeProofArgs{
		Value:               outputValue,
		Nonce:               nonce,
		Asset:               ubRes.Asset,
		AssetBlindingFactor: outputAbf[:],
		ValueBlindFactor:    outputVbf,
		ValueCommit:         valueCommitment[:],
		ScriptPubkey:        confOutputScript,
		MinValue:            1,
		Exp:                 0,
		MinBits:             52,
	}

	rangeProof, err := confidential.RangeProof(rangeProofArgs)
	if err != nil {
		t.Fatal(err)
	}
	log.Printf("range proof size: %v", len(rangeProof))

	//create output
	receiverOutput := transaction.NewTxOutput(lbtc, valueCommitment, confOutputScript)
	receiverOutput.Asset = assetcommitment
	receiverOutput.Value = valueCommitment
	receiverOutput.Nonce = outputNonce.SerializeCompressed()
	receiverOutput.RangeProof = rangeProof
	receiverOutput.SurjectionProof = surjectionProof

	spendingTx.Outputs = append(spendingTx.Outputs, receiverOutput)

	// add feeoutput
	feeValue, _ := elementsutil.SatoshiToElementsValue(fee)
	feeScript := []byte{}
	feeOutput := transaction.NewTxOutput(lbtc, feeValue, feeScript)
	spendingTx.Outputs = append(spendingTx.Outputs, feeOutput)

	// create sighash
	sigHash := spendingTx.HashForWitnessV0(
		0, redeemScript[:], openingtx.Outputs[vout].Value, txscript.SigHashAll)

	sigBytes, err := claimParams.Signer.Sign(sigHash[:])
	if err != nil {
		t.Fatal(err)
	}

	spendingTx.Inputs[0].Witness = onchain.GetPreimageWitness(sigBytes.Serialize(), preimage[:], redeemScript)
	spendingTxHex, err := spendingTx.ToHex()
	if err != nil {
		t.Fatal(err)
	}

	log.Printf("spending tx hex %s", spendingTxHex)
	spendingTxId, err := testSetup.BroadcastAndGenerateN(spendingTxHex, 1)
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}
	t.Logf("spending txId %s", spendingTxId)

	t.Logf("spending tx size %v", spendingTx.SerializeSize(true, true))
}

type Rngesus struct {
	seed    []byte
	counter uint32
}

func NewRngesus(seed []byte) *Rngesus {
	return &Rngesus{
		seed:    seed,
		counter: 0,
	}
}

func (r *Rngesus) Next() ([32]byte, error) {
	intBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(intBytes, r.counter)
	hash := sha256.Sum256(append(r.seed, intBytes...))
	if r.counter == ^uint32(0) {
		return [32]byte{}, errors.New("max uint reached")
	}
	r.counter++
	return hash, nil
}

func Test_Rngesus(t *testing.T) {
	rngesus := NewRngesus([]byte("gude"))
	rngesus.counter = ^uint32(0) - 1
	_, err := rngesus.Next()
	if err != nil {
		t.Fatalf("no error expected %v", err)
	}
	_, err = rngesus.Next()
	if err == nil {
		t.Fatalf("error expected %v", err)
	}
}

/*
	inputs := []*transaction.TxInput{swapInput}
	outputs := []*transaction.TxOutput{receiverOutput}
	p, err := pset.New(inputs, outputs, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Add sighash type and witness utxo to the partial input.
	updater, err := pset.NewUpdater(p)
	if err != nil {
		t.Fatal(err)
	}
	witnessUtxo := &transaction.TxOutput{
		Asset:           assetcommitment,
		Value:           valueCommitment,
		Script:          confOutputScript,
		Nonce:           openingtx.Outputs[vout].Nonce,
		RangeProof:      openingtx.Outputs[vout].RangeProof,
		SurjectionProof: openingtx.Outputs[vout].SurjectionProof,
	}

	if err := updater.AddInWitnessUtxo(witnessUtxo, 0); err != nil {
		t.Fatal(err)
	}
	addFeesToTransaction(p, 500)

	unfinishedSpendingTxHex, err := p.ToHex()
	if err != nil {
		t.Fatal(err)
	}

	spendingTx, err = transaction.NewTxFromHex(unfinishedSpendingTxHex)
	if err != nil {
		t.Fatal(err)
	}
	sigHash := spendingTx.HashForWitnessV0(
		0, redeemScript[:], openingtx.Outputs[vout].Value, txscript.SigHashAll)

	sigBytes, err := claimParams.Signer.Sign(sigHash[:])
	if err != nil {
		t.Fatal(err)
	}

	spendingTx.Inputs[0].Witness = onchain.GetPreimageWitness(sigBytes.Serialize(), preimage[:], redeemScript)
	spendingTxHex, err := spendingTx.ToHex()
	if err != nil {
		t.Fatal(err)
	}

	log.Printf("spending tx hex %s", spendingTxHex)
	spendingTxId, err := testSetup.BroadcastAndGenerateN(spendingTxHex, 1)
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}
	t.Logf("spending txId %s", spendingTxId)
	//err = updater.AddInSighashType(txscript.SigHashAll, 0)
	//if err != nil {
	//	t.Fatal(err)
	//}
	//
	//sigHash := p.UnsignedTx.HashForWitnessV0(0, openingtx.Outputs[0].Script, openingtx.Outputs[0].Value, txscript.SigHashAll)
	//sig, err := claimParams.Signer.Sign(sigHash[:])
	//if err != nil {
	//	t.Fatal(err)
	//}
	//sigWithHashType := append(sig.Serialize(), byte(txscript.SigHashAll))
	//outcome, err := updater.Sign(0,sigWithHashType, alicePrivkey.PubKey().SerializeCompressed(),nil,nil)
	//if err != nil {
	//	t.Fatal(err)
	//}
	//log.Printf("outcome: %v", outcome)

	// create output
	//outputValue, _ := elementsutil.SatoshiToElementsValue(openingParams.Amount - 500)
	//redeemOutput := transaction.NewTxOutput(lbtc, outputValue, confOutputScript)
	//
	//spendingTx.Outputs = append(spendingTx.Outputs, redeemOutput)
	//
	//feeValue, _ := elementsutil.SatoshiToElementsValue(500)
	//feeScript := []byte{}
	//feeOutput := transaction.NewTxOutput(lbtc, feeValue, feeScript)
	//spendingTx.Outputs = append(spendingTx.Outputs, feeOutput)

	//spendingTxHex, err := spendingTx.ToHex()
	//if err != nil {
	//	t.Fatal(err)
	//}
	//p, err := pset.NewPsetFromHex(spendingTxHex)
	//if err != nil {
	//	t.Fatal(err)
	//}
	//updater, err := pset.NewUpdater(p)
	//if err != nil {
	//	t.Fatal(err)
	//}
	//witnessUtxo := &transaction.TxOutput{
	//	Asset:           assetcommitment,
	//	Value:           valueCommitment,
	//	Script:          redeemScript,
	//	Nonce:           openingtx.Outputs[0].Nonce,
	//	RangeProof:      openingtx.Outputs[0].RangeProof,
	//	SurjectionProof: openingtx.Outputs[0].SurjectionProof,
	//}
	//err = updater.AddInWitnessUtxo(witnessUtxo, 0)
	//if err != nil {
	//	t.Fatal(err)
	//}

	//txHex, err = p.ToHex()
	//if err != nil {
	//	t.Fatal(err)
	//}
	//spendingTx, err = transaction.NewTxFromHex(txHex)
	//if err != nil {
	//	t.Fatal(err)
	//}

	//inputValue, _ := elementsutil.SatoshiToElementsValue(ubRes.Value)
	//sigHash := spendingTx.HashForWitnessV0(
	//	0, redeemScript[:], inputValue, txscript.SigHashAll)
	//sigBytes, err := claimParams.Signer.Sign(sigHash[:])
	//if err != nil {
	//	t.Fatal(err)
	//}
	//
	//
	//spendingTx.Inputs[0].Witness = onchain.GetPreimageWitness(sigBytes.Serialize(), preimage[:], redeemScript)
	//spendingTxHex, err := spendingTx.ToHex()
	//if err != nil {
	//	t.Fatal(err)
	//}
	//
	//log.Printf("spending tx hex %s", spendingTxHex)
	//spendingTxId, err := testSetup.BroadcastAndGenerateN(spendingTxHex, 1)
	//if err != nil {
	//	t.Fatalf("error testing rpc wallet %v", err)
	//}
	//t.Logf("spending txId %s", spendingTxId)

}
*/

// pset test
//// create receive output
//receiverValue, _ := elementsutil.SatoshiToElementsValue(openingParams.Amount - 500)
//receiverScript := confOutputScript
//receiverOutput := transaction.NewTxOutput(lbtc, receiverValue, receiverScript)
//
//
//inputs := []*transaction.TxInput{swapInput}
//outputs := []*transaction.TxOutput{receiverOutput}
//p, err := pset.New(inputs, outputs, 2,0)
//if err != nil {
//	t.Fatal(err)
//}
//// Add sighash type and witness utxo to the partial input.
//updater, err := pset.NewUpdater(p)
//if err != nil {
//	t.Fatal(err)
//}
//witnessUtxo  := &transaction.TxOutput{
//	Asset:           assetcommitment,
//	Value:           valueCommitment,
//	Script:          confOutputScript,
//	Nonce:           openingtx.Outputs[vout].Nonce,
//	RangeProof:      openingtx.Outputs[vout].RangeProof,
//	SurjectionProof: openingtx.Outputs[vout].SurjectionProof,
//}
//
//if err := updater.AddInWitnessUtxo(witnessUtxo, 0); err != nil {
//	t.Fatal(err)
//}
//inBlindingPrvKeys := [][]byte{blindingKey.Serialize()}
//outBlindingPrvKeys := [][]byte{confAddr.BlindingKey}
//if err := blindTransaction(
//	p,
//	inBlindingPrvKeys,
//	outBlindingPrvKeys,
//	nil,
//); err != nil {
//	t.Fatal(err)
//}
//
//addFeesToTransaction(p, 500)
//err = updater.AddInSighashType(txscript.SigHashAll, 0)
//if err != nil {
//	t.Fatal(err)
//}
//
//txHex, err = p.ToHex()
//if err != nil {
//	t.Fatal(err)
//}
//txTest, err := transaction.NewTxFromHex(txHex)
//if err != nil {
//	t.Fatal(err)
//}
//sigHash := txTest.HashForWitnessV0(
//	0, redeemScript[:], openingtx.Outputs[0].Value, txscript.SigHashAll)
//sigBytes, err := claimParams.Signer.Sign(sigHash[:])
//if err != nil {
//	t.Fatal(err)
//}
//
//txTest.Inputs[0].Witness = onchain.GetPreimageWitness(sigBytes.Serialize(), preimage[:], redeemScript)
//
//txHex, err = txTest.ToHex()
//if err != nil {
//	t.Fatal(err)
//}
//spendingTxId, err := testSetup.BroadcastAndGenerateN(txHex, 1)
//if err != nil {
//	t.Fatalf("error testing rpc wallet %v", err)
//}
//
//t.Logf("spending txId %s", spendingTxId)
//return
// create output for redeemtransaction
//func Test_RpcWalletCsv(t *testing.T) {
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
//	spendingTx.Inputs[0].Witness = util.GetCsvWitness(sig.Serialize(), redeemScript)
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

func addFeesToTransaction(p *pset.Pset, feeAmount uint64) {
	updater, _ := pset.NewUpdater(p)
	feeScript := []byte{}
	feeValue, _ := elementsutil.SatoshiToElementsValue(feeAmount)
	feeOutput := transaction.NewTxOutput(lbtc, feeValue, feeScript)
	updater.AddOutput(feeOutput)
}

func blindTransaction(
	p *pset.Pset,
	inBlindKeys [][]byte,
	outBlindKeys [][]byte,
	issuanceBlindKeys []pset.IssuanceBlindingPrivateKeys,
) error {
	outputsPrivKeyByIndex := make(map[int][]byte, 0)
	for index, output := range p.UnsignedTx.Outputs {
		if len(output.Script) > 0 {
			outputsPrivKeyByIndex[index] = outBlindKeys[index]
		}
	}

	return blindTransactionByIndex(p, inBlindKeys, outputsPrivKeyByIndex, issuanceBlindKeys)
}
func blindTransactionByIndex(
	p *pset.Pset,
	inBlindKeys [][]byte,
	outBlindKeysMap map[int][]byte,
	issuanceBlindKeys []pset.IssuanceBlindingPrivateKeys,
) error {
	outBlindPubKeysMap := make(map[int][]byte)
	for index, k := range outBlindKeysMap {
		_, pubkey := btcec.PrivKeyFromBytes(btcec.S256(), k)
		outBlindPubKeysMap[index] = pubkey.SerializeCompressed()
	}

	psetBase64, err := p.ToBase64()
	if err != nil {
		return err
	}

	for {
		blindDataLike := make([]pset.BlindingDataLike, len(inBlindKeys), len(inBlindKeys))
		for i, inBlinKey := range inBlindKeys {
			blindDataLike[i] = pset.PrivateBlindingKey(inBlinKey)
		}

		ptx, _ := pset.NewPsetFromBase64(psetBase64)
		blinder, err := pset.NewBlinder(
			ptx,
			blindDataLike,
			outBlindPubKeysMap,
			issuanceBlindKeys,
			nil,
		)
		if err != nil {
			return err
		}

		for {
			if err := blinder.Blind(); err != nil {
				if err != pset.ErrGenerateSurjectionProof {
					return err
				}
				continue
			}
			break
		}

		verify, err := pset.VerifyBlinding(ptx, blindDataLike, outBlindKeysMap, issuanceBlindKeys)
		if err != nil {
			return err
		}

		if verify {
			*p = *ptx
			break
		}
	}

	return nil
}

func getRandomPrivkey() *btcec.PrivateKey {
	privkey, err := btcec.NewPrivateKey(btcec.S256())
	if err != nil {
		return nil
	}
	return privkey
}

type TestSetup struct {
	Elcli     *gelements.Elements
	walletCli *gelements.Elements
	onchain   *onchain.LiquidOnChain
	wallet    wallet.Wallet
}

func NewTestSetup() (*TestSetup, error) {
	walletCli := gelements.NewElements("admin1", "123")
	err := walletCli.StartUp("http://localhost", LiquidPort)
	if err != nil {
		return nil, err
	}
	normalCli := gelements.NewElements("admin1", "123")
	err = normalCli.StartUp("http://localhost", LiquidPort)
	if err != nil {
		return nil, err
	}
	liquidWallet, err := wallet.NewRpcWallet(walletCli, "swaptest")
	if err != nil {
		return nil, err
	}

	onchain := onchain.NewLiquidOnChain(walletCli, liquidWallet, &network.Regtest)

	return &TestSetup{Elcli: normalCli, walletCli: walletCli, onchain: onchain, wallet: liquidWallet}, nil
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

func rand32ByteArray() []byte {
	randArr := [32]byte{}
	_, _ = rand.Read(randArr[:])
	return randArr[:]
}

//
//import (
//	"crypto/rand"
//	"encoding/hex"
//	"fmt"
//	"github.com/btcsuite/btcd/btcec"
//	"github.com/elementsproject/peerswap/onchain"
//	"github.com/elementsproject/peerswap/swap"
//	"log"
//	"testing"
//
//	"github.com/elementsproject/glightning/gelements"
//	"github.com/elementsproject/peerswap/lightning"
//	wallet "github.com/elementsproject/peerswap/wallet"
//	"github.com/vulpemventures/go-elements/elementsutil"
//	"github.com/vulpemventures/go-elements/network"
//)
//
//const (
//	LiquidPort = 18884
//)
//
//var lbtc = append(
//	[]byte{0x01},
//	elementsutil.ReverseBytes(h2b(network.Regtest.AssetID))...,
//)
//
//var (
//	regtestOpReturnAddress = "ert1qfkht0df45q00kzyayagw6vqhfhe8ve7z7wecm0xsrkgmyulewlzqumq3ep"
//)
//
////func Test_AssetSwap(t *testing.T) {
////	testSetup, err := NewTestSetup()
////	if err != nil {
////		t.Fatal(err)
////	}
////	//eCLi := gbitcoin.NewBitcoin("admin1","123","")
////	ecli := gelements.NewElements("admin1", "123")
////	t.Log("new ecli")
////	err = ecli.StartUp("http://localhost", LiquidPort)
////	if err != nil {
////		t.Fatalf("error testing rpc wallet %v", err)
////	}
////	walletCli, err := wallet.NewRpcWallet(ecli, "assettest")
////	if err != nil {
////		t.Fatalf("err creating wallet %v", err)
////	}
////	err = testSetup.FaucetWallet(walletCli, 1)
////	if err != nil {
////		t.Fatal(err)
////	}
////	addr, err := walletCli.GetAddress()
////	if err != nil {
////		t.Fatal(err)
////	}
////
////	mintTx,assetId, err := mint(addr, 10000, "USDT", "USDT")
////	if err != nil {
////		t.Fatal(err)
////	}
////	var assetBytes = append(
////		[]byte{0x01},
////		elementsutil.ReverseBytes(h2b(assetId))...,
////	)
////	t.Logf("minttx %s", mintTx)
////	blockCount, err := ecli.GetBlockHeight()
////	if err != nil {
////		t.Fatalf("error testing rpc wallet %v", err)
////	}
////	// Generate Preimage
////	var preimage lightning.Preimage
////
////	if _, err := rand.Read(preimage[:]); err != nil {
////		t.Fatal(err)
////	}
////	pHash := preimage.Hash()
////
////	alicePrivkey := getRandomPrivkey()
////	bobPrivkey := getRandomPrivkey()
////
////	redeemScript, err := utils.GetOpeningTxScript(alicePrivkey.PubKey().SerializeCompressed(), bobPrivkey.PubKey().SerializeCompressed(), pHash[:], int64(blockCount+1))
////	if err != nil {
////		t.Fatalf("error creating opening tx: %v", err)
////	}
////	//dummyPayment := payment.FromPublicKey(alicePrivkey.PubKey(), &network.Regtest, nil)
////	scriptPubKey := []byte{0x00, 0x20}
////	witnessProgram := sha256.Sum256(redeemScript)
////	scriptPubKey = append(scriptPubKey, witnessProgram[:]...)
////
////	paymentaddr, err := utils.CreateOpeningAddress(redeemScript)
////	if err != nil {
////		t.Fatal(err)
////	}
////	t.Logf("addr %s", paymentaddr)
////	redeemPayment, _ := payment.FromScript(scriptPubKey, &network.Regtest, nil)
////	sats, err := elementsutil.SatoshiToElementsValue(10000)
////	if err != nil {
////		t.Log(err)
////	}
////
////
////	output := transaction.NewTxOutput(assetBytes, sats, redeemPayment.WitnessScript)
////	//feeoutput, _ := utils.GetFeeOutput(1000, &network.Regtest)
////	tx := transaction.NewTx(2)
////	tx.Outputs = append(tx.Outputs, output)
////	t.Logf("len outputs %v", len(tx.Outputs))
////	txHex, err := tx.ToHex()
////	if err != nil {
////		t.Fatal(err)
////	}
////
////	fundedTx, err := ecli.FundRawTx(txHex)
////	if err != nil {
////		t.Fatal(err)
////	}
////
////
////	unblinded, err := ecli.BlindRawTransaction(fundedTx.TxString)
////	if err != nil {
////		t.Fatal(err)
////	}
////
////
////	finalized, err := ecli.SignRawTransactionWithWallet(unblinded)
////	if err != nil {
////		t.Fatal(err)
////	}
////	txid, err := testSetup.BroadcastAndGenerateN(finalized.Hex, 1)
////	if err != nil {
////		t.Fatalf("error testing rpc wallet %v", err)
////	}
////	t.Logf("tx %s", txid)
////
////	// create output for redeemtransaction
////
////	mockTx, err := getRandomTransaction(walletCli, ecli, 5000)
////	if err != nil {
////		t.Fatal(err)
////	}
////
////	newAddr, err := ecli.GetNewAddress(int(gelements.Bech32))
////	if err != nil {
////		t.Fatalf("error testing rpc wallet %v", err)
////	}
////
////	blechScript, err := utils.Blech32ToScript(newAddr, &network.Regtest)
////	if err != nil {
////		t.Fatalf("error creating blechscript %v", err)
////	}
////
////	firstTx, err := transaction.NewTxFromHex(finalized.Hex)
////	if err != nil {
////		t.Fatal(err)
////	}
////
////
////	vout, err := utils.FindVout(firstTx.Outputs, redeemScript)
////	if err != nil {
////		t.Fatal(err)
////	}
////
////	txHash := firstTx.TxHash()
////	spendingInput := transaction.NewTxInput(txHash[:], vout)
////	spendingInput.Sequence = 0
////	spendingSatsBytes, _ := elementsutil.SatoshiToElementsValue(10000)
////
////
////	spendingOutput := transaction.NewTxOutput(assetBytes, spendingSatsBytes[:], blechScript)
////
////
////	mockTx.Inputs = append(mockTx.Inputs, spendingInput)
////	mockTx.Outputs = append(mockTx.Outputs, spendingOutput)
////
////
////
////	sigHash := mockTx.HashForWitnessV0(
////		0,
////		redeemScript[:],
////		spendingSatsBytes,
////		txscript.SigHashAll,
////	)
////
////	sig, err := alicePrivkey.Sign(sigHash[:])
////	if err != nil {
////		t.Fatal(err)
////	}
////
////	mockTx.Inputs[len(mockTx.Inputs) - 1].Witness = getPreimageWitness(sig.Serialize(), preimage[:], redeemScript)
////
////
////	spendingTxHex, err := mockTx.ToHex()
////	if err != nil {
////		t.Fatal(err)
////	}
////
////
////
////	unblinded, err = ecli.UnblindRawtransaction(spendingTxHex)
////	if err != nil {
////		t.Fatal(err)
////	}
////
////
////	finalized, err = ecli.SignRawTransactionWithWallet(unblinded)
////	if err != nil {
////		t.Fatal(err)
////	}
////
////	spendingTxId, err := testSetup.BroadcastAndGenerateN(finalized.Hex, 1)
////	if err != nil {
////		t.Fatalf("error testing rpc wallet %v", err)
////	}
////	t.Logf("spending txId %s", spendingTxId)
////}
////
////func getRandomTransaction(wallet2 wallet.Wallet,elements *gelements.Elements, upperFeeBound uint64) (*transaction.Transaction, error) {
////	addr, err := wallet2.GetAddress()
////	if err != nil {
////		return nil, err
////	}
////	script, err := utils.Blech32ToScript(addr, &network.Regtest)
////	if err != nil {
////		return nil, err
////	}
////	val, _ := elementsutil.SatoshiToElementsValue(upperFeeBound)
////	txOutput := transaction.NewTxOutput(lbtc, val,script)
////	tx := transaction.NewTx(2)
////	tx.Outputs = append(tx.Outputs, txOutput)
////	txHex, err := tx.ToHex()
////	if err != nil {
////		return nil, err
////	}
////	res, err := elements.FundRawTx(txHex)
////	if err != nil {
////		return nil, err
////	}
////
////	return transaction.NewTxFromHex(res.TxString)
////}
////
////func getPreimageWitness(signature, preimage, redeemScript []byte) [][]byte {
////	sigWithHashType := append(signature, byte(txscript.SigHashAll))
////	witness := make([][]byte, 0)
////	witness = append(witness, preimage[:])
////	witness = append(witness, sigWithHashType)
////	witness = append(witness, redeemScript)
////	return witness
////}
////
////func mint(address string, quantity int, name string, ticker string) (string, string, error) {
////	baseUrl, err := apiBaseUrl()
////	if err != nil {
////		return "", "", err
////	}
////
////	url := fmt.Sprintf("%s/mint", baseUrl)
////	payload := map[string]interface{}{
////		"address":  address,
////		"quantity": quantity,
////		"name":     name,
////		"ticker":   ticker,
////	}
////	body, _ := json.Marshal(payload)
////
////	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
////	if err != nil {
////		return "", "", err
////	}
////
////	data, err := ioutil.ReadAll(resp.Body)
////	if err != nil {
////		return "", "", err
////	}
////
////	if res := string(data); len(res) <= 0 || strings.Contains(res, "sendtoaddress") {
////		return "", "", fmt.Errorf("cannot fund address with minted asset: %s", res)
////	}
////
////	respBody := map[string]interface{}{}
////	if err := json.Unmarshal(data, &respBody); err != nil {
////		return "", "", err
////	}
////	return respBody["txId"].(string), respBody["asset"].(string), nil
////}
////func apiBaseUrl() (string, error) {
////	return "http://localhost:3001", nil
////}
//
////func Test_FeeEstimation(t *testing.T) {
////
////	testSetup, err := NewTestSetup()
////	if err != nil {
////		t.Fatal(err)
////	}
////	//eCLi := gbitcoin.NewBitcoin("admin1","123","")
////	ecli := gelements.NewElements("admin1", "123")
////	t.Log("new ecli")
////	err = ecli.StartUp("http://localhost", LiquidPort)
////	if err != nil {
////		t.Fatalf("error testing rpc wallet %v", err)
////	}
////	walletCli, err := wallet.NewRpcWallet(ecli, newWalletId())
////	if err != nil {
////		t.Fatalf("err creating wallet %v", err)
////	}
////	err = testSetup.FaucetWallet(walletCli, 1)
////	if err != nil {
////		t.Fatal(err)
////	}
////
////	// Generate Preimage
////	var preimage lightning.Preimage
////
////	if _, err := rand.Read(preimage[:]); err != nil {
////		t.Fatal(err)
////	}
////	pHash := preimage.Hash()
////
////	alicePrivkey := getRandomPrivkey()
////	bobPrivkey := getRandomPrivkey()
////
////	redeemScript, err := utils.GetOpeningTxScript(alicePrivkey.PubKey().SerializeCompressed(), bobPrivkey.PubKey().SerializeCompressed(), pHash[:], uint32(100))
////	if err != nil {
////		t.Fatalf("error creating opening tx: %v", err)
////	}
////	tx, err := utils.CreateOpeningTransaction(redeemScript, lbtc, 10000)
////	if err != nil {
////		t.Fatal(err)
////	}
////	t.Logf("len outputs %v", len(tx.Outputs))
////	txHex, err := tx.ToHex()
////	if err != nil {
////		t.Fatal(err)
////	}
////
////	fundedTx, err := ecli.FundRawTx(txHex)
////	if err != nil {
////		t.Fatal(err)
////	}
////	err = util.CheckTransactionValidity(fundedTx.TxString, 10000, redeemScript)
////	if err != nil {
////		t.Fatalf("error checking txValidty %v", err)
////	}
////	tx, err = transaction.NewTxFromHex(fundedTx.TxString)
////	if err != nil {
////		t.Fatal(err)
////	}
////
////	t.Logf("size: %v, fee: %f, num outputs %v, num inputs %v, ", tx.VirtualSize(), fundedTx.Fee*100000000, len(tx.Outputs), len(tx.Inputs))
////	for i, o := range tx.Outputs {
////		sats, err := elementsutil.ElementsToSatoshiValue(o.Value)
////		if err != nil {
////			t.Log(err)
////		}
////
////		t.Logf("output %v %v %v %v", i, o.Nonce, len(o.Nonce), sats)
////	}
////
////	unblinded, err := ecli.BlindRawTransaction(fundedTx.TxString)
////	if err != nil {
////		t.Fatal(err)
////	}
////
////	tx, err = transaction.NewTxFromHex(unblinded)
////	if err != nil {
////		t.Fatal(err)
////	}
////	t.Logf("size: %v, fee: %f, num outputs %v, num inputs %v, ", tx.VirtualSize(), fundedTx.Fee*100000000, len(tx.Outputs), len(tx.Inputs))
////	for i, o := range tx.Outputs {
////		sats, err := elementsutil.ElementsToSatoshiValue(o.Value)
////		if err != nil {
////			t.Log(err)
////		}
////
////		t.Logf("output %v %v %v %v", i, o.Nonce, len(o.Nonce), sats)
////	}
////	finalized, err := ecli.SignRawTransactionWithWallet(unblinded)
////	if err != nil {
////		t.Fatal(err)
////	}
////	txid, err := testSetup.BroadcastAndGenerateN(finalized.Hex, 10)
////	if err != nil {
////		t.Fatal(err)
////	}
////	t.Logf("tx %s", txid)
////
////	rawTxHex, err := getRawTx(ecli, txid, finalized.Hex, redeemScript)
////	if err != nil {
////		t.Fatal(err)
////	}
////	// create output for redeemtransaction
////	newAddr, err := ecli.GetNewAddress(int(gelements.Bech32))
////	if err != nil {
////		t.Fatalf("error testing rpc wallet %v", err)
////	}
////	blechScript, err := utils.Blech32ToScript(newAddr, &network.Regtest)
////	if err != nil {
////		t.Fatalf("error creating blechscript %v", err)
////	}
////
////	spendingTx, sigHash, err := util.CreateSpendingTransaction(rawTxHex, 10000, 500, 0, lbtc, redeemScript, blechScript)
////	if err != nil {
////		t.Fatal(err)
////	}
////
////	sig, err := alicePrivkey.Sign(sigHash[:])
////	if err != nil {
////		t.Fatal(err)
////	}
////	spendingTx.Inputs[0].Witness = util.GetPreimageWitness(sig.Serialize(), preimage[:], redeemScript)
////	spendingTxHex, err := spendingTx.ToHex()
////	if err != nil {
////		t.Fatal(err)
////	}
////	spendingTxId, err := testSetup.BroadcastAndGenerateN(spendingTxHex, 1)
////	if err != nil {
////		t.Fatalf("error testing rpc wallet %v", err)
////	}
////	t.Logf("spending txId %s", spendingTxId)
////
////}
//
//func Test_RpcWalletPreimage(t *testing.T) {
//	testSetup, err := NewTestSetup()
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	err = testSetup.FaucetCli(testSetup.walletCli, 1)
//	if err != nil {
//		t.Fatalf("err funding wallet %v", err)
//	}
//	blockCount, err := testSetup.Elcli.GetBlockHeight()
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
//	//blindingKey := getRandomPrivkey()
//
//	openingParams := &swap.OpeningParams{
//		TakerPubkey:  fmt.Sprintf("%x", alicePrivkey.PubKey().SerializeCompressed()),
//		MakerPubkey:  fmt.Sprintf("%x", bobPrivkey.PubKey().SerializeCompressed()),
//		ClaimPaymentHash: fmt.Sprintf("%x", pHash),
//		Amount:           100000,
//	}
//
//	claimParams := &swap.ClaimParams{
//		Preimage: fmt.Sprintf("%x", preimage[:]),
//		Signer:   alicePrivkey,
//	}
//
//	log.Printf("preimage %x", preimage[:])
//
//	redeemScript, err := onchain.ParamsToTxScript(openingParams, onchain.LiquidCsv)
//	if err != nil {
//		t.Fatalf("error creating opening tx: %v", err)
//	}
//
//	//openingTxAddr, err := testSetup.onchain.CreateBlindedOpeningAddress(redeemScript, blindingKey.PubKey())
//	openingTxAddr, err := testSetup.onchain.CreateOpeningAddress(redeemScript)
//	if err != nil {
//		t.Fatalf("error creating opening tx: %v", err)
//	}
//
//	txId, err := testSetup.walletCli.SendToAddress(openingTxAddr, "0.001")
//	if err != nil {
//		t.Fatalf("error testing rpc wallet %v", err)
//	}
//	t.Logf("opening txId %s", txId)
//
//	_, err = testSetup.Elcli.GenerateToAddress(regtestOpReturnAddress, 1)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	// create output for redeemtransaction
//	txId, _, err = testSetup.onchain.CreatePreimageSpendingTransaction(openingParams, claimParams, txId)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	t.Logf("spending txId %s", txId)
//	//
//	//sig, err := alicePrivkey.Sign(sigHash[:])
//	//if err != nil {
//	//	t.Fatal(err)
//	//}
//	//spendingTx.Inputs[0].Witness = onchain.GetPreimageWitness(sig.Serialize(), preimage[:], redeemScript)
//	//spendingTxHex, err := spendingTx.ToHex()
//	//if err != nil {
//	//	t.Fatal(err)
//	//}
//	//spendingTxId, err := testSetup.BroadcastAndGenerateN(spendingTxHex, 1)
//	//if err != nil {
//	//	t.Fatalf("error testing rpc wallet %v", err)
//	//}
//	//t.Logf("spending txId %s", spendingTxId)
//	//
//	//// generate a blocks
//	//_, err = ecli.GenerateToAddress(regtestOpReturnAddress, 1)
//	//if err != nil {
//	//	t.Fatalf("error testing rpc wallet %v", err)
//	//}
//
//}
//
////func Test_RpcWalletCsv(t *testing.T) {
////	testSetup, err := NewTestSetup()
////	if err != nil {
////		t.Fatal(err)
////	}
////	//eCLi := gbitcoin.NewBitcoin("admin1","123","")
////	ecli := gelements.NewElements("admin1", "123")
////	t.Log("new ecli")
////	err = ecli.StartUp("http://localhost", LiquidPort)
////	if err != nil {
////		t.Fatalf("error testing rpc wallet %v", err)
////	}
////	walletCli, err := wallet.NewRpcWallet(ecli, newWalletId())
////	if err != nil {
////		t.Fatalf("err creating wallet %v", err)
////	}
////	err = testSetup.FaucetCli(ecli, 1)
////	if err != nil {
////		t.Fatalf("err fnding wallet %v", err)
////	}
////	blockCount, err := ecli.GetBlockHeight()
////	if err != nil {
////		t.Fatalf("error testing rpc wallet %v", err)
////	}
////	t.Logf("blockcount %v", blockCount)
////
////	// Generate Preimage
////	var preimage lightning.Preimage
////
////	if _, err := rand.Read(preimage[:]); err != nil {
////		t.Fatal(err)
////	}
////	pHash := preimage.Hash()
////
////	alicePrivkey := getRandomPrivkey()
////	bobPrivkey := getRandomPrivkey()
////
////	redeemScript, err := utils.GetOpeningTxScript(alicePrivkey.PubKey().SerializeCompressed(), bobPrivkey.PubKey().SerializeCompressed(), pHash[:], int64(blockCount+5))
////	if err != nil {
////		t.Fatalf("error creating opening tx: %v", err)
////	}
////	openingTxAddr, err := utils.CreateOpeningAddress(redeemScript)
////	if err != nil {
////		t.Fatalf("error creating opening tx: %v", err)
////	}
////	txId, err := walletCli.SendToAddress(openingTxAddr, 10000)
////	if err != nil {
////		t.Fatalf("error testing rpc wallet %v", err)
////	}
////	t.Logf("txId %s", txId)
////
////	_, err = ecli.GenerateToAddress(regtestOpReturnAddress, 5)
////	if err != nil {
////		t.Fatal(err)
////	}
////	// create output for redeemtransaction
////	newAddr, err := ecli.GetNewAddress(int(gelements.Bech32))
////	if err != nil {
////		t.Fatalf("error testing rpc wallet %v", err)
////	}
////	blechScript, err := utils.Blech32ToScript(newAddr, &network.Regtest)
////	if err != nil {
////		t.Fatalf("error creating blechscript %v", err)
////	}
////
////	rawTx, err := getRawTx(ecli, )
////	blockCount, err = ecli.GetBlockHeight()
////	if err != nil {
////		t.Fatalf("error testing rpc wallet %v", err)
////	}
////	util := &utils.Utility{}
////	spendingTx, sigHash, err := util.CreateSpendingTransaction(rawTx, 10000, 500, blockCount, lbtc, redeemScript, blechScript)
////	if err != nil {
////		t.Fatal(err)
////	}
////	sig, err := bobPrivkey.Sign(sigHash[:])
////	if err != nil {
////		t.Fatal(err)
////	}
////
////	spendingTx.Inputs[0].Witness = util.GetCsvWitness(sig.Serialize(), redeemScript)
////	spendingTxHex, err := spendingTx.ToHex()
////	if err != nil {
////		t.Fatal(err)
////	}
////
////	spendingTxId, err := testSetup.BroadcastAndGenerateN(spendingTxHex, 1)
////	if err != nil {
////		t.Fatalf("error testing rpc wallet %v", err)
////	}
////	t.Logf("spending txId %s", spendingTxId)
////
////	// generate a blocks
////	_, err = ecli.GenerateToAddress(regtestOpReturnAddress, 1)
////	if err != nil {
////		t.Fatalf("error testing rpc wallet %v", err)
////	}
////
////}
//
//func getRandomPrivkey() *btcec.PrivateKey {
//	privkey, err := btcec.NewPrivateKey(btcec.S256())
//	if err != nil {
//		return nil
//	}
//	return privkey
//}
//
//type TestSetup struct {
//	Elcli     *gelements.Elements
//	walletCli *gelements.Elements
//	onchain   *onchain.LiquidOnChain
//}
//
//func NewTestSetup() (*TestSetup, error) {
//	walletCli := gelements.NewElements("admin1", "123")
//	err := walletCli.StartUp("http://localhost", LiquidPort)
//	if err != nil {
//		return nil, err
//	}
//	normalCli := gelements.NewElements("admin1", "123")
//	err = normalCli.StartUp("http://localhost", LiquidPort)
//	if err != nil {
//		return nil, err
//	}
//	liquidWallet, err := wallet.NewRpcWallet(walletCli, "swaptest")
//	if err != nil {
//		return nil, err
//	}
//	onchain := onchain.NewLiquidOnChain(walletCli, nil, liquidWallet, &network.Regtest)
//
//	return &TestSetup{Elcli: normalCli, walletCli: walletCli, onchain: onchain}, nil
//}
//
//func (t *TestSetup) FaucetCli(walletCli *gelements.Elements, amount float64) error {
//	addr, err := walletCli.GetNewAddress(0)
//	if err != nil {
//		return err
//	}
//	return t.Faucet(addr, amount)
//}
//func (t *TestSetup) FaucetWallet(wallet wallet.Wallet, amount float64) error {
//	addr, err := wallet.GetAddress()
//	if err != nil {
//		return err
//	}
//	return t.Faucet(addr, amount)
//}
//
//func (t *TestSetup) Faucet(address string, amount float64) error {
//
//	_, err := t.Elcli.SendToAddress(address, fmt.Sprintf("%f", amount))
//	if err != nil {
//		return err
//	}
//	return t.GenerateBlock(1)
//}
//
//func (t *TestSetup) GenerateBlock(n uint) error {
//	_, err := t.Elcli.GenerateToAddress(regtestOpReturnAddress, n)
//	return err
//}
//
//func (t *TestSetup) BroadcastAndGenerateN(txHex string, nBlocks uint) (string, error) {
//	txId, err := t.Elcli.SendRawTx(txHex)
//	if err != nil {
//		return "", err
//	}
//	err = t.GenerateBlock(nBlocks)
//	if err != nil {
//		return "", err
//	}
//	return txId, nil
//
//}
//
//func newWalletId() string {
//	idBytes := make([]byte, 32)
//	_, _ = rand.Read(idBytes[:])
//	return hex.EncodeToString(idBytes)
//}
//func h2b(str string) []byte {
//	buf, _ := hex.DecodeString(str)
//	return buf
//}
//
//func getRawTx(ecli *gelements.Elements, chain *onchain.LiquidOnChain, txid, txHex string, redeemScript []byte) (string, error) {
//	vout, err := chain.VoutFromTxHex(txHex, redeemScript)
//	if err != nil {
//		return "", err
//	}
//	txOut, err := ecli.GetTxOut(txid, vout)
//	if err != nil {
//		return "", err
//	}
//	blockheight, err := ecli.GetBlockHeight()
//	if err != nil {
//		return "", err
//	}
//	blockhash, err := ecli.GetBlockHash(uint32(blockheight) - txOut.Confirmations + 1)
//	if err != nil {
//		return "", err
//	}
//	rawTxHex, err := ecli.GetRawtransactionWithBlockHash(txid, blockhash)
//	if err != nil {
//		return "", err
//	}
//	return rawTxHex, nil
//}
