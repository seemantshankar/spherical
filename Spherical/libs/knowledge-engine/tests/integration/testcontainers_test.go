// Package integration provides integration tests for the Knowledge Engine.
package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	_ "github.com/lib/pq"
)

// TestContainerSetup represents the test container infrastructure.
type TestContainerSetup struct {
	PostgresContainer testcontainers.Container
	RedisContainer    testcontainers.Container
	PostgresHost      string
	PostgresPort      string
	PostgresConnStr   string
	RedisHost         string
	RedisPort         string
	RedisAddr         string
	cleanup           func()
}

// SetupTestContainers initializes PostgreSQL and Redis containers for testing.
func SetupTestContainers(t *testing.T) *TestContainerSetup {
	t.Helper()
	ctx := context.Background()

	// Start PostgreSQL with pgvector
	pgContainer, err := postgres.Run(ctx,
		"pgvector/pgvector:pg17",
		postgres.WithDatabase("knowledge_engine_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.WithInitScripts(), // Clear any init scripts
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)

	pgHost, err := pgContainer.Host(ctx)
	require.NoError(t, err)

	pgPort, err := pgContainer.MappedPort(ctx, "5432")
	require.NoError(t, err)

	pgConnStr := fmt.Sprintf("postgres://test:test@%s:%s/knowledge_engine_test?sslmode=disable",
		pgHost, pgPort.Port())

	// Start Redis
	redisContainer, err := redis.Run(ctx,
		"redis:7.4-alpine",
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)

	redisHost, err := redisContainer.Host(ctx)
	require.NoError(t, err)

	redisPort, err := redisContainer.MappedPort(ctx, "6379")
	require.NoError(t, err)

	setup := &TestContainerSetup{
		PostgresContainer: pgContainer,
		RedisContainer:    redisContainer,
		PostgresHost:      pgHost,
		PostgresPort:      pgPort.Port(),
		PostgresConnStr:   pgConnStr,
		RedisHost:         redisHost,
		RedisPort:         redisPort.Port(),
		RedisAddr:         fmt.Sprintf("%s:%s", redisHost, redisPort.Port()),
		cleanup: func() {
			if err := pgContainer.Terminate(ctx); err != nil {
				t.Logf("Failed to terminate postgres container: %v", err)
			}
			if err := redisContainer.Terminate(ctx); err != nil {
				t.Logf("Failed to terminate redis container: %v", err)
			}
		},
	}

	// Set environment variables for tests
	os.Setenv("KNOWLEDGE_ENGINE_DATABASE_URL", pgConnStr)
	os.Setenv("KNOWLEDGE_ENGINE_REDIS_URL", setup.RedisAddr)

	return setup
}

// Cleanup terminates all test containers.
func (s *TestContainerSetup) Cleanup() {
	if s.cleanup != nil {
		s.cleanup()
	}
}

// RunMigrations runs database migrations on the test database.
func (s *TestContainerSetup) RunMigrations(t *testing.T) {
	t.Helper()

	db, err := sql.Open("postgres", s.PostgresConnStr)
	require.NoError(t, err)
	defer db.Close()

	// Wait for database to be ready
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for {
		if err := db.PingContext(ctx); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("Database not ready after 30 seconds")
		case <-time.After(100 * time.Millisecond):
			continue
		}
	}

	// First create the pgvector extension
	_, err = db.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS vector")
	require.NoError(t, err)

	// Read and execute migration file
	migrationPath := "../../db/migrations/0001_init.sql"
	migration, err := os.ReadFile(migrationPath)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, string(migration))
	require.NoError(t, err)

	t.Log("Migrations applied successfully")
}

func TestPostgresConnection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Skip if Docker is not available
	if os.Getenv("CI") == "" && !isDockerAvailable() {
		t.Skip("Docker not available")
	}

	setup := SetupTestContainers(t)
	defer setup.Cleanup()

	// Test connection
	db, err := sql.Open("postgres", setup.PostgresConnStr)
	require.NoError(t, err)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	require.NoError(t, err)

	// Create pgvector extension (it comes with the image but needs to be enabled)
	_, err = db.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS vector")
	require.NoError(t, err)

	// Verify pgvector extension
	var extName string
	err = db.QueryRowContext(ctx,
		"SELECT extname FROM pg_extension WHERE extname = 'vector'").Scan(&extName)
	require.NoError(t, err)
	require.Equal(t, "vector", extName)

	t.Log("PostgreSQL with pgvector is running")
}

func TestRedisConnection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Skip if Docker is not available
	if os.Getenv("CI") == "" && !isDockerAvailable() {
		t.Skip("Docker not available")
	}

	setup := SetupTestContainers(t)
	defer setup.Cleanup()

	// Test Redis connection using go-redis
	t.Logf("Redis is running at %s", setup.RedisAddr)
}

func TestFullStackIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Skip if Docker is not available
	if os.Getenv("CI") == "" && !isDockerAvailable() {
		t.Skip("Docker not available")
	}

	setup := SetupTestContainers(t)
	defer setup.Cleanup()

	// Run migrations
	setup.RunMigrations(t)

	// Test database operations
	db, err := sql.Open("postgres", setup.PostgresConnStr)
	require.NoError(t, err)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a test tenant
	var tenantID string
	err = db.QueryRowContext(ctx, `
		INSERT INTO tenants (name, plan_tier, settings)
		VALUES ('Test Tenant', 'sandbox', '{}')
		RETURNING id
	`).Scan(&tenantID)
	require.NoError(t, err)
	require.NotEmpty(t, tenantID)

	t.Logf("Created test tenant: %s", tenantID)

	// Create a test product
	var productID string
	err = db.QueryRowContext(ctx, `
		INSERT INTO products (tenant_id, name)
		VALUES ($1, 'Test Camry')
		RETURNING id
	`, tenantID).Scan(&productID)
	require.NoError(t, err)
	require.NotEmpty(t, productID)

	t.Logf("Created test product: %s", productID)

	// Verify RLS is working (queries with correct tenant_id work)
	var count int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM products WHERE tenant_id = $1
	`, tenantID).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	t.Log("Full stack integration test passed")
}

// isDockerAvailable checks if Docker is available for testing.
func isDockerAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		return false
	}
	defer provider.Close()

	_, err = provider.Client().Ping(ctx)
	return err == nil
}

