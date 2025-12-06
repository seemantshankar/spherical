# Data Persistence Configuration

The Orchestrator stores data in persistent locations by default to ensure data survives system restarts.

## Persistent Storage Locations

### Default Paths

By default, the Orchestrator stores data in persistent locations:

1. **Database**: `~/.orchestrator/knowledge-engine.db` (or `./data/knowledge-engine.db` if home directory is unavailable)
2. **Vector Stores**: `~/.orchestrator/vector-stores/` (per-campaign FAISS indexes)
3. **Temporary Files**: `/tmp/orchestrator-temp/` (cleared on reboot, safe to delete)

### Customizing Storage Locations

You can customize storage locations using environment variables:

```bash
# Set persistent data directory (contains database and vector stores)
export ORCHESTRATOR_DATA_DIR="$HOME/my-orchestrator-data"

# Or set paths individually:
export ORCHESTRATOR_VECTOR_STORE_ROOT="$HOME/.orchestrator/vectors"
export KNOWLEDGE_ENGINE_DB_PATH="$HOME/.orchestrator/knowledge-engine.db"
export ORCHESTRATOR_TEMP_DIR="/tmp/orchestrator-temp"
```

## How Data Persistence Works

### On Startup

When the Orchestrator starts:

1. **Database**: Opens the SQLite database from the persistent location. If it doesn't exist, a new one is created.
2. **Vector Stores**: For each campaign, the system:
   - Checks if a FAISS index file exists on disk
   - If the index is empty or missing, loads vectors from the database
   - Does NOT re-embed or re-ingest data

### During Ingestion

When ingesting new data:

1. **Database**: All embeddings and metadata are saved to the persistent SQLite database
2. **Vector Store**: FAISS indexes are saved to persistent disk locations
3. **No Re-embedding**: If you restart after ingestion, vectors are loaded from the database

### Data Flow

```
PDF Extraction → Embedding → Database (persistent)
                              ↓
                    FAISS Index (persistent)
                              ↓
                    Query (uses existing indexes)
```

On restart:
```
Startup → Load Database → Check FAISS Index → Sync from DB if needed → Ready to Query
```

## Migration from Temporary Storage

If you previously used `/tmp/` locations and lost data:

1. **Check if database exists**: Look for `/tmp/knowledge-engine.db`
2. **If found, copy to persistent location**:
   ```bash
   mkdir -p ~/.orchestrator
   cp /tmp/knowledge-engine.db ~/.orchestrator/knowledge-engine.db
   ```
3. **Vector stores**: If you have FAISS indexes in `/tmp/orchestrator-vectors/`, copy them:
   ```bash
   cp -r /tmp/orchestrator-vectors ~/.orchestrator/vector-stores
   ```

## Configuration Priority

The system uses this priority for determining storage paths:

1. Environment variables (highest priority)
2. Configuration file settings
3. Default persistent paths (lowest priority)

## Troubleshooting

### Data is still being lost

1. Check your configuration:
   ```bash
   # Verify paths are not in /tmp
   echo $ORCHESTRATOR_VECTOR_STORE_ROOT
   echo $KNOWLEDGE_ENGINE_DB_PATH
   ```

2. Check if directories exist and are writable:
   ```bash
   ls -la ~/.orchestrator/
   ```

3. Verify database location:
   ```bash
   # The database file should exist in persistent location
   ls -lh ~/.orchestrator/knowledge-engine.db
   ```

### Vectors are being re-embedded

If vectors are being re-embedded on startup, check:

1. FAISS index files exist: `~/.orchestrator/vector-stores/{campaign-id}/index.faiss`
2. Database has embedding vectors in `knowledge_chunks` table
3. Vector store path is configured correctly

## Best Practices

1. **Use persistent paths**: Always configure paths outside `/tmp/`
2. **Backup data**: Regularly backup `~/.orchestrator/` directory
3. **Monitor disk space**: Vector stores can be large
4. **Use environment variables**: Set paths in your shell profile (`.bashrc`, `.zshrc`)

## Example Configuration

Add to your `~/.bashrc` or `~/.zshrc`:

```bash
# Orchestrator persistent storage
export ORCHESTRATOR_DATA_DIR="$HOME/.orchestrator"
export ORCHESTRATOR_VECTOR_STORE_ROOT="$HOME/.orchestrator/vector-stores"
export KNOWLEDGE_ENGINE_DB_PATH="$HOME/.orchestrator/knowledge-engine.db"
```

