package clickhouse

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// Config holds ClickHouse connection configuration
type Config struct {
	Host     string
	Port     int
	Database string
	Username string
	Password string
	Secure   bool
	Debug    bool
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Host:     "localhost",
		Port:     9000,
		Database: "badops",
		Secure:   false,
		Debug:    false,
	}
}

// Client wraps a ClickHouse connection
type Client struct {
	conn   driver.Conn
	config *Config
}

// NewClient creates a new ClickHouse client
func NewClient(cfg *Config) *Client {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Client{config: cfg}
}

// Connect establishes a connection to ClickHouse
func (c *Client) Connect(ctx context.Context) error {
	protocol := clickhouse.Native
	if c.config.Secure {
		protocol = clickhouse.HTTP
	}

	options := &clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)},
		Auth: clickhouse.Auth{
			Database: c.config.Database,
			Username: c.config.Username,
			Password: c.config.Password,
		},
		Protocol: protocol,
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
		DialTimeout:     10 * time.Second,
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
	}

	if c.config.Debug {
		options.Debug = true
	}

	conn, err := clickhouse.Open(options)
	if err != nil {
		return fmt.Errorf("failed to open connection: %w", err)
	}

	// Verify connection
	if err := conn.Ping(ctx); err != nil {
		return fmt.Errorf("failed to ping ClickHouse: %w", err)
	}

	c.conn = conn
	return nil
}

// Close closes the ClickHouse connection
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Conn returns the underlying connection
func (c *Client) Conn() driver.Conn {
	return c.conn
}

// Ping checks if the connection is alive
func (c *Client) Ping(ctx context.Context) error {
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}
	return c.conn.Ping(ctx)
}

// InitSchema creates the required ClickHouse tables
func (c *Client) InitSchema(ctx context.Context) error {
	queries := []string{
		// Price history table
		`CREATE TABLE IF NOT EXISTS price_history (
			product_sku String,
			product_barcode Nullable(String),
			competitor_name String,
			price Decimal(12, 2),
			currency String DEFAULT 'NOK',
			in_stock UInt8 DEFAULT 1,
			stock_quantity Nullable(Int32),
			observed_at DateTime64(3),
			observed_date Date,
			source String DEFAULT 'sync'
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(observed_date)
		ORDER BY (product_sku, competitor_name, observed_at)
		TTL observed_date + INTERVAL 2 YEAR`,

		// Daily price aggregation materialized view
		`CREATE MATERIALIZED VIEW IF NOT EXISTS price_daily_mv
		ENGINE = SummingMergeTree()
		PARTITION BY toYYYYMM(date)
		ORDER BY (product_sku, competitor_name, date)
		AS SELECT
			product_sku,
			competitor_name,
			toDate(observed_at) as date,
			min(price) as min_price,
			max(price) as max_price,
			avg(price) as avg_price,
			count() as observation_count
		FROM price_history
		GROUP BY product_sku, competitor_name, date`,

		// Market position materialized view
		`CREATE MATERIALIZED VIEW IF NOT EXISTS price_position_mv
		ENGINE = SummingMergeTree()
		PARTITION BY toYYYYMM(date)
		ORDER BY (product_sku, date)
		AS SELECT
			product_sku,
			toDate(observed_at) as date,
			min(price) as market_min,
			max(price) as market_max,
			avg(price) as market_avg,
			count(DISTINCT competitor_name) as competitor_count
		FROM price_history
		GROUP BY product_sku, date`,
	}

	for _, query := range queries {
		if err := c.conn.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to execute schema query: %w", err)
		}
	}

	return nil
}

// ConfigFromEnv creates a Config from environment variables
func ConfigFromEnv(usernameEnv, passwordEnv string) *Config {
	cfg := DefaultConfig()
	cfg.Username = os.Getenv(usernameEnv)
	cfg.Password = os.Getenv(passwordEnv)
	return cfg
}

// TableInfo holds information about a ClickHouse table
type TableInfo struct {
	Name      string
	Rows      uint64
	BytesSize uint64
	Engine    string
}

// GetTableInfo returns information about tables in the database
func (c *Client) GetTableInfo(ctx context.Context) ([]TableInfo, error) {
	query := `
		SELECT
			name,
			total_rows,
			total_bytes,
			engine
		FROM system.tables
		WHERE database = currentDatabase()
		ORDER BY total_bytes DESC
	`

	rows, err := c.conn.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query tables: %w", err)
	}
	defer rows.Close()

	var tables []TableInfo
	for rows.Next() {
		var t TableInfo
		var totalRows, totalBytes *uint64
		if err := rows.Scan(&t.Name, &totalRows, &totalBytes, &t.Engine); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		if totalRows != nil {
			t.Rows = *totalRows
		}
		if totalBytes != nil {
			t.BytesSize = *totalBytes
		}
		tables = append(tables, t)
	}

	return tables, rows.Err()
}

// GetDatabaseSize returns the total size of the database
func (c *Client) GetDatabaseSize(ctx context.Context) (uint64, error) {
	var size uint64
	query := `SELECT sum(total_bytes) FROM system.tables WHERE database = currentDatabase()`
	if err := c.conn.QueryRow(ctx, query).Scan(&size); err != nil {
		return 0, fmt.Errorf("failed to get database size: %w", err)
	}
	return size, nil
}
