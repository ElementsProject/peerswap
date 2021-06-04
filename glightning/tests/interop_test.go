package glightning

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/sputn1ck/liquid-loop/gbitcoin"
	"github.com/sputn1ck/liquid-loop/glightning"
	"github.com/stretchr/testify/assert"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"syscall"
	"testing"
	"time"
)

const defaultTimeout int = 10

func check(t *testing.T, err error) {
	if err != nil {
		debug.PrintStack()
		t.Fatal(err)
	}
}

func advanceChain(t *testing.T, n *Node, btc *gbitcoin.Bitcoin, numBlocks uint) {
	timeout := time.Now().Add(time.Duration(defaultTimeout) * time.Second)

	info, _ := n.rpc.GetInfo()
	blockheight := info.Blockheight
	mineBlocks(t, numBlocks, btc)
	for {
		info, _ = n.rpc.GetInfo()
		if info.Blockheight >= uint(blockheight)+numBlocks {
			return
		}
		if time.Now().After(timeout) {
			t.Fatal("timed out waiting for chain to advance")
		}
	}
}

func waitForChannelActive(n *Node, scid string) error {
	timeout := time.Now().Add(time.Duration(defaultTimeout) * time.Second)
	for {
		chans, _ := n.rpc.GetChannel(scid)
		// both need to be active
		active := 0
		for i := 0; i < len(chans); i++ {
			if chans[i].IsActive {
				active += 1
			}
		}
		if active == 2 {
			return nil
		}
		if time.Now().After(timeout) {
			return errors.New(fmt.Sprintf("timed out waiting for scid %s", scid))
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func waitForChannelReady(t *testing.T, from, to *Node) {
	timeout := time.Now().Add(time.Duration(defaultTimeout) * time.Second)
	for {
		info, err := to.rpc.GetInfo()
		check(t, err)
		peer, err := from.rpc.GetPeer(info.Id)
		check(t, err)
		if peer.Channels == nil {
			t.Fatal("no channels for peer")
		}
		if peer.Channels[0].State == "CHANNELD_NORMAL" {
			return
		}
		if time.Now().After(timeout) {
			t.Fatal("timed out waiting for channel normal")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func Init(t *testing.T) (string, string, int, *gbitcoin.Bitcoin) {
	// let's put it in a temporary directory
	testDir, err := ioutil.TempDir("", "gltests-")
	check(t, err)
	dataDir, _, btcPort, btc := SpinUpBitcoind(t, testDir)
	return testDir, dataDir, btcPort, btc
}

func CleanUp(testDir string) {
	os.Remove(testDir)
}

type BNode struct {
	rpc  *gbitcoin.Bitcoin
	dir  string
	port uint
	pid  uint
}

// Returns a bitcoin node w/ RPC client
func SpinUpBitcoind(t *testing.T, dir string) (string, int, int, *gbitcoin.Bitcoin) {
	// make some dirs!
	bitcoindDir := filepath.Join(dir, "bitcoind")
	err := os.Mkdir(bitcoindDir, os.ModeDir|0755)
	check(t, err)

	bitcoinPath, err := exec.LookPath("bitcoind")
	check(t, err)
	btcPort, err := getPort()
	check(t, err)
	btcUser := "btcuser"
	btcPass := "btcpass"
	bitcoind := exec.Command(bitcoinPath, "-regtest",
		fmt.Sprintf("-datadir=%s", bitcoindDir),
		"-server", "-logtimestamps", "-nolisten",
		fmt.Sprintf("-rpcport=%d", btcPort),
		"-addresstype=bech32", "-logtimestamps", "-txindex",
		fmt.Sprintf("-rpcpassword=%s", btcPass),
		fmt.Sprintf("-rpcuser=%s", btcUser))

	bitcoind.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}
	log.Printf("starting %s on %d...", bitcoinPath, btcPort)
	err = bitcoind.Start()
	check(t, err)
	log.Printf(" bitcoind started (%d)!\n", bitcoind.Process.Pid)

	btc := gbitcoin.NewBitcoin(btcUser, btcPass)
	btc.SetTimeout(uint(2))
	// Waits til bitcoind is up
	btc.StartUp("", bitcoindDir, uint(btcPort))
	// Go ahead and run 50 blocks
	addr, err := btc.GetNewAddress(gbitcoin.Bech32)
	check(t, err)
	_, err = btc.GenerateToAddress(addr, 101)
	check(t, err)

	return bitcoindDir, bitcoind.Process.Pid, btcPort, btc
}

func (node *Node) waitForLog(t *testing.T, phrase string, timeoutSec int) {
	timeout := time.Now().Add(time.Duration(timeoutSec) * time.Second)

	// at startup we need to wait for the file to open
	logfilePath := filepath.Join(node.dir, "log")
	for time.Now().Before(timeout) || timeoutSec == 0 {
		if _, err := os.Stat(logfilePath); os.IsNotExist(err) {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		break
	}
	logfile, _ := os.Open(logfilePath)
	defer logfile.Close()

	reader := bufio.NewReader(logfile)
	for timeoutSec == 0 || time.Now().Before(timeout) {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				time.Sleep(100 * time.Millisecond)
			} else {
				check(t, err)
			}
		}
		m, err := regexp.MatchString(phrase, line)
		check(t, err)
		if m {
			return
		}
	}

	t.Fatal(fmt.Sprintf("Unable to find \"%s\" in %s/log", phrase, node.dir))
}

func getPort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

type Node struct {
	rpc *glightning.Lightning
	dir string
}

func LnNode(t *testing.T, testDir, dataDir string, btcPort int, name string, extraOps map[string]string) *Node {
	var err error
	lightningPath := os.Getenv("LIGHTNINGD_PATH")
	if lightningPath == "" {
		// assume it's just a thing i can call
		lightningPath, err = exec.LookPath("lightningd")
		check(t, err)
	}

	lightningdDir := filepath.Join(testDir, fmt.Sprintf("lightningd-%s", name))
	err = os.Mkdir(lightningdDir, os.ModeDir|0755)
	check(t, err)

	port, err := getPort()
	check(t, err)

	args := []string{
		fmt.Sprintf("--lightning-dir=%s", lightningdDir),
		fmt.Sprintf("--bitcoin-datadir=%s", dataDir),
		"--network=regtest", "--funding-confirms=3",
		fmt.Sprintf("--addr=localhost:%d", port),
		fmt.Sprintf("--bitcoin-rpcport=%d", btcPort),
		"--log-file=log",
		"--log-level=debug",
		"--bitcoin-rpcuser=btcuser",
		"--bitcoin-rpcpassword=btcpass",
		"--dev-fast-gossip",
		"--dev-bitcoind-poll=1",
		"--allow-deprecated-apis=false",
	}

	if extraOps != nil {
		for arg, val := range extraOps {
			if val == "" {
				args = append(args, fmt.Sprintf("--%s", arg))
			} else {
				args = append(args, fmt.Sprintf("--%s=%s", arg, val))
			}
		}
	}

	lightningd := exec.Command(lightningPath, args...)

	lightningd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}
	stderr, err := lightningd.StderrPipe()
	check(t, err)
	stdout, err := lightningd.StdoutPipe()
	log.Printf("starting %s on %d...", lightningPath, port)
	err = lightningd.Start()
	check(t, err)
	go func() {
		// print any stderr output to the test log
		log.Printf("Starting stderr scanner")
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Println(scanner.Text())
		}
	}()
	go func() {
		// print any stderr output to the test log
		log.Printf("Starting stdout scanner")
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			log.Println(scanner.Text())
		}
	}()
	go func() {
		err := lightningd.Wait()
		if err != nil {
			t.Fatal(fmt.Sprintf("lightningd exited with error %s", err))
		}
		log.Printf("process exited normally")
	}()

	time.Sleep(200 * time.Millisecond)

	lightningdDir = filepath.Join(lightningdDir, "regtest")
	node := &Node{nil, lightningdDir}
	log.Printf("starting node in %s\n", lightningdDir)
	node.waitForLog(t, "Server started with public key", 30)
	log.Printf(" lightningd started (%d)!\n", lightningd.Process.Pid)

	node.rpc = glightning.NewLightning()
	node.rpc.StartUp("lightning-rpc", lightningdDir)

	return node
}

func short(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}
}

func TestBitcoinProxy(t *testing.T) {
	short(t)

	testDir, _, _, btc := Init(t)
	defer CleanUp(testDir)
	addr, err := btc.GetNewAddress(gbitcoin.Bech32)
	check(t, err)
	assert.NotNil(t, addr)

}

func TestConnectRpc(t *testing.T) {
	short(t)

	testDir, dataDir, btcPid, _ := Init(t)
	defer CleanUp(testDir)
	l1 := LnNode(t, testDir, dataDir, btcPid, "one", nil)

	l1Info, _ := l1.rpc.GetInfo()
	assert.Equal(t, 1, len(l1Info.Binding))

	l1Addr := l1Info.Binding[0]
	l2 := LnNode(t, testDir, dataDir, btcPid, "two", nil)
	peerId, err := l2.rpc.Connect(l1Info.Id, l1Addr.Addr, uint(l1Addr.Port))
	check(t, err)
	assert.Equal(t, peerId, l1Info.Id)
}

func TestConfigsRpc(t *testing.T) {
	short(t)

	testDir, dataDir, btcPid, _ := Init(t)
	defer CleanUp(testDir)
	l1 := LnNode(t, testDir, dataDir, btcPid, "one", nil)

	configs, err := l1.rpc.ListConfigs()
	check(t, err)
	assert.Equal(t, "lightning-rpc", configs["rpc-file"])
	assert.Equal(t, false, configs["always-use-proxy"])

	network, err := l1.rpc.GetConfig("network")
	check(t, err)
	assert.Equal(t, "regtest", network)
}

func TestHelpRpc(t *testing.T) {
	short(t)

	testDir, dataDir, btcPid, _ := Init(t)
	defer CleanUp(testDir)
	l1 := LnNode(t, testDir, dataDir, btcPid, "one", nil)

	commands, err := l1.rpc.Help()
	check(t, err)
	if len(commands) == 0 {
		t.Error("No help commands returned")
	}

	cmd, err := l1.rpc.HelpFor("help")
	check(t, err)
	assert.Equal(t, "help [command]", cmd.NameAndUsage)
}

func TestSignCheckMessage(t *testing.T) {
	short(t)

	msg := "hello there"
	testDir, dataDir, btcPid, _ := Init(t)
	defer CleanUp(testDir)
	l1 := LnNode(t, testDir, dataDir, btcPid, "one", nil)
	l2 := LnNode(t, testDir, dataDir, btcPid, "two", nil)

	l1Info, _ := l1.rpc.GetInfo()

	signed, err := l1.rpc.SignMessage(msg)
	check(t, err)

	v, err := l2.rpc.CheckMessageVerify(msg, signed.ZBase, l1Info.Id)
	check(t, err)
	assert.True(t, v)
}

func TestListTransactions(t *testing.T) {
	short(t)

	testDir, dataDir, btcPid, btc := Init(t)
	defer CleanUp(testDir)
	l1 := LnNode(t, testDir, dataDir, btcPid, "one", nil)

	fundNode(t, "1.0", l1, btc)
	fundNode(t, "1.0", l1, btc)
	waitToSync(l1)
	trans, err := l1.rpc.ListTransactions()
	check(t, err)
	assert.Equal(t, len(trans), 2)
}

func fundNode(t *testing.T, amount string, n *Node, b *gbitcoin.Bitcoin) {
	addr, err := n.rpc.NewAddr()
	check(t, err)
	_, err = b.SendToAddress(addr, amount)
	check(t, err)

	mineBlocks(t, 1, b)
}

// n is number of blocks to mine
func mineBlocks(t *testing.T, n uint, b *gbitcoin.Bitcoin) {
	addr, err := b.GetNewAddress(gbitcoin.Bech32)
	check(t, err)
	_, err = b.GenerateToAddress(addr, n)
	check(t, err)
}

func waitToSync(n *Node) {
	for {
		info, _ := n.rpc.GetInfo()
		if info.IsLightningdSync() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestCreateOnion(t *testing.T) {
	short(t)

	testDir, dataDir, btcPid, _ := Init(t)
	defer CleanUp(testDir)
	l1 := LnNode(t, testDir, dataDir, btcPid, "one", nil)

	hops := []glightning.Hop{
		glightning.Hop{
			Pubkey:  "02eec7245d6b7d2ccb30380bfbe2a3648cd7a942653f5aa340edcea1f283686619",
			Payload: "000000000000000000000000000000000000000000000000000000000000000000",
		},
		glightning.Hop{
			Pubkey:  "0324653eac434488002cc06bbfb7f10fe18991e35f9fe4302dbea6d2353dc0ab1c",
			Payload: "140101010101010101000000000000000100000001",
		},
		glightning.Hop{
			Pubkey:  "027f31ebc5462c1fdce1b737ecff52d37d75dea43ce11c74d25aa297165faa2007",
			Payload: "fd0100000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f404142434445464748494a4b4c4d4e4f505152535455565758595a5b5c5d5e5f606162636465666768696a6b6c6d6e6f707172737475767778797a7b7c7d7e7f808182838485868788898a8b8c8d8e8f909192939495969798999a9b9c9d9e9fa0a1a2a3a4a5a6a7a8a9aaabacadaeafb0b1b2b3b4b5b6b7b8b9babbbcbdbebfc0c1c2c3c4c5c6c7c8c9cacbcccdcecfd0d1d2d3d4d5d6d7d8d9dadbdcdddedfe0e1e2e3e4e5e6e7e8e9eaebecedeeeff0f1f2f3f4f5f6f7f8f9fafbfcfdfeff",
		},
		glightning.Hop{
			Pubkey:  "032c0b7cf95324a07d05398b240174dc0c2be444d96b159aa6c7f7b1e668680991",
			Payload: "140303030303030303000000000000000300000003",
		},
		glightning.Hop{
			Pubkey:  "02edabbd16b41c8371b92ef2f04c1185b4f03b6dcd52ba9b78d9d7c89c8f221145",
			Payload: "000404040404040404000000000000000400000004000000000000000000000000",
		},
	}

	privateHash := "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	resp, err := l1.rpc.CreateOnion(hops, privateHash, "")
	check(t, err)

	assert.Equal(t, len(resp.SharedSecrets), len(hops))
	assert.Equal(t, len(resp.Onion), 2*1366)

	privateHash = "4242424242424242424242424242424242424242424242424242424242424242"
	sessionKey := "4141414141414141414141414141414141414141414141414141414141414141"
	resp, err = l1.rpc.CreateOnion(hops, privateHash, sessionKey)
	check(t, err)

	firstHop := glightning.FirstHop{
		ShortChannelId: "100x1x1",
		Direction:      1,
		AmountMsat:     "1000sat",
		Delay:          8,
	}

	// Ideally we'd do a 'real' send onion but we don't
	// need to know if c-lightning works, only that the API
	// functions correctly...
	_, err = l1.rpc.SendOnionWithDetails(resp.Onion, firstHop, privateHash, "label", resp.SharedSecrets, nil)

	// ... which means we expect an error back!
	assert.NotNil(t, err)
	assert.Equal(t, err.Error(), "204:No connection to first peer found")
}

func getShortChannelId(t *testing.T, node1, node2 *Node) string {
	info, err := node2.rpc.GetInfo()
	check(t, err)
	peer, err := node1.rpc.GetPeer(info.Id)
	check(t, err)
	if peer == nil || len(peer.Channels) == 0 {
		t.Fatal(fmt.Sprintf("peer %s not found", info.Id))
	}
	return peer.Channels[0].ShortChannelId
}

func TestPluginOptions(t *testing.T) {
	short(t)

	testDir, dataDir, btcPid, _ := Init(t)
	defer CleanUp(testDir)

	// try with the defaults
	l1 := LnNode(t, testDir, dataDir, btcPid, "one", nil)

	exPlugin := pluginPath(t, "plugin_example")
	_, err := l1.rpc.StartPlugin(exPlugin)
	check(t, err)
	l1.waitForLog(t, `Is this initial node startup\? false`, 1)
	l1.waitForLog(t, `the bool option is set to true`, 1)
	l1.waitForLog(t, `the int option is set to 11`, 1)
	l1.waitForLog(t, `the flag option is set\? false`, 1)

	// now try with some different values!
	optsMap := make(map[string]string)
	optsMap["plugin"] = exPlugin
	optsMap["int_opt"] = "-55"
	optsMap["bool_opt"] = "false"
	optsMap["flag_opt"] = ""
	l2 := LnNode(t, testDir, dataDir, btcPid, "two", optsMap)
	l2.waitForLog(t, `Is this initial node startup\? true`, 1)
	l2.waitForLog(t, `the bool option is set to false`, 1)
	l2.waitForLog(t, `the int option is set to -55`, 1)
	l2.waitForLog(t, `the flag option is set\? true`, 1)
}

// ok, now let's check the plugin subs+hooks etc
func TestPlugins(t *testing.T) {
	short(t)

	testDir, dataDir, btcPid, btc := Init(t)
	defer CleanUp(testDir)
	l1 := LnNode(t, testDir, dataDir, btcPid, "one", nil)

	plugins, err := l1.rpc.ListPlugins()
	check(t, err)
	pluginCount := len(plugins)

	exPlugin := pluginPath(t, "plugin_example")
	plugins, err = l1.rpc.StartPlugin(exPlugin)
	check(t, err)
	assert.Equal(t, pluginCount+1, len(plugins))
	l1.waitForLog(t, `Is this initial node startup\? false`, 1)

	l1Info, _ := l1.rpc.GetInfo()
	assert.Equal(t, 1, len(l1Info.Binding))

	l1Addr := l1Info.Binding[0]
	l2 := LnNode(t, testDir, dataDir, btcPid, "two", nil)
	plugins, err = l2.rpc.StartPlugin(exPlugin)
	check(t, err)
	l2.waitForLog(t, `Is this initial node startup\? false`, 1)

	// We should have a third node!
	l3 := LnNode(t, testDir, dataDir, btcPid, "three", nil)
	check(t, err)

	peerId, err := l2.rpc.Connect(l1Info.Id, "localhost", uint(l1Addr.Port))
	check(t, err)

	l3Info, _ := l3.rpc.GetInfo()
	peer3, err := l2.rpc.Connect(l3Info.Id, "localhost", uint(l3Info.Binding[0].Port))
	check(t, err)

	fundNode(t, "1.0", l2, btc)
	waitToSync(l1)
	waitToSync(l2)

	// open a channel
	amount := glightning.NewSat(10000000)
	feerate := glightning.NewFeeRate(glightning.PerKw, uint(253))
	pushSats := glightning.NewMsat(10000)
	_, err = l2.rpc.FundChannelExt(peerId, amount, feerate, true, nil, pushSats)
	check(t, err)

	// wait til the change is onchain
	advanceChain(t, l2, btc, 1)

	// fund a second channel!
	_, err = l2.rpc.FundChannelAtFee(peer3, amount, feerate)
	check(t, err)

	mineBlocks(t, 6, btc)

	waitForChannelReady(t, l2, l3)
	waitForChannelReady(t, l2, l1)

	// there's two now??
	scid23 := getShortChannelId(t, l2, l3)
	l2.waitForLog(t, fmt.Sprintf(`Received channel_update for channel %s/. now ACTIVE`, scid23), 20)
	scid21 := getShortChannelId(t, l2, l1)
	l2.waitForLog(t, fmt.Sprintf(`Received channel_update for channel %s/. now ACTIVE`, scid21), 20)

	// wait for everybody to know about other channels
	waitForChannelActive(l1, scid23)
	waitForChannelActive(l3, scid21)

	// warnings go off because of feerate misfires
	l1.waitForLog(t, "Got a warning!!", 1)

	// channel opened notification
	l1.waitForLog(t, "channel opened", 1)

	invAmt := uint64(100000)
	invAmt2 := uint64(10000)
	inv, err := l1.rpc.CreateInvoice(invAmt, "push pay", "money", 100, nil, "", false)
	inv2, err := l3.rpc.CreateInvoice(invAmt, "push pay two", "money two", 100, nil, "", false)
	check(t, err)

	route, err := l2.rpc.GetRouteSimple(peerId, invAmt, 1.0)
	check(t, err)

	// l2 -> l1
	_, err = l2.rpc.SendPayLite(route, inv.PaymentHash)
	check(t, err)
	_, err = l2.rpc.WaitSendPay(inv.PaymentHash, 0)
	check(t, err)

	// SEND PAY SUCCESS
	l2.waitForLog(t, "send pay success!", 1)
	l1.waitForLog(t, "invoice paid", 1)

	/* ?? why no work
	l2.waitForLog(t, "invoice payment called", 1)
	*/

	// now try to route from l1 -> l3 (but with broken middle)
	route2, err := l1.rpc.GetRouteSimple(peer3, invAmt2, 1.0)
	check(t, err)

	_, err = l2.rpc.CloseNormal(peer3)
	check(t, err)
	mineBlocks(t, 1, btc)

	_, err = l1.rpc.SendPayLite(route2, inv2.PaymentHash)
	check(t, err)
	_, failure := l1.rpc.WaitSendPay(inv2.PaymentHash, 0)

	pe, ok := failure.(*glightning.PaymentError)
	if !ok {
		t.Fatal(failure)
	}

	data := pe.Data
	assert.Equal(t, data.Status, "failed")
	assert.Equal(t, data.AmountSentMilliSatoshi, "10001msat")
	// SEND PAY FAILURE
	l1.waitForLog(t, "send pay failure!", 1)
}

func TestAcceptWithClose(t *testing.T) {
	short(t)

	testDir, dataDir, btcPid, btc := Init(t)
	defer CleanUp(testDir)
	l1 := LnNode(t, testDir, dataDir, btcPid, "one", nil)

	exPlugin := pluginPath(t, "plugin_openchan")
	_, err := l1.rpc.StartPlugin(exPlugin)
	l1.waitForLog(t, "successfully init'd!", 1)
	l1Info, _ := l1.rpc.GetInfo()
	assert.Equal(t, 1, len(l1Info.Binding))

	l1Addr := l1Info.Binding[0]
	l2 := LnNode(t, testDir, dataDir, btcPid, "two", nil)

	peerId, err := l2.rpc.Connect(l1Info.Id, "localhost", uint(l1Addr.Port))
	check(t, err)

	fundNode(t, "1.0", l2, btc)
	waitToSync(l1)
	waitToSync(l2)

	feerate := glightning.NewFeeRate(glightning.PerKw, uint(253))
	amount := uint64(100000)
	starter, err := l2.rpc.StartFundChannel(peerId, amount, true, feerate, "")
	check(t, err)

	// build a transaction
	outs := []*gbitcoin.TxOut{
		&gbitcoin.TxOut{
			Address: starter.Address,
			Satoshi: amount,
		},
	}
	rawtx, err := btc.CreateRawTx(nil, outs, nil, nil)
	check(t, err)
	fundedtx, err := btc.FundRawTx(rawtx)
	check(t, err)
	tx, err := btc.DecodeRawTx(fundedtx.TxString)
	check(t, err)
	txout, err := tx.FindOutputIndex(starter.Address)
	check(t, err)
	_, err = l2.rpc.CompleteFundChannel(peerId, tx.TxId, txout)
	check(t, err)

	l1.waitForLog(t, "openchannel called", 1)

	l2info, _ := l2.rpc.GetInfo()
	peer, err := l1.rpc.GetPeer(l2info.Id)
	check(t, err)

	closeTo := "bcrt1q8q4xevfuwgsm7mxant8aadz50xt67768s4332d"
	assert.Equal(t, closeTo, peer.Channels[0].CloseToAddress)
}

func TestCloseTo(t *testing.T) {
	short(t)

	testDir, dataDir, btcPid, btc := Init(t)
	defer CleanUp(testDir)
	l1 := LnNode(t, testDir, dataDir, btcPid, "one", nil)

	l1Info, _ := l1.rpc.GetInfo()
	assert.Equal(t, 1, len(l1Info.Binding))

	l1Addr := l1Info.Binding[0]
	l2 := LnNode(t, testDir, dataDir, btcPid, "two", nil)

	peerId, err := l2.rpc.Connect(l1Info.Id, "localhost", uint(l1Addr.Port))
	check(t, err)

	fundNode(t, "1.0", l2, btc)
	waitToSync(l1)
	waitToSync(l2)

	closeTo, err := btc.GetNewAddress(gbitcoin.Bech32)
	check(t, err)
	feerate := glightning.NewFeeRate(glightning.PerKw, uint(253))
	amount := uint64(100000)
	starter, err := l2.rpc.StartFundChannel(peerId, amount, true, feerate, closeTo)
	check(t, err)

	// build a transaction
	outs := []*gbitcoin.TxOut{
		&gbitcoin.TxOut{
			Address: starter.Address,
			Satoshi: amount,
		},
	}
	rawtx, err := btc.CreateRawTx(nil, outs, nil, nil)
	check(t, err)
	fundedtx, err := btc.FundRawTx(rawtx)
	check(t, err)
	tx, err := btc.DecodeRawTx(fundedtx.TxString)
	check(t, err)
	txout, err := tx.FindOutputIndex(starter.Address)
	check(t, err)
	_, err = l2.rpc.CompleteFundChannel(peerId, tx.TxId, txout)
	check(t, err)

	peer, err := l2.rpc.GetPeer(peerId)
	check(t, err)

	assert.Equal(t, closeTo, peer.Channels[0].CloseToAddress)
}

func TestInvoiceFieldsOnPaid(t *testing.T) {
	short(t)

	testDir, dataDir, btcPid, btc := Init(t)
	defer CleanUp(testDir)
	l1 := LnNode(t, testDir, dataDir, btcPid, "one", nil)

	l1Info, _ := l1.rpc.GetInfo()
	assert.Equal(t, 1, len(l1Info.Binding))

	l1Addr := l1Info.Binding[0]
	l2 := LnNode(t, testDir, dataDir, btcPid, "two", nil)

	peerId, err := l2.rpc.Connect(l1Info.Id, "localhost", uint(l1Addr.Port))
	check(t, err)

	fundNode(t, "1.0", l2, btc)
	waitToSync(l1)
	waitToSync(l2)

	// open a channel
	amount := glightning.NewSat(10000000)
	feerate := glightning.NewFeeRate(glightning.PerKw, uint(253))
	_, err = l2.rpc.FundChannelAtFee(peerId, amount, feerate)
	check(t, err)

	// wait til the change is onchain
	advanceChain(t, l2, btc, 6)
	waitForChannelReady(t, l2, l1)

	invAmt := uint64(100000)
	invO, err := l1.rpc.CreateInvoice(invAmt, "pay me", "money", 100, nil, "", false)
	check(t, err)

	_, err = l2.rpc.PayBolt(invO.Bolt11)
	check(t, err)

	invA, err := l1.rpc.GetInvoice("pay me")
	check(t, err)

	assert.True(t, len(invA.PaymentHash) > 0)
}

func TestBtcBackend(t *testing.T) {
	short(t)

	testDir, dataDir, btcPid, btc := Init(t)
	defer CleanUp(testDir)

	exPlugin := pluginPath(t, "plugin_btc")
	optsMap := make(map[string]string)
	optsMap["disable-plugin"] = "bcli"
	optsMap["plugin"] = exPlugin
	l1 := LnNode(t, testDir, dataDir, btcPid, "one", optsMap)
	l2 := LnNode(t, testDir, dataDir, btcPid, "two", optsMap)
	l1.waitForLog(t, "All Bitcoin plugin commands registered", 1)

	l1.waitForLog(t, "called getchaininfo", 1)
	l1.waitForLog(t, "called blockbyheight", 1)

	fundNode(t, "1.0", l1, btc)
	waitToSync(l1)

	// send yourself some funds, so sendrawtransaction gets called
	addr, err := l1.rpc.NewAddr()
	check(t, err)
	amt := glightning.NewSat(10000)
	rate := glightning.NewFeeRate(glightning.PerKw, 253)
	_, err = l1.rpc.Withdraw(addr, amt, rate, nil)
	check(t, err)
	l1.waitForLog(t, "called sendrawtransaction", 1)
	mineBlocks(t, 1, btc)

	// try to open a channel and then cancel it, so getutxo gets called
	connectInfo := connectNode(t, l1, l2)
	channelfunds := uint64(100000)
	starter, err := l1.rpc.StartFundChannel(connectInfo.Id, channelfunds, true, rate, "")
	check(t, err)

	// build a transaction
	outs := []*gbitcoin.TxOut{
		&gbitcoin.TxOut{
			Address: starter.Address,
			Satoshi: channelfunds,
		},
	}
	rawtx, err := btc.CreateRawTx(nil, outs, nil, nil)
	check(t, err)
	fundedtx, err := btc.FundRawTx(rawtx)
	check(t, err)
	tx, err := btc.DecodeRawTx(fundedtx.TxString)
	check(t, err)
	txout, err := tx.FindOutputIndex(starter.Address)
	check(t, err)
	_, err = l1.rpc.CompleteFundChannel(connectInfo.Id, tx.TxId, txout)
	check(t, err)

	// ok this will call a check for the utxo...
	canceled, err := l1.rpc.CancelFundChannel(connectInfo.Id)
	check(t, err)
	assert.True(t, canceled)
	l1.waitForLog(t, "called getutxo", 1)
	l1.waitForLog(t, "called estimatefees", 1)
}

// let's try out some hooks!
func TestHooks(t *testing.T) {
	short(t)

	testDir, dataDir, btcPid, _ := Init(t)
	defer CleanUp(testDir)
	l1 := LnNode(t, testDir, dataDir, btcPid, "one", nil)
	l2 := LnNode(t, testDir, dataDir, btcPid, "two", nil)

	exPlugin := pluginPath(t, "plugin_example")
	loadPlugin(t, l1, exPlugin)

	l1Info, _ := l1.rpc.GetInfo()
	l2Info, _ := l2.rpc.GetInfo()
	peer := connectNode(t, l1, l2)
	assert.Equal(t, peer.Id, l2Info.Id)

	l1.waitForLog(t, "peer connected called", 1)

	l2.rpc.Disconnect(l1Info.Id, true)
	l1.waitForLog(t, "disconnect called for", 1)
}

func TestDbWrites(t *testing.T) {
	short(t)

	testDir, dataDir, btcPid, _ := Init(t)
	defer CleanUp(testDir)

	exPlugin := pluginPath(t, "plugin_dbwrites")
	optsMap := make(map[string]string)
	optsMap["plugin"] = exPlugin
	l1 := LnNode(t, testDir, dataDir, btcPid, "one", optsMap)
	waitToSync(l1)

	l1.waitForLog(t, "dbwrite called 1", 1)
}

func TestRpcCmd(t *testing.T) {
	short(t)

	testDir, dataDir, btcPid, _ := Init(t)
	defer CleanUp(testDir)
	l1 := LnNode(t, testDir, dataDir, btcPid, "one", nil)
	l2 := LnNode(t, testDir, dataDir, btcPid, "two", nil)

	exPlugin := pluginPath(t, "plugin_rpccmd")
	loadPlugin(t, l1, exPlugin)

	connectInfo := connectNode(t, l1, l2)

	addr, err := l1.rpc.NewAddress(glightning.P2SHSegwit)
	check(t, err)

	// we pass in segwit but the rpc_command hook always gives
	// us back bech32
	assert.NotNil(t, addr.Bech32, "tb1")
	assert.Equal(t, "", addr.P2SHSegwit)

	amt := glightning.NewSat(10000)
	rate := glightning.NewFeeRate(glightning.PerKw, 253)
	res, err := l1.rpc.Withdraw(addr.Bech32, amt, rate, nil)

	assert.Equal(t, "-401:withdrawals not allowed", err.Error())
	assert.Equal(t, &glightning.WithdrawResult{}, res)

	// this fails because we can't handle random responses
	_, err = l1.rpc.Ping(connectInfo.Id)
	assert.NotNil(t, err)
}

func pluginPath(t *testing.T, pluginName string) string {
	// Get the path to our current test binary
	val, ok := os.LookupEnv("PLUGINS_PATH")
	if !ok {
		t.Fatal("No plugin example path (PLUGINS_PATH) passed in")
	}
	return filepath.Join(val, pluginName)
}

func loadPlugin(t *testing.T, n *Node, exPlugin string) {
	_, err := n.rpc.StartPlugin(exPlugin)
	check(t, err)
	//n.waitForLog(t, `successfully init'd`, 5)
}

func connectNode(t *testing.T, from, to *Node) *glightning.ConnectResult {
	info, _ := to.rpc.GetInfo()
	conn, err := from.rpc.ConnectPeer(info.Id, info.Binding[0].Addr, uint(info.Binding[0].Port))
	check(t, err)
	return conn
}

func openChannel(t *testing.T, btc *gbitcoin.Bitcoin, from, to *Node, amt uint64, waitTilReady bool) {
	connectInfo := connectNode(t, from, to)
	amount := glightning.NewSat64(amt)
	feerate := glightning.NewFeeRate(glightning.PerKw, uint(253))
	_, err := from.rpc.FundChannelAtFee(connectInfo.Id, amount, feerate)
	check(t, err)

	mineBlocks(t, 6, btc)

	if waitTilReady {
		waitForChannelReady(t, from, to)
	}
}

func TestFeatureBits(t *testing.T) {
	short(t)

	testDir, dataDir, btcPid, btc := Init(t)
	defer CleanUp(testDir)

	pp := pluginPath(t, "plugin_featurebits")
	optsMap := make(map[string]string)
	optsMap["plugin"] = pp
	l1 := LnNode(t, testDir, dataDir, btcPid, "one", optsMap)
	l2 := LnNode(t, testDir, dataDir, btcPid, "two", nil)
	info := connectNode(t, l2, l1)

	// check for init feature bits in connect response (1 << 101)
	assert.NotNil(t, info.Features)
	assert.True(t, info.Features.IsSet(101))

	// open a channel + wait til active
	l1Info, _ := l1.rpc.GetInfo()

	fundNode(t, "1.0", l2, btc)
	waitToSync(l2)
	amount := glightning.NewSat(10000000)
	feerate := glightning.NewFeeRate(glightning.PerKw, uint(253))
	_, err := l2.rpc.FundChannelAtFee(l1Info.Id, amount, feerate)
	check(t, err)
	mineBlocks(t, 6, btc)
	waitForChannelReady(t, l2, l1)
	waitForChannelReady(t, l1, l2)
	scid21 := getShortChannelId(t, l2, l1)
	err = waitForChannelActive(l2, scid21)
	check(t, err)
	err = waitForChannelActive(l1, scid21)
	check(t, err)

	// check for init message bits (1 << 101)
	peer, _ := l2.rpc.GetPeer(l1Info.Id)
	assert.True(t, peer.Features.IsSet(101))

	// check for 1 << 105
	inv, err := l1.rpc.Invoice(uint64(10000), "test", "desc")
	check(t, err)
	decoded, err := l1.rpc.DecodeBolt11(inv.Bolt11)
	assert.True(t, decoded.Features.IsSet(105))

	time.Sleep(1 * time.Second)

	node, err := l1.rpc.GetNode(l1Info.Id)
	check(t, err)
	assert.NotNil(t, node.Features)
	assert.True(t, node.Features.IsSet(103))
}

func TestHtlcAcceptedHook(t *testing.T) {
	short(t)

	testDir, dataDir, btcPid, btc := Init(t)
	defer CleanUp(testDir)
	l1 := LnNode(t, testDir, dataDir, btcPid, "one", nil)
	l2 := LnNode(t, testDir, dataDir, btcPid, "two", nil)
	l3 := LnNode(t, testDir, dataDir, btcPid, "three", nil)

	// 2nd + 3rd node listens for htlc accepts
	exPlugin := pluginPath(t, "plugin_htlcacc")
	loadPlugin(t, l2, exPlugin)
	loadPlugin(t, l3, exPlugin)

	// fund l1 + l2
	fundNode(t, "1.0", l1, btc)
	fundNode(t, "1.0", l2, btc)
	waitToSync(l1)
	waitToSync(l2)

	// open a channel
	openChannel(t, btc, l2, l3, uint64(10000000), true)
	openChannel(t, btc, l1, l2, uint64(10000000), true)

	// wait for everybody to know about other channels
	scid23 := getShortChannelId(t, l2, l3)
	l2.waitForLog(t, fmt.Sprintf(`Received channel_update for channel %s/. now ACTIVE`, scid23), 20)
	scid21 := getShortChannelId(t, l1, l2)
	l2.waitForLog(t, fmt.Sprintf(`Received channel_update for channel %s/. now ACTIVE`, scid21), 20)
	err := waitForChannelActive(l1, scid23)
	check(t, err)
	err = waitForChannelActive(l3, scid21)
	check(t, err)

	invAmt := uint64(100000)
	inv, err := l3.rpc.CreateInvoice(invAmt, "push pay", "money", 100, nil, "", false)
	check(t, err)

	// now route from l1 -> l3
	_, err = l1.rpc.PayBolt(inv.Bolt11)
	check(t, err)
	_, err = l1.rpc.WaitSendPay(inv.PaymentHash, 0)
	check(t, err)

	// l2 should have gotten an htlc_accept hook call
	l2.waitForLog(t, "htlc_accepted called", 1)
	l2.waitForLog(t, `has perhop\? false`, 1)
	l2.waitForLog(t, `type is tlv`, 1)
	l2.waitForLog(t, `payment secret is empty`, 1)
	l2.waitForLog(t, `amount is empty`, 1)

	// l3 should have gotten an htlc_accept hook call, with different info
	l3.waitForLog(t, "htlc_accepted called", 1)
	l3.waitForLog(t, `type is tlv`, 1)
	l3.waitForLog(t, `has perhop\? false`, 1)
	l3.waitForLog(t, `payment secret is not empty`, 1)
	l3.waitForLog(t, `amount is 10000\dmsat`, 1)
}
