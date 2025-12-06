// Package config provides configuration management for the Orchestrator.
// It loads environment variables, orchestrator-specific settings, and integrates
// with the knowledge-engine configuration system.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	keconfig "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/config"
)

// Config holds all configuration for the Orchestrator.
type Config struct {
	Orchestrator   OrchestratorConfig `yaml:"orchestrator"`
	KnowledgeEngine *keconfig.Config  `yaml:"knowledge_engine"`
	Extraction     ExtractionConfig   `yaml:"extraction"`
	Ingestion      IngestionConfig    `yaml:"ingestion"`
}

// OrchestratorConfig holds orchestrator-specific settings.
type OrchestratorConfig struct {
	VectorStoreRoot string `yaml:"vector_store_root"`
	TempDir         string `yaml:"temp_dir"`
	BinDir          string `yaml:"bin_dir"`
	RepoRoot        string `yaml:"repo_root"`     // Root of the spherical repository
	DataDir         string `yaml:"data_dir"`      // Persistent data directory
}

// ExtractionConfig holds PDF extraction settings.
type ExtractionConfig struct {
	Model string `yaml:"model"` // LLM model override
}

// IngestionConfig holds ingestion-specific settings.
type IngestionConfig struct {
	EmbeddingBatchSize int  `yaml:"embedding_batch_size"`
	AutoPublish        bool `yaml:"auto_publish"`
}

// Load reads configuration from environment variables and optional config file.
func Load(knowledgeEngineConfigPath string) (*Config, error) {
	// Load .env file if it exists (ignore error if not found)
	_ = godotenv.Load()
	
	// Also try loading from current directory and parent directories
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")
	_ = godotenv.Load("../../.env")

	cfg := DefaultConfig()

	// Load knowledge-engine config
	var keCfg *keconfig.Config
	var err error
	
	if knowledgeEngineConfigPath != "" {
		keCfg, err = keconfig.Load(knowledgeEngineConfigPath)
		if err != nil {
			return nil, fmt.Errorf("load knowledge-engine config: %w", err)
		}
	} else {
		// Try to find default config file
		possiblePaths := []string{
			"configs/dev.yaml",
			"../knowledge-engine/configs/dev.yaml",
			"../../knowledge-engine/configs/dev.yaml",
			"libs/knowledge-engine/configs/dev.yaml",
		}
		
		for _, path := range possiblePaths {
			if _, err := os.Stat(path); err == nil {
				keCfg, err = keconfig.Load(path)
				if err == nil {
					break
				}
			}
		}
		
		// If still no config found, use default
		if keCfg == nil {
			keCfg = keconfig.DefaultConfig()
		}
	}
	
	cfg.KnowledgeEngine = keCfg

	// Apply environment variable overrides
	applyEnvOverrides(cfg)

	// Ensure persistent paths are set (override temp paths if needed)
	ensurePersistentPaths(cfg)

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	// Determine repository root
	repoRoot := findRepoRoot()
	
	// Determine persistent data directory
	dataDir := getPersistentDataDir(repoRoot)
	
	return &Config{
		Orchestrator: OrchestratorConfig{
			VectorStoreRoot: filepath.Join(dataDir, "vector-stores"),
			TempDir:         filepath.Join(os.TempDir(), "orchestrator-temp"),
			BinDir:          filepath.Join(repoRoot, "libs/orchestrator/bin"),
			RepoRoot:        repoRoot,
			DataDir:         dataDir,
		},
		Extraction: ExtractionConfig{
			Model: "google/gemini-2.5-flash-preview-09-2025",
		},
		Ingestion: IngestionConfig{
			EmbeddingBatchSize: 75,
			AutoPublish:        false,
		},
	}
}

// getPersistentDataDir returns a persistent data directory path.
// Priority:
// 1. $ORCHESTRATOR_DATA_DIR environment variable
// 2. $HOME/.orchestrator (user's home directory)
// 3. ./data (relative to repo root)
func getPersistentDataDir(repoRoot string) string {
	// Check environment variable first
	if dataDir := os.Getenv("ORCHESTRATOR_DATA_DIR"); dataDir != "" {
		return dataDir
	}
	
	// Try user's home directory
	if homeDir, err := os.UserHomeDir(); err == nil {
		orchestratorHome := filepath.Join(homeDir, ".orchestrator")
		// Create directory if it doesn't exist
		_ = os.MkdirAll(orchestratorHome, 0755)
		return orchestratorHome
	}
	
	// Fallback to repo-relative data directory
	dataDir := filepath.Join(repoRoot, "data")
	_ = os.MkdirAll(dataDir, 0755)
	return dataDir
}

// ensurePersistentPaths ensures database and vector store paths are persistent (not in /tmp).
func ensurePersistentPaths(cfg *Config) {
	dataDir := cfg.Orchestrator.DataDir
	
	// Ensure vector store root is persistent
	if strings.HasPrefix(cfg.Orchestrator.VectorStoreRoot, "/tmp") ||
		strings.HasPrefix(cfg.Orchestrator.VectorStoreRoot, os.TempDir()) {
		cfg.Orchestrator.VectorStoreRoot = filepath.Join(dataDir, "vector-stores")
		// Create directory
		_ = os.MkdirAll(cfg.Orchestrator.VectorStoreRoot, 0755)
	}
	
	// Ensure database path is persistent (for SQLite)
	if cfg.KnowledgeEngine != nil && cfg.KnowledgeEngine.Database.Driver == "sqlite" {
		dbPath := cfg.KnowledgeEngine.Database.SQLite.Path
		if strings.HasPrefix(dbPath, "/tmp") || strings.HasPrefix(dbPath, os.TempDir()) {
			// Move database to persistent location
			cfg.KnowledgeEngine.Database.SQLite.Path = filepath.Join(dataDir, "knowledge-engine.db")
			// Create directory if needed
			_ = os.MkdirAll(filepath.Dir(cfg.KnowledgeEngine.Database.SQLite.Path), 0755)
		}
	}
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	if c.KnowledgeEngine == nil {
		return fmt.Errorf("knowledge-engine config is required")
	}

	// Validate orchestrator config
	if c.Orchestrator.VectorStoreRoot == "" {
		return fmt.Errorf("vector_store_root is required")
	}

	if c.Orchestrator.TempDir == "" {
		return fmt.Errorf("temp_dir is required")
	}

	if c.Orchestrator.BinDir == "" {
		return fmt.Errorf("bin_dir is required")
	}

	// Validate knowledge-engine config
	if err := c.KnowledgeEngine.Validate(); err != nil {
		return fmt.Errorf("knowledge-engine config validation failed: %w", err)
	}

	return nil
}

// GetOpenRouterAPIKey returns the OpenRouter API key from environment.
func (c *Config) GetOpenRouterAPIKey() (string, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENROUTER_API_KEY environment variable is not set")
	}
	return apiKey, nil
}

// GetLLMModel returns the LLM model to use, with fallback priority:
// 1. ExtractionConfig.Model (orchestrator config)
// 2. LLM_MODEL environment variable
// 3. Default model
func (c *Config) GetLLMModel() string {
	if model := os.Getenv("LLM_MODEL"); model != "" {
		return model
	}
	if c.Extraction.Model != "" {
		return c.Extraction.Model
	}
	return "google/gemini-2.5-flash-preview-09-2025"
}

// GetKnowledgeEngineDBPath returns the database path from knowledge-engine config.
func (c *Config) GetKnowledgeEngineDBPath() string {
	return c.KnowledgeEngine.DatabaseDSN()
}

// applyEnvOverrides applies environment variable overrides to config.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("ORCHESTRATOR_VECTOR_STORE_ROOT"); v != "" {
		cfg.Orchestrator.VectorStoreRoot = v
	}

	if v := os.Getenv("ORCHESTRATOR_TEMP_DIR"); v != "" {
		cfg.Orchestrator.TempDir = v
	}

	if v := os.Getenv("ORCHESTRATOR_BIN_DIR"); v != "" {
		cfg.Orchestrator.BinDir = v
	}

	if v := os.Getenv("ORCHESTRATOR_DATA_DIR"); v != "" {
		cfg.Orchestrator.DataDir = v
	}

	if v := os.Getenv("KNOWLEDGE_ENGINE_CONFIG"); v != "" {
		// This will be handled in Load() function
	}

	if v := os.Getenv("KNOWLEDGE_ENGINE_DB_PATH"); v != "" {
		if cfg.KnowledgeEngine.Database.Driver == "sqlite" {
			cfg.KnowledgeEngine.Database.SQLite.Path = v
		}
	}

	if v := os.Getenv("EXTRACTION_MODEL"); v != "" {
		cfg.Extraction.Model = v
	}

	if v := os.Getenv("INGESTION_EMBEDDING_BATCH_SIZE"); v != "" {
		var batchSize int
		if _, err := fmt.Sscanf(v, "%d", &batchSize); err == nil && batchSize > 0 {
			cfg.Ingestion.EmbeddingBatchSize = batchSize
		}
	}
}

// findRepoRoot attempts to find the repository root by looking for
// common repository markers (like .git, or libs/ directory).
func findRepoRoot() string {
	// Start from current working directory
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}

	// Walk up the directory tree looking for markers
	for {
		// Check for .git directory
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}

		// Check for libs/ directory (spherical-specific)
		if _, err := os.Stat(filepath.Join(dir, "libs")); err == nil {
			return dir
		}

		// Check if we've reached the filesystem root
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Fallback: try relative paths from common locations
	if cwd, err := os.Getwd(); err == nil {
		// Try to go up from libs/orchestrator
		if filepath.Base(cwd) == "orchestrator" {
			return filepath.Join(cwd, "../..")
		}
		if filepath.Base(cwd) == "libs" {
			return filepath.Join(cwd, "..")
		}
	}

	return "."
}
