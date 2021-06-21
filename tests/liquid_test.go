package tests

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/btcec"
	"github.com/sputn1ck/glightning/gelements"
	"github.com/sputn1ck/sugarmama/lightning"
	"github.com/sputn1ck/sugarmama/utils"
	wallet "github.com/sputn1ck/sugarmama/wallet"
	"github.com/vulpemventures/go-elements/elementsutil"
	"github.com/vulpemventures/go-elements/network"

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

func Test_RpcWalletPreimage(t *testing.T) {
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
	err = testSetup.FaucetCli(ecli, 1)
	if err != nil {
		t.Fatalf("err fnding wallet %v", err)
	}
	blockCount, err := ecli.GetBlockHeight()
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}
	t.Logf("blockcount %v", blockCount)

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
	openingTxAddr, err := utils.CreateOpeningAddress(redeemScript)
	if err != nil {
		t.Fatalf("error creating opening tx: %v", err)
	}
	txId, err := walletCli.SendToAddress(openingTxAddr, 10000)
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}
	t.Logf("txId %s", txId)

	_, err = ecli.GenerateToAddress(regtestOpReturnAddress, 1)
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

	rawTx, err := ecli.GetRawtransaction(txId)
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}

	params := &utils.SpendingParams{
		Signer:       alicePrivkey,
		OpeningTxHex: rawTx,
		SwapAmount:   10000,
		FeeAmount:    500,
		CurrentBlock: 0,
		Asset:        lbtc,
		OutputScript: blechScript,
		RedeemScript: redeemScript,
	}
	spendingTxHex, err := utils.CreatePreimageSpendingTransaction(params, preimage[:])
	if err != nil {
		t.Fatalf("error creating spending transaction: %v", err)
	}
	spendingTxId, err := testSetup.BroadcastAndGenerateN(spendingTxHex, 1)
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}
	t.Logf("spending txId %s", spendingTxId)

	// generate a blocks
	_, err = ecli.GenerateToAddress(regtestOpReturnAddress, 1)
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}

}
func Test_RpcWalletCltv(t *testing.T) {
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
	err = testSetup.FaucetCli(ecli, 1)
	if err != nil {
		t.Fatalf("err fnding wallet %v", err)
	}
	blockCount, err := ecli.GetBlockHeight()
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}
	t.Logf("blockcount %v", blockCount)

	// Generate Preimage
	var preimage lightning.Preimage

	if _, err := rand.Read(preimage[:]); err != nil {
		t.Fatal(err)
	}
	pHash := preimage.Hash()

	alicePrivkey := getRandomPrivkey()
	bobPrivkey := getRandomPrivkey()

	redeemScript, err := utils.GetOpeningTxScript(alicePrivkey.PubKey().SerializeCompressed(), bobPrivkey.PubKey().SerializeCompressed(), pHash[:], int64(blockCount+5))
	if err != nil {
		t.Fatalf("error creating opening tx: %v", err)
	}
	openingTxAddr, err := utils.CreateOpeningAddress(redeemScript)
	if err != nil {
		t.Fatalf("error creating opening tx: %v", err)
	}
	txId, err := walletCli.SendToAddress(openingTxAddr, 10000)
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}
	t.Logf("txId %s", txId)

	_, err = ecli.GenerateToAddress(regtestOpReturnAddress, 5)
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

	rawTx, err := ecli.GetRawtransaction(txId)
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}
	blockCount, err = ecli.GetBlockHeight()
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}
	params := &utils.SpendingParams{
		Signer:       bobPrivkey,
		OpeningTxHex: rawTx,
		SwapAmount:   10000,
		FeeAmount:    500,
		CurrentBlock: blockCount,
		Asset:        lbtc,
		OutputScript: blechScript,
		RedeemScript: redeemScript,
	}
	spendingTxHex, err := utils.CreateCltvSpendingTransaction(params)
	if err != nil {
		t.Fatalf("error creating spending transaction: %v", err)
	}
	spendingTxId, err := testSetup.BroadcastAndGenerateN(spendingTxHex, 1)
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}
	t.Logf("spending txId %s", spendingTxId)

	// generate a blocks
	_, err = ecli.GenerateToAddress(regtestOpReturnAddress, 1)
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}

}

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
	idBytes := make([]byte, 8)
	_, _ = rand.Read(idBytes[:])
	return hex.EncodeToString(idBytes)
}
func h2b(str string) []byte {
	buf, _ := hex.DecodeString(str)
	return buf
}
