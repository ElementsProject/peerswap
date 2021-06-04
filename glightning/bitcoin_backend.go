package glightning

// as of v0.8.1, lightningd supports 'pluggable' bitcoind backends,
// which allows users to write plugins that support any block
// backend, as long as it supports the provided API
//
// here, we provide a template for building one of these
// 'bitcoind backend' plugins, which must register
// a certain subset of methods
//
// see https://github.com/ElementsProject/lightning/pull/3488
// for all the implementation details, to date.
import (
	"github.com/sputn1ck/liquid-loop/jrpc2"
	"strings"
)

type BtcBackend_MethodName string

const (
	_GetUtxOut           BtcBackend_MethodName = "getutxout"
	_GetChainInfo        BtcBackend_MethodName = "getchaininfo"
	_GetFeeRate          BtcBackend_MethodName = "getfeerate"
	_SendRawTransaction  BtcBackend_MethodName = "sendrawtransaction"
	_GetRawBlockByHeight BtcBackend_MethodName = "getrawblockbyheight"
	_EstimateFees        BtcBackend_MethodName = "estimatefees"
)

type BitcoinBackend struct {
	getUtxOut           func(string, uint32) (string, string, error)
	getChainInfo        func() (*Btc_ChainInfo, error)
	getFeeRate          func(uint32, string) (uint64, error)
	sendRawTransaction  func(string) error
	getRawBlockByHeight func(uint32) (string, string, error)
	estimateFees        func() (*Btc_EstimatedFees, error)

	plugin *Plugin
}

func NewBitcoinBackend(p *Plugin) *BitcoinBackend {
	bb := new(BitcoinBackend)
	bb.plugin = p
	p.SetDynamic(false)
	return bb
}

func (bb *BitcoinBackend) RegisterGetUtxOut(fn func(string, uint32) (string, string, error)) {
	bb.getUtxOut = fn
	m := new(Method_GetUtxOut)
	m.bb = bb
	rpcM := NewRpcMethod(m, "bitcoin gettxout method")
	rpcM.Category = "bitcoin"
	bb.plugin.RegisterMethod(rpcM)
}

func (bb *BitcoinBackend) RegisterGetChainInfo(fn func() (*Btc_ChainInfo, error)) {
	bb.getChainInfo = fn
	m := new(Method_GetChainInfo)
	m.bb = bb
	rpcM := NewRpcMethod(m, "bitcoin getchaininfo method")
	rpcM.Category = "bitcoin"
	bb.plugin.RegisterMethod(rpcM)
}

func (bb *BitcoinBackend) RegisterGetFeeRate(fn func(uint32, string) (uint64, error)) {
	bb.getFeeRate = fn
	m := new(Method_GetFeeRate)
	m.bb = bb
	rpcM := NewRpcMethod(m, "bitcoin getfeerate method")
	rpcM.Category = "bitcoin"
	bb.plugin.RegisterMethod(rpcM)
}

func (bb *BitcoinBackend) RegisterEstimateFees(fn func() (*Btc_EstimatedFees, error)) {
	bb.estimateFees = fn
	m := new(Method_EstimateFees)
	m.bb = bb
	rpcM := NewRpcMethod(m, "bitcoin estimatefees method")
	rpcM.Category = "bitcoin"
	bb.plugin.RegisterMethod(rpcM)
}

func (bb *BitcoinBackend) RegisterSendRawTransaction(fn func(string) error) {
	bb.sendRawTransaction = fn
	m := new(Method_SendRawTransaction)
	m.bb = bb
	rpcM := NewRpcMethod(m, "bitcoin sendrawtransaction method")
	rpcM.Category = "bitcoin"
	bb.plugin.RegisterMethod(rpcM)
}

func (bb *BitcoinBackend) RegisterGetRawBlockByHeight(fn func(uint32) (string, string, error)) {
	bb.getRawBlockByHeight = fn
	m := new(Method_GetRawBlockByHeight)
	m.bb = bb
	rpcM := NewRpcMethod(m, "bitcoin getrawblockbyheight method")
	rpcM.Category = "bitcoin"
	bb.plugin.RegisterMethod(rpcM)
}

type Method_GetUtxOut struct {
	TxId string `json:"txid"`
	Vout uint32 `json:"vout"`

	bb *BitcoinBackend
}

type Btc_GetUtxOut struct {
	Amount *string `json:"amount"`
	Script *string `json:"script"`
}

func (m Method_GetUtxOut) Name() string {
	return string(_GetUtxOut)
}

func (m Method_GetUtxOut) New() interface{} {
	n := new(Method_GetUtxOut)
	n.bb = m.bb
	return n
}

func (m Method_GetUtxOut) Call() (jrpc2.Result, error) {
	amt, script, err := m.bb.getUtxOut(m.TxId, m.Vout)
	if err != nil {
		return nil, err
	}

	var _amt *string
	var _script *string
	// if things are empty, we return 'null'
	if amt == "" {
		_amt = nil
	} else {
		_amt = &amt
	}

	if script == "" {
		_script = nil
	} else {
		_script = &script
	}

	return &Btc_GetUtxOut{_amt, _script}, nil
}

type Method_GetChainInfo struct {
	bb *BitcoinBackend
}

type Btc_ChainInfo struct {
	Chain                string `json:"chain"`
	HeaderCount          uint32 `json:"headercount"`
	BlockCount           uint32 `json:"blockcount"`
	InitialBlockDownload bool   `json:"ibd"`
}

func (m Method_GetChainInfo) Name() string {
	return string(_GetChainInfo)
}

func (m Method_GetChainInfo) New() interface{} {
	n := new(Method_GetChainInfo)
	n.bb = m.bb
	return n
}

func (m Method_GetChainInfo) Call() (jrpc2.Result, error) {
	return m.bb.getChainInfo()
}

type Method_EstimateFees struct {
	bb *BitcoinBackend
}

type Btc_EstimatedFees struct {
	Opening         uint64 `json:"opening"`
	MutualClose     uint64 `json:"mutual_close"`
	UnilateralClose uint64 `json:"unilateral_close"`
	DelayedToUs     uint64 `json:"delayed_to_us"`
	HtlcResolution  uint64 `json:"htlc_resolution"`
	Penalty         uint64 `json:"penalty"`
	MinAcceptable   uint64 `json:"min_acceptable"`
	MaxAcceptable   uint64 `json:"max_acceptable"`
}

func (m Method_EstimateFees) Name() string {
	return string(_EstimateFees)
}

func (m Method_EstimateFees) New() interface{} {
	n := new(Method_EstimateFees)
	n.bb = m.bb
	return n
}

func (m Method_EstimateFees) Call() (jrpc2.Result, error) {
	fees, err := m.bb.estimateFees()
	if err != nil {
		return nil, err
	}
	return fees, nil
}

type Method_GetFeeRate struct {
	Blocks uint32 `json:"blocks"`
	// Will be either CONSERVATIVE or ECONOMICAL
	Mode string `json:"mode"`

	bb *BitcoinBackend
}

type Btc_GetFeeRate struct {
	// to be denominated in satoshi per kilo-vbyte
	FeeRate uint64 `json:"feerate"`
}

func (m Method_GetFeeRate) Name() string {
	return string(_GetFeeRate)
}

func (m Method_GetFeeRate) New() interface{} {
	n := new(Method_GetFeeRate)
	n.bb = m.bb
	return n
}

func (m Method_GetFeeRate) Call() (jrpc2.Result, error) {
	feerate, err := m.bb.getFeeRate(m.Blocks, m.Mode)
	if err != nil {
		return nil, err
	}
	return &Btc_GetFeeRate{feerate}, nil
}

type Method_SendRawTransaction struct {
	TxString string `json:"tx"`
	bb       *BitcoinBackend
}

type Btc_SendRawTransaction struct {
	Success bool   `json:"success"`
	Error   string `json:"errmsg"`
}

func (m Method_SendRawTransaction) Name() string {
	return string(_SendRawTransaction)
}

func (m Method_SendRawTransaction) New() interface{} {
	n := new(Method_SendRawTransaction)
	n.bb = m.bb
	return n
}

func (m Method_SendRawTransaction) Call() (jrpc2.Result, error) {
	err := m.bb.sendRawTransaction(m.TxString)
	if err != nil {
		// this one's a bit weird, because we swallow
		// the error 'into' the response
		return &Btc_SendRawTransaction{false, err.Error()}, nil
	}
	return &Btc_SendRawTransaction{true, ""}, nil
}

type Method_GetRawBlockByHeight struct {
	Height uint32 `json:"height"`

	bb *BitcoinBackend
}

type Btc_GetRawBlockByHeight struct {
	BlockHash *string `json:"blockhash"`
	Block     *string `json:"block"`
}

func (m Method_GetRawBlockByHeight) Name() string {
	return string(_GetRawBlockByHeight)
}

func (m Method_GetRawBlockByHeight) New() interface{} {
	n := new(Method_GetRawBlockByHeight)
	n.bb = m.bb
	return n
}

func (m Method_GetRawBlockByHeight) Call() (jrpc2.Result, error) {
	blockhash, block, err := m.bb.getRawBlockByHeight(m.Height)
	if err != nil {
		if strings.Contains(err.Error(), "Block height out of range") {
			return &Btc_GetRawBlockByHeight{}, nil
		}
		return nil, err
	}
	return &Btc_GetRawBlockByHeight{&blockhash, &block}, nil
}
