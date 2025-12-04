// Package config provides unified configuration loading for the Knowledge Engine.
// Supports YAML files, environment variables, and programmatic overrides.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the Knowledge Engine.
type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Database      DatabaseConfig      `yaml:"database"`
	Vector        VectorConfig        `yaml:"vector"`
	Cache         CacheConfig         `yaml:"cache"`
	Embedding     EmbeddingConfig     `yaml:"embedding"`
	Retrieval     RetrievalConfig     `yaml:"retrieval"`
	Ingestion     IngestionConfig     `yaml:"ingestion"`
	Comparison    ComparisonConfig    `yaml:"comparison"`
	Drift         DriftConfig         `yaml:"drift"`
	Observability ObservabilityConfig `yaml:"observability"`
	Auth          AuthConfig          `yaml:"auth"`
	Tenancy       TenancyConfig       `yaml:"tenancy"`
}

// ServerConfig holds HTTP/gRPC server settings.
type ServerConfig struct {
	Host             string        `yaml:"host"`
	Port             int           `yaml:"port"`
	ReadTimeout      time.Duration `yaml:"read_timeout"`
	WriteTimeout     time.Duration `yaml:"write_timeout"`
	IdleTimeout      time.Duration `yaml:"idle_timeout"`
	GracefulShutdown time.Duration `yaml:"graceful_shutdown"`
}

// DatabaseConfig holds database connection settings.
type DatabaseConfig struct {
	Driver   string         `yaml:"driver"` // sqlite or postgres
	SQLite   SQLiteConfig   `yaml:"sqlite"`
	Postgres PostgresConfig `yaml:"postgres"`
}

// SQLiteConfig holds SQLite-specific settings.
type SQLiteConfig struct {
	Path         string `yaml:"path"`
	MaxOpenConns int    `yaml:"max_open_conns"`
	JournalMode  string `yaml:"journal_mode"`
}

// PostgresConfig holds Postgres-specific settings.
type PostgresConfig struct {
	DSN             string        `yaml:"dsn"`
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

// VectorConfig holds vector store settings.
type VectorConfig struct {
	Adapter  string         `yaml:"adapter"` // faiss or pgvector
	FAISS    FAISSConfig    `yaml:"faiss"`
	PGVector PGVectorConfig `yaml:"pgvector"`
}

// FAISSConfig holds FAISS-specific settings.
type FAISSConfig struct {
	IndexPath string `yaml:"index_path"`
	Dimension int    `yaml:"dimension"`
	NList     int    `yaml:"nlist"`
}

// PGVectorConfig holds PGVector-specific settings.
type PGVectorConfig struct {
	IndexType string `yaml:"index_type"`
	Lists     int    `yaml:"lists"`
}

// CacheConfig holds cache settings.
type CacheConfig struct {
	Driver     string        `yaml:"driver"` // memory or redis
	TTL        time.Duration `yaml:"ttl"`
	MaxEntries int           `yaml:"max_entries"`
	Redis      RedisConfig   `yaml:"redis"`
}

// RedisConfig holds Redis-specific settings.
type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	PoolSize int    `yaml:"pool_size"`
}

// EmbeddingConfig holds embedding model settings.
type EmbeddingConfig struct {
	Model     string `yaml:"model"`
	Dimension int    `yaml:"dimension"`
	BatchSize int    `yaml:"batch_size"`
}

// RetrievalConfig holds retrieval settings.
type RetrievalConfig struct {
	MaxChunks                  int     `yaml:"max_chunks"`
	StructuredFirst            bool    `yaml:"structured_first"`
	SemanticFallback           bool    `yaml:"semantic_fallback"`
	IntentConfidenceThreshold  float64 `yaml:"intent_confidence_threshold"`
	CacheResults               bool    `yaml:"cache_results"`
}

// IngestionConfig holds ingestion pipeline settings.
type IngestionConfig struct {
	PDFExtractorPath   string  `yaml:"pdf_extractor_path"`
	MaxConcurrentJobs  int     `yaml:"max_concurrent_jobs"`
	ChunkSize          int     `yaml:"chunk_size"`
	ChunkOverlap       int     `yaml:"chunk_overlap"`
	DedupeThreshold    float64 `yaml:"dedupe_threshold"`
	EmbeddingBatchSize int     `yaml:"embedding_batch_size"`
}

// ComparisonConfig holds comparison service settings.
type ComparisonConfig struct {
	MaxDimensions   int           `yaml:"max_dimensions"`
	RefreshInterval time.Duration `yaml:"refresh_interval"`
	StaleThreshold  time.Duration `yaml:"stale_threshold"`
}

// DriftConfig holds drift monitoring settings.
type DriftConfig struct {
	CheckInterval      time.Duration `yaml:"check_interval"`
	FreshnessThreshold time.Duration `yaml:"freshness_threshold"`
	AlertChannel       string        `yaml:"alert_channel"`
}

// ObservabilityConfig holds logging and tracing settings.
type ObservabilityConfig struct {
	LogLevel  string     `yaml:"log_level"`
	LogFormat string     `yaml:"log_format"`
	OTEL      OTELConfig `yaml:"otel"`
}

// OTELConfig holds OpenTelemetry settings.
type OTELConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Endpoint    string `yaml:"endpoint"`
	ServiceName string `yaml:"service_name"`
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	Enabled bool        `yaml:"enabled"`
	OAuth2  OAuth2Config `yaml:"oauth2"`
	MTLS    MTLSConfig   `yaml:"mtls"`
}

// OAuth2Config holds OAuth2 settings.
type OAuth2Config struct {
	Issuer   string `yaml:"issuer"`
	Audience string `yaml:"audience"`
}

// MTLSConfig holds mTLS settings.
type MTLSConfig struct {
	Enabled    bool   `yaml:"enabled"`
	CACert     string `yaml:"ca_cert"`
	ServerCert string `yaml:"server_cert"`
	ServerKey  string `yaml:"server_key"`
}

// TenancyConfig holds multi-tenancy settings.
type TenancyConfig struct {
	DefaultTenant string `yaml:"default_tenant"`
	IsolationMode string `yaml:"isolation_mode"` // row_level or schema
}

// Load reads configuration from a YAML file and applies environment overrides.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config file: %w", err)
		}

		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config file: %w", err)
		}
	}

	applyEnvOverrides(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

// DefaultConfig returns a configuration with sensible defaults for development.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:             "0.0.0.0",
			Port:             8085,
			ReadTimeout:      30 * time.Second,
			WriteTimeout:     30 * time.Second,
			IdleTimeout:      120 * time.Second,
			GracefulShutdown: 10 * time.Second,
		},
		Database: DatabaseConfig{
			Driver: "sqlite",
			SQLite: SQLiteConfig{
				Path:         "/tmp/knowledge-engine.db",
				MaxOpenConns: 1,
				JournalMode:  "WAL",
			},
			Postgres: PostgresConfig{
				MaxOpenConns:    25,
				MaxIdleConns:    5,
				ConnMaxLifetime: 5 * time.Minute,
			},
		},
		Vector: VectorConfig{
			Adapter: "faiss",
			FAISS: FAISSConfig{
				IndexPath: "/tmp/knowledge-engine.faiss",
				Dimension: 768,
				NList:     100,
			},
			PGVector: PGVectorConfig{
				IndexType: "ivfflat",
				Lists:     100,
			},
		},
		Cache: CacheConfig{
			Driver:     "memory",
			TTL:        5 * time.Minute,
			MaxEntries: 10000,
			Redis: RedisConfig{
				Addr:     "localhost:6380",
				DB:       0,
				PoolSize: 10,
			},
		},
		Embedding: EmbeddingConfig{
			Model:     "qwen/qwen3-embedding-8b",
			Dimension: 768,
			BatchSize: 100,
		},
		Retrieval: RetrievalConfig{
			MaxChunks:                  8,
			StructuredFirst:            true,
			SemanticFallback:           true,
			IntentConfidenceThreshold:  0.7,
			CacheResults:               true,
		},
		Ingestion: IngestionConfig{
			PDFExtractorPath:   "../pdf-extractor/cmd/pdf-extractor",
			MaxConcurrentJobs:  2,
			ChunkSize:          512,
			ChunkOverlap:       64,
			DedupeThreshold:    0.95,
			EmbeddingBatchSize: 75, // Default batch size for embedding generation
		},
		Comparison: ComparisonConfig{
			MaxDimensions:   20,
			RefreshInterval: 24 * time.Hour,
			StaleThreshold:  7 * 24 * time.Hour,
		},
		Drift: DriftConfig{
			CheckInterval:      24 * time.Hour,
			FreshnessThreshold: 180 * 24 * time.Hour,
			AlertChannel:       "drift.alerts",
		},
		Observability: ObservabilityConfig{
			LogLevel:  "debug",
			LogFormat: "json",
			OTEL: OTELConfig{
				Enabled:     false,
				Endpoint:    "http://localhost:4317",
				ServiceName: "knowledge-engine",
			},
		},
		Auth: AuthConfig{
			Enabled: false,
			OAuth2: OAuth2Config{
				Issuer:   "https://auth.spherical.local",
				Audience: "knowledge-engine",
			},
		},
		Tenancy: TenancyConfig{
			DefaultTenant: "dev",
			IsolationMode: "row_level",
		},
	}
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	if c.Database.Driver != "sqlite" && c.Database.Driver != "postgres" {
		return fmt.Errorf("invalid database driver: %s", c.Database.Driver)
	}

	if c.Vector.Adapter != "faiss" && c.Vector.Adapter != "pgvector" {
		return fmt.Errorf("invalid vector adapter: %s", c.Vector.Adapter)
	}

	if c.Cache.Driver != "memory" && c.Cache.Driver != "redis" {
		return fmt.Errorf("invalid cache driver: %s", c.Cache.Driver)
	}

	if c.Retrieval.MaxChunks < 1 || c.Retrieval.MaxChunks > 20 {
		return fmt.Errorf("max_chunks must be between 1 and 20")
	}

	return nil
}

// IsDevelopment returns true if running in development mode.
func (c *Config) IsDevelopment() bool {
	return c.Database.Driver == "sqlite" || !c.Auth.Enabled
}

// DatabaseDSN returns the appropriate database connection string.
func (c *Config) DatabaseDSN() string {
	if c.Database.Driver == "sqlite" {
		return c.Database.SQLite.Path
	}
	return c.Database.Postgres.DSN
}

// applyEnvOverrides applies environment variable overrides to config.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("SERVER_PORT"); v != "" {
		var port int
		if _, err := fmt.Sscanf(v, "%d", &port); err == nil {
			cfg.Server.Port = port
		}
	}

	if v := os.Getenv("SERVER_HOST"); v != "" {
		cfg.Server.Host = v
	}

	if v := os.Getenv("DATABASE_URL"); v != "" {
		if strings.HasPrefix(v, "sqlite:") {
			cfg.Database.Driver = "sqlite"
			cfg.Database.SQLite.Path = strings.TrimPrefix(v, "sqlite:")
		} else if strings.HasPrefix(v, "postgres") {
			cfg.Database.Driver = "postgres"
			cfg.Database.Postgres.DSN = v
		}
	}

	if v := os.Getenv("POSTGRES_URL"); v != "" {
		cfg.Database.Postgres.DSN = v
	}

	if v := os.Getenv("REDIS_URL"); v != "" {
		cfg.Cache.Driver = "redis"
		// Parse redis://host:port format
		addr := strings.TrimPrefix(v, "redis://")
		cfg.Cache.Redis.Addr = addr
	}

	if v := os.Getenv("VECTOR_ADAPTER"); v != "" {
		cfg.Vector.Adapter = v
	}

	if v := os.Getenv("FAISS_INDEX_PATH"); v != "" {
		cfg.Vector.FAISS.IndexPath = v
	}

	if v := os.Getenv("EMBEDDING_MODEL"); v != "" {
		cfg.Embedding.Model = v
	}

	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.Observability.LogLevel = v
	}

	if v := os.Getenv("LOG_FORMAT"); v != "" {
		cfg.Observability.LogFormat = v
	}

	if v := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); v != "" {
		cfg.Observability.OTEL.Endpoint = v
		cfg.Observability.OTEL.Enabled = true
	}

	if v := os.Getenv("OTEL_SERVICE_NAME"); v != "" {
		cfg.Observability.OTEL.ServiceName = v
	}

	if v := os.Getenv("AUTH_ENABLED"); v == "true" {
		cfg.Auth.Enabled = true
	}

	if v := os.Getenv("OAUTH2_ISSUER"); v != "" {
		cfg.Auth.OAuth2.Issuer = v
	}

	if v := os.Getenv("PDF_EXTRACTOR_PATH"); v != "" {
		cfg.Ingestion.PDFExtractorPath = v
	}
}

// ResolveRelativePath resolves a path relative to the config file location.
func ResolveRelativePath(configPath, targetPath string) string {
	if filepath.IsAbs(targetPath) {
		return targetPath
	}
	configDir := filepath.Dir(configPath)
	return filepath.Join(configDir, targetPath)
}

