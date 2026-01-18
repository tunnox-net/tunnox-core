// Package postgres provides PostgreSQL storage implementation using structured tables.
//
// This package directly connects to PostgreSQL and operates on structured tables:
// - client_configs: Stores client configuration data
// - port_mappings: Stores port mapping data
// - nodes: Stores cluster node data
// - http_domain_mappings: Stores HTTP domain mappings
// - webhooks: Stores webhook configurations
//
// This replaces the old gRPC -> kv_store architecture with direct SQL queries.
package postgres

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"tunnox-core/internal/core/dispose"
)

// Config PostgreSQL storage configuration
type Config struct {
	// DSN is the PostgreSQL connection string
	// Format: postgresql://user:password@host:port/database?sslmode=disable
	DSN string

	// MaxConns is the maximum number of connections in the pool
	MaxConns int32

	// MinConns is the minimum number of connections in the pool
	MinConns int32

	// MaxConnLifetime is the maximum lifetime of a connection
	MaxConnLifetime time.Duration

	// MaxConnIdleTime is the maximum idle time of a connection
	MaxConnIdleTime time.Duration

	// ConnectTimeout is the timeout for establishing a connection
	ConnectTimeout time.Duration
}

// DefaultConfig returns default PostgreSQL configuration
func DefaultConfig() *Config {
	return &Config{
		MaxConns:        20,
		MinConns:        5,
		MaxConnLifetime: 30 * time.Minute,
		MaxConnIdleTime: 5 * time.Minute,
		ConnectTimeout:  10 * time.Second,
	}
}

// Storage is the PostgreSQL storage implementation
type Storage struct {
	*dispose.ManagerBase

	pool   *pgxpool.Pool
	config *Config

	mu sync.RWMutex
}

// New creates a new PostgreSQL storage instance
func New(parentCtx context.Context, config *Config) (*Storage, error) {
	if config == nil {
		return nil, fmt.Errorf("postgres config is required")
	}
	if config.DSN == "" {
		return nil, fmt.Errorf("postgres DSN is required")
	}

	// Apply defaults for missing values
	if config.MaxConns == 0 {
		config.MaxConns = DefaultConfig().MaxConns
	}
	if config.MinConns == 0 {
		config.MinConns = DefaultConfig().MinConns
	}
	if config.MaxConnLifetime == 0 {
		config.MaxConnLifetime = DefaultConfig().MaxConnLifetime
	}
	if config.MaxConnIdleTime == 0 {
		config.MaxConnIdleTime = DefaultConfig().MaxConnIdleTime
	}
	if config.ConnectTimeout == 0 {
		config.ConnectTimeout = DefaultConfig().ConnectTimeout
	}

	// Parse and configure the connection pool
	poolConfig, err := pgxpool.ParseConfig(config.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DSN: %w", err)
	}

	poolConfig.MaxConns = config.MaxConns
	poolConfig.MinConns = config.MinConns
	poolConfig.MaxConnLifetime = config.MaxConnLifetime
	poolConfig.MaxConnIdleTime = config.MaxConnIdleTime
	poolConfig.ConnConfig.ConnectTimeout = config.ConnectTimeout

	// Create connection pool
	ctx, cancel := context.WithTimeout(parentCtx, config.ConnectTimeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	storage := &Storage{
		ManagerBase: dispose.NewManager("PostgresStorage", parentCtx),
		pool:        pool,
		config:      config,
	}

	storage.SetCtx(parentCtx, storage.onClose)

	dispose.Infof("PostgresStorage: connected to database, maxConns=%d, minConns=%d",
		config.MaxConns, config.MinConns)

	return storage, nil
}

// onClose handles resource cleanup
func (s *Storage) onClose() error {
	dispose.Infof("PostgresStorage: closing connection pool")
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.pool != nil {
		s.pool.Close()
		s.pool = nil
	}
	return nil
}

// Pool returns the underlying connection pool for advanced operations
func (s *Storage) Pool() *pgxpool.Pool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pool
}

// Ping checks database connectivity
func (s *Storage) Ping(ctx context.Context) error {
	s.mu.RLock()
	pool := s.pool
	s.mu.RUnlock()

	if pool == nil {
		return fmt.Errorf("connection pool is closed")
	}
	return pool.Ping(ctx)
}

// Close closes the storage connection
func (s *Storage) Close() error {
	s.Dispose.Close()
	return nil
}

// Exec executes a query without returning rows
func (s *Storage) Exec(ctx context.Context, sql string, args ...any) error {
	s.mu.RLock()
	pool := s.pool
	s.mu.RUnlock()

	if pool == nil {
		return fmt.Errorf("connection pool is closed")
	}

	_, err := pool.Exec(ctx, sql, args...)
	return err
}

// Query executes a query and returns rows
func (s *Storage) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	s.mu.RLock()
	pool := s.pool
	s.mu.RUnlock()

	if pool == nil {
		return nil, fmt.Errorf("connection pool is closed")
	}

	return pool.Query(ctx, sql, args...)
}

// QueryRow executes a query that returns a single row
func (s *Storage) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	s.mu.RLock()
	pool := s.pool
	s.mu.RUnlock()

	if pool == nil {
		// Return a dummy row that will error on scan
		return &closedRow{}
	}

	return pool.QueryRow(ctx, sql, args...)
}

// closedRow is a dummy row returned when the pool is closed
type closedRow struct{}

func (r *closedRow) Scan(dest ...any) error {
	return fmt.Errorf("connection pool is closed")
}

// Stats returns pool statistics
func (s *Storage) Stats() *pgxpool.Stat {
	s.mu.RLock()
	pool := s.pool
	s.mu.RUnlock()

	if pool == nil {
		return nil
	}
	return pool.Stat()
}
