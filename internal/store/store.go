package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/JBK2116/checkregress/internal/models"
)

var (
	ErrStatusMismatch = errors.New("headers/body must be null IFF status_match is false")
)

const (
	// batchSize is the number of rows to batch insert at a time.
	batchSize = 3000
	// batchInterval is the interval to batch insert at a time.
	batchInterval = time.Second * 5
	// batchTimeout is the maximum amount of time to wait for a batch insert to complete.
	batchTimeout = time.Second * 3
	// poolOpenTimeout is the maximum amount of time to wait for a connection to open.
	poolOpenTimeout = time.Second * 10
)

// pool is the subset of the SQLite database the Store depends on, kept as an
// interface so tests can inject a fake.
type pool interface {
	// InsertRows inserts a batch of rows into the shadows table.
	InsertRows(context.Context, []*models.ShadowRow) error
	// Close closes the underlying database.
	Close() error
}

// Store provides functionality for interacting with the shadows table.
type Store struct {
	// pool is the underlying database connection.
	pool pool
	// rows is the channel that receives the rows to be batch inserted.
	rows chan *models.ShadowRow
	// logger is the logger to use for logging in this store.
	logger *slog.Logger
	// done is closed when Run returns, after the final drain and flush.
	done chan struct{}
}

// NewStore creates a new store configured with the provided values.
func NewStore(ctx context.Context, logger *slog.Logger, path string) (*Store, error) {
	ctx, cancel := context.WithTimeout(ctx, poolOpenTimeout)
	defer cancel()

	pool, err := newDB(ctx, path)
	if err != nil {
		return nil, err
	}
	return &Store{
		pool:   pool,
		rows:   make(chan *models.ShadowRow, batchSize),
		logger: logger,
		done:   make(chan struct{}),
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
	defer close(s.done)

	ticker := time.NewTicker(batchInterval)
	defer ticker.Stop()

	batch := make([]*models.ShadowRow, 0, batchSize)
	for {
		select {
		case <-ctx.Done():
			s.logger.InfoContext(ctx, "server shutting down...flushing remaining rows")
			s.drainAndFlush(ctx, batch)
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

// drainAndFlush drains any rows still buffered in the channel and flushes them
// together with the in-progress batch. Callers must ensure no further AddRow
// calls are made (the producers have already been drained) so the loop
// terminates.
func (s *Store) drainAndFlush(ctx context.Context, batch []*models.ShadowRow) {
	for {
		select {
		case row := <-s.rows:
			batch = append(batch, row)
		default:
			if len(batch) > 0 {
				if err := s.batch(context.WithoutCancel(ctx), batch); err != nil {
					s.logger.ErrorContext(ctx, "error during batch insert", "err", err)
				}
			}
			return
		}
	}
}

// Wait blocks until Run has returned, i.e. the final drain and flush have
// completed.
func (s *Store) Wait() {
	<-s.done
}

// batch inserts the provided rows into the shadows table.
func (s *Store) batch(ctx context.Context, rows []*models.ShadowRow) error {
	ctxBatch, cancel := context.WithTimeout(ctx, batchTimeout)
	defer cancel()

	if len(rows) == 0 {
		return nil
	}

	if err := s.pool.InsertRows(ctxBatch, rows); err != nil {
		return fmt.Errorf("insert shadows: %w", err)
	}
	return nil
}

// Close closes the underlying database connection.
func (s *Store) Close() {
	_ = s.pool.Close()
}
