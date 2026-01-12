package wallet

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"slices"

	"strings"

	"github.com/elementsproject/glightning/gelements"
	"github.com/elementsproject/glightning/jrpc2"
	"github.com/elementsproject/peerswap/log"
	"github.com/elementsproject/peerswap/swap"
	"github.com/vulpemventures/go-elements/address"
	"github.com/vulpemventures/go-elements/elementsutil"
	"github.com/vulpemventures/go-elements/transaction"
)

var (
	AlreadyExistsError = errors.New("wallet already exists")
	AlreadyLoadedError = errors.New("wallet is already loaded")
)

type RpcClient interface {
	GetNewAddress(addrType int) (string, error)
	SendToAddress(address string, amount string) (string, error)
	GetBalance() (uint64, error)
	LoadWallet(filename string, loadonstartup bool) (string, error)
	CreateWallet(walletname string) (string, error)
	SetRpcWallet(walletname string)
	ListWallets() ([]string, error)
	FundRawWithOptions(txstring string, options *gelements.FundRawOptions, iswitness *bool) (*gelements.FundRawResult, error)
	BlindRawTransaction(txHex string) (string, error)
	SignRawTransactionWithWallet(txHex string) (gelements.SignRawTransactionWithWalletRes, error)
	SendRawTx(txHex string) (string, error)
	EstimateFee(blocks uint32, mode string) (*gelements.FeeResponse, error)
	SetLabel(address, label string) error
	Ping() (bool, error)
	GetNetworkInfo() (*gelements.NetworkInfo, error)
	DecodeRawTx(txstring string) (*gelements.Tx, error)
}

// ElementsRpcWallet uses the elementsd rpc wallet
type ElementsRpcWallet struct {
	walletName string
	rpcClient  RpcClient
}

func NewRpcWallet(rpcClient *gelements.Elements, walletName string) (*ElementsRpcWallet, error) {
	if rpcClient == nil {
		return nil, errors.New("liquid rpc client is nil")
	}
	rpcWallet := &ElementsRpcWallet{
		walletName: walletName,
		rpcClient:  rpcClient,
	}
	err := rpcWallet.setupWallet()
	if err != nil {
		return nil, err
	}
	return rpcWallet, nil
}

// FinalizeTransaction takes a rawtx, blinds it and signs it
func (r *ElementsRpcWallet) FinalizeTransaction(rawTx string) (string, error) {
	unblinded, err := r.rpcClient.BlindRawTransaction(rawTx)
	if err != nil {
		return "", err
	}
	finalized, err := r.rpcClient.SignRawTransactionWithWallet(unblinded)
	if err != nil {
		return "", err
	}
	return finalized.Hex, nil
}

// CreateAndBroadcastTransaction takes a tx with outputs and adds inputs in order to spend the tx
func (r *ElementsRpcWallet) CreateAndBroadcastTransaction(swapParams *swap.OpeningParams,
	outputs []TxOutput) (txid, rawTx string, fee uint64, err error) {
	if len(outputs) == 0 {
		return "", "", 0, errors.New("missing outputs")
	}
	if swapParams.BlindingKey == nil {
		return "", "", 0, errors.New("missing blinding key")
	}
	outputscript, err := address.ToOutputScript(swapParams.OpeningAddress)
	if err != nil {
		return "", "", 0, err
	}

	tx := transaction.NewTx(2)
	for _, out := range outputs {
		assetIdBytes, err := hex.DecodeString(out.AssetID)
		if err != nil {
			return "", "", 0, err
		}
		if len(assetIdBytes) != 32 {
			return "", "", 0, fmt.Errorf("invalid asset id length: %d", len(assetIdBytes))
		}
		assetTag := append([]byte{0x01}, elementsutil.ReverseBytes(assetIdBytes)...)

		sats, err := elementsutil.ValueToBytes(out.Amount)
		if err != nil {
			return "", "", 0, err
		}
		output := transaction.NewTxOutput(assetTag, sats, outputscript)
		output.Nonce = swapParams.BlindingKey.PubKey().SerializeCompressed()
		tx.Outputs = append(tx.Outputs, output)
	}

	txHex, err := tx.ToHex()
	if err != nil {
		return "", "", 0, err
	}
	feerate, err := r.getFeeRate()
	if err != nil {
		return "", "", 0, err
	}
	fundedTx, err := r.rpcClient.FundRawWithOptions(txHex, &gelements.FundRawOptions{
		FeeRate: fmt.Sprintf("%f", feerate),
	}, nil)

	if err != nil {
		return "", "", 0, err
	}
	finalized, err := r.FinalizeTransaction(fundedTx.TxString)
	if err != nil {
		return "", "", 0, err
	}
	txid, err = r.SendRawTx(finalized)
	if err != nil {
		return "", "", 0, err
	}
	return txid, finalized, gelements.ConvertBtc(fundedTx.Fee), nil
}

// setupWallet checks if the swap wallet is already loaded in elementsd, if not it loads/creates it
func (r *ElementsRpcWallet) setupWallet() error {
	loadedWallets, err := r.rpcClient.ListWallets()
	if err != nil {
		return err
	}
	var walletLoaded bool
	if slices.Contains(loadedWallets, r.walletName) {
		walletLoaded = true
	}
	if !walletLoaded {
		_, err = r.rpcClient.LoadWallet(r.walletName, true)
		if err != nil && (strings.Contains(err.Error(), "Wallet file verification failed") || strings.Contains(err.Error(), "not found")) {
			_, err = r.rpcClient.CreateWallet(r.walletName)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

	}
	r.rpcClient.SetRpcWallet(r.walletName)
	acceptdiscountctIsEnabled, err := r.acceptdiscountctIsEnabled()
	if err != nil {
		return err
	}
	if !acceptdiscountctIsEnabled {
		return errors.New("accept-discount-ct is not enabled")
	}
	return nil
}

// GetBalance returns the balance in sats
func (r *ElementsRpcWallet) GetBalance() (uint64, error) {
	balance, err := r.rpcClient.GetBalance()
	if err != nil {
		return 0, err
	}
	return balance, nil
}

// GetAddress returns a new blech32 address
func (r *ElementsRpcWallet) GetAddress() (string, error) {
	address, err := r.rpcClient.GetNewAddress(3)
	if err != nil {
		return "", err
	}
	return address, nil
}

// SendToAddress sends an amount to an address
func (r *ElementsRpcWallet) SendToAddress(address string, amount uint64) (string, error) {
	txId, err := r.rpcClient.SendToAddress(address, satsToAmountString(amount))
	if err != nil {
		return "", err
	}
	return txId, nil
}

func (r *ElementsRpcWallet) SendRawTx(txHex string) (string, error) {
	raw, err := r.rpcClient.SendRawTx(txHex)
	if err != nil {
		errWithCode, ok := err.(*jrpc2.RpcError)
		if ok && errWithCode.Code == -26 {
			return "", MinRelayFeeNotMetError
		}
	}
	return raw, err
}

const (
	// minFeeRateBTCPerKb defines the minimum fee rate in BTC/kB.
	// This value is equivalent to 0.1 sat/byte.
	minFeeRateBTCPerKb = 0.000001
)

// getFeeRate retrieves the optimal fee rate based on the current Liquid network conditions.
// Returns the recommended fee rate in BTC/kB
func (r *ElementsRpcWallet) getFeeRate() (float64, error) {
	feeRes, err := r.rpcClient.EstimateFee(LiquidTargetBlocks, "ECONOMICAL")
	if err != nil {
		return 0, err
	}
	if len(feeRes.Errors) > 0 {
		log.Debugf(" Errors encountered during fee estimation process: %v", feeRes.Errors)
		return minFeeRateBTCPerKb, nil
	}
	return math.Max(minFeeRateBTCPerKb, feeRes.FeeRate), nil
}

const (
	// 1 kb = 1000 bytes
	kb              = 1000
	btcToSatoshiExp = 8
)

func (r *ElementsRpcWallet) GetFee(txSize int64) (uint64, error) {
	feeRate, err := r.getFeeRate()
	if err != nil {
		return 0, fmt.Errorf("error getting fee rate: %v", err)
	}
	satPerByte := feeRate * math.Pow10(btcToSatoshiExp) / kb
	fee := satPerByte * float64(txSize)
	return uint64(fee), nil
}

func (r *ElementsRpcWallet) SetLabel(txID, address, label string) error {
	return r.rpcClient.SetLabel(address, label)
}

// satsToAmountString returns the amount in btc from sats
func satsToAmountString(sats uint64) string {
	bitcoinAmt := float64(sats) / 100000000
	return fmt.Sprintf("%f", bitcoinAmt)
}

func (r *ElementsRpcWallet) Ping() (bool, error) {
	return r.rpcClient.Ping()
}

// acceptdiscountctIsEnabled checks if the acceptdiscountct feature is enabled
// by decoding a hardcoded example transaction and checking for the presence
// of the DiscountVirtualSize field in the decoded transaction.
// Note: if `creatediscountct` is enabled, `acceptdiscountct` is also enabled
// https://github.com/psgreco/elements/blob/5c04f7d9a06d5650c27e0b4476da3cb8c4fc9065/src/chainparams.cpp#L1486
func (r *ElementsRpcWallet) acceptdiscountctIsEnabled() (bool, error) {
	const exampleTransaction = "0200000001016c23108c06f707d1d2e4efaa01ec22befe2a205db96f4bcaed64f9291832fe78010000000000000000020adb2a388a990a039381b9371126708275dd108dc9f94dbb1530fa3e366f1796a209bdc1065b4af21c334c5a376892418fbec225c022a6e481836beaebc043730e5c03d2c5866594d34a4aa5bd5054dbbe55c945bc508a47fa379025db5430236c6c851600146a3d70f1fef408f2402575551a924087b146c7870125b251070e29ca19043cf33ccd7324e2ddab03ecc4ae0b5e77c4fc0e5cf6c95a010000000000000021000000000000000005473044022077ebdd3eba62437e243f6e709bd8631d1966d85271daaf5572a2bd2e64f6315f022019ababfcb1251e0e413efbb55ac3fac62db45922bc6ec49287271699778170390120ad242337ddfa53bf5f7a45e57081df397448a6f264974d9ef7a244c822f176e20000982103a8a6534ae8f64380045b9cedb905092c3dd8ca9cdc688d3d0fa10abdd33cc89bac642103a8a6534ae8f64380045b9cedb905092c3dd8ca9cdc688d3d0fa10abdd33cc89bac6482012088a820a9605068a7f8deb691a1c23beb072abbd396e615ad50864ea8b5653e1a82b5ac886821032c55b7188b53495231b14410dff0bf273dd76715957e67b26224737c85a0dddcac67013cb2680043010001973e88d249ba494cd425c90a46ab71aed4d3cbf1fc670c9cf972f13c49dafbd43d17fa121f48a41a70afc740f39fb301cf363cabea19004fef6db7956ee9382efd4e106033000000000000000163e1f6012c6c59fe1b9ae60f39d919a19b8a54dfee223daa9ec3691171d3dd30c4949c9da2ab37be1ba7d80f3ceebf19a62409b37aeb40c122c29b72870e9fbdae15306be0dd58742cbba08f7b55fbbc2943c9423cd465a868c19609113e6bb85364fa17fecade3faff864fb14b59cd49d3c2e5d96a926763b05365327273565eceab719829d43a2ce670eee1cb8a8da42c2e6621396b151e3379717326f743ce99ca2fdd024cc25fa3f8de8b11017e609d140f75f7f9ff04c78d30f98fb5580b32ee8d21b8007aa7c346bc9bc21005558c88bd0f550d6539d6e25a2f47f7221dec995b85278202cfd73471be42c35a351b2e58d2904cca264ec3685c693ac2a9d1ffc694c390ac0dc5f29869e3fe75b15b088b80865de47bcc8b081021890be1668e272304818d8f4bd32e15b5f9ec05dc266531900ea8a7788451cb3f8933d5e19cda1ef3323e2046cb55f42b1821ae3cc712ee6e94c46d35269d2c8f226db9403de29da6b300161f391c15c00355784b49c3ef221ec164efb10a8d0c1632bd3df89f9ed32ce7022e84250e4c144667135be73f64f832580cb736a63557b75a192f3d7fb253112a9aad6a1213d9a612d6535beda6b4a26dbae5cf9004c0290873cce30fc3d54c5f8c0bed12dea7b615f06ca1c338ef3f8db4326019e4082d6c238c66ebb3cb16d75169f4e158cbe7a0c5106d26aebb2b6607e8fb590d8ed02b3c01fd59fe270a4137e9acad762f47647d07f5480265eafbf9a4ca09646ec4289f1ed67d68842928ecacd460e7014455435c8c38da4c5c22355dffc0e1bf507a9aa1c63405871deafdf4ee128948c8fb3705c95460c25a74b2fa36f8c64f8f77b43c9179bf1d3afab76260d5f44a9e13f30b6adcabd5c8fc631ace279786e58e680cc3eb568e974fb0f8a427b6c1f122f92d8a49ef3f31bd7dea715596258dd652e16626bd28f07f5970a53d74f8e041f80cc4d230f1d23b5cb706fd7f6449a1bf86e91cce9d8f45e0cd6a0a270cf462f6aac00c320c50e6dd077940c689bb77dfe8c55bbd39328920d9dcafd9e670643c6ebd2966c6a5dac9f583ee82b032db0d76a02447efa5a301c397136ae96f5af71e0fd8dfff9412bda022d141c83fcfc545dc58dcf4c0b73af14f75fefa53fef9c8119b2cf80eb3231b4f1232e1f7575be0832a230f47ef3e9826b0f2d10d1a8bd124db5158f467878dc728649579952827d348b47ac635e7e3d0be21061053c62a570cd24d5e46ddfc5b2acccabcc1355af0763bee33dbd600bf918e64f372e081430708c003ba82e31733c8a761791b7756186a7c9c9218ec134a7e456560a04edb92217a029093e706dcd6dc1a058b97e674a8a19e643ea182fbb5afd8f8b49a6e83db50f269f6873ebffc5f127fc9f22089867a3cf7a8aa26c63b0fddb64c883b09a3cd1ea372e5fdc1bbc6f7d2d1c517ca2925f89e6053e7e218f77073170d8b2196ac2d8e11bff0a5d85764d077df7cc628e3d33c7a286af1df3cb1d185aadeb7f4af4d2315e58ae5ba9a3fa765045702a2aac6f1d4e712be49dd6edf3224e23af83454c160e695af395bcc0fb0bc8f8fe72ec074ad5a2e8945374f5f673c74151d666054bfe3f59c8727bafc9f65c1f5c5d93146d00f5d7eab563b77690335b9cdf95016a3f679b4d10a8fff137c003cc4a7fb0e5fef904588b66b5b5187261ae4802ead0e04fb802cef963b892969db9c88d823fbab73809f5d2015f9aba3a466b871973d9209623790fbc46cd74af026b4e053b0cfaa0d42880ce7c615583d65378cc7e99a62c0bf0124eb34518cfd2734e5d572b7a159aab63cc58edeb58f8c75af0656e2b5ceed9ce3ec7ddc14d21b501f057e402e67826a962bc346143d6c4b6628fdf983d5dccf9088f78fc3588ebdf92befdf57dcbef45f9d2f50f1f583f9702190f53d78638012a7948a235f408d7836812674471f211c69c79a9b6415c2279d6a4345ba4e820041ebc1ee6c37f7301853637a2c264c9447da1ff15a824ae361de0f50a5967248fcb7defac8b917b912aed43b5e266844104f7540dadeb67a925b563f1d29ba73d2c7f2868fca207b968bf1e7b91b35d4058f81a2507a2c3dc1e26a36192ac6d111569ce8cfc6460939dd7d1fffb6e3dba60b1c170a9edfee51996974e63f4697eb6599553ac5c0d8e73ce6afb3523aea889470e881e03bbb0e9a8f50decf8ddce9157e2da486b8e3b0dbaedd94669c17429abc11dd5c792d1116f1a48a7f4701623eac2edca49f244820af38fa3967a836aeb433d4c9437e76a6a6ab37e08498a359afdd6d8023cb997e91d43dee7eb6923c89070ecb51e0c0ccde1740a0d486e2bb681b431e6ab859779777b0ad74fa3ccd1b3713a553c9ce1436fafb2af1c3875228181833ff5ade2cf779b1fd16dd2897c505ffb9b9fe6ab0efaaaf93bb946e88bf4472e932d50125ecc4ed5a8063e7b90c03dc9571e093314e35a6495b54ac058ed651650952708c07da3699e77e0caace2de3866b5c1a5129cc5d2ca9308471f7a6248d5a30be54451163c8aa7ec2bad41797ddc8c142b35cd40c713056a71b0ddc5df84246145b77d45bf7f1061e9730c03f49ea7d09a740f7ef75b361ddbe657ab5dc6f23712330166ff23f2affa883c4d6f2e9d900ae34005c465e0eab47d4f37d23ddcf049ea32a90436c19cfdafd2cb1e5fc82e80d96af786422168a236587b75cd1bfa50b402d669e29128bf80d7d85d862de0342597cf7e9017c9535921ee50a12ef3f17efda569810937391e04f99d78961d6c1fa622a43b4776b9383b13f1d8ebf825d8d2db543f727b09b2b66d465a2e3dcf75c9cbe31040148dfdedb047a098e54e806ee5458579b93d15f4e7e788a7b9533fd9a28a98fed161c1646bb619a09d067fc005d3a5ef96d23b88d16de53cc015f61a0a1e13244a5d0d023129902a37cf2bda8d5f15b51a7108aa8d756f4db9d916081c9306aa200819bf18f6dd9c18d1ed1a63280ec2b5bcd0df5826cb053e53ac06f7de89efb8a665066bc0bc0d9555989a3139396cb72d968140dfe9d6c5d2d1ebb715658e09dd6c5c7630d3c1477e686630335ab0ac854ce8ceb29c0b05c83150dc587bd4711fce3a4ac087a2ceb818457850da13788a8416ca874fb130b2ca76bcd8c9572b897db63d950a5166c788b3d773409e067148a0de817d7e3d0fe23f17fd585b58ab631c0e3ab8d7b474be8e8a804c2ff9f6b6c5f1b27b6621be8f7222a623395711f35bf04ab939f7d03bb1363f7eaebef9f5ac2d924ef1bd1c30b3a60a26c59f490c5743917967b4fdb9248fc17b2fb054ac700c5ad56fd4b367bddba1e170add45d075d615b042641ada1a7db52d423431ed7f439a7e63bf275e64ba841e238ad4ea0449030ba6704d74b60d42161ed6e9aa4ebb117ed0f4acf401f8547975f6a801c16e34e14068a302686d2309200e18397500ec8425d0e27e7b229917e16076f0d1ce01aff45bf9016c32c30cd6bd348d517a7b7521cd1d69ad962fc9d295036588ba98f174e70e2cac61e7f50a53b4e86fc5112b90c735019d965f1c5ba9de0e9f5448ccb070a94dd82aba1c326e870e35815991122e721d08734f964712c9307904a4d52241f3ba1239fd1fcb2181440d3b00a9097a9285350cd57391db187dd9131a7e96f2ccaea0ea7997a35da4b817da71b911b11d48e78e4daaa83d4a69b18458d94e07122df9e79466acb7659a975921c5b39d7d1532192c233804a12ea5f05aa7632f0f4638a5e2c26b9659a38a906303ad50e1f714c26f329eb694866926fa8d7affaf8f562fbe8bbe1eed61014f3737cdee1ec29625004c9f4086baca20d16b4ee57368ceba23b51b19369df071dee8553f1d70e4c2eaa156dff49c69dcf5c64c29957fbadda8ebd606ecf059a14423cdaa13c86becccec39c4cd4fde4d37e0badd7299b4d607f93a064a2787d5bac289950a4d4d1f20b2b714cfdb427accb64bb87f8efe4c33f028fccfd10e88126686b0477207bf2d2f8a81f59e4d011ad6a5296ccfcaa8517969f5c03d5337723858bf22f8607e37361b1fcc877a3578b52c5ea2f9d1e08397fd7b58176ccaccdcc3713c7c9dc8a937aaa426b260d6a870424165d4a5d16beb440b272cc5f9a2602956941cf6f42e41189fa255699a907c7826339749aa73bc408fde114012ca9f60a1a1cca7e22023ca0b7c04dd152bdd5fcb32193f31007cc0d90134180e9c6666a645925d9c24e750fd77ab0fef0eb2f500f1d66f46420635704a51c3a872732813b50b38f6cb71a6d49b7224b01297e4843ea3ed21602d795e49e01223dc22e1bf3898a1f54d52251fa5145da5e92bec0b70198bc0ef29d8578ab6876957c3bfcf430313886e26455105ad44140b1b6d61036cc253ee80907deeaedf01132b2304cfe491b2a59c3d9482d8b51bc8104da055dd0c4c1d8326438e9181cb2aa876a59cb5701aed1b11017c1235dc597943ed5f3f7f24954340fea2a1ce592bd0b8adb5070b6965ccc55f55d9978224e74232999840454fb3bbf736a3a6c7194be6c82bb961f4f0fb1fa43980d16c7553cc82f2ec2b32178d82ee7ec4d15c119740b19f5f87e46c3c7a63c0508f638ed9ffff4b5907c709bfea1d682c5ffe020723d37016b2012fc1a4f45376fc2435326fbd991b3f9a0025b0dc7a6836159250a49345ab96005dfac524ae763a6c58ebf79c06aa06fc4b8cc4777db3299ccc9b7d51f41b2cecf88c5e248a745611b7b973f2a2416796c41de502ac27f886b97ef4ce53b83658d16042e69a2bcb2511e2cec4b44ee2ee5302b43deebdcd056971fedacc5f556c0b968ff04322795b75fb3918410509247957b1750490d6bffa0c2a9983245906e5c5ebce95545493cf9449841c2717f1aece5bd40af74cabfdf91319d6790d6d72aa927b16924c889d9f02e906b6cd7774d8f1103046ec3b1490349cb3322f64d9cfb887d4638c9f8349c50fbe5f8c2c3a66e795262d585081b218f13408ea10213653936b319b9af0e0a6c941f957310d270a5c4c89b443e1a505adc0e30a3d1c8b1268912d8e19cef488c37361ae0dc9ae62e86e26a7125ade70046185fa69df8915ad4c90574721321e38635db23e2b18065466ea090d6381fae4af0233b2cad779625a34656ac55d89a63f088dedc400486436c16b6533912bda8a4069f7bc2f0c32bf9bac31f6a9f0a2bc0d0107ec7f4b5bebcc6e8f421b5d37832e31cb716960656128a49f2354e95c82b8d8928356b81d960d32b14bde5cdec79bce343c08ad363dd80be49e05274a8d851c7c2f0381b724bd8a84d93fb34111e1ef1378785558b84de879a830724a3555fc4ada415e7b0feb6be2b4969f6fc7052d8f47d1c69c280260268cc3b7375ed310b3284eea4e342fdea39ad75866b9d3aee71c01540747e04e3ca0e0bee8fa9815acfa730e8fd4c010f642b07e589d068db15c2e4a4ca57c6343527b3647f70f8605e1289369b7b81cc9cc9235f07141424f1a63b35c07023fdb7792e84e6239d14ce935f3553a87a3729503f53e5eba98fabcd44f8d589c80941763519433737932b576dd9d6c66d42f4828ebbbddf907f29bb0e5ee544965872dbe603b1fda70894e9c2c1c662196da13a617bc23b9bba6936d44e615c1e084595de2a3b5e32952001b30be81d8264163440cea47006cc4563fb0abe0c78a812b0164b9093651051fc8b51700fb98567360a86e1da49efeff3a4d4dde9e23ae2603d14ba78311d168c1d7c90a06c77b26f99daa30402d8202a67d6bfc27b8a2e2bb83b488b7da470aaf1e38d0e01e98482ec1ba719fdfea94b9697899b8f24b53a60e2b4e6d5bfe3cc5623abb75eee95bab4159164d9c6861c4f81de2abce70000"
	tx, err := r.rpcClient.DecodeRawTx(exampleTransaction)
	if err != nil {
		return false, fmt.Errorf("failed to check acceptdiscountctIsEnabled: %v", err)
	}
	// check if the transaction has a DiscountVirtualSize field
	return tx.DiscountVirtualSize != 0, nil
}
