package wallet

import (
	"encoding/hex"
	"github.com/btcsuite/btcd/btcec"
	"reflect"
	"testing"
)

var (
	alicePrivkey = "b5ca71cc0ea0587fc40b3650dfb12c1e50fece3b88593b223679aea733c55605"
)

func Test_Privkeys(t *testing.T) {
	privkey, err := btcec.NewPrivateKey(btcec.S256())
	if err != nil {
		t.Fatal(err)
	}

	privkeyHex := hex.EncodeToString(privkey.Serialize())

	newPriv, _ := btcec.PrivKeyFromBytes(btcec.S256(), privkey.Serialize())

	t.Log(privkeyHex)

	if !reflect.DeepEqual(privkey, newPriv) {
		t.Fatal("priv keys not equal")
	}

}

func Test_Address(t *testing.T) {
	privkeyBytes, err := hex.DecodeString(alicePrivkey)
	if err != nil {
		t.Fatal(err)
	}
	privkey,_ := btcec.PrivKeyFromBytes(btcec.S256(), privkeyBytes)

	walletStore := DummyWalletStore{PrivKey: privkey}

	_, err = walletStore.ListAddresses()
	if err != nil {
		t.Fatal(err)
	}

}


