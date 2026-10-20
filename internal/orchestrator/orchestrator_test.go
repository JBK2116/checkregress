package orchestrator

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/JBK2116/checkregress/internal/differ"
	"github.com/JBK2116/checkregress/internal/models"
	"github.com/JBK2116/checkregress/internal/network"
	"github.com/JBK2116/checkregress/internal/shadow"
)

// fakeStore records rows handed to AddRow so tests can assert what the
// Orchestrator persisted.
type fakeStore struct {
	ch chan models.ShadowRow
}

func (f *fakeStore) AddRow(row *models.ShadowRow) {
	f.ch <- *row
}

func mustParseURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("parsing URL %q: %v", s, err)
	}
	return u
}

func TestShouldDiff(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		outcomes []models.Outcome
		want     bool
	}{
		{
			name: "all successful diffs",
			outcomes: []models.Outcome{
				{Error: models.OutcomeErrorNone},
				{Error: models.OutcomeErrorNone},
				{Error: models.OutcomeErrorNone},
			},
			want: true,
		},
		{
			name: "transport error skips",
			outcomes: []models.Outcome{
				{Error: models.ErrTransport},
				{Error: models.OutcomeErrorNone},
				{Error: models.OutcomeErrorNone},
			},
			want: false,
		},
		{
			name: "timeout skips",
			outcomes: []models.Outcome{
				{Error: models.OutcomeErrorNone},
				{Error: models.ErrTimeout},
				{Error: models.OutcomeErrorNone},
			},
			want: false,
		},
		{
			name: "not found still diffs",
			outcomes: []models.Outcome{
				{Error: models.ErrNotFound},
				{Error: models.OutcomeErrorNone},
				{Error: models.OutcomeErrorNone},
			},
			want: true,
		},
		{
			name: "client error still diffs",
			outcomes: []models.Outcome{
				{Error: models.OutcomeErrorNone},
				{Error: models.ErrClient},
				{Error: models.OutcomeErrorNone},
			},
			want: true,
		},
		{
			name: "server error still diffs",
			outcomes: []models.Outcome{
				{Error: models.OutcomeErrorNone},
				{Error: models.OutcomeErrorNone},
				{Error: models.ErrServer},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldDiff(tt.outcomes...); got != tt.want {
				t.Errorf("shouldDiff(...) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenRow(t *testing.T) {
	t.Parallel()

	legacy := models.Outcome{
		CanonicalPath: "GET /api/users/{id}",
		URL:           "http://legacy/api/users/42",
		Status:        http.StatusOK,
	}
	secondary := models.Outcome{URL: "http://secondary/api/users/42"}
	candidate := models.Outcome{
		URL:    "http://candidate/api/users/42",
		Status: http.StatusOK,
	}

	t.Run("status match sets headers and body", func(t *testing.T) {
		t.Parallel()
		res := models.Result{
			StatusMatch: true,
			HeaderDiffs: models.HeaderDiffs{"X-Test": {Present: true, Matching: true}},
			BodyDiffs:   models.BodyDiffs{BytesMatch: true, Paths: map[string]models.Diff{}},
		}
		row := genRow(legacy, secondary, candidate, res)
		if !row.StatusMatch {
			t.Error("StatusMatch = false, want true")
		}
		if row.Headers == nil {
			t.Error("Headers = nil, want non-nil on status match")
		}
		if row.Body == nil {
			t.Error("Body = nil, want non-nil on status match")
		}
	})

	t.Run("status mismatch leaves headers and body nil", func(t *testing.T) {
		t.Parallel()
		res := models.Result{StatusMatch: false}
		row := genRow(legacy, secondary, candidate, res)
		if row.StatusMatch {
			t.Error("StatusMatch = true, want false")
		}
		if row.Headers != nil {
			t.Errorf("Headers = %v, want nil on status mismatch", row.Headers)
		}
		if row.Body != nil {
			t.Errorf("Body = %v, want nil on status mismatch", row.Body)
		}
	})
}

// jsonHandler returns a handler serving body with a JSON content type and the
// given status, optionally delaying before responding.
func jsonHandler(status int, body string, delay time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if delay > 0 {
			time.Sleep(delay)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

// newTestOrchestrator wires real matcher/shadow/proxy/differ around three
// httptest backends and returns an Orchestrator configured with the given
// logger and store. shadowDelay slows the secondary/candidate backends so a
// test can observe the goroutine's behavior before shadows complete.
func newTestOrchestrator(t *testing.T, logger *slog.Logger, store shadowStore, candidateStatus int, shadowDelay time.Duration) *Orchestrator {
	t.Helper()

	const maxBodyBytes = 1 << 20

	legacy := httptest.NewServer(jsonHandler(http.StatusOK, `{"a":1}`, 0))
	t.Cleanup(legacy.Close)
	secondary := httptest.NewServer(jsonHandler(http.StatusOK, `{"a":1}`, shadowDelay))
	t.Cleanup(secondary.Close)
	candidate := httptest.NewServer(jsonHandler(candidateStatus, `{"a":1}`, shadowDelay))
	t.Cleanup(candidate.Close)

	route := models.Route{
		Legacy:    mustParseURL(t, legacy.URL),
		Secondary: mustParseURL(t, secondary.URL),
		Candidate: mustParseURL(t, candidate.URL),
		Paths:     []string{"GET /api/users/{id}"},
	}

	pr := network.NewReverseProxy(route, logger, maxBodyBytes)
	d := differ.NewDiffer()
	m := network.NewMatcher(route.Paths)
	sh := shadow.NewShadow(logger, http.DefaultClient)

	return NewOrchestrator(logger, pr, d, m, sh, store, route, 1000, 5000, maxBodyBytes)
}

func TestServeHTTPStoresShadowRowOnMatch(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	fs := &fakeStore{ch: make(chan models.ShadowRow, 1)}
	o := newTestOrchestrator(t, logger, fs, http.StatusOK, 0)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/users/42", nil)
	o.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("client status = %d, want %d", rr.Code, http.StatusOK)
	}

	select {
	case row := <-fs.ch:
		if !row.StatusMatch {
			t.Errorf("StatusMatch = %v, want true", row.StatusMatch)
		}
		if row.StatusLegacy != http.StatusOK || row.StatusCandidate != http.StatusOK {
			t.Errorf("statuses = %d/%d, want %d/%d", row.StatusLegacy, row.StatusCandidate, http.StatusOK, http.StatusOK)
		}
		if row.Body == nil {
			t.Error("Body = nil, want non-nil on status match")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("store.AddRow was not called")
	}
}

func TestServeHTTPStoresStatusMismatchRow(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	fs := &fakeStore{ch: make(chan models.ShadowRow, 1)}
	o := newTestOrchestrator(t, logger, fs, http.StatusNotFound, 0)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/users/42", nil)
	o.ServeHTTP(rr, req)

	// The client is still served the legacy response.
	if rr.Code != http.StatusOK {
		t.Fatalf("client status = %d, want %d", rr.Code, http.StatusOK)
	}

	select {
	case row := <-fs.ch:
		if row.StatusMatch {
			t.Error("StatusMatch = true, want false")
		}
		if row.Headers != nil {
			t.Errorf("Headers = %v, want nil on status mismatch", row.Headers)
		}
		if row.Body != nil {
			t.Errorf("Body = %v, want nil on status mismatch", row.Body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("store.AddRow was not called")
	}
}

func TestServeHTTPNonShadowedDoesNotStore(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	fs := &fakeStore{ch: make(chan models.ShadowRow, 1)}
	o := newTestOrchestrator(t, logger, fs, http.StatusOK, 0)

	rr := httptest.NewRecorder()
	// A path that does not match the shadow allowlist.
	req := httptest.NewRequest(http.MethodGet, "/api/orders/7", nil)
	o.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("client status = %d, want %d", rr.Code, http.StatusOK)
	}

	// The non-shadow path has no background goroutine, so AddRow can only have
	// happened synchronously by now.
	select {
	case <-fs.ch:
		t.Error("store.AddRow was called for a non-shadowed request")
	default:
		// expected: no row
	}
}

func TestServeHTTPWaitDrainsDiffer(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	fs := &fakeStore{ch: make(chan models.ShadowRow, 1)}
	o := newTestOrchestrator(t, logger, fs, http.StatusOK, 0)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/users/42", nil)
	o.ServeHTTP(rr, req)

	// Wait must block until the detached differ goroutine has delivered its row.
	o.Wait()

	// The row must already be in the store without any further waiting.
	select {
	case row := <-fs.ch:
		if row.CanonicalPath != "GET /api/users/{id}" {
			t.Errorf("CanonicalPath = %q, want %q", row.CanonicalPath, "GET /api/users/{id}")
		}
	default:
		t.Error("store.AddRow was not called before Wait returned")
	}
}
