# Feature Specification: Fix PDF Extractor Output Quality

**Feature Branch**: `003-fix-pdf-extractor`  
**Created**: 2025-11-30  
**Status**: Draft  
**Input**: User description: "the pdf-spec-extractor has the following problems that I want to fix - (Use @libs/pdf-extractor/camry-output-v3.md and '/Users/seemant/Documents/Projects/AIOutcallingAgent/Uploads/Skoda Kodiaq 2025 Brochure.pdf' as reference)

1. The output .md file has codeblocks '''markdown, etc. that should not be there. I want to generate a markdown file without any codeblocks as it interferes with the ingestion engine.

2. The markdown is not using standard terms like Interior > seats > upholstry. At the moment, the datapoints are not properly tagged or the LLM is creating its own nomenclature. I want the LLM to use a nomenclature that is easy to understand and is standard, do not just go by the section names in the brochure, the LLM should use its own understanding of how to un-complicate the tags to standards that anyone can understand.

3. Some automobile models may have variants and trims that are currently not being extracted as they are not properly tagged inside the brochure. The variants and trims are generally found in specification tables (See Page 25-28 in '/Users/seemant/Documents/Projects/AIOutcallingAgent/Uploads/Skoda Kodiaq 2025 Brochure.pdf' where the header row mentions Lounge, Sportline, and Selection L&K) or mentioned in features that are unique to some variants (Eg. Page 22 in '/Users/seemant/Documents/Projects/AIOutcallingAgent/Uploads/Skoda Kodiaq 2025 Brochure.pdf' where it says \"Exclusive to L&K and Exclusive to Sportline).

4. When parsing specification tables (Eg. Pages 25-28 in '/Users/seemant/Documents/Projects/AIOutcallingAgent/Uploads/Skoda Kodiaq 2025 Brochure.pdf'), pay close attention to the checkboxes, etc. that differentiate features between variants and trims. This information needs to be captured in the markdown this is one of the most asked questions by users. Think of modifying the LLM prompt to capture this in the most effective way possible. This is extremely important!"

## Clarifications

### Session 2025-01-27

- Q: What is the exact markdown format for variant availability information in specification tables? → A: Add a dedicated "Variant Availability" column as the 5th column to the specifications table (making it 5 columns total: Category | Specification | Value | Key Features | Variant Availability)
- Q: What is the scope of the standardized hierarchical nomenclature guide? → A: Provide major categories with common subcategories as examples, allow LLM to infer additional ones. Use variable depth (2-4 levels) based on semantic meaning: deeper hierarchies (3-4 levels) for complex features requiring granularity (e.g., Interior > Seats > Upholstery > Material), shallower hierarchies (2 levels) for simpler features (e.g., Engine > Torque). The LLM should use semantic understanding to determine appropriate depth, ensuring similar features use consistent depth across brochures.
- Q: What is the post-processing approach for removing codeblock delimiters? → A: Multi-pass approach: first attempt LLM prompt fix (FR-001), then regex-based fallback removal (FR-002) to handle any codeblock delimiters that may have been generated despite prompt instructions
- Q: What format should be used in the Variant Availability column for features available in all variants? → A: Use "Standard" (single word) in the Variant Availability column for features that are available in all variants
- Q: How should the system handle cases where the LLM cannot clearly identify variant boundaries in complex tables? → A: Extract what can be identified with confidence, mark ambiguous cases (e.g., "Variant Availability: Unknown"), and continue processing. This preserves extractable data, avoids false positives, and allows downstream validation while maintaining pipeline throughput.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Clean Markdown Output Without Codeblocks (Priority: P1)

Users need the extracted markdown files to be clean and directly ingestible by the knowledge engine without manual cleanup. Currently, the LLM sometimes outputs markdown wrapped in code blocks (```markdown ... ```), which breaks ingestion and requires manual intervention.

**Why this priority**: This is a blocking issue that prevents automated ingestion workflows. Without clean markdown, the entire extraction pipeline fails downstream.

**Independent Test**: Can be fully tested by extracting a PDF that previously produced codeblock-wrapped output and verifying the resulting markdown file contains no codeblock delimiters (```) and is directly parseable by the ingestion engine.

**Acceptance Scenarios**:

1. **Given** a PDF brochure is processed, **When** the LLM generates markdown output, **Then** the output contains no markdown codeblock delimiters (```) anywhere in the file
2. **Given** the extracted markdown file, **When** it is passed to the ingestion engine, **Then** it parses successfully without errors related to codeblock syntax
3. **Given** a page with no extractable content, **When** the LLM processes it, **Then** it outputs nothing (empty string) rather than an empty codeblock

---

### User Story 2 - Standard Hierarchical Nomenclature (Priority: P1)

Users need consistent, standard nomenclature for specification categories that follows a logical hierarchy (e.g., Interior > Seats > Upholstery) rather than ad-hoc terms created by the LLM based on brochure section names.

**Why this priority**: Inconsistent nomenclature makes it difficult to query and compare specifications across different vehicle models. Standard terms improve searchability and data quality.

**Independent Test**: Can be fully tested by extracting specifications from multiple brochures and verifying that similar features use consistent hierarchical category paths (e.g., all seat-related specs use "Interior > Seats" as the category prefix).

**Acceptance Scenarios**:

1. **Given** a brochure contains seat upholstery information, **When** it is extracted, **Then** it uses the category path "Interior > Seats > Upholstery" (or equivalent standard hierarchy with appropriate depth) rather than brochure-specific terms
2. **Given** specifications are extracted from multiple brochures, **When** similar features are compared, **Then** they use consistent category nomenclature regardless of how the brochure labels them
3. **Given** a brochure uses non-standard section names, **When** the LLM extracts specifications, **Then** it maps them to standard hierarchical categories with appropriate depth (2-4 levels) based on semantic understanding rather than literal section names, ensuring similar features use consistent depth

---

### User Story 3 - Extract Variant and Trim Information (Priority: P2)

Users need to know which variants and trims are available for a vehicle model, and which features are exclusive to specific variants. This information is critical for comparison queries.

**Why this priority**: Variant/trim differentiation is one of the most frequently asked questions by users. Without this information, users cannot accurately compare options.

**Independent Test**: Can be fully tested by processing a brochure with variant specification tables (like the Skoda Kodiaq pages 25-28) and verifying that all variant names (Lounge, Sportline, Selection L&K) are extracted and associated with their respective features.

**Acceptance Scenarios**:

1. **Given** a specification table with variant headers (e.g., Lounge, Sportline, Selection L&K), **When** the table is processed, **Then** all variant names are extracted and included in the output
2. **Given** a page mentions "Exclusive to [Variant Name]", **When** features are extracted, **Then** they are tagged with the variant name to indicate exclusivity
3. **Given** multiple variants exist in a brochure, **When** specifications are extracted, **Then** each specification row includes variant information when the feature differs between variants

---

### User Story 4 - Capture Variant Differentiation in Specification Tables (Priority: P1)

Users need detailed information about which features are available in which variants, especially when presented in table format with checkboxes or symbols indicating variant availability.

**Why this priority**: This is explicitly called out as "extremely important" and "one of the most asked questions by users." Without this, users cannot make informed decisions about variant selection.

**Independent Test**: Can be fully tested by processing specification tables with checkbox/symbol indicators (like Skoda Kodiaq pages 25-28) and verifying that the output clearly shows which variants have which features.

**Acceptance Scenarios**:

1. **Given** a specification table with checkboxes/symbols indicating variant availability, **When** the table is processed, **Then** each feature row includes variant availability information in the dedicated "Variant Availability" column (5th column) using format like "Lounge: ✓, Sportline: ✓, Selection L&K: ✗" or "Available in: Lounge, Sportline, Selection L&K"
2. **Given** a feature is available in all variants, **When** it is extracted, **Then** the Variant Availability column contains "Standard" rather than listing each variant individually
3. **Given** a feature is exclusive to one variant, **When** it is extracted, **Then** it clearly indicates the exclusive variant (e.g., "Exclusive to: Selection L&K")
4. **Given** a specification table spans multiple pages, **When** variants are extracted, **Then** variant information is consistently maintained across all pages of the table
5. **Given** a complex specification table where variant boundaries are ambiguous, **When** the LLM processes it, **Then** it extracts what can be identified with confidence, marks ambiguous cases as "Unknown" in the Variant Availability column, and continues processing without halting

---

### Edge Cases

- **Non-standard symbols**: When a brochure uses non-standard symbols for variant differentiation (not just checkboxes), the LLM MUST attempt to interpret them based on context and include them in the Variant Availability column
- **Merged/spanning columns**: When specification tables have variant columns that are merged or span multiple rows, the LLM MUST maintain context and associate the correct variant information with each specification row
- **Inconsistent variant names**: When variant names appear in different languages or formats across pages, the LLM MUST extract them as presented (normalization is out of scope per Out of Scope section)
- **No variant information**: When brochures have no variant information (single trim models), the system MUST handle this gracefully without generating false variant data (FR-013)
- **Ambiguous variant boundaries**: When the LLM cannot clearly identify variant boundaries in complex tables, it MUST extract what can be identified with confidence, mark ambiguous cases as "Unknown" in the Variant Availability column, and continue processing (FR-016)
- **Text-only variant information**: When variant information is only in text form (not in tables), the LLM MUST extract variant-exclusive features from text mentions (FR-007) and include them in the Variant Availability column

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The LLM prompt MUST explicitly instruct the model to never output markdown codeblock delimiters (```) in the extracted content (primary prevention)
- **FR-002**: The extraction service MUST post-process LLM output using regex-based removal as a fallback to remove any codeblock delimiters (```, ```markdown, etc.) that may have been generated despite prompt instructions
- **FR-003**: The LLM prompt MUST include a standardized hierarchical nomenclature guide providing major categories (Interior, Exterior, Engine, Safety, Dimensions, etc.) with common subcategories as examples, while allowing the LLM to infer additional categories when needed
- **FR-004**: The LLM MUST use semantic understanding to map brochure-specific section names to standard hierarchical categories rather than using literal section names
- **FR-014**: The LLM MUST use variable depth hierarchies (2-4 levels) based on semantic meaning: use deeper hierarchies (3-4 levels) for complex features requiring granularity (e.g., "Interior > Seats > Upholstery > Material"), and shallower hierarchies (2 levels) for simpler features (e.g., "Engine > Torque"). Similar features MUST use consistent depth across different brochures
- **FR-005**: The LLM prompt MUST explicitly instruct the model to identify and extract all variant and trim names from specification tables and feature descriptions
- **FR-006**: The LLM MUST extract variant names from table headers when present (e.g., columns labeled "Lounge", "Sportline", "Selection L&K")
- **FR-007**: The LLM MUST identify variant-exclusive features from text mentions (e.g., "Exclusive to L&K", "Available only in Sportline")
- **FR-008**: The LLM prompt MUST include detailed instructions for parsing specification tables with checkbox/symbol indicators for variant differentiation
- **FR-009**: The extracted markdown MUST include variant availability information for each specification that differs between variants
- **FR-010**: The LLM MUST capture checkbox/symbol patterns (✓, ✗, ●, ○, etc.) that indicate feature availability per variant
- **FR-011**: The output format MUST use a dedicated "Variant Availability" column as the 5th column in the specifications table (Category | Specification | Value | Key Features | Variant Availability) to clearly indicate which variants have which features
- **FR-015**: For features available in all variants, the Variant Availability column MUST contain "Standard" (single word) rather than listing each variant individually
- **FR-012**: The LLM MUST handle multi-page specification tables and maintain variant context across pages
- **FR-013**: The system MUST handle cases where variant information is missing (single trim models) without generating false variant data
- **FR-016**: When the LLM cannot clearly identify variant boundaries in complex tables, it MUST extract what can be identified with confidence, mark ambiguous cases as "Unknown" in the Variant Availability column, and continue processing rather than skipping the entire table or halting

### Key Entities *(include if feature involves data)*

- **Extracted Specification**: A single specification row with Category, Specification name, Value, Key Features, and Variant Availability (5-column format: Category | Specification | Value | Key Features | Variant Availability)
- **Variant/Trim**: A specific model configuration (e.g., "Lounge", "Sportline", "Selection L&K") that may have unique features or specifications
- **Variant Availability**: Information about which variants include a specific feature, represented as a list of variant names or availability indicators

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of extracted markdown files contain zero markdown codeblock delimiters (```) in the output content
- **SC-002**: 95% of similar specifications across different brochures use consistent hierarchical category nomenclature (e.g., all seat-related specs use "Interior > Seats" prefix)
- **SC-003**: 90% of brochures with variant specification tables have all variant names correctly extracted and associated with their features
- **SC-004**: 85% of specification rows that differ between variants correctly include variant availability information in the extracted markdown
- **SC-005**: Users can successfully query "which variants have [feature]" and receive accurate results for 90% of variant-differentiated features
- **SC-006**: The ingestion engine successfully parses 100% of extracted markdown files without syntax errors related to codeblocks or malformed markdown

## Assumptions

1. The LLM (via OpenRouter) is capable of understanding and following detailed prompt instructions about output format and nomenclature
2. Brochures consistently use table formats with headers for variant specification tables (though symbols/checkboxes may vary)
3. Variant names are typically mentioned in table headers, feature descriptions, or both
4. The ingestion engine expects clean markdown without codeblock delimiters
5. Standard hierarchical nomenclature can be defined for common automobile specification categories (Interior, Exterior, Engine, Safety, etc.)

## Dependencies

- Existing LLM client implementation (`libs/pdf-extractor/internal/llm/client.go`)
- Existing extraction service (`libs/pdf-extractor/internal/extract/service.go`)
- Knowledge engine ingestion pipeline (must accept clean markdown without codeblocks)

## Out of Scope

- Automatic translation of variant names to a standard format (e.g., "L&K" vs "Laurin & Klement")
- Validation of variant name consistency across different pages
- Creation of a comprehensive variant comparison matrix (extraction only, not comparison)
- Post-processing to normalize variant names across different brochures

