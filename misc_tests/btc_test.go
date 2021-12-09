//go:build misc
// +build misc

package misc_tests
//
//import (
//	"bytes"
//	"crypto/sha256"
//	"encoding/binary"
//	"encoding/hex"
//	"encoding/json"
//	"errors"
//	"github.com/sputn1ck/peerswap/swap"
//	"log"
//	"strconv"
//	"testing"
//
//	"github.com/btcsuite/btcd/btcec"
//	"github.com/btcsuite/btcd/chaincfg"
//	"github.com/btcsuite/btcd/txscript"
//	"github.com/btcsuite/btcd/wire"
//	"github.com/btcsuite/btcutil"
//	"github.com/btcsuite/btcutil/psbt"
//	"github.com/sputn1ck/glightning/gbitcoin"
//	"github.com/sputn1ck/glightning/glightning"
//	"github.com/sputn1ck/peerswap/lightning"
//	"github.com/sputn1ck/peerswap/onchain"
//)
//
//// btc swap
//// step 1: create opening tx segwit addresss
//// step 2: tx prepare to opening tx
//// step 3: get fee from prepared opening tx
//// step 4: tx send prepared opening tx
//// step 5: wait for confs
//// step 6: getnewaddress
//// step 7: build claim tx
//// step 8: send claim tx
//
//func Test_Sequence(t *testing.T) {
//	a := 0x13C7 | 0x400000
//	bs := make([]byte, 32)
//	binary.LittleEndian.PutUint32(bs, uint32(a))
//
//	log.Printf("%x", bs)
//	buf := new(bytes.Buffer)
//	err := binary.Write(buf, binary.LittleEndian, bs)
//	if err != nil {
//		t.Fatal(err)
//	}
//	log.Printf("%v", a)
//	log.Printf("%x", buf.Bytes())
//}
//func Test_BitcoinSwapPreimage(t *testing.T) {
//	lcli, err := getLightningClient()
//	if err != nil {
//		t.Fatal(err)
//	}
//	bitcoin, err := getBitcoinClient(lcli)
//	if err != nil {
//		t.Fatal(err)
//	}
//	_, err = bitcoin.GetChainInfo()
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	bitcoinOnchain := onchain.NewBitcoinOnChain(bitcoin, nil, lcli, &chaincfg.RegressionNetParams)
//
//	txParams := NewTxParams(uint32(100))
//	txParams.SwapAmount = 10000
//
//	openingParams := &swap.OpeningParams{
//		TakerPubkeyHash:  hex.EncodeToString(txParams.AliceKey.PubKey().SerializeCompressed()),
//		MakerPubkeyHash:  hex.EncodeToString(txParams.BobKey.PubKey().SerializeCompressed()),
//		ClaimPaymentHash: hex.EncodeToString(txParams.PaymentHash),
//		Amount:           txParams.SwapAmount,
//	}
//	log.Printf("%s %s %s", openingParams.TakerPubkeyHash, openingParams.MakerPubkeyHash, openingParams.ClaimPaymentHash)
//	unpreppedtxHex, _, _, cltv, _, err := bitcoinOnchain.CreateOpeningTransaction(openingParams)
//	if err != nil {
//		t.Fatal(err)
//	}
//	log.Printf("%v", cltv)
//
//	openingTxId, _, err := bitcoinOnchain.BroadcastOpeningTx(unpreppedtxHex)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	_, err = bitcoin.GenerateToAddress("2NDsRVXmnw3LFZ12rTorcKrBiAvX54LkTn1", 3)
//	if err != nil {
//		t.Fatal(err)
//	}
//	claimParams := &swap.ClaimParams{
//		Preimage: txParams.Preimage.String(),
//		Signer:   txParams.AliceKey,
//	}
//	claimTxId, _, err := bitcoinOnchain.CreatePreimageSpendingTransaction(openingParams, claimParams, openingTxId)
//	if err != nil {
//		t.Fatal(err)
//	}
//	log.Printf("%s", claimTxId)
//	_, err = bitcoin.GenerateToAddress("2NDsRVXmnw3LFZ12rTorcKrBiAvX54LkTn1", 3)
//	if err != nil {
//		t.Fatal(err)
//	}
//}
//func Test_BitcoinSwapCsv(t *testing.T) {
//	lcli, err := getLightningClient()
//	if err != nil {
//		t.Fatal(err)
//	}
//	bitcoin, err := getBitcoinClient(lcli)
//	if err != nil {
//		t.Fatal(err)
//	}
//	_, err = bitcoin.GetChainInfo()
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	bitcoinOnchain := onchain.NewBitcoinOnChain(bitcoin, nil, lcli, &chaincfg.RegressionNetParams)
//
//	txParams := NewTxParams(uint32(100))
//	txParams.SwapAmount = 10000
//
//	openingParams := &swap.OpeningParams{
//		TakerPubkeyHash:  hex.EncodeToString(txParams.AliceKey.PubKey().SerializeCompressed()),
//		MakerPubkeyHash:  hex.EncodeToString(txParams.BobKey.PubKey().SerializeCompressed()),
//		ClaimPaymentHash: hex.EncodeToString(txParams.PaymentHash),
//		Amount:           txParams.SwapAmount,
//	}
//	unpreppedtxHex, _, _, cltv, vout, err := bitcoinOnchain.CreateOpeningTransaction(openingParams)
//	if err != nil {
//		t.Fatal(err)
//	}
//	log.Printf("%v", cltv)
//
//	_, openingTxHex, err := bitcoinOnchain.BroadcastOpeningTx(unpreppedtxHex)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	_, err = bitcoin.GenerateToAddress("2NDsRVXmnw3LFZ12rTorcKrBiAvX54LkTn1", uint(cltv))
//	if err != nil {
//		t.Fatal(err)
//	}
//	claimParams := &swap.ClaimParams{
//		Preimage: txParams.Preimage.String(),
//		Signer:   txParams.BobKey,
//	}
//	claimTxId, _, err := bitcoinOnchain.CreateCsvSpendingTransaction(openingParams, claimParams, openingTxHex, vout)
//	if err != nil {
//		t.Fatal(err)
//	}
//	log.Printf("%s", claimTxId)
//}
//func Test_BitcoinSwapCooperative(t *testing.T) {
//	lcli, err := getLightningClient()
//	if err != nil {
//		t.Fatal(err)
//	}
//	bitcoin, err := getBitcoinClient(lcli)
//	if err != nil {
//		t.Fatal(err)
//	}
//	_, err = bitcoin.GetChainInfo()
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	bitcoinOnchain := onchain.NewBitcoinOnChain(bitcoin, nil, lcli, &chaincfg.RegressionNetParams)
//
//	txParams := NewTxParams(uint32(100))
//	txParams.SwapAmount = 10000
//
//	openingParams := &swap.OpeningParams{
//		TakerPubkeyHash:  hex.EncodeToString(txParams.AliceKey.PubKey().SerializeCompressed()),
//		MakerPubkeyHash:  hex.EncodeToString(txParams.BobKey.PubKey().SerializeCompressed()),
//		ClaimPaymentHash: hex.EncodeToString(txParams.PaymentHash),
//		Amount:           txParams.SwapAmount,
//	}
//	unpreppedtxHex, _, _, cltv, vout, err := bitcoinOnchain.CreateOpeningTransaction(openingParams)
//	if err != nil {
//		t.Fatal(err)
//	}
//	log.Printf("%v", cltv)
//
//	openingTxId, openingTxHex, err := bitcoinOnchain.BroadcastOpeningTx(unpreppedtxHex)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	_, err = bitcoin.GenerateToAddress("2NDsRVXmnw3LFZ12rTorcKrBiAvX54LkTn1", 3)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	takerClaimParams := &swap.ClaimParams{
//		Preimage: txParams.Preimage.String(),
//		Signer:   txParams.AliceKey,
//	}
//	makerClaimParams := &swap.ClaimParams{
//		Preimage: txParams.Preimage.String(),
//		Signer:   txParams.BobKey,
//	}
//
//	refundAddr, err := lcli.NewAddr()
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	takerSigHashString, err := bitcoinOnchain.TakerCreateCoopSigHash(openingParams, takerClaimParams, openingTxId, refundAddr)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	claimTxId, _, err := bitcoinOnchain.CreateCooperativeSpendingTransaction(openingParams, makerClaimParams, refundAddr, openingTxHex, vout, takerSigHashString)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	log.Printf("%s", claimTxId)
//	_, err = bitcoin.GenerateToAddress("2NDsRVXmnw3LFZ12rTorcKrBiAvX54LkTn1", 3)
//	if err != nil {
//		t.Fatal(err)
//	}
//}
//func Test_BitcoinSwap(t *testing.T) {
//	lcli, err := getLightningClient()
//	if err != nil {
//		t.Fatal(err)
//	}
//	bitcoin, err := getBitcoinClient(lcli)
//	if err != nil {
//		t.Fatal(err)
//	}
//	ci, err := bitcoin.GetChainInfo()
//	if err != nil {
//		t.Fatal(err)
//	}
//	log.Printf("%v", ci)
//	funds, err := lcli.ListFunds()
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	log.Println(funds)
//
//	txParams := NewTxParams(uint32(100))
//	txParams.SwapAmount = 10000
//
//	addr, err := createOpeningAddress(txParams)
//	if err != nil {
//		t.Fatal(err)
//	}
//	log.Println(addr)
//	outputs := []*glightning.Outputs{
//		&glightning.Outputs{
//			Address: addr,
//			Satoshi: txParams.SwapAmount,
//		},
//	}
//	prepRes, err := lcli.PrepareTx(outputs, &glightning.FeeRate{Directive: glightning.Urgent}, nil)
//	if err != nil {
//		t.Fatal(err)
//	}
//	_, scriptVout, err := VerifyTx(prepRes.UnsignedTx, txParams)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	log.Printf("scriptVout %v", scriptVout)
//	feeSats, err := getFeeSatsFromTx(prepRes.Psbt, prepRes.UnsignedTx)
//	if err != nil {
//		t.Fatal(err)
//	}
//	log.Printf("\n txid: %s txhex %s", prepRes.TxId, prepRes.UnsignedTx)
//	log.Printf("\n feeSats :  %d", feeSats)
//	sendRes, err := lcli.SendTx(prepRes.TxId)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	log.Printf("\n sendRes: %s", sendRes.TxId)
//
//	// decode txbytes
//	txBytes, err := hex.DecodeString(sendRes.SignedTx)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	ok, _, err := VerifyTx(sendRes.SignedTx, txParams)
//	if err != nil {
//		t.Fatal(err)
//	}
//	if !ok {
//		t.Fatal(errors.New("tx should be valid"))
//	}
//	// create wire msgtx
//	openingMsgTx := wire.NewMsgTx(2)
//	err = openingMsgTx.Deserialize(bytes.NewReader(txBytes))
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	// Add Input
//	prevHash := openingMsgTx.TxHash()
//	prevInput := wire.NewOutPoint(&prevHash, uint32(scriptVout))
//
//	// create spendingTx
//	spendingTx := wire.NewMsgTx(2)
//	spendingTx.LockTime = ci.Blocks
//
//	// Add Output
//	newAddr, err := lcli.NewAddr()
//	if err != nil {
//		t.Fatal(err)
//	}
//	log.Printf("%s", newAddr)
//	scriptChangeAddr, err := btcutil.DecodeAddress(newAddr, &chaincfg.RegressionNetParams)
//	if err != nil {
//		t.Fatal(err)
//	}
//	scriptChangeAddrScript := scriptChangeAddr.ScriptAddress()
//	scriptChangeAddrScriptP2pkh, err := txscript.NewScriptBuilder().AddData([]byte{0x00}).AddData(scriptChangeAddrScript).Script()
//	if err != nil {
//		t.Fatal(err)
//	}
//	spendingTxOut := wire.NewTxOut(openingMsgTx.TxOut[scriptVout].Value, scriptChangeAddrScriptP2pkh)
//	spendingTx.AddTxOut(spendingTxOut)
//
//	redeemScript, _ := utils.GetOpeningTxScript(txParams.AliceKey.PubKey().SerializeCompressed(), txParams.BobKey.PubKey().SerializeCompressed(), txParams.PaymentHash, txParams.Csv)
//
//	spendingTxInput := wire.NewTxIn(prevInput, nil, [][]byte{})
//	spendingTxInput.Sequence = 0
//
//	spendingTx.AddTxIn(spendingTxInput)
//	txsize := spendingTx.SerializeSizeStripped() + 74
//	log.Printf("txsize: %v", txsize)
//	satPerByte := float64(7.1)
//
//	spendingTx.TxOut[0].Value = spendingTx.TxOut[0].Value - int64(float64(txsize)*satPerByte)
//
//	sigHashes := txscript.NewTxSigHashes(spendingTx)
//	sigHash, err := txscript.CalcWitnessSigHash(redeemScript, sigHashes, txscript.SigHashAll, spendingTx, 0, int64(txParams.SwapAmount))
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	takerSig, err := txParams.AliceKey.Sign(sigHash[:])
//	if err != nil {
//		t.Fatal(err)
//	}
//	//makerSig, err := txParams.BobKey.Sign(sigHash[:])
//	//if err != nil {
//	//	t.Fatal(err)
//	//}
//
//	//preimageWitness := utils.GetCsvWitness(sig.Serialize(),  redeemScript)
//	spendingTx.TxIn[0].Witness = utils.GetPreimageWitness(takerSig.Serialize(), txParams.Preimage[:], redeemScript)
//	//spendingTx.TxIn[0].Witness = utils.GetCooperativeWitness(takerSig.Serialize(),makerSig.Serialize(),redeemScript)
//	bytesBuffer := new(bytes.Buffer)
//
//	err = spendingTx.Serialize(bytesBuffer)
//	if err != nil {
//		t.Fatal(err)
//	}
//	spendingTxHex := hex.EncodeToString(bytesBuffer.Bytes())
//
//	log.Printf("%s", spendingTxHex)
//	//txSize := tx.MsgTx().SerializeSize()
//	//feerate := float64(feeSats) / float64(txSize)
//
//	sendRawRes, err := bitcoin.SendRawTx(spendingTxHex)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	log.Printf("%s", sendRawRes)
//}
//
//func Test_PrintBytes(t *testing.T) {
//	// print 2 privkeys
//	for i := 0; i < 2; i++ {
//		privkey, _ := btcec.NewPrivateKey(btcec.S256())
//		t.Logf("privkey %v: %s", i+1, hex.EncodeToString(privkey.Serialize()))
//	}
//	// print preimageString
//	preimage, _ := lightning.GetPreimage()
//	t.Logf("%s ", preimage.String())
//}
//
//var privkeyAlice = "de3ed9555a4e245a4bb4dda2e3cdd08506798d9450d0bf9fef640a285cc9804c"
//var privkeyBob = "bd2ba3c638e0bcde93398d535c727e28672e1d98398c1cba9dd863a79ade3d31"
//var preimageString = "d61dafec41bbdd42898f0ce2283128df665a152d35211f2c7e153dcf4ab3415b"
//
//func getFixedSwapParams() (*btcec.PrivateKey, *btcec.PrivateKey, lightning.Preimage) {
//	privkeyAliceBytes, _ := hex.DecodeString(privkeyAlice)
//	pvAlice, _ := btcec.PrivKeyFromBytes(btcec.S256(), privkeyAliceBytes)
//	privkeyBobBytes, _ := hex.DecodeString(privkeyBob)
//	pvBob, _ := btcec.PrivKeyFromBytes(btcec.S256(), privkeyBobBytes)
//
//	preimage, _ := lightning.MakePreimageFromStr(preimageString)
//	return pvAlice, pvBob, preimage
//}
//
//func createOpeningAddress(params *TxParams) (string, error) {
//	redeemScript, err := utils.GetOpeningTxScript(params.AliceKey.PubKey().SerializeCompressed(), params.BobKey.PubKey().SerializeCompressed(), params.PaymentHash, params.Csv)
//	if err != nil {
//		return "", err
//	}
//	witnessProgram := sha256.Sum256(redeemScript)
//	addr, err := btcutil.NewAddressWitnessScriptHash(witnessProgram[:], &chaincfg.RegressionNetParams)
//	if err != nil {
//		return "", err
//	}
//	return addr.EncodeAddress(), nil
//}
//
//var openingTxHex = "0200000001b4edf4891e095bc20084c9c39978616f30843f7c43849d1076dd81a9608b42a20000000000fdffffff02f723052a01000000160014817bc281a4cbc3b2daedea54a869cc1d8adfdc271027000000000000220020bdbd3cd7ff7f249eb7bfc248e023b3d17d6647c31cd5227e6ca55de2f5737d7396030000"
//
//var rawTx = "020000000193eb2b13a1112e970e4bb817f5cb7125703d79b9744f6c2ddf40566f5d9526fb0100000000ffffffff011027000000000000160014817bc281a4cbc3b2daedea54a869cc1d8adfdc2700000000"
//
//func VerifyTx(txHex string, params *TxParams) (bool, int, error) {
//	msgTx := wire.NewMsgTx(2)
//
//	txBytes, err := hex.DecodeString(txHex)
//	if err != nil {
//		return false, 0, err
//	}
//	err = msgTx.Deserialize(bytes.NewReader(txBytes))
//	if err != nil {
//		return false, 0, err
//	}
//
//	var scriptOut *wire.TxOut
//	var vout int
//	for i, out := range msgTx.TxOut {
//		if out.Value == int64(params.SwapAmount) {
//			scriptOut = out
//			vout = i
//			break
//		}
//	}
//	if scriptOut == nil {
//		return false, 0, err
//	}
//
//	redeemScript, err := utils.GetOpeningTxScript(params.AliceKey.PubKey().SerializeCompressed(), params.BobKey.PubKey().SerializeCompressed(), params.PaymentHash, params.Csv)
//	if err != nil {
//		return false, 0, err
//	}
//	witnessProgram := sha256.Sum256(redeemScript)
//	addr, err := btcutil.NewAddressWitnessScriptHash(witnessProgram[:], &chaincfg.RegressionNetParams)
//	if err != nil {
//		return false, 0, err
//	}
//	wantScript, err := txscript.NewScriptBuilder().AddData([]byte{0x00}).AddData(addr.ScriptAddress()).Script()
//	if err != nil {
//		return false, 0, err
//	}
//
//	if bytes.Compare(wantScript, scriptOut.PkScript) != 0 {
//		return false, 0, err
//	}
//	return true, vout, nil
//}
//
//type TxParams struct {
//	AliceKey    *btcec.PrivateKey
//	BobKey      *btcec.PrivateKey
//	Preimage    lightning.Preimage
//	PaymentHash []byte
//	SwapAmount  uint64
//	Csv         uint32
//}
//
//func NewTxParams(csv uint32) *TxParams {
//	preimage, _ := lightning.GetPreimage()
//	pHash := preimage.Hash()
//	return &TxParams{
//		AliceKey:    getRandomPrivkey(),
//		BobKey:      getRandomPrivkey(),
//		Preimage:    preimage,
//		PaymentHash: pHash[:],
//		Csv:         csv,
//	}
//}
//
//func (t *TxParams) recalcHash() {
//	pHash := t.Preimage.Hash()
//	t.PaymentHash = pHash[:]
//}
//func getFeeSatsFromTx(psbtString, txHex string) (int64, error) {
//	rawPsbt, err := psbt.NewFromRawBytes(bytes.NewReader([]byte(psbtString)), true)
//	if err != nil {
//		return 0, err
//	}
//	inputSats, err := psbt.SumUtxoInputValues(rawPsbt)
//	if err != nil {
//		return 0, err
//	}
//	log.Println(inputSats)
//	txBytes, err := hex.DecodeString(txHex)
//	if err != nil {
//		return 0, err
//	}
//
//	tx, err := btcutil.NewTxFromBytes(txBytes)
//	if err != nil {
//		return 0, err
//	}
//
//	outputSats := int64(0)
//	for _, out := range tx.MsgTx().TxOut {
//		outputSats += out.Value
//	}
//
//	return inputSats - outputSats, nil
//}
//
//func getLightningClient() (*glightning.Lightning, error) {
//	l := glightning.NewLightning()
//	err := l.StartUp("lightning-rpc", "/tmp/l1-regtest/regtest")
//
//	return l, err
//}
//
//func getBitcoinClient(li *glightning.Lightning) (*gbitcoin.Bitcoin, error) {
//	configs, err := li.ListConfigs()
//	if err != nil {
//		return nil, err
//	}
//	jsonString, err := json.Marshal(configs)
//	if err != nil {
//		return nil, err
//	}
//	var listconfigRes *ListConfigRes
//	err = json.Unmarshal(jsonString, &listconfigRes)
//	if err != nil {
//		return nil, err
//	}
//	var bcliConfig *ImportantPlugin
//	for _, v := range listconfigRes.ImportantPlugins {
//		if v.Name == "bcli" {
//			bcliConfig = v
//		}
//	}
//	if bcliConfig == nil {
//		return nil, errors.New("bcli config not found")
//	}
//
//	bitcoin := gbitcoin.NewBitcoin(bcliConfig.Options["bitcoin-rpcuser"], bcliConfig.Options["bitcoin-rpcpassword"])
//	bitcoin.SetTimeout(1)
//	rpcPort, err := strconv.Atoi(bcliConfig.Options["bitcoin-rpcport"])
//	if err != nil {
//		return nil, err
//	}
//	bitcoin.StartUp("http://"+bcliConfig.Options["bitcoin-rpcconnect"], "", uint(rpcPort))
//	return bitcoin, nil
//}
//
//type ListConfigRes struct {
//	ImportantPlugins []*ImportantPlugin `json:"important-plugins"`
//}
//
//type ImportantPlugin struct {
//	Path    string
//	Name    string
//	Options map[string]string
//}
//
//// getRandomPrivkey returns a random private key for the swap
//func getRandomPrivkey() *btcec.PrivateKey {
//	privkey, err := btcec.NewPrivateKey(btcec.S256())
//	if err != nil {
//		return nil
//	}
//	return privkey
//}
