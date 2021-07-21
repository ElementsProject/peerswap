package swap

import (
	"encoding/json"
	"errors"
	"github.com/sputn1ck/glightning/glightning"
	"github.com/sputn1ck/peerswap/lightning"
	"github.com/stretchr/testify/assert"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/transaction"
	"log"
	"testing"
)

func Test_SwapMarshalling(t *testing.T) {
	swap := newSwapOutSenderFSM(&SwapServices{})

	swap.Data = &SwapData{
		Id: "gude",
	}

	swapBytes, err := json.Marshal(swap)
	if err != nil {
		t.Fatal(err)
	}

	log.Printf("%s", string(swapBytes))
	var sm *SwapStateMachine

	err = json.Unmarshal(swapBytes, &sm)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, swap.Data.GetId(), sm.Data.Id)
}
func Test_ValidSwap(t *testing.T) {
	swapAmount := uint64(100)
	initiator := "ab123"
	peer := "ba123"
	chanId := "baz"
	FeeInvoice := "feeinv"
	FeePreimage := "preimage"

	store := &dummyStore{dataMap: map[string]*SwapStateMachine{}}
	messenger := &dummyMessenger{}
	lc := &dummyLightningClient{preimage: FeePreimage}
	policy := &dummyPolicy{}
	txWatcher := &DummyTxWatcher{}
	node := &DummyNode{}
	wallet := &DummyWallet{}
	utils := &DummyUtility{}

	swapServices := NewSwapServices(
		store,
		node,
		lc,
		messenger,
		policy,
		txWatcher,
		wallet,
		utils,
	)

	swapFSM := newSwapOutSenderFSM(swapServices)

	err := swapFSM.SendEvent(Event_SwapOutSender_OnSwapOutRequested, &SwapCreationContext{
		amount:      swapAmount,
		initiatorId: initiator,
		peer:        peer,
		channelId:   chanId,
		swapId:      swapFSM.Id,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, initiator, swapFSM.Data.InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.TakerPubkeyHash)

	err = swapFSM.SendEvent(Event_SwapOutSender_OnFeeInvReceived, &FeeMessage{Invoice: FeeInvoice})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutSender_FeeInvoicePaid, swapFSM.Data.GetCurrentState())
	err = swapFSM.SendEvent(Event_OnTxOpenedMessage, &TxOpenedMessage{
		MakerPubkeyHash: "maker",
		Invoice:         "claiminv",
		TxId:            "txid",
		TxHex:           "txhex",
		Cltv:            1,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutSender_TxBroadcasted, swapFSM.Data.GetCurrentState())
	assert.Equal(t, "txhex", swapFSM.Data.OpeningTxHex)

	err = swapFSM.SendEvent(Event_OnTxConfirmed, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, State_ClaimedPreimage, swapFSM.Data.GetCurrentState())
}
func Test_Cancel2(t *testing.T) {
	swapAmount := uint64(100)
	initiator := "foo"
	peer := "bar"
	chanId := "baz"
	FeePreimage := "preimage"
	msgChan := make(chan PeerMessage)
	store := &dummyStore{dataMap: map[string]*SwapStateMachine{}}
	messenger := &dummyMessenger{
		msgChan: msgChan,
	}
	lc := &dummyLightningClient{preimage: FeePreimage}
	policy := &dummyPolicy{}
	txWatcher := &DummyTxWatcher{}
	node := &DummyNode{}
	wallet := &DummyWallet{}
	utils := &DummyUtility{}

	swapServices := NewSwapServices(
		store,
		node,
		lc,
		messenger,
		policy,
		txWatcher,
		wallet,
		utils,
	)

	swapFSM := newSwapOutSenderFSM(swapServices)

	err := swapFSM.SendEvent(Event_SwapOutSender_OnSwapOutRequested, &SwapCreationContext{
		amount:      swapAmount,
		initiatorId: initiator,
		peer:        peer,
		channelId:   chanId,
		swapId:      swapFSM.Id,
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, MESSAGETYPE_SWAPOUTREQUEST, msg.MessageType())
	err = swapFSM.SendEvent(Event_OnCancelReceived, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOut_Canceled, swapFSM.Data.GetCurrentState())
}
func Test_Cancel1(t *testing.T) {
	swapAmount := uint64(100)
	initiator := "foo"
	peer := "bar"
	chanId := "baz"
	FeePreimage := "preimage"
	FeeInvoice := "err"
	msgChan := make(chan PeerMessage)

	store := &dummyStore{dataMap: map[string]*SwapStateMachine{}}
	messenger := &dummyMessenger{
		msgChan: msgChan,
	}
	lc := &dummyLightningClient{preimage: FeePreimage}
	policy := &dummyPolicy{}
	txWatcher := &DummyTxWatcher{}
	node := &DummyNode{}
	wallet := &DummyWallet{}
	utils := &DummyUtility{}

	swapServices := NewSwapServices(
		store,
		node,
		lc,
		messenger,
		policy,
		txWatcher,
		wallet,
		utils,
	)

	swapFSM := newSwapOutSenderFSM(swapServices)

	err := swapFSM.SendEvent(Event_SwapOutSender_OnSwapOutRequested, &SwapCreationContext{
		amount:      swapAmount,
		initiatorId: initiator,
		peer:        peer,
		channelId:   chanId,
		swapId:      swapFSM.Id,
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, MESSAGETYPE_SWAPOUTREQUEST, msg.MessageType())
	err = swapFSM.SendEvent(Event_SwapOutSender_OnFeeInvReceived, &FeeMessage{Invoice: FeeInvoice})
	if err != nil {
		t.Fatal(err)
	}
	msg = <-msgChan
	assert.Equal(t, MESSAGETYPE_CANCELED, msg.MessageType())
	assert.Equal(t, State_SwapOut_Canceled, swapFSM.Data.GetCurrentState())
}
func Test_AbortCltvClaim(t *testing.T) {
	swapAmount := uint64(100)
	initiator := "foo"
	peer := "bar"
	chanId := "baz"
	FeeInvoice := "feeinv"
	FeePreimage := "preimage"
	msgChan := make(chan PeerMessage)

	store := &dummyStore{dataMap: map[string]*SwapStateMachine{}}
	messenger := &dummyMessenger{msgChan}
	lc := &dummyLightningClient{preimage: FeePreimage}
	policy := &dummyPolicy{}
	txWatcher := &DummyTxWatcher{}
	node := &DummyNode{}
	wallet := &DummyWallet{}
	utils := &DummyUtility{}

	swapServices := NewSwapServices(
		store,
		node,
		lc,
		messenger,
		policy,
		txWatcher,
		wallet,
		utils,
	)

	swapFSM := newSwapOutSenderFSM(swapServices)

	err := swapFSM.SendEvent(Event_SwapOutSender_OnSwapOutRequested, &SwapCreationContext{
		amount:      swapAmount,
		initiatorId: initiator,
		peer:        peer,
		channelId:   chanId,
		swapId:      swapFSM.Id,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = <-msgChan
	assert.Equal(t, initiator, swapFSM.Data.InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.TakerPubkeyHash)

	err = swapFSM.SendEvent(Event_SwapOutSender_OnFeeInvReceived, &FeeMessage{Invoice: FeeInvoice})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutSender_FeeInvoicePaid, swapFSM.Data.GetCurrentState())
	err = swapFSM.SendEvent(Event_OnTxOpenedMessage, &TxOpenedMessage{
		MakerPubkeyHash: "maker",
		Invoice:         "claiminv",
		TxId:            "txid",
		TxHex:           "txhex",
		Cltv:            1,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutSender_TxBroadcasted, swapFSM.Data.GetCurrentState())
	assert.Equal(t, "txhex", swapFSM.Data.OpeningTxHex)
	swapFSM.Data.ClaimInvoice = "err"
	err = swapFSM.SendEvent(Event_OnTxConfirmed, nil)
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, State_SwapOut_Canceled, swapFSM.Data.GetCurrentState())
	assert.Equal(t, MESSAGETYPE_CANCELED, msg.MessageType())
	err = swapFSM.SendEvent(Event_OnClaimedCltv, &ClaimedMessage{ClaimTxId: "tx"})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_ClaimedCltv, swapFSM.Data.GetCurrentState())
}

type dummyStore struct {
	dataMap map[string]*SwapStateMachine
}

func (d *dummyStore) ListAll() ([]*SwapStateMachine, error) {
	panic("implement me")
}

func (d *dummyStore) UpdateData(data *SwapStateMachine) error {
	d.dataMap[data.Id] = data
	return nil
}

func (d *dummyStore) GetData(id string) (*SwapStateMachine, error) {
	if _, ok := d.dataMap[id]; !ok {
		return nil, ErrDataNotAvailable
	}
	return d.dataMap[id], nil
}

type dummyMessenger struct {
	msgChan chan PeerMessage
}

func (d *dummyMessenger) AddMessageHandler(f func(peerId string, msgType string, payload string) error) {
	return
}

func (d *dummyMessenger) SendMessage(peerId string, msg PeerMessage) error {
	log.Printf("Dummy sending message %v to %s", msg, peerId)
	if d.msgChan != nil {
		go func() { d.msgChan <- msg }()
	}
	return nil
}

type dummyLightningClient struct {
	preimage        string
	paymentCallback func(*glightning.Payment)
	failpayment     bool
}

func (d *dummyLightningClient) RebalancePayment(payreq string, channel string) (preimage string, err error) {
	if d.failpayment {
		return "", errors.New("payment failed")
	}
	if payreq == "err" {
		return "", errors.New("error paying invoice")
	}
	pi, err := lightning.GetPreimage()
	if err != nil {
		return "", err
	}
	return pi.String(), nil
}

func (d *dummyLightningClient) TriggerPayment(payment *glightning.Payment) {
	d.paymentCallback(payment)
}

func (d *dummyLightningClient) AddPaymentCallback(f func(*glightning.Payment)) {
	d.paymentCallback = f
}

//todo implement
func (d *dummyLightningClient) GetPayreq(msatAmount uint64, preimage string, label string) (string, error) {
	if d.preimage == "err" {
		return "", errors.New("err")
	}
	return "", nil
}

func (d *dummyLightningClient) DecodePayreq(payreq string) (*lightning.Invoice, error) {
	if payreq == "err" {
		return nil, errors.New("error decoding")
	}
	return &lightning.Invoice{
		PHash:       "foo",
		Amount:      100,
		Description: "gude",
	}, nil
}

func (d *dummyLightningClient) CheckChannel(channelId string, amount uint64) error {
	return nil
}

func (d *dummyLightningClient) PayInvoice(payreq string) (preImage string, err error) {
	if d.failpayment {
		return "", errors.New("payment failed")
	}
	if payreq == "err" {
		return "", errors.New("error paying invoice")
	}
	pi, err := lightning.GetPreimage()
	if err != nil {
		return "", err
	}
	return pi.String(), nil
}

type dummyPolicy struct {
}

// todo implement
func (d *dummyPolicy) GetMakerFee(swapValue uint64, swapFee uint64) (uint64, error) {
	return 1, nil
}

func (d *dummyPolicy) ShouldPayFee(swapAmount, feeAmount uint64, peerId, channelId string) bool {
	return true
}

type DummyTxWatcher struct {
	txConfirmedFunc func(swapId string) error
	cltvPassedFunc  func(swapId string) error
}

func (d *DummyTxWatcher) AddCltvTx(swapId string, cltv int64) {

}

func (d *DummyTxWatcher) AddConfirmationsTx(swapId, txId string) {

}

func (d *DummyTxWatcher) AddTxConfirmedHandler(f func(swapId string) error) {
	d.txConfirmedFunc = f
}

func (d *DummyTxWatcher) AddCltvPassedHandler(f func(swapId string) error) {
	d.cltvPassedFunc = f
}

type DummyWallet struct{}

func (d *DummyWallet) GetAddress() (string, error) {
	return "gude", nil
}

func (d *DummyWallet) FinalizeTransaction(rawTx string) (txHex string, err error) {
	return "txHex", nil
}

func (d *DummyWallet) CreateFundedTransaction(preparedTx *transaction.Transaction) (rawTx string, fee uint64, err error) {
	return "rawtx", 100, nil
}

type DummyNode struct{}

func (d *DummyNode) GetLocktime() uint64 {
	return 100
}

func (d *DummyNode) FinalizeAndBroadcastFundedTransaction(rawTx string) (txId string, err error) {
	return "txid", nil
}

// todo implement
func (d *DummyNode) CreateOpeningTransaction(swap *SwapData) error {
	return nil
}

func (d *DummyNode) GetSwapScript(swap *SwapData) ([]byte, error) {
	return []byte("script"), nil
}

func (d *DummyNode) GetBlockHeight() (uint64, error) {
	return 1, nil
}

func (d *DummyNode) GetAddress() (string, error) {
	return "el1qqv7n66qd59mhurtcpnx7w9tjk5pzdrep65zx4q5mztfvgrgxyf73q5k5r50uyrpe2xmpyqs36apx47lzpp6ww6ve7ez6apta3", nil
}

func (d *DummyNode) GetFee(txHex string) uint64 {
	return 1000
}

func (d *DummyNode) GetAsset() []byte {
	return []byte("lbtc")
}

func (d *DummyNode) GetNetwork() *network.Network {
	return &network.Regtest
}

func (d *DummyNode) SendRawTx(txHex string) (string, error) {
	return "txid1", nil
}

type DummyUtility struct{}

func (d *DummyUtility) GetSwapScript(takerPubkeyHash, makerPubkeyHash, paymentHash string, cltv int64) ([]byte, error) {
	return []byte("redeemscript"), nil
}

func (d *DummyUtility) GetCltvWitness(signature, redeemScript []byte) [][]byte {
	return [][]byte{}
}

func (d *DummyUtility) GetPreimageWitness(signature, preimage, redeemScript []byte) [][]byte {
	return [][]byte{}
}

func (d *DummyUtility) CreateSpendingTransaction(openingTxHex string, swapAmount, feeAmount, currentBlock uint64, asset, redeemScript, outputScript []byte) (tx *transaction.Transaction, sigHash [32]byte, err error) {
	return &transaction.Transaction{Inputs: []*transaction.TxInput{&transaction.TxInput{}}}, [32]byte{0, 1, 2, 3, 4, 5}, nil
}

func (d *DummyUtility) CreateOpeningTransaction(redeemScript []byte, asset []byte, amount uint64) (*transaction.Transaction, error) {
	return &transaction.Transaction{}, nil
}

func (d *DummyUtility) VoutFromTxHex(txHex string, redeemScript []byte) (uint32, error) {
	return 0, nil
}

func (d *DummyUtility) Blech32ToScript(blech32Addr string, network *network.Network) ([]byte, error) {
	return []byte("12345"), nil
}
