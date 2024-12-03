package wallet

import (
	"errors"
	"fmt"
	"math"

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

const (
	// https://github.com/ElementsProject/elements/releases/tag/elements-23.2.2
	elementsdFeeDiscountedVersion = 230202
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
	asset []byte) (txid, rawTx string, fee uint64, err error) {
	outputscript, err := address.ToOutputScript(swapParams.OpeningAddress)
	if err != nil {
		return "", "", 0, err
	}
	sats, err := elementsutil.ValueToBytes(swapParams.Amount)
	if err != nil {
		return "", "", 0, err
	}
	output := transaction.NewTxOutput(asset, sats, outputscript)
	output.Nonce = swapParams.BlindingKey.PubKey().SerializeCompressed()

	tx := transaction.NewTx(2)
	tx.Outputs = append(tx.Outputs, output)

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
	for _, v := range loadedWallets {
		if v == r.walletName {
			walletLoaded = true
			break
		}
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
