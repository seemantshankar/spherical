# Gaps, Ambiguities, and Conflicts - Resolution Summary

**Date**: 2025-11-30  
**Status**: All issues addressed in spec.md

## Conflicts Resolved

### CHK089: FR-011 Format Conflict ✅ RESOLVED

- **Issue**: FR-011 mentioned "Value column, Key Features column, or new Variant Availability column" which conflicted with the clarified 5-column format
- **Resolution**: Updated FR-011 to explicitly state: "The output format MUST use a dedicated 'Variant Availability' column as the 5th column in the specifications table (Category | Specification | Value | Key Features | Variant Availability)"
- **Location**: Spec §FR-011

## Ambiguities Resolved

### CHK090: "Standard" vs "All variants" ✅ RESOLVED

- **Issue**: User Story 4 acceptance scenario mentioned both "Standard" and "All variants"
- **Resolution**: Updated to use only "Standard" (single word) as clarified. Updated User Story 4 acceptance scenario 2 and added FR-015
- **Location**: Spec §User Story 4, §FR-015

### CHK091: Edge Case Questions ✅ RESOLVED

- **Issue**: Edge Cases section contained questions rather than explicit requirements
- **Resolution**: Converted all edge case questions to explicit requirements with functional requirement references
- **Location**: Spec §Edge Cases (now contains explicit requirements)

## Gaps Addressed

### CHK005: Variant Availability Column Empty vs Populated ✅ RESOLVED

- **Issue**: No requirement defined for when Variant Availability column should be empty vs. populated
- **Resolution**: Added FR-017: "The Variant Availability column MUST always be present in the output table (5th column), but MAY be empty for single-trim models or when no variant information is available"
- **Location**: Spec §FR-017

### CHK015: Knowledge Engine 5-Column Format Handling ✅ RESOLVED

- **Issue**: No requirement defined for how knowledge engine should handle the new 5-column format
- **Resolution**: Added COMPAT-003: "The knowledge engine ingestion pipeline MUST accept the new 5-column table format while maintaining backward compatibility with 4-column format for existing data". Also added to Dependencies section with explicit requirement
- **Location**: Spec §Non-Functional Requirements §COMPAT-003, §Dependencies

### CHK019, CHK067-069: Post-Processing Edge Cases ✅ RESOLVED

- **Issue**: No requirements defined for empty codeblocks, nested codeblocks, codeblocks in table cells
- **Resolution**: Added FR-018: "The post-processing function MUST handle edge cases: empty codeblocks (remove entirely, output empty string), nested codeblocks (remove outer delimiters, preserve inner content), and codeblocks in table cells (remove delimiters, preserve cell content)"
- **Location**: Spec §FR-018, §Edge Cases

### CHK078: Ingestion Engine Compatibility ✅ RESOLVED

- **Issue**: No requirement defined for ingestion engine compatibility with 5-column format
- **Resolution**: Added COMPAT-003 and explicit requirement in Dependencies section
- **Location**: Spec §Non-Functional Requirements §COMPAT-003, §Dependencies

### CHK087: Dependency Version/Compatibility Requirements ✅ RESOLVED

- **Issue**: No dependency version/compatibility requirements specified
- **Resolution**: Expanded Dependencies section with version requirements and compatibility notes for each dependency
- **Location**: Spec §Dependencies

### CHK088: Dependency Failure Handling ✅ RESOLVED

- **Issue**: No requirements defined for handling dependency failures or changes
- **Resolution**: Added "Dependency Failure Handling" subsection with explicit requirements for each dependency failure scenario
- **Location**: Spec §Dependencies §Dependency Failure Handling

### CHK092: Standard Nomenclature Definition ✅ RESOLVED

- **Issue**: "standard hierarchical nomenclature" not defined with complete list or reference
- **Resolution**: Expanded FR-003 to include complete standard categories list with examples: Engine, Exterior, Interior, Safety, Performance, Dimensions with subcategories
- **Location**: Spec §FR-003

### CHK093: "Similar Specifications" Definition ✅ RESOLVED

- **Issue**: "similar specifications" in SC-002 not defined
- **Resolution**: Added definition to SC-002: "Similar specifications are defined as specifications describing the same type of feature or component (e.g., all seat upholstery specifications, all engine power specifications, all safety airbag specifications) regardless of the specific values or variant availability"
- **Location**: Spec §SC-002

### CHK094: "Variant-Differentiated Features" Definition ✅ RESOLVED

- **Issue**: "variant-differentiated features" in SC-005 not defined
- **Resolution**: Added definition to SC-005: "Variant-differentiated features are defined as specifications where the feature availability, value, or presence differs between at least two variants (i.e., not available in all variants as 'Standard')"
- **Location**: Spec §SC-005

## Additional Improvements

### Success Criteria Measurability ✅ ENHANCED

- Added measurement methods to all success criteria (SC-001 through SC-006)
- Clarified "100% of extracted markdown files" means "per extraction run"
- Added test set requirements and evaluation methods

### Non-Functional Requirements ✅ ADDED

- Added new section "Non-Functional Requirements" with:
  - Performance Requirements (PERF-001 through PERF-003)
  - Compatibility Requirements (COMPAT-001 through COMPAT-003)
  - Reliability Requirements (REL-001 through REL-002)

### Key Entities Updated ✅ ENHANCED

- Updated Extracted Specification definition to reflect 5-column format
- Clarified Variant Availability column is always present but may be empty

### Post-Processing Requirements ✅ ENHANCED

- Updated FR-002 to specify "regex-based removal as a fallback"
- Added FR-018 for edge case handling
- Added FR-019 for idempotency requirement

## Summary

**Total Issues Addressed**: 15

- **Conflicts**: 1 (CHK089)
- **Ambiguities**: 2 (CHK090, CHK091)
- **Gaps**: 12 (CHK005, CHK015, CHK019, CHK067-069, CHK078, CHK087-088, CHK092-094)

**New Functional Requirements Added**: 5 (FR-014 through FR-019)  
**New Non-Functional Requirements Added**: 8 (PERF-001-003, COMPAT-001-003, REL-001-002)

All identified gaps, ambiguities, and conflicts have been resolved in the specification. The spec is now complete, clear, and ready for implementation.
