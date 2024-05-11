package httpc

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_DoReq(t *testing.T) {
	t.Run("no options", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			_, _ = rw.Write([]byte("hello world"))
		}))
		defer s.Close()
		client := New()
		req, err := http.NewRequest(http.MethodGet, s.URL, nil)
		require.NoError(t, err)

		resp, err := client.DoReq(req)

		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		data, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)
		assert.Equal(t, "hello world", string(data))
	})

	t.Run("returns error", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.WriteHeader(http.StatusInternalServerError)
			_, _ = rw.Write([]byte("hello world"))
		}))
		defer s.Close()
		client := New()
		req, err := http.NewRequest(http.MethodGet, s.URL, nil)
		require.NoError(t, err)

		resp, err := client.DoReq(req)

		assert.Error(t, err)
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		data, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)
		assert.Equal(t, "hello world", string(data))
	})
}

func TestClient_JSON(t *testing.T) {
	t.Run("no options", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			_, _ = rw.Write([]byte(`{"name":"John"}`))
		}))
		defer s.Close()
		client := New()
		req, err := http.NewRequest(http.MethodGet, s.URL, nil)
		require.NoError(t, err)

		var res TestStruct
		_, err = client.JSON(req, &res)

		assert.NoError(t, err)
		assert.Equal(t, "John", res.Name)
	})
}

func TestClient_Unwrap(t *testing.T) {
	t.Run("stdlib client", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			assert.Equal(t, "bar", req.Header.Get("foo"))
		}))
		defer s.Close()
		headers := http.Header{}
		headers.Set("foo", "bar")
		client := New(WithHeaders(headers)).Unwrap()

		resp, err := client.Get(s.URL)

		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}
