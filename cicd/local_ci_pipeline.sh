#!/bin/bash

################################################################################
# Local CI/CD Simulation Script
#
# This script simulates CI/CD pipeline execution using Dagger, similar to how
# it would run in GitHub Actions, GitLab CI, or CodeFresh.
#
# Usage:
#   ./local_ci_pipeline.sh --pipeline-type=branch    # Branch commit (build + test)
#   ./local_ci_pipeline.sh --pipeline-type=pr        # PR merge (build + test + deliver)
#   ./local_ci_pipeline.sh --release-candidate       # Build as release candidate
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
SKIP_TESTS=false
PIPELINE_TYPE="branch"
SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONTAINER_REPOSITORY="ttl.sh"
HELM_REPOSITORY="oci://ttl.sh"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --pipeline-type)
            PIPELINE_TYPE="$2"
            shift 2
            ;;
        --release-candidate|-rc)
            RELEASE_CANDIDATE=true
            shift
            ;;
        --skip-tests)
            SKIP_TESTS=true
            shift
            ;;
        --container-repository)
            CONTAINER_REPOSITORY="$2"
            shift 2
            ;;
        --helm-repository)
            HELM_REPOSITORY="$2"
            shift 2
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --pipeline-type <type>      Pipeline type: 'branch' (default) or 'pr'"
            echo "  --release-candidate, -rc    Build as release candidate (appends -rc to version)"
            echo "  --skip-tests                Skip unit tests"
            echo "  --container-repository      Container repository (default: ttl.sh)"
            echo "  --helm-repository           Helm repository (default: oci://ttl.sh)"
            echo "  --help, -h                  Show this help message"
            echo ""
            echo "Pipeline Types:"
            echo "  branch  - Simulates branch commit (Build + UnitTest)"
            echo "  pr      - Simulates PR merge (Build + UnitTest + Deliver)"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Run with --help for usage information"
            exit 1
            ;;
    esac
done

# Validate pipeline type
if [ "$PIPELINE_TYPE" != "branch" ] && [ "$PIPELINE_TYPE" != "pr" ]; then
    echo -e "${RED}Error: Invalid pipeline type '${PIPELINE_TYPE}'${NC}"
    echo "Valid types are: branch, pr"
    exit 1
fi

# Function to print section headers
print_section() {
    echo ""
    echo -e "${CYAN}========================================${NC}"
    echo -e "${CYAN}$1${NC}"
    echo -e "${CYAN}========================================${NC}"
    echo ""
}

# Function to print step headers
print_step() {
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}$1${NC}"
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

################################################################################
# Main Pipeline
################################################################################

print_section "CI/CD Pipeline Starting (${PIPELINE_TYPE})"
print_info "Source directory: $SOURCE_DIR"
print_info "Pipeline type: $PIPELINE_TYPE"
print_info "Release candidate: $RELEASE_CANDIDATE"
print_info "Skip tests: $SKIP_TESTS"

if [ "$PIPELINE_TYPE" = "pr" ]; then
    print_info "Container repository: $CONTAINER_REPOSITORY"
    print_info "Helm repository: $HELM_REPOSITORY"
fi

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
# Step 1: Build
################################################################################

print_step "Step 1: Build Docker Image"

BUILD_CMD="dagger -m cicd call build --source=$SOURCE_DIR"
if [ "$RELEASE_CANDIDATE" = true ]; then
    BUILD_CMD="$BUILD_CMD --release-candidate=true"
fi

print_info "Running: $BUILD_CMD"
echo ""

if eval "$BUILD_CMD" > /dev/null 2>&1; then
    print_success "Build completed successfully"
else
    print_error "Build failed"
    exit 1
fi

################################################################################
# Step 2: Unit Tests
################################################################################

if [ "$SKIP_TESTS" = false ]; then
    print_step "Step 2: Run Unit Tests"
    
    TEST_CMD="dagger -m cicd call unit-test --source=$SOURCE_DIR"
    print_info "Running: $TEST_CMD"
    echo ""
    
    if eval "$TEST_CMD"; then
        print_success "Unit tests passed"
    else
        print_error "Unit tests failed"
        exit 1
    fi
else
    print_warning "Step 2: Skipping unit tests (--skip-tests flag set)"
fi

################################################################################
# Step 3: Deliver (PR merge only)
################################################################################

if [ "$PIPELINE_TYPE" = "pr" ]; then
    print_step "Step 3: Deliver Artifacts"
    
    DELIVER_CMD="dagger -m cicd call deliver --source=$SOURCE_DIR"
    DELIVER_CMD="$DELIVER_CMD --container-repository=$CONTAINER_REPOSITORY"
    DELIVER_CMD="$DELIVER_CMD --helm-repository=$HELM_REPOSITORY"
    
    if [ "$RELEASE_CANDIDATE" = true ]; then
        DELIVER_CMD="$DELIVER_CMD --release-candidate=true"
    fi
    
    print_info "Running: $DELIVER_CMD"
    echo ""
    
    if eval "$DELIVER_CMD"; then
        print_success "Artifacts delivered successfully"
    else
        print_error "Delivery failed"
        exit 1
    fi
else
    print_info "Step 3: Skipping delivery (branch pipeline)"
fi

################################################################################
# Pipeline Summary
################################################################################

print_step "Pipeline Summary"

if [ "$PIPELINE_TYPE" = "branch" ]; then
    echo -e "${GREEN}✓${NC} Branch pipeline completed successfully"
    echo ""
    echo "Executed steps:"
    echo "  1. Build multi-architecture container image"
    if [ "$SKIP_TESTS" = false ]; then
        echo "  2. Run unit tests"
    else
        echo "  2. Unit tests skipped"
    fi
    echo ""
    echo "Branch pipelines validate code changes without publishing artifacts."
else
    echo -e "${GREEN}✓${NC} PR merge pipeline completed successfully"
    echo ""
    echo "Executed steps:"
    echo "  1. Build multi-architecture container image"
    if [ "$SKIP_TESTS" = false ]; then
        echo "  2. Run unit tests"
    else
        echo "  2. Unit tests skipped"
    fi
    echo "  3. Deliver artifacts to repositories"
    echo ""
    print_info "Published artifacts:"
    echo "  - Container images: $CONTAINER_REPOSITORY"
    echo "  - Helm chart: $HELM_REPOSITORY"
fi

echo ""
echo "For Integration Acceptance Testing (IAT), run:"
echo "  ./cicd/local_iat_pipeline.sh"
echo ""
