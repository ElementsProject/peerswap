package swap

import (
	"context"
	"reflect"
	"testing"
)

func TestInMemStore(t *testing.T) {

	store := NewInMemStore()
	storeTest(t, store)

}

func storeTest(t *testing.T, store SwapStore) {
	ctx := context.Background()

	swap1, err := NewSwap(SWAPTYPE_IN, "foo", "bar", 100)
	if err != nil {
		t.Fatal(err)
	}
	swap2, err := NewSwap(SWAPTYPE_OUT, "baz", "qux", 100)
	if err != nil {
		t.Fatal(err)
	}

	err = store.Create(ctx, swap1)
	if err != nil {
		t.Fatal(err)
	}

	err = store.Create(ctx, swap1)
	if err != nil && err != AlreadyExistsError {
		t.Fatal(err)
	}

	err = store.Create(ctx, swap2)
	if err != nil {
		t.Fatal(err)
	}

	swap3, err := store.GetById(ctx, swap1.Id)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(swap1, swap3) {
		t.Fail()
	}

	_, err = store.GetById(ctx, "foobar")
	if err != nil && err != DoesNotExistError {
		t.Fatal(err)
	}

	swaps, err := store.ListAll(ctx)
	if err != nil {
		t.Fatal()
	}
	if len(swaps) != 2 {
		t.Fail()
	}
	err = store.DeleteById(ctx, swap3.Id)
	if err != nil {
		t.Fatal(err)
	}
	err = store.DeleteById(ctx, swap3.Id)
	if err != nil && err != DoesNotExistError {
		t.Fatal(err)
	}

	swaps, err = store.ListAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(swaps) != 1 {
		t.Fail()
	}
	err = store.Update(ctx, swap1)
	if err != nil && err != DoesNotExistError {
		t.Fatal(err)
	}

	swap2.TakerNodeId = "foobaz"
	err = store.Update(ctx, swap2)
	if err != nil {
		t.Fatal(err)
	}
	swap3, err = store.GetById(ctx, swap2.Id)
	if err != nil {
		t.Fatal(err)
	}
	if swap3.TakerNodeId != swap2.TakerNodeId {
		t.Fail()
	}
}

func TestNewSwapId(t *testing.T) {
	id1, err := newSwapId()
	if err != nil {
		t.Fatal(err)
	}
	id2, err := newSwapId()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s, %s", id1, id2)
	if id1 == id2 {
		t.Fail()
	}
}
