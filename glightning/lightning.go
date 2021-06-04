package glightning

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sputn1ck/liquid-loop/jrpc2"
	"log"
	"path/filepath"
)

// This file's the one that holds all the objects for the
// c-lightning RPC commands
type Lightning struct {
	client *jrpc2.Client
	isUp   bool
}

func NewLightning() *Lightning {
	ln := &Lightning{}
	ln.client = jrpc2.NewClient()
	return ln
}

func (l *Lightning) SetTimeout(secs uint) {
	l.client.SetTimeout(secs)
}

func (l *Lightning) StartUp(rpcfile, lightningDir string) {
	up := make(chan bool)
	go func(l *Lightning, rpcfile, lightningDir string, up chan bool) {
		err := l.client.SocketStart(filepath.Join(lightningDir, rpcfile), up)
		if err != nil {
			log.Fatal(err)
		}
	}(l, rpcfile, lightningDir, up)
	l.isUp = <-up
}

func (l *Lightning) Shutdown() {
	l.client.Shutdown()
}

func (l *Lightning) IsUp() bool {
	return l.isUp && l.client.IsUp()
}

func (l *Lightning) Request(m jrpc2.Method, resp interface{}) error {
	return l.client.Request(m, resp)
}

type ListConfigsRequest struct {
	Config string `json:"config,omitempty"`
}

func (r ListConfigsRequest) Name() string {
	return "listconfigs"
}

func (r *ListConfigsRequest) New() interface{} {
	return &ListConfigsRequest{}
}

func (l *Lightning) ListConfigs() (map[string]interface{}, error) {
	var result map[string]interface{}
	err := l.client.Request(&ListConfigsRequest{}, &result)
	return result, err
}

func (l *Lightning) GetConfig(config string) (interface{}, error) {
	var result map[string]interface{}
	err := l.client.Request(&ListConfigsRequest{config}, &result)
	return result[config], err
}

type ListPeersRequest struct {
	PeerId string `json:"id,omitempty"`
	Level  string `json:"level,omitempty"`
}

func (r ListPeersRequest) Name() string {
	return "listpeers"
}

type Peer struct {
	Id           string         `json:"id"`
	Connected    bool           `json:"connected"`
	NetAddresses []string       `json:"netaddr"`
	Features     *Hexed         `json:"features"`
	Channels     []*PeerChannel `json:"channels"`
	Logs         []*Log         `json:"log,omitempty"`
}

type PeerChannel struct {
	State                            string            `json:"state"`
	ScratchTxId                      string            `json:"scratch_txid"`
	Owner                            string            `json:"owner"`
	ShortChannelId                   string            `json:"short_channel_id"`
	ChannelDirection                 int               `json:"direction"`
	ChannelId                        string            `json:"channel_id"`
	FundingTxId                      string            `json:"funding_txid"`
	CloseToAddress                   string            `json:"close_to_addr,omitempty"`
	CloseToScript                    string            `json:"close_to,omitempty"`
	Status                           []string          `json:"status"`
	Private                          bool              `json:"private"`
	FundingAllocations               map[string]uint64 `json:"funding_allocation_msat"`
	FundingMsat                      map[string]string `json:"funding_msat"`
	MilliSatoshiToUs                 uint64            `json:"msatoshi_to_us"`
	ToUsMsat                         string            `json:"to_us_msat"`
	MilliSatoshiToUsMin              uint64            `json:"msatoshi_to_us_min"`
	MinToUsMsat                      string            `json:"min_to_us_msat"`
	MilliSatoshiToUsMax              uint64            `json:"msatoshi_to_us_max"`
	MaxToUsMsat                      string            `json:"max_to_us_msat"`
	MilliSatoshiTotal                uint64            `json:"msatoshi_total"`
	TotalMsat                        string            `json:"total_msat"`
	DustLimitSatoshi                 uint64            `json:"dust_limit_satoshis"`
	DustLimitMsat                    string            `json:"dust_limit_msat"`
	MaxHtlcValueInFlightMilliSatoshi uint64            `json:"max_htlc_value_in_flight_msat"`
	MaxHtlcValueInFlightMsat         string            `json:"max_total_htlc_in_msat"`
	TheirChannelReserveSatoshi       uint64            `json:"their_channel_reserve_satoshis"`
	TheirReserveMsat                 string            `json:"their_reserve_msat"`
	OurChannelReserveSatoshi         uint64            `json:"our_channel_reserve_satoshis"`
	OurReserveMsat                   string            `json:"our_reserve_msat"`
	SpendableMilliSatoshi            uint64            `json:"spendable_msatoshi"`
	SpendableMsat                    string            `json:"spendable_msat"`
	ReceivableMilliSatoshi           uint64            `json:"receivable_msatoshi"`
	ReceivableMsat                   string            `json:"receivable_msat"`
	HtlcMinMilliSatoshi              uint64            `json:"htlc_minimum_msat"`
	MinimumHtlcInMsat                string            `json:"minimum_htlc_in_msat"`
	TheirToSelfDelay                 uint              `json:"their_to_self_delay"`
	OurToSelfDelay                   uint              `json:"our_to_self_delay"`
	MaxAcceptedHtlcs                 uint              `json:"max_accepted_htlcs"`
	InPaymentsOffered                uint64            `json:"in_payments_offered"`
	InMilliSatoshiOffered            uint64            `json:"in_msatoshi_offered"`
	IncomingOfferedMsat              string            `json:"in_offered_msat"`
	InPaymentsFulfilled              uint64            `json:"in_payments_fulfilled"`
	InMilliSatoshiFulfilled          uint64            `json:"in_msatoshi_fulfilled"`
	IncomingFulfilledMsat            string            `json:"in_fulfilled_msat"`
	OutPaymentsOffered               uint64            `json:"out_payments_offered"`
	OutMilliSatoshiOffered           uint64            `json:"out_msatoshi_offered"`
	OutgoingOfferedMsat              string            `json:"out_offered_msat"`
	OutPaymentsFulfilled             uint64            `json:"out_payments_fulfilled"`
	OutMilliSatoshiFulfilled         uint64            `json:"out_msatoshi_fulfilled"`
	OutgoingFulfilledMsat            string            `json:"out_fulfilled_msat"`
	Htlcs                            []*Htlc           `json:"htlcs"`
}

type Htlc struct {
	Direction    string `json:"direction"`
	Id           uint64 `json:"id"`
	MilliSatoshi uint64 `json:"msatoshi"`
	AmountMsat   string `json:"amount_msat"`
	Expiry       uint64 `json:"expiry"`
	PaymentHash  string `json:"payment_hash"`
	State        string `json:"state"`
	LocalTrimmed bool   `json:"local_trimmed"`
}

// Show current peer {peerId}.
func (l *Lightning) GetPeer(peerId string) (*Peer, error) {
	return l.GetPeerWithLogs(peerId, None)
}

func (l *Lightning) GetPeerWithLogs(peerId string, level LogLevel) (*Peer, error) {
	peers, err := l.getPeers(peerId, level)
	if len(peers) == 0 {
		return nil, errors.New(fmt.Sprintf("Peer %s not found", peerId))
	}
	return peers[0], err
}

// Show current peers, if {level} is set, include logs.
func (l *Lightning) ListPeersWithLogs(level LogLevel) ([]*Peer, error) {
	return l.getPeers("", level)
}

// Show current peers
func (l *Lightning) ListPeers() ([]*Peer, error) {
	return l.getPeers("", None)
}

// Show current peer {peerId}. If {level} is set, include logs.
func (l *Lightning) getPeers(peerId string, level LogLevel) ([]*Peer, error) {
	var result struct {
		Peers []*Peer `json:"peers"`
	}

	request := &ListPeersRequest{
		PeerId: peerId,
	}
	if level != None {
		request.Level = level.String()
	}

	err := l.client.Request(request, &result)
	return result.Peers, err
}

type ListNodeRequest struct {
	NodeId string `json:"id,omitempty"`
}

func (ln ListNodeRequest) Name() string {
	return "listnodes"
}

type Node struct {
	Id            string    `json:"nodeid"`
	Alias         string    `json:"alias"`
	Color         string    `json:"color"`
	LastTimestamp uint      `json:"last_timestamp"`
	Features      *Hexed    `json:"features"`
	Addresses     []Address `json:"addresses"`
}

type Address struct {
	// todo: map to enum (ipv4, ipv6, torv2, torv3)
	Type string `json:"type"`
	Addr string `json:"address"`
	Port int    `json:"port"`
}

// Get all nodes in our local network view, filter on node {id},
// if provided
func (l *Lightning) GetNode(nodeId string) (*Node, error) {
	nodes, err := l.getNodes(nodeId)
	if len(nodes) == 0 || nodes[0] == nil {
		return nil, fmt.Errorf("Node %s not found", nodeId)
	}
	return nodes[0], err
}

// List all nodes in our local network view
func (l *Lightning) ListNodes() ([]*Node, error) {
	return l.getNodes("")
}

func (l *Lightning) getNodes(nodeId string) ([]*Node, error) {
	var result struct {
		Nodes []*Node `json:"nodes"`
	}
	err := l.client.Request(&ListNodeRequest{nodeId}, &result)
	return result.Nodes, err
}

type RouteRequest struct {
	PeerId        string   `json:"id"`
	MilliSatoshis uint64   `json:"msatoshi"`
	RiskFactor    float32  `json:"riskfactor"`
	Cltv          uint     `json:"cltv"`
	FromId        string   `json:"fromid,omitempty"`
	FuzzPercent   float32  `json:"fuzzpercent"`
	Seed          string   `json:"seed,omitempty"`
	Exclude       []string `json:"exclude,omitempty"`
	MaxHops       int32    `json:"maxhops,omitempty"`
}

type Route struct {
	Hops []RouteHop `json:"route"`
}

type RouteHop struct {
	Id             string `json:"id"`
	ShortChannelId string `json:"channel"`
	MilliSatoshi   uint64 `json:"msatoshi"`
	AmountMsat     string `json:"amount_msat,omitempty"`
	Delay          uint   `json:"delay"`
	Direction      uint8  `json:"direction,omitempty"`
}

func (rr RouteRequest) Name() string {
	return "getroute"
}

func (l *Lightning) GetRouteSimple(peerId string, msats uint64, riskfactor float32) ([]RouteHop, error) {
	return l.GetRoute(peerId, msats, riskfactor, 0, "", 0, nil, 0)
}

// Show route to {id} for {msatoshis}, using a {riskfactor} and optional
// {cltv} value (defaults to 9). If specified, search from {fromId} otherwise
// use current node as the source. Randomize the route with up to {fuzzpercent}
// (0.0 -> 100.0, default 5.0).
//
// If you wish to exclude a set of channels from the route, you can pass in an optional
// set of channel id's with a direction (scid/direction)
func (l *Lightning) GetRoute(peerId string, msats uint64, riskfactor float32, cltv uint, fromId string, fuzzpercent float32, exclude []string, maxHops int32) ([]RouteHop, error) {
	if peerId == "" {
		return nil, fmt.Errorf("Must provide a peerId to route to")
	}

	if msats == 0 {
		return nil, fmt.Errorf("No value set for payment. (`msatoshis` is equal to zero).")
	}

	if riskfactor <= 0 || riskfactor >= 100 {
		return nil, fmt.Errorf("The risk factor must set above 0 and beneath 100")
	}

	if fuzzpercent == 0 {
		fuzzpercent = 5.0
	} else if fuzzpercent < 0 || fuzzpercent > 100 {
		return nil, fmt.Errorf("The `fuzzpercent` value must be between 0 and 100")
	}

	if cltv == 0 {
		cltv = 9
	}

	var result Route
	err := l.client.Request(&RouteRequest{
		PeerId:        peerId,
		MilliSatoshis: msats,
		RiskFactor:    riskfactor,
		Cltv:          cltv,
		FromId:        fromId,
		FuzzPercent:   fuzzpercent,
		Exclude:       exclude,
		MaxHops:       maxHops,
	}, &result)
	return result.Hops, err
}

type SendOnionMessageRequest struct {
	Hops []OnionMessageHop `json:"hops"`
}

type OnionMessageHop struct {
	Id string `json:"id"`
	RawTlv string `json:"rawtlv"`
}


func (r SendOnionMessageRequest) Name() string {
	return "sendonionmessage"
}

func (l *Lightning) SendOnionMessage(hops []OnionMessageHop) (*string, error) {
	var response string

	req := SendOnionMessageRequest{
		Hops:       hops,
	}

	err := l.client.Request(&req, &response)
	return &response, err
}


type SendOnionRequest struct {
	Onion         string   `json:"onion"`
	FirstHop      FirstHop `json:"first_hop"`
	PaymentHash   string   `json:"payment_hash"`
	Label         string   `json:"label,omitempty"`
	SharedSecrets []string `json:"shared_secrets,omitempty"`
	// For MPP payments!
	PartId uint64 `json:"partid,omitempty"`
}

type FirstHop struct {
	ShortChannelId string `json:"channel"`
	Direction      uint8  `json:"direction"`
	AmountMsat     string `json:"amount_msat"`
	Delay          uint   `json:"delay"`
}

func (r SendOnionRequest) Name() string {
	return "sendonion"
}

func (l *Lightning) SendOnion(onion string, hop FirstHop, paymentHash string) (*SendPayFields, error) {
	return l.SendOnionWithDetails(onion, hop, paymentHash, "", nil, nil)
}

func (l *Lightning) SendOnionWithDetails(onion string, hop FirstHop, paymentHash string, label string, secrets []string, partId *uint64) (*SendPayFields, error) {
	var response SendPayFields

	req := SendOnionRequest{
		Onion:       onion,
		FirstHop:    hop,
		PaymentHash: paymentHash,
	}

	if len(label) > 0 {
		req.Label = label
	}
	if secrets != nil {
		req.SharedSecrets = secrets
	}
	if partId != nil {
		req.PartId = *partId
	}

	err := l.client.Request(&req, &response)
	return &response, err
}

type CreateOnionRequest struct {
	Hops []Hop `json:"hops"`
	// Data onion should commit to, must match `payment_hash`
	AssociatedData string `json:"assocdata"`
	// Optional, can be used to generate shared secrets
	SessionKey string `json:"session_key,omitempty"`
}

func (r CreateOnionRequest) Name() string {
	return "createonion"
}

type Hop struct {
	Pubkey  string `json:"pubkey"`
	Payload string `json:"payload"`
}

type CreateOnionResponse struct {
	Onion         string   `json:"onion"`
	SharedSecrets []string `json:"shared_secrets"`
}

func (l *Lightning) CreateOnion(hops []Hop, paymentHash, sessionKey string) (*CreateOnionResponse, error) {
	var response CreateOnionResponse
	req := CreateOnionRequest{
		Hops:           hops,
		AssociatedData: paymentHash,
		SessionKey:     sessionKey,
	}

	err := l.client.Request(&req, &response)
	return &response, err
}

type ListChannelRequest struct {
	ShortChannelId string `json:"short_channel_id,omitempty"`
	Source         string `json:"source,omitempty"`
}

func (lc ListChannelRequest) Name() string {
	return "listchannels"
}

type Channel struct {
	Source                   string `json:"source"`
	Destination              string `json:"destination"`
	ShortChannelId           string `json:"short_channel_id"`
	IsPublic                 bool   `json:"public"`
	Satoshis                 uint64 `json:"satoshis"`
	AmountMsat               string `json:"amount_msat"`
	MessageFlags             uint   `json:"message_flags"`
	ChannelFlags             uint   `json:"channel_flags"`
	IsActive                 bool   `json:"active"`
	LastUpdate               uint   `json:"last_update"`
	BaseFeeMillisatoshi      uint64 `json:"base_fee_millisatoshi"`
	FeePerMillionth          uint64 `json:"fee_per_millionth"`
	Delay                    uint   `json:"delay"`
	HtlcMinimumMilliSatoshis string `json:"htlc_minimum_msat"`
	HtlcMaximumMilliSatoshis string `json:"htlc_maximum_msat"`
}

// Get channel by {shortChanId}
func (l *Lightning) GetChannel(shortChanId string) ([]*Channel, error) {
	var result struct {
		Channels []*Channel `json:"channels"`
	}
	err := l.client.Request(&ListChannelRequest{shortChanId, ""}, &result)
	if len(result.Channels) == 0 {
		return nil, errors.New(fmt.Sprintf("No channel found for short channel id %s", shortChanId))
	}
	return result.Channels, err
}

func (l *Lightning) ListChannelsBySource(nodeId string) ([]*Channel, error) {
	var result struct {
		Channels []*Channel `json:"channels"`
	}
	err := l.client.Request(&ListChannelRequest{"", nodeId}, &result)
	return result.Channels, err
}

func (l *Lightning) ListChannels() ([]*Channel, error) {
	return l.GetChannel("")
}

type InvoiceRequest struct {
	MilliSatoshis string   `json:"msatoshi"`
	Label         string   `json:"label"`
	Description   string   `json:"description"`
	ExpirySeconds uint32   `json:"expiry,omitempty"`
	Fallbacks     []string `json:"fallbacks,omitempty"`
	PreImage      string   `json:"preimage,omitempty"`
	// Note that these both have the same json key. we use checks
	// to make sure that only one of them is filled in
	ExposePrivChansFlag *bool    `json:"exposeprivatechannels,omitempty"`
	ExposeTheseChannels []string `json:"exposeprivatechannels,omitempty"`
}

func (ir InvoiceRequest) Name() string {
	return "invoice"
}

type Invoice struct {
	Label                   string `json:"label"`
	Bolt11                  string `json:"bolt11"`
	PaymentHash             string `json:"payment_hash"`
	AmountMilliSatoshi      string `json:"amount_msat,omitempty"`
	AmountMilliSatoshiRaw   uint64 `json:"msatoshi,omitempty"`
	Status                  string `json:"status"`
	PayIndex                uint64 `json:"pay_index,omitempty"`
	MilliSatoshiReceivedRaw uint64 `json:"msatoshi_received,omitempty"`
	MilliSatoshiReceived    string `json:"amount_received_msat,omitempty"`
	PaidAt                  uint64 `json:"paid_at,omitempty"`
	PaymentPreImage         string `json:"payment_preimage,omitempty"`
	WarningOffline          string `json:"warning_offline,omitempty"`
	WarningCapacity         string `json:"warning_capacity,omitempty"`
	Description             string `json:"description"`
	ExpiresAt               uint64 `json:"expires_at"`
}

// Creates an invoice with a value of "any", that can be paid with any amount
func (l *Lightning) CreateInvoiceAny(label, description string, expirySeconds uint32, fallbacks []string, preimage string, exposePrivateChans bool) (*Invoice, error) {
	return createInvoice(l, "any", label, description, expirySeconds, fallbacks, preimage, exposePrivateChans, nil)
}

// Creates an invoice with a value of `msat`. Label and description must be set.
//
// The 'label' is a unique string or number (which is treated as a string); it is
// never revealed to other nodes, but it can be used to query the status of this
// invoice.
//
// The 'description' is a short description of purpose of payment. It is encoded
// into the invoice. Must be UTF-8, cannot use '\n' JSON escape codes.
//
// The 'expiry' is optionally the number of seconds the invoice is valid for.
// Defaults to 3600 (1 hour).
//
// 'fallbacks' is one or more fallback addresses to include in the invoice. They
// should be ordered from most preferred to least. Noe that these are not
// currently tracked to fulfill the invoice.
//
// The 'preimage' is a 64-digit hex string to be used as payment preimage for
// the created invoice. By default, c-lightning will generate a secure
// pseudorandom preimage seeded from an appropriate entropy source on your
// system. **NOTE**: if you specify the 'preimage', you are responsible for
// both ensuring that a suitable psuedorandom generator with sufficient entropy
// was used in its creation and keeping it secret.
// This parameter is an advanced feature intended for use with cutting-edge
// cryptographic protocols and should not be used unless explicitly needed.
func (l *Lightning) CreateInvoice(msat uint64, label, description string, expirySeconds uint32, fallbacks []string, preimage string, willExposePrivateChans bool) (*Invoice, error) {

	if msat <= 0 {
		return nil, fmt.Errorf("No value set for invoice. (`msat` is less than or equal to zero).")
	}
	return createInvoice(l, fmt.Sprint(msat), label, description, expirySeconds, fallbacks, preimage, willExposePrivateChans, nil)
}

func (l *Lightning) CreateInvoiceExposing(msat uint64, label, description string, expirySeconds uint32, fallbacks []string, preimage string, exposePrivChans []string) (*Invoice, error) {
	if msat <= 0 {
		return nil, fmt.Errorf("No value set for invoice. (`msat` is less than or equal to zero).")
	}
	return createInvoice(l, fmt.Sprint(msat), label, description, expirySeconds, fallbacks, preimage, false, exposePrivChans)
}

func (l *Lightning) Invoice(msat uint64, label, description string) (*Invoice, error) {
	if msat <= 0 {
		return nil, fmt.Errorf("No value set for invoice. (`msat` is less than or equal to zero).")
	}
	return createInvoice(l, fmt.Sprint(msat), label, description, 0, nil, "", false, nil)
}

func createInvoice(l *Lightning, msat, label, description string, expirySeconds uint32, fallbacks []string, preimage string, flagExposePrivate bool, exposeShortChannelIds []string) (*Invoice, error) {

	if label == "" {
		return nil, fmt.Errorf("Must set a label on an invoice")
	}
	if description == "" {
		return nil, fmt.Errorf("Must set a description on an invoice")
	}

	if flagExposePrivate && exposeShortChannelIds != nil {
		return nil, fmt.Errorf("Cannot both flag to expose private and provide list of short channel ids")
	}

	var exposePrivFlag *bool
	if flagExposePrivate {
		exposePrivFlag = &flagExposePrivate
	} else if exposeShortChannelIds == nil || len(exposeShortChannelIds) == 0 {
		f := false
		exposePrivFlag = &f
	} else {
		exposePrivFlag = nil
	}

	var result Invoice
	err := l.client.Request(&InvoiceRequest{
		MilliSatoshis:       msat,
		Label:               label,
		Description:         description,
		ExpirySeconds:       expirySeconds,
		Fallbacks:           fallbacks,
		PreImage:            preimage,
		ExposePrivChansFlag: exposePrivFlag,
		ExposeTheseChannels: exposeShortChannelIds,
	}, &result)
	return &result, err
}

type ListInvoiceRequest struct {
	Label string `json:"label,omitempty"`
}

func (r ListInvoiceRequest) Name() string {
	return "listinvoices"
}

// List all invoices
func (l *Lightning) ListInvoices() ([]*Invoice, error) {
	return l.getInvoices("")
}

// Show invoice {label}.
func (l *Lightning) GetInvoice(label string) (*Invoice, error) {
	list, err := l.getInvoices(label)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, errors.New(fmt.Sprintf("Invoice %s not found", label))
	}
	return list[0], err
}

func (l *Lightning) getInvoices(label string) ([]*Invoice, error) {
	var result struct {
		List []*Invoice `json:"invoices"`
	}
	err := l.client.Request(&ListInvoiceRequest{label}, &result)
	return result.List, err
}

type DeleteInvoiceRequest struct {
	Label  string `json:"label"`
	Status string `json:"status"`
}

func (r DeleteInvoiceRequest) Name() string {
	return "delinvoice"
}

// Delete unpaid invoice {label} with {status}
func (l *Lightning) DeleteInvoice(label, status string) (*Invoice, error) {
	var result Invoice
	err := l.client.Request(&DeleteInvoiceRequest{label, status}, &result)
	return &result, err
}

type WaitAnyInvoiceRequest struct {
	LastPayIndex uint  `json:"lastpay_index,omitempty"`
	Timeout      *uint `json:"timeout,omitempty"`
}

func (r WaitAnyInvoiceRequest) Name() string {
	return "waitanyinvoice"
}

// Waits until an invoice is paid, then returns a single entry.
// Will not return or provide any invoices paid prior to or including
// the lastPayIndex.
//
// The 'pay index' is a monotonically-increasing number assigned to
// an invoice when it gets paid. The first valid 'pay index' is 1.
//
// This blocks until it receives a response.
func (l *Lightning) WaitAnyInvoice(lastPayIndex uint) (*Invoice, error) {
	var result Invoice
	req := &WaitAnyInvoiceRequest{
		LastPayIndex: lastPayIndex,
		Timeout:      nil,
	}
	err := l.client.RequestNoTimeout(req, &result)
	return &result, err
}

// Note that if timeout is zero, it won't be sent. which is fine, a zero timeout doesn't really
// make a lot of sense
func (l *Lightning) WaitAnyInvoiceTimeout(lastPayIndex uint, timeout uint) (*Invoice, error) {
	var result Invoice
	req := &WaitAnyInvoiceRequest{
		LastPayIndex: lastPayIndex,
		Timeout:      &timeout,
	}
	err := l.client.RequestNoTimeout(req, &result)
	return &result, err
}

type WaitInvoiceRequest struct {
	Label string `json:"label"`
}

func (r WaitInvoiceRequest) Name() string {
	return "waitinvoice"
}

// Wait for invoice to be filled or for invoice to expire.
// This blocks until a result is returned from the server and by
// passes client timeout safeguards.
func (l *Lightning) WaitInvoice(label string) (*Invoice, error) {
	if label == "" {
		return nil, fmt.Errorf("Must call wait invoice with a label")
	}

	var result Invoice
	err := l.client.RequestNoTimeout(&WaitInvoiceRequest{label}, &result)
	return &result, err
}

type DeleteExpiredInvoiceReq struct {
	MaxExpiryTime uint64 `json:"maxexpirytime,omitempty"`
}

func (r DeleteExpiredInvoiceReq) Name() string {
	return "delexpiredinvoice"
}

func (l *Lightning) DeleteExpiredInvoicesSince(unixTime uint64) error {
	var result interface{}
	return l.client.Request(&DeleteExpiredInvoiceReq{unixTime}, &result)
}

type AutoCleanInvoiceRequest struct {
	CycleSeconds     uint32 `json:"cycle_seconds"`
	ExpiredBySeconds uint32 `json:"expired_by,omitempty"`
}

type AutoCleanResult struct{}

func (r AutoCleanInvoiceRequest) Name() string {
	return "autocleaninvoice"
}

func (l *Lightning) DisableInvoiceAutoclean() error {
	return l.SetInvoiceAutoclean(0, 0)
}

// Perform cleanup every {cycle_seconds} (default 3600), or disable autoclean if 0.
// Clean up expired invoices that have expired for {expired_by} seconds (default 86400).
func (l *Lightning) SetInvoiceAutoclean(intervalSeconds, expiredBySeconds uint32) error {
	var result string
	err := l.client.Request(&AutoCleanInvoiceRequest{intervalSeconds, expiredBySeconds}, &result)
	return err
}

type DecodePayRequest struct {
	Bolt11      string `json:"bolt11"`
	Description string `json:"description,omitempty"`
}

func (r DecodePayRequest) Name() string {
	return "decodepay"
}

type DecodedBolt11 struct {
	Currency           string        `json:"currency"`
	CreatedAt          uint64        `json:"created_at"`
	Expiry             uint64        `json:"expiry"`
	Payee              string        `json:"payee"`
	MilliSatoshis      uint64        `json:"msatoshi"`
	AmountMsat         string        `json:"amount_msat"`
	Description        string        `json:"description"`
	DescriptionHash    string        `json:"description_hash"`
	MinFinalCltvExpiry int           `json:"min_final_cltv_expiry"`
	Fallbacks          []Fallback    `json:"fallbacks"`
	Routes             [][]BoltRoute `json:"routes"`
	Extra              []BoltExtra   `json:"extra"`
	PaymentHash        string        `json:"payment_hash"`
	Signature          string        `json:"signature"`
	Features           Hexed         `json:"features"`
}

type Fallback struct {
	// fixme: use enum (P2PKH,P2SH,P2WPKH,P2WSH)
	Type    string `json:"type"`
	Address string `json:"addr"`
	Hex     *Hexed `json:"hex"`
}

type BoltRoute struct {
	Pubkey                    string `json:"pubkey"`
	ShortChannelId            string `json:"short_channel_id"`
	FeeBaseMilliSatoshis      uint64 `json:"fee_base_msat"`
	FeeProportionalMillionths uint64 `json:"fee_proportional_millionths"`
	CltvExpiryDelta           uint   `json:"cltv_expiry_delta"`
}

type BoltExtra struct {
	Tag  string `json:"tag"`
	Data string `json:"data"`
}

func (l *Lightning) DecodeBolt11(bolt11 string) (*DecodedBolt11, error) {
	return l.DecodePay(bolt11, "")
}

// Decode the {bolt11}, using the provided 'description' if necessary.*
//
// * This is only necesary if the bolt11 includes a description hash.
// The provided description must match the included hash.
func (l *Lightning) DecodePay(bolt11, desc string) (*DecodedBolt11, error) {
	if bolt11 == "" {
		return nil, fmt.Errorf("Must call decode pay with a bolt11")
	}

	var result DecodedBolt11
	err := l.client.Request(&DecodePayRequest{bolt11, desc}, &result)
	return &result, err
}

type PayStatus struct {
	Bolt11       string       `json:"bolt11"`
	MilliSatoshi uint64       `json:"msatoshi"`
	AmountMsat   string       `json:"amount_msat"`
	Destination  string       `json:"destination"`
	Attempts     []PayAttempt `json:"attempts"`
}

type PayAttempt struct {
	Strategy          string            `json:"strategy"`
	AgeInSeconds      uint64            `json:"age_in_seconds"`
	DurationInSeconds uint64            `json:"duration_in_seconds"`
	StartTime         string            `json:"start_time"`
	EndTime           string            `json:"end_time,omitempty"`
	ExcludedChannels  []string          `json:"excluded_channels,omitempty"`
	Route             []RouteHop        `json:"route,omitempty"`
	Failure           PayAttemptFailure `json:"failure,omitempty"`
}

type PayAttemptFailure struct {
	Code    uint32           `json:"code"`
	Message string           `json:"message"`
	Data    PaymentErrorData `json:"data,omitempty"`
}

type PayStatusRequest struct {
	Bolt11 string `json:"bolt11,omitempty"`
}

func (r PayStatusRequest) Name() string {
	return "paystatus"
}

// List detailed information about all payment attempts
func (l *Lightning) ListPayStatuses() ([]PayStatus, error) {
	return l.paystatus("")
}

// Detailed information about a payment attempt to a given bolt11
func (l *Lightning) GetPayStatus(bolt11 string) (*PayStatus, error) {
	result, err := l.paystatus(bolt11)
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, errors.New("No status for bolt11 found.")
	}
	return &result[0], nil
}

func (l *Lightning) paystatus(bolt11 string) ([]PayStatus, error) {
	var result struct {
		Pays []PayStatus `json:"pay"`
	}
	err := l.client.Request(&PayStatusRequest{bolt11}, &result)
	if err != nil {
		return nil, err
	}
	return result.Pays, nil
}

type HelpRequest struct {
	Command string `json:"command,omitempty"`
}

func (r HelpRequest) Name() string {
	return "help"
}

type Command struct {
	NameAndUsage string `json:"command"`
	Description  string `json:"description"`
	Verbose      string `json:"verbose"`
	Category     string `json:"category"`
}

// Show available c-lightning RPC commands
func (l *Lightning) Help() ([]*Command, error) {
	var result struct {
		Commands []*Command `json:"help"`
	}
	err := l.client.Request(&HelpRequest{}, &result)
	return result.Commands, err
}

func (l *Lightning) HelpFor(command string) (*Command, error) {
	var result struct {
		Commands []*Command `json:"help"`
	}
	err := l.client.Request(&HelpRequest{command}, &result)
	if err != nil {
		return nil, err
	}
	if len(result.Commands) <= 0 {
		return nil, errors.New(fmt.Sprintf("Command '%s' not found", command))
	}
	return result.Commands[0], nil
}

type StopRequest struct{}

func (r StopRequest) Name() string {
	return "stop"
}

// Shut down the c-lightning process. Will return a string
// of "Shutting down" on success.
func (l *Lightning) Stop() (string, error) {
	var result string
	err := l.client.Request(&StopRequest{}, &result)
	return result, err
}

type LogLevel int

const (
	None LogLevel = iota
	Info
	Unusual
	Debug
	Io
)

func (l LogLevel) String() string {
	return []string{
		"",
		"info",
		"unusual",
		"debug",
		"io",
	}[l]
}

type LogRequest struct {
	Level string `json:"level,omitempty"`
}

func (r LogRequest) Name() string {
	return "getlog"
}

type LogResponse struct {
	CreatedAt string `json:"created_at"`
	BytesUsed uint64 `json:"bytes_used"`
	BytesMax  uint64 `json:"bytes_max"`
	Logs      []Log  `json:"log"`
}

type Log struct {
	Type       string `json:"type"`
	Time       string `json:"time,omitempty"`
	Source     string `json:"source,omitempty"`
	Message    string `json:"log,omitempty"`
	NumSkipped uint   `json:"num_skipped,omitempty"`
}

// Show logs, with optional log {level} (info|unusual|debug|io)
func (l *Lightning) GetLog(level LogLevel) (*LogResponse, error) {
	var result LogResponse
	err := l.client.Request(&LogRequest{level.String()}, &result)
	return &result, err
}

type DevRHashRequest struct {
	Secret string `json:"secret"`
}

func (r DevRHashRequest) Name() string {
	return "dev-rhash"
}

type DevHashResult struct {
	RHash string `json:"rhash"`
}

// Show SHA256 of {secret}
func (l *Lightning) DevHash(secret string) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("Must pass in a valid secret to hash")
	}

	var result DevHashResult
	err := l.client.Request(&DevRHashRequest{secret}, &result)
	return result.RHash, err
}

type DevCrashRequest struct{}

func (r DevCrashRequest) Name() string {
	return "dev-crash"
}

// Crash lightningd by calling fatal(). Returns nothing.
func (l *Lightning) DevCrash() (interface{}, error) {
	err := l.client.Request(&DevCrashRequest{}, nil)
	return nil, err
}

type DevQueryShortChanIdsRequest struct {
	PeerId       string   `json:"id"`
	ShortChanIds []string `json:"scids"`
}

func (r DevQueryShortChanIdsRequest) Name() string {
	return "dev-query-scids"
}

type QueryShortChannelIdsResponse struct {
	IsComplete bool `json:"complete"`
}

// Ask a peer for a particular set of short channel ids
func (l *Lightning) DevQueryShortChanIds(peerId string, shortChanIds []string) (*QueryShortChannelIdsResponse, error) {
	if peerId == "" {
		return nil, fmt.Errorf("Must provide a peer id")
	}

	if len(shortChanIds) == 0 {
		return nil, fmt.Errorf("Must specify short channel ids to query for")
	}

	var result QueryShortChannelIdsResponse
	err := l.client.Request(&DevQueryShortChanIdsRequest{peerId, shortChanIds}, &result)
	return &result, err
}

type GetInfoRequest struct{}

func (r GetInfoRequest) Name() string {
	return "getinfo"
}

type NodeInfo struct {
	Id                         string            `json:"id"`
	Alias                      string            `json:"alias"`
	Color                      string            `json:"color"`
	PeerCount                  int               `json:"num_peers"`
	PendingChannelCount        int               `json:"num_pending_channels"`
	ActiveChannelCount         int               `json:"num_active_channels"`
	InactiveChannelCount       int               `json:"num_inactive_channels"`
	Addresses                  []Address         `json:"address"`
	Binding                    []AddressInternal `json:"binding"`
	Version                    string            `json:"version"`
	Blockheight                uint              `json:"blockheight"`
	Network                    string            `json:"network"`
	FeesCollectedMilliSatoshis uint64            `json:"msatoshi_fees_collected"`
	FeesCollected              string            `json:"fees_collected_msat"`
	LightningDir               string            `json:"lightning-dir"`
	WarningBitcoinSync         string            `json:"warning_bitcoind_sync,omitempty"`
	WarningLightningSync       string            `json:"warning_lightningd_sync,omitempty"`
}

func (n *NodeInfo) IsBitcoindSync() bool {
	return n.WarningBitcoinSync == ""
}

func (n *NodeInfo) IsLightningdSync() bool {
	return n.WarningLightningSync == ""
}

type AddressInternal struct {
	Type    string  `json:"type"`
	Addr    string  `json:"address"`
	Port    int     `json:"port"`
	Socket  string  `json:"socket"`
	Service Address `json:"service"`
	Name    string  `json:"name"`
}

func (l *Lightning) GetInfo() (*NodeInfo, error) {
	var result NodeInfo
	err := l.client.Request(&GetInfoRequest{}, &result)
	return &result, err
}

type SignedMessage struct {
	Signature string `json:"signature"`
	RecId     string `json:"recid"`
	ZBase     string `json:"zbase"`
}

type SignMessageRequest struct {
	Message string `json:"message"`
}

func (r SignMessageRequest) Name() string {
	return "signmessage"
}

func (l *Lightning) SignMessage(message string) (*SignedMessage, error) {
	var result SignedMessage
	err := l.client.Request(&SignMessageRequest{message}, &result)
	return &result, err
}

type CheckedMessage struct {
	Pubkey   string `json:"pubkey"`
	Verified bool   `json:"verified"`
}

type CheckMessageRequest struct {
	Message string `json:"message"`
	ZBase   string `json:"zbase"`
	Pubkey  string `json:"pubkey,omitempty"`
}

func (r CheckMessageRequest) Name() string {
	return "checkmessage"
}

// No pubkey provided, so we return the pubkey
func (l *Lightning) CheckMessage(message, zbase string) (bool, string, error) {
	var result CheckedMessage
	request := &CheckMessageRequest{
		Message: message,
		ZBase:   zbase,
	}
	err := l.client.Request(request, &result)
	return result.Verified, result.Pubkey, err
}

// Pubkey provided, so we return whether or not is verified
func (l *Lightning) CheckMessageVerify(message, zbase, pubkey string) (bool, error) {
	var result CheckedMessage
	err := l.client.Request(&CheckMessageRequest{message, zbase, pubkey}, &result)
	return result.Verified, err
}

type SendPayRequest struct {
	Route         []RouteHop `json:"route"`
	PaymentHash   string     `json:"payment_hash"`
	Label         string     `json:"label,omitempty"`
	MilliSatoshis *uint64    `json:"msatoshi,omitempty"`
	Bolt11        string     `json:"bolt11,omitempty"`
	PaymentSecret string     `json:"payment_secret,omitempty"`
	PartId        *uint64    `json:"partid,omitempty"`
}

func (r SendPayRequest) Name() string {
	return "sendpay"
}

type SendPayFields struct {
	Id                    uint64 `json:"id"`
	PaymentHash           string `json:"payment_hash"`
	Destination           string `json:"destination,omitempty"`
	AmountMilliSatoshiRaw uint64 `json:"msatoshi,omitempty"`
	AmountMilliSatoshi    string `json:"amount_msat"`
	MilliSatoshiSentRaw   uint64 `json:"msatoshi_sent"`
	MilliSatoshiSent      string `json:"amount_sent_msat"`
	CreatedAt             uint64 `json:"created_at"`
	Status                string `json:"status"`
	PaymentPreimage       string `json:"payment_preimage,omitempty"`
	Label                 string `json:"label,omitempty"`
	Bolt11                string `json:"bolt11,omitempty"`
	PartId                uint64 `json:"partid,omitempty"`
	ErrorOnion            string `json:"erroronion,omitempty"`
}

type SendPayResult struct {
	Message string `json:"message"`
	SendPayFields
}

// SendPay, but without description or millisatoshi value
func (l *Lightning) SendPayLite(route []RouteHop, paymentHash string) (*SendPayResult, error) {
	return l.SendPay(route, paymentHash, "", nil, "", "", nil)
}

// Send along {route} in return for preimage of {paymentHash}
//  Description and msat are optional.
// Generally a client would call GetRoute to resolve a route, then
// use SendPay to send it.  If it fails, it would call GetRoute again
// to retry.
//
// Response will occur when payment is on its way to the destination.
// Does not wait for a definitive success or failure. Use 'waitsendpay'
// to poll or wait for definite success or failure.
//
// 'description', if provided, will be returned in 'waitsendpay' and
// 'listsendpays' results.
//
// 'msat', if provided, is the amount that will be recorded as the target
// payment value. If not specified, it will be the final amount to the
// destination (specified in route).  If specified, then the final amount
// at the destination must be from the specified 'msat' to twice that
// value, inclusive. This is inteded to obscure payments by overpaying
// slightly at the destination -- the acutal target paymnt is what
// should be specified as the 'msat' argument.
//
// Once a payment has succeeded, calls to 'SendPay' with the same
// 'paymentHash' but a different 'msat' or destination will fail; this
// prevents accidental multiple payments. Calls with the same 'paymentHash',
// 'msat' and destination as a previous successful payment will return
// immediately with a success, even if the route is different.
func (l *Lightning) SendPay(route []RouteHop, paymentHash, label string, msat *uint64, bolt11 string, paymentSecret string, partId *uint64) (*SendPayResult, error) {
	if paymentHash == "" {
		return nil, fmt.Errorf("Must specify a paymentHash to pay")
	}
	if len(route) == 0 {
		return nil, fmt.Errorf("Must specify a route to send payment along")
	}

	var result SendPayResult
	err := l.client.Request(&SendPayRequest{
		Route:         route,
		PaymentHash:   paymentHash,
		Label:         label,
		MilliSatoshis: msat,
		Bolt11:        bolt11,
		PaymentSecret: paymentSecret,
		PartId:        partId,
	}, &result)
	return &result, err
}

type WaitSendPayRequest struct {
	PaymentHash string  `json:"payment_hash"`
	Timeout     uint    `json:"timeout,omitempty"`
	PartId      *uint64 `json:"partid,omitempty"`
}

func (r WaitSendPayRequest) Name() string {
	return "waitsendpay"
}

type PaymentError struct {
	*jrpc2.RpcError
	Data *PaymentErrorData
}

type PaymentErrorData struct {
	*PaymentFields
	OnionReply      string `json:"onionreply,omitempty"`
	ErringIndex     uint64 `json:"erring_index"`
	FailCode        int    `json:"failcode"`
	ErringNode      string `json:"erring_node,omitempty"`
	ErringChannel   string `json:"erring_channel,omitempty"`
	ErringDirection int    `json:"erring_direction,omitempty"`
	FailCodeName    string `json:"failcodename,omitempty"`
	RawMessage      string `json:"raw_message,omitempty"`
}

// Polls or waits for the status of an outgoing payment that was
// initiated by a previous 'SendPay' invocation.
//
// May provide a 'timeout, in seconds. When provided, will return a
// 200 error code (payment still in progress) if timeout elapses
// before the payment is definitively concluded (success or fail).
// If no 'timeout' is provided, the call waits indefinitely.
//
// NB: Blocking. Bypasses the default client request timeout mechanism
func (l *Lightning) WaitSendPay(paymentHash string, timeout uint) (*SendPayFields, error) {
	return l.WaitSendPayPart(paymentHash, timeout, nil)
}

func (l *Lightning) WaitSendPayPart(paymentHash string, timeout uint, partId *uint64) (*SendPayFields, error) {
	if paymentHash == "" {
		return nil, fmt.Errorf("Must provide a payment hash to pay")
	}

	var result SendPayFields
	err := l.client.RequestNoTimeout(&WaitSendPayRequest{
		PaymentHash: paymentHash,
		Timeout:     timeout,
		PartId:      partId,
	}, &result)
	if err, ok := err.(*jrpc2.RpcError); ok {
		var paymentErrData PaymentErrorData
		parseErr := err.ParseData(&paymentErrData)
		if parseErr != nil {
			log.Printf(parseErr.Error())
			return &result, err
		}
		return &result, &PaymentError{err, &paymentErrData}
	}

	return &result, err
}

type PayRequest struct {
	Bolt11        string  `json:"bolt11"`
	MilliSatoshi  uint64  `json:"msatoshi,omitempty"`
	Desc          string  `json:"description,omitempty"`
	RiskFactor    float32 `json:"riskfactor,omitempty"`
	MaxFeePercent float32 `json:"maxfeeprecent,omitempty"`
	RetryFor      uint    `json:"retry_for,omitempty"`
	MaxDelay      uint    `json:"maxdelay,omitempty"`
	ExemptFee     bool    `json:"exemptfee,omitempty"`
}

func (r PayRequest) Name() string {
	return "pay"
}

// todo: there's lots of different data that comes back for
// payment failures, that for now we totally lose
type PaymentSuccess struct {
	SendPayFields
	GetRouteTries int          `json:"getroute_tries"`
	SendPayTries  int          `json:"sendpay_tries"`
	Route         []RouteHop   `json:"route"`
	Failures      []PayFailure `json:"failures"`
}

type PayFailure struct {
	Message       string     `json:"message"`
	Type          string     `json:"type"`
	OnionReply    string     `json:"onionreply"`
	ErringIndex   int        `json:"erring_index"`
	FailCode      int        `json:"failcode"`
	ErringNode    string     `json:"erring_node"`
	ErringChannel string     `json:"erring_channel"`
	ChannelUpdate string     `json:"channel_update"`
	Route         []RouteHop `json:"route"`
}

func (l *Lightning) PayBolt(bolt11 string) (*PaymentSuccess, error) {
	return l.Pay(&PayRequest{
		Bolt11: bolt11,
	})
}

// Send payment as specified by 'Bolt11' with 'MilliSatoshi'
// (Millisatoshis amount is ignored if the 'Bolt11' includes an amount).
//
// 'description' is required if the 'bolt11' includes a description hash.
//
// 'riskfactor' is optional, defaults to 1.0
// Briefly, the 'riskfactor' is the estimated annual cost of your funds
// being stuck (as a percentage), multiplied by the percent change of
// each node failing. Ex: 1% chance of node failure and a 20% annual cost
// would give you a risk factor of 20. c-lightning defaults to 1.0
//
// 'MaxFeePercent' is the max percentage of a payment that can be paid
// in fees. c-lightning defaults to 0.5.
//
// 'ExemptFee' can be used for tiny paymetns which would otherwise be
// dominated by the fee leveraged by forwarding nodes. Setting 'ExemptFee'
// allows 'MaxFeePercent' check to be skipped on fees that are smaller than
// 'ExemptFee'. c-lightning default is 5000 millisatoshi.
//
// c-lightning will keep finding routes and retrying payment until it succeeds
// or the given 'RetryFor' seconds have elapsed.  Note that the command may
// stop retrying while payment is pending. You can continuing monitoring
// payment status with the ListSendPays or WaitSendPay. 'RetryFor' defaults
// to 60 seconds.
//
// 'MaxDelay' is used when determining whether a route incurs an acceptable
// delay. A route will not be used if the estimated delay is above this.
// Defaults to the configured locktime max (--max-locktime-blocks)
// Units is in blocks.
func (l *Lightning) Pay(req *PayRequest) (*PaymentSuccess, error) {
	if req.Bolt11 == "" {
		return nil, fmt.Errorf("Must supply a Bolt11 to pay")
	}
	if req.RiskFactor < 0 {
		return nil, fmt.Errorf("Risk factor must be postiive %f", req.RiskFactor)
	}
	if req.MaxFeePercent < 0 || req.MaxFeePercent > 100 {
		return nil, fmt.Errorf("MaxFeePercent must be a percentage. %f", req.MaxFeePercent)
	}
	var result PaymentSuccess
	err := l.client.RequestNoTimeout(req, &result)
	return &result, err
}

type PaymentFields struct {
	Bolt11                 string `json:"bolt11"`
	Status                 string `json:"status"`
	PaymentPreImage        string `json:"payment_preimage"`
	AmountSentMilliSatoshi string `json:"amount_sent_msat"`
	Label                  string `json:"label,omitempty"`
}

type ListPaysRequest struct {
	Bolt11 string `json:"bolt11,omitempty"`
}

func (r ListPaysRequest) Name() string {
	return "listpays"
}

func (l *Lightning) ListPays() ([]PaymentFields, error) {
	var result struct {
		Payments []PaymentFields `json:"pays"`
	}
	err := l.client.Request(&ListPaysRequest{}, &result)
	return result.Payments, err
}

func (l *Lightning) ListPaysToBolt11(bolt11 string) ([]PaymentFields, error) {
	var result struct {
		Payments []PaymentFields `json:"payments"`
	}
	err := l.client.Request(&ListPaysRequest{bolt11}, &result)
	return result.Payments, err
}

type ListSendPaysRequest struct {
	Bolt11      string `json:"bolt11,omitempty"`
	PaymentHash string `json:"payment_hash,omitempty"`
}

func (r ListSendPaysRequest) Name() string {
	return "listsendpays"
}

func (l *Lightning) ListSendPaysAll() ([]SendPayFields, error) {
	return l.listSendPays(&ListSendPaysRequest{})
}

// Show outgoing payments, regarding {bolt11}
func (l *Lightning) ListSendPays(bolt11 string) ([]SendPayFields, error) {
	return l.listSendPays(&ListSendPaysRequest{
		Bolt11: bolt11,
	})
}

// Show outgoing payments, regarding {paymentHash}
func (l *Lightning) ListSendPaysByHash(paymentHash string) ([]SendPayFields, error) {
	return l.listSendPays(&ListSendPaysRequest{
		PaymentHash: paymentHash,
	})
}

func (l *Lightning) listSendPays(req *ListSendPaysRequest) ([]SendPayFields, error) {
	var result struct {
		Payments []SendPayFields `json:"payments"`
	}
	err := l.client.Request(req, &result)
	return result.Payments, err
}

type TransactionsRequest struct {
}

func (r TransactionsRequest) Name() string {
	return "listtransactions"
}

type Transaction struct {
	Hash        string     `json:"hash"`
	RawTx       string     `json:"rawtx"`
	Blockheight uint       `json:"blockheight"`
	TxIndex     uint       `json:"txindex"`
	LockTime    uint64     `json:"locktime"`
	Version     uint       `json:"version"`
	Inputs      []TxInput  `json:"inputs"`
	Outputs     []TxOutput `json:"outputs"`
	Type        []string   `json:"type,omitempty"`
}

type TxInput struct {
	TxId     string `json:"txid"`
	Index    uint   `json:"index"`
	Sequence uint64 `json:"sequence"`
	Type     string `json:"type,omitempty"`
}

type TxOutput struct {
	Index        uint   `json:"index"`
	Satoshis     string `json:"satoshis"`
	ScriptPubkey string `json:"scriptPubKey"`
	Type         string `json:"type,omitempty"`
}

func (l *Lightning) ListTransactions() ([]Transaction, error) {
	var result struct {
		Transactions []Transaction `json:"transactions"`
	}
	err := l.client.Request(&TransactionsRequest{}, &result)
	return result.Transactions, err
}

type ConnectRequest struct {
	PeerId string `json:"id"`
	Host   string `json:"host"`
	Port   uint   `json:"port"`
}

func (r ConnectRequest) Name() string {
	return "connect"
}

type ConnectSuccess struct {
	PeerId string `json:"id"`
}

type ConnectResult struct {
	Id       string `json:"id"`
	Features *Hexed `json:"features"`
}

// Connect to {peerId} at {host}:{port}. Returns result with peer id and peer's features
func (l *Lightning) ConnectPeer(peerId, host string, port uint) (*ConnectResult, error) {
	var result ConnectResult
	err := l.client.Request(&ConnectRequest{peerId, host, port}, &result)
	return &result, err
}

// Connect to {peerId} at {host}:{port}. Returns peer id on success
// Sort of deprecated, use ConnectPeer, as it gives you back the peer's init features as well
func (l *Lightning) Connect(peerId, host string, port uint) (string, error) {
	result, err := l.ConnectPeer(peerId, host, port)
	return result.Id, err
}

type FundChannelRequest struct {
	Id       string  `json:"id"`
	Amount   string  `json:"amount"`
	FeeRate  string  `json:"feerate,omitempty"`
	Announce bool    `json:"announce"`
	MinConf  *uint16 `json:"minconf,omitempty"`
	PushMsat string  `json:"push_msat,omitempty"`
}

func (r FundChannelRequest) Name() string {
	return "fundchannel"
}

type FundChannelResult struct {
	FundingTx   string `json:"tx"`
	FundingTxId string `json:"txid"`
	ChannelId   string `json:"channel_id"`
}

// Fund channel, defaults to public channel and default feerate.
func (l *Lightning) FundChannel(id string, amount *Sat) (*FundChannelResult, error) {
	return l.FundChannelExt(id, amount, nil, true, nil, nil)
}

func (l *Lightning) FundPrivateChannel(id string, amount *Sat) (*FundChannelResult, error) {
	return l.FundChannelExt(id, amount, nil, false, nil, nil)
}

func (l *Lightning) FundChannelAtFee(id string, amount *Sat, feerate *FeeRate) (*FundChannelResult, error) {
	return l.FundChannelExt(id, amount, feerate, true, nil, nil)
}

func (l *Lightning) FundPrivateChannelAtFee(id string, amount *Sat, feerate *FeeRate) (*FundChannelResult, error) {
	return l.FundChannelExt(id, amount, feerate, false, nil, nil)
}

// Fund channel with node {id} using {satoshi} satoshis, with feerate of {feerate}. Uses
// default feerate if unset.
// If announce is false, channel announcements will not be sent.
// can send an optional 'pushMsat', of millisatoshis to push to peer (from your funding amount)
// Any pushed msats are irrevocably gifted to the peer. (use only if you enjoy being a sats santa!)
func (l *Lightning) FundChannelExt(id string, amount *Sat, feerate *FeeRate, announce bool, minConf *uint16, pushMSat *MSat) (*FundChannelResult, error) {
	if amount == nil || (amount.Value == 0 && !amount.SendAll) {
		return nil, fmt.Errorf("Must set satoshi amount to send")
	}

	req := &FundChannelRequest{
		Id:       id,
		Amount:   amount.RawString(),
		Announce: announce,
	}
	if feerate != nil {
		req.FeeRate = feerate.String()
	}
	if pushMSat != nil {
		req.PushMsat = pushMSat.String()
	}
	req.MinConf = minConf

	var result FundChannelResult
	err := l.client.Request(req, &result)
	return &result, err
}

type FundChannelStart struct {
	Id       string `json:"id"`
	Amount   uint64 `json:"amount"`
	Announce bool   `json:"announce"`
	FeeRate  string `json:"feerate,omitempty"`
	CloseTo  string `json:"close_to,omitempty"`
}
type StartResponse struct {
	Address      string `json:"funding_address"`
	ScriptPubkey string `json:"scriptpubkey"`
}

func (r FundChannelStart) Name() string {
	return "fundchannel_start"
}

// Returns a string that's a bech32 address. this address is the funding output address.
func (l *Lightning) StartFundChannel(id string, amount uint64, announce bool, feerate *FeeRate, closeTo string) (*StartResponse, error) {
	var result StartResponse

	req := &FundChannelStart{
		Id:       id,
		Amount:   amount,
		Announce: announce,
		CloseTo:  closeTo,
	}

	if feerate != nil {
		req.FeeRate = feerate.String()
	}

	err := l.client.Request(req, &result)
	return &result, err
}

type FundChannelComplete struct {
	PeerId string `json:"id"`
	TxId   string `json:"txid"`
	TxOut  uint32 `json:"txout"`
}

func (r FundChannelComplete) Name() string {
	return "fundchannel_complete"
}

func (l *Lightning) CompleteFundChannel(peerId, txId string, txout uint32) (channelId string, err error) {
	var result struct {
		ChannelId          string `json:"channel_id"`
		CommitmentsSecured bool   `json:"commitments_secured"`
	}

	err = l.client.Request(&FundChannelComplete{peerId, txId, txout}, &result)
	return result.ChannelId, err
}

type FundChannelCancel struct {
	PeerId string `json:"id"`
}

func (r FundChannelCancel) Name() string {
	return "fundchannel_cancel"
}

func (l *Lightning) CancelFundChannel(peerId string) (bool, error) {
	var result struct {
		Cancelled string `json:"cancelled"`
	}

	err := l.client.Request(&FundChannelCancel{peerId}, &result)
	return err == nil, err
}

type CloseRequest struct {
	PeerId             string `json:"id"`
	Timeout            uint   `json:"unilateraltimeout,omitempty"`
	Destination        string `json:"destination,omitempty"`
	FeeNegotiationStep string `json:"fee_negotiation_step,omitempty"`
}

func (r CloseRequest) Name() string {
	return "close"
}

type CloseResult struct {
	Tx   string `json:"tx"`
	TxId string `json:"txid"`
	// todo: enum (mutual, unilateral)
	Type string `json:"type"`
}

func (l *Lightning) CloseNormal(id string) (*CloseResult, error) {
	return l.Close(id, 0, "")
}

func (l *Lightning) CloseTo(id, destination string) (*CloseResult, error) {
	return l.Close(id, 0, destination)
}

func (l *Lightning) CloseWithStep(id, step string) (*CloseResult, error) {
	return l.close_internal(id, 0, "", step)
}

func (l *Lightning) CloseToWithStep(id, destination, step string) (*CloseResult, error) {
	return l.close_internal(id, 0, destination, step)
}

func (l *Lightning) CloseToTimeoutWithStep(id string, timeout uint, destination, step string) (*CloseResult, error) {
	return l.close_internal(id, timeout, destination, step)
}

// Close the channel with peer {id}, timing out with {timeout} seconds, at whence a
// unilateral close is initiated.
//
// If unspecified, forces a close (timesout) in 48hours
//
// Can pass either peer id or channel id as {id} field.
//
// Note that a successful result *may* be null.
func (l *Lightning) Close(id string, timeout uint, destination string) (*CloseResult, error) {
	return l.close_internal(id, timeout, destination, "")
}

func (l *Lightning) close_internal(id string, timeout uint, destination string, step string) (*CloseResult, error) {
	var result CloseResult
	err := l.client.Request(&CloseRequest{id, timeout, destination, step}, &result)
	return &result, err
}

type DevSignLastTxRequest struct {
	PeerId string `json:"id"`
}

func (r DevSignLastTxRequest) Name() string {
	return "dev-sign-last-tx"
}

// Sign and show the last commitment transaction with peer {peerId}
// Returns the signed tx on success
func (l *Lightning) DevSignLastTx(peerId string) (string, error) {
	var result struct {
		Tx string `json:"tx"`
	}
	err := l.client.Request(&DevSignLastTxRequest{peerId}, &result)
	return result.Tx, err
}

type DevFailRequest struct {
	PeerId string `json:"id"`
}

func (r DevFailRequest) Name() string {
	return "dev-fail"
}

// Fail with peer {id}
func (l *Lightning) DevFail(peerId string) error {
	var result interface{}
	err := l.client.Request(&DevFailRequest{peerId}, result)
	return err
}

type DevReenableCommitRequest struct {
	PeerId string `json:"id"`
}

func (r DevReenableCommitRequest) Name() string {
	return "dev-reenable-commit"
}

// Re-enable the commit timer on peer {id}
func (l *Lightning) DevReenableCommit(id string) error {
	var result interface{}
	err := l.client.Request(&DevReenableCommitRequest{id}, result)
	return err
}

type PingRequest struct {
	Id        string `json:"id"`
	Len       uint   `json:"len"`
	PongBytes uint   `json:"pongbytes"`
}

func (r PingRequest) Name() string {
	return "ping"
}

type Pong struct {
	TotalLen int `json:"totlen"`
}

// Send {peerId} a ping of size 128, asking for 128 bytes in response
func (l *Lightning) Ping(peerId string) (*Pong, error) {
	return l.PingWithLen(peerId, 128, 128)
}

// Send {peerId} a ping of length {pingLen} asking for bytes {pongByteLen}
func (l *Lightning) PingWithLen(peerId string, pingLen, pongByteLen uint) (*Pong, error) {
	var result Pong
	err := l.client.Request(&PingRequest{peerId, pingLen, pongByteLen}, &result)
	return &result, err
}

type DevMemDumpRequest struct{}

func (r DevMemDumpRequest) Name() string {
	return "dev-memdump"
}

type MemDumpEntry struct {
	ParentPtr string          `json:"parent"`
	ValuePtr  string          `json:"value"`
	Label     string          `json:"label"`
	Children  []*MemDumpEntry `json:"children"`
}

// Show memory objects currently in use
func (l *Lightning) DevMemDump() ([]*MemDumpEntry, error) {
	var result []*MemDumpEntry
	err := l.client.Request(&DevMemDumpRequest{}, &result)
	return result, err
}

type DevMemLeakRequest struct{}

func (r DevMemLeakRequest) Name() string {
	return "dev-memleak"
}

type MemLeakResult struct {
	Leaks []*MemLeak `json:"leaks"`
}

type MemLeak struct {
	PointerValue string   `json:"value"`
	Label        string   `json:"label"`
	Backtrace    []string `json:"backtrace"`
	Parents      []string `json:"parents"`
}

// Show unreferenced memory objects
func (l *Lightning) DevMemLeak() ([]*MemLeak, error) {
	var result MemLeakResult
	err := l.client.Request(&DevMemLeakRequest{}, &result)
	return result.Leaks, err
}

type WithdrawRequest struct {
	Destination string   `json:"destination"`
	Satoshi     string   `json:"satoshi"`
	FeeRate     string   `json:"feerate,omitempty"`
	MinConf     uint16   `json:"minconf,omitempty"`
	Utxos       []string `json:"utxos,omitempty"`
}

type FeeDirective int

const (
	Normal FeeDirective = iota
	Urgent
	Slow
)

func (f FeeDirective) String() string {
	return []string{
		"normal",
		"urgent",
		"slow",
	}[f]
}

type FeeRateStyle int

const (
	PerKb FeeRateStyle = iota
	PerKw
)

type FeeRate struct {
	Rate      uint
	Style     FeeRateStyle
	Directive FeeDirective
}

func (r FeeRateStyle) String() string {
	return []string{"perkb", "perkw"}[r]
}

func (f *FeeRate) String() string {
	if f.Rate > 0 {
		return fmt.Sprint(f.Rate) + f.Style.String()
	}
	// defaults to 'normal'
	return f.Directive.String()
}

func NewFeeRate(style FeeRateStyle, rate uint) *FeeRate {
	return &FeeRate{
		Style: style,
		Rate:  rate,
	}
}

func NewFeeRateByDirective(style FeeRateStyle, directive FeeDirective) *FeeRate {
	return &FeeRate{
		Style:     style,
		Directive: directive,
	}
}

func (r WithdrawRequest) Name() string {
	return "withdraw"
}

type WithdrawResult struct {
	Tx   string `json:"tx"`
	TxId string `json:"txid"`
}

// Withdraw sends funds from c-lightning's internal wallet to the
// address specified in {destination}. Address can be of any Bitcoin
// accepted type, including bech32.
//
// {satoshi} is the amount to be withdrawn from the wallet.
//
// {feerate} is an optional feerate to use. Can be either a directive
// (urgent, normal, or slow) or a number with an optional suffix.
// 'perkw' means the number is interpreted as satoshi-per-kilosipa (weight)
// and 'perkb' means it is interpreted bitcoind-style as satoshi-per-kilobyte.
// Omitting the suffix is equivalent to 'perkb'
// If not set, {feerate} defaults to 'normal'.
func (l *Lightning) Withdraw(destination string, amount *Sat, feerate *FeeRate, minConf *uint16) (*WithdrawResult, error) {
	return l.WithdrawWithUtxos(destination, amount, feerate, minConf, nil)
}

func (l *Lightning) WithdrawWithUtxos(destination string, amount *Sat, feerate *FeeRate, minConf *uint16, utxos []*Utxo) (*WithdrawResult, error) {
	if amount == nil || (amount.Value == 0 && !amount.SendAll) {
		return nil, fmt.Errorf("Must set satoshi amount to send")
	}
	if destination == "" {
		return nil, fmt.Errorf("Must supply a destination for withdrawal")
	}

	request := &WithdrawRequest{
		Destination: destination,
		Satoshi:     amount.RawString(),
	}
	if feerate != nil {
		request.FeeRate = feerate.String()
	}
	if minConf != nil {
		request.MinConf = *minConf
	}

	if utxos != nil {
		request.Utxos = stringifyUtxos(utxos)
	}

	var result WithdrawResult
	err := l.client.Request(request, &result)
	return &result, err
}

type NewAddrRequest struct {
	AddressType string `json:"addresstype,omitempty"`
}

func (r NewAddrRequest) Name() string {
	return "newaddr"
}

type AddressType int

const (
	Bech32 AddressType = iota
	P2SHSegwit
	All
)

type NewAddrResult struct {
	Bech32     string `json:"bech32"`
	P2SHSegwit string `json:"p2sh-segwit"`
}

func (a AddressType) String() string {
	return []string{"bech32", "p2sh-segwit", "all"}[a]
}

// Get new Bech32 address for the internal wallet.
func (l *Lightning) NewAddr() (string, error) {
	addr, err := l.NewAddress(Bech32)
	return addr.Bech32, err
}

// Get new address of type {addrType} from the internal wallet.
func (l *Lightning) NewAddress(addrType AddressType) (*NewAddrResult, error) {
	var result NewAddrResult
	err := l.client.Request(&NewAddrRequest{addrType.String()}, &result)

	return &result, err
}

type Outputs struct {
	Address string
	Satoshi uint64
}

func (o *Outputs) Marshal() []byte {
	return []byte(fmt.Sprintf(`{"%s":"%vsat"}`, o.Address, o.Satoshi))
}

// Because we're using a weird JSON marshaler for parameter packing
// we encode the outputs before passing them along as a request (instead
// of writing a custom json Marshaler)
func stringifyOutputs(outputs []*Outputs) []json.RawMessage {
	results := make([]json.RawMessage, len(outputs))

	for i := 0; i < len(outputs); i++ {
		results[i] = json.RawMessage(outputs[i].Marshal())
	}

	return results
}

type Utxo struct {
	TxId  string
	Index uint
}

func (u *Utxo) String() string {
	return fmt.Sprintf("%s:%v", u.TxId, u.Index)
}

func stringifyUtxos(utxos []*Utxo) []string {
	results := make([]string, len(utxos))

	for i := 0; i < len(utxos); i++ {
		results[i] = utxos[i].String()
	}

	return results
}

type TxPrepare struct {
	Outputs []json.RawMessage `json:"outputs"`
	FeeRate string            `json:"feerate,omitempty"`
	MinConf uint16            `json:"minconf,omitempty"`
	Utxos   []string          `json:"utxos,omitempty"`
}

type TxResult struct {
	Tx   string `json:"unsigned_tx"`
	TxId string `json:"txid"`
}

func (r *TxPrepare) Name() string {
	return "txprepare"
}

func (l *Lightning) PrepareTx(outputs []*Outputs, feerate *FeeRate, minConf *uint16) (*TxResult, error) {
	return l.PrepareTxWithUtxos(outputs, feerate, minConf, nil)
}

func (l *Lightning) PrepareTxWithUtxos(outputs []*Outputs, feerate *FeeRate, minConf *uint16, utxos []*Utxo) (*TxResult, error) {
	if len(outputs) < 0 {
		return nil, fmt.Errorf("Must supply at least one output")
	}

	request := &TxPrepare{
		Outputs: stringifyOutputs(outputs),
	}

	if feerate != nil {
		request.FeeRate = feerate.String()
	}

	if minConf != nil {
		request.MinConf = *minConf
	}

	if utxos != nil {
		request.Utxos = stringifyUtxos(utxos)
	}

	var result TxResult
	err := l.client.Request(request, &result)
	return &result, err
}

type TxDiscard struct {
	TxId string `json:"txid"`
}

func (r *TxDiscard) Name() string {
	return "txdiscard"
}

// Abandon a transaction created by PrepareTx
func (l *Lightning) DiscardTx(txid string) (*TxResult, error) {
	var result TxResult
	err := l.client.Request(&TxDiscard{txid}, &result)
	return &result, err
}

type TxSend struct {
	TxId string `json:"txid"`
}

func (r *TxSend) Name() string {
	return "txsend"
}

// Sign and broadcast a transaction created by PrepareTx
func (l *Lightning) SendTx(txid string) (*TxResult, error) {
	var result TxResult
	err := l.client.Request(&TxSend{txid}, &result)
	return &result, err
}

type ListFundsRequest struct{}

func (r *ListFundsRequest) Name() string {
	return "listfunds"
}

type FundsResult struct {
	Outputs  []*FundOutput     `json:"outputs"`
	Channels []*FundingChannel `json:"channels"`
}

type FundOutput struct {
	TxId               string `json:"txid"`
	Output             int    `json:"output"`
	Value              uint64 `json:"value"`
	AmountMilliSatoshi string `json:"amount_msat"`
	Address            string `json:"address"`
	Status             string `json:"status"`
	Blockheight        int    `json:"blockheight,omitempty"`
}

type FundingChannel struct {
	Id                    string `json:"peer_id"`
	ShortChannelId        string `json:"short_channel_id"`
	OurAmountMilliSatoshi string `json:"our_amount_msat"`
	AmountMilliSatoshi    string `json:"amount_msat"`
	ChannelSatoshi        uint64 `json:"channel_sat"`
	ChannelTotalSatoshi   uint64 `json:"channel_total_sat"`
	FundingTxId           string `json:"funding_txid"`
	FundingOutput         int    `json:"funding_output"`
	Connected             bool   `json:"connected"`
	State                 string `json:"state"`
}

// Funds in wallet.
func (l *Lightning) ListFunds() (*FundsResult, error) {
	var result FundsResult
	err := l.client.Request(&ListFundsRequest{}, &result)
	return &result, err
}

type ListForwardsRequest struct{}

func (r *ListForwardsRequest) Name() string {
	return "listforwards"
}

type Forwarding struct {
	InChannel       string  `json:"in_channel"`
	OutChannel      string  `json:"out_channel"`
	MilliSatoshiIn  uint64  `json:"in_msatoshi"`
	InMsat          string  `json:"in_msat"`
	MilliSatoshiOut uint64  `json:"out_msatoshi"`
	OutMsat         string  `json:"out_msat"`
	Fee             uint64  `json:"fee"`
	FeeMsat         string  `json:"fee_msat"`
	Status          string  `json:"status"`
	PaymentHash     string  `json:"payment_hash"`
	FailCode        int     `json:"failcode"`
	FailReason      string  `json:"failreason"`
	ReceivedTime    float64 `json:"received_time"`
	ResolvedTime    float64 `json:"resolved_time"`
}

// List all forwarded payments and their information
func (l *Lightning) ListForwards() ([]Forwarding, error) {
	var result struct {
		Forwards []Forwarding `json:"forwards"`
	}
	err := l.client.Request(&ListForwardsRequest{}, &result)
	return result.Forwards, err
}

type DevRescanOutputsRequest struct{}

func (r *DevRescanOutputsRequest) Name() string {
	return "dev-rescan-outputs"
}

type Output struct {
	TxId     string `json:"txid"`
	Output   uint   `json:"output"`
	OldState uint   `json:"oldstate"`
	NewState uint   `json:"newstate"`
}

// Synchronize the state of our funds with bitcoind
func (l *Lightning) DevRescanOutputs() ([]Output, error) {
	var result struct {
		Outputs []Output `json:"outputs"`
	}
	err := l.client.Request(&DevRescanOutputsRequest{}, &result)
	return result.Outputs, err
}

type DevForgetChannelRequest struct {
	PeerId string `json:"id"`
	Force  bool   `json:"force"`
}

func (r *DevForgetChannelRequest) Name() string {
	return "dev-forget-channel"
}

type ForgetChannelResult struct {
	WasForced        bool   `json:"forced"`
	IsFundingUnspent bool   `json:"funding_unspent"`
	FundingTxId      string `json:"funding_txid"`
}

// Forget channel with id {peerId}. Optionally {force} if has active channel.
// Caution, this might lose you funds.
func (l *Lightning) DevForgetChannel(peerId string, force bool) (*ForgetChannelResult, error) {
	var result ForgetChannelResult
	err := l.client.Request(&DevForgetChannelRequest{peerId, force}, &result)
	return &result, err
}

type DisconnectRequest struct {
	PeerId string `json:"id"`
	Force  bool   `json:"force"`
}

func (r *DisconnectRequest) Name() string {
	return "disconnect"
}

// Disconnect from peer with {peerId}. Optionally {force} if has active channel.
// Returns a nil response on success
func (l *Lightning) Disconnect(peerId string, force bool) error {
	var result interface{}
	err := l.client.Request(&DisconnectRequest{peerId, force}, &result)
	return err
}

type FeeRatesRequest struct {
	Style string `json:"style"`
}

func (r *FeeRatesRequest) Name() string {
	return "feerates"
}

type FeeRateEstimate struct {
	Style           FeeRateStyle
	Details         *FeeRateDetails
	OnchainEstimate *OnchainEstimate `json:"onchain_fee_estimates"`
	Warning         string           `json:"warning"`
}

type OnchainEstimate struct {
	OpeningChannelSatoshis  uint64 `json:"opening_channel_satoshis"`
	MutualCloseSatoshis     uint64 `json:"mutual_close_satoshis"`
	UnilateralCloseSatoshis uint64 `json:"unilateral_close_satoshis"`
	HtlcTimeoutSatoshis     uint64 `json:"htlc_timeout_satoshis"`
	HtlcSuccessSatoshis     uint64 `json:"htlc_success_satoshis"`
}

type FeeRateDetails struct {
	Urgent          int  `json:"urgent"`
	Normal          int  `json:"normal"`
	Slow            int  `json:"slow"`
	MinAcceptable   int  `json:"min_acceptable"`
	MaxAcceptable   int  `json:"max_acceptable"`
	Opening         uint `json:"opening"`
	MutualClose     uint `json:"mutual_close"`
	UnilateralClose uint `json:"unilateral_close"`
	DelayedToUs     uint `json:"delayed_to_us"`
	HtlcResolution  uint `json:"htlc_resolution"`
	Penalty         uint `json:"penalty"`
}

// Return feerate estimates, either satoshi-per-kw or satoshi-per-kb {style}
func (l *Lightning) FeeRates(style FeeRateStyle) (*FeeRateEstimate, error) {
	var result struct {
		PerKw           *FeeRateDetails  `json:"perkw"`
		PerKb           *FeeRateDetails  `json:"perkb"`
		OnchainEstimate *OnchainEstimate `json:"onchain_fee_estimates"`
		Warning         string           `json:"warning"`
	}
	err := l.client.Request(&FeeRatesRequest{style.String()}, &result)
	if err != nil {
		return nil, err
	}

	var details *FeeRateDetails
	switch style {
	case PerKb:
		details = result.PerKb
	case PerKw:
		details = result.PerKw
	}

	return &FeeRateEstimate{
		Style:           style,
		Details:         details,
		OnchainEstimate: result.OnchainEstimate,
		Warning:         result.Warning,
	}, nil
}

type ChannelFeeResult struct {
	Base           uint64        `json:"base"`
	PartPerMillion uint64        `json:"ppm"`
	Channels       []ChannelInfo `json:"channels"`
}

type ChannelInfo struct {
	PeerId         string `json:"peer_id"`
	ChannelId      string `json:"channel_id"`
	ShortChannelId string `json:"short_channel_id"`
}

type SetChannelFeeRequest struct {
	Id                string `json:"id"`
	BaseMilliSatoshis string `json:"base,omitempty"`
	PartPerMillion    uint32 `json:"ppm,omitempty"`
}

func (r *SetChannelFeeRequest) Name() string {
	return "setchannelfee"
}

// Set the channel fee for a given channel. 'id' can be a peer id, a channel id,
// a short channel id, or all, for all channels.
func (l *Lightning) SetChannelFee(id string, baseMsat string, ppm uint32) (*ChannelFeeResult, error) {
	var result ChannelFeeResult
	err := l.client.Request(&SetChannelFeeRequest{id, baseMsat, ppm}, &result)
	return &result, err
}

type PluginInfo struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

type PluginRequest struct {
	Subcommand string `json:"subcommand"`
}

type pluginResponse struct {
	Plugins []PluginInfo `json:"plugins"`
}

type stopPluginResponse struct {
	Result string `json:"result"`
}

func (r *PluginRequest) Name() string {
	return "plugin"
}

func (l *Lightning) ListPlugins() ([]PluginInfo, error) {
	var result pluginResponse
	err := l.client.Request(&PluginRequest{"list"}, &result)
	return result.Plugins, err
}

func (l *Lightning) RescanPlugins() ([]PluginInfo, error) {
	var result pluginResponse
	err := l.client.Request(&PluginRequest{"rescan"}, &result)
	return result.Plugins, err
}

type PluginRequestDir struct {
	Subcommand string `json:"subcommand"`
	Directory  string `json:"directory"`
}

func (r *PluginRequestDir) Name() string {
	return "plugin"
}

func (l *Lightning) SetPluginStartDir(directory string) ([]PluginInfo, error) {
	var result pluginResponse
	err := l.client.Request(&PluginRequestDir{"start-dir", directory}, &result)
	return result.Plugins, err
}

type PluginRequestPlugin struct {
	Subcommand string `json:"subcommand"`
	PluginName string `json:"plugin"`
}

func (r *PluginRequestPlugin) Name() string {
	return "plugin"
}

func (l *Lightning) StartPlugin(pluginName string) ([]PluginInfo, error) {
	var result pluginResponse
	err := l.client.Request(&PluginRequestPlugin{"start", pluginName}, &result)
	return result.Plugins, err
}

func (l *Lightning) StopPlugin(pluginName string) (string, error) {
	var result stopPluginResponse
	err := l.client.Request(&PluginRequestPlugin{"stop", pluginName}, &result)
	return result.Result, err
}

type SharedSecretRequest struct {
	Point string `json:"point"`
}

func (r *SharedSecretRequest) Name() string {
	return "getsharedsecret"
}

type SharedSecretResp struct {
	SharedSecret string `json:"shared_secret"`
}

/* Returns the shared secret, a hexadecimal string of the 256-bit SHA-2 of the
   compressed public key DER-encoding of the  SECP256K1  point  that  is  the
   shared secret generated using the Elliptic Curve Diffie-Hellman algorithm.
   This field is 32 bytes (64 hexadecimal characters in a string). */
func (l *Lightning) GetSharedSecret(point string) (string, error) {
	var result SharedSecretResp
	err := l.client.Request(&SharedSecretRequest{point}, &result)
	return result.SharedSecret, err
}

// List of all non-dev RPC methods
var Lightning_RpcMethods map[string](func() jrpc2.Method)

// we register all of the methods here, so the rpc command
// hook in the plugin works as expected
// FIXME: have this registry be generated dynamically
//        at build
func init() {
	Lightning_RpcMethods = make(map[string]func() jrpc2.Method)

	Lightning_RpcMethods[(&ListConfigsRequest{}).Name()] = func() jrpc2.Method { return new(ListConfigsRequest) }
	Lightning_RpcMethods[(&ListPeersRequest{}).Name()] = func() jrpc2.Method { return new(ListPeersRequest) }
	Lightning_RpcMethods[(&ListNodeRequest{}).Name()] = func() jrpc2.Method { return new(ListNodeRequest) }
	Lightning_RpcMethods[(&RouteRequest{}).Name()] = func() jrpc2.Method { return new(RouteRequest) }
	Lightning_RpcMethods[(&SendOnionMessageRequest{}).Name()] = func() jrpc2.Method { return new(SendOnionMessageRequest) }
	Lightning_RpcMethods[(&SendOnionRequest{}).Name()] = func() jrpc2.Method { return new(SendOnionRequest) }
	Lightning_RpcMethods[(&CreateOnionRequest{}).Name()] = func() jrpc2.Method { return new(CreateOnionRequest) }
	Lightning_RpcMethods[(&ListChannelRequest{}).Name()] = func() jrpc2.Method { return new(ListChannelRequest) }
	Lightning_RpcMethods[(&InvoiceRequest{}).Name()] = func() jrpc2.Method { return new(InvoiceRequest) }
	Lightning_RpcMethods[(&ListInvoiceRequest{}).Name()] = func() jrpc2.Method { return new(ListInvoiceRequest) }
	Lightning_RpcMethods[(&DeleteInvoiceRequest{}).Name()] = func() jrpc2.Method { return new(DeleteInvoiceRequest) }
	Lightning_RpcMethods[(&WaitAnyInvoiceRequest{}).Name()] = func() jrpc2.Method { return new(WaitAnyInvoiceRequest) }
	Lightning_RpcMethods[(&WaitInvoiceRequest{}).Name()] = func() jrpc2.Method { return new(WaitInvoiceRequest) }
	Lightning_RpcMethods[(&DeleteExpiredInvoiceReq{}).Name()] = func() jrpc2.Method { return new(DeleteExpiredInvoiceReq) }
	Lightning_RpcMethods[(&AutoCleanInvoiceRequest{}).Name()] = func() jrpc2.Method { return new(AutoCleanInvoiceRequest) }
	Lightning_RpcMethods[(&DecodePayRequest{}).Name()] = func() jrpc2.Method { return new(DecodePayRequest) }
	Lightning_RpcMethods[(&PayStatusRequest{}).Name()] = func() jrpc2.Method { return new(PayStatusRequest) }
	Lightning_RpcMethods[(&HelpRequest{}).Name()] = func() jrpc2.Method { return new(HelpRequest) }
	Lightning_RpcMethods[(&StopRequest{}).Name()] = func() jrpc2.Method { return new(StopRequest) }
	Lightning_RpcMethods[(&LogRequest{}).Name()] = func() jrpc2.Method { return new(LogRequest) }

	// we skip all the Dev-commands

	Lightning_RpcMethods[(&GetInfoRequest{}).Name()] = func() jrpc2.Method { return new(GetInfoRequest) }
	Lightning_RpcMethods[(&SignMessageRequest{}).Name()] = func() jrpc2.Method { return new(SignMessageRequest) }
	Lightning_RpcMethods[(&CheckMessageRequest{}).Name()] = func() jrpc2.Method { return new(CheckMessageRequest) }
	Lightning_RpcMethods[(&SendPayRequest{}).Name()] = func() jrpc2.Method { return new(SendPayRequest) }
	Lightning_RpcMethods[(&WaitSendPayRequest{}).Name()] = func() jrpc2.Method { return new(WaitSendPayRequest) }
	Lightning_RpcMethods[(&PayRequest{}).Name()] = func() jrpc2.Method { return new(PayRequest) }
	Lightning_RpcMethods[(&ListPaysRequest{}).Name()] = func() jrpc2.Method { return new(ListPaysRequest) }
	Lightning_RpcMethods[(&ListSendPaysRequest{}).Name()] = func() jrpc2.Method { return new(ListSendPaysRequest) }
	Lightning_RpcMethods[(&TransactionsRequest{}).Name()] = func() jrpc2.Method { return new(TransactionsRequest) }
	Lightning_RpcMethods[(&ConnectRequest{}).Name()] = func() jrpc2.Method { return new(ConnectRequest) }
	Lightning_RpcMethods[(&FundChannelRequest{}).Name()] = func() jrpc2.Method { return new(FundChannelRequest) }
	Lightning_RpcMethods[(&FundChannelStart{}).Name()] = func() jrpc2.Method { return new(FundChannelStart) }
	Lightning_RpcMethods[(&FundChannelComplete{}).Name()] = func() jrpc2.Method { return new(FundChannelComplete) }
	Lightning_RpcMethods[(&FundChannelCancel{}).Name()] = func() jrpc2.Method { return new(FundChannelCancel) }
	Lightning_RpcMethods[(&CloseRequest{}).Name()] = func() jrpc2.Method { return new(CloseRequest) }
	Lightning_RpcMethods[(&PingRequest{}).Name()] = func() jrpc2.Method { return new(PingRequest) }
	Lightning_RpcMethods[(&WithdrawRequest{}).Name()] = func() jrpc2.Method { return new(WithdrawRequest) }
	Lightning_RpcMethods[(&NewAddrRequest{}).Name()] = func() jrpc2.Method { return new(NewAddrRequest) }
	Lightning_RpcMethods[(&TxPrepare{}).Name()] = func() jrpc2.Method { return new(TxPrepare) }
	Lightning_RpcMethods[(&TxDiscard{}).Name()] = func() jrpc2.Method { return new(TxDiscard) }
	Lightning_RpcMethods[(&TxSend{}).Name()] = func() jrpc2.Method { return new(TxSend) }
	Lightning_RpcMethods[(&ListFundsRequest{}).Name()] = func() jrpc2.Method { return new(ListFundsRequest) }
	Lightning_RpcMethods[(&ListForwardsRequest{}).Name()] = func() jrpc2.Method { return new(ListForwardsRequest) }
	Lightning_RpcMethods[(&DisconnectRequest{}).Name()] = func() jrpc2.Method { return new(DisconnectRequest) }
	Lightning_RpcMethods[(&FeeRatesRequest{}).Name()] = func() jrpc2.Method { return new(FeeRatesRequest) }
	Lightning_RpcMethods[(&SetChannelFeeRequest{}).Name()] = func() jrpc2.Method { return new(SetChannelFeeRequest) }
	Lightning_RpcMethods[(&PluginRequest{}).Name()] = func() jrpc2.Method { return new(PluginRequest) }
	Lightning_RpcMethods[(&SharedSecretRequest{}).Name()] = func() jrpc2.Method { return new(SharedSecretRequest) }
}
