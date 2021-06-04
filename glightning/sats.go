package glightning

import (
	"fmt"
)

type Sat struct {
	Value   uint64
	SendAll bool
}

type MSat struct {
	Value uint64
}

func NewMsat(val uint64) *MSat {
	return &MSat{val}
}

// Always rounds up to nearest satoshi
func (m *MSat) ConvertSat() *Sat {
	a := m.Value / 1000
	if m.Value%1000 > 0 {
		a += 1
	}
	return &Sat{a, false}
}

func (s *Sat) ConvertMsat() *MSat {
	v := s.Value * 1000

	// we rolled. flat panic.
	if v < s.Value {
		panic(fmt.Sprintf("overflowed converting %dmsats to sats", s.Value))
	}

	return &MSat{v}
}

func (m *MSat) String() string {
	return fmt.Sprintf("%dmsat", m.Value)
}

func ConvertBtc(btc float64) *Sat {
	sat := btc * 100000000
	if sat != btc*100000000 {
		panic(fmt.Sprintf("overflowed converting %f to sat", btc))
	}
	return NewSat64(uint64(sat))
}

func (s *Sat) RawString() string {
	if s.SendAll {
		return "all"
	}
	return fmt.Sprintf("%d", s.Value)
}

func (s *Sat) String() string {
	if s.SendAll {
		return "all"
	}
	return fmt.Sprintf("%dsat", s.Value)
}

func NewSat64(amount uint64) *Sat {
	return &Sat{
		Value: amount,
	}
}

func NewSat(amount int) *Sat {
	return &Sat{
		Value: uint64(amount),
	}
}

func AllSats() *Sat {
	return &Sat{
		SendAll: true,
	}
}
