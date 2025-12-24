package httpc

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

type transport struct {
	*http.Transport

	h3Transport *http3.Transport
	h3Support   map[string]h3Status
}

type h3Status uint8

const (
	h3StatusSupported h3Status = iota + 1
	h3StatusAltSvc
	h3StatusNotSupported
)

var _ http.RoundTripper = (*transport)(nil)

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	switch t.h3Support[req.URL.Host] {
	case h3StatusSupported:
		return t.h3Transport.RoundTrip(req)
	case h3StatusNotSupported:
		return t.Transport.RoundTrip(req)
	case h3StatusAltSvc:
		return t.tryHttp3(req)
	default:
		return t.defaultCall(req)
	}
}

func (t *transport) defaultCall(req *http.Request) (*http.Response, error) {
	resp, err := t.Transport.RoundTrip(req)
	if err != nil || resp == nil {
		return resp, err
	}

	if strings.Contains(resp.Header.Get("Alt-Svc"), "h3") {
		t.h3Support[req.URL.Host] = h3StatusAltSvc
	}

	return resp, nil
}

func (t *transport) tryHttp3(req *http.Request) (*http.Response, error) {
	clonedReq := req.Clone(req.Context())
	resp, err := t.h3Transport.RoundTrip(req)
	if err == nil {
		t.h3Support[req.URL.Host] = h3StatusSupported
		return resp, nil
	}

	var idleErr *quic.IdleTimeoutError
	if errors.As(err, &idleErr) {
		t.h3Support[req.URL.Host] = h3StatusNotSupported
	}
	var netError net.Error
	if errors.As(err, &netError) && netError.Timeout() {
		t.h3Support[req.URL.Host] = h3StatusNotSupported
	}

	return t.Transport.RoundTrip(clonedReq)
}
