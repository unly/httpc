package httpc

import (
	"fmt"
	"io"
	"net/http"
)

// WithJSON unmarshalls the body of the http.Response into the given
// object using the Config.JsonUnmarshal function.
func WithJSON(obj any) RespOption {
	return func(c *Client, _ *http.Response, body []byte) error {
		return c.cfg.JsonUnmarshal(body, obj)
	}
}

// WithCopy copies the body of the http.Response to the given io.Writer.
func WithCopy(w io.Writer) RespOption {
	return func(_ *Client, _ *http.Response, body []byte) error {
		_, err := w.Write(body)
		return err
	}
}

// WithStatusCode checks if the http.Response matches the given HTTP
// status code. Returns an error if the status code does not match.
func WithStatusCode(code int) RespOption {
	return func(_ *Client, resp *http.Response, _ []byte) error {
		if resp.StatusCode == code {
			return nil
		}

		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}
