package onchain

import (
	"encoding/hex"
	"github.com/btcsuite/btcd/txscript"
	"github.com/sputn1ck/peerswap/swap"
)

func ParamsToTxScript(p *swap.OpeningParams, locktimeHeight uint32) ([]byte, error) {
	takerBytes, err := hex.DecodeString(p.TakerPubkeyHash)
	if err != nil {
		return nil, err
	}
	makerBytes, err := hex.DecodeString(p.MakerPubkeyHash)
	if err != nil {
		return nil, err
	}
	phashBytes, err := hex.DecodeString(p.ClaimPaymentHash)
	if err != nil {
		return nil, err
	}
	return GetOpeningTxScript(takerBytes, makerBytes, phashBytes, locktimeHeight)
}

// GetOpeningTxScript returns the script for the opening transaction of a swap,
// where the taker is the peer paying the invoice and the maker the peer providing the lbtc
func GetOpeningTxScript(takerPubkeyHash []byte, makerPubkeyHash []byte, pHash []byte, csv uint32) ([]byte, error) {
	script := txscript.NewScriptBuilder().
		AddData(makerPubkeyHash).
		AddOp(txscript.OP_CHECKSIG).
		AddOp(txscript.OP_NOTIF).
		AddData(makerPubkeyHash).
		AddOp(txscript.OP_CHECKSIG).
		AddOp(txscript.OP_NOTIF).
		AddOp(txscript.OP_SIZE).
		AddData(h2b("20")).
		AddOp(txscript.OP_EQUALVERIFY).
		AddOp(txscript.OP_SHA256).
		AddData(pHash[:]).
		AddOp(txscript.OP_EQUALVERIFY).
		AddOp(txscript.OP_ENDIF).
		AddData(takerPubkeyHash).
		AddOp(txscript.OP_CHECKSIG).
		AddOp(txscript.OP_ELSE).
		AddInt64(int64(csv)).
		AddOp(txscript.OP_CHECKSEQUENCEVERIFY).
		AddOp(txscript.OP_ENDIF)
	return script.Script()
}

// GetPreimageWitness returns the witness for spending the transaction with the preimage
func GetPreimageWitness(signature, preimage, redeemScript []byte) [][]byte {
	sigWithHashType := append(signature, byte(txscript.SigHashAll))
	witness := make([][]byte, 0)
	//log.Printf("%s, \n %s,\n %s", hex.EncodeToString(sigWithHashType), hex.EncodeToString(preimage), hex.EncodeToString(redeemScript))
	witness = append(witness, sigWithHashType)
	witness = append(witness, preimage[:])
	witness = append(witness, []byte{})
	witness = append(witness, []byte{})
	witness = append(witness, redeemScript)
	return witness
}

// GetCsvWitness returns the witness for spending the transaction with a passed csv
func GetCsvWitness(signature, redeemScript []byte) [][]byte {
	sigWithHashType := append(signature, byte(txscript.SigHashAll))
	witness := make([][]byte, 0)
	witness = append(witness, sigWithHashType)
	witness = append(witness, redeemScript)
	return witness
}

func GetCooperativeWitness(takerSig, makerSig, redeemScript []byte) [][]byte {
	witness := make([][]byte, 0)
	witness = append(witness, append(takerSig, byte(txscript.SigHashAll)))
	witness = append(witness, append(makerSig, byte(txscript.SigHashAll)))
	witness = append(witness, []byte{})
	witness = append(witness, redeemScript)
	return witness
}
func h2b(str string) []byte {
	buf, _ := hex.DecodeString(str)
	return buf
}
