package httpc

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestStruct struct {
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
		var res TestStruct

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
		var res TestStruct

		resp, err := client.DoReq(req, WithJSON(&res))

		assert.Error(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
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
