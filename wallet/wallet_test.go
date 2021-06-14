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
	walletStore := DummyWalletStore{}
	err := walletStore.Initialize()
	if err != nil {
		t.Fatal(err)
	}
	_, err = walletStore.ListAddresses()
	if err != nil {
		t.Fatal(err)
	}

}

func Test_getUtxos(t *testing.T) {
	fooUtxo := getutxo("foo", 1000)
	barUtxo := getutxo("bar", 2000)
	type args struct {
		amount    uint64
		haveUtxos []*Utxo
	}
	tests := []struct {
		name          string
		args          args
		wantUtxos     []*Utxo
		wantChange    uint64
		wantErr       bool
		specificError error
	}{
		{
			name: "ez",
			args: args{
				amount:    1000,
				haveUtxos: []*Utxo{fooUtxo},
			},
			wantUtxos:  []*Utxo{fooUtxo},
			wantChange: 0,
			wantErr:    false,
		},
		{
			name: "ez2",
			args: args{
				amount:    1500,
				haveUtxos: []*Utxo{fooUtxo, barUtxo},
			},
			wantUtxos:  []*Utxo{fooUtxo, barUtxo},
			wantChange: 1500,
			wantErr:    false,
		},
		{
			name: "ez2",
			args: args{
				amount:    3500,
				haveUtxos: []*Utxo{fooUtxo, barUtxo},
			},
			wantUtxos:     nil,
			wantChange:    0,
			wantErr:       true,
			specificError: NotEnoughBalanceError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUtxos, gotChange, err := getUtxos(tt.args.amount, tt.args.haveUtxos)
			if (err != nil) != tt.wantErr {
				t.Errorf("getUtxos() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (err != nil && tt.wantErr) && (err.Error() != tt.specificError.Error()) {
				t.Errorf("getUtxos() error = %s, want specificErr %s", err.Error(), tt.specificError.Error())
			}
			if !reflect.DeepEqual(gotUtxos, tt.wantUtxos) {
				t.Errorf("getUtxos() gotUtxos = %v, want %v", gotUtxos, tt.wantUtxos)
			}
			if gotChange != tt.wantChange {
				t.Errorf("getUtxos() gotChange = %v, want %v", gotChange, tt.wantChange)
			}
		})
	}
}

func getutxo(id string, amount uint64) *Utxo {
	return &Utxo{
		TxId:  id,
		Value: amount,
	}
}
