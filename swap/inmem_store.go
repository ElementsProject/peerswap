package swap

import (
	"context"
	"errors"
)

var (
	AlreadyExistsError = errors.New("Swap is already present in database")
	DoesNotExistError = errors.New("Swap does not exist")
)

type InMemStore struct {
	swapMap map[string]*Swap
}

func (i *InMemStore) Create(ctx context.Context, swap *Swap) error {
	if _,ok := i.swapMap[swap.Id]; ok {
		return AlreadyExistsError
	}
	i.swapMap[swap.Id] = swap
	return nil
}

func (i *InMemStore) Update(ctx context.Context, swap *Swap) error {
	if _,ok := i.swapMap[swap.Id]; !ok {
		return DoesNotExistError
	}
	i.swapMap[swap.Id] = swap
	return nil
}

func (i *InMemStore) DeleteById(ctx context.Context, s string) error {
	if _,ok := i.swapMap[s]; !ok {
		return DoesNotExistError
	}
	delete(i.swapMap, s)
	return nil
}

func (i *InMemStore) GetById(ctx context.Context, s string) (*Swap, error) {
	if v,ok := i.swapMap[s]; ok {
		return v,nil
	}
	return nil, DoesNotExistError
}

func (i *InMemStore) ListAll(ctx context.Context) ([]*Swap, error) {
	var swaps []*Swap
	for _,v := range i.swapMap {
		swaps = append(swaps, v)
	}
	return swaps, nil
}

