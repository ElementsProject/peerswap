package lightning

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// From https://github.com/lightningnetwork/lnd/blob/master/lntypes/preimage.go

// PreimageSize of array used to store preimagees.
const PreimageSize = 32

// Preimage is used in several of the lightning messages and common structures.
// It represents a payment preimage.
type Preimage [PreimageSize]byte

// String returns the Preimage as a hexadecimal string.
func (p Preimage) String() string {
	return hex.EncodeToString(p[:])
}

// MakePreimage returns a new Preimage from a bytes slice. An error is returned
// if the number of bytes passed in is not PreimageSize.
func MakePreimage(newPreimage []byte) (Preimage, error) {
	nhlen := len(newPreimage)
	if nhlen != PreimageSize {
		return Preimage{}, fmt.Errorf("invalid preimage length of %v, "+
			"want %v", nhlen, PreimageSize)
	}

	var preimage Preimage
	copy(preimage[:], newPreimage)

	return preimage, nil
}

// RandomPreimage returns a random Preimage
func GetPreimage() (Preimage, error) {
	var preimage Preimage

	if _, err := rand.Read(preimage[:]); err != nil {
		return preimage, err
	}
	return preimage, nil
}

// MakePreimageFromStr creates a Preimage from a hex preimage string.
func MakePreimageFromStr(newPreimage string) (Preimage, error) {
	// Return error if preimage string is of incorrect length.
	if len(newPreimage) != PreimageSize*2 {
		return Preimage{}, fmt.Errorf("invalid preimage string length "+
			"of %v, want %v", len(newPreimage), PreimageSize*2)
	}

	preimage, err := hex.DecodeString(newPreimage)
	if err != nil {
		return Preimage{}, err
	}

	return MakePreimage(preimage)
}

// Hash returns the sha256 hash of the preimage.
func (p *Preimage) Hash() Hash {
	return Hash(sha256.Sum256(p[:]))
}

// Matches returns whether this preimage is the preimage of the given hash.
func (p *Preimage) Matches(h Hash) bool {
	return h == p.Hash()
}

// HashSize of array used to store hashes.
const HashSize = 32

// ZeroHash is a predefined hash containing all zeroes.
var ZeroHash Hash

// Hash is used in several of the lightning messages and common structures. It
// typically represents a payment hash.
type Hash [HashSize]byte

// String returns the Hash as a hexadecimal string.
func (hash Hash) String() string {
	return hex.EncodeToString(hash[:])
}
