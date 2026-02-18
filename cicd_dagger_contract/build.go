package main

import (
	"context"

	"dagger/goserv/internal/dagger"
)

// Build builds a multi-architecture Docker image and exports it as an OCI tarball
func (m *Goserv) Build(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// +optional
	// Whether to build as release candidate (appends -rc to version)
	releaseCandidate bool,
) (*dagger.File, error) {
	// Print message and return
	output, err := dag.Container().
		From("alpine:latest").
		WithExec([]string{"echo", "This is the Build function"}).
		Stdout(ctx)

	if err != nil {
		return nil, err
	}

	// Print to show the message
	println(output)

	// Return a dummy file since the function signature requires *dagger.File
	return dag.Container().From("alpine:latest").File("/etc/hostname"), nil
}
