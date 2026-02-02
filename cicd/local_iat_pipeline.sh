#!/bin/bash

################################################################################
# Local IAT (Integration and Acceptance Testing) Pipeline Script
#
# This script simulates a CI/CD pipeline IAT stage using Dagger functions.
# It ensures the local Kubernetes environment is running, deploys the application,
# and executes integration tests.
#
# Prerequisites:
#   - Colima installed and configured
#   - kubectl installed
#   - Dagger CLI installed
#   - pismolocal colima profile exists
#
# Usage:
#   ./local_iat_pipeline.sh [OPTIONS]
#
# Options:
#   --release-candidate, -rc    Deploy and test release candidate version
#   --skip-deploy              Skip deployment step (use existing deployment)
#   --help, -h                 Show this help message
################################################################################

set -e  # Exit on error
set -u  # Exit on undefined variable

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Default configuration
RELEASE_CANDIDATE=false
SKIP_DEPLOY=false
SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COLIMA_PROFILE="pismolocal"
KUBECTL_CONTEXT="colima-${COLIMA_PROFILE}"
NAMESPACE="default"
RELEASE_NAME="goserv"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --release-candidate|-rc)
            RELEASE_CANDIDATE=true
            shift
            ;;
        --skip-deploy)
            SKIP_DEPLOY=true
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --release-candidate, -rc    Deploy and test release candidate version"
            echo "  --skip-deploy              Skip deployment step (use existing deployment)"
            echo "  --help, -h                 Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Run with --help for usage information"
            exit 1
            ;;
    esac
done

# Function to print section headers
print_section() {
    echo ""
    echo -e "${CYAN}========================================${NC}"
    echo -e "${CYAN}$1${NC}"
    echo -e "${CYAN}========================================${NC}"
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

# Function to print step headers
print_step() {
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

################################################################################
# Main Pipeline
################################################################################

print_section "Local IAT Pipeline Starting"
print_info "Source directory: $SOURCE_DIR"
print_info "Colima profile: $COLIMA_PROFILE"
print_info "Kubectl context: $KUBECTL_CONTEXT"
print_info "Namespace: $NAMESPACE"
print_info "Release name: $RELEASE_NAME"
print_info "Release candidate: $RELEASE_CANDIDATE"
print_info "Skip deploy: $SKIP_DEPLOY"

# Read VERSION file
if [ -f "$SOURCE_DIR/VERSION" ]; then
    VERSION=$(cat "$SOURCE_DIR/VERSION" | tr -d '[:space:]')
    if [ "$RELEASE_CANDIDATE" = true ]; then
        VERSION="${VERSION}-rc"
    fi
    print_info "Version: $VERSION"
else
    print_error "VERSION file not found"
    exit 1
fi

################################################################################
# Step 1: Ensure Colima Environment is Running
################################################################################

print_step "Step 1: Verify Colima Environment"

# Check if colima is installed
if ! command -v colima &> /dev/null; then
    print_error "Colima is not installed"
    echo ""
    echo "Please install colima:"
    echo "  brew install colima"
    exit 1
fi
print_success "Colima is installed"

# Check if pismolocal profile exists and is running
if ! colima list | grep -q "^${COLIMA_PROFILE}"; then
    print_error "Colima profile '${COLIMA_PROFILE}' does not exist"
    echo ""
    echo "Please create the colima profile:"
    echo "  colima start ${COLIMA_PROFILE} --cpu 4 --memory 8 --disk 50 --kubernetes"
    exit 1
fi

# Check if profile is running
COLIMA_STATUS=$(colima list | grep "^${COLIMA_PROFILE}" | awk '{print $2}')
if [ "$COLIMA_STATUS" != "Running" ]; then
    print_warning "Colima profile '${COLIMA_PROFILE}' is not running (status: ${COLIMA_STATUS})"
    print_info "Starting colima profile '${COLIMA_PROFILE}'..."
    
    if colima start "${COLIMA_PROFILE}"; then
        print_success "Colima profile '${COLIMA_PROFILE}' started successfully"
    else
        print_error "Failed to start colima profile '${COLIMA_PROFILE}'"
        exit 1
    fi
else
    print_success "Colima profile '${COLIMA_PROFILE}' is running"
fi

################################################################################
# Step 2: Verify and Set Kubectl Context
################################################################################

print_step "Step 2: Verify Kubectl Context"

# Check if kubectl is installed
if ! command -v kubectl &> /dev/null; then
    print_error "kubectl is not installed"
    echo ""
    echo "Please install kubectl:"
    echo "  brew install kubectl"
    exit 1
fi
print_success "kubectl is installed"

# Check if context exists
if ! kubectl config get-contexts "${KUBECTL_CONTEXT}" &> /dev/null; then
    print_error "Kubectl context '${KUBECTL_CONTEXT}' does not exist"
    echo ""
    echo "Available contexts:"
    kubectl config get-contexts
    exit 1
fi

# Get current context
CURRENT_CONTEXT=$(kubectl config current-context)
if [ "$CURRENT_CONTEXT" != "$KUBECTL_CONTEXT" ]; then
    print_warning "Current kubectl context is '${CURRENT_CONTEXT}', switching to '${KUBECTL_CONTEXT}'..."
    kubectl config use-context "${KUBECTL_CONTEXT}"
    print_success "Switched to kubectl context '${KUBECTL_CONTEXT}'"
else
    print_success "Kubectl context '${KUBECTL_CONTEXT}' is already selected"
fi

# Verify cluster connectivity
if kubectl cluster-info &> /dev/null; then
    print_success "Successfully connected to Kubernetes cluster"
else
    print_error "Failed to connect to Kubernetes cluster"
    exit 1
fi

################################################################################
# Step 3: Deploy Application using Dagger
################################################################################

if [ "$SKIP_DEPLOY" = false ]; then
    print_step "Step 3: Deploy Application"
    
    # Build Dagger Deploy command
    DEPLOY_CMD="dagger -m cicd call deploy --source=${SOURCE_DIR}"
    DEPLOY_CMD="${DEPLOY_CMD} --kubeconfig=file:${HOME}/.kube/config"
    DEPLOY_CMD="${DEPLOY_CMD} --release-name=${RELEASE_NAME}"
    DEPLOY_CMD="${DEPLOY_CMD} --namespace=${NAMESPACE}"
    DEPLOY_CMD="${DEPLOY_CMD} --container-repository=ttl.sh"
    
    if [ "$RELEASE_CANDIDATE" = true ]; then
        DEPLOY_CMD="${DEPLOY_CMD} --release-candidate=true"
    fi
    
    print_info "Running: ${DEPLOY_CMD}"
    echo ""
    
    if eval "$DEPLOY_CMD"; then
        print_success "Application deployed successfully"
    else
        print_error "Deployment failed"
        exit 1
    fi
    
    # Wait for deployment to be ready
    print_info "Waiting for deployment to be ready..."
    if kubectl rollout status deployment/${RELEASE_NAME}-goserv -n ${NAMESPACE} --timeout=120s; then
        print_success "Deployment is ready"
    else
        print_error "Deployment did not become ready in time"
        echo ""
        echo "Pod status:"
        kubectl get pods -n ${NAMESPACE} -l app.kubernetes.io/name=goserv
        exit 1
    fi
else
    print_warning "Step 3: Skipping deployment (--skip-deploy flag set)"
    
    # Verify existing deployment
    print_info "Verifying existing deployment..."
    if kubectl get deployment/${RELEASE_NAME}-goserv -n ${NAMESPACE} &> /dev/null; then
        print_success "Deployment '${RELEASE_NAME}-goserv' exists"
    else
        print_error "Deployment '${RELEASE_NAME}-goserv' not found in namespace '${NAMESPACE}'"
        exit 1
    fi
fi

################################################################################
# Step 4: Set Up Port Forward for Testing
################################################################################

print_step "Step 4: Set Up Port Forward"

# Find an available local port
LOCAL_PORT=8080
while lsof -Pi :${LOCAL_PORT} -sTCP:LISTEN -t >/dev/null 2>&1; do
    LOCAL_PORT=$((LOCAL_PORT + 1))
done

print_info "Using local port: ${LOCAL_PORT}"

# Start port-forward in background
print_info "Starting port-forward to ${RELEASE_NAME}-goserv service..."
kubectl port-forward -n ${NAMESPACE} svc/${RELEASE_NAME}-goserv ${LOCAL_PORT}:80 &> /tmp/kubectl-port-forward.log &
PORT_FORWARD_PID=$!

# Function to cleanup port-forward on exit
cleanup_port_forward() {
    if [ -n "${PORT_FORWARD_PID:-}" ]; then
        print_info "Stopping port-forward (PID: ${PORT_FORWARD_PID})..."
        kill ${PORT_FORWARD_PID} 2>/dev/null || true
        wait ${PORT_FORWARD_PID} 2>/dev/null || true
    fi
}

trap cleanup_port_forward EXIT

# Wait for port-forward to be ready
print_info "Waiting for port-forward to be ready..."
MAX_RETRIES=30
RETRY_COUNT=0
while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    if curl -s -f -o /dev/null --max-time 1 "http://localhost:${LOCAL_PORT}/health" 2>/dev/null; then
        print_success "Port-forward is ready"
        break
    fi
    RETRY_COUNT=$((RETRY_COUNT + 1))
    sleep 1
done

if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
    print_error "Port-forward did not become ready in time"
    echo ""
    echo "Port-forward logs:"
    cat /tmp/kubectl-port-forward.log
    exit 1
fi

################################################################################
# Step 5: Run Integration Tests using Dagger
################################################################################

print_step "Step 5: Run Integration Tests"

# Build Dagger IntegrationTest command
# Use host.docker.internal to reach localhost from inside Dagger container
TEST_CMD="dagger -m cicd call integration-test --source=${SOURCE_DIR}"
TEST_CMD="${TEST_CMD} --target-host=host.docker.internal"
TEST_CMD="${TEST_CMD} --target-port=${LOCAL_PORT}"

print_info "Running: ${TEST_CMD}"
echo ""

if eval "$TEST_CMD"; then
    print_success "Integration tests passed"
    TEST_RESULT=0
else
    print_error "Integration tests failed"
    TEST_RESULT=1
fi

################################################################################
# Pipeline Summary
################################################################################

print_section "IAT Pipeline Complete"

if [ $TEST_RESULT -eq 0 ]; then
    print_success "All steps completed successfully!"
    echo ""
    print_info "Deployment details:"
    echo "  • Namespace: ${NAMESPACE}"
    echo "  • Release: ${RELEASE_NAME}"
    echo "  • Version: ${VERSION}"
    echo ""
    print_info "Cleanup:"
    echo "  • To remove deployment: helm uninstall ${RELEASE_NAME} -n ${NAMESPACE}"
    echo "  • To stop colima: colima stop ${COLIMA_PROFILE}"
    echo ""
    exit 0
else
    print_error "Integration tests failed"
    echo ""
    print_info "Troubleshooting:"
    echo "  • Check pod logs: kubectl logs -n ${NAMESPACE} -l app.kubernetes.io/name=goserv"
    echo "  • Check pod status: kubectl get pods -n ${NAMESPACE}"
    echo "  • Check service: kubectl get svc -n ${NAMESPACE} ${RELEASE_NAME}-goserv"
    echo ""
    exit 1
fi
