#!/bin/bash
# Quick test script for orchestrator CLI

set -e

echo "🧪 Orchestrator CLI Quick Test"
echo "================================"
echo ""

# Check if we're in the right directory
if [ ! -f "go.mod" ]; then
    echo "❌ Error: Must run from libs/orchestrator directory"
    exit 1
fi

echo "1️⃣  Building orchestrator CLI..."
if go build -o orchestrator ./cmd/orchestrator; then
    echo "   ✅ Build successful!"
else
    echo "   ❌ Build failed!"
    exit 1
fi

echo ""
echo "2️⃣  Checking CLI executable..."
if [ -f "./orchestrator" ]; then
    echo "   ✅ Executable exists"
    ./orchestrator --version 2>/dev/null || echo "   ℹ️  Version command not available (OK)"
else
    echo "   ❌ Executable not found!"
    exit 1
fi

echo ""
echo "3️⃣  Testing help commands..."
if ./orchestrator --help > /dev/null 2>&1; then
    echo "   ✅ Root help works"
else
    echo "   ❌ Root help failed"
    exit 1
fi

echo ""
echo "4️⃣  Checking required files..."
REQUIRED_FILES=(
    "cmd/orchestrator/main.go"
    "cmd/orchestrator/commands/root.go"
    "cmd/orchestrator/commands/start.go"
    "cmd/orchestrator/commands/extract.go"
    "cmd/orchestrator/commands/ingest.go"
    "cmd/orchestrator/commands/query.go"
)

ALL_EXIST=true
for file in "${REQUIRED_FILES[@]}"; do
    if [ -f "$file" ]; then
        echo "   ✅ $file"
    else
        echo "   ❌ $file (MISSING)"
        ALL_EXIST=false
    fi
done

if [ "$ALL_EXIST" = false ]; then
    echo ""
    echo "   ⚠️  Some required files are missing!"
    exit 1
fi

echo ""
echo "5️⃣  Checking environment..."
if [ -z "$OPENROUTER_API_KEY" ]; then
    echo "   ⚠️  OPENROUTER_API_KEY not set (will need .env file or export)"
else
    echo "   ✅ OPENROUTER_API_KEY is set"
fi

echo ""
echo "✅ All basic checks passed!"
echo ""
echo "📋 Next steps:"
echo "   1. Ensure OPENROUTER_API_KEY is set (or in .env file)"
echo "   2. Run: ./orchestrator start"
echo "   3. Or test standalone commands:"
echo "      ./orchestrator extract --help"
echo "      ./orchestrator ingest --help"
echo "      ./orchestrator query --help"
echo ""
