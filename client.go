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
	// Config struct holding all the configurations of an HTTP Client.
	// Can be modified via the With* options.
	Config struct {
		// Transport a pointer to the underlying http.Transport. This is used as
		// the base for the http.Client and the http.RoundTripper. Defaults to
		// DefaultTransport. Can
		Transport *http.Transport
		// CheckRedirect redirecting logic as defined in the http.Client to
		// determine redirects. Defaults to nil. Can be set via WithCheckRedirect.
		CheckRedirect func(req *http.Request, via []*http.Request) error
		// Jar the cookie storage logic of the http.CookieJar to be used by the
		// http.Client. Defaults to nil. Can be set via WithCookieJar.
		Jar http.CookieJar
		// Timeout for each outgoing HTTP request. A value of 0 means no timeout.
		// Defaults to DefaultTimeout. Can be set via WithTimeout.
		Timeout time.Duration
		// JsonUnmarshal unmarshal function to decode JSON payload into an object.
		// Defaults to json.Unmarshal.
		JsonUnmarshal JsonUnmarshaler

		layers       []Layer
		errorHandler ErrorHandler
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

	// JsonUnmarshaler decode the given JSON data into the given object.
	JsonUnmarshaler func(data []byte, obj any) error

	// Layer is a function wo wrap one http.RoundTripper into the next. The
	// given base must be executed.
	Layer func(base http.RoundTripper) http.RoundTripper

	// ErrorHandler this function will be called when the client was able to
	// successfully perform an HTTP call, yet the response call was not within
	// the 200 range. The returned error will be returned to the caller.
	ErrorHandler func(c *Client, resp *http.Response, body []byte) error

	// Option function to modify the Config when creating or updating a client.
	Option func(cfg *Config)

	// RespOption is an option to handle a successful http.Response pointer.
	// Aborts if the first option returns an error. The response's body is
	// already read and closed. The read data is passed as parameter.
	RespOption func(c *Client, resp *http.Response, body []byte) error
)

// New creates a new Client with the defaults in Config. More Option can be
// provided to adjust the default config.
func New(opts ...Option) *Client {
	client := newDefaultClient()
	client.applyOptions(opts)
	return client
}

// DoReq wraps the standard implementation of Do(). The response body is read
// in full and will be closed. For non-closed http.Response see Do or Stream.
// Both the http.Response and the read in body serve as input for the given
// RespOption. All non 2xx responses will be sent to the error handler of the
// Client.
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

// JSON is a wrapper for DoReq in combination with the WithJSON option.
func (c *Client) JSON(req *http.Request, obj any, opts ...RespOption) (*http.Response, error) {
	return c.DoReq(req, append([]RespOption{WithJSON(obj)}, opts...)...)
}

// Stream wraps a Do call and copies the http.Response body to the given
// io.Writer.
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

// Unwrap returns the underlying http.Client to use with http.RoundTripper
// applied
func (c *Client) Unwrap() *http.Client {
	return c.Client
}

// AddOptions add more options to the current Client.
func (c *Client) AddOptions(opts ...Option) {
	c.applyOptions(opts)
}

// Extend creates a new Client based on the Config of the current with the
// optional given Option.
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
