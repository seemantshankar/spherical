// Package startup provides startup checks and initialization.
package startup

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MigrationManager handles database migration checks and execution.
type MigrationManager struct {
	db          *sql.DB
	migrationDir string
	driver      string // "sqlite" or "postgres"
}

// NewMigrationManager creates a new migration manager.
func NewMigrationManager(db *sql.DB, migrationDir string, driver string) *MigrationManager {
	return &MigrationManager{
		db:           db,
		migrationDir: migrationDir,
		driver:       driver,
	}
}

// MigrationStatus represents the status of migrations.
type MigrationStatus struct {
	UpToDate   bool
	Pending    []string
	Current    string
	Total      int
}

// CheckMigrations checks the current migration status.
func (m *MigrationManager) CheckMigrations(ctx context.Context) (*MigrationStatus, error) {
	status := &MigrationStatus{
		Pending: []string{},
	}
	
	// Ensure schema_migrations table exists so we can record applied versions
	if err := m.ensureSchemaMigrationsTable(ctx); err != nil {
		return nil, fmt.Errorf("ensure schema_migrations table: %w", err)
	}

	// Get all migration files
	migrations, err := m.listMigrationFiles()
	if err != nil {
		return nil, fmt.Errorf("list migration files: %w", err)
	}
	
	status.Total = len(migrations)
	
	if len(migrations) == 0 {
		status.UpToDate = true
		return status, nil
	}
	
	// Check current migration version
	currentVersion, err := m.getCurrentVersion(ctx)
	if err != nil {
		// Database might not be initialized yet
		status.Pending = migrations
		return status, nil
	}
	
	status.Current = currentVersion
	
	// Find pending migrations
	for _, migration := range migrations {
		if migration > currentVersion {
			status.Pending = append(status.Pending, migration)
		}
	}
	
	status.UpToDate = len(status.Pending) == 0
	return status, nil
}

// RunMigrations runs all pending migrations.
func (m *MigrationManager) RunMigrations(ctx context.Context, status *MigrationStatus) error {
	if len(status.Pending) == 0 {
		return nil
	}
	
	// Sort pending migrations to ensure order
	sort.Strings(status.Pending)
	
	for _, migration := range status.Pending {
		migrationPath := filepath.Join(m.migrationDir, migration)
		if err := m.runMigration(ctx, migrationPath); err != nil {
			return fmt.Errorf("run migration %s: %w", migration, err)
		}
	}
	
	return nil
}

// ensureSchemaMigrationsTable creates the schema_migrations table if it doesn't exist.
func (m *MigrationManager) ensureSchemaMigrationsTable(ctx context.Context) error {
	var query string
	switch m.driver {
	case "sqlite", "":
		query = `
			CREATE TABLE IF NOT EXISTS schema_migrations (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				version TEXT UNIQUE NOT NULL,
				applied_at TEXT NOT NULL DEFAULT (datetime('now'))
			);
		`
	default:
		query = `
			CREATE TABLE IF NOT EXISTS schema_migrations (
				id SERIAL PRIMARY KEY,
				version TEXT UNIQUE NOT NULL,
				applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
			);
		`
	}
	_, err := m.db.ExecContext(ctx, query)
	return err
}

// listMigrationFiles lists all migration files in the migration directory, filtered by database driver.
func (m *MigrationManager) listMigrationFiles() ([]string, error) {
	entries, err := os.ReadDir(m.migrationDir)
	if err != nil {
		return nil, fmt.Errorf("read migration directory: %w", err)
	}
	
	// Collect all migration files
	sqliteMigrations := make(map[string]string) // base name -> sqlite filename
	regularMigrations := make(map[string]string) // base name -> regular filename
	
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		
		name := entry.Name()
		
		// Check if it's a SQLite-specific migration
		if strings.HasSuffix(name, "_sqlite.sql") {
			// Extract base name (e.g., "0001_init" from "0001_init_sqlite.sql")
			baseName := strings.TrimSuffix(name, "_sqlite.sql")
			sqliteMigrations[baseName] = name
		} else {
			// Regular migration file
			baseName := strings.TrimSuffix(name, ".sql")
			regularMigrations[baseName] = name
		}
	}
	
	// Collect all base names
	allBaseNames := make(map[string]bool)
	for baseName := range sqliteMigrations {
		allBaseNames[baseName] = true
	}
	for baseName := range regularMigrations {
		allBaseNames[baseName] = true
	}
	
	// Filter based on driver
	var migrations []string
	for baseName := range allBaseNames {
		if m.driver == "sqlite" {
			// For SQLite, prefer _sqlite.sql version if it exists, otherwise use regular .sql
			if sqliteFile, exists := sqliteMigrations[baseName]; exists {
				migrations = append(migrations, sqliteFile)
			} else if regularFile, exists := regularMigrations[baseName]; exists {
				migrations = append(migrations, regularFile)
			}
		} else {
			// For PostgreSQL, use regular .sql files (ignore _sqlite.sql files)
			if regularFile, exists := regularMigrations[baseName]; exists {
				migrations = append(migrations, regularFile)
			}
		}
	}
	
	return migrations, nil
}

// getCurrentVersion gets the current migration version from the database.
func (m *MigrationManager) getCurrentVersion(ctx context.Context) (string, error) {
	// Check if migrations table exists
	var exists bool
	checkQuery := `
		SELECT EXISTS (
			SELECT 1 FROM sqlite_master 
			WHERE type='table' AND name='schema_migrations'
		)
	`
	
	err := m.db.QueryRowContext(ctx, checkQuery).Scan(&exists)
	if err != nil {
		// Table doesn't exist - assume no migrations run
		return "", nil
	}
	
	if !exists {
		return "", nil
	}
	
	// Get current version
	var version string
	err = m.db.QueryRowContext(ctx, "SELECT version FROM schema_migrations ORDER BY id DESC LIMIT 1").Scan(&version)
	if err == sql.ErrNoRows {
		return "", nil
	}
	
	return version, err
}

// runMigration executes a single migration file.
func (m *MigrationManager) runMigration(ctx context.Context, migrationPath string) error {
	data, err := os.ReadFile(migrationPath)
	if err != nil {
		return fmt.Errorf("read migration file: %w", err)
	}
	
	migrationName := filepath.Base(migrationPath)
	sqlContent := string(data)
	
	// For SQLite, we need special handling for ALTER TABLE ADD COLUMN
	// since SQLite doesn't support IF NOT EXISTS for ADD COLUMN
	if m.driver == "sqlite" {
		// Check if this migration contains ALTER TABLE ADD COLUMN statements
		// If so, we need to split and handle duplicate column errors
		hasAlterTable := strings.Contains(strings.ToUpper(sqlContent), "ALTER TABLE") &&
			strings.Contains(strings.ToUpper(sqlContent), "ADD COLUMN")
		
		if hasAlterTable {
			// Split migration into individual statements for ALTER TABLE handling
			statements := splitSQLStatements(sqlContent)
			
			for _, stmt := range statements {
				stmt = strings.TrimSpace(stmt)
				if stmt == "" || strings.HasPrefix(stmt, "--") {
					continue
				}
				
				// Execute the statement
				_, err = m.db.ExecContext(ctx, stmt)
				if err != nil {
					// If it's a "duplicate column" error, ignore it (column already exists)
					// This makes the migration idempotent
					errStr := strings.ToLower(err.Error())
					if strings.Contains(errStr, "duplicate column") || 
					   strings.Contains(errStr, "duplicate column name") {
						// Column already exists, skip this statement
						continue
					}
					return fmt.Errorf("execute migration: %w", err)
				}
			}
		} else {
			// For migrations without ALTER TABLE, execute as a single block
			// This is important for triggers and views that need to be kept together
			_, err = m.db.ExecContext(ctx, sqlContent)
			if err != nil {
				return fmt.Errorf("execute migration: %w", err)
			}
		}
	} else {
		// For PostgreSQL, execute normally (it supports IF NOT EXISTS)
		_, err = m.db.ExecContext(ctx, sqlContent)
		if err != nil {
			return fmt.Errorf("execute migration: %w", err)
		}
	}
	
	// Record migration version (if schema_migrations table exists)
	_, _ = m.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO schema_migrations (version, applied_at) 
		VALUES (?, datetime('now'))
	`, migrationName)
	
	return nil
}

// splitSQLStatements splits SQL text into individual statements by semicolon.
// It properly handles triggers with BEGIN...END blocks and strings.
func splitSQLStatements(sql string) []string {
	var statements []string
	var current strings.Builder
	inString := false
	stringChar := byte(0)
	inTrigger := false
	beginCount := 0
	
	upperSQL := strings.ToUpper(sql)
	
	for i := 0; i < len(sql); i++ {
		char := sql[i]
		current.WriteByte(char)
		
		if !inString {
			// Check for BEGIN (start of trigger body or transaction)
			if i+5 < len(upperSQL) && upperSQL[i:i+5] == "BEGIN" {
				// Check if it's a standalone BEGIN (not part of a word)
				if i == 0 || !isAlphanumeric(sql[i-1]) {
					// Check if next char is space or newline (not part of a word)
					if i+5 < len(sql) && (sql[i+5] == ' ' || sql[i+5] == '\n' || sql[i+5] == '\t') {
						inTrigger = true
						beginCount++
					}
				}
			}
			
			// Check for END (end of trigger body)
			if inTrigger && i+3 < len(upperSQL) && upperSQL[i:i+3] == "END" {
				// Check if it's a standalone END
				if i == 0 || !isAlphanumeric(sql[i-1]) {
					if i+3 < len(sql) && (sql[i+3] == ';' || sql[i+3] == ' ' || sql[i+3] == '\n') {
						beginCount--
						if beginCount == 0 {
							inTrigger = false
						}
					}
				}
			}
			
			if char == '\'' || char == '"' {
				inString = true
				stringChar = char
			} else if char == ';' && !inTrigger {
				// Only split on semicolon if we're not inside a trigger
				stmt := strings.TrimSpace(current.String())
				if stmt != "" && !strings.HasPrefix(stmt, "--") {
					statements = append(statements, stmt)
				}
				current.Reset()
			}
		} else {
			if char == stringChar && i > 0 && sql[i-1] != '\\' {
				inString = false
			}
		}
	}
	
	// Add remaining statement
	stmt := strings.TrimSpace(current.String())
	if stmt != "" && !strings.HasPrefix(stmt, "--") {
		statements = append(statements, stmt)
	}
	
	return statements
}

// isAlphanumeric checks if a byte is alphanumeric or underscore.
func isAlphanumeric(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

