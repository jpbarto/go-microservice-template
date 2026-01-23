#!/bin/bash
set -e

# Build script for goserv
# This script builds the Docker image using the Dockerfile in the repository root

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Default values
IMAGE_NAME="${IMAGE_NAME:-goserv}"
IMAGE_TAG="${IMAGE_TAG:-latest}"
REGISTRY="${REGISTRY:-}"

# If registry is provided, prefix the image name
if [ -n "$REGISTRY" ]; then
    FULL_IMAGE_NAME="${REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}"
else
    FULL_IMAGE_NAME="${IMAGE_NAME}:${IMAGE_TAG}"
fi

echo "Building Docker image: ${FULL_IMAGE_NAME}"
echo "Repository root: ${REPO_ROOT}"

# Build the Docker image
cd "$REPO_ROOT"
docker build -t "${FULL_IMAGE_NAME}" .

echo "Successfully built image: ${FULL_IMAGE_NAME}"

# Optionally push to registry if specified
if [ -n "$REGISTRY" ] && [ "${PUSH_IMAGE:-false}" = "true" ]; then
    echo "Pushing image to registry..."
    docker push "${FULL_IMAGE_NAME}"
    echo "Successfully pushed image: ${FULL_IMAGE_NAME}"
fi
