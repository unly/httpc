package httpc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
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
			time.Sleep(10 * time.Millisecond)
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
		defer resp.Body.Close()

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

		resp, err := client.Get(s.URL)
		defer resp.Body.Close()

		assert.NoError(t, err)
	})
}

func TestWithH3Transport(t *testing.T) {
	tlsConfig, err := generateTLSConfig(t)
	require.NoError(t, err)

	t.Run("unknown h3 status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		defer srv.Close()

		client := New(WithH3Transport(&http3.Transport{}))
		defer client.Close()
		req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
		require.NoError(t, err)

		resp, err := client.DoReq(req)

		assert.NoError(t, err)
		assert.NotEqual(t, 3, resp.ProtoMajor)
	})

	t.Run("alt-svc available", func(t *testing.T) {
		addr, cleanup := startH3Server(t, tlsConfig)
		defer cleanup()

		port, _ := strconv.Atoi(strings.Split(addr, ":")[1])
		_, cleanup2 := startH12Server(t, tlsConfig, port)
		defer cleanup2()

		tr := &http3.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}

		client := New(WithH3Transport(tr), WithTransport(&http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}))
		defer client.Close()
		req, err := http.NewRequest(http.MethodGet, "https://"+addr, nil)
		require.NoError(t, err)

		resp, err := client.DoReq(req)

		assert.NoError(t, err)
		assert.NotEqual(t, 3, resp.ProtoMajor)

		req, err = http.NewRequest(http.MethodGet, "https://"+addr, nil)
		require.NoError(t, err)

		resp, err = client.DoReq(req)

		assert.NoError(t, err)
		assert.Equal(t, 3, resp.ProtoMajor)
	})

	t.Run("alt-svc unavailable", func(t *testing.T) {
		addr, cleanup := startH12Server(t, tlsConfig, 0)
		defer cleanup()

		tr := &http3.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			QUICConfig: &quic.Config{
				HandshakeIdleTimeout: 500 * time.Millisecond,
			},
		}

		client := New(WithH3Transport(tr), WithTransport(&http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}))
		defer client.Close()
		req, err := http.NewRequest(http.MethodGet, "https://"+addr, nil)
		require.NoError(t, err)

		resp, err := client.DoReq(req)

		assert.NoError(t, err)
		assert.NotEqual(t, 3, resp.ProtoMajor)

		req, err = http.NewRequest(http.MethodGet, "https://"+addr, nil)
		require.NoError(t, err)

		resp, err = client.DoReq(req)

		assert.NoError(t, err)
		assert.NotEqual(t, 3, resp.ProtoMajor)

		req, err = http.NewRequest(http.MethodGet, "https://"+addr, nil)
		require.NoError(t, err)

		resp, err = client.DoReq(req)

		assert.NoError(t, err)
		assert.NotEqual(t, 3, resp.ProtoMajor)
	})

	t.Run("h3 support", func(t *testing.T) {
		addr, cleanup := startH3Server(t, tlsConfig)
		defer cleanup()

		tr := &http3.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}

		client := New(WithH3Transport(tr))
		defer client.Close()
		internalTr, ok := client.Transport.(*transport)
		require.True(t, ok)
		internalTr.h3Support[addr] = h3StatusSupported
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://%s", addr), nil)
		require.NoError(t, err)

		resp, err := client.DoReq(req)

		assert.NoError(t, err)
		assert.Equal(t, 3, resp.ProtoMajor)
	})
}

func startH12Server(t *testing.T, tlsConfig *tls.Config, port int) (string, func()) {
	t.Helper()

	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	require.NoError(t, err)
	portUsed := strings.Split(listener.Addr().String(), ":")[1]

	srv := &http.Server{
		Addr:      ":" + portUsed,
		TLSConfig: tlsConfig,
		Handler: http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.Header().Add("Alt-Svc", fmt.Sprintf(`h3=":%s"; ma=2592000`, portUsed))
		}),
	}
	go func() {
		_ = srv.ServeTLS(listener, "", "")
	}()

	return srv.Addr, func() {
		require.NoError(t, srv.Close())
	}
}

func startH3Server(t *testing.T, cfg *tls.Config) (string, func()) {
	t.Helper()

	listener, err := quic.ListenAddr("localhost:0", cfg, nil)
	require.NoError(t, err)

	h3Srv := &http3.Server{
		TLSConfig: http3.ConfigureTLSConfig(cfg),
		Handler:   http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}),
	}
	go func() {
		_ = h3Srv.ServeListener(listener)
	}()

	return listener.Addr().String(), func() {
		require.NoError(t, h3Srv.Close())
		require.NoError(t, listener.Close())
	}
}

func generateTLSConfig(t *testing.T) (*tls.Config, error) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{http3.NextProtoH3},
	}, nil
}
