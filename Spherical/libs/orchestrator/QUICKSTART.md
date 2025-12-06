# Quick Start: Testing the Orchestrator CLI

## 🚀 Quick Test (5 minutes)

### Step 1: Build the CLI

```bash
cd /Users/seemant/Documents/Projects/spherical/Spherical/libs/orchestrator
go build -o orchestrator ./cmd/orchestrator
```

### Step 2: Set Environment Variable

```bash
export OPENROUTER_API_KEY="your-api-key-here"
# OR create .env file in repository root with:
# OPENROUTER_API_KEY=your-api-key-here
```

### Step 3: Run Quick Test

```bash
# Test the build
./orchestrator --help

# Test extract command help
./orchestrator extract --help
```

### Step 4: Test Interactive Mode

```bash
./orchestrator start
```

This will:
1. Run startup checks (environment, database, migrations, CLI builds)
2. Show main menu
3. Allow you to test all features

## 📝 Test PDF Available

You can use the test PDF from pdf-extractor:
- `../pdf-extractor/camry.pdf` (2.7MB)

## 🔍 Quick Commands

```bash
# Extract a PDF
./orchestrator extract --pdf ../pdf-extractor/camry.pdf --output /tmp/test.md

# See all commands
./orchestrator --help

# Verbose mode for debugging
./orchestrator --verbose start
```

## 📚 Full Testing Guide

See `TESTING.md` for comprehensive testing instructions.

