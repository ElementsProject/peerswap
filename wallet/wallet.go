package wallet

import (
	"errors"
	"github.com/btcsuite/btcd/btcec"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/payment"
	"github.com/vulpemventures/go-elements/transaction"
)

var (
	NotEnoughBalanceError = errors.New("Not enough balance on utxos")
)

type WalletStore interface {
	LoadPrivKey() (*btcec.PrivateKey, error)
	Initialize() error
}
type BlockchainService interface {
	GetBlockHeight() (int, error)
	BroadcastTransaction(string) (string, error)
	FetchTxHex(txId string) (string, error)
	FetchTx(txId string) (*Transaction, error)
	FetchUtxos(address string) ([]*Utxo, error)
	WalletUtxosToTxInputs(utxos []*Utxo) ([]*transaction.TxInput, error)
}
type Utxo struct {
	TxId   string      `json:"txid"`
	VOut   uint32      `json:"vout"`
	Status interface{} `json:"status"`
	Value  uint64      `json:"value"`
	Asset  string      `json:"asset"`
}
type Transaction struct {
	Txid     string `json:"txid"`
	Version  int    `json:"version"`
	Locktime int    `json:"locktime"`
	Vin      []struct {
		Txid    string `json:"txid"`
		Vout    int    `json:"vout"`
		Prevout struct {
			Scriptpubkey        string `json:"scriptpubkey"`
			ScriptpubkeyAsm     string `json:"scriptpubkey_asm"`
			ScriptpubkeyType    string `json:"scriptpubkey_type"`
			ScriptpubkeyAddress string `json:"scriptpubkey_address"`
			Value               int    `json:"value"`
			Asset               string `json:"asset"`
		} `json:"prevout"`
		Scriptsig             string   `json:"scriptsig"`
		ScriptsigAsm          string   `json:"scriptsig_asm"`
		Witness               []string `json:"witness"`
		IsCoinbase            bool     `json:"is_coinbase"`
		Sequence              int      `json:"sequence"`
		InnerWitnessscriptAsm string   `json:"inner_witnessscript_asm"`
		IsPegin               bool     `json:"is_pegin"`
	} `json:"vin"`
	Vout []struct {
		Scriptpubkey        string `json:"scriptpubkey"`
		ScriptpubkeyAsm     string `json:"scriptpubkey_asm"`
		ScriptpubkeyType    string `json:"scriptpubkey_type"`
		ScriptpubkeyAddress string `json:"scriptpubkey_address,omitempty"`
		Value               int    `json:"value"`
		Asset               string `json:"asset"`
	} `json:"vout"`
	Size   int `json:"size"`
	Weight int `json:"weight"`
	Fee    int `json:"fee"`
	Status struct {
		Confirmed   bool   `json:"confirmed"`
		BlockHeight int    `json:"block_height"`
		BlockHash   string `json:"block_hash"`
		BlockTime   int    `json:"block_time"`
	} `json:"status"`
}

type LiquiddWallet struct {
	Store      WalletStore
	Blockchain BlockchainService

	network *network.Network
}

func NewLiquiddWallet(store WalletStore, blockchain BlockchainService, network *network.Network) *LiquiddWallet {
	return &LiquiddWallet{Store: store, Blockchain: blockchain, network: network}
}

func (d *LiquiddWallet) ListAddresses() ([]string, error) {
	pubkey, err := d.GetPubkey()
	if err != nil {
		return nil, err
	}
	p2pkhBob := payment.FromPublicKey(pubkey, d.network, nil)
	address, err := p2pkhBob.PubKeyHash()
	if err != nil {
		return nil, err
	}
	return []string{address}, nil
}
func (s *LiquiddWallet) GetBalance() (uint64, error) {
	addresses, err := s.ListAddresses()
	if err != nil {
		return 0, err
	}
	var balance uint64
	var addressUnspents []*Utxo
	for _, address := range addresses {
		addressUnspents, err = s.Blockchain.FetchUtxos(address)
		if err != nil {
			return 0, err
		}
		for _, tx := range addressUnspents {
			balance += tx.Value
		}
	}
	return balance, nil
}

func (s *LiquiddWallet) GetPubkey() (*btcec.PublicKey, error) {
	privkey, err := s.Store.LoadPrivKey()
	if err != nil {
		return nil, err
	}
	return privkey.PubKey(), nil
}

func (s *LiquiddWallet) GetPrivKey() (*btcec.PrivateKey, error) {
	return s.Store.LoadPrivKey()
}
func (s *LiquiddWallet) ListUtxos() ([]*Utxo, error) {
	var utxos []*Utxo
	addresses, err := s.ListAddresses()
	if err != nil {
		return nil, err
	}

	var addressUnspents []*Utxo
	for _, address := range addresses {
		addressUnspents, err = s.Blockchain.FetchUtxos(address)
		if err != nil {
			return nil, err
		}
		utxos = append(utxos, addressUnspents...)
	}
	return utxos, nil
}

// GetUtxos returns a slice of uxtos that match the given amount, as well as the change for the
func (s *LiquiddWallet) GetUtxos(amount uint64) ([]*Utxo, uint64, error) {
	addresses, err := s.ListAddresses()
	if err != nil {
		return nil, 0, err
	}
	haveBalance := uint64(0)
	var utxos []*Utxo

	var addressUnspents []*Utxo
	for _, address := range addresses {
		addressUnspents, err = s.Blockchain.FetchUtxos(address)
		if err != nil {
			return nil, 0, err
		}
		for _, utxo := range addressUnspents {
			haveBalance += utxo.Value
			utxos = append(utxos, utxo)
			if haveBalance >= amount {
				return utxos, haveBalance - amount, nil
			}
		}
	}
	return nil, 0, NotEnoughBalanceError
}

func getUtxos(amount uint64, haveUtxos []*Utxo) (utxos []*Utxo, change uint64, err error) {
	haveBalance := uint64(0)
	for _, utxo := range haveUtxos {
		haveBalance += utxo.Value
		utxos = append(utxos, utxo)
		if haveBalance >= amount {
			return utxos, haveBalance - amount, nil
		}
	}
	return nil, 0, NotEnoughBalanceError
}
