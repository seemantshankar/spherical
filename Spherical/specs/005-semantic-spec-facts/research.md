# Research

## Findings

- Decision: Use existing knowledge-engine LLM integration to generate single-sentence, field-bounded explanations; enforce prompt guardrails and capture failures per row.  
  Rationale: Reuses established provider/config; minimizes new dependencies while meeting safety needs.  
  Alternatives considered: New provider (adds integration cost); multi-sentence outputs (violates spec display constraint); skipping explanations on failure (reduces recall/value).

- Decision: Store enriched spec_fact chunks and explanations in Postgres with pgvector (existing embedding store), mirroring SQLite for dev; deduplicate on (category, name, value, variant) to avoid redundant embeddings.  
  Rationale: Aligns with current storage and embedding infra; avoids new services.  
  Alternatives considered: External vector DB (adds ops surface); storing only raw fields (hurts semantic recall); relying solely on keyword index (fails goal).

- Decision: Trigger semantic fallback when keyword confidence is low or empty, returning structured facts with explanations; keep keyword path primary.  
  Rationale: Meets requirement to avoid zero-results while preserving precision for high-confidence keywords.  
  Alternatives considered: Always semantic-first (risk of irrelevance); manual synonym patches (not scalable); hybrid without confidence gating (unpredictable ranking).

- Decision: On LLM timeout/error, record an explicit failure marker and continue ingest; allow retry path without blocking batch completion.  
  Rationale: Keeps ingest resilient and traceable.  
  Alternatives considered: Hard fail batch (availability risk); silently skip explanations (debug pain).

## Open Questions Resolved

None pending; all clarifications addressed in decisions above.
