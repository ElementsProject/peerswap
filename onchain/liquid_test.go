package onchain

import (
	"bytes"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/elementsproject/peerswap/swap"
	"github.com/vulpemventures/go-elements/confidential"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/transaction"
	secp256k1 "github.com/vulpemventures/go-secp256k1-zkp"
)

func Test_ScriptAddress(t *testing.T) {
	liquidOnCain := NewLiquidOnChain(nil, &network.Testnet)
	swapParams := &swap.OpeningParams{
		TakerPubkey:      "02752e1beeeeb6472959117a0aa5d172900680c033ddf86b1a8318311e2b10223f",
		MakerPubkey:      "02c30ff537639962f493d326a77f1c6cb591ee3d21ca8d89194bb69cb288f497e8",
		ClaimPaymentHash: "b94f26d422d5ce3a1e65dd4abb398d0d369aefe8f71d112c5591aa45eea1e75c",
		Amount:           5000,
	}
	redeemScript, err := ParamsToTxScript(swapParams, 0)
	if err != nil {
		t.Fatal(err)
	}
	addr, err := liquidOnCain.CreateOpeningAddress(redeemScript)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("addr %s", addr)
}

func TestLiquidOpeningOutputValidation(t *testing.T) {
	liquidOnChain := NewLiquidOnChain(nil, &network.Regtest)
	openingParams := newLiquidOpeningParams(t)
	policyAsset := append([]byte(nil), liquidOnChain.asset[1:]...)
	attackerAsset := bytes.Repeat([]byte{0x42}, 32)

	validAbf := testScalar(1)
	attackerAbf := testScalar(2)
	forgedAbf := testScalar(3)

	tests := []struct {
		name             string
		committedAsset   []byte
		disclosedAsset   []byte
		committedAbf     []byte
		disclosedAbf     []byte
		amount           uint64
		wantError        string
		testSpendingPath bool
	}{
		{
			name:           "blinded policy asset",
			committedAsset: policyAsset,
			disclosedAsset: policyAsset,
			committedAbf:   validAbf,
			disclosedAbf:   validAbf,
			amount:         openingParams.Amount,
		},
		{
			name:             "consistent attacker asset",
			committedAsset:   attackerAsset,
			disclosedAsset:   attackerAsset,
			committedAbf:     attackerAbf,
			disclosedAbf:     attackerAbf,
			amount:           openingParams.Amount,
			wantError:        "invalid asset id",
			testSpendingPath: true,
		},
		{
			name:             "forged policy asset disclosure",
			committedAsset:   attackerAsset,
			disclosedAsset:   policyAsset,
			committedAbf:     attackerAbf,
			disclosedAbf:     forgedAbf,
			amount:           openingParams.Amount,
			wantError:        "invalid asset commitment",
			testSpendingPath: true,
		},
		{
			name:             "incorrect amount",
			committedAsset:   policyAsset,
			disclosedAsset:   policyAsset,
			committedAbf:     validAbf,
			disclosedAbf:     validAbf,
			amount:           openingParams.Amount + 1,
			wantError:        "tx value is not equal to the swap contract",
			testSpendingPath: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			txHex := newBlindedOpeningTx(
				t,
				liquidOnChain,
				openingParams,
				test.committedAsset,
				test.disclosedAsset,
				test.committedAbf,
				test.disclosedAbf,
				test.amount,
			)

			valid, err := liquidOnChain.ValidateTx(openingParams, txHex)
			if test.wantError == "" {
				checkNoError(t, err)
				if !valid {
					t.Fatal("ValidateTx() = false, want true")
				}
				return
			}

			if valid {
				t.Fatal("ValidateTx() = true, want false")
			}
			checkErrorContains(t, err, test.wantError)

			if test.testSpendingPath {
				assertSpendingTransactionRejected(
					t,
					liquidOnChain,
					openingParams,
					txHex,
					test.wantError,
				)
			}
		})
	}
}

func newLiquidOpeningParams(t *testing.T) *swap.OpeningParams {
	t.Helper()

	blindingKey, err := btcec.NewPrivateKey()
	checkNoError(t, err)

	return &swap.OpeningParams{
		TakerPubkey:      "02752e1beeeeb6472959117a0aa5d172900680c033ddf86b1a8318311e2b10223f",
		MakerPubkey:      "02c30ff537639962f493d326a77f1c6cb591ee3d21ca8d89194bb69cb288f497e8",
		ClaimPaymentHash: "b94f26d422d5ce3a1e65dd4abb398d0d369aefe8f71d112c5591aa45eea1e75c",
		Amount:           5000,
		BlindingKey:      blindingKey,
	}
}

func newBlindedOpeningTx(
	t *testing.T,
	liquidOnChain *LiquidOnChain,
	openingParams *swap.OpeningParams,
	committedAsset []byte,
	disclosedAsset []byte,
	committedAbf []byte,
	disclosedAbf []byte,
	amount uint64,
) string {
	t.Helper()

	assetCommitment, err := confidential.AssetCommitment(
		committedAsset, committedAbf,
	)
	checkNoError(t, err)

	valueBlindingFactor := testScalar(4)
	valueCommitment, err := confidential.ValueCommitment(
		amount, assetCommitment, valueBlindingFactor,
	)
	checkNoError(t, err)

	outputScript, err := liquidOnChain.GetOutputScript(openingParams)
	checkNoError(t, err)

	ephemeralKey, err := btcec.NewPrivateKey()
	checkNoError(t, err)
	nonce, err := confidential.NonceHash(
		openingParams.BlindingKey.PubKey().SerializeCompressed(),
		ephemeralKey.Serialize(),
	)
	checkNoError(t, err)

	var valueBlindingFactorArray [32]byte
	copy(valueBlindingFactorArray[:], valueBlindingFactor)
	rangeProof, err := testRangeProof(
		amount,
		nonce,
		disclosedAsset,
		disclosedAbf,
		valueBlindingFactorArray,
		valueCommitment,
		assetCommitment,
		outputScript,
	)
	checkNoError(t, err)

	openingTx := transaction.NewTx(2)
	openingTx.AddInput(transaction.NewTxInput(make([]byte, 32), 0))
	openingTx.AddOutput(&transaction.TxOutput{
		Asset:      assetCommitment,
		Value:      valueCommitment,
		Script:     outputScript,
		Nonce:      ephemeralKey.PubKey().SerializeCompressed(),
		RangeProof: rangeProof,
	})
	txHex, err := openingTx.ToHex()
	checkNoError(t, err)

	return txHex
}

func assertSpendingTransactionRejected(
	t *testing.T,
	liquidOnChain *LiquidOnChain,
	openingParams *swap.OpeningParams,
	openingTxHex string,
	wantError string,
) {
	t.Helper()

	redeemScript, err := ParamsToTxScript(openingParams, LiquidCsv)
	checkNoError(t, err)

	_, _, err = liquidOnChain.createSpendingTransaction(
		openingTxHex,
		openingParams.Amount,
		0,
		liquidOnChain.asset,
		redeemScript,
		"",
		100,
		openingParams.BlindingKey,
		nil,
		nil,
		nil,
	)
	checkErrorContains(t, err, wantError)
}

func checkNoError(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatal(err)
	}
}

func checkErrorContains(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected error containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err, want)
	}
}

func testScalar(value byte) []byte {
	scalar := make([]byte, 32)
	scalar[len(scalar)-1] = value
	return scalar
}

func testRangeProof(
	amount uint64,
	nonce [32]byte,
	disclosedAsset []byte,
	disclosedAbf []byte,
	valueBlindingFactor [32]byte,
	valueCommitment []byte,
	assetCommitment []byte,
	outputScript []byte,
) ([]byte, error) {
	context, _ := secp256k1.ContextCreate(secp256k1.ContextBoth)
	defer secp256k1.ContextDestroy(context)

	commitment, err := secp256k1.CommitmentParse(context, valueCommitment)
	if err != nil {
		return nil, err
	}
	generator, err := secp256k1.GeneratorParse(context, assetCommitment)
	if err != nil {
		return nil, err
	}

	message := make([]byte, 0, len(disclosedAsset)+len(disclosedAbf))
	message = append(message, disclosedAsset...)
	message = append(message, disclosedAbf...)

	return secp256k1.RangeProofSign(
		context,
		1,
		commitment,
		valueBlindingFactor,
		nonce,
		0,
		52,
		amount,
		message,
		outputScript,
		generator,
	)
}
