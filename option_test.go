package httpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithCheckRedirect(t *testing.T) {
	t.Run("redirect", func(t *testing.T) {
		mux := http.NewServeMux()
		s := httptest.NewServer(mux)
		defer s.Close()
		mux.HandleFunc("/", func(rw http.ResponseWriter, _ *http.Request) {
			rw.Header().Set("Location", s.URL+"/redirect")
			rw.WriteHeader(http.StatusTemporaryRedirect)
		})
		redirect := func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
		client := New(WithCheckRedirect(redirect))

		resp, err := client.Get(s.URL)

		assert.NoError(t, err)
		assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		assert.Equal(t, s.URL+"/redirect", resp.Header.Get("Location"))
	})
}

type CookieStore struct {
	cookies []*http.Cookie
}

func (c *CookieStore) SetCookies(_ *url.URL, cookies []*http.Cookie) {
	c.cookies = append(c.cookies, cookies...)
}

func (c *CookieStore) Cookies(_ *url.URL) []*http.Cookie {
	return c.cookies
}

func TestWithCookieJar(t *testing.T) {
	t.Run("cookie jar", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.Header().Set("Set-Cookie", "foo=bar")
		}))
		defer s.Close()
		jar := &CookieStore{}
		client := New(WithCookieJar(jar))

		_, err := client.Get(s.URL)

		assert.NoError(t, err)
		assert.Len(t, jar.cookies, 1)
		assert.Equal(t, "foo", jar.cookies[0].Name)
		assert.Equal(t, "bar", jar.cookies[0].Value)
	})
}

func TestWithTimeout(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			time.Sleep(10 * time.Nanosecond)
		}))
		defer s.Close()
		client := New(WithTimeout(1))

		_, err := client.Get(s.URL)

		assert.Error(t, err)
	})
}

func TestWithTransport(t *testing.T) {
	t.Run("custom transport", func(t *testing.T) {
		transportCalled := false
		transport := &http.Transport{
			DialTLSContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				transportCalled = true
				return nil, assert.AnError
			},
		}
		client := New(WithTransport(transport))

		_, err := client.Get("https://example.com")

		assert.Error(t, err)
		assert.True(t, transportCalled)
	})
}

type LayerFn func(*http.Request) (*http.Response, error)

func (l LayerFn) RoundTrip(req *http.Request) (*http.Response, error) {
	return l(req)
}

func TestWithLayer(t *testing.T) {
	t.Run("with sample layer", func(t *testing.T) {
		emptyResponse := &http.Response{}
		l := func(base http.RoundTripper) http.RoundTripper {
			return LayerFn(func(req *http.Request) (*http.Response, error) {
				return emptyResponse, nil
			})
		}
		client := New(WithLayer(l))

		resp, err := client.Get("example.com")

		assert.NoError(t, err)
		assert.Equal(t, emptyResponse, resp)
	})
}

func TestWithHeaders(t *testing.T) {
	t.Run("default headers", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			assert.Equal(t, []string{"hello", "world"}, req.Header.Values("Key"))
		}))
		defer s.Close()
		client := New(WithHeaders(map[string][]string{
			"Key": {"hello", "world"},
		}))

		_, err := client.Get(s.URL)

		assert.NoError(t, err)
	})
}

type CustomJSONError struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

func (c *CustomJSONError) Error() string {
	return fmt.Sprintf("Custom JSON error: FirstName: %s, LastName: %s", c.FirstName, c.LastName)
}

func TestWithCustomJSONError(t *testing.T) {
	t.Run("json error", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.WriteHeader(http.StatusBadRequest)
			_, err := rw.Write([]byte(`{"firstName":"john","lastName":"doe"}`))
			require.NoError(t, err)
		}))
		defer s.Close()
		client := New(WithCustomJSONError[*CustomJSONError]())
		req, _ := http.NewRequest(http.MethodGet, s.URL, nil)

		_, err := client.DoReq(req)

		assert.Error(t, err)
		got := &CustomJSONError{}
		assert.ErrorAs(t, err, &got)
		assert.Equal(t, "john", got.FirstName)
	})

	t.Run("invalid json error", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.WriteHeader(http.StatusBadRequest)
			_, err := rw.Write([]byte(`{"firstName":42,"lastName":"doe"}`))
			require.NoError(t, err)
		}))
		defer s.Close()
		client := New(WithCustomJSONError[*CustomJSONError]())
		req, _ := http.NewRequest(http.MethodGet, s.URL, nil)

		_, err := client.DoReq(req)

		assert.Error(t, err)
		got := &CustomJSONError{}
		assert.False(t, errors.As(err, &got))
	})

	t.Run("empty json error", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.WriteHeader(http.StatusBadRequest)
		}))
		defer s.Close()
		client := New(WithCustomJSONError[*CustomJSONError]())
		req, _ := http.NewRequest(http.MethodGet, s.URL, nil)

		_, err := client.DoReq(req)

		assert.Error(t, err)
	})
}

func TestWithJSONError(t *testing.T) {
	t.Run("json error", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.WriteHeader(http.StatusBadRequest)
			_, err := rw.Write([]byte(`{"hello":"world"}`))
			require.NoError(t, err)
		}))
		defer s.Close()
		client := New(WithJSONError())
		req, _ := http.NewRequest(http.MethodGet, s.URL, nil)

		_, err := client.DoReq(req)

		assert.Error(t, err)
		var got JSONErrorBody
		assert.ErrorAs(t, err, &got)
		assert.Equal(t, "world", got["hello"])
	})

	t.Run("empty json error", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.WriteHeader(http.StatusBadRequest)
		}))
		defer s.Close()
		client := New(WithJSONError())
		req, _ := http.NewRequest(http.MethodGet, s.URL, nil)

		_, err := client.DoReq(req)

		assert.Error(t, err)
	})
}

func TestWithBytesError(t *testing.T) {
	t.Run("error message", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.WriteHeader(http.StatusBadRequest)
			_, err := rw.Write([]byte(`hello world`))
			require.NoError(t, err)
		}))
		defer s.Close()
		client := New(WithBytesError())
		req, _ := http.NewRequest(http.MethodGet, s.URL, nil)

		_, err := client.DoReq(req)

		assert.Error(t, err)
		var got BytesErrorBody
		assert.ErrorAs(t, err, &got)
		assert.Equal(t, []byte("hello world"), []byte(got))
	})

	t.Run("empty json error", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.WriteHeader(http.StatusBadRequest)
		}))
		defer s.Close()
		client := New(WithBytesError())
		req, _ := http.NewRequest(http.MethodGet, s.URL, nil)

		_, err := client.DoReq(req)

		assert.Error(t, err)
		var got BytesErrorBody
		assert.ErrorAs(t, err, &got)
	})
}
