package tests

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcutil/psbt"
	"github.com/sputn1ck/glightning/gbitcoin"
	"github.com/sputn1ck/glightning/glightning"
	"github.com/sputn1ck/peerswap/lightning"
	"github.com/sputn1ck/peerswap/utils"
	"github.com/vulpemventures/go-elements/address"
	"github.com/vulpemventures/go-elements/payment"
	"github.com/vulpemventures/go-elements/transaction"
	"log"
	"strconv"
	"testing"
)


// btc swap
// step 1: create opening tx segwit addresss
// step 2: tx prepare to opening tx
// step 3: get fee from prepared opening tx
// step 4: tx send prepared opening tx
// step 5: wait for confs
// step 6: getnewaddress
// step 7: build claim tx
// step 8: send claim tx
func Test_BitcoinSwap(t *testing.T) {
	lcli, err := getLightningClient()
	if err != nil {
		t.Fatal(err)
	}
	bitcoin, err := getBitcoinClient(lcli)
	if err != nil {
		t.Fatal(err)
	}
	ci, err := bitcoin.GetChainInfo()
	if err != nil {
		t.Fatal(err)
	}
	log.Printf("%v", ci)
	funds, err := lcli.ListFunds()
	if err != nil {
		t.Fatal(err)
	}

	log.Println(funds)

	txParams := NewTxParams(uint64(100))


	addr, err := createOpeningAddress(txParams)
	if err != nil {
		t.Fatal(err)
	}
	log.Println(addr)
	outputs := []*glightning.Outputs{
		&glightning.Outputs{
			Address: addr,
			Satoshi: 10000,
		},
	}
	confs := uint16(1)
	prepRes, err := lcli.PrepareTx(outputs, &glightning.FeeRate{Directive: glightning.Urgent}, &confs)
	if err != nil {
		t.Fatal(err)
	}
	//msgtx := &wire.MsgTx{}
	//err = msgtx.Deserialize(bytes.NewReader([]byte(prepRes.Tx)))
	//if err != nil {
	//	t.Fatal(err)
	//}

	feeSats, err := getFeeSatsFromTx(prepRes.Psbt, prepRes.UnsignedTx)
	if err != nil {
		t.Fatal(err)
	}
	log.Printf("\n txid: %s txhex %s", prepRes.TxId, prepRes.UnsignedTx)
	log.Printf("\n feeSats :  %d", feeSats)
	sendRes, err := lcli.SendTx(prepRes.TxId)
	if err != nil {
		t.Fatal(err)
	}


	log.Printf("\n sendRes: %s", sendRes.TxId)


	// decode txbytes
	txBytes, err := hex.DecodeString(sendRes.SignedTx)
	if err != nil {
		t.Fatal(err)
	}

	// create wire msgtx
	openingMsgTx := wire.NewMsgTx(2)
	err = openingMsgTx.Deserialize(bytes.NewReader(txBytes))
	if err != nil {
		t.Fatal(err)
	}
	// Find Vout
	scriptVout := 0
	changeVout := 0
	for index,v := range openingMsgTx.TxOut {
		if v.Value == 10000{
			scriptVout = index
		}
	}
	if scriptVout == 0 {
		changeVout = 1
	}
	// Add Input
	prevHash := openingMsgTx.TxHash()
	prevInput := wire.NewOutPoint(&prevHash, uint32(scriptVout))

	// create spendingTx
	spendingTx := wire.NewMsgTx(2)
	spendingTx.LockTime = ci.Blocks

	// Add Output
	spendingTxOut := wire.NewTxOut(openingMsgTx.TxOut[scriptVout].Value - 200, openingMsgTx.TxOut[changeVout].PkScript)
	spendingTx.AddTxOut(spendingTxOut)


	redeemScript,_ := utils.GetOpeningTxScript(txParams.AliceKey.PubKey().SerializeCompressed(), txParams.BobKey.PubKey().SerializeCompressed(), txParams.PaymentHash, int64(txParams.Cltv))


	spendingTxInput := wire.NewTxIn(prevInput, nil, [][]byte{})
	spendingTxInput.Sequence = 0

	spendingTx.AddTxIn(spendingTxInput)

	sigHashes := txscript.NewTxSigHashes(spendingTx)
	sigHash,err := txscript.CalcWitnessSigHash(redeemScript, sigHashes, txscript.SigHashAll, spendingTx, 0, 10000)
	if err != nil {
		t.Fatal(err)
	}

	sig, err := txParams.AliceKey.Sign(sigHash[:])
	if err != nil {
		t.Fatal(err)
	}

	preimageBytes := txParams.Preimage
	sigWithHashType := append(sig.Serialize(), byte(txscript.SigHashAll))
	witness := make([][]byte, 0)
	witness = append(witness, preimageBytes[:])
	witness = append(witness, sigWithHashType)
	witness = append(witness, redeemScript)
	//preimageWitness := utils.GetCltvWitness(sig.Serialize(),  redeemScript)
	spendingTx.TxIn[0].Witness = witness

	bytesBuffer := new(bytes.Buffer)

	err = spendingTx.Serialize(bytesBuffer)
	if err != nil {
		t.Fatal(err)
	}
	spendingTxHex := hex.EncodeToString(bytesBuffer.Bytes())
	log.Printf("%s", spendingTxHex)
	//txSize := tx.MsgTx().SerializeSize()
	//feerate := float64(feeSats) / float64(txSize)

	sendRawRes, err := bitcoin.SendRawTx(spendingTxHex)
	if err != nil {
		t.Fatal(err)
	}

	log.Printf("%s", sendRawRes)
}

func Test_PrintBytes(t *testing.T) {
	// print 2 privkeys
	for i := 0; i < 2; i++ {
		privkey,_ := btcec.NewPrivateKey(btcec.S256())
		t.Logf("privkey %v: %s",i+1, hex.EncodeToString(privkey.Serialize()))
	}
	// print preimageString
	preimage, _ := lightning.GetPreimage()
	t.Logf("%s ",preimage.String())
}
var privkeyAlice = "de3ed9555a4e245a4bb4dda2e3cdd08506798d9450d0bf9fef640a285cc9804c"
var privkeyBob = "bd2ba3c638e0bcde93398d535c727e28672e1d98398c1cba9dd863a79ade3d31"
var preimageString = "d61dafec41bbdd42898f0ce2283128df665a152d35211f2c7e153dcf4ab3415b"

func getFixedSwapParams() (*btcec.PrivateKey, *btcec.PrivateKey, lightning.Preimage) {
	privkeyAliceBytes, _ := hex.DecodeString(privkeyAlice)
	pvAlice,_ := btcec.PrivKeyFromBytes(btcec.S256(), privkeyAliceBytes)
	privkeyBobBytes, _ := hex.DecodeString(privkeyBob)
	pvBob,_ := btcec.PrivKeyFromBytes(btcec.S256(), privkeyBobBytes)

	preimage, _ := lightning.MakePreimageFromStr(preimageString)
	return pvAlice, pvBob, preimage
}
func createOpeningAddress(params *TxParams) (string, error) {
	redeemScript,err := utils.GetOpeningTxScript(params.AliceKey.PubKey().SerializeCompressed(), params.BobKey.PubKey().SerializeCompressed(), params.PaymentHash, 100)
	if err != nil {
		return "", err
	}
	scriptPubKey := []byte{0x00, 0x20}
	witnessProgram := sha256.Sum256(redeemScript)
	scriptPubKey = append(scriptPubKey, witnessProgram[:]...)
	redeemPayment, err := payment.FromScript(scriptPubKey, nil, nil)
	if err != nil {
		return "", err
	}
	version := byte(0x00)
	payload := &address.Bech32{"bcrt", version, redeemPayment.WitnessHash}
	addr, err := address.ToBech32(payload)
	if err != nil {
		return "", nil
	}
	return addr, nil
}

var openingTxHex = "0200000001b4edf4891e095bc20084c9c39978616f30843f7c43849d1076dd81a9608b42a20000000000fdffffff02f723052a01000000160014817bc281a4cbc3b2daedea54a869cc1d8adfdc271027000000000000220020bdbd3cd7ff7f249eb7bfc248e023b3d17d6647c31cd5227e6ca55de2f5737d7396030000"
//func Test_SpendingTx(t *testing.T) {
//
//	// decode txbytes
//	txBytes, err := hex.DecodeString(openingTxHex)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	// create wire msgtx
//	openingMsgTx := wire.NewMsgTx(2)
//	err = openingMsgTx.Deserialize(bytes.NewReader(txBytes))
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	// create spendingTx
//	spendingTx := wire.NewMsgTx(2)
//
//	// Add Output
//	spendingTxOut := wire.NewTxOut(openingMsgTx.TxOut[scriptVou].Value - 200, openingMsgTx.TxOut[0].PkScript)
//	spendingTx.AddTxOut(spendingTxOut)
//
//
//	// Add Input
//	prevHash := openingMsgTx.TxHash()
//	prevInput := wire.NewOutPoint(&prevHash, 1)
//	scriptPubKey := []byte{0x00, 0x20}
//	redeemSript,_ := utils.GetOpeningTxScript([]byte("aa"),[]byte("aa"),[]byte("aa"),100)
//	witnessProgram := sha256.Sum256(redeemSript)
//	scriptPubKey = append(scriptPubKey, witnessProgram[:]...)
//	spendingTxInput := wire.NewTxIn(prevInput, scriptPubKey, [][]byte{})
//	spendingTx.AddTxIn(spendingTxInput)
//
//
//	bytesBuffer := new(bytes.Buffer)
//	//writer := bufio.NewWriter()
//	err = spendingTx.Serialize(bytesBuffer)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	log.Printf("%s", hex.EncodeToString(bytesBuffer.Bytes()))
//
//}
var rawTx = "020000000193eb2b13a1112e970e4bb817f5cb7125703d79b9744f6c2ddf40566f5d9526fb0100000000ffffffff011027000000000000160014817bc281a4cbc3b2daedea54a869cc1d8adfdc2700000000"
func createSpendingTxPreimage(params *TxParams, openingTxHex string, currentBlock uint64) (tx *transaction.Transaction, sigHash [32]byte, err error) {
	//redeemScript,err := utils.GetOpeningTxScript(params.AliceKey.PubKey().SerializeCompressed(), params.BobKey.PubKey().SerializeCompressed(), params.PaymentHash, int64(params.Cltv))
	//if err != nil {
	//	return nil, [32]byte{}, err
	//}

	//firstTx, err := transaction.NewTxFromHex(openingTxHex)
	//if err != nil {
	//	log.Printf("error creating first tx %s, %v", openingTxHex, err)
	//	return nil, [32]byte{}, err
	//}


	//swapInValue, err := elementsutil.SatoshiToElementsValue(params.SwapAmount)
	//if err != nil {
	//	log.Printf("error getting swapin value")
	//	return nil, [32]byte{}, err
	//}
	//vout, err := utils.FindVout(firstTx.Outputs, redeemScript)
	//if err != nil {
	//	log.Printf("error finding vour")
	//	return nil, [32]byte{}, err
	//}
	//txBytes, err := hex.DecodeString(openingTxHex)
	//if err != nil {
	//	log.Printf("error finding vour")
	//	return nil, [32]byte{}, err
	//}
	//
	//openingMsgTx := wire.NewMsgTx(2)
	//openingPacket, _, _,err := createPsbtFromSignedTx(txBytes)
	//if err != nil {
	//	log.Printf("error finding vour")
	//	return nil, [32]byte{}, err
	//}
	//
	//txOut, err := wire.NewTxOut()
	//
	//
	//spendingPsbt,err := psbt.New(nil, nil,2, uint32(currentBlock),nil)
	//if err != nil {
	//	log.Printf("error finding vour")
	//	return nil, [32]byte{}, err
	//}
	//updater,err := psbt.NewUpdater(spendingPsbt)
	//if err != nil {
	//	log.Printf("error finding vour")
	//	return nil, [32]byte{}, err
	//}
	//err = updater.AddInWitnessUtxo(openingPacket.Outputs[0].,0)
	//if err != nil {
	//	log.Printf("error finding vour")
	//	return nil, [32]byte{}, err
	//}
	//spendingTx := &transaction.Transaction{
	//	Version:  2,
	//	Flag:     0,
	//	Locktime: uint32(currentBlock),
	//	Inputs:   []*transaction.TxInput{nil},
	//	Outputs:  nil,
	//}
	//
	//sigHash = spendingTx.HashForWitnessV0(
	//	0,
	//	redeemScript[:],
	//	swapInValue,
	//	txscript.SigHashAll,
	//)
	//return spendingTx, sigHash, nil
	return nil,[32]byte{},nil
}
func createPsbtFromSignedTx(serializedSignedTx []byte) (
	*psbt.Packet, [][]byte, []wire.TxWitness, error) {

	tx := wire.NewMsgTx(2)
	err := tx.Deserialize(bytes.NewReader(serializedSignedTx))
	if err != nil {
		return nil, nil, nil, err
	}
	scriptSigs := make([][]byte, 0, len(tx.TxIn))
	witnesses := make([]wire.TxWitness, 0, len(tx.TxIn))
	tx2 := tx.Copy()

	// Blank out signature info in inputs
	for i, tin := range tx2.TxIn {
		tin.SignatureScript = nil
		scriptSigs = append(scriptSigs, tx.TxIn[i].SignatureScript)
		tin.Witness = nil
		witnesses = append(witnesses, tx.TxIn[i].Witness)

	}

	// Outputs always contain: (value, scriptPubkey) so don't need
	// amending.  Now tx2 is tx with all signing data stripped out
	unsignedPsbt, err := psbt.NewFromUnsignedTx(tx2)
	if err != nil {
		return nil, nil, nil, err
	}
	return unsignedPsbt, scriptSigs, witnesses, nil
}

type TxParams struct {
	AliceKey *btcec.PrivateKey
	BobKey *btcec.PrivateKey
	Preimage lightning.Preimage
	PaymentHash []byte
	SwapAmount uint64
	Cltv uint64
}

func NewTxParams(cltv uint64) *TxParams {
	preimage, _ := lightning.GetPreimage()
	pHash := preimage.Hash()
	return &TxParams{
		AliceKey: getRandomPrivkey(),
		BobKey: getRandomPrivkey(),
		Preimage: preimage,
		PaymentHash: pHash[:],
		Cltv: cltv,
	}
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

	tx,err := btcutil.NewTxFromBytes(txBytes)
	if err != nil {
		return 0, err
	}

	outputSats := int64(0)
	for _, out := range tx.MsgTx().TxOut {
		outputSats += out.Value
	}

	 return inputSats - outputSats, nil
}

func getLightningClient() (*glightning.Lightning, error) {
	l := glightning.NewLightning()
	err := l.StartUp("lightning-rpc","/tmp/l1-regtest/regtest")

	return l, err
}

func getBitcoinClient(li *glightning.Lightning) (*gbitcoin.Bitcoin,  error) {
	configs, err := li.ListConfigs()
	if err != nil {
		return nil, err
	}
	jsonString, err := json.Marshal(configs)
	if err != nil {
		return nil, err
	}
	var listconfigRes *ListConfigRes
	err = json.Unmarshal(jsonString, &listconfigRes)
	if err != nil {
		return nil, err
	}
	var bcliConfig *ImportantPlugin
	for _,v := range listconfigRes.ImportantPlugins {
		if v.Name == "bcli" {
			bcliConfig = v
		}
	}
	if bcliConfig == nil {
		return nil, errors.New("bcli config not found")
	}

	bitcoin := gbitcoin.NewBitcoin(bcliConfig.Options["bitcoin-rpcuser"], bcliConfig.Options["bitcoin-rpcpassword"])
	bitcoin.SetTimeout(1)
	rpcPort, err := strconv.Atoi(bcliConfig.Options["bitcoin-rpcport"])
	if err != nil {
		return nil, err
	}
	bitcoin.StartUp("http://"+bcliConfig.Options["bitcoin-rpcconnect"],"",uint(rpcPort))
	return bitcoin, nil
}

type ListConfigRes struct {
	ImportantPlugins []*ImportantPlugin `json:"important-plugins"`
}

type ImportantPlugin struct {
	Path string
	Name string
	Options map[string] string
}

