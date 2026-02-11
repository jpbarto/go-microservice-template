#!/bin/bash

################################################################################
# Local Deploy Pipeline Script
#
# This script simulates a production deployment pipeline using Dagger functions.
# It deploys the application to a Kubernetes cluster and validates the deployment.
#
# Prerequisites:
#   - Colima installed and configured (or other Kubernetes cluster)
#   - kubectl installed and configured
#   - Dagger CLI installed
#   - Published container image and Helm chart in repositories
#
# Usage:
#   ./local_deploy_pipeline.sh [OPTIONS]
#
# Options:
#   --release-candidate, -rc   Deploy release candidate version
#   --release-name <name>      Helm release name (default: goserv)
#   --namespace <name>         Kubernetes namespace (default: goserv)
#   --colima-profile <name>    Colima profile to use (default: acme-local)
#   --skip-validation          Skip validation step after deployment
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
NC='\033[0m' # No Color

# Default configuration
RELEASE_CANDIDATE=false
SKIP_VALIDATION=false
SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COLIMA_PROFILE="${COLIMA_PROFILE:-acme-local}"
KUBECTL_CONTEXT="colima-${COLIMA_PROFILE}"
RELEASE_NAME="${RELEASE_NAME:-goserv}"
NAMESPACE="${NAMESPACE:-goserv}"
HELM_REPOSITORY_URL="${HELM_REPOSITORY_URL:-oci://ttl.sh}"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --release-candidate|-rc)
            RELEASE_CANDIDATE=true
            shift
            ;;
        --skip-validation)
            SKIP_VALIDATION=true
            shift
            ;;
        --release-name=*)
            RELEASE_NAME="${1#*=}"
            shift
            ;;
        --release-name)
            RELEASE_NAME="$2"
            shift 2
            ;;
        --namespace=*)
            NAMESPACE="${1#*=}"
            shift
            ;;
        --namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        --colima-profile=*)
            COLIMA_PROFILE="${1#*=}"
            KUBECTL_CONTEXT="colima-${COLIMA_PROFILE}"
            shift
            ;;
        --colima-profile)
            COLIMA_PROFILE="$2"
            KUBECTL_CONTEXT="colima-${COLIMA_PROFILE}"
            shift 2
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --release-candidate, -rc   Deploy release candidate version"
            echo "  --skip-validation          Skip validation step after deployment"
            echo "  --release-name <name>      Helm release name (default: goserv)"
            echo "  --namespace <name>         Kubernetes namespace (default: goserv)"
            echo "  --colima-profile <name>    Colima profile to use (default: acme-local)"
            echo "  --help, -h                 Show this help message"
            echo ""
            echo "Description:"
            echo "  This pipeline deploys the goserv application to a Kubernetes cluster"
            echo "  using the Deploy Dagger function and validates the deployment."
            echo ""
            echo "Examples:"
            echo "  # Deploy to local Colima cluster"
            echo "  ./local_deploy_pipeline.sh"
            echo ""
            echo "  # Deploy release candidate"
            echo "  ./local_deploy_pipeline.sh --release-candidate"
            echo ""
            echo "  # Deploy to specific namespace"
            echo "  ./local_deploy_pipeline.sh --namespace=production"
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

print_section "Deploy Pipeline Starting"
print_info "Source directory: $SOURCE_DIR"
print_info "Colima profile: $COLIMA_PROFILE"
print_info "Kubectl context: $KUBECTL_CONTEXT"
print_info "Release name: $RELEASE_NAME"
print_info "Namespace: $NAMESPACE"
print_info "Release candidate: $RELEASE_CANDIDATE"
print_info "Helm repository: $HELM_REPOSITORY_URL"

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
# Step 1: Verify Colima Environment
################################################################################

print_step "Step 1: Verify Kubernetes Environment"

# Check if colima is installed
if command -v colima &> /dev/null; then
    print_success "Colima is installed"
    
    # Check if profile exists and is running
    if colima list | grep -q "^${COLIMA_PROFILE}"; then
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
    else
        print_warning "Colima profile '${COLIMA_PROFILE}' does not exist"
        echo ""
        echo "To create the profile, run:"
        echo "  colima start ${COLIMA_PROFILE} --cpu 4 --memory 8 --disk 50 --kubernetes"
        echo ""
        echo "Or use a different profile with: --colima-profile=<profile-name>"
    fi
else
    print_info "Colima not found - assuming external Kubernetes cluster"
fi

################################################################################
# Step 2: Verify kubectl Context
################################################################################

print_step "Step 2: Verify kubectl Context"

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
if kubectl config get-contexts "${KUBECTL_CONTEXT}" &> /dev/null; then
    # Get current context
    CURRENT_CONTEXT=$(kubectl config current-context)
    if [ "$CURRENT_CONTEXT" != "$KUBECTL_CONTEXT" ]; then
        print_warning "Current kubectl context is '${CURRENT_CONTEXT}', switching to '${KUBECTL_CONTEXT}'..."
        kubectl config use-context "${KUBECTL_CONTEXT}"
        print_success "Switched to kubectl context '${KUBECTL_CONTEXT}'"
    else
        print_success "Kubectl context '${KUBECTL_CONTEXT}' is already selected"
    fi
else
    print_warning "Kubectl context '${KUBECTL_CONTEXT}' does not exist"
    print_info "Using current context: $(kubectl config current-context)"
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

print_step "Step 3: Deploy Application"

# Build Dagger Deploy command
DEPLOY_CMD="dagger -m cicd call deploy --source=${SOURCE_DIR}"
DEPLOY_CMD="${DEPLOY_CMD} --kubeconfig=file:${HOME}/.kube/config"
DEPLOY_CMD="${DEPLOY_CMD} --release-name=${RELEASE_NAME}"
DEPLOY_CMD="${DEPLOY_CMD} --namespace=${NAMESPACE}"
DEPLOY_CMD="${DEPLOY_CMD} --helm-repository=${HELM_REPOSITORY_URL}"

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

################################################################################
# Step 4: Validate Deployment using Dagger
################################################################################

if [ "$SKIP_VALIDATION" = false ]; then
    print_step "Step 4: Validate Deployment"
    
    # Build Dagger Validate command
    VALIDATE_CMD="dagger -m cicd call validate --source=${SOURCE_DIR}"
    VALIDATE_CMD="${VALIDATE_CMD} --kubeconfig=file:${HOME}/.kube/config"
    VALIDATE_CMD="${VALIDATE_CMD} --release-name=${RELEASE_NAME}"
    VALIDATE_CMD="${VALIDATE_CMD} --namespace=${NAMESPACE}"
    
    if [ "$RELEASE_CANDIDATE" = true ]; then
        VALIDATE_CMD="${VALIDATE_CMD} --release-candidate=true"
    fi
    
    print_info "Running: ${VALIDATE_CMD}"
    echo ""
    
    if eval "$VALIDATE_CMD"; then
        print_success "Deployment validation passed"
    else
        print_error "Deployment validation failed"
        exit 1
    fi
else
    print_warning "Step 4: Skipping validation (--skip-validation flag set)"
fi

################################################################################
# Pipeline Summary
################################################################################

print_section "Deploy Pipeline Complete"

print_success "Application deployed and validated successfully!"
echo ""
print_info "Deployment Details:"
echo "  • Namespace: ${NAMESPACE}"
echo "  • Release: ${RELEASE_NAME}"
echo "  • Version: ${VERSION}"
echo "  • Chart: ${HELM_REPOSITORY_URL}/charts/goserv:${VERSION}"
echo ""
print_info "Access the Application:"
echo "  • Port-forward:"
echo "    kubectl port-forward -n ${NAMESPACE} svc/${RELEASE_NAME} 8080:80"
echo ""
echo "  • Then visit: http://localhost:8080"
echo ""
print_info "Check Deployment Status:"
echo "  • View pods:"
echo "    kubectl get pods -n ${NAMESPACE}"
echo ""
echo "  • View services:"
echo "    kubectl get svc -n ${NAMESPACE}"
echo ""
echo "  • View logs:"
echo "    kubectl logs -n ${NAMESPACE} -l app.kubernetes.io/name=goserv"
echo ""
print_info "Manage Deployment:"
echo "  • Upgrade to new version:"
echo "    helm upgrade ${RELEASE_NAME} ${HELM_REPOSITORY_URL}/charts/goserv --version <new-version> -n ${NAMESPACE}"
echo ""
echo "  • Uninstall:"
echo "    helm uninstall ${RELEASE_NAME} -n ${NAMESPACE}"
echo ""

exit 0
