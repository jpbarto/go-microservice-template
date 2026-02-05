#!/bin/bash
set -e

# Configuration
RELEASE_NAME="${RELEASE_NAME:-goserv}"
NAMESPACE="${NAMESPACE:-default}"
SERVICE_NAME="${SERVICE_NAME:-goserv}"
TIMEOUT=5
MAX_WAIT=60

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

echo -e "${BLUE}======================================${NC}"
echo -e "${BLUE}Goserv Kubernetes Validation${NC}"
echo -e "${BLUE}======================================${NC}"
echo "Release Name: ${RELEASE_NAME}"
echo "Namespace: ${NAMESPACE}"
echo "Service Name: ${SERVICE_NAME}"
echo ""

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

# Function to print section header
print_section() {
    echo ""
    echo -e "${BLUE}>>> $1${NC}"
}

# Test 1: Check if Helm release exists
print_section "Checking Helm Release"
test_helm_release_exists() {
    if helm status "$RELEASE_NAME" -n "$NAMESPACE" > /dev/null 2>&1; then
        local status=$(helm status "$RELEASE_NAME" -n "$NAMESPACE" -o json | jq -r '.info.status')
        if [ "$status" = "deployed" ]; then
            print_test_result "Helm release exists and is deployed" "PASS"
        else
            print_test_result "Helm release exists and is deployed" "FAIL" "Status is: $status"
        fi
    else
        print_test_result "Helm release exists and is deployed" "FAIL" "Helm release '$RELEASE_NAME' not found in namespace '$NAMESPACE'"
    fi
}
test_helm_release_exists

# Test 2: Check if deployment exists and has correct replica count
print_section "Checking Kubernetes Deployment"
test_deployment_exists() {
    if kubectl get deployment "$RELEASE_NAME" -n "$NAMESPACE" > /dev/null 2>&1; then
        print_test_result "Deployment exists" "PASS"
        
        # Check replica status
        local ready=$(kubectl get deployment "$RELEASE_NAME" -n "$NAMESPACE" -o jsonpath='{.status.readyReplicas}')
        local desired=$(kubectl get deployment "$RELEASE_NAME" -n "$NAMESPACE" -o jsonpath='{.spec.replicas}')
        
        if [ "$ready" = "$desired" ] && [ "$ready" -gt 0 ]; then
            print_test_result "Deployment has correct number of ready replicas ($ready/$desired)" "PASS"
        else
            print_test_result "Deployment has correct number of ready replicas" "FAIL" "Ready: ${ready:-0}, Desired: ${desired:-0}"
        fi
    else
        print_test_result "Deployment exists" "FAIL" "Deployment '$RELEASE_NAME' not found in namespace '$NAMESPACE'"
    fi
}
test_deployment_exists

# Test 3: Check if all pods are running and ready
print_section "Checking Pods"
test_pods_running() {
    local pods=$(kubectl get pods -n "$NAMESPACE" -l "app.kubernetes.io/name=goserv,app.kubernetes.io/instance=$RELEASE_NAME" -o json)
    local pod_count=$(echo "$pods" | jq -r '.items | length')
    
    if [ "$pod_count" -eq 0 ]; then
        print_test_result "Pods exist" "FAIL" "No pods found with label app.kubernetes.io/instance=$RELEASE_NAME"
        return
    fi
    
    print_test_result "Pods exist ($pod_count found)" "PASS"
    
    # Check each pod status
    local all_running=true
    local all_ready=true
    local pod_names=$(echo "$pods" | jq -r '.items[].metadata.name')
    
    while IFS= read -r pod_name; do
        if [ -n "$pod_name" ]; then
            local phase=$(kubectl get pod "$pod_name" -n "$NAMESPACE" -o jsonpath='{.status.phase}')
            local ready=$(kubectl get pod "$pod_name" -n "$NAMESPACE" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}')
            
            if [ "$phase" != "Running" ]; then
                all_running=false
                print_test_result "Pod $pod_name is Running" "FAIL" "Phase is: $phase"
            fi
            
            if [ "$ready" != "True" ]; then
                all_ready=false
                print_test_result "Pod $pod_name is Ready" "FAIL" "Ready condition is: $ready"
            fi
        fi
    done <<< "$pod_names"
    
    if [ "$all_running" = true ]; then
        print_test_result "All pods are Running" "PASS"
    fi
    
    if [ "$all_ready" = true ]; then
        print_test_result "All pods are Ready" "PASS"
    fi
}
test_pods_running

# Test 4: Check if service exists and has endpoints
print_section "Checking Kubernetes Service"
test_service_exists() {
    if kubectl get service "$RELEASE_NAME" -n "$NAMESPACE" > /dev/null 2>&1; then
        print_test_result "Service exists" "PASS"
        
        # Check service endpoints
        local endpoints=$(kubectl get endpoints "$RELEASE_NAME" -n "$NAMESPACE" -o jsonpath='{.subsets[*].addresses[*].ip}' | wc -w | tr -d ' ')
        
        if [ "$endpoints" -gt 0 ]; then
            print_test_result "Service has endpoints ($endpoints)" "PASS"
        else
            print_test_result "Service has endpoints" "FAIL" "No endpoints found for service"
        fi
    else
        print_test_result "Service exists" "FAIL" "Service '$RELEASE_NAME' not found in namespace '$NAMESPACE'"
    fi
}
test_service_exists

# Test 5: Test application endpoints via port-forward
print_section "Testing Application Endpoints"

# Start port-forward in background
echo "Setting up port-forward to service..."
PORT_FORWARD_PORT=8888
kubectl port-forward -n "$NAMESPACE" "svc/$RELEASE_NAME" "$PORT_FORWARD_PORT:80" > /dev/null 2>&1 &
PORT_FORWARD_PID=$!

# Ensure port-forward is killed on exit
cleanup_port_forward() {
    if [ -n "$PORT_FORWARD_PID" ]; then
        kill $PORT_FORWARD_PID 2>/dev/null || true
        wait $PORT_FORWARD_PID 2>/dev/null || true
    fi
}
trap cleanup_port_forward EXIT

# Wait for port-forward to be ready
echo "Waiting for port-forward to be ready..."
sleep 3

for i in {1..10}; do
    if curl -s -f -m 1 "http://localhost:$PORT_FORWARD_PORT/health" > /dev/null 2>&1; then
        echo -e "${GREEN}Port-forward is ready${NC}"
        break
    fi
    if [ $i -eq 10 ]; then
        echo -e "${RED}Port-forward failed to become ready${NC}"
        exit 1
    fi
    sleep 1
done

BASE_URL="http://localhost:$PORT_FORWARD_PORT"

# Test 5a: Health endpoint
test_health_endpoint() {
    local response
    response=$(curl -s -w "\n%{http_code}" -m $TIMEOUT "${BASE_URL}/health")
    local http_code=$(echo "$response" | tail -1)
    local body=$(echo "$response" | sed '$d')
    
    if [ "$http_code" = "200" ]; then
        # Check if response contains expected JSON
        local status=$(echo "$body" | jq -r '.status' 2>/dev/null)
        if [ "$status" = "healthy" ]; then
            print_test_result "/health endpoint responds with 200 and correct status" "PASS"
        else
            print_test_result "/health endpoint responds with 200 and correct status" "FAIL" "Status field is: $status"
        fi
    else
        print_test_result "/health endpoint responds with 200 and correct status" "FAIL" "Got HTTP $http_code"
    fi
}
test_health_endpoint

# Test 5b: Ready endpoint
test_ready_endpoint() {
    local response
    response=$(curl -s -w "\n%{http_code}" -m $TIMEOUT "${BASE_URL}/ready")
    local http_code=$(echo "$response" | tail -1)
    local body=$(echo "$response" | sed '$d')
    
    if [ "$http_code" = "200" ]; then
        local status=$(echo "$body" | jq -r '.status' 2>/dev/null)
        if [ "$status" = "ready" ]; then
            print_test_result "/ready endpoint responds with 200 and correct status" "PASS"
        else
            print_test_result "/ready endpoint responds with 200 and correct status" "FAIL" "Status field is: $status"
        fi
    else
        print_test_result "/ready endpoint responds with 200 and correct status" "FAIL" "Got HTTP $http_code"
    fi
}
test_ready_endpoint

# Test 5c: Root endpoint
test_root_endpoint() {
    local response
    response=$(curl -s -w "\n%{http_code}" -m $TIMEOUT "${BASE_URL}/")
    local http_code=$(echo "$response" | tail -1)
    local body=$(echo "$response" | sed '$d')
    
    if [ "$http_code" = "200" ]; then
        print_test_result "Root endpoint responds with 200 OK" "PASS"
    else
        print_test_result "Root endpoint responds with 200 OK" "FAIL" "Got HTTP $http_code"
        return
    fi
    
    # Validate JSON response
    if echo "$body" | jq . > /dev/null 2>&1; then
        print_test_result "Root endpoint returns valid JSON" "PASS"
    else
        print_test_result "Root endpoint returns valid JSON" "FAIL" "Response is not valid JSON"
        return
    fi
    
    # Check required fields
    local service_name=$(echo "$body" | jq -r '.service_name' 2>/dev/null)
    local service_version=$(echo "$body" | jq -r '.service_version' 2>/dev/null)
    local ip_address=$(echo "$body" | jq -r '.ip_address' 2>/dev/null)
    local instance_uuid=$(echo "$body" | jq -r '.instance_uuid' 2>/dev/null)
    local timestamp=$(echo "$body" | jq -r '.timestamp' 2>/dev/null)
    
    if [ "$service_name" != "null" ] && [ -n "$service_name" ]; then
        print_test_result "Root endpoint includes service_name" "PASS"
    else
        print_test_result "Root endpoint includes service_name" "FAIL" "Field is missing or null"
    fi
    
    if [ "$service_version" != "null" ] && [ -n "$service_version" ]; then
        print_test_result "Root endpoint includes service_version" "PASS"
    else
        print_test_result "Root endpoint includes service_version" "FAIL" "Field is missing or null"
    fi
    
    if [ "$ip_address" != "null" ] && [ -n "$ip_address" ]; then
        print_test_result "Root endpoint includes ip_address" "PASS"
    else
        print_test_result "Root endpoint includes ip_address" "FAIL" "Field is missing or null"
    fi
    
    if [ "$instance_uuid" != "null" ] && [ -n "$instance_uuid" ]; then
        # Validate UUID format
        if echo "$instance_uuid" | grep -qE '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'; then
            print_test_result "Root endpoint includes valid UUID" "PASS"
        else
            print_test_result "Root endpoint includes valid UUID" "FAIL" "UUID format is invalid: $instance_uuid"
        fi
    else
        print_test_result "Root endpoint includes valid UUID" "FAIL" "Field is missing or null"
    fi
    
    if [ "$timestamp" != "null" ] && [ -n "$timestamp" ]; then
        print_test_result "Root endpoint includes timestamp" "PASS"
    else
        print_test_result "Root endpoint includes timestamp" "FAIL" "Field is missing or null"
    fi
}
test_root_endpoint

# Test 5d: 404 for invalid path
test_404_response() {
    local response
    response=$(curl -s -w "\n%{http_code}" -m $TIMEOUT "${BASE_URL}/invalid-path")
    local http_code=$(echo "$response" | tail -1)
    
    if [ "$http_code" = "404" ]; then
        print_test_result "Invalid path returns 404" "PASS"
    else
        print_test_result "Invalid path returns 404" "FAIL" "Got HTTP $http_code"
    fi
}
test_404_response

# Test 6: Check pod logs for errors
print_section "Checking Pod Logs"
test_pod_logs() {
    local pods=$(kubectl get pods -n "$NAMESPACE" -l "app.kubernetes.io/name=goserv,app.kubernetes.io/instance=$RELEASE_NAME" -o jsonpath='{.items[0].metadata.name}')
    
    if [ -z "$pods" ]; then
        print_test_result "Pod logs are accessible" "FAIL" "No pods found"
        return
    fi
    
    local first_pod=$(echo "$pods" | head -n 1)
    
    if kubectl logs "$first_pod" -n "$NAMESPACE" --tail=50 > /dev/null 2>&1; then
        print_test_result "Pod logs are accessible" "PASS"
        
        # Check for common error patterns
        local error_count=$(kubectl logs "$first_pod" -n "$NAMESPACE" --tail=100 | grep -iE "(error|fatal|panic)" | wc -l | tr -d ' ')
        
        if [ "$error_count" -eq 0 ]; then
            print_test_result "Pod logs contain no errors" "PASS"
        else
            print_test_result "Pod logs contain no errors" "FAIL" "Found $error_count error-like messages in last 100 log lines"
        fi
    else
        print_test_result "Pod logs are accessible" "FAIL" "Could not retrieve logs from pod $first_pod"
    fi
}
test_pod_logs

# Cleanup port-forward
cleanup_port_forward

# Print summary
echo ""
echo -e "${BLUE}======================================${NC}"
echo -e "${BLUE}Validation Summary${NC}"
echo -e "${BLUE}======================================${NC}"
echo -e "Total Tests: ${TESTS_RUN}"
echo -e "${GREEN}Passed: ${TESTS_PASSED}${NC}"
echo -e "${RED}Failed: ${TESTS_FAILED}${NC}"
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}✓ All validation tests passed!${NC}"
    echo -e "${GREEN}The goserv application is properly deployed and healthy.${NC}"
    exit 0
else
    echo -e "${RED}✗ Some validation tests failed.${NC}"
    echo -e "${YELLOW}Please review the failures above and check your deployment.${NC}"
    exit 1
fi
