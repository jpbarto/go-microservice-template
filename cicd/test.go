package main

import (
	"context"
	"encoding/json"
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
		// Report an error if no build artifact is provided
		return "", fmt.Errorf("no build artifact provided; use Build function to create OCI tarball")
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
	// Kubernetes config file content
	kubeconfig *dagger.File,
	// +optional
	// AWS configuration for accessing private registries
	awsconfig *dagger.Secret,
	// +optional
	// Deployment context from Deploy function
	deploymentContext *dagger.File,
	// +optional
	// Validation context from Validate function
	validationContext *dagger.File,
) (string, error) {
	var targetHost string
	var targetPort string

	// Extract endpoint from deployment context if provided
	if deploymentContext != nil {
		contextContent, err := deploymentContext.Contents(ctx)
		if err == nil {
			var depContext map[string]interface{}
			if err := json.Unmarshal([]byte(contextContent), &depContext); err == nil {
				if endpoint, ok := depContext["endpoint"].(string); ok {
					// Endpoint format: http://service.namespace.svc.cluster.local:8080
					// Extract just the URL as targetHost
					targetHost = endpoint
					// For scripts expecting separate host/port, parse if needed
					// For now, we'll pass the full endpoint and empty port
					targetPort = ""
				}
			}
		}
	}

	// Check validation status if validation context provided
	if validationContext != nil {
		contextContent, err := validationContext.Contents(ctx)
		if err == nil {
			var valContext map[string]interface{}
			if err := json.Unmarshal([]byte(contextContent), &valContext); err == nil {
				if status, ok := valContext["status"].(string); ok && status != "healthy" {
					return "", fmt.Errorf("skipping integration tests: validation status is %s", status)
				}
			}
		}
	}

	// Use defaults if not extracted from context
	if targetHost == "" {
		targetHost = "localhost"
	}
	if targetPort == "" {
		targetPort = "8080"
	}
	// Run the integration test script in a container with k6 and other dependencies
	// The integration_test.sh script will call performance_test.sh and acceptance_test.sh
	// Install k6 directly as a binary instead of from apt repository
	testContainer := dag.Container().
		From("debian:bookworm-slim").
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{"apt-get", "install", "-y", "bash", "curl", "jq", "ca-certificates", "wget", "tar", "xz-utils"}).
		WithExec([]string{"sh", "-c", "wget -qO- https://github.com/grafana/k6/releases/download/v0.49.0/k6-v0.49.0-linux-amd64.tar.gz | tar xz --strip-components=1 -C /usr/local/bin k6-v0.49.0-linux-amd64/k6"}).
		WithMountedDirectory("/workspace", source).
		WithWorkdir("/workspace")

	// Mount kubeconfig if provided for accessing deployed services
	if kubeconfig != nil {
		testContainer = testContainer.
			WithMountedFile("/root/.kube/config", kubeconfig)
	}

	// Set environment variables for test scripts
	testContainer = testContainer.
		WithEnvVariable("TEST_HOST", targetHost).
		WithEnvVariable("TEST_PORT", targetPort)

	// Execute integration test script
	// Pass arguments based on whether we have full endpoint or separate host/port
	var execArgs []string
	if targetPort != "" {
		execArgs = []string{"bash", "/workspace/tests/integration_test.sh", targetHost, targetPort}
	} else {
		// Pass full endpoint as single argument
		execArgs = []string{"bash", "/workspace/tests/integration_test.sh", targetHost}
	}

	testOutput, err := testContainer.
		WithExec(execArgs).
		Stdout(ctx)

	if err != nil {
		return "", err
	}

	return testOutput, nil
}
