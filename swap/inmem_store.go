package swap

import (
	"errors"
)

var (
	AlreadyExistsError = errors.New("Swap is already present in database")
	DoesNotExistError  = errors.New("Swap does not exist")
)

type InMemStore struct {
	swapMap map[string]*Swap
}

func NewInMemStore() *InMemStore {
	return &InMemStore{swapMap: make(map[string]*Swap)}
}

func (i *InMemStore) Create(swap *Swap) error {
	if _, ok := i.swapMap[swap.Id]; ok {
		return AlreadyExistsError
	}
	i.swapMap[swap.Id] = swap
	return nil
}

func (i *InMemStore) Update(swap *Swap) error {
	if _, ok := i.swapMap[swap.Id]; !ok {
		return DoesNotExistError
	}
	i.swapMap[swap.Id] = swap
	return nil
}

func (i *InMemStore) DeleteById(s string) error {
	if _, ok := i.swapMap[s]; !ok {
		return DoesNotExistError
	}
	delete(i.swapMap, s)
	return nil
}

func (i *InMemStore) GetById(s string) (*Swap, error) {
	if v, ok := i.swapMap[s]; ok {
		return v, nil
	}
	return nil, DoesNotExistError
}

func (i *InMemStore) ListAll() ([]*Swap, error) {
	var swaps []*Swap
	for _, v := range i.swapMap {
		swaps = append(swaps, v)
	}
	return swaps, nil
}
