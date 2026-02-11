#!/bin/bash

################################################################################
# Local Staging Pipeline Script
#
# This script simulates a CI/CD pipeline staging stage using Dagger functions.
# It performs blue-green deployment testing by:
# 1. Deploying and validating the current version (green)
# 2. Deploying and validating the previous release tag (blue)
# 3. Re-deploying and validating the current version (green)
#
# This validates that rollback scenarios work correctly and that both
# versions can coexist during deployment.
#
# Prerequisites:
#   - Colima installed and configured
#   - kubectl installed
#   - Dagger CLI installed
#   - acme-local colima profile exists
#   - At least one git tag exists in the repository
#
# Usage:
#   ./local_staging_pipeline.sh [OPTIONS]
#
# Options:
#   --help, -h                 Show this help message
#
################################################################################

set -e  # Exit on error
set -u  # Exit on undefined variable

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Load environment variables from local_cicd.env in the script's directory
if [ -f "$SCRIPT_DIR/local_cicd.env" ]; then
    export $(grep -v '^#' "$SCRIPT_DIR/local_cicd.env" | xargs)
fi

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
NC='\033[0m' # No Color

# Default configuration
RELEASE_CANDIDATE=true  # Staging always uses release candidate builds
SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COLIMA_PROFILE="acme-local"
KUBECTL_CONTEXT="colima-${COLIMA_PROFILE}"
RELEASE_NAME="goserv"
NAMESPACE="goserv"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "This script performs blue-green deployment testing:"
            echo "  1. Deploy and validate current version"
            echo "  2. Deploy and validate previous release tag"
            echo "  3. Re-deploy and validate current version"
            echo ""
            echo "Options:"
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

# Function to print deployment phase headers
print_phase() {
    echo ""
    echo -e "${MAGENTA}╔════════════════════════════════════════╗${NC}"
    echo -e "${MAGENTA}║  $1${NC}"
    echo -e "${MAGENTA}╚════════════════════════════════════════╝${NC}"
    echo ""
}

# Function to deploy and validate a version
deploy_and_validate() {
    local version_label="$1"
    local version="$2"
    local is_release_candidate="$3"  # true or false
    
    print_phase "Deploying $version_label (v$version)"
    
    # Build Dagger Deploy command
    DEPLOY_CMD="dagger -m cicd call deploy --source=${SOURCE_DIR}"
    DEPLOY_CMD="${DEPLOY_CMD} --kubeconfig=file:${HOME}/.kube/config"
    DEPLOY_CMD="${DEPLOY_CMD} --release-name=${RELEASE_NAME}"
    DEPLOY_CMD="${DEPLOY_CMD} --namespace=${NAMESPACE}"
    DEPLOY_CMD="${DEPLOY_CMD} --helm-repository=${HELM_REPOSITORY_URL}"
    
    if [ "$is_release_candidate" = "true" ]; then
        DEPLOY_CMD="${DEPLOY_CMD} --release-candidate=true"
    fi
    
    print_info "Running: ${DEPLOY_CMD}"
    echo ""
    
    if eval "$DEPLOY_CMD"; then
        print_success "Deployment completed"
    else
        print_error "Deployment failed"
        return 1
    fi
    
    # Wait for deployment to be ready
    print_info "Waiting for deployment rollout..."
    if kubectl rollout status deployment/${RELEASE_NAME} -n ${NAMESPACE} --timeout=120s; then
        print_success "Deployment is ready"
    else
        print_error "Deployment did not become ready in time"
        kubectl get pods -n ${NAMESPACE} -l app.kubernetes.io/name=goserv
        return 1
    fi
    
    # Run validation
    print_info "Running validation tests..."
    echo ""
    
    VALIDATE_CMD="dagger -m cicd call validate --source=${SOURCE_DIR}"
    VALIDATE_CMD="${VALIDATE_CMD} --kubeconfig=file:${HOME}/.kube/config"
    VALIDATE_CMD="${VALIDATE_CMD} --release-name=${RELEASE_NAME}"
    VALIDATE_CMD="${VALIDATE_CMD} --namespace=${NAMESPACE}"
    
    if [ "$is_release_candidate" = "true" ]; then
        VALIDATE_CMD="${VALIDATE_CMD} --release-candidate=true"
    fi
    
    print_info "Running: ${VALIDATE_CMD}"
    echo ""
    
    if eval "$VALIDATE_CMD"; then
        print_success "Validation passed for $version_label"
    else
        print_error "Validation failed for $version_label"
        return 1
    fi
}

################################################################################
# Main Pipeline
################################################################################

print_section "Local Staging Pipeline Starting"
print_info "Source directory: $SOURCE_DIR"
print_info "Colima profile: $COLIMA_PROFILE"
print_info "Kubectl context: $KUBECTL_CONTEXT"
print_info "Release name: $RELEASE_NAME"
print_info "Namespace: $NAMESPACE"
print_info "Release candidate: $RELEASE_CANDIDATE"

# Save current branch/commit
CURRENT_REF=$(git rev-parse --abbrev-ref HEAD)
if [ "$CURRENT_REF" = "HEAD" ]; then
    # Detached HEAD state, save commit hash
    CURRENT_REF=$(git rev-parse HEAD)
fi
print_info "Current ref: $CURRENT_REF"

# Read VERSION file for current version
if [ -f "$SOURCE_DIR/VERSION" ]; then
    CURRENT_VERSION=$(cat "$SOURCE_DIR/VERSION" | tr -d '[:space:]')
    if [ "$RELEASE_CANDIDATE" = true ]; then
        CURRENT_VERSION="${CURRENT_VERSION}-rc"
    fi
    print_info "Current version: $CURRENT_VERSION"
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

# Check if acme-local profile exists and is running
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
# Step 3: Identify Previous Release Tag
################################################################################

print_step "Step 3: Identify Previous Release Tag"

# Get the most recent git tag
PREVIOUS_TAG=$(git tag --sort=-v:refname | head -1)

if [ -z "$PREVIOUS_TAG" ]; then
    print_error "No git tags found in repository"
    echo ""
    echo "This pipeline requires at least one git tag to test rollback scenarios."
    echo "Please create a tag first:"
    echo "  git tag -a 1.0.0 -m 'Release 1.0.0'"
    exit 1
fi

print_success "Found previous release tag: $PREVIOUS_TAG"

################################################################################
# Phase 1: Deploy and Validate Current Version (Green Deployment)
################################################################################

deploy_and_validate "Current Version (Green)" "$CURRENT_VERSION" "true"

################################################################################
# Phase 2: Deploy and Validate Previous Release (Blue Deployment)
################################################################################

print_phase "Switching to Previous Release"
print_info "Checking out tag: $PREVIOUS_TAG"

# Checkout the previous release tag
if git checkout "$PREVIOUS_TAG" 2>/dev/null; then
    print_success "Checked out $PREVIOUS_TAG"
else
    print_error "Failed to checkout $PREVIOUS_TAG"
    git checkout "$CURRENT_REF"
    exit 1
fi

# Read VERSION from the previous release
if [ -f "$SOURCE_DIR/VERSION" ]; then
    PREVIOUS_VERSION=$(cat "$SOURCE_DIR/VERSION" | tr -d '[:space:]')
else
    print_error "VERSION file not found in $PREVIOUS_TAG"
    git checkout "$CURRENT_REF"
    exit 1
fi

deploy_and_validate "Previous Release (Blue)" "$PREVIOUS_VERSION" "false"

################################################################################
# Phase 3: Re-deploy and Validate Current Version (Back to Green)
################################################################################

print_phase "Returning to Current Version"
print_info "Checking out: $CURRENT_REF"

# Return to original ref
if git checkout "$CURRENT_REF" 2>/dev/null; then
    print_success "Returned to $CURRENT_REF"
else
    print_error "Failed to checkout $CURRENT_REF"
    exit 1
fi

deploy_and_validate "Current Version (Green - Final)" "$CURRENT_VERSION" "true"

################################################################################
# Pipeline Summary
################################################################################

print_section "Staging Pipeline Complete"

print_success "All deployment phases completed successfully!"
echo ""
print_info "Blue-Green Deployment Summary:"
echo "  • Phase 1: Current version (v${CURRENT_VERSION}) - ✓ Deployed and Validated"
echo "  • Phase 2: Previous release (v${PREVIOUS_VERSION}) - ✓ Deployed and Validated"
echo "  • Phase 3: Current version (v${CURRENT_VERSION}) - ✓ Re-deployed and Validated"
echo ""
print_info "This validates that:"
echo "  ✓ Current version deploys successfully"
echo "  ✓ Rollback to previous release works correctly"
echo "  ✓ Re-deployment of current version is stable"
echo ""
print_info "Deployment details:"
echo "  • Namespace: ${NAMESPACE}"
echo "  • Release: ${RELEASE_NAME}"
echo "  • Active Version: ${CURRENT_VERSION}"
echo ""
print_info "Cleanup:"
echo "  • To remove deployment: helm uninstall ${RELEASE_NAME} -n ${NAMESPACE}"
echo "  • To stop colima: colima stop ${COLIMA_PROFILE}"
echo ""

exit 0
