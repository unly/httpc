package httpc

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/quic-go/quic-go/http3"
)

const DefaultTimeout = 30 * time.Second

var DefaultTransport = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          100,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

type (
	// Config struct holding all the configurations of an HTTP Client.
	// Can be modified via the With* options.
	Config struct {
		// Transport a pointer to the underlying http.Transport. This is used as
		// the base for the http.Client and the http.RoundTripper. Defaults to
		// DefaultTransport. Can be set via WithTransport.
		Transport *http.Transport
		// H3Transport is an optional pointer to a http3.Transport. If this is
		// set the Client attempts to try h3 connections first. Based on the domain
		// the Client stores if the domain supports h3 or not. Can be set via
		// WithH3Transport.
		H3Transport *http3.Transport
		// CheckRedirect redirecting logic as defined in the http.Client to
		// determine redirects. Defaults to nil. Can be set via WithCheckRedirect.
		CheckRedirect func(req *http.Request, via []*http.Request) error
		// Jar the cookie storage logic of the http.CookieJar to be used by the
		// http.Client. Defaults to nil. Can be set via WithCookieJar.
		Jar http.CookieJar
		// Timeout for each outgoing HTTP request. A value of 0 means no timeout.
		// Defaults to DefaultTimeout. Can be set via WithTimeout.
		Timeout time.Duration
		// MemoryPooling enables the use of a memory pool to hold buffers for
		// reading in the HTTP bodies. This feature can result in performance
		// increases but does not set the http.Response body back after reading.
		// Defaults to false. Can be set via WithMemoryPooling.
		MemoryPooling bool
		// Shutdowns slice of shutdown functions executed on Client.Close call.
		Shutdowns []func() error

		layers      []Layer
		respOptions []RespOption
	}

	// Client the HTTP client wraps an existing http.Client with some helper
	// function. Can be used as a regular http.Client. All Layer will be applied
	// to the underlying client and therefore will be executed even for calls
	// such as Do() or Get(). Call Unwrap to get the underlying client to use
	// it as a regular http.Client.
	Client struct {
		*http.Client

		cfg Config
	}

	// Layer is a function wo wrap one http.RoundTripper into the next. The
	// given base must be executed.
	Layer func(base http.RoundTripper) http.RoundTripper
)

// New creates a new Client with the defaults in Config. More ClientOption can be
// provided to adjust the default config.
func New(opts ...ClientOption) *Client {
	client := newDefaultClient()
	client.applyOptions(opts)
	return client
}

var memPool = sync.Pool{
	New: func() any {
		s := make([]byte, 0, 1024)
		return &s
	},
}

// DoReq wraps the standard implementation of Do(). The response body is read
// in full and will be closed. For non-closed http.Response see Do or Stream.
// Both the http.Response and the read in body serve as input for the given
// RespOption.
func (c *Client) DoReq(req *http.Request, opts ...RespOption) (*http.Response, error) {
	resp, err := c.Do(req)
	if err != nil || resp == nil {
		return resp, err
	}

	var body []byte
	if c.cfg.MemoryPooling {
		ptr := memPool.Get().(*[]byte)
		*ptr, err = readRespBody(resp, *ptr)
		body = *ptr
		defer func() {
			*ptr = (*ptr)[:0]
			memPool.Put(ptr)
		}()
	} else {
		body, err = io.ReadAll(resp.Body)
		defer setResponseBody(resp, body)
	}
	_ = resp.Body.Close()
	if err != nil {
		return resp, err
	}

	var errs []error
	for _, opt := range c.cfg.respOptions {
		err = opt(resp, body)
		if err != nil {
			errs = append(errs, err)
		}
	}

	for _, opt := range opts {
		err = opt(resp, body)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return resp, errors.Join(errs...)
}

// JSON is a wrapper for DoReq in combination with the WithJSON option.
func (c *Client) JSON(req *http.Request, obj any, opts ...RespOption) (*http.Response, error) {
	return c.DoReq(req, append([]RespOption{WithJSON(obj)}, opts...)...)
}

// Stream wraps a Do call and copies the http.Response body to the given
// io.Writer.
func (c *Client) Stream(req *http.Request, w io.Writer) (int64, error) {
	resp, err := c.Do(req)
	if err != nil || resp == nil {
		return 0, err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	r := bufio.NewReader(resp.Body)
	return io.Copy(w, r)
}

// Unwrap returns the underlying http.Client to use with http.RoundTripper
// applied
func (c *Client) Unwrap() *http.Client {
	return c.Client
}

// AddOptions add more options to the current Client.
func (c *Client) AddOptions(opts ...ClientOption) {
	c.applyOptions(opts)
}

// Extend creates a new Client based on the Config of the current with the
// optional given ClientOption.
func (c *Client) Extend(opts ...ClientOption) *Client {
	client := &Client{
		cfg: c.cfg,
	}
	client.applyOptions(opts)
	return client
}

// Close will close all shutdown hooks attached to Config.Shutdowns and
// return the combined error. The Client should not be used after calling
// this function.
func (c *Client) Close() error {
	var errs []error
	for _, fn := range c.cfg.Shutdowns {
		if err := fn(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (c *Client) applyOptions(opts []ClientOption) {
	for _, opt := range opts {
		opt(&c.cfg)
	}

	rt := c.getTransport()
	for _, l := range c.cfg.layers {
		rt = l(rt)
	}

	c.Client = &http.Client{
		Transport:     rt,
		CheckRedirect: c.cfg.CheckRedirect,
		Jar:           c.cfg.Jar,
		Timeout:       c.cfg.Timeout,
	}
}

func (c *Client) getTransport() http.RoundTripper {
	if c.cfg.H3Transport == nil {
		return c.cfg.Transport
	}

	return &transport{
		Transport:   c.cfg.Transport,
		h3Transport: c.cfg.H3Transport,
		h3Support:   make(map[string]h3Status),
	}
}

func newDefaultClient() *Client {
	return &Client{
		cfg: Config{
			Timeout:   DefaultTimeout,
			Transport: DefaultTransport,
		},
	}
}

func setResponseBody(resp *http.Response, body []byte) {
	resp.Body = io.NopCloser(bytes.NewReader(body))
}

func readRespBody(resp *http.Response, b []byte) ([]byte, error) {
	if resp.ContentLength > 0 && resp.ContentLength > int64(cap(b)) {
		b = make([]byte, 0, resp.ContentLength)
	}

	for {
		n, err := resp.Body.Read(b[len(b):cap(b)])
		b = b[:len(b)+n]
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return b, err
		}

		if len(b) == cap(b) {
			b = append(b, 0)[:len(b)]
		}
	}
}
