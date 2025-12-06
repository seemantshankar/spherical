# Testing Guide for Orchestrator CLI

This guide provides step-by-step instructions for testing the orchestrator CLI.

> **Note**: The orchestrator is now part of the `knowledge-engine` module. All build commands and paths reference `libs/knowledge-engine` instead of `libs/orchestrator`. The orchestrator binary is built and run from the `libs/knowledge-engine` directory.

## Prerequisites

### 1. Environment Setup

#### Required Environment Variables

Create a `.env` file in the repository root (`/Users/seemant/Documents/Projects/spherical/Spherical/.env`) or in the knowledge-engine directory:

```bash
# Required
OPENROUTER_API_KEY=sk-or-your-api-key-here

# Optional (has defaults)
LLM_MODEL=google/gemini-2.5-flash-preview-09-2025
KNOWLEDGE_ENGINE_DB_PATH=~/.orchestrator/knowledge-engine.db
```

#### System Requirements

- **Go 1.25.0+**: Ensure Go is installed and `go version` shows 1.25.0 or later
- **MuPDF Libraries**: Required for PDF extraction (see pdf-extractor README)
  - macOS: `brew install mupdf`
  - Ubuntu/Debian: `sudo apt-get install libmupdf-dev`
- **Access to OpenRouter API**: For LLM operations

### 2. Build the CLI

From the knowledge-engine directory:

```bash
cd libs/knowledge-engine
go build -o orchestrator ./cmd/orchestrator
```

Or build from repository root:

```bash
cd /Users/seemant/Documents/Projects/spherical/Spherical
go build -o libs/knowledge-engine/orchestrator ./libs/knowledge-engine/cmd/orchestrator
```

Verify the build:

```bash
cd libs/knowledge-engine
./orchestrator --help
```

## Testing Workflows

### Test 1: Interactive Menu (Full Flow)

This is the main user workflow:

```bash
cd libs/knowledge-engine
./orchestrator start
```

**What to expect:**

1. **Startup Checks** (Automatic):
   - ✓ Environment check (API key verification)
   - ✓ Database connection check
   - ✓ Migration check (runs migrations if needed)
   - ✓ CLI tools check (builds pdf-extractor and knowledge-engine-cli if needed)

2. **Main Menu** appears with 5 options

3. **Test Creating a Campaign** (Option 2):
   - Select "2. Create New Campaign"
   - Enter PDF file path (e.g., `../pdf-extractor/camry.pdf`)
   - Watch extraction progress
   - Complete metadata if prompted
   - Watch ingestion progress
   - Optionally query immediately after

4. **Test Querying** (Option 1):
   - Select "1. Query Existing Campaign"
   - Select a campaign from the list
   - Enter questions interactively
   - Type "quit" to exit

5. **Test Deleting Campaign** (Option 3):
   - Select "3. Delete Campaign"
   - Select a campaign to delete
   - Confirm deletion

### Test 2: Standalone Extract Command

Test PDF extraction only:

```bash
cd libs/knowledge-engine

# Extract PDF (use a test PDF from pdf-extractor directory)
./orchestrator extract \
  --pdf ../pdf-extractor/camry.pdf \
  --output /tmp/test-extraction.md

# Verify output
cat /tmp/test-extraction.md
```

**Expected Result:**
- Markdown file created with extracted content
- Metadata extracted (make, model, year, etc.)
- Progress indicators shown during extraction

### Test 3: Standalone Ingest Command

Test ingestion only (requires an existing campaign):

```bash
cd libs/knowledge-engine

# First, get a campaign ID from interactive mode or database
CAMPAIGN_ID="<campaign-uuid>"
MARKDOWN_FILE="/tmp/test-extraction.md"

./orchestrator ingest \
  --campaign "$CAMPAIGN_ID" \
  --markdown "$MARKDOWN_FILE"
```

**Expected Result:**
- Content ingested into knowledge base
- Vector store updated
- Summary showing specs, features, USPs, chunks created

### Test 4: Standalone Query Command

Test querying (single question):

```bash
cd libs/knowledge-engine

CAMPAIGN_ID="<campaign-uuid>"

# Single query
./orchestrator query \
  --campaign "$CAMPAIGN_ID" \
  --question "What colors are available?"

# Interactive query mode (no --question flag)
./orchestrator query --campaign "$CAMPAIGN_ID"
```

**Expected Result:**
- Query results displayed with structured facts
- Semantic chunks shown with similarity scores
- Confidence percentages displayed

### Test 5: Command-Line Flags

Test all available flags:

```bash
cd libs/knowledge-engine

# Help for root command
./orchestrator --help

# Help for specific commands
./orchestrator extract --help
./orchestrator ingest --help
./orchestrator query --help
./orchestrator start --help

# Verbose mode
./orchestrator --verbose start

# No color output
./orchestrator --no-color start

# Custom config file (from knowledge-engine configs directory)
./orchestrator --config configs/dev.yaml start
```

## Quick Test Script

Create a test script to verify basic functionality:

```bash
#!/bin/bash
# quick-test.sh

set -e

cd "$(dirname "$0")/../knowledge-engine"

echo "🔨 Building orchestrator..."
go build -o orchestrator ./cmd/orchestrator

echo ""
echo "✅ Build successful!"
echo ""
echo "📋 Available commands:"
./orchestrator --help

echo ""
echo "📋 Extract command help:"
./orchestrator extract --help

echo ""
echo "✅ Basic verification complete!"
echo ""
echo "To test interactively, run:"
echo "  cd libs/knowledge-engine"
echo "  ./orchestrator start"
```

Run it from the orchestrator directory (the script will navigate to knowledge-engine):

```bash
cd libs/orchestrator
chmod +x quick-test.sh
./quick-test.sh
```

Or run directly from knowledge-engine:

```bash
cd libs/knowledge-engine
go build -o orchestrator ./cmd/orchestrator
./orchestrator --help
```

Or run it from the knowledge-engine directory:

```bash
cd libs/knowledge-engine
go build -o orchestrator ./cmd/orchestrator
./orchestrator --help
```

## Testing Checklist

### ✅ Basic Functionality

- [ ] CLI builds successfully
- [ ] `--help` flag works for all commands
- [ ] Environment variables load correctly
- [ ] Configuration file loading works

### ✅ Startup Checks

- [ ] Environment check (API key verification)
- [ ] Database connection established
- [ ] Migrations run automatically if needed
- [ ] CLI binaries built/updated if needed

### ✅ Interactive Menu

- [ ] Main menu displays correctly
- [ ] All 5 menu options accessible
- [ ] Can exit gracefully (Option 5)

### ✅ Create Campaign Flow

- [ ] Can create new campaign
- [ ] PDF extraction works
- [ ] Progress indicators display
- [ ] Metadata completion prompts work
- [ ] Ingestion completes successfully
- [ ] Vector store created

### ✅ Query Flow

- [ ] Can list existing campaigns
- [ ] Campaign selection works
- [ ] Query mode enters successfully
- [ ] Questions can be asked
- [ ] Results display correctly
- [ ] Can exit query mode

### ✅ Standalone Commands

- [ ] Extract command works independently
- [ ] Ingest command works independently
- [ ] Query command works independently
- [ ] All flags work correctly

### ✅ Error Handling

- [ ] Missing API key shows helpful error
- [ ] Invalid PDF path shows helpful error
- [ ] Database connection errors are clear
- [ ] Invalid campaign ID shows helpful error

## Common Test Scenarios

### Scenario 1: First-Time User

```bash
# 1. Build CLI
cd libs/knowledge-engine
go build -o orchestrator ./cmd/orchestrator

# 2. Set up environment
export OPENROUTER_API_KEY="your-key-here"
# Or create .env file in repository root or knowledge-engine directory

# 3. Start interactive mode
./orchestrator start

# 4. Create first campaign
# - Select option 2
# - Provide PDF path
# - Complete metadata
# - Watch extraction and ingestion

# 5. Query the campaign
# - Select option 1
# - Select the campaign
# - Ask questions
```

### Scenario 2: Quick Extraction Test

```bash
cd libs/knowledge-engine

# Extract a PDF quickly
./orchestrator extract \
  --pdf ../pdf-extractor/testdata/sample.pdf \
  --output /tmp/test.md

# Check the output
head -50 /tmp/test.md
```

### Scenario 3: End-to-End Workflow

```bash
cd libs/knowledge-engine

# 1. Extract PDF
./orchestrator extract \
  --pdf ../pdf-extractor/camry.pdf \
  --output /tmp/camry-specs.md

# 2. Create campaign (via interactive menu)
./orchestrator start
# Select option 2, use /tmp/camry-specs.md as input

# 3. Query the campaign
./orchestrator query \
  --campaign "<campaign-id>" \
  --question "What are the key features?"
```

## Troubleshooting

### Issue: "OPENROUTER_API_KEY not set"

**Solution:**
```bash
export OPENROUTER_API_KEY="your-key-here"
# Or add to .env file in repository root
```

### Issue: "Database connection failed"

**Solution:**
- Check if knowledge-engine database exists
- Verify database path in config
- Check permissions on database file/directory

### Issue: "CLI binaries not found"

**Solution:**
- The CLI automatically builds pdf-extractor and knowledge-engine-cli
- Ensure Go workspace is set up correctly
- Check that source code for dependencies is available

### Issue: "PDF extraction fails"

**Solution:**
- Verify MuPDF libraries are installed
- Check PDF file is valid and accessible
- Verify API key has sufficient credits

### Issue: "Migrations fail"

**Solution:**
- Check database permissions
- Verify migration files exist in knowledge-engine
- Check database isn't locked by another process

## Debug Mode

Enable verbose output for troubleshooting:

```bash
cd libs/knowledge-engine
./orchestrator --verbose start
```

This shows detailed logs for:
- Configuration loading
- Database operations
- API calls
- Internal processing

## Next Steps

After basic testing:

1. **Integration Testing**: Test with real PDFs from production
2. **Performance Testing**: Test with large PDFs (20+ pages)
3. **Error Scenario Testing**: Test error handling and recovery
4. **User Acceptance Testing**: Have business users test the interface

## Test Data

You can use test PDFs from:
- `libs/pdf-extractor/camry.pdf` (if exists)
- `libs/pdf-extractor/testdata/` directory
- Any valid product brochure PDF

---

**Happy Testing!** 🚀

For issues or questions, check the logs with `--verbose` flag or review the implementation status in `IMPLEMENTATION_STATUS.md`.

