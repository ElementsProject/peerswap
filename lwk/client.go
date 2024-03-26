package lwk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/elementsproject/glightning/jrpc2"
)

type lwkclient struct {
	api api
}

func NewLwk(endpoint string) *lwkclient {
	return &lwkclient{
		api: *NewAPI(endpoint),
	}
}

func (l *lwkclient) request(ctx context.Context, m jrpc2.Method, resp interface{}) error {
	id := l.api.nextID()
	mr := &jrpc2.Request{Id: id, Method: m}
	jbytes, err := json.Marshal(mr)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.api.BaseURL, bytes.NewBuffer(jbytes))
	if err != nil {
		return err
	}
	rezp, err := l.api.do(req)
	if err != nil {
		return err
	}
	defer l.api.drain(rezp)
	switch rezp.StatusCode {
	case http.StatusUnauthorized:
		return errors.New("authorization failed: Incorrect user or password")
	case http.StatusBadRequest, http.StatusNotFound, http.StatusInternalServerError:
		// do nothing
	default:
		if rezp.StatusCode > http.StatusBadRequest {
			return errors.New(fmt.Sprintf("server returned HTTP error %d", rezp.StatusCode))
		} else if rezp.ContentLength == 0 {
			return errors.New("no response from server")
		}
	}

	var rawResp jrpc2.RawResponse

	decoder := json.NewDecoder(rezp.Body)
	err = decoder.Decode(&rawResp)
	if err != nil {
		return err
	}

	if rawResp.Error != nil {
		return rawResp.Error
	}
	return json.Unmarshal(rawResp.Raw, resp)
}

type addressRequest struct {
	Index      *uint32 `json:"index,omitempty"`
	WalletName string  `json:"name"`
	Signer     *string `json:"signer,omitempty"`
}

func (r *addressRequest) Name() string {
	return "address"
}

type addressResponse struct {
	Address string  `json:"address"`
	Index   *uint32 `json:"index,omitempty"`
}

func (l *lwkclient) address(ctx context.Context, req *addressRequest) (*addressResponse, error) {
	var resp addressResponse
	err := l.request(ctx, req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type unvalidatedAddressee struct {
	Address string `json:"address"`
	Asset   string `json:"asset"`
	Satoshi uint64 `json:"satoshi"`
}

type sendRequest struct {
	Addressees []*unvalidatedAddressee `json:"addressees"`
	// Optional fee rate in sat/vb
	FeeRate    *float64 `json:"fee_rate,omitempty"`
	WalletName string   `json:"name"`
}

type sendResponse struct {
	Pset string `json:"pset"`
}

func (s *sendRequest) Name() string {
	return "send_many"
}

func (l *lwkclient) send(ctx context.Context, s *sendRequest) (*sendResponse, error) {
	var resp sendResponse
	err := l.request(ctx, s, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type signRequest struct {
	SignerName string `json:"name"`
	Pset       string `json:"pset"`
}

type signResponse struct {
	Pset string `json:"pset"`
}

func (s *signRequest) Name() string {
	return "sign"
}

func (l *lwkclient) sign(ctx context.Context, s *signRequest) (*signResponse, error) {
	var resp signResponse
	err := l.request(ctx, s, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type broadcastRequest struct {
	DryRun     bool   `json:"dry_run"`
	WalletName string `json:"name"`
	Pset       string `json:"pset"`
}

type broadcastResponse struct {
	Txid string `json:"txid"`
}

func (b *broadcastRequest) Name() string {
	return "broadcast"
}

func (l *lwkclient) broadcast(ctx context.Context, b *broadcastRequest) (*broadcastResponse, error) {
	var resp broadcastResponse
	err := l.request(ctx, b, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type balanceRequest struct {
	WalletName  string `json:"name"`
	WithTickers bool   `json:"with_tickers"`
}

func (b *balanceRequest) Name() string {
	return "balance"
}

type balanceResponse struct {
	Balance map[string]int64 `json:"balance"`
}

func (l *lwkclient) balance(ctx context.Context, b *balanceRequest) (*balanceResponse, error) {
	var resp balanceResponse
	err := l.request(ctx, b, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type walletDetailsRequest struct {
	WalletName string `json:"name"`
}

func (w *walletDetailsRequest) Name() string {
	return "wallet_details"
}

type signerDetails struct {
	Fingerprint string `json:"fingerprint"`
	Name        string `json:"name"`
}

type walletDetailsResponse struct {
	Signers  []signerDetails `json:"signers"`
	Type     string          `json:"type"`
	Warnings string          `json:"warnings"`
}

func (l *lwkclient) walletDetails(ctx context.Context, w *walletDetailsRequest) (*walletDetailsResponse, error) {
	var resp walletDetailsResponse
	err := l.request(ctx, w, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}
