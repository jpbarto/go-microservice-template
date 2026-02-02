#!/bin/bash

################################################################################
# Local CI/CD Simulation Script
#
# This script simulates CI/CD pipeline execution using Dagger, similar to how
# it would run in GitHub Actions, GitLab CI, or CodeFresh.
#
# Usage:
#   ./local_ci.sh                    # Run full pipeline
#   ./local_ci.sh --release-candidate # Build as release candidate
#   ./local_ci.sh --skip-tests       # Skip unit tests
################################################################################

set -e  # Exit on error
set -u  # Exit on undefined variable

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default configuration
RELEASE_CANDIDATE=false
SKIP_TESTS=false
SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --release-candidate|-rc)
            RELEASE_CANDIDATE=true
            shift
            ;;
        --skip-tests)
            SKIP_TESTS=true
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --release-candidate, -rc    Build as release candidate (appends -rc to version)"
            echo "  --skip-tests                Skip unit tests"
            echo "  --help, -h                  Show this help message"
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
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}========================================${NC}"
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

print_section "CI/CD Pipeline Starting"
print_info "Source directory: $SOURCE_DIR"
print_info "Release candidate: $RELEASE_CANDIDATE"
print_info "Skip tests: $SKIP_TESTS"

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

print_section "Step 1: Build Docker Image"

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
    print_section "Step 2: Run Unit Tests"
    
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
# Pipeline Summary
################################################################################

print_section "CI/CD Pipeline Complete"
print_success "All steps completed successfully!"
echo ""
print_info "Next steps:"
if [ "$RELEASE_CANDIDATE" = true ]; then
    echo "  • Deliver release candidate: dagger -m cicd call deliver --source=$SOURCE_DIR --release-candidate=true"
else
    echo "  • Deliver to registry: dagger -m cicd call deliver --source=$SOURCE_DIR"
fi
echo "  • Deploy to Kubernetes: dagger -m cicd call deploy --source=$SOURCE_DIR --kubeconfig=file:~/.kube/config"
echo ""
