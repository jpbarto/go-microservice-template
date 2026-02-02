#!/bin/bash

################################################################################
# Integration Test Wrapper Script
#
# This script orchestrates the execution of all integration tests for the
# goserv application. It runs performance tests and acceptance tests in sequence.
#
# Prerequisites:
#   - goserv application running
#   - All test dependencies installed (k6, curl, jq)
#
# Usage:
#   ./integration_test.sh [HOST] [PORT]
#
# Examples:
#   ./integration_test.sh                    # Uses localhost:8080
#   ./integration_test.sh localhost 8080     # Explicit host and port
#   ./integration_test.sh myservice.local 80
################################################################################

set -e  # Exit on error

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
TEST_HOST="${1:-localhost}"
TEST_PORT="${2:-8080}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Test results tracking
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
declare -a FAILED_TEST_NAMES

# Function to print section headers
print_header() {
    echo ""
    echo -e "${CYAN}========================================${NC}"
    echo -e "${CYAN}$1${NC}"
    echo -e "${CYAN}========================================${NC}"
    echo ""
}

# Function to print test section
print_test_section() {
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}Test: $1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

# Function to print success messages
print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

# Function to print error messages
print_error() {
    echo -e "${RED}✗ $1${NC}"
}

# Function to print warning messages
print_warning() {
    echo -e "${YELLOW}⚠ $1${NC}"
}

# Function to print info messages
print_info() {
    echo -e "${BLUE}ℹ $1${NC}"
}

# Function to run a test script
run_test() {
    local test_name="$1"
    local test_script="$2"
    local test_args="$3"
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    print_test_section "${test_name}"
    
    if [ ! -f "${test_script}" ]; then
        print_error "Test script not found: ${test_script}"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        FAILED_TEST_NAMES+=("${test_name} (script not found)")
        return 1
    fi
    
    if [ ! -x "${test_script}" ]; then
        print_warning "Test script not executable, attempting to make it executable..."
        chmod +x "${test_script}"
    fi
    
    # Run the test and capture output
    if "${test_script}" ${test_args} 2>&1; then
        PASSED_TESTS=$((PASSED_TESTS + 1))
        print_success "${test_name} PASSED"
        return 0
    else
        FAILED_TESTS=$((FAILED_TESTS + 1))
        FAILED_TEST_NAMES+=("${test_name}")
        print_error "${test_name} FAILED"
        return 1
    fi
}

################################################################################
# Main Test Execution
################################################################################

print_header "Integration Test Suite"
print_info "Target: ${TEST_HOST}:${TEST_PORT}"
print_info "Test Directory: ${SCRIPT_DIR}"
echo ""

# Check if service is reachable
print_info "Verifying service availability..."
if ! curl -s -f -o /dev/null --max-time 5 "http://${TEST_HOST}:${TEST_PORT}/health"; then
    print_error "Service is not reachable at http://${TEST_HOST}:${TEST_PORT}"
    print_error "Please ensure the goserv application is running before running tests"
    exit 1
fi
print_success "Service is available"
echo ""

# Allow tests to continue even if one fails
set +e

################################################################################
# Test 1: Performance Tests
################################################################################

run_test "Performance Tests" \
    "${SCRIPT_DIR}/performance_test.sh" \
    "${TEST_HOST} ${TEST_PORT}"

################################################################################
# Test 2: Acceptance Tests
################################################################################

run_test "Acceptance Tests" \
    "${SCRIPT_DIR}/acceptance_test.sh" \
    "${TEST_HOST} ${TEST_PORT}"

################################################################################
# Test Summary
################################################################################

print_header "Test Summary"

echo -e "${BLUE}Total Tests:${NC}   ${TOTAL_TESTS}"
echo -e "${GREEN}Passed:${NC}        ${PASSED_TESTS}"
echo -e "${RED}Failed:${NC}        ${FAILED_TESTS}"
echo ""

if [ ${FAILED_TESTS} -gt 0 ]; then
    echo -e "${RED}Failed Tests:${NC}"
    for failed_test in "${FAILED_TEST_NAMES[@]}"; do
        echo -e "  ${RED}✗${NC} ${failed_test}"
    done
    echo ""
    print_header "INTEGRATION TESTS FAILED"
    exit 1
else
    print_success "All integration tests passed!"
    print_header "INTEGRATION TESTS PASSED"
    exit 0
fi
