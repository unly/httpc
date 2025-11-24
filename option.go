package httpc

import (
	"net/http"
	"time"
)

// ClientOption function to modify the Config when creating or updating
// a Client.
type ClientOption func(cfg *Config)

// WithCheckRedirect sets the redirect function for the http.Client.
// Defaults to nil.
func WithCheckRedirect(fn func(req *http.Request, via []*http.Request) error) ClientOption {
	return func(cfg *Config) {
		cfg.CheckRedirect = fn
	}
}

// WithCookieJar sets the cookie jar implementation for the http.Client.
// Defaults to nil.
func WithCookieJar(jar http.CookieJar) ClientOption {
	return func(cfg *Config) {
		cfg.Jar = jar
	}
}

// WithTimeout sets the timeout used for every HTTP request. Defaults to
// DefaultTimeout.
func WithTimeout(t time.Duration) ClientOption {
	return func(cfg *Config) {
		cfg.Timeout = t
	}
}

// WithTransport sets the http.Transport user for every HTTP request.
// Defaults to DefaultTransport.
func WithTransport(t *http.Transport) ClientOption {
	return func(cfg *Config) {
		cfg.Transport = t
	}
}

// WithLayer adds a new Layer to the stack of layers executed for every
// HTTP request.
func WithLayer(l Layer) ClientOption {
	return func(cfg *Config) {
		cfg.layers = append(cfg.layers, l)
	}
}

// WithHeaders adds the given headers to every outgoing call by default.
func WithHeaders(h http.Header) ClientOption {
	return WithLayer(func(base http.RoundTripper) http.RoundTripper {
		return &headerLayer{
			base:    base,
			headers: h,
		}
	})
}

type headerLayer struct {
	base    http.RoundTripper
	headers http.Header
}

func (h *headerLayer) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, values := range h.headers {
		req.Header[k] = append(req.Header[k], values...)
	}

	return h.base.RoundTrip(req)
}

// WithRespOption adds a default response option used in every
// Client.DoReq call before the furtherly passed response options.
func WithRespOption(opt RespOption) ClientOption {
	return func(cfg *Config) {
		cfg.respOptions = append(cfg.respOptions, opt)
	}
}
