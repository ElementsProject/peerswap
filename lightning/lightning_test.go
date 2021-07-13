package lightning

import (
	"encoding/hex"
	"github.com/stretchr/testify/assert"
	"log"
	"math/big"
	"testing"
)

var (
	featureString = "8000000020008000002822aaa2"
)

func Test_FeatureBit(t *testing.T) {
	flagBytes, err := hex.DecodeString(featureString)
	if err != nil {
		t.Fatal(err)
	}
	hasFeature := checkFeatures(flagBytes, 69)
	assert.True(t, hasFeature)
}

func Test_FeatureBit2(t *testing.T) {
	flagBytes, err := hex.DecodeString(featureString)
	if err != nil {
		t.Fatal(err)
	}
	hasFeature := checkFeatures(flagBytes, 71)
	assert.False(t, hasFeature)
}

func checkFeatures(features []byte, featureBit int64) bool {
	featuresInt := big.NewInt(0)
	featuresInt = featuresInt.SetBytes(features)
	bitInt := big.NewInt(0)
	bitInt = bitInt.Exp(big.NewInt(2), big.NewInt(featureBit), nil)
	compareInt := big.NewInt(0)
	compareInt = compareInt.And(featuresInt, bitInt)
	log.Printf("compare: %v %v %v", featuresInt, bitInt, compareInt)
	return compareInt.Cmp(bitInt) == 0
}
