package clightning

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/sputn1ck/glightning/glightning"
	"github.com/sputn1ck/glightning/jrpc2"
	"github.com/sputn1ck/peerswap/policy"
	"github.com/sputn1ck/peerswap/swap"
)

type SwapCanceledError string

func (e SwapCanceledError) Error() string {
	return fmt.Sprintf("swap canceled, reason: %s", string(e))
}

// GetAddressMethod returns a new liquid address
type GetAddressMethod struct {
	cl *ClightningClient `json:"-"`
}

func (g *GetAddressMethod) New() interface{} {
	return &GetAddressMethod{
		cl: g.cl,
	}
}

func (g *GetAddressMethod) Name() string {
	return "peerswap-liquid-getaddress"
}

func (g *GetAddressMethod) Call() (jrpc2.Result, error) {
	if g.cl == nil {
		return nil, errors.New("liquid swaps are not enabled")
	}
	res, err := g.cl.wallet.GetAddress()
	if err != nil {
		return nil, err
	}
	log.Printf("[Wallet] Getting address %s", res)
	return &GetAddressResponse{LiquidAddress: res}, nil
}

type GetAddressResponse struct {
	LiquidAddress string `json:"liquid_address"`
}

// GetBalance returns the liquid balance
type GetBalanceMethod struct {
	cl *ClightningClient
}

func (g *GetBalanceMethod) Name() string {
	return "peerswap-liquid-getbalance"
}

func (g *GetBalanceMethod) New() interface{} {
	return &GetBalanceMethod{
		cl: g.cl,
	}
}

func (g *GetBalanceMethod) Call() (jrpc2.Result, error) {
	if g.cl == nil {
		return nil, errors.New("liquid swaps are not enabled")
	}
	res, err := g.cl.wallet.GetBalance()
	if err != nil {
		return nil, err
	}
	return &GetBalanceResponse{
		res,
	}, nil
}

type GetBalanceResponse struct {
	LiquidBalance uint64 `json:"liquid_balance_sat"`
}

// SendToAddressMethod sends
type SendToAddressMethod struct {
	Address   string `json:"address"`
	AmountSat uint64 `json:"amount_sat"`
	cl        *ClightningClient
}

func (s *SendToAddressMethod) Name() string {
	return "peerswap-liquid-sendtoaddress"
}

func (s *SendToAddressMethod) New() interface{} {
	return &SendToAddressMethod{
		cl: s.cl,
	}
}

func (s *SendToAddressMethod) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &SendToAddressMethod{
		cl: client,
	}
}

func (s *SendToAddressMethod) Call() (jrpc2.Result, error) {
	if s.cl == nil {
		return nil, errors.New("liquid swaps are not enabled")
	}
	if s.Address == "" {
		return nil, errors.New("address must be set")
	}
	if s.AmountSat == 0 {
		return nil, errors.New("amount_sat must be set")
	}
	res, err := s.cl.wallet.SendToAddress(s.Address, s.AmountSat)
	if err != nil {
		log.Printf("error %v", err)
		return nil, err
	}
	return &SendToAddressResponse{TxId: res}, nil
}

type SendToAddressResponse struct {
	TxId string `json:"txid"`
}

func (s *SendToAddressMethod) Description() string {
	return "sends lbtc to an address"
}

func (s *SendToAddressMethod) LongDescription() string {
	return "'"
}

// SwapOut starts a new swapout (paying an Invoice for onchain liquidity)
type SwapOut struct {
	SatAmt         uint64            `json:"amt_sat"`
	ShortChannelId string            `json:"short_channel_id"`
	Asset          string            `json:"asset"`
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

	if fundingChannels.ChannelSatoshi < l.SatAmt {
		return nil, errors.New("not enough outbound capacity to perform swapOut")
	}
	if !fundingChannels.Connected {
		return nil, errors.New("fundingChannels is not connected")
	}

	err = l.cl.PeerRunsPeerSwap(fundingChannels.Id)
	if err != nil {
		return nil, err
	}

	if strings.Compare(l.Asset, "l-btc") == 0 {
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
		funds, err := l.cl.glightning.ListFunds()
		if err != nil {
			return nil, err
		}
		sats := uint64(0)
		for _, v := range funds.Outputs {
			sats += v.Value
		}
		if sats < 5000 {
			return nil, errors.New("you require more some onchain-btc for fees")
		}
	} else {
		return nil, errors.New("invalid asset (btc or l-btc)")
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
			if swapOut.Current == swap.State_SwapOutSender_AwaitFeeResponse || swapOut.Current == swap.State_SwapOutSender_PayFeeInvoice || swapOut.Current == swap.State_SwapOutSender_AwaitTxBroadcastedMessage {
				continue
			}
			if swapOut.Current == swap.State_SwapCanceled {
				if swapOut.Data.CancelMessage != "" {
					return nil, SwapCanceledError(swapOut.Data.CancelMessage)
				}
				if swapOut.Data.LastErr == nil {
					return nil, SwapCanceledError("unknown")
				}
				return nil, swapOut.Data.LastErr

			}
			if swapOut.Current == swap.State_SwapOutSender_AwaitTxConfirmation {
				return swapOut.Data.ToPrettyPrint(), nil
			}

		}
	}
}

// SwapIn Starts a new swap in(providing onchain liquidity)
type SwapIn struct {
	SatAmt         uint64 `json:"amt_sat"`
	ShortChannelId string `json:"short_channel_id"`
	Asset          string `json:"asset"`

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
	if l.ShortChannelId == "" {
		return nil, errors.New("Missing required short_channel_id parameter")
	}
	if l.SatAmt <= 0 {
		return nil, errors.New("Missing required amt_sat parameter")
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

	err = l.cl.PeerRunsPeerSwap(fundingChannels.Id)
	if err != nil {
		return nil, err
	}

	if l.Asset == "l-btc" {
		if !l.cl.swaps.LiquidEnabled {
			return nil, errors.New("liquid swaps are not enabled")
		}
		if l.cl.Gelements == nil {
			return nil, errors.New("peerswap was not started with liquid node config")
		}
		liquidBalance, err := l.cl.wallet.GetBalance()
		if err != nil {
			return nil, err
		}
		if liquidBalance < l.SatAmt {
			return nil, errors.New("Not enough balance on liquid wallet")
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
		// todo need some onchain balance for fees
		if sats < l.SatAmt {
			return nil, errors.New("Not enough balance on c-lightning onchain wallet")
		}
	} else {
		return nil, errors.New("invalid asset (btc or l-btc)")
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
				if swapIn.Data.CancelMessage != "" {
					return nil, SwapCanceledError(swapIn.Data.CancelMessage)
				}
				if swapIn.Data.LastErr == nil {
					return nil, SwapCanceledError("unknown")
				}
				return nil, swapIn.Data.LastErr

			}
			if swapIn.Current == swap.State_SwapInSender_SendTxBroadcastedMessage {
				return swapIn.Data.ToPrettyPrint(), nil
			}

		}
	}

}

// ListSwaps list all active and finished swaps
type ListSwaps struct {
	PrettyPrint bool              `json:"pretty_print,omitempty"`
	cl          *ClightningClient `json:"-"`
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
	if l.PrettyPrint {
		var pswasp []*swap.PrettyPrintSwapData
		for _, v := range swaps {
			if v.Data == nil {
				continue
			}
			pswasp = append(pswasp, v.Data.ToPrettyPrint())
		}
		return pswasp, nil
	}
	return swaps, nil
}

type ListNodes struct {
	cl *ClightningClient
}

func (l *ListNodes) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &ListNodes{cl: client}
}

func (l *ListNodes) Description() string {
	return "lists nodes that support the peerswap plugin"
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
	nodes, err := l.cl.glightning.ListNodes()
	if err != nil {
		return nil, err
	}

	peerSwapNodes := []*glightning.Node{}
	for _, node := range nodes {
		if node.Features != nil && checkFeatures(node.Features.Raw, featureBit) {
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
	return "lists peers supporting the peerswap plugin"
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
							SenderSatsOut += s.Data.Amount
						} else {
							SenderSwapsIn++
							SenderSatsIn += s.Data.Amount
						}
					} else {
						if s.Type == swap.SWAPTYPE_OUT {
							ReceiverSwapsOut++
							ReceiverSatsOut += s.Data.Amount
						} else {
							ReceiverSwapsIn++
							ReceiverSatsIn += s.Data.Amount
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
						ChannelId:     c.ShortChannelId,
						LocalBalance:  c.ChannelSatoshi,
						RemoteBalance: uint64(c.ChannelTotalSatoshi - c.ChannelSatoshi),
						Balance:       float64(c.ChannelSatoshi) / float64(c.ChannelTotalSatoshi),
						State:         c.State,
					})
				}
			}

			peerSwapPeer.Channels = peerSwapPeerChannels
			peerSwappers = append(peerSwappers, peerSwapPeer)
		}
	}
	return peerSwappers, nil
}

type ResendLastMessage struct {
	SwapId string `json:"swap_id"`
	cl     *ClightningClient
}

func (s *ResendLastMessage) Description() string {
	return "resends last swap message"
}

func (s *ResendLastMessage) LongDescription() string {
	return "'"
}

func (g *ResendLastMessage) Name() string {
	return "peerswap-resendmsg"
}

func (g *ResendLastMessage) New() interface{} {
	return &ResendLastMessage{
		cl:     g.cl,
		SwapId: g.SwapId,
	}
}
func (g *ResendLastMessage) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &ResendLastMessage{
		cl: client,
	}
}

func (g *ResendLastMessage) Call() (jrpc2.Result, error) {
	if g.SwapId == "" {
		return nil, errors.New("swap_id required")
	}
	err := g.cl.swaps.ResendLastMessage(g.SwapId)
	if err != nil {
		return nil, err
	}
	return nil, nil
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
	if g.SwapId == "" {
		return nil, errors.New("swap_id required")
	}
	swap, err := g.cl.swaps.GetSwap(g.SwapId)
	if err != nil {
		return nil, err
	}
	return swap, nil
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
	ReloadFile() error
	Get() policy.Policy
}
type ReloadPolicyFile struct {
	cl   *ClightningClient
	name string
}

func (c ReloadPolicyFile) Name() string {
	return "peerswap-reload-policy"
}

func (c ReloadPolicyFile) New() interface{} {
	return c
}

func (c ReloadPolicyFile) Call() (jrpc2.Result, error) {
	log.Println(c.cl.policy)
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

type GetRequestedSwaps struct {
	cl   *ClightningClient
	name string
}

func (c GetRequestedSwaps) Name() string {
	return c.name
}

func (c GetRequestedSwaps) New() interface{} {
	return c
}

func (c GetRequestedSwaps) Call() (jrpc2.Result, error) {
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

type PeerSwapPeerChannel struct {
	ChannelId     string  `json:"short_channel_id"`
	LocalBalance  uint64  `json:"local_balance"`
	RemoteBalance uint64  `json:"remote_balance"`
	Balance       float64 `json:"balance"`
	State         string  `json:"state"`
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

// checkFeatures checks if a node runs the peerswap plugin
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
