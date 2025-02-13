package httpc

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
)

// RespOption is an option to handle a successful http.Response pointer.
// Aborts if the first option returns an error. The response's body is
// already read and closed. The read data is passed as parameter. The
// body parameter should not be stored, but rather copied if needed. It
// must not be stored in combination with the WithMemoryPooling Option.
type RespOption func(resp *http.Response, body []byte) error

// WithJSON unmarshalls the body of a successful http.Response into the
// given object using the json.Unmarshal function.
func WithJSON(obj any) RespOption {
	return func(resp *http.Response, body []byte) error {
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return json.Unmarshal(body, obj)
		}

		return nil
	}
}

// WithCopy copies the body of the http.Response to the given io.Writer.
func WithCopy(w io.Writer) RespOption {
	return func(_ *http.Response, body []byte) error {
		_, err := w.Write(body)
		return err
	}
}

// WithStatusCode checks if the http.Response matches the given HTTP
// status code. Returns an error if the status code does not match.
func WithStatusCode(code int) RespOption {
	return WithStatusCodeRange(code, code+1)
}

// WithNon2xxError is WithStatusCodeRange with the 2xx HTTP status code
// range.zxl
func WithNon2xxError() RespOption {
	return WithStatusCodeRange(200, 300)
}

// WithStatusCodeRange checks if the HTTP response code is within the
// lower (inclusive) and the upper (exclusive). Returns an error if not.
func WithStatusCodeRange(lower, upper int) RespOption {
	return func(resp *http.Response, _ []byte) error {
		if resp.StatusCode >= lower && resp.StatusCode < upper {
			return nil
		}

		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}

// WithCustomJSONError returns an error of the given type E. Must
// implement the error interface. Will use the json.Unmarshal to read
// in the HTTP response's body.
func WithCustomJSONError[E error]() RespOption {
	return func(resp *http.Response, body []byte) error {
		if resp.StatusCode < 400 {
			return nil
		}

		var err E
		parseErr := json.Unmarshal(body, &err)
		if parseErr != nil {
			return parseErr
		}

		return err
	}
}

// WithJSONError wraps WithCustomJSONError with the JSONBodyError type.
// Allows generic JSON data as error struct.
func WithJSONError() RespOption {
	return WithCustomJSONError[JSONBodyError]()
}

// WithBytesError returns a BytesBodyError if the HTTP call failed.
// Represents a raw []byte body.
func WithBytesError() RespOption {
	return func(resp *http.Response, body []byte) error {
		if resp.StatusCode < 400 {
			return nil
		}

		return BytesBodyError(slices.Clone(body))
	}
}
