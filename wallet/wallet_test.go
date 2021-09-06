// +build docker

package wallet

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/sputn1ck/glightning/gelements"
	"github.com/stretchr/testify/assert"
	"testing"
)

var (
	regtestOpReturnAddress = "ert1qfkht0df45q00kzyayagw6vqhfhe8ve7z7wecm0xsrkgmyulewlzqumq3ep"
)

const (
	LiquidPort = 18884
)

func Test_RpcWallet(t *testing.T) {

	testSetup, err := NewTestSetup()
	if err != nil {
		t.Fatal(err)
	}

	walletCli := gelements.NewElements("admin1", "123")
	err = walletCli.StartUp("http://localhost", LiquidPort)
	if err != nil {
		t.Fatalf("error testing rpc wallet %v", err)
	}

	wallet, err := NewRpcWallet(walletCli, newwalletId())
	if err != nil {
		t.Fatalf("error creating wallet %v", err)
	}

	startingBalance, err := wallet.GetBalance()
	if err != nil {
		t.Fatalf("error getting balance %v", err)
	}

	addr, err := wallet.GetAddress()
	if err != nil {
		t.Fatalf("error getting address %v", err)
	}

	err = testSetup.Faucet(addr, 0.1)
	if err != nil {
		t.Fatalf("error funding wallet %v", err)
	}

	newBalance, err := wallet.GetBalance()
	if err != nil {
		t.Fatalf("error getting balance")
	}

	assert.Equal(t, startingBalance+10000000, newBalance)

	_, err = wallet.SendToAddress(regtestOpReturnAddress, 5000000)
	if err != nil {
		t.Fatalf("error sending to address %v", err)
	}

	err = testSetup.GenerateBlock()
	if err != nil {
		t.Fatalf("error generating block %v", err)
	}

	newBalance, err = wallet.GetBalance()
	if err != nil {
		t.Fatalf("error getting balance %v", err)
	}

	assert.Less(t, newBalance, startingBalance-5000000)

}

type TestSetup struct {
	Elcli *gelements.Elements
}

func NewTestSetup() (*TestSetup, error) {
	walletCli := gelements.NewElements("admin1", "123")
	err := walletCli.StartUp("http://localhost", LiquidPort)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("error creating test setup %v", err))
	}
	return &TestSetup{Elcli: walletCli}, nil
}

func (t *TestSetup) Faucet(address string, amount float64) error {

	_, err := t.Elcli.SendToAddress(address, fmt.Sprintf("%f", amount))
	if err != nil {
		return err
	}
	return t.GenerateBlock()
}

func (t *TestSetup) GenerateBlock() error {
	_, err := t.Elcli.GenerateToAddress(regtestOpReturnAddress, 1)
	return err
}

func newwalletId() string {
	idBytes := make([]byte, 8)
	_, _ = rand.Read(idBytes[:])
	return hex.EncodeToString(idBytes)
}
