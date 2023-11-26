package testframework

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/elementsproject/peerswap/lightning"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

func getLndConfig() map[string]string {
	return map[string]string{
		"bitcoin.active":           "true",
		"bitcoin.regtest":          "true",
		"bitcoin.node":             "bitcoind",
		"bitcoin.defaultchanconfs": "1",
		"noseedbackup":             "true",
		"norest":                   "true",
		"debuglevel":               "debug",
		"trickledelay":             "1800",
		"bitcoind.estimatemode":    "ECONOMICAL",
	}
}

type LndNode struct {
	*DaemonProcess
	*LndRpcClient

	DataDir      string
	ConfigFile   string
	RpcPort      int
	ListenPort   int
	TlsPath      string
	MacaroonPath string
	Info         *lnrpc.GetInfoResponse

	bitcoin *BitcoinNode
}

func NewLndNode(testDir string, bitcoin *BitcoinNode, id int, extraConfig map[string]string) (*LndNode, error) {
	listen, err := GetFreePort()
	if err != nil {
		return nil, fmt.Errorf("getFreePort() %w", err)
	}

	rpcListen := listen
	for rpcListen == listen {
		rpcListen, err = GetFreePort()
		if err != nil {
			return nil, fmt.Errorf("getFreePort() %w", err)
		}
	}

	rngDirExtension, err := GenerateRandomString(5)
	if err != nil {
		return nil, fmt.Errorf("generateRandomString(5) %w", err)
	}

	lndDir := filepath.Join(testDir, fmt.Sprintf("lnd-%s", rngDirExtension))
	dataDir := filepath.Join(lndDir, "data")
	err = os.MkdirAll(dataDir, os.ModeDir|os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("os.MkdirAll() %w", err)
	}

	regtestConfig := getLndConfig()
	regtestConfig["lnddir"] = lndDir
	regtestConfig["datadir"] = dataDir
	regtestConfig["listen"] = fmt.Sprintf("localhost:%d", listen)
	regtestConfig["rpclisten"] = fmt.Sprintf("localhost:%d", rpcListen)
	regtestConfig["bitcoind.dir"] = bitcoin.DataDir
	regtestConfig["bitcoind.rpchost"] = fmt.Sprintf("%s:%d", bitcoin.rpcHost, bitcoin.rpcPort)
	regtestConfig["bitcoind.rpcuser"] = bitcoin.RpcUser
	regtestConfig["bitcoind.rpcpass"] = bitcoin.RpcPassword
	regtestConfig["bitcoind.zmqpubrawblock"] = bitcoin.ZmqPubRawBlock
	regtestConfig["bitcoind.zmqpubrawtx"] = bitcoin.ZmqPubRawTx

	for k, v := range extraConfig {
		regtestConfig[k] = v
	}

	configFile := filepath.Join(dataDir, "lnd.conf")
	WriteConfig(configFile, regtestConfig, nil, "")

	cmdLine := []string{
		"lnd",
		fmt.Sprintf("--configfile=%s", configFile),
	}

	tlsPath := filepath.Join(dataDir, "..", "tls.cert")
	macaroonPath := filepath.Join(dataDir, "chain", "bitcoin", "regtest", "admin.macaroon")

	return &LndNode{
		DaemonProcess: NewDaemonProcess(cmdLine, fmt.Sprintf("lnd-%d", id)),
		LndRpcClient:  nil,
		DataDir:       dataDir,
		ConfigFile:    configFile,
		RpcPort:       rpcListen,
		ListenPort:    listen,
		TlsPath:       tlsPath,
		MacaroonPath:  macaroonPath,
		bitcoin:       bitcoin,
	}, nil
}

func (n *LndNode) Run(waitForReady, waitForBitcoinSynced bool) error {
	var err error
	n.DaemonProcess.Run()
	if waitForReady {
		err := n.WaitForLog("LightningWallet opened", TIMEOUT)
		if err != nil {
			return fmt.Errorf("LndNode.Run() %w", err)
		}

		err = n.WaitForLog("Starting sub RPC server: RouterRPC", TIMEOUT)
		if err != nil {
			return fmt.Errorf("LndNode.Run() %w", err)
		}
	}

	n.LndRpcClient, err = NewLndRpcClient(fmt.Sprintf("localhost:%d", n.RpcPort), filepath.Join(n.DataDir, "..", "tls.cert"), filepath.Join(n.DataDir, "chain", "bitcoin", "regtest", "admin.macaroon"))
	if err != nil {
		return fmt.Errorf("NewLndClientProxy() %w", err)
	}

	// Cache info
	n.Info, err = n.Rpc.GetInfo(context.Background(), &lnrpc.GetInfoRequest{})
	if err != nil {
		return fmt.Errorf("GetInfo() %w", err)
	}

	// Wait for sync with bitcoin network
	if waitForBitcoinSynced {
		// Wait for sync with bitcoin network
		return WaitForWithErr(func() (bool, error) {
			info, err := n.Rpc.GetInfo(context.Background(), &lnrpc.GetInfoRequest{})
			if err != nil {
				log.Printf("rpc.GetInfo() %v", err)
				return false, fmt.Errorf("rpc.GetInfo() %w", err)
			}

			r, err := n.bitcoin.Rpc.Call("getblockcount")
			if err != nil {
				return false, fmt.Errorf("bitcoin.rpc.Call(\"getblockcount\") %w", err)
			}

			chainHeight, err := r.GetFloat()
			if err != nil {
				return false, fmt.Errorf("GetFloat() %w", err)
			}

			return info.SyncedToChain && chainHeight == float64(info.BlockHeight), nil
		}, TIMEOUT)
	}
	return nil
}

func (n *LndNode) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()

	_, err := n.Rpc.StopDaemon(ctx, &lnrpc.StopRequest{})
	return err
}

func (n *LndNode) Address() string {
	return fmt.Sprintf("%s@127.0.0.1:%d", n.Info.IdentityPubkey, n.ListenPort)
}

func (n *LndNode) Id() (id string) {
	return n.Info.IdentityPubkey
}

func (n *LndNode) GetBtcBalanceSat() (uint64, error) {
	r, err := n.Rpc.WalletBalance(context.Background(), &lnrpc.WalletBalanceRequest{})
	if err != nil {
		return 0, fmt.Errorf("WalletBalance() %w", err)
	}
	return uint64(r.TotalBalance), nil
}

func (n *LndNode) GetChannelBalanceSat(scid string) (sats uint64, err error) {
	r, err := n.Rpc.ListChannels(context.Background(), &lnrpc.ListChannelsRequest{})
	if err != nil {
		return 0, fmt.Errorf("rpc.ListChannels() %w", err)
	}

	for _, ch := range r.Channels {
		if ScidFromLndChanId(ch.ChanId) == scid {
			return uint64(ch.LocalBalance), nil
		}
	}

	return 0, fmt.Errorf("no channel found with scid %s", scid)
}

func (n *LndNode) GetScid(peer LightningNode) (scid string, err error) {
	res, err := n.Rpc.ListChannels(context.Background(), &lnrpc.ListChannelsRequest{})
	if err != nil {
		return "", fmt.Errorf("ListChannels() %w", err)
	}

	for _, ch := range res.Channels {
		if ch.RemotePubkey == peer.Id() {
			return ScidFromLndChanId(ch.ChanId), nil
		}
	}

	return "", fmt.Errorf("peer not found")
}

func (n *LndNode) Connect(peer LightningNode, waitForConnection bool) error {
	pk, host, port, err := SplitLnAddr(peer.Address())
	if err != nil {
		return fmt.Errorf("SplitLnAddr() %w", err)
	}

	_, err = n.Rpc.ConnectPeer(context.Background(),
		&lnrpc.ConnectPeerRequest{Addr: &lnrpc.LightningAddress{
			Pubkey: pk,
			Host:   fmt.Sprintf("%s:%d", host, port),
		}})
	if err != nil {
		return fmt.Errorf("ConnectPeer() %w", err)
	}

	if waitForConnection {
		if waitForConnection {
			return WaitForWithErr(func() (bool, error) {
				localIsConnected, err := n.IsConnected(peer)
				if err != nil {
					return false, fmt.Errorf("IsConnected() %w", err)
				}
				peerIsConnected, err := peer.IsConnected(n)
				if err != nil {
					return false, fmt.Errorf("IsConnected() %w", err)
				}
				return localIsConnected && peerIsConnected, nil
			}, TIMEOUT)
		}
	}

	return nil
}

func (n *LndNode) FundWallet(sats uint64, mineBlock bool) (string, error) {
	addr, err := n.Rpc.NewAddress(context.Background(), &lnrpc.NewAddressRequest{})
	if err != nil {
		return "", fmt.Errorf("NewAddress() %w", err)
	}

	r, err := n.bitcoin.Call("sendtoaddress", addr.Address, float64(sats)/math.Pow(10., 8))
	if err != nil {
		return "", fmt.Errorf("sendtoaddress %w", err)
	}

	var a struct {
		TxId   string
		Abc    error
		Reason int
	}

	var txId string
	err = r.GetObject(&a)
	if err != nil {
		txId, err = r.GetString()
		if err != nil {
			return "", err
		}
	}

	if mineBlock {
		err = n.bitcoin.GenerateBlocks(1)
		if err != nil {
			return "", fmt.Errorf("bitcoin.GenerateBlocks() %w", err)
		}
		err = n.WaitForLog(fmt.Sprintf("Marking unconfirmed transaction %s", txId), TIMEOUT)
		if err != nil {
			return "", err
		}
	}

	return addr.Address, nil
}

func (n *LndNode) OpenChannel(peer LightningNode, capacity, pushAmt uint64, connect, confirm, waitForChannelActive bool) (string, error) {
	// fund wallet 10*cap
	_, err := n.FundWallet(uint64(1.1*float64(capacity)), true)
	if err != nil {
		return "", fmt.Errorf("FundWallet() %w", err)
	}

	isConnected, err := n.IsConnected(peer)
	if err != nil {
		return "", fmt.Errorf("IsConnected() %w", err)
	}

	if !isConnected && connect {
		err = n.Connect(peer, true)
		if err != nil {
			return "", fmt.Errorf("Connect() %w", err)
		}
	}

	pk, err := hex.DecodeString(peer.Id())
	if err != nil {
		return "", fmt.Errorf("DecodeString() %w", err)
	}
	stream, err := n.Rpc.OpenChannel(context.Background(), &lnrpc.OpenChannelRequest{
		NodePubkey:         pk,
		LocalFundingAmount: int64(capacity),
		PushSat:            int64(pushAmt),
	})
	if err != nil {
		return "", fmt.Errorf("OpenChannel() %w", err)
	}

	// Wait for channel pending
	u, err := stream.Recv()
	if err != nil {
		return "", fmt.Errorf("stream.Recv() %w", err)
	}
	chp := u.GetChanPending()
	if chp == nil {
		return "", fmt.Errorf("GetChanPending() was nil")
	}

	if waitForChannelActive || confirm {
		n.bitcoin.GenerateBlocks(3)
	}

	if waitForChannelActive {
		err = WaitForWithErr(func() (bool, error) {
			u, err := stream.Recv()
			if err != nil {
				return false, fmt.Errorf("stream.Recv() %w", err)
			}

			open := u.GetChanOpen()
			return open != nil, nil
		}, TIMEOUT)
		if err != nil {
			return "", fmt.Errorf("error waiting for active channel: %w", err)
		}

		var localActive bool
		var remoteActive bool
		err = WaitForWithErr(func() (bool, error) {
			scid, err := n.GetScid(peer)
			if err != nil {
				return false, fmt.Errorf("GetScid() %w", err)
			}
			if scid == "" {
				return false, nil
			}

			if !localActive {
				localActive, err = n.IsChannelActive(scid)
				if err != nil {
					return false, fmt.Errorf("IsChannelActive() %w", err)
				}
			}
			if !remoteActive {
				remoteActive, err = peer.IsChannelActive(scid)
				if err != nil {
					return false, fmt.Errorf("IsChannelActive() %w", err)
				}
			}
			hasRoute, err := n.HasRoute(peer.Id(), scid)
			if err != nil {
				return false, nil
			}
			return remoteActive && localActive && hasRoute, nil
		}, TIMEOUT)
		if err != nil {
			return "", fmt.Errorf("error waiting for active channel: %w", err)
		}
	}

	scid, err := n.GetScid(peer)
	if err != nil {
		return "", fmt.Errorf("GetScid() %w", err)
	}

	return scid, nil
}

// HasRoute check the route is constructed
func (n *LndNode) HasRoute(remote, scid string) (bool, error) {
	chsRes, err := n.Rpc.ListChannels(context.Background(), &lnrpc.ListChannelsRequest{})
	if err != nil {
		return false, fmt.Errorf("ListChannels() %w", err)
	}
	var channel *lnrpc.Channel
	for _, ch := range chsRes.GetChannels() {
		channelShortId := lnwire.NewShortChanIDFromInt(ch.ChanId)
		if channelShortId.String() == lightning.Scid(scid).LndStyle() {
			channel = ch
		}
	}
	if channel.GetChanId() == 0 {
		return false, fmt.Errorf("could not find a channel with scid: %s", scid)
	}
	v, err := route.NewVertexFromStr(channel.GetRemotePubkey())
	if err != nil {
		return false, fmt.Errorf("NewVertexFromStr() %w", err)
	}

	routeres, err := n.RpcV2.BuildRoute(context.Background(), &routerrpc.BuildRouteRequest{
		AmtMsat:        channel.GetLocalBalance() * 1000 / 2,
		FinalCltvDelta: 9,
		OutgoingChanId: channel.GetChanId(),
		HopPubkeys:     [][]byte{v[:]},
	})
	if err != nil {
		return false, fmt.Errorf("BuildRoute() %w", err)
	}
	return len(routeres.GetRoute().Hops) > 0, nil
}

func (n *LndNode) IsBlockHeightSynced() (bool, error) {
	r, err := n.bitcoin.Rpc.Call("getblockcount")
	if err != nil {
		return false, fmt.Errorf("bitcoin.rpc.Call(\"getblockcount\") %w", err)
	}

	chainHeight, err := r.GetFloat()
	if err != nil {
		return false, fmt.Errorf("GetFloat() %w", err)
	}

	nodeInfo, err := n.Rpc.GetInfo(context.Background(), &lnrpc.GetInfoRequest{})
	if err != nil {
		return false, fmt.Errorf("GetInfo() %w", err)
	}
	return uint(nodeInfo.BlockHeight) >= uint(chainHeight), nil
}

func (n *LndNode) IsChannelActive(scid string) (bool, error) {
	r, err := n.Rpc.ListChannels(context.Background(), &lnrpc.ListChannelsRequest{})
	if err != nil {
		return false, fmt.Errorf("ListChannels() %w", err)
	}

	for _, ch := range r.Channels {
		chScid := ScidFromLndChanId(ch.ChanId)
		if chScid == scid {
			return ch.Active, nil
		}
	}

	return false, nil
}

func (n *LndNode) IsConnected(remote LightningNode) (bool, error) {
	peers, err := n.Rpc.ListPeers(context.Background(), &lnrpc.ListPeersRequest{})
	if err != nil {
		return false, fmt.Errorf("rpc.ListPeers() %w", err)
	}

	for _, peer := range peers.Peers {
		if peer.PubKey == remote.Id() {
			return true, nil
		}
	}

	return false, nil
}

func (n *LndNode) HasPendingHtlcOnChannel(scid string) (bool, error) {
	r, err := n.Rpc.ListChannels(context.Background(), &lnrpc.ListChannelsRequest{})
	if err != nil {
		return false, fmt.Errorf("rpc.ListChannels() %w", err)
	}

	for _, ch := range r.Channels {
		if ScidFromLndChanId(ch.ChanId) == scid {
			return (ch.PendingHtlcs != nil && len(ch.PendingHtlcs) > 0), nil
		}
	}

	return false, fmt.Errorf("channel %s not found", scid)
}

func (n *LndNode) ChanIdFromScid(scid string) (uint64, error) {
	r, err := n.Rpc.ListChannels(context.Background(), &lnrpc.ListChannelsRequest{})
	if err != nil {
		return 0, fmt.Errorf("rpc.ListChannels() %w", err)
	}

	for _, ch := range r.Channels {
		if ScidFromLndChanId(ch.ChanId) == scid {
			return ch.ChanId, nil
		}
	}

	return 0, fmt.Errorf("no channel found with scid %s", scid)

}

func (n *LndNode) AddInvoice(amt uint64, desc, _ string) (payreq string, err error) {
	inv, err := n.Rpc.AddInvoice(context.Background(), &lnrpc.Invoice{Value: int64(amt), Memo: desc})
	if err != nil {
		return "", err
	}
	return inv.PaymentRequest, nil
}

func (n *LndNode) PayInvoice(payreq string) error {
	pstream, err := n.Rpc.SendPaymentSync(context.Background(), &lnrpc.SendRequest{PaymentRequest: payreq})
	if err != nil {
		return err
	}
	if len(pstream.PaymentError) > 0 {
		return fmt.Errorf("got payment error %s", pstream.PaymentError)
	}
	return nil
}

func (n *LndNode) SendPay(bolt11, _ string) error {
	return n.PayInvoice(bolt11)
}

func (n *LndNode) GetLatestInvoice() (payreq string, err error) {
	r, err := n.Rpc.ListInvoices(context.Background(), &lnrpc.ListInvoiceRequest{})
	if err != nil {
		return "", err
	}

	if r.Invoices != nil {
		return r.Invoices[len(r.Invoices)-1].PaymentRequest, nil
	}
	return "", fmt.Errorf("Invioces list is nil")
}

func (n *LndNode) GetMemoFromPayreq(bolt11 string) (string, error) {
	r, err := n.Rpc.DecodePayReq(context.Background(), &lnrpc.PayReqString{PayReq: bolt11})
	if err != nil {
		return "", err
	}
	return r.Description, nil
}

func ScidFromLndChanId(id uint64) string {
	lndScid := lnwire.NewShortChanIDFromInt(id)
	return fmt.Sprintf("%dx%dx%d", lndScid.BlockHeight, lndScid.TxIndex, lndScid.TxPosition)
}

func (n *LndNode) GetFeeInvoiceAmtSat() (sat uint64, err error) {
	rx := regexp.MustCompile(`^peerswap .* fee .*`)
	var feeInvoiceAmt uint64
	r, err := n.Rpc.ListInvoices(context.Background(), &lnrpc.ListInvoiceRequest{})
	if err != nil {
		return 0, err
	}

	for _, i := range r.Invoices {
		if rx.MatchString(i.GetMemo()) {
			feeInvoiceAmt += uint64(i.GetValue())
		}
	}
	return feeInvoiceAmt, nil
}

func (n *LndNode) SetHtlcMaximumMilliSatoshis(scid string, maxHtlcMsat uint64) (msat uint64, err error) {
	s := lightning.Scid(scid)
	res, err := n.Rpc.ListChannels(context.Background(), &lnrpc.ListChannelsRequest{})
	if err != nil {
		return 0, fmt.Errorf("ListChannels() %w", err)
	}
	for _, ch := range res.GetChannels() {
		channelShortId := lnwire.NewShortChanIDFromInt(ch.ChanId)
		if channelShortId.String() == s.LndStyle() {
			r, err := n.Rpc.GetChanInfo(context.Background(), &lnrpc.ChanInfoRequest{
				ChanId: ch.ChanId,
			})
			if err != nil {
				return 0, err
			}
			parts := strings.Split(r.ChanPoint, ":")
			if len(parts) != 2 {
				return 0, fmt.Errorf("expected scid to be composed of 3 blocks")
			}
			txPosition, err := strconv.Atoi(parts[1])
			if err != nil {
				return 0, err
			}
			_, err = n.Rpc.UpdateChannelPolicy(context.Background(), &lnrpc.PolicyUpdateRequest{
				Scope: &lnrpc.PolicyUpdateRequest_ChanPoint{ChanPoint: &lnrpc.ChannelPoint{
					FundingTxid: &lnrpc.ChannelPoint_FundingTxidStr{
						FundingTxidStr: parts[0],
					},
					OutputIndex: uint32(txPosition),
				}},
				BaseFeeMsat:          1000,
				FeeRate:              1,
				FeeRatePpm:           0,
				TimeLockDelta:        40,
				MaxHtlcMsat:          maxHtlcMsat,
				MinHtlcMsat:          msat,
				MinHtlcMsatSpecified: false,
			})
			if err != nil {
				return 0, err
			}
			return maxHtlcMsat, err
		}
	}
	return 0, fmt.Errorf("could not find a channel with scid: %s", scid)
}
