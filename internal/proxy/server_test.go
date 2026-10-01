package proxy

import (
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
