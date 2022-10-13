package clightning

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/elementsproject/peerswap/log"
	"github.com/elementsproject/peerswap/peerswaprpc"

	"github.com/elementsproject/glightning/glightning"
	"github.com/elementsproject/glightning/jrpc2"
	"github.com/elementsproject/peerswap/policy"
	"github.com/elementsproject/peerswap/swap"
)

type SwapCanceledError string

func (e SwapCanceledError) Error() string {
	return fmt.Sprintf("swap canceled, reason: %s", string(e))
}

// LiquidGetAddress returns a new liquid address
type LiquidGetAddress struct {
	cl *ClightningClient `json:"-"`
}

func (g *LiquidGetAddress) New() interface{} {
	return &LiquidGetAddress{
		cl: g.cl,
	}
}

func (g *LiquidGetAddress) Name() string {
	return "peerswap-lbtc-getaddress"
}

func (g *LiquidGetAddress) Call() (jrpc2.Result, error) {
	if !g.cl.isReady {
		return nil, ErrWaitingForReady
	}

	if g.cl.liquidWallet == nil {
		return nil, errors.New("liquid swaps are not enabled")
	}
	res, err := g.cl.liquidWallet.GetAddress()
	if err != nil {
		return nil, err
	}
	log.Infof("[Wallet] Getting lbtc address %s", res)
	return &GetAddressResponse{LiquidAddress: res}, nil
}

func (g *LiquidGetAddress) Description() string {
	return "Returns a new liquid address of the liquid peerswap wallet."
}

func (g *LiquidGetAddress) LongDescription() string {
	return ""
}

func (g *LiquidGetAddress) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &LiquidGetAddress{
		cl: client,
	}
}

type GetAddressResponse struct {
	LiquidAddress string `json:"lbtc_address"`
}

// GetBalance returns the liquid balance
type LiquidGetBalance struct {
	cl *ClightningClient
}

func (g *LiquidGetBalance) Name() string {
	return "peerswap-lbtc-getbalance"
}

func (g *LiquidGetBalance) New() interface{} {
	return &LiquidGetBalance{
		cl: g.cl,
	}
}

func (g *LiquidGetBalance) Call() (jrpc2.Result, error) {
	if !g.cl.isReady {
		return nil, ErrWaitingForReady
	}

	if g.cl.liquidWallet == nil {
		return nil, errors.New("lbtc swaps are not enabled")
	}
	res, err := g.cl.liquidWallet.GetBalance()
	if err != nil {
		return nil, err
	}
	return &GetBalanceResponse{
		res,
	}, nil
}

func (g *LiquidGetBalance) Description() string {
	return "Returns the liquid balance"
}

func (g *LiquidGetBalance) LongDescription() string {
	return ""
}

func (g *LiquidGetBalance) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &LiquidGetBalance{
		cl: client,
	}
}

type GetBalanceResponse struct {
	LiquidBalance uint64 `json:"lbtc_balance_sat"`
}

// LiquidSendToAddress sends
type LiquidSendToAddress struct {
	Address   string `json:"address"`
	AmountSat uint64 `json:"amount_sat"`
	cl        *ClightningClient
}

func (s *LiquidSendToAddress) Name() string {
	return "peerswap-lbtc-sendtoaddress"
}

func (s *LiquidSendToAddress) New() interface{} {
	return &LiquidSendToAddress{
		cl: s.cl,
	}
}

func (s *LiquidSendToAddress) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &LiquidSendToAddress{
		cl: client,
	}
}

func (s *LiquidSendToAddress) Call() (jrpc2.Result, error) {
	if !s.cl.isReady {
		return nil, ErrWaitingForReady
	}

	if s.cl.liquidWallet == nil {
		return nil, errors.New("lbtc swaps are not enabled")
	}
	if s.Address == "" {
		return nil, errors.New("address must be set")
	}
	if s.AmountSat == 0 {
		return nil, errors.New("amount_sat must be set")
	}
	res, err := s.cl.liquidWallet.SendToAddress(s.Address, s.AmountSat)
	if err != nil {
		log.Infof("Error sending to address %v", err)
		return nil, err
	}
	return &SendToAddressResponse{TxId: res}, nil
}

type SendToAddressResponse struct {
	TxId string `json:"txid"`
}

func (s *LiquidSendToAddress) Description() string {
	return "sends lbtc to an address"
}

func (s *LiquidSendToAddress) LongDescription() string {
	return "'"
}

// SwapOut starts a new swapout (paying an Invoice for onchain liquidity)
type SwapOut struct {
	SatAmt         uint64            `json:"amt_sat"`
	ShortChannelId string            `json:"short_channel_id"`
	Asset          string            `json:"asset"`
	Force          bool              `json:"force"`
	cl             *ClightningClient `json:"-"`
}

func (l *SwapOut) New() interface{} {
	return &SwapOut{
		cl: l.cl,
	}
}

func (l *SwapOut) Name() string {
	return "peerswap-swap-out"
}

func (l *SwapOut) Call() (jrpc2.Result, error) {
	if !l.cl.isReady {
		return nil, ErrWaitingForReady
	}

	if l.SatAmt <= 0 {
		return nil, errors.New("Missing required amt_sat parameter")
	}
	if l.ShortChannelId == "" {
		return nil, errors.New("Missing required short_channel_id parameter")
	}

	funds, err := l.cl.glightning.ListFunds()
	if err != nil {
		return nil, err
	}
	var fundingChannels *glightning.FundingChannel
	for _, v := range funds.Channels {
		if v.ShortChannelId == l.ShortChannelId {
			fundingChannels = v
			break
		}
	}
	if fundingChannels == nil {
		return nil, errors.New("fundingChannels not found")
	}

	if fundingChannels.ChannelSatoshi < (l.SatAmt + 5000) {
		return nil, errors.New("not enough outbound capacity to perform swapOut")
	}
	if !fundingChannels.Connected {
		return nil, errors.New("fundingChannels is not connected")
	}

	// Skip this check when `force` is set.
	if !l.Force && !l.cl.peerRunsPeerSwap(fundingChannels.Id) {
		return nil, fmt.Errorf("peer does not run peerswap")
	}

	if !l.cl.isPeerConnected(fundingChannels.Id) {
		return nil, fmt.Errorf("peer is not connected")
	}

	if strings.Compare(l.Asset, "lbtc") == 0 {
		if !l.cl.swaps.LiquidEnabled {
			return nil, errors.New("liquid swaps are not enabled")
		}
		if l.cl.Gelements == nil {
			return nil, errors.New("peerswap was not started with liquid node config")
		}

	} else if strings.Compare(l.Asset, "btc") == 0 {
		if !l.cl.swaps.BitcoinEnabled {
			return nil, errors.New("bitcoin swaps are not enabled")
		}
	} else {
		return nil, errors.New("invalid asset (btc or lbtc)")
	}

	pk := l.cl.GetNodeId()
	swapOut, err := l.cl.swaps.SwapOut(fundingChannels.Id, l.Asset, l.ShortChannelId, pk, l.SatAmt)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return nil, errors.New("rpc timeout reached, use peerswap-listswaps for info")
		default:
			if swapOut.Current == swap.State_SwapOutSender_AwaitAgreement || swapOut.Current == swap.State_SwapOutSender_PayFeeInvoice || swapOut.Current == swap.State_SwapOutSender_AwaitTxBroadcastedMessage {
				continue
			}
			if swapOut.Current == swap.State_SwapCanceled {
				return nil, SwapCanceledError(swapOut.Data.GetCancelMessage())

			}
			if swapOut.Current == swap.State_SwapOutSender_AwaitTxConfirmation {
				return peerswaprpc.PrettyprintFromServiceSwap(swapOut), nil
			}
		}
	}
}

func (l *SwapOut) Description() string {
	return "Initiates a swap out with a peer"
}

func (l *SwapOut) LongDescription() string {
	return ""
}

func (g *SwapOut) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &SwapOut{
		cl: client,
	}
}

// SwapIn Starts a new swap in(providing onchain liquidity)
type SwapIn struct {
	SatAmt         uint64 `json:"amt_sat"`
	ShortChannelId string `json:"short_channel_id"`
	Asset          string `json:"asset"`
	Force          bool   `json:"force"`

	cl *ClightningClient `json:"-"`
}

func (l *SwapIn) New() interface{} {
	return &SwapIn{
		cl: l.cl,
	}
}

func (l *SwapIn) Name() string {
	return "peerswap-swap-in"
}

func (l *SwapIn) Call() (jrpc2.Result, error) {
	if !l.cl.isReady {
		return nil, ErrWaitingForReady
	}

	if l.SatAmt <= 0 {
		return nil, errors.New("Missing required amt_sat parameter")
	}

	if l.ShortChannelId == "" {
		return nil, errors.New("Missing required short_channel_id parameter")
	}

	funds, err := l.cl.glightning.ListFunds()
	if err != nil {
		return nil, err
	}
	var fundingChannels *glightning.FundingChannel
	for _, v := range funds.Channels {
		if v.ShortChannelId == l.ShortChannelId {
			fundingChannels = v
			break
		}
	}
	if fundingChannels == nil {
		return nil, errors.New("fundingChannels not found")
	}
	if fundingChannels.ChannelTotalSatoshi-fundingChannels.ChannelSatoshi < l.SatAmt {
		return nil, errors.New("not enough inbound capacity to perform swap")
	}
	if !fundingChannels.Connected {
		return nil, errors.New("fundingChannels is not connected")
	}

	// Skip this check when `force` is set.
	if !l.Force && !l.cl.peerRunsPeerSwap(fundingChannels.Id) {
		return nil, fmt.Errorf("peer does not run peerswap")
	}

	if !l.cl.isPeerConnected(fundingChannels.Id) {
		return nil, fmt.Errorf("peer is not connected")
	}
	if l.Asset == "lbtc" {
		if !l.cl.swaps.LiquidEnabled {
			return nil, errors.New("liquid swaps are not enabled")
		}
		if l.cl.Gelements == nil {
			return nil, errors.New("peerswap was not started with liquid node config")
		}
		liquidBalance, err := l.cl.liquidWallet.GetBalance()
		if err != nil {
			return nil, err
		}
		if liquidBalance < l.SatAmt {
			return nil, errors.New("Not enough balance on liquid liquidWallet")
		}
	} else if l.Asset == "btc" {
		if !l.cl.swaps.BitcoinEnabled {
			return nil, errors.New("bitcoin swaps are not enabled")
		}
		funds, err := l.cl.glightning.ListFunds()
		if err != nil {
			return nil, err
		}
		sats := uint64(0)
		for _, v := range funds.Outputs {
			sats += v.Value
		}

		if sats < l.SatAmt+2000 {
			return nil, errors.New("Not enough balance on c-lightning onchain liquidWallet")
		}
	} else {
		return nil, errors.New("invalid asset (btc or lbtc)")
	}

	pk := l.cl.GetNodeId()
	swapIn, err := l.cl.swaps.SwapIn(fundingChannels.Id, l.Asset, l.ShortChannelId, pk, l.SatAmt)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return nil, errors.New("rpc timeout reached, use peerswap-listswaps for info")
		default:
			if swapIn.Current == swap.State_SwapInSender_AwaitAgreement || swapIn.Current == swap.State_SwapInSender_BroadcastOpeningTx {
				continue
			}
			if swapIn.Current == swap.State_SwapCanceled {
				return nil, SwapCanceledError(swapIn.Data.GetCancelMessage())

			}
			if swapIn.Current == swap.State_SwapInSender_SendTxBroadcastedMessage {
				return peerswaprpc.PrettyprintFromServiceSwap(swapIn), nil
			}
		}
	}
}

func (l *SwapIn) Description() string {
	return "Initiates a swap in with a peer"
}

func (l *SwapIn) LongDescription() string {
	return ""
}

func (g *SwapIn) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &SwapIn{
		cl: client,
	}
}

// ListSwaps list all active and finished swaps
type ListSwaps struct {
	DetailedPrint bool              `json:"detailed,omitempty"`
	cl            *ClightningClient `json:"-"`
}

func (l *ListSwaps) New() interface{} {
	return &ListSwaps{
		cl: l.cl,
	}
}

func (l *ListSwaps) Name() string {
	return "peerswap-listswaps"
}

func (l *ListSwaps) Call() (jrpc2.Result, error) {
	if !l.cl.isReady {
		return nil, ErrWaitingForReady
	}

	swaps, err := l.cl.swaps.ListSwaps()
	if err != nil {
		return nil, err
	}
	sort.Slice(swaps, func(i, j int) bool {
		if swaps[i].Data != nil && swaps[j].Data != nil {
			return swaps[i].Data.CreatedAt < swaps[j].Data.CreatedAt
		}
		return false
	})
	if !l.DetailedPrint {
		var pretty []*peerswaprpc.PrettyPrintSwap
		for _, v := range swaps {
			pretty = append(pretty, peerswaprpc.PrettyprintFromServiceSwap(v))
		}
		return &peerswaprpc.ListSwapsResponse{Swaps: pretty}, nil
	}
	return swaps, nil
}

func (l *ListSwaps) Description() string {
	return "Returns a list of historical swaps."
}

func (l *ListSwaps) LongDescription() string {
	return ""
}

func (g *ListSwaps) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &ListSwaps{
		cl: client,
	}
}

type ListNodes struct {
	cl *ClightningClient
}

func (l *ListNodes) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &ListNodes{cl: client}
}

func (l *ListNodes) Description() string {
	return "lists nodes that support the peerswap Plugin"
}

func (l *ListNodes) LongDescription() string {
	return ""
}

func (l *ListNodes) New() interface{} {
	return &ListNodes{
		cl: l.cl,
	}
}

func (l *ListNodes) Name() string {
	return "peerswap-listnodes"
}

func (l *ListNodes) Call() (jrpc2.Result, error) {
	getInfo, err := l.cl.glightning.GetInfo()
	if err != nil {
		return nil, err
	}

	nodes, err := l.cl.glightning.ListNodes()
	if err != nil {
		return nil, err
	}

	peerSwapNodes := []*glightning.Node{}
	for _, node := range nodes {
		if node.Features != nil && checkFeatures(node.Features.Raw, featureBit) && node.Id != getInfo.Id {
			peerSwapNodes = append(peerSwapNodes, node)
		}
	}

	return peerSwapNodes, nil
}

type PeerSwapNodes struct {
	Nodes []*glightning.Node `json:"nodes"`
}

// ListPeers lists all peerswap-ready peers
type ListPeers struct {
	cl *ClightningClient
}

func (l *ListPeers) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &ListPeers{cl: client}
}

func (l *ListPeers) Description() string {
	return "lists peers supporting the peerswap Plugin"
}

func (l *ListPeers) LongDescription() string {
	return ""
}

func (l *ListPeers) New() interface{} {
	return &ListPeers{
		cl: l.cl,
	}
}

func (l *ListPeers) Name() string {
	return "peerswap-listpeers"
}

func (l *ListPeers) Call() (jrpc2.Result, error) {
	if !l.cl.isReady {
		return nil, ErrWaitingForReady
	}

	peers, err := l.cl.glightning.ListPeers()
	if err != nil {
		return nil, err
	}

	// cache all channels for later search
	fundingChannels := make(map[string]*glightning.FundingChannel)
	funds, err := l.cl.glightning.ListFunds()
	if err != nil {
		return nil, err
	}
	for _, channel := range funds.Channels {
		fundingChannels[channel.ShortChannelId] = channel
	}

	// get polls
	polls, err := l.cl.pollService.GetPolls()
	if err != nil {
		return nil, err
	}

	peerSwappers := []*PeerSwapPeer{}
	for _, peer := range peers {
		if p, ok := polls[peer.Id]; ok {
			swaps, err := l.cl.swaps.ListSwapsByPeer(peer.Id)
			if err != nil {
				return nil, err
			}

			var paidFees uint64
			var ReceiverSwapsOut, ReceiverSwapsIn, ReceiverSatsOut, ReceiverSatsIn uint64
			var SenderSwapsOut, SenderSwapsIn, SenderSatsOut, SenderSatsIn uint64
			for _, s := range swaps {
				// We only list successful swaps. They all end in an
				// State_ClaimedPreimage state.
				if s.Current == swap.State_ClaimedPreimage {
					if s.Role == swap.SWAPROLE_SENDER {
						paidFees += s.Data.OpeningTxFee
						if s.Type == swap.SWAPTYPE_OUT {
							SenderSwapsOut++
							SenderSatsOut += s.Data.GetAmount()
						} else {
							SenderSwapsIn++
							SenderSatsIn += s.Data.GetAmount()
						}
					} else {
						if s.Type == swap.SWAPTYPE_OUT {
							ReceiverSwapsOut++
							ReceiverSatsOut += s.Data.GetAmount()
						} else {
							ReceiverSwapsIn++
							ReceiverSatsIn += s.Data.GetAmount()
						}
					}
				}
			}

			peerSwapPeer := &PeerSwapPeer{
				NodeId:          peer.Id,
				SwapsAllowed:    p.PeerAllowed,
				SupportedAssets: p.Assets,
				AsSender: &SwapStats{
					SwapsOut: SenderSwapsOut,
					SwapsIn:  SenderSwapsIn,
					SatsOut:  SenderSatsOut,
					SatsIn:   SenderSatsIn,
				},
				AsReceiver: &SwapStats{
					SwapsOut: ReceiverSwapsOut,
					SwapsIn:  ReceiverSwapsIn,
					SatsOut:  ReceiverSatsOut,
					SatsIn:   ReceiverSatsIn,
				},
				PaidFee: paidFees,
			}

			peerSwapPeerChannels := []*PeerSwapPeerChannel{}
			for _, channel := range peer.Channels {
				if c, ok := fundingChannels[channel.ShortChannelId]; ok {
					peerSwapPeerChannels = append(peerSwapPeerChannels, &PeerSwapPeerChannel{
						ChannelId:       c.ShortChannelId,
						LocalBalance:    c.ChannelSatoshi,
						RemoteBalance:   uint64(c.ChannelTotalSatoshi - c.ChannelSatoshi),
						LocalPercentage: float64(c.ChannelSatoshi) / float64(c.ChannelTotalSatoshi),
						State:           c.State,
					})
				}
			}

			peerSwapPeer.Channels = peerSwapPeerChannels
			peerSwappers = append(peerSwappers, peerSwapPeer)
		}
	}
	return peerSwappers, nil
}

type GetSwap struct {
	SwapId string `json:"swap_id"`
	cl     *ClightningClient
}

func (g *GetSwap) Name() string {
	return "peerswap-getswap"
}

func (g *GetSwap) New() interface{} {
	return &GetSwap{
		cl:     g.cl,
		SwapId: g.SwapId,
	}
}

func (g *GetSwap) Call() (jrpc2.Result, error) {
	if !g.cl.isReady {
		return nil, ErrWaitingForReady
	}

	if g.SwapId == "" {
		return nil, errors.New("swap_id required")
	}
	swap, err := g.cl.swaps.GetSwap(g.SwapId)
	if err != nil {
		return nil, err
	}
	return MSerializedSwapStateMachine(swap), nil
}

func (g *GetSwap) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &GetSwap{
		cl: client,
	}
}

func (g *GetSwap) Description() string {
	return "returns swap by swapid"
}

func (g *GetSwap) LongDescription() string {
	return ""
}

type PolicyReloader interface {
	AddToAllowlist(pubkey string) error
	RemoveFromAllowlist(pubkey string) error
	AddToSuspiciousPeerList(pubkey string) error
	RemoveFromSuspiciousPeerList(pubkey string) error
	NewSwapsAllowed() bool
	DisableSwaps() error
	EnableSwaps() error
	ReloadFile() error
	Get() policy.Policy
}
type ReloadPolicyFile struct {
	cl *ClightningClient
}

func (c ReloadPolicyFile) Name() string {
	return "peerswap-reloadpolicy"
}

func (c ReloadPolicyFile) New() interface{} {
	return c
}

func (c ReloadPolicyFile) Call() (jrpc2.Result, error) {
	if !c.cl.isReady {
		return nil, ErrWaitingForReady
	}

	log.Debugf("reloading policy %v", c.cl.policy)
	err := c.cl.policy.ReloadFile()
	if err != nil {
		return nil, err
	}
	// Resend poll
	c.cl.pollService.PollAllPeers()
	return c.cl.policy.Get(), nil
}

func (c ReloadPolicyFile) Description() string {
	return "Reload the policy file."
}

func (c *ReloadPolicyFile) LongDescription() string {
	return `If the policy file has changed, reload the policy
	from the file specified in the config. Overrides the
	default config, so fields that are not set are interpreted as default.`
}

func (c *ReloadPolicyFile) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &ReloadPolicyFile{
		cl: client,
	}
}

type GetRequestedSwaps struct {
	cl *ClightningClient
}

func (c GetRequestedSwaps) Name() string {
	return "peerswap-listswaprequests"
}

func (c GetRequestedSwaps) New() interface{} {
	return c
}

func (c GetRequestedSwaps) Call() (jrpc2.Result, error) {
	if !c.cl.isReady {
		return nil, ErrWaitingForReady
	}

	requestedSwaps, err := c.cl.requestedSwaps.Get()
	if err != nil {
		return nil, err
	}
	return requestedSwaps, nil
}

func (c GetRequestedSwaps) Description() string {
	return "Returns unhandled swaps requested by peer nodes."
}

func (c GetRequestedSwaps) LongDescription() string {
	return `This command can give you insight of swaps requested by peer nodes that could not have
		been performed because either the peer is not in the allowlist or the asset is not set.`
}

func (c *GetRequestedSwaps) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &GetRequestedSwaps{
		cl: client,
	}
}

type ListActiveSwaps struct {
	cl *ClightningClient
}

func (g *ListActiveSwaps) Name() string {
	return "peerswap-listactiveswaps"
}

func (g *ListActiveSwaps) New() interface{} {
	return &ListActiveSwaps{
		cl: g.cl,
	}
}

func (g *ListActiveSwaps) Call() (jrpc2.Result, error) {
	if !g.cl.isReady {
		return nil, ErrWaitingForReady
	}

	swaps, err := g.cl.swaps.ListActiveSwaps()
	if err != nil {
		return nil, err
	}

	var pretty []*peerswaprpc.PrettyPrintSwap
	for _, v := range swaps {
		pretty = append(pretty, peerswaprpc.PrettyprintFromServiceSwap(v))
	}
	return &peerswaprpc.ListSwapsResponse{Swaps: pretty}, nil
}

func (g *ListActiveSwaps) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &ListActiveSwaps{
		cl: client,
	}
}

func (c ListActiveSwaps) Description() string {
	return "Returns active swaps."
}

func (c ListActiveSwaps) LongDescription() string {
	return `This command can give you information if you are ready to upgrade your peerswap daemon.`
}

type AllowSwapRequests struct {
	AllowSwapRequestsString string `json:"allow_swap_requests"`

	cl *ClightningClient
}

func (g *AllowSwapRequests) Name() string {
	return "peerswap-allowswaprequests"
}

func (g *AllowSwapRequests) New() interface{} {
	return &AllowSwapRequests{
		cl:                      g.cl,
		AllowSwapRequestsString: g.AllowSwapRequestsString,
	}
}

func (g *AllowSwapRequests) Call() (jrpc2.Result, error) {
	if !g.cl.isReady {
		return nil, ErrWaitingForReady
	}

	if g.AllowSwapRequestsString == "" {
		return nil, fmt.Errorf("missing argument:1 to allow, 0 to disallow")
	}

	if g.AllowSwapRequestsString == "1" || strings.ToLower(g.AllowSwapRequestsString) == "true" {
		g.cl.policy.EnableSwaps()
	} else if g.AllowSwapRequestsString == "0" || strings.ToLower(g.AllowSwapRequestsString) == "false" {
		g.cl.policy.DisableSwaps()
	}

	pol := g.cl.policy.Get()
	return peerswaprpc.GetPolicyMessage(pol), nil
}

func (g *AllowSwapRequests) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &AllowSwapRequests{
		cl:                      client,
		AllowSwapRequestsString: g.AllowSwapRequestsString,
	}
}

func (c AllowSwapRequests) Description() string {
	return "Sets peerswap to allow incoming swap requests"
}

func (c AllowSwapRequests) LongDescription() string {
	return `This command can be used to wait for all swaps to complete, while not allowing new swaps. This is helps with upgrading`
}

type AddPeer struct {
	PeerPubkey string `json:"peer_pubkey"`
	cl         *ClightningClient
}

func (g *AddPeer) Name() string {
	return "peerswap-addpeer"
}

func (g *AddPeer) New() interface{} {
	return &AddPeer{
		cl:         g.cl,
		PeerPubkey: g.PeerPubkey,
	}
}

func (g *AddPeer) Call() (jrpc2.Result, error) {
	if !g.cl.isReady {
		return nil, ErrWaitingForReady
	}

	err := g.cl.policy.AddToAllowlist(g.PeerPubkey)
	if err != nil {
		return nil, err
	}
	return g.cl.policy.Get(), nil
}

func (g *AddPeer) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &AddPeer{
		cl:         client,
		PeerPubkey: g.PeerPubkey,
	}
}

func (c AddPeer) Description() string {
	return "Add peer to allowlist"
}

func (c AddPeer) LongDescription() string {
	return `This command can be used to add a peer to the allowlist`
}

type RemovePeer struct {
	PeerPubkey string `json:"peer_pubkey"`
	cl         *ClightningClient
}

func (g *RemovePeer) Name() string {
	return "peerswap-removepeer"
}

func (g *RemovePeer) New() interface{} {
	return &RemovePeer{
		cl:         g.cl,
		PeerPubkey: g.PeerPubkey,
	}
}

func (g *RemovePeer) Call() (jrpc2.Result, error) {
	if !g.cl.isReady {
		return nil, ErrWaitingForReady
	}

	err := g.cl.policy.RemoveFromAllowlist(g.PeerPubkey)
	if err != nil {
		return nil, err
	}
	return g.cl.policy.Get(), nil
}

func (g *RemovePeer) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &RemovePeer{
		cl:         client,
		PeerPubkey: g.PeerPubkey,
	}
}

func (c RemovePeer) Description() string {
	return "Remove peer from allowlist"
}

func (c RemovePeer) LongDescription() string {
	return `This command can be used to remove a peer from the allowlist`
}

type AddSuspiciousPeer struct {
	PeerPubkey string `json:"peer_pubkey"`
	cl         *ClightningClient
}

func (g *AddSuspiciousPeer) Name() string {
	return "peerswap-addsuspeer"
}

func (g *AddSuspiciousPeer) New() interface{} {
	return &AddSuspiciousPeer{
		cl:         g.cl,
		PeerPubkey: g.PeerPubkey,
	}
}

func (g *AddSuspiciousPeer) Call() (jrpc2.Result, error) {
	if !g.cl.isReady {
		return nil, ErrWaitingForReady
	}

	err := g.cl.policy.AddToSuspiciousPeerList(g.PeerPubkey)
	if err != nil {
		return nil, err
	}
	return g.cl.policy.Get(), nil
}

func (g *AddSuspiciousPeer) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &AddSuspiciousPeer{
		cl:         client,
		PeerPubkey: g.PeerPubkey,
	}
}

func (c AddSuspiciousPeer) Description() string {
	return "Add peer to suspicious peer list"
}

func (c AddSuspiciousPeer) LongDescription() string {
	return `This command can be used to add a peer to the list of suspicious` +
		`peers. Peers on this list are not allowed to request swaps with this node`
}

type RemoveSuspiciousPeer struct {
	PeerPubkey string `json:"peer_pubkey"`
	cl         *ClightningClient
}

func (g *RemoveSuspiciousPeer) Name() string {
	return "peerswap-removesuspeer"
}

func (g *RemoveSuspiciousPeer) New() interface{} {
	return &RemoveSuspiciousPeer{
		cl:         g.cl,
		PeerPubkey: g.PeerPubkey,
	}
}

func (g *RemoveSuspiciousPeer) Call() (jrpc2.Result, error) {
	if !g.cl.isReady {
		return nil, ErrWaitingForReady
	}

	err := g.cl.policy.RemoveFromSuspiciousPeerList(g.PeerPubkey)
	if err != nil {
		return nil, err
	}
	return g.cl.policy.Get(), nil
}

func (g *RemoveSuspiciousPeer) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &RemoveSuspiciousPeer{
		cl:         client,
		PeerPubkey: g.PeerPubkey,
	}
}

func (c RemoveSuspiciousPeer) Description() string {
	return "Remove peer from suspicious peer list"
}

func (c RemoveSuspiciousPeer) LongDescription() string {
	return `This command can be used to remove  a peer to the list of` +
		`suspicious peers. Peers on this list are not allowed to request swaps` +
		`with this node`
}

type PeerSwapPeerChannel struct {
	ChannelId       string  `json:"short_channel_id"`
	LocalBalance    uint64  `json:"local_balance"`
	RemoteBalance   uint64  `json:"remote_balance"`
	LocalPercentage float64 `json:"local_percentage"`
	State           string  `json:"state"`
}

type SwapStats struct {
	SwapsOut uint64 `json:"total_swaps_out"`
	SwapsIn  uint64 `json:"total_swaps_in"`
	SatsOut  uint64 `json:"total_sats_swapped_out"`
	SatsIn   uint64 `json:"total_sats_swapped_in"`
}

type PeerSwapPeer struct {
	NodeId          string                 `json:"nodeid"`
	SwapsAllowed    bool                   `json:"swaps_allowed"`
	SupportedAssets []string               `json:"supported_assets"`
	Channels        []*PeerSwapPeerChannel `json:"channels"`
	AsSender        *SwapStats             `json:"sent,omitempty"`
	AsReceiver      *SwapStats             `json:"received,omitempty"`
	PaidFee         uint64                 `json:"total_fee_paid"`
}

// checkFeatures checks if a node runs the peerswap Plugin
func checkFeatures(features []byte, featureBit int64) bool {
	featuresInt := big.NewInt(0)
	featuresInt = featuresInt.SetBytes(features)
	bitInt := big.NewInt(0)
	bitInt = bitInt.Exp(big.NewInt(2), big.NewInt(featureBit), nil)
	compareInt := big.NewInt(0)
	compareInt = compareInt.And(featuresInt, bitInt)
	return compareInt.Cmp(bitInt) == 0
}

// randomString returns a random 32 byte random string
func randomString() string {
	idBytes := make([]byte, 32)
	_, _ = rand.Read(idBytes[:])
	return hex.EncodeToString(idBytes)
}
