package liquid

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/sputn1ck/liquid-loop/wallet"
	"github.com/vulpemventures/go-elements/transaction"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type EsploraClient struct {
	baseUrl    string
	httpClient *http.Client
}

func NewEsploraClient(baseUrl string) *EsploraClient {
	return &EsploraClient{
		httpClient: &http.Client{
			Timeout: time.Minute,
		}, baseUrl: baseUrl}
}

func (e *EsploraClient) GetBlockHeight() (int, error) {
	data, err := e.getRequest("/blocks/tip/height")
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(string(data))
}

func (e *EsploraClient) BroadcastTransaction(txHex string) (string, error) {
	resp, err := http.Post(fmt.Sprintf("%s/tx", e.baseUrl), "text/plain", strings.NewReader(txHex))
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

func (e *EsploraClient) FetchTxHex(txId string) (string, error) {
	byteString, err := e.getRequest(fmt.Sprintf("/tx/%s/hex", txId))
	if err != nil {
		return "", err
	}
	return string(byteString), nil
}

func (e *EsploraClient) FetchUtxos(address string) ([]*wallet.Utxo, error) {
	var utxos []*wallet.Utxo

	err := e.getJsonRequest(fmt.Sprintf("/address/%s/utxo", address), &utxos)
	if err != nil {
		return nil, err
	}

	return utxos, nil
}

func (e *EsploraClient) DEV_Fundaddress(address string) (string,error) {
	url := fmt.Sprintf("%s/faucet", "http://localhost:3001")
	payload := map[string]string{"address": address, "amount": fmt.Sprintf("%v", 1)}
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
func (e *EsploraClient) getJsonRequest(endpoint string, returnVal interface{}) error {
	data, err := e.getRequest(endpoint)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &returnVal)
}

func (e *EsploraClient) getRequest(endpoint string) ([]byte, error) {
	url := fmt.Sprintf("%s%s", e.baseUrl, endpoint)
	resp, err := e.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(resp.Body)
}

func (e *EsploraClient) WalletUtxosToTxInputs(utxos []*wallet.Utxo) ([]*transaction.TxInput, error) {
	var txInputs []*transaction.TxInput
	for _, v := range utxos {
		txIdBytes, err := hex.DecodeString(v.TxId)
		if err != nil {
			return nil, err
		}
		input := transaction.NewTxInput(txIdBytes, v.VOut)
		txInputs = append(txInputs, input)
	}
	return txInputs, nil
}
