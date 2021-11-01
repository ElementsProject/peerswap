package swap

import (
	"encoding/json"
	"fmt"
	"io"
)

type JsonEnty struct {
	NodeId   string                                  `json:"node_id"`
	Requests map[string]map[string]*JsonAssetRequest `json:"requests"`
}

type JsonAssetRequest struct {
	AmountMsat uint64 `json:"amount_msat"`
	NRequests  uint64 `json:"n_requests"`
}

type RequestedSwapsPrinter struct {
	store RequestedSwapsStore
}

func NewRequestedSwapsPrinter(store RequestedSwapsStore) *RequestedSwapsPrinter {
	return &RequestedSwapsPrinter{store: store}
}

func (p *RequestedSwapsPrinter) Write(w io.Writer) {
	reqswaps, err := p.Get()
	if err != nil {
		w.Write([]byte(fmt.Sprintf("error reading requested swaps: %v", err)))
	}

	b, err := json.MarshalIndent(reqswaps, "", "\t")
	if err != nil {
		w.Write([]byte(fmt.Sprintf("error marshalling requested swaps: %v", err)))
		return
	}

	w.Write(b)
}

func (p *RequestedSwapsPrinter) Get() ([]JsonEnty, error) {
	reqswaps, err := p.store.GetAll()
	if err != nil {
		return nil, fmt.Errorf("error reading requested swaps: %w", err)
	}

	var reqbuf []JsonEnty
	for nodeId, reqswapz := range reqswaps {
		e := JsonEnty{NodeId: nodeId, Requests: map[string]map[string]*JsonAssetRequest{}}
		for _, reqswap := range reqswapz {
			if _, ok := e.Requests[reqswap.Type.String()]; !ok {
				e.Requests[reqswap.Type.String()] = map[string]*JsonAssetRequest{}
			}
			if _, ok := e.Requests[reqswap.Type.String()][reqswap.Asset]; !ok {
				e.Requests[reqswap.Type.String()][reqswap.Asset] = &JsonAssetRequest{AmountMsat: reqswap.AmountMsat, NRequests: 1}
			} else {
				e.Requests[reqswap.Type.String()][reqswap.Asset].AmountMsat = e.Requests[reqswap.Type.String()][reqswap.Asset].AmountMsat + reqswap.AmountMsat
				e.Requests[reqswap.Type.String()][reqswap.Asset].NRequests = e.Requests[reqswap.Type.String()][reqswap.Asset].NRequests + 1
			}
		}
		reqbuf = append(reqbuf, e)
	}

	return reqbuf, nil
}
