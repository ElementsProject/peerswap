package wallet

import (
	"errors"
	"github.com/btcsuite/btcd/btcec"
)

var (
	NotEnoughBalanceError = errors.New("Not enough balance on utxos")
)

type WalletStore interface {
	LoadPrivKey() (*btcec.PrivateKey, error)
	ListAddresses() ([]string, error)
}
type BlockchainService interface {
	GetBlockHeight() (int, error)
	BroadcastTransaction(string) (string, error)
	FetchTxHex(txId string) (string, error)
	FetchUtxos(address string) ([]*Utxo, error)
}
type Utxo struct {
	TxId   string      `json:"txid"`
	VOut   uint32      `json:"vout"`
	Status interface{} `json:"status"`
	Value  uint64      `json:"value"`
	Asset  string      `json:"asset"`
}

type LiquiddWallet struct {
	Store      WalletStore
	Blockchain BlockchainService
}

func (d *LiquiddWallet) ListAddresses() ([]string, error) {
	return d.Store.ListAddresses()
}
func (s *LiquiddWallet) GetBalance() (uint64, error) {
	addresses, err := s.Store.ListAddresses()
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
	addresses, err := s.Store.ListAddresses()
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
	addresses, err := s.Store.ListAddresses()
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
