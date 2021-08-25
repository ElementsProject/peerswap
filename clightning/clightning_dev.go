//go:build dev
// +build dev

package clightning

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"github.com/sputn1ck/glightning/jrpc2"
	"github.com/sputn1ck/peerswap/lightning"
)

var (
	regtestOpReturnAddress = "ert1qfkht0df45q00kzyayagw6vqhfhe8ve7z7wecm0xsrkgmyulewlzqumq3ep"
)

func init() {
	devmethods = append(devmethods, &FaucetMethod{}, &GenerateMethod{}, &BigInvoice{}, &BigPay{})
}

type FaucetMethod struct {
	cl *ClightningClient `json:"-"`
}

func (g *FaucetMethod) Description() string {
	return "faucets liquid funds to local wallet"
}

func (g *FaucetMethod) LongDescription() string {
	return ""
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

func (g *GenerateMethod) Description() string {
	return "generates liquid blocks"
}

func (g *GenerateMethod) LongDescription() string {
	return ""
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

type BigInvoice struct {
	SatAmt uint64
	cl     *ClightningClient
}

func (b *BigInvoice) Name() string {
	return "biginvoice"
}

func (b *BigInvoice) New() interface{} {
	return &BigInvoice{
		cl:     b.cl,
		SatAmt: b.SatAmt,
	}
}

func (b *BigInvoice) Call() (jrpc2.Result, error) {
	if b.SatAmt == 0 {
		b.SatAmt = 50000000
	}
	log.Printf("satamt: %v", b.SatAmt)
	preimage, err := lightning.GetPreimage()
	if err != nil {
		return nil, err
	}
	invoice, err := b.cl.glightning.CreateInvoice(uint64(b.SatAmt*1000), randomString(), randomString(), 72000000, nil, preimage.String(), false)
	if err != nil {
		return nil, err
	}
	return invoice.Bolt11, nil
}

func (b *BigInvoice) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &BigInvoice{
		cl: client,
	}
}

func (b *BigInvoice) Description() string {
	return "biginvoice"
}

func (b *BigInvoice) LongDescription() string {
	return "biginvoice"
}

type BigPay struct {
	Payreq    string
	ChannelId string
	cl        *ClightningClient
}

func (b *BigPay) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &BigPay{cl: client}
}

func (b *BigPay) Description() string {
	return "bigpay"
}

func (b *BigPay) LongDescription() string {
	return "bigpay"
}

func (b *BigPay) Name() string {
	return "bigpay"
}

func (b *BigPay) New() interface{} {
	return &BigPay{cl: b.cl}
}

func (b *BigPay) Call() (jrpc2.Result, error) {
	res, err := b.cl.RebalancePayment(b.Payreq, b.ChannelId)
	if err != nil {
		return nil, err
	}
	return res, nil
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
