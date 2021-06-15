package tests

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"github.com/sputn1ck/sugarmama/lightning"
	"github.com/sputn1ck/sugarmama/liquid"
	"github.com/sputn1ck/sugarmama/swap"
	"github.com/sputn1ck/sugarmama/wallet"
	"github.com/vulpemventures/go-elements/network"
	"testing"
	"time"
)

func Test_Swap(t *testing.T) {
	//Setup Communicators
	aliceCom := &TestCommunicator{
		testing: t,
	}
	bobCom := &TestCommunicator{other: aliceCom, testing: t}
	aliceCom.other = bobCom

	// Generate Preimage
	var preimage lightning.Preimage

	if _, err := rand.Read(preimage[:]); err != nil {
		t.Fatal(err)
	}

	// Create Setups
	aliceSetup, err := GetTestSetup(preimage, aliceCom)
	if err != nil {
		t.Fatal(err)
	}
	bobSetup, err := GetTestSetup(preimage, bobCom)
	if err != nil {
		t.Fatal(err)
	}

	// Fund Wallets
	err = aliceSetup.FundWallet()
	if err != nil {
		t.Fatal(err)
	}
	err = bobSetup.FundWallet()
	if err != nil {
		t.Fatal(err)
	}
	// Start tx watcher
	aliceSetup.Start(t)
	bobSetup.Start(t)

	time.Sleep(5 * time.Second)
	//Swap out
	err = aliceSetup.swap.StartSwapIn("bob", "foo", 6000000)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Second)

}

func GetTestSetup(preimage lightning.Preimage, communicator *TestCommunicator) (*Test_Setup, error) {
	ctx := context.Background()
	esplora := liquid.NewEsploraClient("http://localhost:3001")
	walletStore := &wallet.DummyWalletStore{}
	err := walletStore.Initialize()
	if err != nil {
		return nil, err
	}
	walletService := wallet.NewLiquiddWallet(walletStore, esplora, &network.Regtest)

	clightning := &TestLightningClient{
		Payreq:   "gude",
		PreImage: preimage,
		Value:    100,
	}
	swapStore := swap.NewInMemStore()

	swapService := swap.NewService(ctx, swapStore, walletService, communicator, esplora, clightning, &network.Regtest)

	messageHandler := swap.NewMessageHandler(communicator, swapService)
	err = messageHandler.Start()
	if err != nil {
		return nil, err
	}

	return &Test_Setup{
		swap:    swapService,
		esplora: esplora,
		wallet:  walletService,
		ln:      clightning,
		mh:      messageHandler,
	}, nil
}

func (s *Test_Setup) FundWallet() error {
	addr, err := s.wallet.ListAddresses()
	if err != nil {
		return err
	}
	_, err = s.esplora.DEV_Fundaddress(addr[0])
	if err != nil {
		return err
	}
	return nil
}

func (s *Test_Setup) Start(t *testing.T) {
	go func() {
		err := s.swap.StartWatchingTxs()
		if err != nil {
			t.Fatal(err)
		}
	}()
}

type Test_Setup struct {
	swap    *swap.Service
	esplora *liquid.EsploraClient
	wallet  *wallet.LiquiddWallet
	ln      *TestLightningClient
	mh      *swap.MessageHandler
}

type TestLightningClient struct {
	Payreq   string
	PreImage lightning.Preimage
	Value    uint64
}

func (t *TestLightningClient) GetPreimage() (lightning.Preimage, error) {
	return t.PreImage, nil
}

func (t *TestLightningClient) GetPayreq(amount uint64, preImage string, label string) (string, error) {
	return "gude", nil
}

func (t *TestLightningClient) DecodePayreq(payreq string) (*lightning.Invoice, error) {
	return &lightning.Invoice{
		PHash:       t.PreImage.Hash().String(),
		Amount:      t.Value,
		Description: "",
	}, nil
}

func (t *TestLightningClient) PayInvoice(payreq string) (preimage string, err error) {
	return t.PreImage.String(), nil
}

type TestCommunicator struct {
	testing *testing.T
	other   *TestCommunicator
	F       func(peerId string, messageType string, payload string) error
}

func (t *TestCommunicator) SendMessage(peerId string, message lightning.PeerMessage) error {
	msg, err := json.Marshal(message)
	if err != nil {
		t.testing.Fatal(err)
	}
	err = t.other.F(peerId, message.MessageType(), hex.EncodeToString(msg))
	if err != nil {
		t.testing.Fatal(err)
	}
	return nil
}

func (t *TestCommunicator) AddMessageHandler(f func(peerId string, messageType string, payload string) error) error {
	t.F = f
	return nil
}
