#!/bin/bash

################################################################################
# Acceptance Test Script
#
# This script runs acceptance tests for the goserv application.
# These tests verify that the application meets acceptance criteria.
#
# Prerequisites:
#   - goserv application running
#   - curl and jq installed
#
# Usage:
#   ./acceptance_test.sh [BASE_URL]
#
# Examples:
#   ./acceptance_test.sh http://localhost:8080
#   ./acceptance_test.sh http://myservice.local:80
################################################################################

set -e  # Exit on error

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
BASE_URL="${1}"

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Acceptance Tests for goserv${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""
echo -e "${BLUE}Configuration:${NC}"
echo -e "  Base URL: ${BASE_URL}"
echo ""

# Function to run a test
run_test() {
    local test_name="$1"
    local test_command="$2"
    
    TESTS_RUN=$((TESTS_RUN + 1))
    
    if eval "${test_command}" > /dev/null 2>&1; then
        echo -e "${GREEN}✓${NC} ${test_name}"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        echo -e "${RED}✗${NC} ${test_name}"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

# Check if service is available
echo -e "${BLUE}Checking service availability...${NC}"
if ! curl -s -f -o /dev/null --max-time 5 "${BASE_URL}/health"; then
    echo -e "${RED}Error: Service is not reachable at ${BASE_URL}${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Service is available${NC}"
echo ""

echo -e "${BLUE}Running acceptance tests...${NC}"
echo ""

################################################################################
# Acceptance Tests
################################################################################

# Test: Service returns valid JSON
run_test "Service returns valid JSON" \
    "curl -s ${BASE_URL}/ | jq -e . > /dev/null"

# Test: Service has required fields
run_test "Response contains service_name field" \
    "curl -s ${BASE_URL}/ | jq -e '.service_name' > /dev/null"

run_test "Response contains service_version field" \
    "curl -s ${BASE_URL}/ | jq -e '.service_version' > /dev/null"

run_test "Response contains instance_uuid field" \
    "curl -s ${BASE_URL}/ | jq -e '.instance_uuid' > /dev/null"

# Test: Health endpoint works
run_test "Health endpoint returns 200" \
    "curl -s -o /dev/null -w '%{http_code}' ${BASE_URL}/health | grep -q '200'"

# Test: Ready endpoint works
run_test "Ready endpoint returns 200" \
    "curl -s -o /dev/null -w '%{http_code}' ${BASE_URL}/ready | grep -q '200'"

# Test: Service name is correct
run_test "Service name is 'goserv'" \
    "curl -s ${BASE_URL}/ | jq -e '.service_name == \"goserv\"' > /dev/null"

# Test: UUID format is valid
run_test "Instance UUID is valid format" \
    "curl -s ${BASE_URL}/ | jq -r '.instance_uuid' | grep -qE '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'"

################################################################################
# Test Summary
################################################################################

echo ""
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Test Results${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""
echo -e "Total Tests:   ${TESTS_RUN}"
echo -e "${GREEN}Passed:${NC}        ${TESTS_PASSED}"
echo -e "${RED}Failed:${NC}        ${TESTS_FAILED}"
echo ""

if [ ${TESTS_FAILED} -gt 0 ]; then
    echo -e "${RED}========================================${NC}"
    echo -e "${RED}Acceptance Tests FAILED${NC}"
    echo -e "${RED}========================================${NC}"
    exit 1
else
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}Acceptance Tests PASSED${NC}"
    echo -e "${GREEN}========================================${NC}"
    exit 0
fi
