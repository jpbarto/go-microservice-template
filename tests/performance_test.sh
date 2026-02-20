#!/bin/bash

################################################################################
# Performance Test Script using k6
#
# This script tests the performance of the goserv application using k6.
# It verifies that all endpoints respond within 500ms under load.
#
# Prerequisites:
#   - k6 installed (https://k6.io/docs/getting-started/installation/)
#   - goserv application running
#
# Usage:
#   ./performance_test.sh [BASE_URL]
#
# Examples:
#   ./performance_test.sh http://localhost:8080
#   ./performance_test.sh http://myservice.local:80
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

# Performance thresholds
MAX_RESPONSE_TIME=500  # milliseconds
REQUESTS_PER_SECOND=10
TEST_DURATION="120s"

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}k6 Performance Test for goserv${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""
echo -e "${BLUE}Configuration:${NC}"
echo -e "  Base URL: ${BASE_URL}"
echo -e "  Max Response Time: ${MAX_RESPONSE_TIME}ms"
echo -e "  Load: ${REQUESTS_PER_SECOND} requests/second"
echo -e "  Duration: ${TEST_DURATION}"
echo ""

# Check if k6 is installed
if ! command -v k6 &> /dev/null; then
    echo -e "${RED}Error: k6 is not installed${NC}"
    echo ""
    echo "Please install k6 from https://k6.io/docs/getting-started/installation/"
    echo ""
    echo "macOS:"
    echo "  brew install k6"
    echo ""
    echo "Linux:"
    echo "  sudo gpg -k"
    echo "  sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69"
    echo "  echo \"deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main\" | sudo tee /etc/apt/sources.list.d/k6.list"
    echo "  sudo apt-get update"
    echo "  sudo apt-get install k6"
    exit 1
fi

# Check if service is reachable
echo -e "${BLUE}Checking if service is available...${NC}"
if ! curl -s -f -o /dev/null --max-time 5 "${BASE_URL}/health"; then
    echo -e "${RED}Error: Service is not reachable at ${BASE_URL}${NC}"
    echo "Please ensure the goserv application is running"
    exit 1
fi
echo -e "${GREEN}✓ Service is available${NC}"
echo ""

# Create k6 test script in current directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
K6_SCRIPT="${PROJECT_ROOT}/.k6-test-$$.js"

cat > "${K6_SCRIPT}" << 'EOF'
import http from 'k6/http';
import { check, group } from 'k6';
import { Rate } from 'k6/metrics';

// Custom metrics
const errorRate = new Rate('errors');
const slowResponseRate = new Rate('slow_responses');

// Test configuration
export const options = {
    scenarios: {
        constant_load: {
            executor: 'constant-arrival-rate',
            rate: __ENV.REQUESTS_PER_SECOND,
            timeUnit: '1s',
            duration: __ENV.TEST_DURATION,
            preAllocatedVUs: 10,
            maxVUs: 50,
        },
    },
    thresholds: {
        'http_req_duration': ['p(95)<' + __ENV.MAX_RESPONSE_TIME],
        'http_req_failed': ['rate<0.01'],  // Less than 1% errors
        'errors': ['rate<0.01'],
        'slow_responses': ['rate<0.05'],  // Less than 5% slow responses
    },
};

const BASE_URL = __ENV.BASE_URL;
const MAX_RESPONSE_TIME = parseInt(__ENV.MAX_RESPONSE_TIME);

export default function() {
    // Test root endpoint
    group('Root Endpoint', function() {
        const response = http.get(`${BASE_URL}/`);
        
        const success = check(response, {
            'status is 200': (r) => r.status === 200,
            'response time < threshold': (r) => r.timings.duration < MAX_RESPONSE_TIME,
            'content-type is JSON': (r) => r.headers['Content-Type'] && r.headers['Content-Type'].includes('application/json'),
            'has service_name': (r) => {
                try {
                    const body = JSON.parse(r.body);
                    return body.service_name !== undefined;
                } catch (e) {
                    return false;
                }
            },
            'has service_version': (r) => {
                try {
                    const body = JSON.parse(r.body);
                    return body.service_version !== undefined;
                } catch (e) {
                    return false;
                }
            },
            'has instance_uuid': (r) => {
                try {
                    const body = JSON.parse(r.body);
                    return body.instance_uuid !== undefined;
                } catch (e) {
                    return false;
                }
            },
        });
        
        errorRate.add(!success);
        slowResponseRate.add(response.timings.duration >= MAX_RESPONSE_TIME);
    });
    
    // Test health endpoint
    group('Health Endpoint', function() {
        const response = http.get(`${BASE_URL}/health`);
        
        const success = check(response, {
            'status is 200': (r) => r.status === 200,
            'response time < threshold': (r) => r.timings.duration < MAX_RESPONSE_TIME,
        });
        
        errorRate.add(!success);
        slowResponseRate.add(response.timings.duration >= MAX_RESPONSE_TIME);
    });
    
    // Test ready endpoint
    group('Ready Endpoint', function() {
        const response = http.get(`${BASE_URL}/ready`);
        
        const success = check(response, {
            'status is 200': (r) => r.status === 200,
            'response time < threshold': (r) => r.timings.duration < MAX_RESPONSE_TIME,
        });
        
        errorRate.add(!success);
        slowResponseRate.add(response.timings.duration >= MAX_RESPONSE_TIME);
    });
}
EOF

# Verify the script file was created
if [ ! -f "${K6_SCRIPT}" ]; then
    echo -e "${RED}Error: Failed to create k6 test script${NC}"
    exit 1
fi

# Run k6 test
echo -e "${BLUE}Running performance test...${NC}"
echo ""

# Debug: verify file exists and show its path
if [ ! -f "${K6_SCRIPT}" ]; then
    echo -e "${RED}Error: k6 script file not found at: ${K6_SCRIPT}${NC}"
    exit 1
fi
echo -e "${BLUE}k6 script: ${K6_SCRIPT}${NC}"
echo -e "${BLUE}File size: $(wc -c < "${K6_SCRIPT}") bytes${NC}"
echo ""

export BASE_URL
export MAX_RESPONSE_TIME
export REQUESTS_PER_SECOND
export TEST_DURATION

# Run k6 - handle both native binary and Docker wrapper
# If k6 is a Docker wrapper, it needs the right volume mount
if grep -q "docker run" "$(which k6)" 2>/dev/null; then
    # k6 is a Docker wrapper - call docker directly with proper mount
    # Replace localhost with host.docker.internal for Docker networking
    DOCKER_BASE_URL="${BASE_URL/localhost/host.docker.internal}"
    
    docker run --rm \
        -v "${PROJECT_ROOT}:/src" \
        --workdir /src \
        -e BASE_URL="${DOCKER_BASE_URL}" \
        -e MAX_RESPONSE_TIME="${MAX_RESPONSE_TIME}" \
        -e REQUESTS_PER_SECOND="${REQUESTS_PER_SECOND}" \
        -e TEST_DURATION="${TEST_DURATION}" \
        grafana/k6 run --quiet ".k6-test-$$.js" > /tmp/k6-output.txt 2>&1
    K6_EXIT=$?
    cat /tmp/k6-output.txt
    rm -f /tmp/k6-output.txt
else
    # k6 is a native binary
    k6 run --quiet "${K6_SCRIPT}"
    K6_EXIT=$?
fi

if [ ${K6_EXIT} -eq 0 ]; then
    EXIT_CODE=0
    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}Performance Test PASSED${NC}"
    echo -e "${GREEN}========================================${NC}"
else
    EXIT_CODE=1
    echo ""
    echo -e "${RED}========================================${NC}"
    echo -e "${RED}Performance Test FAILED${NC}"
    echo -e "${RED}========================================${NC}"
fi

# Cleanup
rm -f "${K6_SCRIPT}"

exit ${EXIT_CODE}
