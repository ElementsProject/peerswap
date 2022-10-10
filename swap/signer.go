package swap

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
)

type Secp256k1Signer struct {
	key *btcec.PrivateKey
}

func (s *Secp256k1Signer) Sign(hash []byte) (*ecdsa.Signature, error) {
	return ecdsa.Sign(s.key, hash), nil
}
