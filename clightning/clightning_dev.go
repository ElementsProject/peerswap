// +build dev

package clightning

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/sputn1ck/glightning/jrpc2"
	"io/ioutil"
	"net/http"
	"strings"
)

var (
	regtestOpReturnAddress = "ert1qfkht0df45q00kzyayagw6vqhfhe8ve7z7wecm0xsrkgmyulewlzqumq3ep"
)

func init() {
	methods = append(methods, &FaucetMethod{}, &GenerateMethod{})
}

type FaucetMethod struct {
	cl *ClightningClient `json:"-"`
}

func (g *FaucetMethod) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &FaucetMethod{
		cl: client,
	}
}

func (g *FaucetMethod) Name() string {
	return "dev-liquid-faucet"
}

func (g *FaucetMethod) New() interface{} {
	return &FaucetMethod{
		cl: g.cl,
	}
}

func (g *FaucetMethod) Call() (jrpc2.Result, error) {
	addr, err := g.cl.wallet.GetAddress()
	if err != nil {
		return nil, err
	}
	res, err := faucet(addr)
	return res, err
}

type GenerateMethod struct {
	amount int `json:"amount`

	cl *ClightningClient `json:"-"`
}

func (g *GenerateMethod) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &GenerateMethod{
		cl: client,
	}
}

func (g *GenerateMethod) Name() string {
	return "dev-liquid-generate"
}

func (g *GenerateMethod) New() interface{} {
	return &GenerateMethod{
		cl: g.cl,
	}
}

func (g *GenerateMethod) Call() (jrpc2.Result, error) {
	if g.amount == 0 {
		g.amount = 1
	}
	res, err := g.cl.Gelements.GenerateToAddress(regtestOpReturnAddress, uint(g.amount))
	return res, err
}

func faucet(address string) (string, error) {
	baseURL := "http://localhost:3001"

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
