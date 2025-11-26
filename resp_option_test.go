package httpc

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testStruct struct {
	Name string `json:"name"`
}

func TestWithJSON(t *testing.T) {
	t.Run("unmarshal into struct", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			_, err := rw.Write([]byte(`{"name":"test"}`))
			require.NoError(t, err)
		}))
		defer s.Close()
		client := New()
		req, err := http.NewRequest(http.MethodGet, s.URL, nil)
		require.NoError(t, err)
		var res testStruct

		resp, err := client.DoReq(req, WithJSON(&res))

		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "test", res.Name)
	})

	t.Run("invalid json for struct", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			_, err := rw.Write([]byte(`{"name":42}`))
			require.NoError(t, err)
		}))
		defer s.Close()
		client := New()
		req, err := http.NewRequest(http.MethodGet, s.URL, nil)
		require.NoError(t, err)
		var res testStruct

		resp, err := client.DoReq(req, WithJSON(&res))

		assert.Error(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("server error", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.WriteHeader(http.StatusInternalServerError)
		}))
		defer s.Close()
		client := New()
		req, err := http.NewRequest(http.MethodGet, s.URL, nil)
		require.NoError(t, err)
		var res testStruct

		resp, err := client.DoReq(req, WithJSON(&res))

		assert.NoError(t, err)
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.Equal(t, testStruct{}, res)
	})
}

func TestWithCopy(t *testing.T) {
	t.Run("copy to writer", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			_, err := rw.Write([]byte("hello world"))
			require.NoError(t, err)
		}))
		defer s.Close()
		client := New()
		req, err := http.NewRequest(http.MethodGet, s.URL, nil)
		require.NoError(t, err)
		buf := &bytes.Buffer{}

		resp, err := client.DoReq(req, WithCopy(buf))

		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "hello world", buf.String())
	})

	t.Run("no content", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.WriteHeader(http.StatusNoContent)
		}))
		defer s.Close()
		client := New()
		req, err := http.NewRequest(http.MethodGet, s.URL, nil)
		require.NoError(t, err)
		buf := &bytes.Buffer{}

		resp, err := client.DoReq(req, WithCopy(buf))

		assert.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
		assert.Equal(t, "", buf.String())
	})
}

func TestWithStatusCode(t *testing.T) {
	tests := []struct {
		sent     int
		expected int
	}{
		{
			sent:     200,
			expected: 200,
		},
		{
			sent:     204,
			expected: 200,
		},
		{
			sent:     200,
			expected: 204,
		},
	}

	for _, tt := range tests {
		scenario := fmt.Sprintf("sent: %d, expect: %d", tt.sent, tt.expected)
		t.Run(scenario, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
				rw.WriteHeader(tt.sent)
			}))
			defer s.Close()
			client := New()
			req, err := http.NewRequest(http.MethodGet, s.URL, nil)
			require.NoError(t, err)

			resp, err := client.DoReq(req, WithStatusCode(tt.expected))

			assert.Equal(t, tt.sent, resp.StatusCode)
			if tt.expected == tt.sent {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestWithStatusCodeRange(t *testing.T) {
	tests := []struct {
		sent        int
		low         int
		high        int
		errExpected bool
	}{
		{
			sent: 200,
			low:  200,
			high: 300,
		},
		{
			sent: 204,
			low:  200,
			high: 300,
		},
		{
			sent:        200,
			low:         200,
			high:        200,
			errExpected: true,
		},
	}

	for _, tt := range tests {
		scenario := fmt.Sprintf("sent: %d, range: [%d, %d)", tt.sent, tt.low, tt.high)
		t.Run(scenario, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
				rw.WriteHeader(tt.sent)
			}))
			defer s.Close()
			client := New()
			req, err := http.NewRequest(http.MethodGet, s.URL, nil)
			require.NoError(t, err)

			resp, err := client.DoReq(req, WithStatusCodeRange(tt.low, tt.high))

			assert.Equal(t, tt.sent, resp.StatusCode)
			if tt.errExpected {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

type customJSONError struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

func (c *customJSONError) Error() string {
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
		client := New(WithRespOption(WithCustomJSONError[*customJSONError]()))
		req, err := http.NewRequest(http.MethodGet, s.URL, nil)
		require.NoError(t, err)

		_, err = client.DoReq(req)

		assert.Error(t, err)
		got := &customJSONError{}
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
		client := New(WithRespOption(WithCustomJSONError[*customJSONError]()))
		req, err := http.NewRequest(http.MethodGet, s.URL, nil)
		require.NoError(t, err)

		_, err = client.DoReq(req)

		assert.Error(t, err)
		got := &customJSONError{}
		assert.False(t, errors.As(err, &got))
	})

	t.Run("empty json error", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.WriteHeader(http.StatusBadRequest)
		}))
		defer s.Close()
		client := New(WithRespOption(WithCustomJSONError[*customJSONError]()))
		req, err := http.NewRequest(http.MethodGet, s.URL, nil)
		require.NoError(t, err)

		_, err = client.DoReq(req)

		assert.Error(t, err)
	})

	t.Run("200 response code", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			_, _ = rw.Write([]byte("hello world"))
		}))
		defer s.Close()
		client := New()
		req, err := http.NewRequest(http.MethodGet, s.URL, nil)
		require.NoError(t, err)

		_, err = client.DoReq(req, WithCustomJSONError[*customJSONError]())

		assert.NoError(t, err)
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
		client := New(WithRespOption(WithJSONError()))
		req, _ := http.NewRequest(http.MethodGet, s.URL, nil)

		_, err := client.DoReq(req)

		assert.Error(t, err)
		var got JSONBodyError
		assert.ErrorAs(t, err, &got)
		assert.Equal(t, "world", got["hello"])
	})

	t.Run("empty json error", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.WriteHeader(http.StatusBadRequest)
		}))
		defer s.Close()
		client := New(WithRespOption(WithJSONError()))
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
		client := New(WithRespOption(WithBytesError()))
		req, err := http.NewRequest(http.MethodGet, s.URL, nil)
		require.NoError(t, err)

		_, err = client.DoReq(req)

		assert.Error(t, err)
		var got BytesBodyError
		assert.ErrorAs(t, err, &got)
		assert.Equal(t, []byte("hello world"), []byte(got))
	})

	t.Run("empty json error", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.WriteHeader(http.StatusBadRequest)
		}))
		defer s.Close()
		client := New(WithRespOption(WithBytesError()))
		req, err := http.NewRequest(http.MethodGet, s.URL, nil)
		require.NoError(t, err)

		_, err = client.DoReq(req)

		assert.Error(t, err)
		var got BytesBodyError
		assert.ErrorAs(t, err, &got)
	})

	t.Run("200 response code", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			_, _ = rw.Write([]byte("hello world"))
		}))
		defer s.Close()
		client := New(WithRespOption(WithBytesError()))
		req, err := http.NewRequest(http.MethodGet, s.URL, nil)
		require.NoError(t, err)

		_, err = client.DoReq(req)

		assert.NoError(t, err)
	})
}
