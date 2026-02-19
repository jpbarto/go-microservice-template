package main

import (
	"context"
	"fmt"

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
		// Report an error if no build artifact is provided, since the Deliver function requires building from source
		return "", fmt.Errorf("no build artifact provided; building from source is required for testing")
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
	// Target URL where goserv is deployed (default: http://localhost:8080)
	targetUrl string,
) (string, error) {
	if targetUrl == "" {
		targetUrl = "http://localhost:8080"
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
		WithEnvVariable("TEST_URL", targetUrl).
		WithExec([]string{"bash", "/workspace/tests/integration_test.sh", targetUrl}).
		Stdout(ctx)

	if err != nil {
		return "", err
	}

	return testOutput, nil
}
