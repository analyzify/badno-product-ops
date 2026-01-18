package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	DefaultConfigDir  = ".badops"
	DefaultConfigFile = "config.yaml"
)

// Config represents the application configuration
type Config struct {
	Sources   SourcesConfig   `yaml:"sources"`
	Outputs   OutputsConfig   `yaml:"outputs"`
	Database  DatabaseConfig  `yaml:"database,omitempty"`
	Defaults  DefaultsConfig  `yaml:"defaults,omitempty"`
}

// SourcesConfig contains configuration for all source connectors
type SourcesConfig struct {
	Shopify  ShopifySourceConfig  `yaml:"shopify"`
	NOBB     NOBBConfig           `yaml:"nobb"`
	TigerNL  TigerNLConfig        `yaml:"tiger_nl"`
}

// ShopifySourceConfig holds Shopify source settings
type ShopifySourceConfig struct {
	Store     string `yaml:"store"`       // Store name (e.g., "badno")
	APIKeyEnv string `yaml:"api_key_env"` // Environment variable for API key
}

// NOBBConfig holds NOBB settings
type NOBBConfig struct {
	UsernameEnv string `yaml:"username_env"` // Environment variable for username
	PasswordEnv string `yaml:"password_env"` // Environment variable for password
}

// TigerNLConfig holds Tiger.nl settings
type TigerNLConfig struct {
	RateLimitMs int `yaml:"rate_limit_ms"` // Milliseconds between requests
}

// OutputsConfig contains configuration for all output adapters
type OutputsConfig struct {
	Shopify    ShopifyOutputConfig    `yaml:"shopify"`
	ClickHouse ClickHouseConfig       `yaml:"clickhouse"`
	File       FileOutputConfig       `yaml:"file"`
}

// ShopifyOutputConfig holds Shopify output settings
type ShopifyOutputConfig struct {
	Store     string `yaml:"store"`       // Store name
	APIKeyEnv string `yaml:"api_key_env"` // Environment variable for API key
}

// ClickHouseConfig holds ClickHouse settings
type ClickHouseConfig struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	Database    string `yaml:"database"`
	UsernameEnv string `yaml:"username_env"`
	PasswordEnv string `yaml:"password_env"`
	Table       string `yaml:"table"`
	Secure      bool   `yaml:"secure"`
}

// FileOutputConfig holds file output settings
type FileOutputConfig struct {
	OutputDir string `yaml:"output_dir"`
	Pretty    bool   `yaml:"pretty"`
}

// DatabaseConfig holds database connection settings
type DatabaseConfig struct {
	Postgres   PostgresConfig   `yaml:"postgres"`
	ClickHouse ClickHouseDBConfig `yaml:"clickhouse"`
	UseDB      bool             `yaml:"use_db"` // Enable database backend
}

// PostgresConfig holds PostgreSQL settings
type PostgresConfig struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	Database    string `yaml:"database"`
	UsernameEnv string `yaml:"username_env"`
	PasswordEnv string `yaml:"password_env"`
	SSLMode     string `yaml:"ssl_mode"`
}

// ClickHouseDBConfig holds ClickHouse database settings for analytics
type ClickHouseDBConfig struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	Database    string `yaml:"database"`
	UsernameEnv string `yaml:"username_env"`
	PasswordEnv string `yaml:"password_env"`
	Secure      bool   `yaml:"secure"`
}

// DefaultsConfig holds default settings
type DefaultsConfig struct {
	Vendor          string   `yaml:"vendor,omitempty"`           // Default vendor filter
	EnhanceSources  []string `yaml:"enhance_sources,omitempty"`  // Default enhancement sources
	ExportFormat    string   `yaml:"export_format,omitempty"`    // Default export format
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Sources: SourcesConfig{
			Shopify: ShopifySourceConfig{
				Store:     "badno",
				APIKeyEnv: "SHOPIFY_API_KEY",
			},
			NOBB: NOBBConfig{
				UsernameEnv: "NOBB_USERNAME",
				PasswordEnv: "NOBB_PASSWORD",
			},
			TigerNL: TigerNLConfig{
				RateLimitMs: 150,
			},
		},
		Outputs: OutputsConfig{
			Shopify: ShopifyOutputConfig{
				Store:     "badno",
				APIKeyEnv: "SHOPIFY_API_KEY",
			},
			ClickHouse: ClickHouseConfig{
				Host:        "localhost",
				Port:        9000,
				Database:    "products",
				UsernameEnv: "CLICKHOUSE_USERNAME",
				PasswordEnv: "CLICKHOUSE_PASSWORD",
				Table:       "enhanced_products",
			},
			File: FileOutputConfig{
				OutputDir: "./output",
				Pretty:    true,
			},
		},
		Database: DatabaseConfig{
			UseDB: false, // Disabled by default, use JSON state
			Postgres: PostgresConfig{
				Host:        "localhost",
				Port:        5432,
				Database:    "badops",
				UsernameEnv: "POSTGRES_USER",
				PasswordEnv: "POSTGRES_PASSWORD",
				SSLMode:     "prefer",
			},
			ClickHouse: ClickHouseDBConfig{
				Host:        "localhost",
				Port:        9000,
				Database:    "badops",
				UsernameEnv: "CLICKHOUSE_USERNAME",
				PasswordEnv: "CLICKHOUSE_PASSWORD",
				Secure:      false,
			},
		},
		Defaults: DefaultsConfig{
			Vendor:         "Tiger",
			EnhanceSources: []string{"tiger_nl", "nobb"},
			ExportFormat:   "matrixify",
		},
	}
}

// GetConfigPath returns the path to the config file
func GetConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(home, DefaultConfigDir, DefaultConfigFile), nil
}

// Load reads the configuration from the config file
func Load() (*Config, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	return LoadFrom(configPath)
}

// LoadFrom reads the configuration from a specific path
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default config if file doesn't exist
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply defaults for missing values
	applyDefaults(&config)

	return &config, nil
}

// Save writes the configuration to the config file
func Save(config *Config) error {
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	return SaveTo(config, configPath)
}

// SaveTo writes the configuration to a specific path
func SaveTo(config *Config, path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Init creates a new config file with defaults
func Init() error {
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	// Check if file already exists
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("config file already exists: %s", configPath)
	}

	return Save(DefaultConfig())
}

// Exists checks if the config file exists
func Exists() bool {
	configPath, err := GetConfigPath()
	if err != nil {
		return false
	}

	_, err = os.Stat(configPath)
	return err == nil
}

// applyDefaults fills in missing values with defaults
func applyDefaults(config *Config) {
	defaults := DefaultConfig()

	// Sources
	if config.Sources.TigerNL.RateLimitMs <= 0 {
		config.Sources.TigerNL.RateLimitMs = defaults.Sources.TigerNL.RateLimitMs
	}

	// Outputs
	if config.Outputs.ClickHouse.Port == 0 {
		config.Outputs.ClickHouse.Port = defaults.Outputs.ClickHouse.Port
	}
	if config.Outputs.File.OutputDir == "" {
		config.Outputs.File.OutputDir = defaults.Outputs.File.OutputDir
	}
}

// Set updates a specific config value
func Set(key, value string) error {
	config, err := Load()
	if err != nil {
		return err
	}

	switch key {
	case "sources.shopify.store":
		config.Sources.Shopify.Store = value
	case "sources.shopify.api_key_env":
		config.Sources.Shopify.APIKeyEnv = value
	case "sources.nobb.username_env":
		config.Sources.NOBB.UsernameEnv = value
	case "sources.nobb.password_env":
		config.Sources.NOBB.PasswordEnv = value
	case "outputs.file.output_dir":
		config.Outputs.File.OutputDir = value
	case "defaults.vendor":
		config.Defaults.Vendor = value
	case "defaults.export_format":
		config.Defaults.ExportFormat = value
	case "database.use_db":
		config.Database.UseDB = value == "true"
	case "database.postgres.host":
		config.Database.Postgres.Host = value
	case "database.postgres.database":
		config.Database.Postgres.Database = value
	case "database.postgres.username_env":
		config.Database.Postgres.UsernameEnv = value
	case "database.postgres.password_env":
		config.Database.Postgres.PasswordEnv = value
	case "database.clickhouse.host":
		config.Database.ClickHouse.Host = value
	case "database.clickhouse.database":
		config.Database.ClickHouse.Database = value
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}

	return Save(config)
}

// Get retrieves a specific config value
func Get(key string) (string, error) {
	config, err := Load()
	if err != nil {
		return "", err
	}

	switch key {
	case "sources.shopify.store":
		return config.Sources.Shopify.Store, nil
	case "sources.shopify.api_key_env":
		return config.Sources.Shopify.APIKeyEnv, nil
	case "sources.nobb.username_env":
		return config.Sources.NOBB.UsernameEnv, nil
	case "sources.nobb.password_env":
		return config.Sources.NOBB.PasswordEnv, nil
	case "outputs.file.output_dir":
		return config.Outputs.File.OutputDir, nil
	case "defaults.vendor":
		return config.Defaults.Vendor, nil
	case "defaults.export_format":
		return config.Defaults.ExportFormat, nil
	case "database.use_db":
		if config.Database.UseDB {
			return "true", nil
		}
		return "false", nil
	case "database.postgres.host":
		return config.Database.Postgres.Host, nil
	case "database.postgres.database":
		return config.Database.Postgres.Database, nil
	case "database.postgres.username_env":
		return config.Database.Postgres.UsernameEnv, nil
	case "database.postgres.password_env":
		return config.Database.Postgres.PasswordEnv, nil
	case "database.clickhouse.host":
		return config.Database.ClickHouse.Host, nil
	case "database.clickhouse.database":
		return config.Database.ClickHouse.Database, nil
	default:
		return "", fmt.Errorf("unknown config key: %s", key)
	}
}
