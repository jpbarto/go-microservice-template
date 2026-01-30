#!/bin/bash
set -e

# Configuration
HOST="${TEST_HOST:-localhost}"
PORT="${TEST_PORT:-8080}"
BASE_URL="http://${HOST}:${PORT}"
TIMEOUT=5

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Function to print test results
print_test_result() {
    local test_name="$1"
    local result="$2"
    local message="$3"
    
    TESTS_RUN=$((TESTS_RUN + 1))
    
    if [ "$result" = "PASS" ]; then
        echo -e "${GREEN}✓ PASS${NC}: $test_name"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        echo -e "${RED}✗ FAIL${NC}: $test_name"
        echo -e "  ${YELLOW}Details${NC}: $message"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
}

# Function to check if service is running
wait_for_service() {
    echo "Waiting for goserv to be available at ${BASE_URL}..."
    local max_attempts=30
    local attempt=0
    
    while [ $attempt -lt $max_attempts ]; do
        if curl -s -f -m 2 "${BASE_URL}/" > /dev/null 2>&1; then
            echo -e "${GREEN}Service is available!${NC}"
            return 0
        fi
        attempt=$((attempt + 1))
        sleep 1
    done
    
    echo -e "${RED}Service is not available after ${max_attempts} seconds${NC}"
    exit 1
}

# Test 1: Check if service responds to root endpoint
test_root_endpoint_responds() {
    local response
    response=$(curl -s -w "\n%{http_code}" -m $TIMEOUT "${BASE_URL}/")
    local http_code=$(echo "$response" | tail -n 1)
    local body=$(echo "$response" | head -n -1)
    
    if [ "$http_code" = "200" ]; then
        print_test_result "Root endpoint responds with 200 OK" "PASS"
    else
        print_test_result "Root endpoint responds with 200 OK" "FAIL" "Got HTTP $http_code"
    fi
}

# Test 2: Check if response is valid JSON
test_response_is_json() {
    local response
    response=$(curl -s -m $TIMEOUT "${BASE_URL}/")
    
    if echo "$response" | jq . > /dev/null 2>&1; then
        print_test_result "Response is valid JSON" "PASS"
    else
        print_test_result "Response is valid JSON" "FAIL" "Response is not valid JSON: $response"
    fi
}

# Test 3: Check if response contains required fields
test_response_contains_required_fields() {
    local response
    response=$(curl -s -m $TIMEOUT "${BASE_URL}/")
    
    local has_service_name=$(echo "$response" | jq -r '.service_name' 2>/dev/null)
    local has_service_version=$(echo "$response" | jq -r '.service_version' 2>/dev/null)
    local has_ip_address=$(echo "$response" | jq -r '.ip_address' 2>/dev/null)
    local has_instance_uuid=$(echo "$response" | jq -r '.instance_uuid' 2>/dev/null)
    local has_timestamp=$(echo "$response" | jq -r '.timestamp' 2>/dev/null)
    
    if [ "$has_service_name" != "null" ] && [ "$has_service_name" != "" ]; then
        print_test_result "Response contains service_name field" "PASS"
    else
        print_test_result "Response contains service_name field" "FAIL" "service_name is missing or null"
    fi
    
    if [ "$has_service_version" != "null" ] && [ "$has_service_version" != "" ]; then
        print_test_result "Response contains service_version field" "PASS"
    else
        print_test_result "Response contains service_version field" "FAIL" "service_version is missing or null"
    fi
    
    if [ "$has_ip_address" != "null" ] && [ "$has_ip_address" != "" ]; then
        print_test_result "Response contains ip_address field" "PASS"
    else
        print_test_result "Response contains ip_address field" "FAIL" "ip_address is missing or null"
    fi
    
    if [ "$has_instance_uuid" != "null" ] && [ "$has_instance_uuid" != "" ]; then
        print_test_result "Response contains instance_uuid field" "PASS"
    else
        print_test_result "Response contains instance_uuid field" "FAIL" "instance_uuid is missing or null"
    fi
    
    if [ "$has_timestamp" != "null" ] && [ "$has_timestamp" != "" ]; then
        print_test_result "Response contains timestamp field" "PASS"
    else
        print_test_result "Response contains timestamp field" "FAIL" "timestamp is missing or null"
    fi
}

# Test 4: Check if service_name is correct
test_service_name_value() {
    local response
    response=$(curl -s -m $TIMEOUT "${BASE_URL}/")
    local service_name=$(echo "$response" | jq -r '.service_name')
    
    if [ "$service_name" = "goserv" ]; then
        print_test_result "Service name is 'goserv'" "PASS"
    else
        print_test_result "Service name is 'goserv'" "FAIL" "Got: $service_name"
    fi
}

# Test 5: Check if instance_uuid is valid UUID format
test_instance_uuid_format() {
    local response
    response=$(curl -s -m $TIMEOUT "${BASE_URL}/")
    local uuid=$(echo "$response" | jq -r '.instance_uuid')
    
    # UUID v4 format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
    if [[ "$uuid" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$ ]]; then
        print_test_result "Instance UUID is valid format" "PASS"
    else
        print_test_result "Instance UUID is valid format" "FAIL" "Got: $uuid"
    fi
}

# Test 6: Check if instance_uuid remains consistent across requests
test_instance_uuid_consistency() {
    local response1
    local response2
    response1=$(curl -s -m $TIMEOUT "${BASE_URL}/")
    response2=$(curl -s -m $TIMEOUT "${BASE_URL}/")
    
    local uuid1=$(echo "$response1" | jq -r '.instance_uuid')
    local uuid2=$(echo "$response2" | jq -r '.instance_uuid')
    
    if [ "$uuid1" = "$uuid2" ]; then
        print_test_result "Instance UUID remains consistent across requests" "PASS"
    else
        print_test_result "Instance UUID remains consistent across requests" "FAIL" "UUID1: $uuid1, UUID2: $uuid2"
    fi
}

# Test 7: Check if timestamp format is valid RFC3339
test_timestamp_format() {
    local response
    response=$(curl -s -m $TIMEOUT "${BASE_URL}/")
    local timestamp=$(echo "$response" | jq -r '.timestamp')
    
    # Try to parse the timestamp using date command
    if date -d "$timestamp" > /dev/null 2>&1 || date -j -f "%Y-%m-%dT%H:%M:%SZ" "$timestamp" > /dev/null 2>&1; then
        print_test_result "Timestamp is in valid RFC3339 format" "PASS"
    else
        print_test_result "Timestamp is in valid RFC3339 format" "FAIL" "Got: $timestamp"
    fi
}

# Test 8: Check if Content-Type header is application/json
test_content_type_header() {
    local content_type
    content_type=$(curl -s -I -m $TIMEOUT "${BASE_URL}/" | grep -i "content-type" | awk '{print $2}' | tr -d '\r\n')
    
    if [[ "$content_type" == *"application/json"* ]]; then
        print_test_result "Content-Type header is application/json" "PASS"
    else
        print_test_result "Content-Type header is application/json" "FAIL" "Got: $content_type"
    fi
}

# Test 9: Check if non-root paths return 404
test_invalid_path_returns_404() {
    local http_code
    http_code=$(curl -s -o /dev/null -w "%{http_code}" -m $TIMEOUT "${BASE_URL}/invalid-path")
    
    if [ "$http_code" = "404" ]; then
        print_test_result "Invalid path returns 404" "PASS"
    else
        print_test_result "Invalid path returns 404" "FAIL" "Got HTTP $http_code"
    fi
}

# Main execution
main() {
    echo "========================================"
    echo "  goserv Unit Tests"
    echo "========================================"
    echo "Target: ${BASE_URL}"
    echo ""
    
    wait_for_service
    echo ""
    
    echo "Running tests..."
    echo ""
    
    test_root_endpoint_responds
    test_response_is_json
    test_response_contains_required_fields
    test_service_name_value
    test_instance_uuid_format
    test_instance_uuid_consistency
    test_timestamp_format
    test_content_type_header
    test_invalid_path_returns_404
    
    echo ""
    echo "========================================"
    echo "  Test Results"
    echo "========================================"
    echo "Tests Run:    $TESTS_RUN"
    echo -e "Tests Passed: ${GREEN}$TESTS_PASSED${NC}"
    echo -e "Tests Failed: ${RED}$TESTS_FAILED${NC}"
    echo "========================================"
    
    if [ $TESTS_FAILED -gt 0 ]; then
        exit 1
    fi
    
    exit 0
}

main
