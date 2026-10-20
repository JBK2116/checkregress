package store

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/JBK2116/checkregress/internal/models"
	"github.com/jackc/pgx/v5"
)

// fakePool records the rows it copies so tests can assert on flush behavior
// without a real database.
type fakePool struct {
	mu     sync.Mutex
	copied []int // row count of each CopyFrom call
	closed bool
}

func (f *fakePool) CopyFrom(_ context.Context, _ pgx.Identifier, _ []string, src pgx.CopyFromSource) (int64, error) {
	var count int64
	for src.Next() {
		if _, err := src.Values(); err != nil {
			return count, err
		}
		count++
	}
	f.mu.Lock()
	f.copied = append(f.copied, int(count))
	f.mu.Unlock()
	return count, src.Err()
}

func (f *fakePool) Close() {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
}

func (f *fakePool) totalCopied() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	for _, n := range f.copied {
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

	if got := fp.totalCopied(); got != 3 {
		t.Errorf("copied %d rows, want 3", got)
	}
}

// TestRunFlushesAtCapacity verifies the batchSize trigger still flushes.
func TestRunFlushesAtCapacity(t *testing.T) {
	t.Parallel()

	st, fp := newTestStore()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	for i := 0; i < batchSize; i++ {
		st.AddRow(&models.ShadowRow{CanonicalPath: "/x"})
	}

	// The Run goroutine drains into its batch and flushes once it reaches
	// batchSize. Poll briefly for that flush.
	deadline := time.Now().Add(2 * time.Second)
	for fp.totalCopied() < batchSize && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if got := fp.totalCopied(); got != batchSize {
		t.Errorf("copied %d rows, want %d", got, batchSize)
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

	if got := fp.totalCopied(); got != 0 {
		t.Errorf("copied %d rows, want 0", got)
	}
}
