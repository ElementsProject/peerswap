package swap

import "sync"

type requestedSwapsStoreMock struct {
	sync.RWMutex
	data      map[string][]RequestedSwap
	errReturn error
}

func (s *requestedSwapsStoreMock) Add(id string, reqswap RequestedSwap) error {
	if s.errReturn != nil {
		return s.errReturn
	}
	s.Lock()
	defer s.Unlock()
	if data, ok := s.data[id]; ok {
		s.data[id] = append(data, reqswap)
	} else {
		s.data[id] = []RequestedSwap{reqswap}
	}
	return nil
}
func (s *requestedSwapsStoreMock) Get(id string) ([]RequestedSwap, error) {
	if s.errReturn != nil {
		return nil, s.errReturn
	}
	if data, ok := s.data[id]; ok {
		return data, nil
	}
	return nil, nil
}
func (s *requestedSwapsStoreMock) GetAll() (map[string][]RequestedSwap, error) {
	if s.errReturn != nil {
		return nil, s.errReturn
	}
	return s.data, nil
}
