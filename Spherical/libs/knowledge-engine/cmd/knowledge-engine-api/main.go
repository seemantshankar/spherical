// Package main provides the Knowledge Engine API server entrypoint.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/config"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
)

func main() {
	// Load configuration
	cfgPath := os.Getenv("CONFIG_PATH")
	if len(os.Args) > 2 && os.Args[1] == "--config" {
		cfgPath = os.Args[2]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger := observability.NewLogger(observability.LogConfig{
		Level:       cfg.Observability.LogLevel,
		Format:      cfg.Observability.LogFormat,
		ServiceName: cfg.Observability.OTEL.ServiceName,
	})

	logger.Info().
		Str("host", cfg.Server.Host).
		Int("port", cfg.Server.Port).
		Str("database", cfg.Database.Driver).
		Str("vector", cfg.Vector.Adapter).
		Msg("Starting Knowledge Engine API")

	// Create app config
	appCfg := &AppConfig{
		RequestTimeout:     cfg.Server.ReadTimeout,
		CacheSize:          cfg.Cache.MaxEntries,
		CacheTTL:           cfg.Cache.TTL,
		MaxChunks:          cfg.Retrieval.MaxChunks,
		MaxConcurrentJobs:  cfg.Ingestion.MaxConcurrentJobs,
		EmbeddingDimension: cfg.Embedding.Dimension,
		DriftCheckInterval: cfg.Drift.CheckInterval,
		StalenessWindow:    cfg.Drift.FreshnessThreshold,
	}

	// Initialize router with all handlers
	router := NewRouter(logger, appCfg, cfg)

	// Create server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start server in goroutine
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info().Str("addr", addr).Msg("HTTP server listening")
		serverErrors <- srv.ListenAndServe()
	}()

	// Wait for interrupt or error
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		logger.Error().Err(err).Msg("Server error")
	case sig := <-shutdown:
		logger.Info().Str("signal", sig.String()).Msg("Shutdown signal received")
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.GracefulShutdown)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("Graceful shutdown failed")
		if err := srv.Close(); err != nil {
			logger.Error().Err(err).Msg("Forced shutdown failed")
		}
	}

	logger.Info().Msg("Server stopped")
}
