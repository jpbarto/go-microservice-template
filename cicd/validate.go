package main

import (
	"context"
	"fmt"
	"strings"

	"dagger/goserv/internal/dagger"
)

// Validate runs the validation script to verify that the deployment is healthy and functioning correctly
func (m *Goserv) Validate(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// Kubernetes config file content
	kubeconfig *dagger.Secret,
	// +optional
	// Release name (default: goserv)
	releaseName string,
	// +optional
	// Kubernetes namespace (default: goserv)
	namespace string,
	// +optional
	// Expected version to validate (if not provided, reads from VERSION file)
	expectedVersion string,
	// +optional
	// Build as release candidate (appends -rc to version)
	releaseCandidate bool,
) (string, error) {
	if releaseName == "" {
		releaseName = "goserv"
	}

	if namespace == "" {
		namespace = "goserv"
	}

	// If expectedVersion not provided, read from VERSION file
	if expectedVersion == "" {
		versionContent, err := source.File("VERSION").Contents(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to read VERSION file: %w", err)
		}
		expectedVersion = strings.TrimSpace(versionContent)
	}

	// Append -rc suffix for release candidates
	if releaseCandidate {
		expectedVersion = expectedVersion + "-rc"
	}

	// Run the validation script in a container with kubectl, helm, and other dependencies
	// Install kubectl and helm as binaries instead of from apt repositories
	validationOutput, err := dag.Container().
		From("debian:bookworm-slim").
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{"apt-get", "install", "-y", "bash", "curl", "jq", "ca-certificates", "wget"}).
		WithExec([]string{"sh", "-c", "curl -LO https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"}).
		WithExec([]string{"install", "-o", "root", "-g", "root", "-m", "0755", "kubectl", "/usr/local/bin/kubectl"}).
		WithExec([]string{"sh", "-c", "curl -fsSL https://get.helm.sh/helm-v3.13.3-linux-amd64.tar.gz | tar xz --strip-components=1 -C /usr/local/bin linux-amd64/helm"}).
		WithSecretVariable("KUBECONFIG_CONTENT", kubeconfig).
		WithExec([]string{"sh", "-c", "mkdir -p /root/.kube && echo \"$KUBECONFIG_CONTENT\" > /root/.kube/config"}).
		WithMountedDirectory("/workspace", source).
		WithWorkdir("/workspace").
		WithEnvVariable("RELEASE_NAME", releaseName).
		WithEnvVariable("NAMESPACE", namespace).
		WithEnvVariable("EXPECTED_VERSION", expectedVersion).
		WithExec([]string{"bash", "/workspace/tests/validate.sh"}).
		Stdout(ctx)

	if err != nil {
		return "", err
	}

	return validationOutput, nil
}
