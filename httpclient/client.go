package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	// DefaultTTL the entire time cost, contains N times retry
	DefaultTTL = time.Minute
	// DefaultRetryTimes retry how many times
	DefaultRetryTimes = 3
	// DefaultRetryDelay delay some time before retry
	DefaultRetryDelay = time.Millisecond * 100

	// HTTP StatusCode
	// _StatusReadRespErr read resp body err, should re-call doHTTP again.
	_StatusReadRespErr = -204
	// _StatusDoReqErr do req err, should re-call doHTTP again.
	_StatusDoReqErr = -500
)


// TODO retry的code不一定正确，缺失或者多余待实际使用中修改。
func shouldRetry(ctx context.Context, httpCode int) bool {
	select {
	case <-ctx.Done():
		return false
	default:
	}

	switch httpCode {
	case
		_StatusDoReqErr,    // customize
		_StatusReadRespErr, // customize

		http.StatusRequestTimeout,
		http.StatusLocked,
		http.StatusTooEarly,
		http.StatusTooManyRequests,

		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:

		return true

	default:
		return false
	}
}

// Get get something by form values
func Get(url string, query url.Values, options ...Option) (body []byte, err error) {
	if url == "" {
		return nil, errors.New("url required")
	}

	if len(query) > 0 {
		if url, err = AddQueryValuesIntoURL(url, query); err != nil {
			return
		}
	}

	opt := getOption()
	defer func() {
		releaseOption(opt)
	}()

	for _, f := range options {
		f(opt)
	}
	opt.header["Content-Type"] = []string{"application/x-www-form-urlencoded; charset=utf-8"}

	ttl := opt.ttl
	if ttl <= 0 {
		ttl = DefaultTTL
	}

	ctx, cancel := context.WithTimeout(context.Background(), ttl)
	defer cancel()

	retryTimes := opt.retryTimes
	if retryTimes <= 0 {
		retryTimes = DefaultRetryTimes
	}

	retryDelay := opt.retryDelay
	if retryDelay <= 0 {
		retryDelay = DefaultRetryDelay
	}

	var httpCode int
	for k := 0; k < retryTimes; k++ {
		body, httpCode, err = doHTTP(ctx, http.MethodGet, url, opt.header, nil, opt)
		if shouldRetry(ctx, httpCode) {
			time.Sleep(retryDelay)
			continue
		}

		return
	}

	err = errors.Errorf("%s %s still fails after %d retries", http.MethodGet, url, retryTimes)
	return
}

// PostForm post some form values
func PostForm(url string, form url.Values, options ...Option) (body []byte, err error) {
	if url == "" {
		return nil, errors.New("url required")
	}
	if len(form) == 0 {
		return nil, errors.New("form required")
	}

	opt := getOption()
	defer func() {
		releaseOption(opt)
	}()

	for _, f := range options {
		f(opt)
	}
	opt.header["Content-Type"] = []string{"application/x-www-form-urlencoded; charset=utf-8"}

	ttl := opt.ttl
	if ttl <= 0 {
		ttl = DefaultTTL
	}

	ctx, cancel := context.WithTimeout(context.Background(), ttl)
	defer cancel()

	formValue := form.Encode()

	retryTimes := opt.retryTimes
	if retryTimes <= 0 {
		retryTimes = DefaultRetryTimes
	}

	retryDelay := opt.retryDelay
	if retryDelay <= 0 {
		retryDelay = DefaultRetryDelay
	}

	var httpCode int
	for k := 0; k < retryTimes; k++ {
		body, httpCode, err = doHTTP(ctx, http.MethodPost, url, opt.header, []byte(formValue), opt)
		if shouldRetry(ctx, httpCode) {
			time.Sleep(retryDelay)
			continue
		}

		return
	}

	err = errors.Errorf("%s %s still fails after %d retries", http.MethodPost, url, retryTimes)
	return
}

// PostJSON post some json
func PostJSON(url string, raw json.RawMessage, options ...Option) (body []byte, err error) {
	if url == "" {
		return nil, errors.New("url required")
	}
	if len(raw) == 0 {
		return nil, errors.New("raw required")
	}

	opt := getOption()
	defer func() {

		releaseOption(opt)
	}()

	for _, f := range options {
		f(opt)
	}
	opt.header["Content-Type"] = []string{"application/json; charset=utf-8"}

	ttl := opt.ttl
	if ttl <= 0 {
		ttl = DefaultTTL
	}

	ctx, cancel := context.WithTimeout(context.Background(), ttl)
	defer cancel()

	retryTimes := opt.retryTimes
	if retryTimes <= 0 {
		retryTimes = DefaultRetryTimes
	}

	retryDelay := opt.retryDelay
	if retryDelay <= 0 {
		retryDelay = DefaultRetryDelay
	}

	var httpCode int
	for k := 0; k < retryTimes; k++ {
		body, httpCode, err = doHTTP(ctx, http.MethodPost, url, opt.header, raw, opt)
		if shouldRetry(ctx, httpCode) {
			time.Sleep(retryDelay)
			continue
		}

		return
	}

	err = errors.Errorf("%s %s still fails after %d retries", http.MethodPost, url, retryTimes)
	return
}


func doHTTP(ctx context.Context, method, url string, header map[string][]string, payload []byte, opt *option) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
	if err != nil {
		return nil, -1, errors.Wrapf(err, "new request %s %s err", method, url)
	}

	req.Header = opt.header
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		err = errors.Wrapf(err, "do request %s %s err", method, url)

		if opt.logger != nil {
			opt.logger.Warn("doHTTP got err", zap.Error(err))
		}
		return nil, _StatusDoReqErr, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil && resp.StatusCode != http.StatusNoContent {
		err = errors.Wrapf(err, "read resp body from %s %s err", method, url)

		if opt.logger != nil {
			opt.logger.Warn("doHTTP got err", zap.Error(err))
		}
		return nil, _StatusReadRespErr, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, newReplyErr(
			resp.StatusCode,
			body,
			errors.Errorf("do %s %s return code: %d message: %s", method, url, resp.StatusCode, string(body)),
		)
	}

	return body, http.StatusOK, nil
}

// AddQueryValuesIntoURL append url.Values into url string
func AddQueryValuesIntoURL(rawURL string, query url.Values) (string, error) {
	if rawURL == "" {
		return "", errors.New("rawURL required")
	}
	if len(query) == 0 {
		return "", errors.New("form required")
	}

	target, err := url.Parse(rawURL)
	if err != nil {
		return "", errors.Wrapf(err, "parse rawURL `%s` err", rawURL)
	}

	urlValues := target.Query()
	for key, values := range query {
		for _, value := range values {
			urlValues.Add(key, value)
		}
	}

	target.RawQuery = urlValues.Encode()
	return target.String(), nil
}
