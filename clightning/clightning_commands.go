package clightning

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/elementsproject/peerswap/log"
	"github.com/elementsproject/peerswap/peerswaprpc"
	"github.com/elementsproject/peerswap/premium"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

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
	return &peerswaprpc.GetAddressResponse{Address: res}, nil
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
	return &peerswaprpc.GetBalanceResponse{
		SatAmount: res,
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
	return &peerswaprpc.SendToAddressResponse{TxId: res}, nil
}

func (s *LiquidSendToAddress) Description() string {
	return "sends lbtc to an address"
}

func (s *LiquidSendToAddress) LongDescription() string {
	return "'"
}

// SwapOut starts a new swapout (paying an Invoice for onchain liquidity)
type SwapOut struct {
	ShortChannelId      string            `json:"short_channel_id"`
	SatAmt              uint64            `json:"amt_sat"`
	Asset               string            `json:"asset"`
	PremiumLimitRatePPM int64             `json:"premium_rate_limit_ppm"`
	Force               bool              `json:"force"`
	cl                  *ClightningClient `json:"-"`
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

	if fundingChannels.AmountMilliSatoshi.MSat() < (l.SatAmt+5000)*1000 {
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
		if ok, perr := l.cl.liquidWallet.Ping(); perr != nil || !ok {
			return nil, fmt.Errorf("liquid wallet not reachable: %v", perr)
		}

	} else if strings.Compare(l.Asset, "btc") == 0 {
		if !l.cl.swaps.BitcoinEnabled {
			return nil, errors.New("bitcoin swaps are not enabled")
		}
	} else {
		return nil, errors.New("invalid asset (btc or lbtc)")
	}

	pk := l.cl.GetNodeId()
	swapOut, err := l.cl.swaps.SwapOut(fundingChannels.Id, l.Asset, l.ShortChannelId, pk, l.SatAmt, l.PremiumLimitRatePPM)
	if err != nil {
		return nil, err
	}
	// In order to be responsive we wait for the `opening_tx` to be sent before
	// we return. Time out if we wait too long.
	if !swapOut.WaitForStateChange(func(st swap.StateType) bool {
		switch st {
		case swap.State_SwapOutSender_AwaitTxConfirmation:
			return true
		case swap.State_SwapCanceled:
			err = SwapCanceledError(swapOut.Data.GetCancelMessage())
			return true
		default:
			return false
		}
	}, 30*time.Second) {
		// Timeout.
		return nil, errors.New("rpc timeout reached, use peerswap-listswaps for info")
	}
	if err != nil {
		return nil, err
	}
	return peerswaprpc.PrettyprintFromServiceSwap(swapOut), nil
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
	ShortChannelId      string            `json:"short_channel_id"`
	SatAmt              uint64            `json:"amt_sat"`
	Asset               string            `json:"asset"`
	PremiumLimitRatePPM int64             `json:"premium_limit_ppm"`
	Force               bool              `json:"force"`
	cl                  *ClightningClient `json:"-"`
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
	if fundingChannels.AmountMilliSatoshi.MSat()-fundingChannels.OurAmountMilliSatoshi.MSat() < (l.SatAmt * 1000) {
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
	switch l.Asset {
	case "lbtc":
		if !l.cl.swaps.LiquidEnabled {
			return nil, errors.New("liquid swaps are not enabled")
		}
	case "btc":
		if !l.cl.swaps.BitcoinEnabled {
			return nil, errors.New("bitcoin swaps are not enabled")
		}
	default:
		return nil, errors.New("invalid asset (btc or lbtc)")
	}

	pk := l.cl.GetNodeId()
	swapIn, err := l.cl.swaps.SwapIn(fundingChannels.Id, l.Asset, l.ShortChannelId, pk, l.SatAmt, l.PremiumLimitRatePPM)
	if err != nil {
		return nil, err
	}
	// In order to be responsive we wait for the `opening_tx` to be sent before
	// we return. Time out if we wait too long.
	if !swapIn.WaitForStateChange(func(st swap.StateType) bool {
		switch st {
		case swap.State_SwapInSender_SendTxBroadcastedMessage:
			return true
		case swap.State_SwapCanceled:
			err = SwapCanceledError(swapIn.Data.GetCancelMessage())
			return true
		default:
			return false
		}
	}, 30*time.Second) {
		// Timeout.
		return nil, errors.New("rpc timeout reached, use peerswap-listswaps for info")
	}
	if err != nil {
		return nil, err
	}
	return peerswaprpc.PrettyprintFromServiceSwap(swapIn), nil
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
	polls, err := l.cl.pollService.GetCompatiblePolls()
	if err != nil {
		return nil, err
	}

	peerSwappers := []*peerswaprpc.PeerSwapPeer{}
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

			peerSwapPeer := &peerswaprpc.PeerSwapPeer{
				NodeId:          peer.Id,
				SwapsAllowed:    p.PeerAllowed,
				SupportedAssets: p.Assets,
				AsSender: &peerswaprpc.SwapStats{
					SwapsOut: SenderSwapsOut,
					SwapsIn:  SenderSwapsIn,
					SatsOut:  SenderSatsOut,
					SatsIn:   SenderSatsIn,
				},
				AsReceiver: &peerswaprpc.SwapStats{
					SwapsOut: ReceiverSwapsOut,
					SwapsIn:  ReceiverSwapsIn,
					SatsOut:  ReceiverSatsOut,
					SatsIn:   ReceiverSatsIn,
				},
				PaidFee: paidFees,
				PeerPremium: &Premium{
					BTCSwapInPremiumRatePPM:   p.BTCSwapInPremiumRatePPM,
					BTCSwapOutPremiumRatePPM:  p.BTCSwapOutPremiumRatePPM,
					LBTCSwapInPremiumRatePPM:  p.LBTCSwapInPremiumRatePPM,
					LBTCSwapOutPremiumRatePPM: p.LBTCSwapOutPremiumRatePPM,
				},
			}
			channels, err := l.cl.glightning.ListChannelsBySource(peer.Id)
			if err != nil {
				return nil, err
			}
			peerSwapPeerChannels := []*peerswaprpc.PeerSwapPeerChannel{}
			for _, channel := range channels {
				if c, ok := fundingChannels[channel.ShortChannelId]; ok {
					scid, err := peerswaprpc.NewScidFromString(c.ShortChannelId)
					if err != nil {
						return nil, err
					}
					peerSwapPeerChannels = append(peerSwapPeerChannels, &peerswaprpc.PeerSwapPeerChannel{
						ChannelId:     scid.ToUint64(),
						LocalBalance:  c.OurAmountMilliSatoshi.MSat() / 1000,
						RemoteBalance: (c.AmountMilliSatoshi.MSat() - c.OurAmountMilliSatoshi.MSat()) / 1000,
						Active:        channelActive(c.State),
					})
				}
			}

			peerSwapPeer.Channels = peerSwapPeerChannels
			peerSwappers = append(peerSwappers, peerSwapPeer)
		}
	}
	return peerSwappers, nil
}

func channelActive(state string) bool {
	return state == "CHANNELD_NORMAL"
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

type ListConfig struct {
	cl *ClightningClient
}

func (c *ListConfig) Name() string {
	return "peerswap-listconfig"
}

func (c *ListConfig) New() interface{} {
	return &ListConfig{
		cl: c.cl,
	}
}

func (c *ListConfig) Call() (jrpc2.Result, error) {
	if !c.cl.isReady {
		return nil, ErrWaitingForReady
	}

	return c.cl.peerswapConfig, nil
}

func (c *ListConfig) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &ListConfig{
		cl: client,
	}
}

func (c ListConfig) Description() string {
	return "Show the peerswap config"
}

func (c ListConfig) LongDescription() string {
	return c.Description()
}

func toPremiumAssetType(asset string) premium.AssetType {
	switch strings.ToUpper(asset) {
	case "BTC":
		return premium.BTC
	case "LBTC":
		return premium.LBTC
	default:
		return premium.AsserUnspecified
	}
}

func toPremiumOperationType(operation string) premium.OperationType {
	switch strings.ToUpper(operation) {
	case "SWAP_IN":
		return premium.SwapIn
	case "SWAP_OUT":
		return premium.SwapOut
	default:
		return premium.OperationUnspecified
	}
}

type GetPremiumRate struct {
	PeerID    string            `json:"peer_id"`
	Asset     string            `json:"asset"`
	Operation string            `json:"operation"`
	cl        *ClightningClient `json:"-"`
}

func (c *GetPremiumRate) Name() string {
	return "peerswap-getpremiumrate"
}

func (c *GetPremiumRate) New() interface{} {
	return &GetPremiumRate{
		cl: c.cl,
	}
}

type response struct {
	json.RawMessage
}

// formatProtoMessage formats a proto message to a human readable
func formatProtoMessage(m proto.Message) (response, error) {
	mb, err := protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "  ",
		AllowPartial:    false,
		UseProtoNames:   true,
		UseEnumNumbers:  false,
		EmitUnpopulated: true,
		Resolver:        nil,
	}.Marshal(m)
	return response{json.RawMessage(mb)}, err
}

func (c *GetPremiumRate) Call() (jrpc2.Result, error) {
	if !c.cl.isReady {
		return nil, ErrWaitingForReady
	}
	e, err := c.cl.ps.GetRate(c.PeerID, toPremiumAssetType(c.Asset),
		toPremiumOperationType(c.Operation))
	if err != nil {
		return nil, fmt.Errorf("error getting premium rate: %v", err)
	}
	return formatProtoMessage(&peerswaprpc.PremiumRate{
		Asset:          peerswaprpc.ToAssetType(e.Asset()),
		Operation:      peerswaprpc.ToOperationType(e.Operation()),
		PremiumRatePpm: e.PremiumRatePPM().Value(),
	})
}

func (c *GetPremiumRate) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &GetPremiumRate{
		cl: client,
	}
}

func (c GetPremiumRate) Description() string {
	return "Get the premium rate for a peer"
}

func (c GetPremiumRate) LongDescription() string {
	return c.Description()
}

type UpdatePremiumRate struct {
	PeerID         string            `json:"peer_id"`
	Asset          string            `json:"asset"`
	Operation      string            `json:"operation"`
	PremiumRatePPM int64             `json:"premium_rate_ppm"`
	cl             *ClightningClient `json:"-"`
}

func (c *UpdatePremiumRate) Name() string {
	return "peerswap-updatepremiumrate"
}

func (c *UpdatePremiumRate) New() interface{} {
	return &UpdatePremiumRate{
		cl: c.cl,
	}
}

func (c *UpdatePremiumRate) Call() (jrpc2.Result, error) {
	if !c.cl.isReady {
		return nil, ErrWaitingForReady
	}
	rate, err := premium.NewPremiumRate(toPremiumAssetType(c.Asset),
		toPremiumOperationType(c.Operation), premium.NewPPM(c.PremiumRatePPM))
	if err != nil {
		return nil, fmt.Errorf("error creating premium rate: %v", err)
	}
	err = c.cl.ps.SetRate(c.PeerID, rate)
	if err != nil {
		return nil, fmt.Errorf("error setting premium rate: %v", err)
	}
	return formatProtoMessage(&peerswaprpc.PremiumRate{
		Asset:          peerswaprpc.ToAssetType(rate.Asset()),
		Operation:      peerswaprpc.ToOperationType(rate.Operation()),
		PremiumRatePpm: rate.PremiumRatePPM().Value(),
	})
}

func (c *UpdatePremiumRate) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &UpdatePremiumRate{
		cl: client,
	}
}

func (c UpdatePremiumRate) Description() string {
	return "Set the premium rate for a peer"
}

func (c UpdatePremiumRate) LongDescription() string {
	return c.Description()
}

type DeletePremiumRate struct {
	PeerID    string            `json:"peer_id"`
	Asset     string            `json:"asset"`
	Operation string            `json:"operation"`
	cl        *ClightningClient `json:"-"`
}

func (c *DeletePremiumRate) Name() string {
	return "peerswap-deletepremiumrate"
}

func (c *DeletePremiumRate) New() interface{} {
	return &DeletePremiumRate{
		cl: c.cl,
	}
}

func (c *DeletePremiumRate) Call() (jrpc2.Result, error) {
	if !c.cl.isReady {
		return nil, ErrWaitingForReady
	}
	err := c.cl.ps.DeleteRate(c.PeerID, toPremiumAssetType(c.Asset),
		toPremiumOperationType(c.Operation))
	if err != nil {
		return nil, fmt.Errorf("error deleting premium rate: %v", err)
	}
	return formatProtoMessage(&peerswaprpc.PremiumRate{
		Asset:          peerswaprpc.ToAssetType(toPremiumAssetType(c.Asset)),
		Operation:      peerswaprpc.ToOperationType(toPremiumOperationType(c.Operation)),
		PremiumRatePpm: 0,
	})
}

func (c *DeletePremiumRate) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &DeletePremiumRate{
		cl: client,
	}
}

func (c DeletePremiumRate) Description() string {
	return "Delete the premium rate for a peer"
}

func (c DeletePremiumRate) LongDescription() string {
	return c.Description()
}

type UpdateGlobalPremiumRate struct {
	Asset          string            `json:"asset"`
	Operation      string            `json:"operation"`
	PremiumRatePPM int64             `json:"premium_rate_ppm"`
	cl             *ClightningClient `json:"-"`
}

func (c *UpdateGlobalPremiumRate) Name() string {
	return "peerswap-updateglobalpremiumrate"
}

func (c *UpdateGlobalPremiumRate) New() interface{} {
	return &UpdateGlobalPremiumRate{
		cl: c.cl,
	}
}

func (c *UpdateGlobalPremiumRate) Call() (jrpc2.Result, error) {
	if !c.cl.isReady {
		return nil, ErrWaitingForReady
	}
	rate, err := premium.NewPremiumRate(toPremiumAssetType(c.Asset),
		toPremiumOperationType(c.Operation), premium.NewPPM(c.PremiumRatePPM))
	if err != nil {
		return nil, fmt.Errorf("error creating premium rate: %v", err)
	}
	err = c.cl.ps.SetDefaultRate(rate)
	if err != nil {
		return nil, fmt.Errorf("error setting default premium rate: %v", err)
	}
	return formatProtoMessage(&peerswaprpc.PremiumRate{
		Asset:          peerswaprpc.ToAssetType(rate.Asset()),
		Operation:      peerswaprpc.ToOperationType(rate.Operation()),
		PremiumRatePpm: rate.PremiumRatePPM().Value(),
	})
}

func (c *UpdateGlobalPremiumRate) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &UpdateGlobalPremiumRate{
		cl: client,
	}
}

func (c UpdateGlobalPremiumRate) Description() string {
	return "Set the default premium rate"
}

func (c UpdateGlobalPremiumRate) LongDescription() string {
	return c.Description()
}

type GetGlobalPremiumRate struct {
	Asset     string            `json:"asset"`
	Operation string            `json:"operation"`
	cl        *ClightningClient `json:"-"`
}

func (c *GetGlobalPremiumRate) Name() string {
	return "peerswap-getglobalpremiumrate"
}

func (c *GetGlobalPremiumRate) New() interface{} {
	return &GetGlobalPremiumRate{
		cl: c.cl,
	}
}

func (c *GetGlobalPremiumRate) Call() (jrpc2.Result, error) {
	if !c.cl.isReady {
		return nil, ErrWaitingForReady
	}
	rate, err := c.cl.ps.GetDefaultRate(toPremiumAssetType(c.Asset),
		toPremiumOperationType(c.Operation))
	if err != nil {
		return nil, fmt.Errorf("error getting default premium rate: %v", err)
	}
	return formatProtoMessage(&peerswaprpc.PremiumRate{
		Asset:          peerswaprpc.ToAssetType(rate.Asset()),
		Operation:      peerswaprpc.ToOperationType(rate.Operation()),
		PremiumRatePpm: rate.PremiumRatePPM().Value(),
	})
}

func (c *GetGlobalPremiumRate) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &GetGlobalPremiumRate{
		cl: client,
	}
}

func (c GetGlobalPremiumRate) Description() string {
	return "Get the default premium rate"
}

func (c GetGlobalPremiumRate) LongDescription() string {
	return c.Description()
}

type PeerSwapPeerChannel struct {
	ChannelId     string `json:"short_channel_id"`
	LocalBalance  uint64 `json:"local_balance"`
	RemoteBalance uint64 `json:"remote_balance"`
	State         string `json:"state"`
}

type SwapStats struct {
	SwapsOut uint64 `json:"total_swaps_out"`
	SwapsIn  uint64 `json:"total_swaps_in"`
	SatsOut  uint64 `json:"total_sats_swapped_out"`
	SatsIn   uint64 `json:"total_sats_swapped_in"`
}

type Premium struct {
	BTCSwapInPremiumRatePPM   int64 `json:"btc_swap_in_premium_rate_ppm"`
	BTCSwapOutPremiumRatePPM  int64 `json:"btc_swap_out_premium_rate_ppm"`
	LBTCSwapInPremiumRatePPM  int64 `json:"lbtc_swap_in_premium_rate_ppm"`
	LBTCSwapOutPremiumRatePPM int64 `json:"lbtc_swap_out_premium_rate_ppm"`
}

type PeerSwapPeer struct {
	NodeId          string                 `json:"nodeid"`
	SwapsAllowed    bool                   `json:"swaps_allowed"`
	SupportedAssets []string               `json:"supported_assets"`
	Channels        []*PeerSwapPeerChannel `json:"channels"`
	AsSender        *SwapStats             `json:"sent,omitempty"`
	AsReceiver      *SwapStats             `json:"received,omitempty"`
	PaidFee         uint64                 `json:"total_fee_paid,omitempty"`
	PeerPremium     *Premium               `json:"premium,omitempty"`
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
