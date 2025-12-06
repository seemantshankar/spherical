// Package query provides query orchestration.
package query

import (
	"context"
	"fmt"

	keretrieval "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
)

// Orchestrator handles query operations.
type Orchestrator struct {
	router *keretrieval.Router
}

// NewOrchestrator creates a new query orchestrator.
func NewOrchestrator(router *keretrieval.Router) *Orchestrator {
	return &Orchestrator{
		router: router,
	}
}

// QueryResult represents the result of a query operation.
type QueryResult struct {
	Intent          keretrieval.Intent
	StructuredFacts []keretrieval.SpecFact
	SemanticChunks  []keretrieval.SemanticChunk
	LatencyMs       int64
}

// Query executes a query against the knowledge base.
func (o *Orchestrator) Query(ctx context.Context, req keretrieval.RetrievalRequest) (*QueryResult, error) {
	resp, err := o.router.Query(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	
	return &QueryResult{
		Intent:          resp.Intent,
		StructuredFacts: resp.StructuredFacts,
		SemanticChunks:  resp.SemanticChunks,
		LatencyMs:       resp.LatencyMs,
	}, nil
}

