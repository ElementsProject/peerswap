//go:build misc
// +build misc

package lnd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/sputn1ck/peerswap/onchain"
	"io/ioutil"
	"log"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcutil/psbt"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	"github.com/lightningnetwork/lnd/lnrpc/walletrpc"
	"github.com/lightningnetwork/lnd/macaroons"
	"github.com/sputn1ck/glightning/gbitcoin"
	"github.com/sputn1ck/peerswap/lightning"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"
)

var ()

func Test_Lnd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//bcli, err := getBitcoinClient()
	//if err != nil {
	//	t.Fatal(err)
	//}
	//btconchain := onchain2.NewBitcoinOnChain(bcli, nil, &chaincfg.SigNetParams)

	lndConn, err := getClientConnectenLocal(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	lndClient := lnrpc.NewLightningClient(lndConn)
	walletClient := walletrpc.NewWalletKitClient(lndConn)
	gi, err := lndClient.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	log.Printf("%v", gi)
	//err = releaseFunds(ctx, walletClient)
	//if err != nil {
	//	t.Fatal(err)
	//}

	swapParams := NewTxParams(gi.BlockHeight+100, 10000)
	//psbtOutput, err := createPsbtOutput(swapParams)
	//if err != nil {
	//	t.Fatal(err)
	//}
	openingAddress, err := createOpeningAddress(swapParams)
	if err != nil {
		t.Fatal(err)
	}

	fundPsbtTemplate := &walletrpc.TxTemplate{
		Outputs: map[string]uint64{
			openingAddress: swapParams.SwapAmount,
		},
	}
	fundRes, err := walletClient.FundPsbt(ctx, &walletrpc.FundPsbtRequest{
		Template: &walletrpc.FundPsbtRequest_Raw{Raw: fundPsbtTemplate},
		Fees:     &walletrpc.FundPsbtRequest_SatPerVbyte{SatPerVbyte: 20},
	})
	if err != nil {
		t.Fatal(err)
	}

	unsignedPacket, err := psbt.NewFromRawBytes(bytes.NewReader(fundRes.FundedPsbt), false)
	if err != nil {
		t.Fatal(err)
	}

	bytesBuffer := new(bytes.Buffer)
	err = unsignedPacket.Serialize(bytesBuffer)
	if err != nil {
		t.Fatal(err)
	}
	finalizeRes, err := walletClient.FinalizePsbt(ctx, &walletrpc.FinalizePsbtRequest{
		FundedPsbt: bytesBuffer.Bytes(),
	})
	if err != nil {
		t.Fatal(err)
	}
	psbtString := base64.StdEncoding.EncodeToString(finalizeRes.SignedPsbt)
	rawTxHex := hex.EncodeToString(finalizeRes.RawFinalTx)
	log.Printf("psbt string %s ", psbtString)

	log.Printf("rawtxhex string %s ", rawTxHex)

	valid, vout, err := VerifyTx(rawTxHex, swapParams)
	if err != nil {
		t.Fatal(err)
	}
	fee, err := getFeeSatsFromTx(psbtString, rawTxHex)
	if err != nil {
		t.Fatal(err)
	}
	//openingTx := wire.NewMsgTx(2)
	//err = openingTx.Deserialize(bytes.NewReader(finalizeRes.RawFinalTx))
	//if err != nil {
	//	t.Fatal(err)
	//}

	txBytes, err := hex.DecodeString(rawTxHex)
	if err != nil {
		t.Fatal(err)
	}
	openingTx := wire.NewMsgTx(2)
	err = openingTx.Deserialize(bytes.NewReader(txBytes))
	if err != nil {
		t.Fatal(err)
	}

	log.Printf("valid %v, vout %v, fee %v", valid, vout, fee)
	_, err = walletClient.PublishTransaction(ctx, &walletrpc.Transaction{TxHex: txBytes})
	if err != nil {
		t.Fatal(err)
	}
	log.Printf("openingtx %s", openingTx.TxHash().String())

	// create wire msgtx
	openingMsgTx := wire.NewMsgTx(2)
	err = openingMsgTx.Deserialize(bytes.NewReader(finalizeRes.RawFinalTx))
	if err != nil {
		t.Fatal(err)
	}

	// Add Input
	prevHash := openingMsgTx.TxHash()
	prevInput := wire.NewOutPoint(&prevHash, uint32(vout))

	spendingTx := wire.NewMsgTx(2)

	newAddressRes, err := lndClient.NewAddress(ctx, &lnrpc.NewAddressRequest{Type: lnrpc.AddressType_WITNESS_PUBKEY_HASH})
	if err != nil {
		t.Fatal(err)
	}

	scriptChangeAddr, err := btcutil.DecodeAddress(newAddressRes.Address, &chaincfg.RegressionNetParams)
	if err != nil {
		t.Fatal(err)
	}
	scriptChangeAddrScript := scriptChangeAddr.ScriptAddress()
	scriptChangeAddrScriptP2pkh, err := txscript.NewScriptBuilder().AddData([]byte{0x00}).AddData(scriptChangeAddrScript).Script()
	if err != nil {
		t.Fatal(err)
	}
	spendingTxOut := wire.NewTxOut(openingMsgTx.TxOut[vout].Value, scriptChangeAddrScriptP2pkh)
	spendingTx.AddTxOut(spendingTxOut)

	redeemScript, _ := onchain.GetOpeningTxScript(swapParams.AliceKey.PubKey().SerializeCompressed(), swapParams.BobKey.PubKey().SerializeCompressed(), swapParams.PaymentHash, swapParams.Csv)

	spendingTxInput := wire.NewTxIn(prevInput, nil, [][]byte{})
	spendingTxInput.Sequence = 0

	spendingTx.AddTxIn(spendingTxInput)
	txsize := spendingTx.SerializeSizeStripped() + 74
	log.Printf("txsize: %v", txsize)
	satPerByte := float64(7.1)

	spendingTx.TxOut[0].Value = spendingTx.TxOut[0].Value - int64(float64(txsize)*satPerByte)

	sigHashes := txscript.NewTxSigHashes(spendingTx)
	sigHash, err := txscript.CalcWitnessSigHash(redeemScript, sigHashes, txscript.SigHashAll, spendingTx, 0, 10000)
	if err != nil {
		t.Fatal(err)
	}

	sig, err := swapParams.AliceKey.Sign(sigHash[:])
	if err != nil {
		t.Fatal(err)
	}

	preimageBytes := swapParams.Preimage
	//sigWithHashType := append(sig.Serialize(), byte(txscript.SigHashAll))
	//witness := make([][]byte, 0)
	//witness = append(witness, preimageBytes[:])
	//witness = append(witness, sigWithHashType)
	//witness = append(witness, redeemScript)
	preimageWitness := onchain.GetPreimageWitness(sig.Serialize(), preimageBytes[:], redeemScript)
	spendingTx.TxIn[0].Witness = preimageWitness

	bytesBuffer = new(bytes.Buffer)

	err = spendingTx.Serialize(bytesBuffer)
	if err != nil {
		t.Fatal(err)
	}
	spendingTxHex := hex.EncodeToString(bytesBuffer.Bytes())

	_, err = walletClient.PublishTransaction(ctx, &walletrpc.Transaction{TxHex: bytesBuffer.Bytes()})
	if err != nil {
		t.Fatal(err)
	}
	spendingTxId := spendingTx.TxHash()
	log.Printf("spending txid %s \n tx hex %s", spendingTxId, spendingTxHex)
	//openingSendRes, err :=
	//log.Printf("%v",unsignedPacket)
	//bytesBuffer := new(bytes.Buffer)
	//err = unsignedPacker.Serialize(bytesBuffer)
	//if err != nil {
	//	t.Fatal(err)
	//}
	//spendingTxHex := hex.EncodeToString(bytesBuffer.Bytes())

}
func Test_FeeBump(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lndConn, err := getClientConnectenLocal(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	lndClient := lnrpc.NewLightningClient(lndConn)
	walletClient := walletrpc.NewWalletKitClient(lndConn)
	gi, err := lndClient.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	log.Printf("%v", gi)

	outpoint := &lnrpc.OutPoint{
		TxidStr:     "630e1630a395de2b46a230c3bac1b121419ebff2ec1f2bd2d67e05dd6cbed1b9",
		OutputIndex: 0,
	}
	bumpres, err := walletClient.BumpFee(ctx, &walletrpc.BumpFeeRequest{
		Outpoint:    outpoint,
		Force:       false,
		SatPerVbyte: 20,
	})
	log.Printf("%v", bumpres)
}

func Test_BigPayment(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lndConn1, err := getClientConnectenLocal(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	lndClient1 := lnrpc.NewLightningClient(lndConn1)

	lndConn2, err := getClientConnectenLocal(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	lndClient2 := lnrpc.NewLightningClient(lndConn2)
	routerClient2 := routerrpc.NewRouterClient(lndConn2)

	amt := uint64(4300000)
	inv, err := lndClient1.AddInvoice(ctx, &lnrpc.Invoice{
		Value:  4300000,
		Memo:   "big",
		Expiry: 3600})
	if err != nil {
		t.Fatal(err)
	}

	gi1, err := lndClient1.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	pkBytes, err := hex.DecodeString(gi1.IdentityPubkey)
	if err != nil {
		t.Fatal(err)
	}
	channels, err := lndClient2.ListChannels(ctx, &lnrpc.ListChannelsRequest{
		Peer: pkBytes,
	})
	if err != nil {
		t.Fatal(err)
	}

	route, err := routerClient2.BuildRoute(ctx, &routerrpc.BuildRouteRequest{
		AmtMsat:        int64(amt * 1000),
		OutgoingChanId: channels.Channels[0].ChanId,
		HopPubkeys:     [][]byte{pkBytes},
		FinalCltvDelta: 40,
	})

	if err != nil {
		t.Fatal(err)
	}
	log.Printf("route: %v", route)
	sendToRoute, err := routerClient2.SendToRouteV2(ctx, &routerrpc.SendToRouteRequest{
		Route:       route.Route,
		PaymentHash: inv.RHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sendToRoute.Failure != nil {
		log.Printf("%s", sendToRoute.Failure.Code)
	}
	log.Printf("payreq: %v", sendToRoute)
}

// No featurebit pr for now
// func Test_Featurebits(t *testing.T) {
// 	ctx, cancel := context.WithCancel(context.Background())
// 	defer cancel()
// 	lndConn, err := getClientConnectenLocal(ctx, 1)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	lndClient := lnrpc.NewLightningClient(lndConn)

// 	updateFeatures := []*lnrpc.UpdateFeatureAction{
// 		&lnrpc.UpdateFeatureAction{
// 			Action:     lnrpc.UpdateAction_ADD,
// 			FeatureBit: 69,
// 		},
// 	}
// 	color := GetRandomColorInHex()
// 	log.Printf("new color %s", color)
// 	updateres, err := lndClient.UpdateNodeAnnouncement(ctx, &lnrpc.NodeAnnouncementUpdateRequest{
// 		FeatureUpdates: updateFeatures,
// 		Color:          color,
// 	})
// 	//if err != nil {
// 	//	t.Fatal(err)
// 	//}
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	log.Printf("%v", updateres)

// }

func releaseFunds(ctx context.Context, walletClient walletrpc.WalletKitClient) error {
	leases, err := walletClient.ListLeases(ctx, &walletrpc.ListLeasesRequest{})
	if err != nil {
		return err
	}
	for _, v := range leases.LockedUtxos {
		_, err := walletClient.ReleaseOutput(ctx, &walletrpc.ReleaseOutputRequest{Id: v.Id, Outpoint: &lnrpc.OutPoint{TxidStr: v.Outpoint.TxidStr}})
		if err != nil {
			return err
		}
	}
	return nil
}

func createPsbtOutput(params *TxParams) (*wire.TxOut, error) {
	redeemScript, err := onchain.GetOpeningTxScript(params.AliceKey.PubKey().SerializeCompressed(), params.BobKey.PubKey().SerializeCompressed(), params.PaymentHash, params.Csv)
	if err != nil {
		return nil, err
	}
	witnessProgram := sha256.Sum256(redeemScript)
	pkScript := []byte{0x00}
	pkScript = append(pkScript, witnessProgram[:]...)
	output := wire.NewTxOut(int64(params.SwapAmount), pkScript)
	return output, nil
}

func createOpeningAddress(params *TxParams) (string, error) {
	redeemScript, err := onchain.GetOpeningTxScript(params.AliceKey.PubKey().SerializeCompressed(), params.BobKey.PubKey().SerializeCompressed(), params.PaymentHash, params.Csv)
	if err != nil {
		return "", err
	}
	witnessProgram := sha256.Sum256(redeemScript)
	addr, err := btcutil.NewAddressWitnessScriptHash(witnessProgram[:], &chaincfg.RegressionNetParams)
	if err != nil {
		return "", err
	}
	return addr.EncodeAddress(), nil
}

func VerifyTx(txHex string, params *TxParams) (bool, int, error) {
	msgTx := wire.NewMsgTx(2)

	txBytes, err := hex.DecodeString(txHex)
	if err != nil {
		return false, 0, err
	}
	err = msgTx.Deserialize(bytes.NewReader(txBytes))
	if err != nil {
		return false, 0, err
	}

	var scriptOut *wire.TxOut
	var vout int
	for i, out := range msgTx.TxOut {
		if out.Value == int64(params.SwapAmount) {
			scriptOut = out
			vout = i
			break
		}
	}
	if scriptOut == nil {
		return false, 0, err
	}

	redeemScript, err := onchain.GetOpeningTxScript(params.AliceKey.PubKey().SerializeCompressed(), params.BobKey.PubKey().SerializeCompressed(), params.PaymentHash, params.Csv)
	if err != nil {
		return false, 0, err
	}
	witnessProgram := sha256.Sum256(redeemScript)
	addr, err := btcutil.NewAddressWitnessScriptHash(witnessProgram[:], &chaincfg.RegressionNetParams)
	if err != nil {
		return false, 0, err
	}
	wantScript, err := txscript.NewScriptBuilder().AddData([]byte{0x00}).AddData(addr.ScriptAddress()).Script()
	if err != nil {
		return false, 0, err
	}

	if bytes.Compare(wantScript, scriptOut.PkScript) != 0 {
		return false, 0, err
	}
	return true, vout, nil
}

type TxParams struct {
	AliceKey    *btcec.PrivateKey
	BobKey      *btcec.PrivateKey
	Preimage    lightning.Preimage
	PaymentHash []byte
	SwapAmount  uint64
	Csv         uint32
}

func NewTxParams(csv uint32, amount uint64) *TxParams {
	preimage, _ := lightning.GetPreimage()
	pHash := preimage.Hash()
	return &TxParams{
		AliceKey:    getRandomPrivkey(),
		BobKey:      getRandomPrivkey(),
		Preimage:    preimage,
		PaymentHash: pHash[:],
		Csv:         csv,
		SwapAmount:  amount,
	}
}

func getRandomPrivkey() *btcec.PrivateKey {
	privkey, err := btcec.NewPrivateKey(btcec.S256())
	if err != nil {
		return nil
	}
	return privkey
}

func (t *TxParams) recalcHash() {
	pHash := t.Preimage.Hash()
	t.PaymentHash = pHash[:]
}

func getFeeSatsFromTx(psbtString, txHex string) (int64, error) {
	rawPsbt, err := psbt.NewFromRawBytes(bytes.NewReader([]byte(psbtString)), true)
	if err != nil {
		return 0, err
	}
	inputSats, err := psbt.SumUtxoInputValues(rawPsbt)
	if err != nil {
		return 0, err
	}
	log.Println(inputSats)
	txBytes, err := hex.DecodeString(txHex)
	if err != nil {
		return 0, err
	}

	tx, err := btcutil.NewTxFromBytes(txBytes)
	if err != nil {
		return 0, err
	}

	outputSats := int64(0)
	for _, out := range tx.MsgTx().TxOut {
		outputSats += out.Value
	}

	return inputSats - outputSats, nil
}

func getVoutAndVerify(txHex string, params *TxParams) (bool, uint32, error) {
	msgTx := wire.NewMsgTx(2)

	txBytes, err := hex.DecodeString(txHex)
	if err != nil {
		return false, 0, err
	}
	err = msgTx.Deserialize(bytes.NewReader(txBytes))
	if err != nil {
		return false, 0, err
	}

	var scriptOut *wire.TxOut
	var vout uint32
	for i, out := range msgTx.TxOut {
		if out.Value == int64(params.SwapAmount) {
			scriptOut = out
			vout = uint32(i)
			break
		}
	}
	if scriptOut == nil {
		return false, 0, err
	}

	redeemScript, err := onchain.GetOpeningTxScript(params.AliceKey.PubKey().SerializeCompressed(), params.BobKey.PubKey().SerializeCompressed(), params.PaymentHash, uint32(params.Csv))
	if err != nil {
		return false, 0, err
	}
	witnessProgram := sha256.Sum256(redeemScript)
	addr, err := btcutil.NewAddressWitnessScriptHash(witnessProgram[:], &chaincfg.RegressionNetParams)
	if err != nil {
		return false, 0, err
	}
	wantScript, err := txscript.NewScriptBuilder().AddData([]byte{0x00}).AddData(addr.ScriptAddress()).Script()
	if err != nil {
		return false, 0, err
	}

	if bytes.Compare(wantScript, scriptOut.PkScript) != 0 {
		return false, vout, nil
	}
	return true, vout, nil
}

func getBitcoinClient() (*gbitcoin.Bitcoin, error) {
	bitcoin := gbitcoin.NewBitcoin("admin1", "123")
	bitcoin.SetTimeout(1)
	rpcPort, err := strconv.Atoi("18443")
	if err != nil {
		return nil, err
	}
	err = bitcoin.StartUp("http://localhost", "", uint(rpcPort))
	if err != nil {
		return nil, err
	}
	return bitcoin, nil
}

// gets the lnd grpc connection
func getClientConnectenLocal(ctx context.Context, lndNumber int) (*grpc.ClientConn, error) {
	homeDir := fmt.Sprintf("lnd-regtest-%v", lndNumber)

	tlsCertPath := filepath.Join("/tmp", homeDir, "tls.cert")
	macaroonPath := filepath.Join("/tmp", homeDir, "/data/chain/bitcoin/regtest/admin.macaroon")
	address := fmt.Sprintf("localhost:%v", 10101+lndNumber*100)
	log.Printf("tlsCertPath: %s, macpath %s, address %s ", tlsCertPath, macaroonPath, address)
	maxMsgRecvSize := grpc.MaxCallRecvMsgSize(1 * 1024 * 1024 * 500)

	creds, err := credentials.NewClientTLSFromFile(tlsCertPath, "")
	if err != nil {
		return nil, err
	}
	macBytes, err := ioutil.ReadFile(macaroonPath)
	if err != nil {
		return nil, err
	}
	mac := &macaroon.Macaroon{}
	if err := mac.UnmarshalBinary(macBytes); err != nil {
		return nil, err
	}
	cred, err := macaroons.NewMacaroonCredential(mac)
	if err != nil {
		return nil, err
	}
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithBlock(),
		grpc.WithPerRPCCredentials(cred),
		grpc.WithDefaultCallOptions(maxMsgRecvSize),
	}
	conn, err := grpc.DialContext(ctx, address, opts...)
	if err != nil {
		return nil, err
	}
	return conn, nil

}
