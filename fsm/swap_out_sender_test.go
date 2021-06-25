package fsm

import (
	"errors"
	"github.com/sputn1ck/peerswap/lightning"
	"github.com/sputn1ck/peerswap/utils"
	"github.com/stretchr/testify/assert"
	"github.com/vulpemventures/go-elements/network"
	"log"
	"testing"
)

func Test_ValidSwap(t *testing.T) {
	swapAmount := uint64(100)
	initiator := "foo"
	peer := "bar"
	chanId := "baz"
	FeeInvoice := "feeinv"
	FeePreimage := "preimage"

	store := &dummyStore{dataMap: map[string]Data{}}
	msg := &dummyMessenger{}
	lc := &dummyLightningClient{preimage: FeePreimage}
	policy := &dummyPolicy{}
	txWatcher := &DummyTxWatcher{}
	node := &DummyNode{}

	serviceMap := map[string]interface{}{
		"messenger": msg,
		"lightning": lc,
		"policy":    policy,
		"txwatcher": txWatcher,
		"node":      node,
	}

	swapFSM := newSwapOutSenderFSM("", store, serviceMap)

	err := swapFSM.SendEvent(Event_SwapOutSender_OnSwapOutCreated, &SwapCreationContext{
		amount:      swapAmount,
		initiatorId: initiator,
		peer:        peer,
		channelId:   chanId,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, initiator, swapFSM.Data.(*Swap).InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.(*Swap).TakerPubkeyHash)

	err = swapFSM.SendEvent(Event_SwapOutSender_OnFeeInvReceived, &FeeRequestContext{FeeInvoice: FeeInvoice})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutSender_FeeInvoicePaid, swapFSM.Data.GetCurrentState())
	err = swapFSM.SendEvent(Event_SwapOutSender_OnTxOpenedMessage, &TxBroadcastedContext{
		MakerPubkeyHash: "maker",
		ClaimInvoice:    "claiminv",
		TxId:            "txid",
		TxHex:           "txhex",
		Cltv:            1,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutSender_TxBroadcasted, swapFSM.Data.GetCurrentState())
	assert.Equal(t, "txhex", swapFSM.Data.(*Swap).OpeningTxHex)

	err = swapFSM.SendEvent(Event_SwapOutSender_OnTxConfirmations, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, State_SwapOutSender_ClaimedPreimage, swapFSM.Data.GetCurrentState())
}
func Test_Cancel2(t *testing.T) {
	swapAmount := uint64(100)
	initiator := "foo"
	peer := "bar"
	chanId := "baz"
	FeePreimage := "preimage"
	msgChan := make(chan string)
	store := &dummyStore{dataMap: map[string]Data{}}
	messenger := &dummyMessenger{
		msgChan: msgChan,
	}
	lc := &dummyLightningClient{preimage: FeePreimage}
	policy := &dummyPolicy{}
	txWatcher := &DummyTxWatcher{}
	node := &DummyNode{}

	serviceMap := map[string]interface{}{
		"messenger": messenger,
		"lightning": lc,
		"policy":    policy,
		"txwatcher": txWatcher,
		"node":      node,
	}

	swapFSM := newSwapOutSenderFSM("", store, serviceMap)

	err := swapFSM.SendEvent(Event_SwapOutSender_OnSwapOutCreated, &SwapCreationContext{
		amount:      swapAmount,
		initiatorId: initiator,
		peer:        peer,
		channelId:   chanId,
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, "request", msg)
	err = swapFSM.SendEvent(Event_SwapOutSender_OnCancelMsgReceived, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutSender_Canceled, swapFSM.Data.GetCurrentState())
}
func Test_Cancel1(t *testing.T) {
	swapAmount := uint64(100)
	initiator := "foo"
	peer := "bar"
	chanId := "baz"
	FeePreimage := "preimage"
	FeeInvoice := "err"
	msgChan := make(chan string)

	store := &dummyStore{dataMap: map[string]Data{}}
	messenger := &dummyMessenger{
		msgChan: msgChan,
	}
	lc := &dummyLightningClient{preimage: FeePreimage}
	policy := &dummyPolicy{}
	txWatcher := &DummyTxWatcher{}
	node := &DummyNode{}

	serviceMap := map[string]interface{}{
		"messenger": messenger,
		"lightning": lc,
		"policy":    policy,
		"txwatcher": txWatcher,
		"node":      node,
	}

	swapFSM := newSwapOutSenderFSM("", store, serviceMap)

	err := swapFSM.SendEvent(Event_SwapOutSender_OnSwapOutCreated, &SwapCreationContext{
		amount:      swapAmount,
		initiatorId: initiator,
		peer:        peer,
		channelId:   chanId,
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, "request", msg)
	err = swapFSM.SendEvent(Event_SwapOutSender_OnFeeInvReceived, &FeeRequestContext{FeeInvoice: FeeInvoice})
	if err != nil {
		t.Fatal(err)
	}
	msg = <-msgChan
	assert.Equal(t, "cancel", msg)
	assert.Equal(t, State_SwapOutSender_Canceled, swapFSM.Data.GetCurrentState())
}
func Test_AbortCltvClaim(t *testing.T) {
	swapAmount := uint64(100)
	initiator := "foo"
	peer := "bar"
	chanId := "baz"
	FeeInvoice := "feeinv"
	FeePreimage := "preimage"
	msgChan := make(chan string)

	store := &dummyStore{dataMap: map[string]Data{}}
	messenger := &dummyMessenger{msgChan}
	lc := &dummyLightningClient{preimage: FeePreimage}
	policy := &dummyPolicy{}
	txWatcher := &DummyTxWatcher{}
	node := &DummyNode{}

	serviceMap := map[string]interface{}{
		"messenger": messenger,
		"lightning": lc,
		"policy":    policy,
		"txwatcher": txWatcher,
		"node":      node,
	}

	swapFSM := newSwapOutSenderFSM("", store, serviceMap)

	err := swapFSM.SendEvent(Event_SwapOutSender_OnSwapOutCreated, &SwapCreationContext{
		amount:      swapAmount,
		initiatorId: initiator,
		peer:        peer,
		channelId:   chanId,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = <-msgChan
	assert.Equal(t, initiator, swapFSM.Data.(*Swap).InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.(*Swap).TakerPubkeyHash)

	err = swapFSM.SendEvent(Event_SwapOutSender_OnFeeInvReceived, &FeeRequestContext{FeeInvoice: FeeInvoice})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutSender_FeeInvoicePaid, swapFSM.Data.GetCurrentState())
	err = swapFSM.SendEvent(Event_SwapOutSender_OnTxOpenedMessage, &TxBroadcastedContext{
		MakerPubkeyHash: "maker",
		ClaimInvoice:    "claiminv",
		TxId:            "txid",
		TxHex:           "txhex",
		Cltv:            1,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutSender_TxBroadcasted, swapFSM.Data.GetCurrentState())
	assert.Equal(t, "txhex", swapFSM.Data.(*Swap).OpeningTxHex)
	swapFSM.Data.(*Swap).Payreq = "err"
	err = swapFSM.SendEvent(Event_SwapOutSender_OnTxConfirmations, nil)
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, State_SwapOutSender_Aborted, swapFSM.Data.GetCurrentState())
	assert.Equal(t, "abort", msg)
	err = swapFSM.SendEvent(Event_SwapOutSender_OnCltvClaimMsgReceived, &ClaimedContext{TxId: "tx"})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutSender_ClaimedCltv, swapFSM.Data.GetCurrentState())
}

type dummyStore struct {
	dataMap map[string]Data
}

func (d *dummyStore) UpdateData(data Data) error {
	d.dataMap[data.GetId()] = data
	return nil
}

func (d *dummyStore) GetData(id string) (Data, error) {
	if _, ok := d.dataMap[id]; !ok {
		return nil, ErrDataNotAvailable
	}
	return d.dataMap[id], nil
}

type dummyMessenger struct {
	msgChan chan string
}

func (d *dummyMessenger) SendMessage(peerId string, hexMsg string) error {
	log.Printf("Dummy sending message %s to %s", hexMsg, peerId)
	if d.msgChan != nil {
		go func() { d.msgChan <- hexMsg }()
	}
	return nil
}

type dummyLightningClient struct {
	preimage string
}

//todo implement
func (d *dummyLightningClient) GetPayreq(msatAmount uint64, preimage string, label string) (string, error) {
	panic("implement me")
}

func (d *dummyLightningClient) DecodeInvoice(payreq string) (*lightning.Invoice, error) {
	if payreq == "err" {
		return nil, errors.New("error decoding")
	}
	return &lightning.Invoice{
		PHash:       "foo",
		Amount:      100,
		Description: "gude",
	}, nil
}

func (d *dummyLightningClient) CheckChannel(channelId string, amount uint64) (bool, error) {
	return true, nil
}

func (d *dummyLightningClient) PayInvoice(payreq string) (preImage string, err error) {
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
	panic("implement me")
}

func (d *dummyPolicy) ShouldPayFee(feeAmount uint64, peerId, channelId string) bool {
	return true
}

type DummyTxWatcher struct {
}

func (d *DummyTxWatcher) AddTx(swapId, txId, txHex string) {

}

type DummyNode struct{}

// todo implement
func (d *DummyNode) CreateOpeningTransaction(swap *Swap) error {
	panic("implement me")
}
// todo implement
func (d *DummyNode) CreatePreimageSpendingTransaction(params *utils.SpendingParams, preimage []byte) (string, error) {
	return "txhex", nil
}

func (d *DummyNode) GetSwapScript(swap *Swap) ([]byte, error) {
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
