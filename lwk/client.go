package lwk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/elementsproject/glightning/jrpc2"
	"github.com/elementsproject/peerswap/log"
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
	WithTextQr bool    `json:"with_text_qr"`
	WithUriQr  *uint8  `json:"with_uri_qr,omitempty"`
}

func (r *addressRequest) Name() string {
	return "wallet_address"
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
	FeeRate          *float64 `json:"fee_rate,omitempty"`
	WalletName       string   `json:"name"`
	EnableCtDiscount bool     `json:"enable_ct_discount"`
}

type sendResponse struct {
	Pset string `json:"pset"`
}

func (s *sendRequest) Name() string {
	return "wallet_send_many"
}

// send sends a request using the lwkclient and handles retries in case of a
// "missing transaction" error. It uses an exponential backoff strategy for
// retries, with a maximum of 5 retries. This is a temporary workaround for
// an issue where a missing transaction error occurs even when the UTXO exists.
// If the issue persists, the backoff strategy may need adjustment.
func (l *lwkclient) send(ctx context.Context, s *sendRequest) (*sendResponse, error) {
	var resp sendResponse
	// Allow configuration of maxRetries and backoff strategy
	maxRetries := 5
	backoffStrategy := backoff.NewExponentialBackOff()
	backoffStrategy.MaxElapsedTime = 2 * time.Minute

	err := backoff.Retry(func() error {
		innerErr := l.request(ctx, s, &resp)
		if innerErr != nil {
			log.Infof("Error during send request: %v", innerErr)
			if strings.Contains(innerErr.Error(), "missing transaction") {
				log.Infof("Retrying due to missing transaction error: %v", innerErr)
				return innerErr
			}
			return backoff.Permanent(fmt.Errorf("permanent error: %w", innerErr))
		}
		return nil
	}, backoff.WithMaxRetries(backoffStrategy, uint64(maxRetries)))
	return &resp, err
}

type signRequest struct {
	SignerName string `json:"name"`
	Pset       string `json:"pset"`
}

type signResponse struct {
	Pset string `json:"pset"`
}

func (s *signRequest) Name() string {
	return "signer_sign"
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
	return "wallet_broadcast"
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
	return "wallet_balance"
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

type generateSignerRequest struct {
}

func (r *generateSignerRequest) Name() string {
	return "signer_generate"
}

type generateSignerResponse struct {
	Mnemonic string `json:"mnemonic"`
}

func (l *lwkclient) generateSigner(ctx context.Context) (*generateSignerResponse, error) {
	var resp generateSignerResponse
	err := l.request(ctx, &generateSignerRequest{}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type loadSoftwareSignerRequest struct {
	Mnemonic   string `json:"mnemonic"`
	SignerName string `json:"name"`
	Persist    bool   `json:"persist"`
}

func (r *loadSoftwareSignerRequest) Name() string {
	return "signer_load_software"
}

type loadSoftwareSignerResponse struct {
	Fingerprint string `json:"fingerprint"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Xpub        string `json:"xpub"`
}

func (l *lwkclient) loadSoftwareSigner(ctx context.Context, req *loadSoftwareSignerRequest) (*loadSoftwareSignerResponse, error) {
	var resp loadSoftwareSignerResponse
	err := l.request(ctx, req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type singlesigDescriptorRequest struct {
	DescriptorBlindingKey string `json:"descriptor_blinding_key"`
	SignerName            string `json:"name"`
	SinglesigKind         string `json:"singlesig_kind"`
}

func (r *singlesigDescriptorRequest) Name() string {
	return "signer_singlesig_descriptor"
}

type singlesigDescriptorResponse struct {
	Descriptor string `json:"descriptor"`
}

func (l *lwkclient) singlesigDescriptor(ctx context.Context, req *singlesigDescriptorRequest) (*singlesigDescriptorResponse, error) {
	var resp singlesigDescriptorResponse
	err := l.request(ctx, req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type loadWalletRequest struct {
	Descriptor string `json:"descriptor"`
	WalletName string `json:"name"`
}

func (r *loadWalletRequest) Name() string {
	return "wallet_load"
}

type loadWalletResponse struct {
	Descriptor string `json:"descriptor"`
	Name       string `json:"name"`
}

func (l *lwkclient) loadWallet(ctx context.Context, req *loadWalletRequest) (*loadWalletResponse, error) {
	var resp loadWalletResponse
	err := l.request(ctx, req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type versionRequest struct {
}

func (r *versionRequest) Name() string {
	return "version"
}

type versionResponse struct {
	Version string `json:"version"`
	Network string `json:"network"`
}

func (l *lwkclient) version(ctx context.Context) (*versionResponse, error) {
	var resp versionResponse
	err := l.request(ctx, &versionRequest{}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type WalletSetTxMemoRequest struct {
	Memo       string `json:"memo"`
	WalletName string `json:"name"`
	Txid       string `json:"txid"`
}

func (r *WalletSetTxMemoRequest) Name() string {
	return "wallet_set_tx_memo"
}

type WalletSetTxMemoResponse struct {
}

func (l *lwkclient) walletSetTxMemo(ctx context.Context, req *WalletSetTxMemoRequest) error {
	return l.request(ctx, req, &WalletSetTxMemoResponse{})
}
