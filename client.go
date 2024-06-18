package httpc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"
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
	Config struct {
		Transport     *http.Transport
		CheckRedirect func(req *http.Request, via []*http.Request) error
		Jar           http.CookieJar
		Timeout       time.Duration
		JsonUnmarshal JsonUnmarshaler

		layers       []Layer
		errorHandler ErrorHandler
	}

	JsonUnmarshaler func(data []byte, obj any) error

	Layer func(base http.RoundTripper) http.RoundTripper

	ErrorHandler func(c *Client, resp *http.Response, body []byte) error

	Option func(cfg *Config)

	// RespOption is an option to handle a successful http.Response pointer.
	// Aborts if the first option returns an error. The response's body is
	// already read and closed. The read data is passed as parameter.
	RespOption func(c *Client, resp *http.Response, body []byte) error
)

func New(opts ...Option) *Client {
	client := newDefaultClient()
	client.applyOptions(opts)
	return client
}

type Client struct {
	*http.Client

	cfg Config
}

func (c *Client) DoReq(req *http.Request, opts ...RespOption) (*http.Response, error) {
	resp, err := c.Do(req)
	if err != nil {
		return resp, err
	}

	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return resp, err
	}
	defer setResponseBody(resp, body)

	if resp.StatusCode >= 400 {
		return resp, c.cfg.errorHandler(c, resp, body)
	}

	for _, opt := range opts {
		err = opt(c, resp, body)
		if err != nil {
			return resp, err
		}
	}

	return resp, nil
}

func (c *Client) JSON(req *http.Request, obj any, opts ...RespOption) (*http.Response, error) {
	return c.DoReq(req, append([]RespOption{WithJSON(obj)}, opts...)...)
}

func (c *Client) Stream(req *http.Request, w io.Writer) (int64, error) {
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	r := bufio.NewReader(resp.Body)
	return io.Copy(w, r)
}

func (c *Client) Unwrap() *http.Client {
	return c.Client
}

func (c *Client) AddOptions(opts ...Option) {
	c.applyOptions(opts)
}

func (c *Client) Extend(opts ...Option) *Client {
	client := &Client{
		cfg: c.cfg,
	}
	client.applyOptions(opts)
	return client
}

func (c *Client) applyOptions(opts []Option) {
	for _, opt := range opts {
		opt(&c.cfg)
	}

	var rt http.RoundTripper = c.cfg.Transport
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

func newDefaultClient() *Client {
	return &Client{
		cfg: Config{
			Timeout:       DefaultTimeout,
			Transport:     DefaultTransport,
			JsonUnmarshal: json.Unmarshal,
			errorHandler:  bytesErrorHandler,
		},
	}
}

func setResponseBody(resp *http.Response, body []byte) {
	resp.Body = io.NopCloser(bytes.NewReader(body))
}
