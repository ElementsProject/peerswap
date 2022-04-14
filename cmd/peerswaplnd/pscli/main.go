package main

import (
	"context"
	"fmt"
	log2 "log"
	"os"

	"github.com/lightninglabs/protobuf-hex-display/jsonpb"
	"github.com/lightninglabs/protobuf-hex-display/proto"
	"github.com/sputn1ck/peerswap/peerswaprpc"
	"github.com/urfave/cli"
	"google.golang.org/grpc"
)

func main() {
	app := cli.NewApp()
	app.Name = "pscli"
	app.Usage = "PeerSwap Cli"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "rpchost",
			Value: "localhost:42069",
			Usage: "peerswapd grpc address host:port",
		},
	}
	app.Commands = []cli.Command{
		swapOutCommand, swapInCommand, getSwapCommand, listSwapsCommand,
		listPeersCommand, listNodesCommand, reloadPolicyFileCommand, listRequestedSwapsCommand,
		liquidGetBalanceCommand, liquidGetAddressCommand, liquidSendToAddressCommand,
		stopCommand, listActiveSwapsCommand, rejectSwapsCommand, addPeerCommand, removePeerCommand,
	}
	err := app.Run(os.Args)
	if err != nil {
		log2.Fatal(err)
	}

}

var (
	satAmountFlag = cli.Uint64Flag{
		Name:     "sat_amt",
		Usage:    "Amount of Sats to swap for",
		Required: true,
	}
	channelIdFlag = cli.Uint64Flag{
		Name:     "channel_id",
		Usage:    "channel id of channel to swap over",
		Required: true,
	}
	assetFlag = cli.StringFlag{
		Name:     "asset",
		Usage:    "asset to swap with: 'btc' | 'lbtc'",
		Required: true,
	}
	swapIdFlag = cli.StringFlag{
		Name:     "id",
		Required: true,
	}
	liquidAddressFlag = cli.StringFlag{
		Name:     "address",
		Required: true,
	}
	rejectFlag = cli.BoolFlag{
		Name:     "reject",
		Required: true,
	}
	pubkeyFlag = cli.StringFlag{
		Name:     "peer_pubkey",
		Required: true,
	}

	swapOutCommand = cli.Command{
		Name:  "swapout",
		Usage: "Perform a swap-out (sending lightning funds to receive onchain funds)",
		Flags: []cli.Flag{
			satAmountFlag,
			channelIdFlag,
			assetFlag,
		},
		Action: swapOut,
	}

	swapInCommand = cli.Command{
		Name:  "swapin",
		Usage: "Perform a swap-in (sending onchain funds to receive lightning funds)",
		Flags: []cli.Flag{
			satAmountFlag,
			channelIdFlag,
			assetFlag,
		},
		Action: swapIn,
	}

	getSwapCommand = cli.Command{
		Name:  "getswap",
		Usage: "Get a swap by its id",
		Flags: []cli.Flag{
			swapIdFlag,
		},
		Action: getSwap,
	}

	listSwapsCommand = cli.Command{
		Name:   "listswaps",
		Usage:  "lists all swaps",
		Flags:  []cli.Flag{},
		Action: listSwaps,
	}

	listPeersCommand = cli.Command{
		Name:   "listpeers",
		Usage:  "lists all peerswap-enabled peers",
		Flags:  []cli.Flag{},
		Action: listPeers,
	}
	listNodesCommand = cli.Command{
		Name:   "listnodes",
		Usage:  "lists all peerswap-enabled nodes in the network",
		Flags:  []cli.Flag{},
		Action: listNodes,
	}
	reloadPolicyFileCommand = cli.Command{
		Name:   "reloadpolicy",
		Usage:  "reloads the policy file and polls all peers with the new policy",
		Flags:  []cli.Flag{},
		Action: reloadPolicyFile,
	}
	listRequestedSwapsCommand = cli.Command{
		Name:   "listswaprequests",
		Usage:  "lists requested swaps by peers",
		Flags:  []cli.Flag{},
		Action: listRequestedSwaps,
	}
	liquidGetAddressCommand = cli.Command{
		Name:   "lbtc-getaddress",
		Usage:  "gets a new lbtc address",
		Flags:  []cli.Flag{},
		Action: liquidGetAddress,
	}
	liquidGetBalanceCommand = cli.Command{
		Name:   "lbtc-getbalance",
		Usage:  "gets the current lbtc balance",
		Flags:  []cli.Flag{},
		Action: liquidGetBalance,
	}
	liquidSendToAddressCommand = cli.Command{
		Name:  "lbtc-sendtoaddress",
		Usage: "sends the sat amount to a lbtc address",
		Flags: []cli.Flag{
			satAmountFlag,
			liquidAddressFlag,
		},
		Action: liquidSendToAddress,
	}
	listActiveSwapsCommand = cli.Command{
		Name:   "listactiveswaps",
		Usage:  "list active swaps",
		Action: listActiveSwaps,
	}
	rejectSwapsCommand = cli.Command{
		Name:  "rejectswaps",
		Usage: "Sets peerswap to reject all incoming swap requests",
		Flags: []cli.Flag{
			rejectFlag,
		},
		Action: rejectSwaps,
	}
	addPeerCommand = cli.Command{
		Name:  "addpeer",
		Usage: "Adds a peer to the allowlist",
		Flags: []cli.Flag{
			pubkeyFlag,
		},
		Action: addPeer,
	}
	removePeerCommand = cli.Command{
		Name:  "removepeer",
		Usage: "Removes a peer from the allowlist",
		Flags: []cli.Flag{
			pubkeyFlag,
		},
		Action: removePeer,
	}
	stopCommand = cli.Command{
		Name:   "stop",
		Usage:  "stops the peerswap daemon",
		Flags:  []cli.Flag{},
		Action: stopPeerswap,
	}
)

func swapIn(ctx *cli.Context) error {

	client, cleanup, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	res, err := client.SwapIn(context.Background(), &peerswaprpc.SwapInRequest{
		ChannelId:  ctx.Uint64(channelIdFlag.Name),
		SwapAmount: ctx.Uint64(satAmountFlag.Name),
		Asset:      ctx.String(assetFlag.Name),
	})
	if err != nil {
		return err
	}
	printRespJSON(res)
	return nil
}

func swapOut(ctx *cli.Context) error {

	client, cleanup, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	res, err := client.SwapOut(context.Background(), &peerswaprpc.SwapOutRequest{
		ChannelId:  ctx.Uint64(channelIdFlag.Name),
		SwapAmount: ctx.Uint64(satAmountFlag.Name),
		Asset:      ctx.String(assetFlag.Name),
	})
	if err != nil {
		return err
	}
	printRespJSON(res)
	return nil
}

func getSwap(ctx *cli.Context) error {
	client, cleanup, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	res, err := client.GetSwap(context.Background(), &peerswaprpc.GetSwapRequest{
		SwapId: ctx.String(swapIdFlag.Name),
	})
	if err != nil {
		return err
	}
	printRespJSON(res)
	return nil
}

func listSwaps(ctx *cli.Context) error {
	client, cleanup, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	res, err := client.ListSwaps(context.Background(), &peerswaprpc.ListSwapsRequest{})
	if err != nil {
		return err
	}
	printRespJSON(res)
	return nil
}

func listPeers(ctx *cli.Context) error {
	client, cleanup, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	res, err := client.ListPeers(context.Background(), &peerswaprpc.ListPeersRequest{})
	if err != nil {
		return err
	}
	printRespJSON(res)
	return nil
}

func listNodes(ctx *cli.Context) error {
	client, cleanup, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	res, err := client.ListNodes(context.Background(), &peerswaprpc.ListNodesRequest{})
	if err != nil {
		return err
	}
	printRespJSON(res)
	return nil
}

func reloadPolicyFile(ctx *cli.Context) error {
	client, cleanup, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	res, err := client.ReloadPolicyFile(context.Background(), &peerswaprpc.ReloadPolicyFileRequest{})
	if err != nil {
		return err
	}
	printRespJSON(res)
	return nil
}

func listRequestedSwaps(ctx *cli.Context) error {
	client, cleanup, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	res, err := client.ListRequestedSwaps(context.Background(), &peerswaprpc.ListRequestedSwapsRequest{})
	if err != nil {
		return err
	}
	printRespJSON(res)
	return nil
}

func liquidGetAddress(ctx *cli.Context) error {
	client, cleanup, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	res, err := client.LiquidGetAddress(context.Background(), &peerswaprpc.GetAddressRequest{})
	if err != nil {
		return err
	}
	printRespJSON(res)
	return nil
}

func liquidGetBalance(ctx *cli.Context) error {
	client, cleanup, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	res, err := client.LiquidGetBalance(context.Background(), &peerswaprpc.GetBalanceRequest{})
	if err != nil {
		return err
	}
	printRespJSON(res)
	return nil
}

func liquidSendToAddress(ctx *cli.Context) error {
	client, cleanup, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	res, err := client.LiquidSendToAddress(context.Background(), &peerswaprpc.SendToAddressRequest{
		Address:   ctx.String(liquidAddressFlag.Name),
		SatAmount: ctx.Uint64(satAmountFlag.Name),
	})
	if err != nil {
		return err
	}
	printRespJSON(res)
	return nil
}

func listActiveSwaps(ctx *cli.Context) error {
	client, cleanup, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	res, err := client.ListActiveSwaps(context.Background(), &peerswaprpc.ListSwapsRequest{})
	if err != nil {
		return err
	}
	printRespJSON(res)
	return nil
}

func rejectSwaps(ctx *cli.Context) error {
	client, cleanup, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	res, err := client.RejectSwaps(context.Background(), &peerswaprpc.RejectSwapsRequest{
		Reject: ctx.Bool(rejectFlag.Name),
	})
	if err != nil {
		return err
	}
	printRespJSON(res)
	return nil
}

func addPeer(ctx *cli.Context) error {
	client, cleanup, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	res, err := client.AddPeer(context.Background(), &peerswaprpc.AddPeerRequest{
		PeerPubkey: ctx.String(pubkeyFlag.Name),
	})
	if err != nil {
		return err
	}
	printRespJSON(res)
	return nil
}

func removePeer(ctx *cli.Context) error {
	client, cleanup, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	res, err := client.RemovePeer(context.Background(), &peerswaprpc.RemovePeerRequest{
		PeerPubkey: ctx.String(pubkeyFlag.Name),
	})
	if err != nil {
		return err
	}
	printRespJSON(res)
	return nil
}

func stopPeerswap(ctx *cli.Context) error {
	client, cleanup, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	res, err := client.Stop(context.Background(), &peerswaprpc.Empty{})
	if err != nil {
		return err
	}
	printRespJSON(res)
	return nil
}

func getClient(ctx *cli.Context) (peerswaprpc.PeerSwapClient, func(), error) {
	rpcServer := ctx.GlobalString("rpchost")

	conn, err := getClientConn(rpcServer)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { conn.Close() }

	psClient := peerswaprpc.NewPeerSwapClient(conn)
	return psClient, cleanup, nil
}

func getClientConn(address string) (*grpc.ClientConn,
	error) {

	maxMsgRecvSize := grpc.MaxCallRecvMsgSize(1 * 1024 * 1024 * 200)
	opts := []grpc.DialOption{
		grpc.WithDefaultCallOptions(maxMsgRecvSize),
		grpc.WithInsecure(),
	}

	conn, err := grpc.Dial(address, opts...)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to RPC server: %v",
			err)
	}

	return conn, nil
}

func printRespJSON(resp proto.Message) {
	jsonMarshaler := &jsonpb.Marshaler{
		OrigName:     true,
		EmitDefaults: true,
		Indent:       "    ",
	}

	jsonStr, err := jsonMarshaler.MarshalToString(resp)
	if err != nil {
		fmt.Println("unable to decode response: ", err)
		return
	}

	fmt.Println(jsonStr)
}
