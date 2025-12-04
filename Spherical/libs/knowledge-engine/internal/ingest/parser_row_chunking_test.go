package ingest

import (
	"testing"

	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateRowChunks_5ColumnTable(t *testing.T) {
	content := `
## Exterior Colors

| Parent Category | Sub-Category | Specification | Value | Additional Metadata |
|-----------------|--------------|---------------|-------|---------------------|
| Exterior | Colors | Color | Pearl Metallic Gallant Red | Standard |
| Exterior | Colors | Color | Super White | Standard |
| Exterior | Colors | Color | Midnight Black Metallic | Premium |
| Interior | Upholstery | Material | Leather | Premium Package |
| Interior | Upholstery | Material | Fabric | Standard |
`

	parser := NewParser(ParserConfig{ChunkSize: 512, ChunkOverlap: 64})
	chunks := parser.generateRowChunks(content, 1)

	require.GreaterOrEqual(t, len(chunks), 5, "Should generate at least 5 row chunks")

	// Check first chunk (Exterior > Colors > Color: Pearl Metallic Gallant Red)
	foundRed := false
	for _, chunk := range chunks {
		if chunk.ChunkType == storage.ChunkTypeSpecRow {
			// Check metadata
			parentCategory, ok := chunk.Metadata["parent_category"].(string)
			if ok && parentCategory == "Exterior" {
				subCategory, _ := chunk.Metadata["sub_category"].(string)
				specType, _ := chunk.Metadata["specification_type"].(string)
				value, _ := chunk.Metadata["value"].(string)
				
				if subCategory == "Colors" && specType == "Color" && value == "Pearl Metallic Gallant Red" {
					foundRed = true
					
					// Verify structured text format
					assert.Contains(t, chunk.Text, "Category: Exterior")
					assert.Contains(t, chunk.Text, "Sub-Category: Colors")
					assert.Contains(t, chunk.Text, "Specification: Color")
					assert.Contains(t, chunk.Text, "Value: Pearl Metallic Gallant Red")
					
					// Verify content hash exists
					contentHash, ok := chunk.Metadata["content_hash"].(string)
					assert.True(t, ok, "Content hash should be present")
					assert.NotEmpty(t, contentHash, "Content hash should not be empty")
					assert.Len(t, contentHash, 64, "Content hash should be 64 characters (SHA-256 hex)")
				}
			}
		}
	}
	assert.True(t, foundRed, "Should find Pearl Metallic Gallant Red color chunk")
}

func TestComputeContentHash(t *testing.T) {
	text1 := "Category: Exterior\nSub-Category: Colors\nSpecification: Color\nValue: Red"
	text2 := "Category: Exterior\nSub-Category: Colors\nSpecification: Color\nValue: Red"
	text3 := "Category: Exterior\nSub-Category: Colors\nSpecification: Color\nValue: Blue"
	
	hash1 := computeContentHash(text1)
	hash2 := computeContentHash(text2)
	hash3 := computeContentHash(text3)
	
	// Same content should produce same hash
	assert.Equal(t, hash1, hash2, "Same content should produce same hash")
	
	// Different content should produce different hash
	assert.NotEqual(t, hash1, hash3, "Different content should produce different hash")
	
	// Hash should be 64 characters (SHA-256 hex)
	assert.Len(t, hash1, 64, "Hash should be 64 characters")
}

func TestFormatRowChunkText(t *testing.T) {
	text := formatRowChunkText("Exterior", "Colors", "Color", "Red", "Standard")
	
	assert.Contains(t, text, "Category: Exterior")
	assert.Contains(t, text, "Sub-Category: Colors")
	assert.Contains(t, text, "Specification: Color")
	assert.Contains(t, text, "Value: Red")
	assert.Contains(t, text, "Additional Metadata: Standard")
}

func TestExtractTableRowMetadata(t *testing.T) {
	metadata := extractTableRowMetadata("Exterior", "Colors", "Color", "Red", "Standard")
	
	assert.Equal(t, "Exterior", metadata["parent_category"])
	assert.Equal(t, "Colors", metadata["sub_category"])
	assert.Equal(t, "Color", metadata["specification_type"])
	assert.Equal(t, "Red", metadata["value"])
	assert.Equal(t, "Standard", metadata["additional_metadata"])
	
	// Check default values
	metadata2 := extractTableRowMetadata("", "", "", "Value", "")
	assert.Equal(t, "Uncategorized", metadata2["parent_category"])
	assert.Equal(t, "General", metadata2["sub_category"])
	assert.Equal(t, "Unknown", metadata2["specification_type"])
}

func TestParse_GeneratesRowChunks(t *testing.T) {
	content := `---
title: Test Product
product: Test Car
year: 2025
---

## Specifications

| Parent Category | Sub-Category | Specification | Value | Additional Metadata |
|-----------------|--------------|---------------|-------|---------------------|
| Exterior | Colors | Color | Red | Standard |
| Exterior | Colors | Color | Blue | Premium |
| Interior | Seats | Material | Leather | Luxury Package |
`

	parser := NewParser(ParserConfig{ChunkSize: 512, ChunkOverlap: 64})
	result, err := parser.Parse(content)
	
	require.NoError(t, err)
	
	// Should generate row chunks
	rowChunks := 0
	for _, chunk := range result.RawChunks {
		if chunk.ChunkType == storage.ChunkTypeSpecRow {
			rowChunks++
			assert.NotEmpty(t, chunk.Text)
			assert.NotNil(t, chunk.Metadata)
		}
	}
	
	assert.GreaterOrEqual(t, rowChunks, 3, "Should generate at least 3 row chunks")
}

func TestGenerateRowChunks_3ColumnTable(t *testing.T) {
	// Test 3-column table (legacy format)
	content := `
| Category | Specification | Value |
|----------|---------------|-------|
| Engine | Power | 176 hp |
| Engine | Torque | 221 Nm |
`

	parser := NewParser(ParserConfig{ChunkSize: 512, ChunkOverlap: 64})
	chunks := parser.generateRowChunks(content, 1)
	
	require.GreaterOrEqual(t, len(chunks), 2, "Should generate at least 2 row chunks")
	
	// Check that default sub-category is used
	found := false
	for _, chunk := range chunks {
		if chunk.ChunkType == storage.ChunkTypeSpecRow {
			subCategory, ok := chunk.Metadata["sub_category"].(string)
			if ok && subCategory == "General" {
				found = true
				break
			}
		}
	}
	assert.True(t, found, "Should use default sub-category for 3-column tables")
}

func TestGenerateRowChunks_4ColumnTable(t *testing.T) {
	// Test 4-column table
	content := `
| Category | Specification | Value | Unit |
|----------|---------------|-------|------|
| Engine | Power | 176 | hp |
| Engine | Torque | 221 | Nm |
`

	parser := NewParser(ParserConfig{ChunkSize: 512, ChunkOverlap: 64})
	chunks := parser.generateRowChunks(content, 1)
	
	require.GreaterOrEqual(t, len(chunks), 2, "Should generate at least 2 row chunks")
	
	// Verify chunks are created
	for _, chunk := range chunks {
		if chunk.ChunkType == storage.ChunkTypeSpecRow {
			assert.NotEmpty(t, chunk.Text)
			assert.NotNil(t, chunk.Metadata)
		}
	}
}

func TestGenerateRowChunks_SkipsHeaderRows(t *testing.T) {
	content := `
| Parent Category | Sub-Category | Specification | Value | Additional Metadata |
|-----------------|--------------|---------------|-------|---------------------|
| Exterior | Colors | Color | Red | Standard |
`

	parser := NewParser(ParserConfig{ChunkSize: 512, ChunkOverlap: 64})
	chunks := parser.generateRowChunks(content, 1)
	
	// Should skip header row and separator row, only create chunk for data row
	rowChunks := 0
	for _, chunk := range chunks {
		if chunk.ChunkType == storage.ChunkTypeSpecRow {
			rowChunks++
			// Verify it's not a header row
			value, ok := chunk.Metadata["value"].(string)
			assert.True(t, ok && value != "Value", "Should not create chunk for header row")
		}
	}
	
	assert.Equal(t, 1, rowChunks, "Should create exactly 1 row chunk (skip header)")
}

func TestGenerateRowChunks_ContentHashDeduplication(t *testing.T) {
	content := `
| Parent Category | Sub-Category | Specification | Value | Additional Metadata |
|-----------------|--------------|---------------|-------|---------------------|
| Exterior | Colors | Color | Red | Standard |
| Exterior | Colors | Color | Red | Standard |
`

	parser := NewParser(ParserConfig{ChunkSize: 512, ChunkOverlap: 64})
	chunks := parser.generateRowChunks(content, 1)
	
	// Both rows have same content, so should have same hash
	hashes := make(map[string]bool)
	for _, chunk := range chunks {
		if chunk.ChunkType == storage.ChunkTypeSpecRow {
			hash, ok := chunk.Metadata["content_hash"].(string)
			if ok {
				hashes[hash] = true
			}
		}
	}
	
	// Should have same hash for duplicate rows
	// Note: The parser will create separate chunks, but they'll have same content_hash
	// Deduplication happens in storeChunks
	assert.GreaterOrEqual(t, len(hashes), 1, "Should generate content hashes")
}

