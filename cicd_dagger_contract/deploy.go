package main

import (
	"context"

	"dagger/goserv/internal/dagger"
)

// Deploy installs the Helm chart from a Helm repository to a Kubernetes cluster
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
	// Print message
	output, err := dag.Container().
		From("alpine:latest").
		WithExec([]string{"echo", "this is the Deploy function"}).
		Stdout(ctx)

	if err != nil {
		return "", err
	}

	return output, nil
}
