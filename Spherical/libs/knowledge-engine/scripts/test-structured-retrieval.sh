#!/bin/bash
# Comprehensive test runner for structured retrieval system

set -e

echo "=========================================="
echo "Structured Retrieval Comprehensive Tests"
echo "=========================================="
echo ""

cd "$(dirname "$0")/.."

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Test counters
PASSED=0
FAILED=0

# Function to run tests and track results
run_test() {
    local name=$1
    shift
    echo -e "${YELLOW}Running: $name${NC}"
    if go test "$@" 2>&1 | tee /tmp/test_output.log; then
        echo -e "${GREEN}✓ $name PASSED${NC}"
        ((PASSED++))
    else
        echo -e "${RED}✗ $name FAILED${NC}"
        ((FAILED++))
        return 1
    fi
    echo ""
}

# 1. Unit Tests
echo "=== 1. Unit Tests ==="
run_test "Spec Normalizer Tests" ./internal/retrieval -v -run TestSpecNormalizer
run_test "Availability Detector Tests" ./internal/retrieval -v -run TestAvailabilityDetector
run_test "Confidence Calculator Tests" ./internal/retrieval -v -run TestConfidenceCalculator

# 2. Integration Tests
echo "=== 2. Integration Tests ==="
run_test "Structured Retrieval Integration" ./tests/integration -v -run TestStructuredRetrieval_Basic
run_test "Structured Retrieval Synonyms" ./tests/integration -v -run TestStructuredRetrieval_Synonym
run_test "Structured Retrieval Batch" ./tests/integration -v -run TestStructuredRetrieval_Batch

# 3. Contract Tests
echo "=== 3. Contract Tests ==="
run_test "REST API Contract Tests" ./tests/integration -v -run TestStructuredRetrieval_REST

# 4. Comprehensive Tests (skip if short mode)
if [ "$1" != "--short" ]; then
    echo "=== 4. Comprehensive Tests ==="
    run_test "Concurrency Tests" ./tests/integration -v -run TestStructuredRetrieval_Concurrency
    run_test "Performance Tests" ./tests/integration -v -run TestStructuredRetrieval_Performance
    run_test "Edge Case Tests" ./tests/integration -v -run TestStructuredRetrieval_EdgeCases
    run_test "Realistic Scenario Tests" ./tests/integration -v -run TestStructuredRetrieval_RealisticScenario
    run_test "Caching Tests" ./tests/integration -v -run TestStructuredRetrieval_Caching
fi

# 5. Benchmark Tests
if [ "$1" != "--short" ]; then
    echo "=== 5. Benchmark Tests ==="
    echo -e "${YELLOW}Running benchmarks...${NC}"
    go test ./internal/retrieval -bench=BenchmarkStructuredRetrieval -benchmem -run=^$ 2>&1 | head -50
    echo ""
fi

# 6. Coverage Report
echo "=== 6. Coverage Report ==="
echo -e "${YELLOW}Generating coverage report...${NC}"
go test ./internal/retrieval ./tests/integration -coverprofile=coverage.out -coverpkg=./internal/retrieval 2>&1 | tail -5
if [ -f coverage.out ]; then
    COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}')
    echo -e "Total Coverage: ${COVERAGE}"
    echo "Detailed report: coverage.out"
    echo "HTML report: go tool cover -html=coverage.out"
fi
echo ""

# Summary
echo "=========================================="
echo "Test Summary"
echo "=========================================="
echo -e "${GREEN}Passed: $PASSED${NC}"
if [ $FAILED -gt 0 ]; then
    echo -e "${RED}Failed: $FAILED${NC}"
    exit 1
else
    echo -e "${GREEN}Failed: $FAILED${NC}"
    echo ""
    echo -e "${GREEN}All tests passed! ✓${NC}"
    exit 0
fi



