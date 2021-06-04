package tests

import (
	"github.com/sputn1ck/liquid-loop/gelements"
	"testing"
)

func TestElementsConnection(t *testing.T) {
	elements := gelements.NewElements("admin1","123")
	elements.StartUp("http://localhost", 7041)

	ci, err := elements.GetChainInfo()
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("%v",ci)
}
