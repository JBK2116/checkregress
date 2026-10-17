package network

import (
	"net"
	"net/http"
	"time"
)

const (
	// dialTimeout is the maximum amount of time to wait for a TCP connect.
	dialTimeout = 3 * time.Second
	// dialKeepAlive is the TCP keep-alive interval for open connections.
	dialKeepAlive = 30 * time.Second
	// maxIdleConns is the maximum number of idle connections across all backends.
	maxIdleConns = 1024
	// maxIdleConnsPerHost is the maximum number of idle connections to a single backend.
	maxIdleConnsPerHost = 256
	// maxConnsPerHost is the maximum number of connections to a single backend (0 = unlimited).
	maxConnsPerHost = 0
	// idleConnTimeout is the maximum amount of time a connection may be idle before being closed.
	idleConnTimeout = 90 * time.Second
	// tlsHandshakeTimeout is the maximum amount of time to wait for a TLS handshake.
	tlsHandshakeTimeout = 10 * time.Second
	// expectContinueTimeout is the maximum amount of time to wait for a 100-continue response.
	expectContinueTimeout = 1 * time.Second

	// proxyResponseHeaderTimeout caps how long the proxy waits on a backend for response headers.
	proxyResponseHeaderTimeout = 60 * time.Second
	// clientResponseHeaderTimeout is a backstop for the outbound client. Keep it at or below
	// the client's max context timeout, otherwise it will never fire.
	clientResponseHeaderTimeout = 30 * time.Second
)

// newTransport builds a transport with the shared settings.
func newTransport(responseHeaderTimeout time.Duration, disableCompression bool) *http.Transport {
	dialer := &net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: dialKeepAlive,
	}

	return &http.Transport{
		DialContext:           dialer.DialContext,
		MaxIdleConns:          maxIdleConns,
		MaxIdleConnsPerHost:   maxIdleConnsPerHost,
		MaxConnsPerHost:       maxConnsPerHost,
		IdleConnTimeout:       idleConnTimeout,
		TLSHandshakeTimeout:   tlsHandshakeTimeout,
		ResponseHeaderTimeout: responseHeaderTimeout,
		ExpectContinueTimeout: expectContinueTimeout,
		DisableCompression:    disableCompression,
		// ForceAttemptHTTP2 enables HTTP/2 negotiation (ALPN) despite the custom dialer.
		// It falls back to HTTP/1.1 if the backend doesn't support h2.
		ForceAttemptHTTP2: true,
	}
}

// newProxyTransport returns a transport for the reverse proxy.
//
// Compression is disabled so responses are forwarded exactly as the backend sent them.
// It is isolated from the client transport so client load can never starve the proxy.
func newProxyTransport() *http.Transport {
	return newTransport(proxyResponseHeaderTimeout, true)
}

// newShadowTransport returns a transport for outbound shadow client requests.
//
// Callers must still apply a per-request context deadline, which also covers body reads.
func newShadowTransport() *http.Transport {
	return newTransport(clientResponseHeaderTimeout, false)
}
