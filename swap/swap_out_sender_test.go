package swap

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/sputn1ck/peerswap/lightning"
	"github.com/sputn1ck/peerswap/messages"
	"github.com/stretchr/testify/assert"
)

func Test_SwapMarshalling(t *testing.T) {
	swap := newSwapOutSenderFSM(&SwapServices{}, "alice", "bob")

	swap.Data = &SwapData{
		Id: NewSwapId(),
	}

	swapBytes, err := json.Marshal(swap)
	if err != nil {
		t.Fatal(err)
	}

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
	takerpubkeyhash := "abcdef"
	chanId := "baz"
	FeeInvoice := "feeinv"
	msgChan := make(chan PeerMessage)

	timeOutD := &timeOutDummy{}

	swapServices := getSwapServices(msgChan)
	swapServices.toService = timeOutD
	swapFSM := newSwapOutSenderFSM(swapServices, initiator, peer)

	_, err := swapFSM.SendEvent(Event_OnSwapOutStarted, &SwapOutRequestMessage{
		Amount:          swapAmount,
		Scid:            chanId,
		SwapId:          swapFSM.SwapId,
		Pubkey:          takerpubkeyhash,
		Network:         "mainnet",
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Check if timeout was set
	assert.Equal(t, 1, timeOutD.getCalled())

	assert.Equal(t, initiator, swapFSM.Data.InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.SwapOutRequest.Pubkey)

	_, err = swapFSM.SendEvent(Event_OnFeeInvoiceReceived, &SwapOutAgreementMessage{Payreq: FeeInvoice})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutSender_AwaitTxBroadcastedMessage, swapFSM.Data.GetCurrentState())
	_, err = swapFSM.SendEvent(Event_OnTxOpenedMessage, &OpeningTxBroadcastedMessage{
		Payreq:    "claiminv",
		TxId:      "txid",
		ScriptOut: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutSender_AwaitTxConfirmation, swapFSM.Data.GetCurrentState())

	_, err = swapFSM.SendEvent(Event_OnTxConfirmed, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "txid", swapFSM.Data.OpeningTxBroadcasted.TxId)

	assert.Equal(t, State_ClaimedPreimage, swapFSM.Data.GetCurrentState())
}
func Test_Cancel2(t *testing.T) {
	swapAmount := uint64(100)
	initiator := "foo"
	peer := "bar"
	takerpubkeyhash := "abcdef"
	chanId := "baz"
	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)
	swapFSM := newSwapOutSenderFSM(swapServices, initiator, peer)

	_, err := swapFSM.SendEvent(Event_OnSwapOutStarted, &SwapOutRequestMessage{
		Amount:          swapAmount,
		Scid:            chanId,
		SwapId:          swapFSM.SwapId,
		Pubkey:          takerpubkeyhash,
		Network:         "mainnet",
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, messages.MESSAGETYPE_SWAPOUTREQUEST, msg.MessageType())
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
	takerpubkeyhash := "abcdef"
	FeeInvoice := "err"
	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)
	swapFSM := newSwapOutSenderFSM(swapServices, initiator, peer)

	_, err := swapFSM.SendEvent(Event_OnSwapOutStarted, &SwapOutRequestMessage{
		Amount:          swapAmount,
		Scid:            chanId,
		SwapId:          swapFSM.SwapId,
		Pubkey:          takerpubkeyhash,
		Network:         "mainnet",
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	assert.Equal(t, messages.MESSAGETYPE_SWAPOUTREQUEST, msg.MessageType())
	_, err = swapFSM.SendEvent(Event_OnFeeInvoiceReceived, &SwapOutAgreementMessage{Payreq: FeeInvoice})
	if err != nil {
		t.Fatal(err)
	}
	msg = <-msgChan
	assert.Equal(t, messages.MESSAGETYPE_CANCELED, msg.MessageType())
	assert.Equal(t, State_SwapCanceled, swapFSM.Data.GetCurrentState())
}
func Test_AbortCsvClaim(t *testing.T) {
	swapAmount := uint64(100)
	initiator := "foo"
	peer := "bar"
	chanId := "baz"
	FeeInvoice := "feeinv"
	takerpubkeyhash := "abcdef"
	msgChan := make(chan PeerMessage)

	swapServices := getSwapServices(msgChan)

	swapFSM := newSwapOutSenderFSM(swapServices, initiator, peer)

	_, err := swapFSM.SendEvent(Event_OnSwapOutStarted, &SwapOutRequestMessage{
		Amount:          swapAmount,
		Scid:            chanId,
		SwapId:          swapFSM.SwapId,
		Pubkey:          takerpubkeyhash,
		Network:         "mainnet",
		ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
	})
	if err != nil {
		t.Fatal(err)
	}
	<-msgChan
	assert.Equal(t, initiator, swapFSM.Data.InitiatorNodeId)
	assert.NotEqual(t, "", swapFSM.Data.SwapOutRequest.Pubkey)

	_, err = swapFSM.SendEvent(Event_OnFeeInvoiceReceived, &SwapOutAgreementMessage{Payreq: FeeInvoice})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutSender_AwaitTxBroadcastedMessage, swapFSM.Data.GetCurrentState())
	_, err = swapFSM.SendEvent(Event_OnTxOpenedMessage, &OpeningTxBroadcastedMessage{
		Payreq: "claiminv",
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, State_SwapOutSender_AwaitTxConfirmation, swapFSM.Data.GetCurrentState())

	swapFSM.Data.OpeningTxBroadcasted.Payreq = "err"
	_, err = swapFSM.SendEvent(Event_OnTxConfirmed, nil)
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgChan
	// wants to await the csv claim before it goes to a
	// finish state, such that the channel is still
	// locked for furhter peerswap requests.
	assert.Equal(t, State_ClaimedCoop, swapFSM.Data.GetCurrentState())
	assert.Equal(t, messages.MESSAGETYPE_COOPCLOSE, msg.MessageType())
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

func (d *dummyMessenger) AddMessageHandler(f func(peerId string, msgType string, payload []byte) error) {
}

func (d *dummyMessenger) SendMessage(peerId string, msg []byte, msgType int) error {
	if d.msgChan != nil {
		go func() { d.msgChan <- DummyMessageType(msgType) }()
	}
	return nil
}

type DummyMessageType messages.MessageType

func (d DummyMessageType) MessageType() messages.MessageType {
	return messages.MessageType(d)
}

type dummyLightningClient struct {
	preimage        string
	paymentCallback func(swapId string, invoiceType InvoiceType)
	failpayment     bool
}

func (d *dummyLightningClient) AddPaymentNotifier(swapId string, payreq string, invoiceType InvoiceType) (alreadyPaid bool) {
	return false
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

func (d *dummyLightningClient) TriggerPayment(swapId string, invoiceType InvoiceType) {
	d.paymentCallback(swapId, invoiceType)
}

func (d *dummyLightningClient) AddPaymentCallback(f func(string, InvoiceType)) {
	d.paymentCallback = f
}

//todo implement
func (d *dummyLightningClient) GetPayreq(msatAmount uint64, preimage string, swapId string, invoiceType InvoiceType, expiry uint64) (string, error) {
	if d.preimage == "err" {
		return "", errors.New("err")
	}
	return "", nil
}

func (d *dummyLightningClient) DecodePayreq(payreq string) (string, uint64, error) {
	if payreq == "err" {
		return "", 0, errors.New("error decoding")
	}
	return "foo", 100 * 1000, nil
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

func (d *dummyPolicy) GetReserveOnchainMsat() uint64 {
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

type dummyChain struct {
	txConfirmedFunc func(swapId string, txHex string) error
	csvPassedFunc   func(swapId string) error

	calledGetCSVHeight int64
	returnGetCSVHeight uint32
}

func (d *dummyChain) GetCSVHeight() uint32 {
	d.calledGetCSVHeight++
	return d.returnGetCSVHeight
}

func (d *dummyChain) EstimateTxFee(txSize uint64) (uint64, error) {
	return 100, nil
}

func (d *dummyChain) GetOutputScript(params *OpeningParams) ([]byte, error) {
	return []byte{}, nil
}
func (cl *dummyChain) GetAsset() string {
	return "a420"
}

func (cl *dummyChain) GetNetwork() string {
	return "mainnet"
}

func (d *dummyChain) TxIdFromHex(txHex string) (string, error) {
	return "txid", nil
}

func (d *dummyChain) CreatePreimageSpendingTransaction(swapParams *OpeningParams, claimParams *ClaimParams) (string, string, error) {
	return "txid", "txhex", nil
}

func (d *dummyChain) CreateCsvSpendingTransaction(swapParams *OpeningParams, claimParams *ClaimParams) (txId, txHex string, error error) {
	return "txid", "txhex", nil
}

func (d *dummyChain) CreateCoopSpendingTransaction(swapParams *OpeningParams, claimParams *ClaimParams, takerSigner Signer) (txId, txHex string, error error) {
	return "txid", "txhex", nil
}

func (d *dummyChain) AddWaitForConfirmationTx(swapId, txId string, vout, startingHeight uint32, wantscript []byte) {

}

func (d *dummyChain) AddWaitForCsvTx(swapId, txId string, vout uint32, startingHeight uint32, wantscript []byte) {

}

func (d *dummyChain) GetBlockHeight() (uint32, error) {
	return 1, nil
}

func (d *dummyChain) GetRefundFee() (uint64, error) {
	return 100, nil
}

func (d *dummyChain) CreateOpeningTransaction(swapParams *OpeningParams) (unpreparedTxHex string, fee uint64, vout uint32, err error) {
	return "txhex", 0, 0, nil
}

func (d *dummyChain) AddCsvCallback(f func(swapId string) error) {
	d.csvPassedFunc = f
}

func (d *dummyChain) NewAddress() (string, error) {
	return "addr", nil
}

func (d *dummyChain) BroadcastOpeningTx(unpreparedTxHex string) (txId, txHex string, error error) {
	return "txid", "txhex", nil
}

func (d *dummyChain) AddConfirmationCallback(f func(swapId string, txHex string) error) {
	d.txConfirmedFunc = f
}

func (d *dummyChain) ValidateTx(swapParams *OpeningParams, openingTxId string) (bool, error) {
	return true, nil
}
