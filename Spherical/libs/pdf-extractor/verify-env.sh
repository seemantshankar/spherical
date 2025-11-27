#!/bin/bash
# Script to verify .env configuration for testing

cd "$(dirname "$0")"

echo "🔍 Verifying environment configuration..."
echo ""

# Check if .env exists
if [ ! -f .env ]; then
    echo "❌ .env file not found!"
    echo "   Please create it by running: cp .env.template .env"
    echo "   Then add your OPENROUTER_API_KEY"
    exit 1
fi

# Source .env file
source .env

# Check API key
if [ -z "$OPENROUTER_API_KEY" ]; then
    echo "❌ OPENROUTER_API_KEY is not set in .env"
    exit 1
elif [ "$OPENROUTER_API_KEY" = "sk-or-your-api-key-here" ]; then
    echo "❌ OPENROUTER_API_KEY is still set to placeholder value"
    echo "   Please replace it with your actual API key"
    exit 1
else
    echo "✓ OPENROUTER_API_KEY is set"
fi

# Check model (optional)
if [ -n "$LLM_MODEL" ]; then
    echo "✓ LLM_MODEL override: $LLM_MODEL"
else
    echo "✓ Using default model: google/gemini-2.5-flash-preview-09-2025"
fi

# Check for test PDF
TEST_PDF="/Users/seemant/Documents/Projects/AIOutcallingAgent/Uploads/Arena-Wagon-r-Brochure.pdf"
if [ -f "$TEST_PDF" ]; then
    echo "✓ Test PDF found: $TEST_PDF"
else
    echo "⚠️  Test PDF not found at: $TEST_PDF"
    echo "   Integration tests will be skipped"
fi

echo ""
echo "🎉 Environment configuration looks good!"
echo ""
echo "Ready to run tests:"
echo "  • Unit tests:        go test ./internal/..."
echo "  • Short tests:       go test -short ./..."
echo "  • Integration tests: go test -v ./tests/integration/"
echo "  • Run CLI:           ./pdf-extractor <pdf-file>"





