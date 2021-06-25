package swap

type InMemStore struct {
	swapMap map[string]*Swap
}

func NewInMemStore() *InMemStore {
	return &InMemStore{swapMap: make(map[string]*Swap)}
}

func (i *InMemStore) Create(swap *Swap) error {
	if _, ok := i.swapMap[swap.Id]; ok {
		return ErrAlreadyExists
	}
	i.swapMap[swap.Id] = swap
	return nil
}

func (i *InMemStore) Update(swap *Swap) error {
	if _, ok := i.swapMap[swap.Id]; !ok {
		return ErrDoesNotExist
	}
	i.swapMap[swap.Id] = swap
	return nil
}

func (i *InMemStore) DeleteById(s string) error {
	if _, ok := i.swapMap[s]; !ok {
		return ErrDoesNotExist
	}
	delete(i.swapMap, s)
	return nil
}

func (i *InMemStore) GetById(s string) (*Swap, error) {
	if v, ok := i.swapMap[s]; ok {
		return v, nil
	}
	return nil, ErrDoesNotExist
}

func (i *InMemStore) ListAll() ([]*Swap, error) {
	var swaps []*Swap
	for _, v := range i.swapMap {
		swaps = append(swaps, v)
	}
	return swaps, nil
}
