package main

import (
	"context"
	"fmt"
	"time"

	"dagger/goserv/internal/dagger"
)

// Deploy deploys the application using ArgoCD by applying the ArgoCD Application manifest
// +cache = "never"
func (m *Goserv) Deploy(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// Kubernetes config file content
	kubeconfig *dagger.Secret,
	// +optional
	// Helm chart repository URL (default: oci://ttl.sh)
	helmRepository string,
	// +optional
	// Release name (default: goserv)
	releaseName string,
	// +optional
	// Kubernetes namespace (default: goserv)
	namespace string,
	// +optional
	// Build as release candidate (appends -rc to version tag)
	releaseCandidate bool,
) (string, error) {
	// Apply defaults (parameters not used in ArgoCD deployment but kept for compatibility)
	if releaseName == "" {
		releaseName = "goserv"
	}
	if helmRepository == "" {
		helmRepository = "oci://ttl.sh"
	}
	if namespace == "" {
		namespace = "goserv"
	}
	// Create a container with kubectl installed
	container := dag.Container().
		From("debian:bookworm-slim").
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{"apt-get", "install", "-y", "curl"}).
		WithExec([]string{"sh", "-c", "curl -LO https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"}).
		WithExec([]string{"install", "-o", "root", "-g", "root", "-m", "0755", "kubectl", "/usr/local/bin/kubectl"}).
		WithSecretVariable("KUBECONFIG_CONTENT", kubeconfig).
		WithExec([]string{"sh", "-c", "mkdir -p /root/.kube && echo \"$KUBECONFIG_CONTENT\" > /root/.kube/config"}).
		WithMountedDirectory("/workspace", source).
		WithWorkdir("/workspace")

	// Apply the ArgoCD Application manifest
	output, err := container.
		WithEnvVariable("CACHE_BUSTER", time.Now().String()).
		WithExec([]string{
			"kubectl", "apply",
			"-f", "deploy/goserv-application.yaml",
			"-n", "argocd",
		}).
		Stdout(ctx)

	if err != nil {
		return "", fmt.Errorf("failed to apply ArgoCD application: %w", err)
	}

	return output, nil
}
