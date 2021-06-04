package glightning

import (
	"encoding/hex"
	"fmt"
	"math/big"
)

type Hexed struct {
	Str string
	Raw []byte
}

func (h *Hexed) String() string {
	return h.Str
}

func (h *Hexed) MarshalJSON() ([]byte, error) {
	// we return the marshaled bytes
	dst := make([]byte, hex.EncodedLen(len(h.Raw)))
	hex.Encode(dst, h.Raw)

	// gotta make it into a string
	ret := make([]byte, 1)
	ret[0] = '"'
	ret = append(ret, dst...)
	ret = append(ret, '"')

	return ret, nil
}

func (h *Hexed) UnmarshalJSON(b []byte) error {
	// assumed string, check first and last are '"'
	if b[0] != '"' || b[len(b)-1] != '"' {
		return fmt.Errorf("%s is not a string", string(b))
	}
	// trim string markers
	b = b[1 : len(b)-1]

	// b - bytes of string of hex
	h.Raw = make([]byte, hex.DecodedLen(len(b)))
	_, err := hex.Decode(h.Raw, b)
	if err != nil {
		return err
	}
	h.Str = string(b)
	return nil
}

func NewHex(hexstring string) (*Hexed, error) {
	raw, err := hex.DecodeString(hexstring)
	if err != nil {
		return nil, err
	}
	return &Hexed{hexstring, raw}, nil
}

func NewHexx(hexb []byte) *Hexed {
	return &Hexed{hex.EncodeToString(hexb), hexb}
}

func (h *Hexed) IsSet(bitpos int) bool {
	bg := big.NewInt(0)
	bg.SetBytes(h.Raw)
	return bg.Bit(bitpos) == 1
}
