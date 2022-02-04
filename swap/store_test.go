package swap

import (
	"encoding/json"
	"go.etcd.io/bbolt"
	"log"
	"path/filepath"
	"testing"
)

func Test_EventCtxMarshal(t *testing.T) {
	dir := t.TempDir()
	swapDb, err := bbolt.Open(filepath.Join(dir, "swaps"), 0700, nil)
	if err != nil {
		t.Fatal(err)
	}

	db, err := NewBboltStore(swapDb)
	if err != nil {
		t.Fatal(err)
	}
	swapId := NewSwapId()
	swapfsm := newSwapInReceiverFSM(swapId, nil)
	err = db.Create(swapfsm)
	if err != nil {
		t.Fatal(err)
	}
	msg := SwapOutAgreementMessage{
		SwapId:          swapId,
		ProtocolVersion: 1,
		Pubkey:          "123123",
	}
	swapfsm.Data.LastMessage = msg
	err = db.Update(swapfsm)
	if err != nil {
		t.Fatal(err)
	}

	swapfsm2, err := db.GetById(swapId.String())
	if err != nil {
		t.Fatal(err)
	}
	log.Printf("%x", swapfsm2.Data.LastMessage)

	var msg2 SwapOutAgreementMessage
	err = Marshal(swapfsm2.Data.LastMessage, &msg2)
	if err != nil {
		t.Fatal(err)
	}

	log.Printf("%x", msg2)
	if msg.Pubkey != "123123" {
		t.Fatal("pubkey is incorrect")
	}
	swapDb.Close()
}

func Marshal(ctx EventContext, v interface{}) error {
	jsonBytes, err := json.Marshal(ctx)
	if err != nil {
		return err
	}
	err = json.Unmarshal(jsonBytes, v)
	if err != nil {
		return err
	}
	return nil
}
