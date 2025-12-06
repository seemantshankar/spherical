# Orchestrator CLI Library

A unified, user-friendly CLI interface for business users to manage product campaigns. The orchestrator coordinates between PDF extraction, knowledge engine ingestion, and query services.

## Overview

The Orchestrator provides a simple command-line interface that guides users through:
- Extracting specifications from PDF brochures
- Ingesting extracted content into knowledge databases
- Querying campaigns with natural language questions
- Managing campaigns and database operations

## Features

- User-friendly interface with clear language and visual feedback
- Automated database migrations
- Automatic CLI binary building and updates
- Per-campaign vector store management
- Interactive menus and prompts
- Progress indicators for all operations

## Installation

```bash
cd libs/orchestrator
go build -o orchestrator ./cmd/orchestrator
```

## Usage

```bash
# Start interactive mode
./orchestrator start

# Or use direct commands
./orchestrator extract --pdf brochure.pdf
./orchestrator ingest --campaign <id> --markdown specs.md
./orchestrator query --campaign <id> --question "What colors are available?"
```

## Configuration

Create a `.env` file in the project root:

```env
OPENROUTER_API_KEY=your-api-key-here
LLM_MODEL=google/gemini-2.5-flash-preview-09-2025
KNOWLEDGE_ENGINE_DB_PATH=/tmp/knowledge-engine.db
```

## Requirements

- Go 1.24.0 or later
- Access to knowledge-engine database
- OpenRouter API key for LLM operations

