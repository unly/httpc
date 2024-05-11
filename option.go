package httpc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func WithCheckRedirect(fn func(req *http.Request, via []*http.Request) error) Option {
	return func(cfg *Config) {
		cfg.CheckRedirect = fn
	}
}

func WithCookieJar(jar http.CookieJar) Option {
	return func(cfg *Config) {
		cfg.Jar = jar
	}
}

func WithTimeout(t time.Duration) Option {
	return func(cfg *Config) {
		cfg.Timeout = t
	}
}

func WithTransport(t *http.Transport) Option {
	return func(cfg *Config) {
		cfg.Transport = t
	}
}

func WithLayer(l Layer) Option {
	return func(cfg *Config) {
		cfg.layers = append(cfg.layers, l)
	}
}

func WithErrorHandler(h ErrorHandler) Option {
	return func(cfg *Config) {
		cfg.errorHandler = h
	}
}

func WithHeaders(h http.Header) Option {
	return WithLayer(func(base http.RoundTripper) http.RoundTripper {
		return &HeaderLayer{
			base:    base,
			headers: h,
		}
	})
}

type HeaderLayer struct {
	base    http.RoundTripper
	headers http.Header
}

func (h *HeaderLayer) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, values := range h.headers {
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}

	return h.base.RoundTrip(req)
}

func WithCustomJSONError[E error]() Option {
	return WithErrorHandler(func(c *Client, _ *http.Response, body []byte) error {
		var err E
		parseErr := c.cfg.JsonUnmarshal(body, &err)
		if parseErr != nil {
			return parseErr
		}

		return err
	})
}

func WithJSONError() Option {
	return WithCustomJSONError[JSONErrorBody]()
}

func WithBytesError() Option {
	return WithErrorHandler(bytesErrorHandler)
}

func bytesErrorHandler(_ *Client, _ *http.Response, body []byte) error {
	return BytesErrorBody(body)
}

type JSONErrorBody map[string]any

func (e JSONErrorBody) Error() string {
	encoded, _ := json.Marshal(e)
	return fmt.Sprintf("http body: %s", encoded)
}

type BytesErrorBody []byte

func (e BytesErrorBody) Error() string {
	return fmt.Sprintf("http body: %s", string(e))
}
