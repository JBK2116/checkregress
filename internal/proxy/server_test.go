package proxy

import (
	"net"
	"net/http"
	"testing"
	"time"
)

func TestNewServerConfiguresTimeouts(t *testing.T) {
	t.Parallel()

	h := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	s := NewServer("127.0.0.1:9000", h, 5*time.Second, 60*time.Second)

	if s.srv.Addr != "127.0.0.1:9000" {
		t.Errorf("Addr = %q, want %q", s.srv.Addr, "127.0.0.1:9000")
	}
	if s.srv.Handler == nil {
		t.Error("Handler = nil, want the provided handler")
	}
	if s.srv.ReadHeaderTimeout != 5*time.Second {
		t.Errorf("ReadHeaderTimeout = %v, want %v", s.srv.ReadHeaderTimeout, 5*time.Second)
	}
	if s.srv.IdleTimeout != 60*time.Second {
		t.Errorf("IdleTimeout = %v, want %v", s.srv.IdleTimeout, 60*time.Second)
	}
	// ReadTimeout and WriteTimeout must remain unbounded so that slow or
	// streaming requests to the proxied application are never truncated.
	if s.srv.ReadTimeout != 0 {
		t.Errorf("ReadTimeout = %v, want 0 (must not truncate proxied app uploads)", s.srv.ReadTimeout)
	}
	if s.srv.WriteTimeout != 0 {
		t.Errorf("WriteTimeout = %v, want 0 (must not truncate proxied app responses)", s.srv.WriteTimeout)
	}
}

func TestServerListenAndServe(t *testing.T) {
	t.Parallel()

	// Reserve a free ephemeral port, then release it so ListenAndServe can bind.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving ephemeral port: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	s := NewServer(addr, h, time.Second, time.Second)

	errc := make(chan error, 1)
	go func() { errc <- s.ListenAndServe() }()

	// Poll until the server accepts a connection, or fail after a deadline.
	var resp *http.Response
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err = http.Get("http://" + addr)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if resp == nil {
		t.Fatalf("server did not start: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	if err := s.srv.Close(); err != nil {
		t.Errorf("closing server: %v", err)
	}
	if err := <-errc; err != http.ErrServerClosed {
		t.Errorf("ListenAndServe = %v, want %v", err, http.ErrServerClosed)
	}
}
