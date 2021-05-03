package httpclient

import (
	"crypto/tls"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

var (
	cache = &sync.Pool{
		New: func() interface{} {
			return &option{header: make(map[string][]string)}
		},
	}
)

// Option custom setup config
type Option func(*option)

type option struct {
	ttl             time.Duration
	header          http.Header
	logger          *zap.Logger
	retryTimes      int
	retryDelay      time.Duration
	tlsClientConfig *tls.Config
}

func (o *option) reset() {
	o.ttl = 0
	o.header = make(http.Header)
	o.logger = nil
	o.retryTimes = 0
	o.retryDelay = 0
}

func getOption() *option {
	return cache.Get().(*option)
}

func releaseOption(opt *option) {
	opt.reset()
	cache.Put(opt)
}

// WithTTL how long this rpc will cost
func WithTTL(ttl time.Duration) Option {
	return func(opt *option) {
		opt.ttl = ttl
	}
}

// WithHeader setup header, this func can call multi times
func WithHeader(key, value string) Option {
	return func(opt *option) {
		opt.header.Add(key, value)
	}
}

// WithLogger use logger to log something
func WithLogger(logger *zap.Logger) Option {
	return func(opt *option) {
		opt.logger = logger
	}
}

// WithRetryTimes retry how many times
func WithRetryTimes(retryTimes int) Option {
	return func(opt *option) {
		opt.retryTimes = retryTimes
	}
}

// WithRetryDelay delay how long before retry
func WithRetryDelay(retryDelay time.Duration) Option {
	return func(opt *option) {
		opt.retryDelay = retryDelay
	}
}
