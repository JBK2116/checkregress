package shadow

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
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

// newTestShadow returns a Shadow backed by the default client and a discarding
// logger.
func newTestShadow() *Shadow {
	return NewShadow(discardLogger(), http.DefaultClient)
}

func TestSetXForwarded(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		remoteAddr string
		host       string
		https      bool
		spoof      bool
		wantFor    string
		wantHost   string
		wantProto  string
	}{
		{
			name:       "http request",
			remoteAddr: "192.0.2.1:4242",
			host:       "client.example.com",
			wantFor:    "192.0.2.1",
			wantHost:   "client.example.com",
			wantProto:  "http",
		},
		{
			name:       "https request",
			remoteAddr: "192.0.2.2:4242",
			host:       "client.example.com",
			https:      true,
			wantFor:    "192.0.2.2",
			wantHost:   "client.example.com",
			wantProto:  "https",
		},
		{
			name:       "client supplied forwarding headers are overwritten",
			remoteAddr: "192.0.2.3:4242",
			host:       "client.example.com",
			spoof:      true,
			wantFor:    "192.0.2.3",
			wantHost:   "client.example.com",
			wantProto:  "http",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &Shadow{}
			in := httptest.NewRequest(http.MethodGet, "/", nil)
			in.RemoteAddr = tt.remoteAddr
			in.Host = tt.host
			if tt.https {
				in.TLS = &tls.ConnectionState{}
			}

			out := httptest.NewRequest(http.MethodGet, "http://backend/", nil)
			if tt.spoof {
				// Mimic client-supplied forwarding headers that must be
				// discarded rather than trusted.
				out.Header.Set("Forwarded", "for=6.6.6.6")
				out.Header.Set("X-Forwarded-For", "6.6.6.6")
				out.Header.Set("X-Forwarded-Host", "spoof.example.com")
				out.Header.Set("X-Forwarded-Proto", "https")
			}

			s.setXForwarded(in, out)

			if got := out.Header.Get("X-Forwarded-For"); got != tt.wantFor {
				t.Errorf("X-Forwarded-For = %q, want %q", got, tt.wantFor)
			}
			if got := out.Header.Get("X-Forwarded-Host"); got != tt.wantHost {
				t.Errorf("X-Forwarded-Host = %q, want %q", got, tt.wantHost)
			}
			if got := out.Header.Get("X-Forwarded-Proto"); got != tt.wantProto {
				t.Errorf("X-Forwarded-Proto = %q, want %q", got, tt.wantProto)
			}
			if got := out.Header.Get("Forwarded"); got != "" {
				t.Errorf("Forwarded = %q, want empty", got)
			}
		})
	}
}

func TestDelHeaders(t *testing.T) {
	t.Parallel()
	s := &Shadow{}
	r := httptest.NewRequest(http.MethodGet, "http://backend/", nil)

	// Hop-by-hop headers that must be stripped.
	r.Header.Set("Connection", "keep-alive, X-Custom")
	r.Header.Set("Proxy-Connection", "keep-alive")
	r.Header.Set("Keep-Alive", "timeout=5")
	r.Header.Set("Proxy-Authenticate", "Basic")
	r.Header.Set("Proxy-Authorization", "Basic dXNlcjpwYXNz")
	r.Header.Set("Te", "trailers")
	r.Header.Set("Trailer", "X-Checksum")
	r.Header.Set("Transfer-Encoding", "chunked")
	r.Header.Set("Upgrade", "websocket")
	// X-Custom is named by Connection, so it must be stripped too.
	r.Header.Set("X-Custom", "secret")
	// Ordinary headers that must survive.
	r.Header.Set("Authorization", "Bearer abc")
	r.Header.Set("Content-Type", "application/json")

	s.delHeaders(r)

	stripped := []string{
		"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate",
		"Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade",
		"X-Custom",
	}
	for _, h := range stripped {
		if got := r.Header.Get(h); got != "" {
			t.Errorf("header %q = %q, want stripped", h, got)
		}
	}

	kept := map[string]string{
		"Authorization": "Bearer abc",
		"Content-Type":  "application/json",
	}
	for h, want := range kept {
		if got := r.Header.Get(h); got != want {
			t.Errorf("header %q = %q, want %q", h, got, want)
		}
	}
}

func TestBuildTarget(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		base      string
		target    string
		want      string
		wantHost  string
		wantPath  string
		wantQuery string
	}{
		{
			name:      "origin form path and query",
			base:      "http://users-v2:8081",
			target:    "/users/123?active=true",
			want:      "http://users-v2:8081/users/123?active=true",
			wantHost:  "users-v2:8081",
			wantPath:  "/users/123",
			wantQuery: "active=true",
		},
		{
			name:     "https base",
			base:     "https://orders-v2:8443",
			target:   "/orders/7",
			want:     "https://orders-v2:8443/orders/7",
			wantHost: "orders-v2:8443",
			wantPath: "/orders/7",
		},
		{
			name:      "absolute form request cannot move host",
			base:      "http://users-v2:8081",
			target:    "http://evil.com/steal?x=1",
			want:      "http://users-v2:8081/steal?x=1",
			wantHost:  "users-v2:8081",
			wantPath:  "/steal",
			wantQuery: "x=1",
		},
		{
			name:     "encoded path preserved",
			base:     "http://users-v2:8081",
			target:   "/files/a%2Fb",
			want:     "http://users-v2:8081/files/a%2Fb",
			wantHost: "users-v2:8081",
			wantPath: "/files/a/b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &Shadow{}
			in := httptest.NewRequest(http.MethodGet, tt.target, nil)

			got := s.buildTarget(in, mustParseURL(t, tt.base))

			if got.String() != tt.want {
				t.Errorf("String() = %q, want %q", got.String(), tt.want)
			}
			if got.Host != tt.wantHost {
				t.Errorf("Host = %q, want %q", got.Host, tt.wantHost)
			}
			if got.Path != tt.wantPath {
				t.Errorf("Path = %q, want %q", got.Path, tt.wantPath)
			}
			if got.RawQuery != tt.wantQuery {
				t.Errorf("RawQuery = %q, want %q", got.RawQuery, tt.wantQuery)
			}
		})
	}
}

// backendRequest captures the fields of a shadow request that the Fire tests
// assert on.
type backendRequest struct {
	method         string
	requestURI     string
	host           string
	forwardedFor   string
	forwardedHost  string
	forwardedProto string
	authorization  string
	xCustom        string
	body           string
}

func TestFireSuccess(t *testing.T) {
	t.Parallel()

	got := make(chan backendRequest, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- backendRequest{
			method:         r.Method,
			requestURI:     r.URL.RequestURI(),
			host:           r.Host,
			forwardedFor:   r.Header.Get("X-Forwarded-For"),
			forwardedHost:  r.Header.Get("X-Forwarded-Host"),
			forwardedProto: r.Header.Get("X-Forwarded-Proto"),
			authorization:  r.Header.Get("Authorization"),
			xCustom:        r.Header.Get("X-Custom"),
			body:           string(body),
		}
		w.Header().Set("X-Backend", "v2")
		w.Write([]byte("shadow response"))
	}))
	t.Cleanup(backend.Close)

	backendURL := mustParseURL(t, backend.URL)
	s := newTestShadow()
	outcome := &models.Outcome{Type: models.OutcomeCandidate}

	in := httptest.NewRequest(http.MethodPost, "/api/users?x=1", nil)
	in.Host = "client.example.com"
	in.RemoteAddr = "192.0.2.1:4242"
	in.Header.Set("Authorization", "Bearer abc")
	in.Header.Set("Connection", "X-Custom")
	in.Header.Set("X-Custom", "secret")

	s.Fire(context.Background(), outcome, backendURL, []byte("req-body"), in, 1<<20)

	// Request side: the backend must receive the correctly rewritten request.
	req := <-got
	if req.method != http.MethodPost {
		t.Errorf("backend method = %q, want %q", req.method, http.MethodPost)
	}
	if req.requestURI != "/api/users?x=1" {
		t.Errorf("backend requestURI = %q, want %q", req.requestURI, "/api/users?x=1")
	}
	if req.host != backendURL.Host {
		t.Errorf("backend Host = %q, want %q", req.host, backendURL.Host)
	}
	if req.forwardedFor != "192.0.2.1" {
		t.Errorf("X-Forwarded-For = %q, want %q", req.forwardedFor, "192.0.2.1")
	}
	if req.forwardedHost != "client.example.com" {
		t.Errorf("X-Forwarded-Host = %q, want %q", req.forwardedHost, "client.example.com")
	}
	if req.forwardedProto != "http" {
		t.Errorf("X-Forwarded-Proto = %q, want %q", req.forwardedProto, "http")
	}
	if req.authorization != "Bearer abc" {
		t.Errorf("Authorization = %q, want %q", req.authorization, "Bearer abc")
	}
	if req.xCustom != "" {
		t.Errorf("X-Custom = %q, want stripped (hop-by-hop)", req.xCustom)
	}
	if req.body != "req-body" {
		t.Errorf("backend body = %q, want %q", req.body, "req-body")
	}

	// Response side: the outcome must capture the backend response.
	if outcome.Status != http.StatusOK {
		t.Errorf("Status = %d, want %d", outcome.Status, http.StatusOK)
	}
	if string(outcome.Body) != "shadow response" {
		t.Errorf("Body = %q, want %q", string(outcome.Body), "shadow response")
	}
	if got := outcome.Headers.Get("X-Backend"); got != "v2" {
		t.Errorf("Header X-Backend = %q, want %q", got, "v2")
	}
	if outcome.Error != models.OutcomeErrorNone {
		t.Errorf("Error = %v, want %v", outcome.Error, models.OutcomeErrorNone)
	}

	// The outcome must capture the full URL forwarded to the backend.
	if wantURL := backendURL.String() + "/api/users?x=1"; outcome.URL != wantURL {
		t.Errorf("URL = %q, want %q", outcome.URL, wantURL)
	}
}

func TestFireErrorStatusCodes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status int
		want   models.OutcomeErrorType
	}{
		{name: "not found", status: http.StatusNotFound, want: models.ErrNotFound},
		{name: "client error", status: http.StatusBadRequest, want: models.ErrClient},
		{name: "server error", status: http.StatusInternalServerError, want: models.ErrServer},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			t.Cleanup(backend.Close)

			s := newTestShadow()
			outcome := &models.Outcome{Type: models.OutcomeCandidate}
			in := httptest.NewRequest(http.MethodGet, "/", nil)

			s.Fire(context.Background(), outcome, mustParseURL(t, backend.URL), nil, in, 1<<20)

			if outcome.Status != tt.status {
				t.Errorf("Status = %d, want %d", outcome.Status, tt.status)
			}
			if outcome.Error != tt.want {
				t.Errorf("Error = %v, want %v", outcome.Error, tt.want)
			}
		})
	}
}

func TestFireTransportError(t *testing.T) {
	t.Parallel()
	s := newTestShadow()
	outcome := &models.Outcome{Type: models.OutcomeCandidate}
	in := httptest.NewRequest(http.MethodGet, "/", nil)

	s.Fire(context.Background(), outcome, closedServerURL(t), nil, in, 1<<20)

	if outcome.Error != models.ErrTransport {
		t.Errorf("Error = %v, want %v", outcome.Error, models.ErrTransport)
	}
}
