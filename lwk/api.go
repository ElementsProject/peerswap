package lwk

import (
	"io"
	"net/http"
	"sync/atomic"

	"github.com/elementsproject/glightning/jrpc2"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

type api struct {
	BaseURL        string
	logger         *zap.Logger
	httpClient     *retryablehttp.Client
	interceptors   []InterceptorFunc
	requestCounter int64
}

func NewAPI(baseURL string) *api {
	return &api{
		BaseURL:    baseURL,
		logger:     zap.NewNop(),
		httpClient: defaultHttpClient(),
	}
}

// for now, use a counter as the id for requests
func (a *api) nextID() *jrpc2.Id {
	val := atomic.AddInt64(&a.requestCounter, 1)
	return jrpc2.NewIdAsInt(val)
}

func (a *api) do(req *http.Request) (*http.Response, error) {
	e := a.call
	is := a.interceptors
	for i := len(is) - 1; i >= 0; i-- {
		e = is[i](e)
	}
	return e(req)
}

func (a *api) call(req *http.Request) (*http.Response, error) {
	req.Header.Set("Content-Type", "application/json")
	rReq, err := retryablehttp.FromRequest(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create api request")
	}
	res, err := a.httpClient.Do(rReq)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call api request")
	}
	return res, nil
}

func (a *api) drain(res *http.Response) {
	defer func() {
		_ = res.Body.Close()
	}()
	_, err := io.Copy(io.Discard, res.Body)
	if err != nil {
		a.logger.Warn("failed to drain response body")
	}
}
