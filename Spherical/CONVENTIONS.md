# Project Conventions & Architecture

This document outlines the structural conventions and architectural standards for the Spherical platform. All new code and features should adhere to these guidelines.

## Directory Structure

The project follows a monorepo-like structure where distinct features and tools are organized into self-contained libraries.

```
.
├── libs/                  # Code libraries and tools
│   ├── pdf-extractor/     # Example: Go-based library
│   └── markdown-tools/    # Example: Script collection
├── specs/                 # Feature specifications (Spec Kit)
├── .agent/                # Agent workflows and configuration
└── CONVENTIONS.md         # This file
```

## Library Guidelines (`libs/`)

All functional code should reside in `libs/`. Do not create top-level directories for code.

### 1. Naming
- Use **kebab-case** for library directory names (e.g., `pdf-extractor`, `image-processor`).
- Names should be descriptive of the library's primary function.

### 2. Structure by Language/Type

#### Go Libraries
Go projects should be self-contained within their library directory.
- **`go.mod`**: Must exist at the library root (e.g., `libs/my-lib/go.mod`).
- **Standard Layout**: Follow standard Go project layout:
    - `cmd/`: Main applications/binaries.
    - `pkg/`: Library code usable by external projects.
    - `internal/`: Private application and library code.
- **Execution**: Run Go commands from the library directory, not the project root.
  ```bash
  cd libs/my-lib
  go test ./...
  ```

#### Script Collections (Python/Shell)
Group related scripts into a single library directory (e.g., `libs/markdown-tools`).
- **Portability**: Scripts should be written to be runnable from the project root or their own directory. Use relative path resolution (e.g., `$(dirname "$0")`) to locate dependencies.
- **Dependencies**: If Python scripts require dependencies, include a `requirements.txt` in the library directory.

### 3. New Features
- **One Feature = One Library**: Generally, a major new feature should be developed as a new library in `libs/`.
- **Shared Code**: If code is shared between libraries, consider creating a dedicated `libs/common` or `libs/shared` library.

## Specifications (`specs/`)

- All feature work should start with a specification in the `specs/` directory, following the GitHub Spec Kit workflow.
- Use `/speckit-specify` to generate new specs.

## General Rules

- **Root Directory**: Keep the root directory clean. It should only contain project-level configuration (`.gitignore`, `README.md`, `CONVENTIONS.md`) and directories.
- **Artifacts**: Build artifacts (binaries, logs) should be ignored via `.gitignore` and never committed.
