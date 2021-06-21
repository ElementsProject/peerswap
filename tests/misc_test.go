package tests

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"github.com/sputn1ck/glightning/gelements"
	"github.com/sputn1ck/sugarmama/blockchain"
	"github.com/sputn1ck/sugarmama/lightning"
	"github.com/sputn1ck/sugarmama/swap"
	wallet2 "github.com/sputn1ck/sugarmama/wallet"
	"github.com/stretchr/testify/assert"
	"github.com/vulpemventures/go-elements/network"
	"testing"
	"time"
)

// integration test without c-lightning
// todo check balances, swap states etc.

func Test_CltvSwapOut(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testSetup, err := NewTestSetup()
	if err != nil {
		t.Fatalf("error creating testSetup")
	}

	multiBlockChan := make (chan uint)
	// create some blocks
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case nChan := <- multiBlockChan:
				err := testSetup.GenerateBlock(nChan)
				if err != nil {
					t.Fatal(err)
				}
			default:
				err := testSetup.GenerateBlock(1)
				if err != nil {
					t.Fatal(err)
				}
				time.Sleep(500 * time.Millisecond)
			}
		}
	}()

	//Setup Communicators
	aliceCom := &TestCommunicator{
		testing: t,
		Id:      "alice",
	}
	bobCom := &TestCommunicator{other: aliceCom, testing: t, Id: "bob"}
	aliceCom.other = bobCom

	// Generate Preimage
	var preimage lightning.Preimage

	if _, err := rand.Read(preimage[:]); err != nil {
		t.Fatal(err)
	}

	// Create Setups
	aliceSetup, err := GetTestSetup(newWalletId(), preimage, aliceCom)
	if err != nil {
		t.Fatal(err)
	}
	bobSetup, err := GetTestSetup(newWalletId(), preimage, bobCom)
	if err != nil {
		t.Fatal(err)
	}

	oldBalance, err := bobSetup.wallet.GetBalance()
	if err != nil {
		t.Fatal(err)
	}
	go testSetup.FaucetWallet(bobSetup.wallet, 1)
	_, err = waitForBalanceChange(ctx, bobSetup.wallet, oldBalance)
	if err != nil {
		t.Fatal(err)
	}
	// Start tx watcher
	// only bob is watching tx in order to spend the cltv path
	bobSetup.Start(t)

	//Swap out
	err = aliceSetup.swap.StartSwapOut("bob", "foo", 50000)
	if err != nil {
		t.Fatal(err)
	}

	bobBalance, err := bobSetup.wallet.GetBalance()
	if err != nil {
		t.Fatal(err)
	}
	multiBlockChan <- 124
	newB, err := waitForBalanceChange(ctx, bobSetup.wallet, bobBalance)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%v", newB)

	// assert the balance is greater than 1lbtc - swap amount
	assert.Greater(t,newB, uint64(99950000))
}
func Test_CltvSwapIn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testSetup, err := NewTestSetup()
	if err != nil {
		t.Fatalf("error creating testSetup")
	}
	multiBlockChan := make (chan uint)
	// create some blocks
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case nChan := <- multiBlockChan:
				err := testSetup.GenerateBlock(nChan)
				if err != nil {
					t.Fatal(err)
				}
			default:
				err := testSetup.GenerateBlock(1)
				if err != nil {
					t.Fatal(err)
				}
				time.Sleep(500 * time.Millisecond)
			}
		}
	}()
	//Setup Communicators
	aliceCom := &TestCommunicator{
		testing: t,
		Id:      "alice",
	}
	bobCom := &TestCommunicator{other: aliceCom, testing: t, Id: "bob"}
	aliceCom.other = bobCom

	// Generate Preimage
	var preimage lightning.Preimage

	if _, err := rand.Read(preimage[:]); err != nil {
		t.Fatal(err)
	}

	// Create Setups
	aliceSetup, err := GetTestSetup(newWalletId(), preimage, aliceCom)
	if err != nil {
		t.Fatal(err)
	}
	_, err = GetTestSetup(newWalletId(), preimage, bobCom)
	if err != nil {
		t.Fatal(err)
	}

	oldBalance, err := aliceSetup.wallet.GetBalance()
	if err != nil {
		t.Fatal(err)
	}
	go testSetup.FaucetWallet(aliceSetup.wallet, 1)
	_, err = waitForBalanceChange(ctx, aliceSetup.wallet, oldBalance)
	if err != nil {
		t.Fatal(err)
	}
	// Start tx watcher
	aliceSetup.Start(t)
	//bobSetup.Start(t)

	//Swap out
	err = aliceSetup.swap.StartSwapIn("bob", "foo", 50000)
	if err != nil {
		t.Fatal(err)
	}

	oldBalance, err = aliceSetup.wallet.GetBalance()
	if err != nil {
		t.Fatal(err)
	}
	multiBlockChan <- 124
	aliceBalance, err := waitForBalanceChange(ctx, aliceSetup.wallet, oldBalance)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%v", aliceBalance)
	// assert the balance is greater than 1lbtc - swap amount
	assert.Greater(t,aliceBalance, uint64(99950000))
}
func Test_PreimageSwapIn(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testSetup, err := NewTestSetup()
	if err != nil {
		t.Fatalf("error creating testSetup")
	}
	// create some blocks
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				err := testSetup.GenerateBlock(1)
				if err != nil {
					t.Fatal(err)
				}
				time.Sleep(500 * time.Millisecond)
			}
		}
	}()
	//Setup Communicators
	aliceCom := &TestCommunicator{
		testing: t,
		Id:      "alice",
	}
	bobCom := &TestCommunicator{other: aliceCom, testing: t, Id: "bob"}
	aliceCom.other = bobCom

	// Generate Preimage
	var preimage lightning.Preimage

	if _, err := rand.Read(preimage[:]); err != nil {
		t.Fatal(err)
	}

	// Create Setups
	aliceSetup, err := GetTestSetup(newWalletId(), preimage, aliceCom)
	if err != nil {
		t.Fatal(err)
	}
	bobSetup, err := GetTestSetup(newWalletId(), preimage, bobCom)
	if err != nil {
		t.Fatal(err)
	}

	oldBalance, err := aliceSetup.wallet.GetBalance()
	if err != nil {
		t.Fatal(err)
	}

	go testSetup.FaucetWallet(aliceSetup.wallet, 1)
	_, err = waitForBalanceChange(ctx, aliceSetup.wallet, oldBalance)
	if err != nil {
		t.Fatal(err)
	}
	// Start tx watcher
	aliceSetup.Start(t)
	bobSetup.Start(t)

	oldBalance, err = bobSetup.wallet.GetBalance()
	if err != nil {
		t.Fatal(err)
	}
	//Swap out
	err = aliceSetup.swap.StartSwapIn("bob", "foo", 10000)
	if err != nil {
		t.Fatal(err)
	}

	bobBalance, err := waitForBalanceChange(ctx, bobSetup.wallet, oldBalance)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%v", bobBalance)
	assert.Equal(t, uint64(8000),bobBalance)
}
func Test_PreimageSwapOut(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testSetup, err := NewTestSetup()
	if err != nil {
		t.Fatalf("error creating testSetup")
	}
	// create some blocks
	go func() {
		for {
			select {
			case <-ctx.Done():
				t.Logf("\n context done")
				return
			default:
				err := testSetup.GenerateBlock(1)
				if err != nil {
					t.Fatal(err)
				}
				time.Sleep(500 * time.Millisecond)
			}
		}
	}()
	//Setup Communicators
	aliceCom := &TestCommunicator{
		testing: t,
		Id:      "alice",
	}
	bobCom := &TestCommunicator{other: aliceCom, testing: t, Id: "bob"}
	aliceCom.other = bobCom

	// Generate Preimage
	var preimage lightning.Preimage

	if _, err := rand.Read(preimage[:]); err != nil {
		t.Fatal(err)
	}

	// Create Setups
	aliceSetup, err := GetTestSetup(newWalletId(), preimage, aliceCom)
	if err != nil {
		t.Fatal(err)
	}
	bobSetup, err := GetTestSetup(newWalletId(), preimage, bobCom)
	if err != nil {
		t.Fatal(err)
	}

	oldBalance, err := bobSetup.wallet.GetBalance()
	if err != nil {
		t.Fatal(err)
	}
	err = testSetup.FaucetWallet(bobSetup.wallet, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = waitForBalanceChange(ctx, bobSetup.wallet, oldBalance)
	if err != nil {
		t.Fatal(err)
	}

	// Start tx watcher
	aliceSetup.Start(t)
	bobSetup.Start(t)

	oldBalance, err = aliceSetup.wallet.GetBalance()
	if err != nil {
		t.Fatal(err)
	}

	//Swap out
	err = aliceSetup.swap.StartSwapOut("bob", "foo", 10000)
	if err != nil {
		t.Fatal(err)
	}


	go testSetup.GenerateBlock(1)
	aliceBalance, err := waitForBalanceChange(ctx, aliceSetup.wallet, oldBalance)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("%v", aliceBalance)
	assert.Equal(t, uint64(8000), aliceBalance)
}

func waitForBalanceChange(ctx context.Context, wallet wallet2.Wallet, oldBalance uint64) (uint64, error) {
	balanceChan := make(chan uint64)
	go func() {
		for {
			select {
				case <-ctx.Done():
					close(balanceChan)
					return
			default:
				balance, err := wallet.GetBalance()
				if err != nil {
					close(balanceChan)
					return
				}
				if balance != oldBalance {
					balanceChan<-balance
					return
				}
			}
		}
	}()
	newB := <- balanceChan
	return newB, nil
}

func GetTestSetup(id string, preimage lightning.Preimage, communicator *TestCommunicator) (*TestPeer, error) {
	ctx := context.Background()

	walletCli := gelements.NewElements("admin1", "123")
	err := walletCli.StartUp("http://localhost", LiquidPort)
	if err != nil {
		return nil, err
	}

	wallet, err := wallet2.NewRpcWallet(walletCli, id)
	if err != nil {
		return nil, err
	}

	clightning := &TestLightningClient{
		NodeId:   id,
		Payreq:   "gude",
		PreImage: preimage,
		Value:    100,
	}
	swapStore := swap.NewInMemStore()

	swapService := swap.NewService(ctx, swapStore, wallet, communicator, walletCli, clightning, &network.Regtest)

	messageHandler := swap.NewMessageHandler(communicator, swapService)

	err = messageHandler.Start()
	if err != nil {
		return nil, err
	}

	return &TestPeer{
		swap:       swapService,
		blockchain: walletCli,
		wallet:     wallet,
		ln:         clightning,
		mh:         messageHandler,
	}, nil
}

func (s *TestPeer) Start(t *testing.T) {
	go func() {
		err := s.swap.StartWatchingTxs()
		if err != nil {
			t.Fatal(err)
		}
	}()
}

type TestPeer struct {
	swap       *swap.Service
	blockchain blockchain.Blockchain
	wallet     wallet2.Wallet
	ln         *TestLightningClient
	mh         *swap.MessageHandler
}

type TestLightningClient struct {
	Payreq   string
	PreImage lightning.Preimage
	Value    uint64
	NodeId   string
}

func (t *TestLightningClient) GetNodeId() string {
	return t.NodeId
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
	Id      string
}

func (t *TestCommunicator) SendMessage(peerId string, message lightning.PeerMessage) error {
	msg, err := json.Marshal(message)
	if err != nil {
		t.testing.Fatal(err)
	}
	err = t.other.F(t.Id, message.MessageType(), hex.EncodeToString(msg))
	if err != nil {
		t.testing.Fatal(err)
	}
	return nil
}

func (t *TestCommunicator) AddMessageHandler(f func(peerId string, messageType string, payload string) error) error {
	t.F = f
	return nil
}
