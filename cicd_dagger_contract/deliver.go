package main

import (
	"context"

	"dagger/goserv/internal/dagger"
)

// Deliver publishes the goserv container and Helm chart to repositories
func (m *Goserv) Deliver(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// +optional
	// Container repository (default: ttl.sh)
	containerRepository string,
	// +optional
	// Helm chart repository URL (default: oci://ttl.sh)
	helmRepository string,
	// +optional
	// Build output from the Build function (if not provided, will build from source)
	buildArtifact *dagger.File,
	// +optional
	// Build as release candidate (appends -rc to version tag)
	releaseCandidate bool,
) (string, error) {
	// Print message
	output, err := dag.Container().
		From("alpine:latest").
		WithExec([]string{"echo", "this is the Deliver function"}).
		Stdout(ctx)

	if err != nil {
		return "", err
	}

	return output, nil
}
