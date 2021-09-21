package swap

import (
	"encoding/json"
	"errors"
	"log"
	"testing"

	"github.com/sputn1ck/glightning/glightning"
	"github.com/sputn1ck/peerswap/lightning"
	"github.com/stretchr/testify/assert"
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

	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)

	swapFSM := newSwapOutSenderFSM(swapServices)

	_, err := swapFSM.SendEvent(Event_OnSwapOutStarted, &SwapCreationContext{
		amount:      swapAmount,
		initiatorId: initiator,
		peer:        peer,
		channelId:   chanId,
		swapId:      swapFSM.Id,
		asset:       "btc",
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, initiator, swapFSM.Data.InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.TakerPubkeyHash)

	_, err = swapFSM.SendEvent(Event_OnFeeInvoiceReceived, &FeeMessage{Invoice: FeeInvoice})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutSender_AwaitTxBroadcastedMessage, swapFSM.Data.GetCurrentState())
	_, err = swapFSM.SendEvent(Event_OnTxOpenedMessage, &TxOpenedMessage{
		MakerPubkeyHash: "maker",
		Invoice:         "claiminv",
		TxId:            "txid",
		Cltv:            1,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutSender_AwaitTxConfirmation, swapFSM.Data.GetCurrentState())

	_, err = swapFSM.SendEvent(Event_OnTxConfirmed, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "txid", swapFSM.Data.OpeningTxId)
	assert.Equal(t, State_ClaimedPreimage, swapFSM.Data.GetCurrentState())
}
func Test_Cancel2(t *testing.T) {
	swapAmount := uint64(100)
	initiator := "foo"
	peer := "bar"
	chanId := "baz"
	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)
	swapFSM := newSwapOutSenderFSM(swapServices)

	_, err := swapFSM.SendEvent(Event_OnSwapOutStarted, &SwapCreationContext{
		amount:      swapAmount,
		initiatorId: initiator,
		peer:        peer,
		channelId:   chanId,
		swapId:      swapFSM.Id,
		asset:       "btc",
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, MESSAGETYPE_SWAPOUTREQUEST, msg.MessageType())
	_, err = swapFSM.SendEvent(Event_OnCancelReceived, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapCanceled, swapFSM.Data.GetCurrentState())
}
func Test_Cancel1(t *testing.T) {
	swapAmount := uint64(100)
	initiator := "foo"
	peer := "bar"
	chanId := "baz"
	FeeInvoice := "err"
	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)
	swapFSM := newSwapOutSenderFSM(swapServices)

	_, err := swapFSM.SendEvent(Event_OnSwapOutStarted, &SwapCreationContext{
		amount:      swapAmount,
		initiatorId: initiator,
		peer:        peer,
		channelId:   chanId,
		swapId:      swapFSM.Id,
		asset:       "btc",
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, MESSAGETYPE_SWAPOUTREQUEST, msg.MessageType())
	_, err = swapFSM.SendEvent(Event_OnFeeInvoiceReceived, &FeeMessage{Invoice: FeeInvoice})
	if err != nil {
		t.Fatal(err)
	}
	msg = <-msgChan
	assert.Equal(t, MESSAGETYPE_CANCELED, msg.MessageType())
	assert.Equal(t, State_SwapCanceled, swapFSM.Data.GetCurrentState())
}
func Test_AbortCltvClaim(t *testing.T) {
	swapAmount := uint64(100)
	initiator := "foo"
	peer := "bar"
	chanId := "baz"
	FeeInvoice := "feeinv"
	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)

	swapFSM := newSwapOutSenderFSM(swapServices)

	_, err := swapFSM.SendEvent(Event_OnSwapOutStarted, &SwapCreationContext{
		amount:      swapAmount,
		initiatorId: initiator,
		peer:        peer,
		channelId:   chanId,
		swapId:      swapFSM.Id,
		asset:       "btc",
	})
	if err != nil {
		t.Fatal(err)
	}
	<-msgChan
	assert.Equal(t, initiator, swapFSM.Data.InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.TakerPubkeyHash)

	_, err = swapFSM.SendEvent(Event_OnFeeInvoiceReceived, &FeeMessage{Invoice: FeeInvoice})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutSender_AwaitTxBroadcastedMessage, swapFSM.Data.GetCurrentState())
	_, err = swapFSM.SendEvent(Event_OnTxOpenedMessage, &TxOpenedMessage{
		MakerPubkeyHash: "maker",
		Invoice:         "claiminv",
		TxId:            "txid",
		Cltv:            1,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutSender_AwaitTxConfirmation, swapFSM.Data.GetCurrentState())

	swapFSM.Data.ClaimInvoice = "err"
	_, err = swapFSM.SendEvent(Event_OnTxConfirmed, nil)
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	// wants to await the cltv claim before it goes to a
	// finish state, such that the channel is still
	// locked for furhter peerswap requests.
	assert.Equal(t, State_SwapCanceled, swapFSM.Data.GetCurrentState())
	assert.Equal(t, MESSAGETYPE_CANCELED, msg.MessageType())
}

type dummyStore struct {
	dataMap map[string]*SwapStateMachine
}

func (d *dummyStore) ListAll() ([]*SwapStateMachine, error) {
	panic("implement me")
}

func (d *dummyStore) ListAllByPeer(peer string) ([]*SwapStateMachine, error) {
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

func (d *dummyPolicy) GetTankReserve() uint64 {
	return 1
}

func (d *dummyPolicy) IsPeerAllowed(peer string) bool {
	return true
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

type dummyChain struct {
	txConfirmedFunc func(swapId string) error
	cltvPassedFunc  func(swapId string) error
}

func (d *dummyChain) CreateOpeningTransaction(swapParams *OpeningParams) (unpreparedTxHex string, txid string, fee uint64, cltv int64, vout uint32, err error) {
	return "txhex", "", 0, 0, 0, nil
}

func (d *dummyChain) BroadcastOpeningTx(unpreparedTxHex string) (txId, txHex string, error error) {
	return "txid", "txhex", nil
}

func (d *dummyChain) CreatePreimageSpendingTransaction(swapParams *OpeningParams, claimParams *ClaimParams, openingTxId string) (txId, txHex string, error error) {
	return "txid", "txhex", nil
}

func (d *dummyChain) CreateCltvSpendingTransaction(swapParams *OpeningParams, claimParams *ClaimParams, openingTxHex string, vout uint32) (txId, txHex string, error error) {
	return "txid", "txhex", nil
}

func (d *dummyChain) AddWaitForConfirmationTx(swapId, txId string) (err error) {
	return nil
}

func (d *dummyChain) AddWaitForCltvTx(swapId, txId string, blockheight uint64) (err error) {
	return nil
}

func (d *dummyChain) AddConfirmationCallback(f func(swapId string) error) {
	d.txConfirmedFunc = f
}

func (d *dummyChain) AddCltvCallback(f func(swapId string) error) {
	d.cltvPassedFunc = f
}

func (d *dummyChain) ValidateTx(swapParams *OpeningParams, cltv int64, openingTxId string) (bool, error) {
	return true, nil
}
