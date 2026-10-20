package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// maxConns is the maximum number of open connections to the database.
	maxConns = 30
	// minConns is the steady-state minimum number of open connections to the database.
	minConns = 5
	// maxConnsLifetime is the maximum amount of time a connection may be reused before reopening fully.
	maxConnLifetime = time.Minute * 15
	// maxConnLifetimeJitter is random jitter added to maxConnsLifetime to avoid stampeding.
	maxConnLifetimeJitter = maxConnLifetime / 15
	// maxConnIdleTime is the maximum amount of time a connection may be idle before being closed.
	maxConnIdleTime = time.Minute * 3
	// healthCheckPeriod is the frequency to check the health of the database connections.
	healthCheckPeriod = time.Second * 20
	// pingCheckPeriod is the max amount of time to wait for a ping response.
	pingCheckPeriod = time.Second * 5
)

// newPool creates a new database connection pool.
func newPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	conf, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	conf.MaxConns = maxConns
	conf.MinConns = minConns
	conf.MaxConnLifetime = maxConnLifetime
	conf.MaxConnLifetimeJitter = maxConnLifetimeJitter
	conf.MaxConnIdleTime = maxConnIdleTime
	conf.HealthCheckPeriod = healthCheckPeriod

	pool, err := pgxpool.NewWithConfig(ctx, conf)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, pingCheckPeriod)
	defer cancel()
	err = pool.Ping(ctx)
	if err != nil {
		return nil, err
	}

	return pool, nil
}
