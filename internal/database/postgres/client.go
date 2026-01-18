package postgres

import (
	"context"
	"embed"
	"fmt"
	"os"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Config holds PostgreSQL connection configuration
type Config struct {
	Host         string
	Port         int
	Database     string
	Username     string
	Password     string
	SSLMode      string
	MaxConns     int32
	MinConns     int32
	MaxConnLife  time.Duration
	MaxConnIdle  time.Duration
	HealthCheck  time.Duration
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Host:        "localhost",
		Port:        5432,
		Database:    "badops",
		SSLMode:     "prefer",
		MaxConns:    25,
		MinConns:    5,
		MaxConnLife: time.Hour,
		MaxConnIdle: 30 * time.Minute,
		HealthCheck: time.Minute,
	}
}

// Client wraps a PostgreSQL connection pool
type Client struct {
	pool   *pgxpool.Pool
	config *Config
}

// NewClient creates a new PostgreSQL client
func NewClient(cfg *Config) *Client {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Client{config: cfg}
}

// Connect establishes a connection to the database
func (c *Client) Connect(ctx context.Context) error {
	connString := c.buildConnectionString()

	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return fmt.Errorf("failed to parse connection string: %w", err)
	}

	poolConfig.MaxConns = c.config.MaxConns
	poolConfig.MinConns = c.config.MinConns
	poolConfig.MaxConnLifetime = c.config.MaxConnLife
	poolConfig.MaxConnIdleTime = c.config.MaxConnIdle
	poolConfig.HealthCheckPeriod = c.config.HealthCheck

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	c.pool = pool
	return nil
}

// Close closes the database connection pool
func (c *Client) Close() {
	if c.pool != nil {
		c.pool.Close()
	}
}

// Pool returns the underlying connection pool
func (c *Client) Pool() *pgxpool.Pool {
	return c.pool
}

// Ping checks if the database connection is alive
func (c *Client) Ping(ctx context.Context) error {
	if c.pool == nil {
		return fmt.Errorf("database not connected")
	}
	return c.pool.Ping(ctx)
}

// Stats returns connection pool statistics
func (c *Client) Stats() *pgxpool.Stat {
	if c.pool == nil {
		return nil
	}
	return c.pool.Stat()
}

// RunMigrations applies all pending database migrations
func (c *Client) RunMigrations() error {
	d, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	connString := c.buildConnectionString()
	m, err := migrate.NewWithSourceInstance("iofs", d, connString)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration failed: %w", err)
	}

	return nil
}

// MigrationVersion returns the current migration version
func (c *Client) MigrationVersion() (uint, bool, error) {
	d, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return 0, false, fmt.Errorf("failed to load migrations: %w", err)
	}

	connString := c.buildConnectionString()
	m, err := migrate.NewWithSourceInstance("iofs", d, connString)
	if err != nil {
		return 0, false, fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer m.Close()

	return m.Version()
}

// RollbackMigration rolls back the last migration
func (c *Client) RollbackMigration() error {
	d, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	connString := c.buildConnectionString()
	m, err := migrate.NewWithSourceInstance("iofs", d, connString)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer m.Close()

	if err := m.Steps(-1); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("rollback failed: %w", err)
	}

	return nil
}

// buildConnectionString constructs the PostgreSQL connection URL
func (c *Client) buildConnectionString() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.config.Username,
		c.config.Password,
		c.config.Host,
		c.config.Port,
		c.config.Database,
		c.config.SSLMode,
	)
}

// ConfigFromEnv creates a Config from environment variables
func ConfigFromEnv(usernameEnv, passwordEnv string) *Config {
	cfg := DefaultConfig()
	cfg.Username = os.Getenv(usernameEnv)
	cfg.Password = os.Getenv(passwordEnv)
	return cfg
}

// TableStats represents statistics for a database table
type TableStats struct {
	TableName string
	RowCount  int64
	Size      string
}

// GetTableStats returns row counts and sizes for all badops tables
func (c *Client) GetTableStats(ctx context.Context) ([]TableStats, error) {
	if c.pool == nil {
		return nil, fmt.Errorf("database not connected")
	}

	query := `
		SELECT
			relname as table_name,
			n_live_tup as row_count,
			pg_size_pretty(pg_total_relation_size(relid)) as size
		FROM pg_stat_user_tables
		WHERE schemaname = 'public'
		ORDER BY n_live_tup DESC
	`

	rows, err := c.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query table stats: %w", err)
	}
	defer rows.Close()

	var stats []TableStats
	for rows.Next() {
		var s TableStats
		if err := rows.Scan(&s.TableName, &s.RowCount, &s.Size); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		stats = append(stats, s)
	}

	return stats, rows.Err()
}

// DatabaseInfo contains information about the database
type DatabaseInfo struct {
	Version        string
	DatabaseName   string
	DatabaseSize   string
	ConnectionsMax int
	ConnectionsNow int
}

// GetDatabaseInfo returns general database information
func (c *Client) GetDatabaseInfo(ctx context.Context) (*DatabaseInfo, error) {
	if c.pool == nil {
		return nil, fmt.Errorf("database not connected")
	}

	info := &DatabaseInfo{}

	// Get PostgreSQL version
	err := c.pool.QueryRow(ctx, "SELECT version()").Scan(&info.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}

	// Get database name
	err = c.pool.QueryRow(ctx, "SELECT current_database()").Scan(&info.DatabaseName)
	if err != nil {
		return nil, fmt.Errorf("failed to get database name: %w", err)
	}

	// Get database size
	err = c.pool.QueryRow(ctx, "SELECT pg_size_pretty(pg_database_size(current_database()))").Scan(&info.DatabaseSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get database size: %w", err)
	}

	// Get connection limits
	err = c.pool.QueryRow(ctx, "SELECT setting::int FROM pg_settings WHERE name = 'max_connections'").Scan(&info.ConnectionsMax)
	if err != nil {
		return nil, fmt.Errorf("failed to get max connections: %w", err)
	}

	// Get current connections
	err = c.pool.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()").Scan(&info.ConnectionsNow)
	if err != nil {
		return nil, fmt.Errorf("failed to get current connections: %w", err)
	}

	return info, nil
}
