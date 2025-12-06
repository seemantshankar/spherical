package commands

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	
	orcconfig "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/config"
)

// openDatabase opens a database connection using the knowledge-engine config.
func openDatabase(cfg *orcconfig.Config) (*sql.DB, error) {
	keCfg := cfg.KnowledgeEngine
	if keCfg == nil {
		return nil, fmt.Errorf("knowledge-engine config is required")
	}
	
	dsn := keCfg.DatabaseDSN()
	
	var driver string
	if keCfg.Database.Driver == "sqlite" {
		driver = "sqlite3"
	} else if keCfg.Database.Driver == "postgres" {
		driver = "postgres"
		return nil, fmt.Errorf("postgres driver not yet implemented in orchestrator")
	} else {
		return nil, fmt.Errorf("unsupported database driver: %s", keCfg.Database.Driver)
	}
	
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	
	// Set connection pool settings
	if keCfg.Database.Driver == "sqlite" {
		db.SetMaxOpenConns(keCfg.Database.SQLite.MaxOpenConns)
	}
	
	return db, nil
}

// resolveTenantID resolves a tenant ID or name to a UUID.
func resolveTenantID(idOrName string) (uuid.UUID, error) {
	if idOrName == "" {
		return uuid.Nil, fmt.Errorf("empty tenant ID or name")
	}
	
	// Try to parse as UUID
	if id, err := uuid.Parse(idOrName); err == nil {
		return id, nil
	}
	
	// If not a UUID, generate a deterministic UUID from name (for dev/testing)
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(idOrName)), nil
}

// getDefaultTenantID gets the default tenant ID from config or environment.
func getDefaultTenantID(cfg *orcconfig.Config) (uuid.UUID, error) {
	defaultTenant := os.Getenv("DEFAULT_TENANT")
	if defaultTenant == "" {
		defaultTenant = cfg.KnowledgeEngine.Tenancy.DefaultTenant
	}
	if defaultTenant == "" {
		defaultTenant = "dev"
	}
	
	return resolveTenantID(defaultTenant)
}

// findMigrationDir finds the knowledge-engine migration directory.
func findMigrationDir(cfg *orcconfig.Config) string {
	possiblePaths := []string{
		"../knowledge-engine/db/migrations",
		"../../knowledge-engine/db/migrations",
		"libs/knowledge-engine/db/migrations",
		filepath.Join(cfg.Orchestrator.RepoRoot, "libs/knowledge-engine/db/migrations"),
	}
	
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	
	// Default fallback
	return filepath.Join(cfg.Orchestrator.RepoRoot, "libs/knowledge-engine/db/migrations")
}

