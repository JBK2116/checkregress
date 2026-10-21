package store

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JBK2116/checkregress/internal/models"
)

// fakePool records the batches it inserts so tests can assert on flush
// behavior without a real database.
type fakePool struct {
	mu      sync.Mutex
	batches []int // row count of each InsertRows call
	closed  bool
}

func (f *fakePool) InsertRows(_ context.Context, rows []*models.ShadowRow) error {
	f.mu.Lock()
	f.batches = append(f.batches, len(rows))
	f.mu.Unlock()
	return nil
}

func (f *fakePool) Close() error {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	return nil
}

func (f *fakePool) totalInserted() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	for _, n := range f.batches {
		total += n
	}
	return total
}

func newTestStore() (*Store, *fakePool) {
	fp := &fakePool{}
	st := &Store{
		pool:   fp,
		rows:   make(chan *models.ShadowRow, batchSize),
		logger: slog.New(slog.DiscardHandler),
		done:   make(chan struct{}),
	}
	return st, fp
}

// TestRunFlushesBacklogOnShutdown verifies that canceling Run's context drains
// the channel backlog (not just the in-progress batch) and flushes it before
// Wait returns.
func TestRunFlushesBacklogOnShutdown(t *testing.T) {
	t.Parallel()

	st, fp := newTestStore()

	st.AddRow(&models.ShadowRow{CanonicalPath: "/a"})
	st.AddRow(&models.ShadowRow{CanonicalPath: "/b"})
	st.AddRow(&models.ShadowRow{CanonicalPath: "/c"})

	ctx, cancel := context.WithCancel(context.Background())
	go st.Run(ctx)
	cancel()

	st.Wait()

	if got := fp.totalInserted(); got != 3 {
		t.Errorf("inserted %d rows, want 3", got)
	}
}

// TestRunFlushesAtCapacity verifies the batchSize trigger still flushes.
func TestRunFlushesAtCapacity(t *testing.T) {
	t.Parallel()

	st, fp := newTestStore()

	ctx := t.Context()
	go st.Run(ctx)

	for range batchSize {
		st.AddRow(&models.ShadowRow{CanonicalPath: "/x"})
	}

	// The Run goroutine drains into its batch and flushes once it reaches
	// batchSize. Poll briefly for that flush.
	deadline := time.Now().Add(2 * time.Second)
	for fp.totalInserted() < batchSize && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if got := fp.totalInserted(); got != batchSize {
		t.Errorf("inserted %d rows, want %d", got, batchSize)
	}
}

// TestRunShutdownWithNoRows verifies a clean shutdown flushes nothing and Wait
// still returns.
func TestRunShutdownWithNoRows(t *testing.T) {
	t.Parallel()

	st, fp := newTestStore()

	ctx, cancel := context.WithCancel(context.Background())
	go st.Run(ctx)
	cancel()

	st.Wait()

	if got := fp.totalInserted(); got != 0 {
		t.Errorf("inserted %d rows, want 0", got)
	}
}

// matchedRow returns a shadow row whose statuses match and whose headers and
// body are populated, mirroring a successful diff.
func matchedRow(legacyURL string) *models.ShadowRow {
	return &models.ShadowRow{
		CanonicalPath:   "GET /api/users/{id}",
		LegacyURL:       legacyURL,
		SecondaryURL:    "http://secondary/api/users/1",
		CandidateURL:    "http://candidate/api/users/1",
		StatusLegacy:    200,
		StatusCandidate: 200,
		StatusMatch:     true,
		Headers:         models.HeaderDiffs{"X-Test": {Present: true, Matching: true}},
		Body:            &models.BodyDiffs{BytesMatch: true, Paths: map[string]models.Diff{"/name": {Present: true, Matching: true}}},
	}
}

// TestNewStoreCreatesSchemaAndInserts verifies that NewStore creates the
// schema, inserts matched and mismatched rows, stores status_match and
// headers correctly, and rejects a row that violates the row-kind constraint.
func TestNewStoreCreatesSchemaAndInserts(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "checkregress.db")
	st, err := NewStore(context.Background(), slog.New(slog.DiscardHandler), path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer st.Close()

	matched := matchedRow("http://legacy/api/users/1")
	mismatched := &models.ShadowRow{
		CanonicalPath:   "POST /api/users",
		LegacyURL:       "http://legacy/api/users",
		SecondaryURL:    "http://secondary/api/users",
		CandidateURL:    "http://candidate/api/users",
		StatusLegacy:    200,
		StatusCandidate: 500,
		StatusMatch:     false,
	}

	if err := st.pool.InsertRows(context.Background(), []*models.ShadowRow{matched, mismatched}); err != nil {
		t.Fatalf("InsertRows: %v", err)
	}

	sqldb := st.pool.(*sqliteDB).db
	var statusMatch int
	var headers string
	if err := sqldb.QueryRowContext(context.Background(),
		`SELECT status_match, headers FROM shadows WHERE legacy_url = ?`,
		"http://legacy/api/users/1",
	).Scan(&statusMatch, &headers); err != nil {
		t.Fatalf("read matched row: %v", err)
	}
	if statusMatch != 1 {
		t.Errorf("status_match = %d, want 1", statusMatch)
	}
	if !strings.Contains(headers, "X-Test") {
		t.Errorf("headers = %q, want JSON containing X-Test", headers)
	}

	// A status-matched row must carry headers and body (row-kind CHECK).
	violated := &models.ShadowRow{
		CanonicalPath:   "GET /api/users/{id}",
		LegacyURL:       "http://legacy/api/users/2",
		SecondaryURL:    "http://secondary/api/users/2",
		CandidateURL:    "http://candidate/api/users/2",
		StatusLegacy:    200,
		StatusCandidate: 200,
		StatusMatch:     true,
	}
	if err := st.pool.InsertRows(context.Background(), []*models.ShadowRow{violated}); err == nil {
		t.Fatal("InsertRows accepted a status-matched row with no headers/body")
	}
}

// TestNewStoreCreatesSchemaIdempotently verifies that reopening the same
// database file does not fail when the schema already exists.
func TestNewStoreCreatesSchemaIdempotently(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "checkregress.db")

	first, err := NewStore(context.Background(), slog.New(slog.DiscardHandler), path)
	if err != nil {
		t.Fatalf("first NewStore: %v", err)
	}
	first.Close()

	second, err := NewStore(context.Background(), slog.New(slog.DiscardHandler), path)
	if err != nil {
		t.Fatalf("second NewStore: %v", err)
	}
	second.Close()
}
