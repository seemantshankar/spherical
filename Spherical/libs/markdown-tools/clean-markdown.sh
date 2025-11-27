#!/bin/bash
# Comprehensive markdown cleanup script
# Applies all fixes: empty sections, spacing, and formatting

set -e

if [ $# -lt 1 ]; then
    echo "Usage: ./clean-markdown.sh <markdown-file>"
    exit 1
fi

INPUT_FILE="$1"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "🧹 Cleaning markdown file: $INPUT_FILE"
echo ""

# Step 1: Fix empty sections
echo "1️⃣  Removing empty sections..."
python3 "$SCRIPT_DIR/clean-empty-sections.py" "$INPUT_FILE" || echo "   (skipped)"

# Step 2: Fix spacing
echo "2️⃣  Fixing spacing..."
python3 "$SCRIPT_DIR/fix-spacing.py" "$INPUT_FILE" || echo "   (skipped)"

# Step 3: Remove any remaining artifacts
echo "3️⃣  Removing embedded messages..."
sed -i '' 's/((No technical product specifications.*$//' "$INPUT_FILE" 2>/dev/null || true

# Step 4: Clean up blank lines
echo "4️⃣  Normalizing blank lines..."
sed -i '' '/^[[:space:]]*$/d' "$INPUT_FILE"
# Add back single blank lines between sections
sed -i '' 's/^\(##.*\)$/\n\1\n/' "$INPUT_FILE"

echo ""
echo "✅ Cleanup complete!"
echo "   File: $INPUT_FILE"
echo "   Lines: $(wc -l < "$INPUT_FILE")"





