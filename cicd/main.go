// A generated module for Goserv functions
//
// This module has been generated via dagger init and serves as a reference to
// basic module structure as you get started with Dagger.
//
// Two functions have been pre-created. You can modify, delete, or add to them,
// as needed. They demonstrate usage of arguments and return types using simple
// echo and grep commands. The functions can be called from the dagger CLI or
// from one of the SDKs.
//
// The first line in this comment block is a short description line and the
// rest is a long description with more detail on the module's purpose or usage,
// if appropriate. All modules should have a short description.

package main

import (
	"context"
	"dagger/goserv/internal/dagger"
)

type Goserv struct{}

// Returns a container that echoes whatever string argument is provided
func (m *Goserv) ContainerEcho(stringArg string) *dagger.Container {
	return dag.Container().From("alpine:latest").WithExec([]string{"echo", stringArg})
}

// Returns lines that match a pattern in the files of the provided Directory
func (m *Goserv) GrepDir(ctx context.Context, directoryArg *dagger.Directory, pattern string) (string, error) {
	return dag.Container().
		From("alpine:latest").
		WithMountedDirectory("/mnt", directoryArg).
		WithWorkdir("/mnt").
		WithExec([]string{"grep", "-R", pattern, "."}).
		Stdout(ctx)
}

// Build builds the Docker image using the Dockerfile in the project directory
func (m *Goserv) Build(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// +optional
	// Image tag (default: latest)
	tag string,
) (*dagger.Container, error) {
	if tag == "" {
		tag = "latest"
	}

	// Build the container using the Dockerfile in the source directory
	container := source.DockerBuild()

	return container, nil
}

// UnitTest runs the goserv container and executes unit tests against it
func (m *Goserv) UnitTest(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
) (string, error) {
	// Build the application container
	appContainer, err := m.Build(ctx, source, "latest")
	if err != nil {
		return "", err
	}

	// Start the application container as a service on port 8080
	appService := appContainer.
		WithExposedPort(8080).
		AsService()

	// Run the unit test script in a container with the app service bound
	testOutput, err := dag.Container().
		From("debian:bookworm-slim").
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{"apt-get", "install", "-y", "bash", "curl", "jq"}).
		WithMountedDirectory("/tests", source.Directory("tests")).
		WithServiceBinding("goserv", appService).
		WithEnvVariable("TEST_HOST", "goserv").
		WithEnvVariable("TEST_PORT", "8080").
		WithExec([]string{"bash", "/tests/unit_test.sh"}).
		Stdout(ctx)

	if err != nil {
		return "", err
	}

	return testOutput, nil
}
