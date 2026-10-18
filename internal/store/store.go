package store

import (
	"context"
	"fmt"

	"errors"
	"log/slog"
	"time"

	"github.com/JBK2116/checkregress/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrStatusMismatch = errors.New("headers/body must be null IFF status_match is false")
)

// shadowCols is the list of columns to copy into the shadows table.
var shadowCols = []string{
	"canonical_path", "legacy_url", "secondary_url", "candidate_url",
	"status_legacy", "status_candidate", "status_match",
	"headers", "body",
}

const (
	// batchSize is the number of rows to batch insert at a time.
	batchSize = 3000
	// batchInterval is the interval to batch insert at a time.
	batchInterval = time.Second * 5
	// batchTimeout is the maximum amount of time to wait for a batch insert to complete.
	batchTimeout = time.Second * 3
	// poolOpenTimeout is the maximum amount of time to wait for a connection pool to open.
	poolOpenTimeout = time.Second * 10
)

// Store provides functionality for interacting with the shadows table.
type Store struct {
	// db is the underlying database connection.
	pool *pgxpool.Pool
	// rowChan is the channel that receives the rows to be batch inserted.
	rows chan *models.ShadowRow
	// logger is the logger to use for logging in this store.
	logger *slog.Logger
}

// NewStore creates a new store configured with the provided values.
func NewStore(ctx context.Context, logger *slog.Logger, url string) (*Store, error) {
	ctx, cancel := context.WithTimeout(ctx, poolOpenTimeout)
	defer cancel()

	pool, err := newPool(ctx, url)
	if err != nil {
		return nil, err
	}
	return &Store{
		pool:   pool,
		rows:   make(chan *models.ShadowRow, batchSize),
		logger: logger,
	}, nil
}

// AddRow adds a new shadow to the store.
func (s *Store) AddRow(shadow *models.ShadowRow) {
	select {
	case s.rows <- shadow:
		return
	default:
		// if the channel is full, the event is just dropped as it is not too important.
		return
	}
}

// Run is a periodic function that batches the rows in the row channel and inserts them into the shadows table.
//
// Rows are batched if the ticker fires, or if the rowChan is full.
func (s *Store) Run(ctx context.Context) {
	ticker := time.NewTicker(batchInterval)
	defer ticker.Stop()

	batch := make([]*models.ShadowRow, 0, batchSize)
	for {
		select {
		case <-ctx.Done():
			s.logger.InfoContext(ctx, "server shutting down...flushing remaining rows")
			if err := s.batch(context.WithoutCancel(ctx), batch); err != nil {
				s.logger.ErrorContext(ctx, "error during batch insert", "err", err)
			}
			return
		case <-ticker.C:
			if len(batch) == 0 {
				continue
			}
			if err := s.batch(ctx, batch); err != nil {
				s.logger.ErrorContext(ctx, "error during batch insert", "err", err)
			}
			clear(batch)
			batch = batch[:0]
		case row := <-s.rows:
			batch = append(batch, row)
			if len(batch) == batchSize {
				if err := s.batch(ctx, batch); err != nil {
					s.logger.ErrorContext(ctx, "error during batch insert", "err", err)
				}
				clear(batch)
				batch = batch[:0]
			}
		}
	}
}

// batch inserts the provided rows into the shadows table.
func (s *Store) batch(ctx context.Context, rows []*models.ShadowRow) error {
	ctxBatch, cancel := context.WithTimeout(ctx, batchTimeout)
	defer cancel()

	if len(rows) == 0 {
		return nil
	}

	src := pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
		r := rows[i]

		// Untyped nil becomes SQL NULL. The row-kind CHECK requires NULL, not JSON 'null'.
		var headers, body any
		if r.Headers != nil {
			headers = r.Headers
		}
		if r.Body != nil {
			body = r.Body
		}

		return []any{
			r.CanonicalPath, r.LegacyURL, r.SecondaryURL, r.CandidateURL,
			r.StatusLegacy, r.StatusCandidate, r.StatusMatch,
			headers, body,
		}, nil
	})

	n, err := s.pool.CopyFrom(ctxBatch, pgx.Identifier{"shadows"}, shadowCols, src)
	if err != nil {
		return fmt.Errorf("copy shadows: %w", err)
	}
	if int(n) != len(rows) {
		return fmt.Errorf("copy shadows: inserted %d of %d rows", n, len(rows))
	}
	return nil
}

// Close closes the underlying database connection.
func (s *Store) Close() {
	s.pool.Close()
}
