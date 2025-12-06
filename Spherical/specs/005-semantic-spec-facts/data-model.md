# Data Model

## Entities

### spec_values (table)
- id (uuid) — primary key
- category (text)
- name (text)
- value (text)
- unit (text, nullable)
- key_features (text, nullable)
- variant_availability (text, nullable)
- explanation (text, nullable) — one-sentence, field-bounded summary. NULL indicates generation failed (see FR-004). Note: "explanation" is the primary user-facing text stored per spec row; distinct from optional "gloss" in spec_fact_chunks.
- explanation_failed (boolean, default false) — flag indicating explanation generation failed after retries
- created_at / updated_at (timestamps)

Validation/Rules:
- explanation must be <= 1 sentence (max 200 characters, ends with single punctuation, no line breaks) and derived only from fields in the row (see FR-002 for definition).
- value may carry unit; ensure consistent formatting before explanation generation.
- Missing optional fields must not produce hallucinated content.
- If explanation is NULL, explanation_failed must be true (indicates LLM failure after retries).

### spec_fact_chunks (derived/store for embeddings)
- id (uuid)
- spec_value_id (fk → spec_values.id)
- chunk_text (text) — formatted: "Category > Name: Value [unit]; Key features: ...; Availability: ...; Gloss: ..." (see FR-003 for exact format specification)
- gloss (text, nullable) — optional one-sentence clarifying text included in chunk_text when it helps clarify meaning. Note: "gloss" is distinct from "explanation" - explanation is stored in spec_values.explanation and is required; gloss is optional additional context in the chunk_text.
- embedding (vector)
- source (enum: ingest)
- created_at / updated_at

Validation/Rules:
- chunk_text must include category > name, value (with unit), key features (if any), variant availability, and gloss when present.
- **Deduplication Rules**: Embeddings must be deduplicated using exact match on composite key (category, name, value, variant_availability). All four fields must match exactly (case-sensitive string comparison, no normalization or fuzzy matching). If a duplicate is detected during ingest, reuse the existing embedding record, update its timestamp, and do not create a new chunk. The deduplication check must occur before embedding generation to avoid unnecessary LLM/embedding API calls.

### retrieval result (response shape)
- fact_id (spec_value_id)
- category
- name
- value
- unit (if available)
- key_features (optional)
- variant_availability (optional)
- explanation (single sentence)
- confidence (keyword or semantic score)
- provenance (keyword|semantic)

## Relationships
- spec_fact_chunks.spec_value_id → spec_values.id (1:1 or 1:many if multiple gloss variants; prefer 1:1).
- retrieval results reference spec_values and/or spec_fact_chunks for provenance.

## Notes
- Keep migrations in Postgres and SQLite aligned for explanation column and any embedding table adjustments.
- Ensure spec view includes explanation for downstream rendering.
