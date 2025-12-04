# Priority Recommendations - Knowledge Engine Tasks

Based on analysis of remaining tasks, here are the priority recommendations:

## High Priority (Core Functionality)

### T045 - CLI triggers for recomputing comparisons (US3)
**Status**: In Progress  
**File**: `cmd/knowledge-engine-cli/comparisons.go`  
**Description**: Backfill CLI/ADMIN triggers for recomputing comparisons. This enables administrators to manually trigger comparison recomputation when product data changes.

### T054 - Retention/purge tooling (US4)
**Status**: Pending  
**File**: `cmd/knowledge-engine-cli/purge.go`  
**Description**: Implement retention/purge tooling that deletes tenant data within 30 days and logs audit trails. Critical for compliance and data retention policies.

### T055 - Embedding version guard (US4)
**Status**: Pending  
**File**: `internal/monitoring/embedding_guard.go`  
**Description**: Detect embedding model version mismatches and queue re-embedding jobs so mixed vectors are never queried together. Critical for data integrity.

## Medium Priority (Testing & Reporting)

### T057 - CLI drift report command (US4)
**Status**: Pending  
**File**: `cmd/knowledge-engine-cli/drift_report.go`  
**Description**: Add CLI drift report command summarizing open alerts for analysts. Useful for operational monitoring.

### Integration Tests
- **T027** [US2] Add audit logging integration test for retrieval requests
- **T040** [US3] Add integration test for comparison materializer job
- **T041** [US3] Add audit logging integration test for comparison requests
- **T049** [US4] Add integration test covering drift detection, purge flow, and embedding-version guardrails

## Lower Priority (Security Hardening)

### T060 - Security hardening
**Status**: Pending  
**File**: `cmd/knowledge-engine-api/middleware/auth.go`  
**Description**: Harden security (OAuth2 scopes, mTLS verification, tenancy guards). Can be done later as part of production hardening.

---

## Implementation Order

1. **T045** - Enables comparison recomputation (core functionality)
2. **T055** - Prevents data integrity issues (critical guard)
3. **T054** - Compliance requirement (critical for production)
4. **T057** - Operational visibility (nice to have)
5. Integration tests - Quality assurance
6. T060 - Production hardening

