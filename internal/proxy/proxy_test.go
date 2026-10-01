package proxy

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/JBK2116/checkregress/internal/models"
)

// discardLogger returns a logger that discards all output, for tests that do
// not assert on logging.
func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// mustParseURL parses s as a URL and fails the test on error.
func mustParseURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("parsing URL %q: %v", s, err)
	}
	return u
}

// closedServerURL starts a server, records its URL, and immediately closes it,
// returning an address that is guaranteed to refuse connections.
func closedServerURL(t *testing.T) *url.URL {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	u := mustParseURL(t, srv.URL)
	srv.Close()
	return u
}

// backendRequest captures the fields of a backend request that the proxy tests
// assert on.
type backendRequest struct {
	host           string
	requestURI     string
	forwardedFor   string
	forwardedHost  string
	forwardedProto string
}

func TestReverseProxyForwardsToLegacy(t *testing.T) {
	t.Parallel()

	got := make(chan backendRequest, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- backendRequest{
			host:           r.Host,
			requestURI:     r.URL.RequestURI(),
			forwardedFor:   r.Header.Get("X-Forwarded-For"),
			forwardedHost:  r.Header.Get("X-Forwarded-Host"),
			forwardedProto: r.Header.Get("X-Forwarded-Proto"),
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	legacy := mustParseURL(t, backend.URL)
	rp := newReverseProxy(models.Route{ServerName: "svc", Legacy: legacy}, discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/orders?page=2", nil)
	req.Host = "client.example.com"
	req.RemoteAddr = "192.0.2.1:4242"
	// A client-supplied X-Forwarded-For must be ignored, not trusted.
	req.Header.Set("X-Forwarded-For", "6.6.6.6")

	rec := httptest.NewRecorder()
	rp.Proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	info := <-got
	if info.host != legacy.Host {
		t.Errorf("backend Host = %q, want %q", info.host, legacy.Host)
	}
	if info.requestURI != "/api/orders?page=2" {
		t.Errorf("backend RequestURI = %q, want %q", info.requestURI, "/api/orders?page=2")
	}
	if info.forwardedFor != "192.0.2.1" {
		t.Errorf("X-Forwarded-For = %q, want %q (client spoof must be ignored)", info.forwardedFor, "192.0.2.1")
	}
	if info.forwardedHost != "client.example.com" {
		t.Errorf("X-Forwarded-Host = %q, want %q", info.forwardedHost, "client.example.com")
	}
	if info.forwardedProto != "http" {
		t.Errorf("X-Forwarded-Proto = %q, want %q", info.forwardedProto, "http")
	}
}

func TestReverseProxyErrorReturnsBadGateway(t *testing.T) {
	t.Parallel()

	rp := newReverseProxy(models.Route{ServerName: "svc", Legacy: closedServerURL(t)}, discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Host = "client.example.com"
	rec := httptest.NewRecorder()
	rp.Proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestReverseProxyErrorLogs(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	rp := newReverseProxy(models.Route{ServerName: "svc", Legacy: closedServerURL(t)}, logger)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Host = "client.example.com"
	rec := httptest.NewRecorder()
	rp.Proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}

	output := buf.String()
	for _, want := range []string{"proxy error", "method=GET", "path=/health", "err="} {
		if !strings.Contains(output, want) {
			t.Errorf("log output %q does not contain %q", output, want)
		}
	}
}
