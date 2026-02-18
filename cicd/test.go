package main

import (
	"context"

	"dagger/goserv/internal/dagger"
)

// UnitTest runs the goserv container and executes unit tests against it
func (m *Goserv) UnitTest(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// +optional
	// Pre-built OCI image tarball (if not provided, will build from source)
	buildArtifact *dagger.File,
) (string, error) {
	var appContainer *dagger.Container

	if buildArtifact != nil {
		// Import the pre-built OCI image
		// This will automatically select the appropriate platform variant for the host
		appContainer = dag.Container().Import(buildArtifact)
	} else {
		// Build from source if no tarball provided
		tarball, err := m.Build(ctx, source, false)
		if err != nil {
			return "", err
		}
		appContainer = dag.Container().Import(tarball)
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
		WithMountedDirectory("/workspace", source).
		WithServiceBinding("goserv", appService).
		WithEnvVariable("TEST_HOST", "goserv").
		WithEnvVariable("TEST_PORT", "8080").
		WithExec([]string{"bash", "/workspace/tests/unit_test.sh"}).
		Stdout(ctx)

	if err != nil {
		return "", err
	}

	return testOutput, nil
}

// IntegrationTest runs integration tests against a deployed goserv instance
func (m *Goserv) IntegrationTest(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// +optional
	// Target host where goserv is deployed (default: localhost)
	targetHost string,
	// +optional
	// Target port (default: 8080)
	targetPort string,
) (string, error) {
	if targetHost == "" {
		targetHost = "localhost"
	}
	if targetPort == "" {
		targetPort = "8080"
	}

	// Run the integration test script in a container with k6 and other dependencies
	// The integration_test.sh script will call performance_test.sh and acceptance_test.sh
	// Install k6 directly as a binary instead of from apt repository
	testOutput, err := dag.Container().
		From("debian:bookworm-slim").
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{"apt-get", "install", "-y", "bash", "curl", "jq", "ca-certificates", "wget", "tar", "xz-utils"}).
		WithExec([]string{"sh", "-c", "wget -qO- https://github.com/grafana/k6/releases/download/v0.49.0/k6-v0.49.0-linux-amd64.tar.gz | tar xz --strip-components=1 -C /usr/local/bin k6-v0.49.0-linux-amd64/k6"}).
		WithMountedDirectory("/workspace", source).
		WithWorkdir("/workspace").
		WithEnvVariable("TEST_HOST", targetHost).
		WithEnvVariable("TEST_PORT", targetPort).
		WithExec([]string{"bash", "/workspace/tests/integration_test.sh", targetHost, targetPort}).
		Stdout(ctx)

	if err != nil {
		return "", err
	}

	return testOutput, nil
}
