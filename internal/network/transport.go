package network

import (
	"net/http"
	"time"
)

// Transport is the engine behind the applications shadow clients and proxy.
type Transport struct {
	// transport provides functionality for the struct.
	Transport *http.Transport
}

// NewTransport returns a new Transport object for use.
func NewTransport() Transport {
	transport := http.Transport{
		MaxIdleConns:          512, // total idle conns across hosts
		MaxIdleConnsPerHost:   128, // per-host idle
		MaxConnsPerHost:       0,   // no hard cap
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second, // bounds a slow backend that never sends headers
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return Transport{Transport: &transport}
}
