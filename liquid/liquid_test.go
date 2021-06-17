package liquid

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/btcsuite/btcd/btcec"
	"github.com/sputn1ck/glightning/gelements"
	"github.com/sputn1ck/sugarmama/lightning"
	"github.com/sputn1ck/sugarmama/utils"
	"github.com/sputn1ck/sugarmama/wallet"
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
	alicePrivkey           = "b5ca71cc0ea0587fc40b3650dfb12c1e50fece3b88593b223679aea733c55605"
	esplora                = NewEsploraClient("http://localhost:3001")
	regtestOpReturnAddress = "ert1qfkht0df45q00kzyayagw6vqhfhe8ve7z7wecm0xsrkgmyulewlzqumq3ep"
)

func Test_RpcWalletPreimage(t *testing.T) {
	//eCLi := gbitcoin.NewBitcoin("admin1","123","")
	walletCli := gelements.NewElements("admin1", "123", "swap")
	t.Log("new walletCli")
	err := walletCli.StartUp("http://localhost", 7041)
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}

	blockCount, err := walletCli.GetBlockcount()
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
	txId, err := walletCli.SendToAddress(openingTxAddr, "0.0001")
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}
	t.Logf("txId %s", txId)

	_, err = walletCli.GenerateToAddress(regtestOpReturnAddress, 1)
	if err != nil {
		t.Fatal(err)
	}
	// create output for redeemtransaction
	newAddr, err := walletCli.GetNewAddress(gelements.Bech32)
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}
	blechScript, err := utils.Blech32ToScript(newAddr, &network.Regtest)
	if err != nil {
		t.Fatalf("error creating blechscript %v", err)
	}

	rawTx, err := walletCli.GetRawtransaction(txId)
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}

	spendingTxHex, err := utils.CreatePreimageSpendingTransaction(alicePrivkey, rawTx, 10000, 500, 0, lbtc, blechScript, redeemScript, preimage[:])
	if err != nil {
		t.Fatalf("error creating spending transaction: %v", err)
	}
	spendingTxId, err := esplora.BroadcastTransaction(spendingTxHex)
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}
	t.Logf("spending txId %s", spendingTxId)

	// generate a blocks
	_, err = walletCli.GenerateToAddress(regtestOpReturnAddress, 1)
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}

}

func faucet(address string, amount float64) (string, error) {
	if address == "" {
		address = getRandomAddress()
	}
	nextBlockChan := make(chan int)
	waitNextBlock(nextBlockChan)

	url := fmt.Sprintf("%s/faucet", "http://localhost:3001")
	payload := map[string]string{"address": address, "amount": fmt.Sprintf("%v", amount)}
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

	<-nextBlockChan
	return respBody["txId"], nil
}

func getRandomAddress() string {
	rWallet := &wallet.DummyWalletStore{}
	_ = rWallet.Initialize()
	addr, _ := rWallet.ListAddresses()
	return addr[0]
}

func getRandomPrivkey() *btcec.PrivateKey {
	rWallet := &wallet.DummyWalletStore{}
	_ = rWallet.Initialize()
	privkey, _ := rWallet.LoadPrivKey()
	return privkey
}

func waitNextBlock(nextBlockChan chan int) {
	timeOut := time.After(10 * time.Second)
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
					log.Printf("error getting bext block %v", err)
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

func waitBalanceChange(walletService *wallet.LiquiddWallet, newBalanceChan chan uint64) (uint64, error) {
	timeOut := time.After(10 * time.Second)
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
