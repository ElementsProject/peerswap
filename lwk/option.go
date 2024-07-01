package lwk

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"go.uber.org/zap"
)

type Option struct {
	ConnTimeout  time.Duration
	ReadTimeOut  time.Duration
	RetryWaitMin time.Duration
	RetryWaitMax time.Duration
	RetryMax     int
}
type LogWrapper struct {
	logger *zap.Logger
}

func (a *api) WithLogger(logger *zap.Logger) *api {
	a.logger = logger
	a.httpClient.Logger = &LogWrapper{logger: logger}
	return a
}

func (a *api) WithOption(option *Option) *api {
	setHttpClientOption(a.httpClient, option)
	return a
}

type (
	InterceptorFunc    func(RequestHandlerFunc) RequestHandlerFunc
	RequestHandlerFunc func(*http.Request) (*http.Response, error)
)

func (a *api) WithInterceptors(is ...InterceptorFunc) *api {
	a.interceptors = is
	return a
}

func defaultOption() *Option {
	return &Option{
		ConnTimeout:  10 * time.Second,
		ReadTimeOut:  10 * time.Second,
		RetryWaitMin: 1 * time.Second,
		RetryWaitMax: 3 * time.Second,
		RetryMax:     1,
	}
}

func defaultHttpClient() *retryablehttp.Client {
	c := retryablehttp.NewClient()
	c.HTTPClient = &http.Client{}
	c.Backoff = retryablehttp.LinearJitterBackoff // use jitter
	c.ErrorHandler = nil                          // not used
	c.Logger = nil                                // disable default logger
	c.CheckRetry = checkRetry
	setHttpClientOption(c, defaultOption())
	return c
}

func checkRetry(ctx context.Context, res *http.Response, err error) (bool, error) {
	doRetry, err := retryablehttp.ErrorPropagatedRetryPolicy(ctx, res, err)
	if doRetry && res != nil {
		// if a response is received, retry is terminated
		return false, nil
	}
	return doRetry, err
}

func setHttpClientOption(c *retryablehttp.Client, o *Option) {
	if o.ConnTimeout > 0 {
		c.HTTPClient.Transport = transportWithTimeout(o.ConnTimeout)
	}
	if o.ReadTimeOut > 0 {
		c.HTTPClient.Timeout = o.ReadTimeOut
	}
	if o.RetryWaitMin > 0 {
		c.RetryWaitMin = o.RetryWaitMin
	}
	if o.RetryWaitMax > 0 {
		c.RetryWaitMax = o.RetryWaitMax
	}
	c.RetryMax = o.RetryMax
}

func transportWithTimeout(d time.Duration) *http.Transport {
	// clone default transport
	dtp, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil
	}
	tp := dtp.Clone()
	// set timeout
	dial := &net.Dialer{Timeout: d, KeepAlive: 30 * time.Second}
	tp.DialContext = (dial).DialContext
	return tp
}
