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
	var serviceName, namespace, servicePort string

	// Extract service information from validation context if provided
	if validationContext != nil {
		contextContent, err := validationContext.Contents(ctx)
		if err == nil {
			var valContext map[string]interface{}
			if err := json.Unmarshal([]byte(contextContent), &valContext); err == nil {
				// Check validation status first
				if status, ok := valContext["status"].(string); ok && status != "healthy" {
					return "", fmt.Errorf("skipping integration tests: validation status is %s", status)
				}

				// Extract service details
				if sn, ok := valContext["serviceName"].(string); ok {
					serviceName = sn
				}
				if ns, ok := valContext["namespace"].(string); ok {
					namespace = ns
				}
				if sp, ok := valContext["servicePort"].(string); ok {
					servicePort = sp
				}
			}
		}
	}

	// Use defaults if not extracted from context
	if serviceName == "" {
		serviceName = "goserv"
	}
	if namespace == "" {
		namespace = "goserv"
	}
	if servicePort == "" {
		servicePort = "8080"
	}

	// Run the integration test script in a container with k6, kubectl, and other dependencies
	// The integration_test.sh script will call performance_test.sh and acceptance_test.sh
	testContainer := dag.Container().
		From("debian:bookworm-slim").
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{"apt-get", "install", "-y", "bash", "curl", "jq", "ca-certificates", "wget", "tar", "xz-utils"}).
		WithExec([]string{"sh", "-c", "wget -qO- https://github.com/grafana/k6/releases/download/v0.49.0/k6-v0.49.0-linux-amd64.tar.gz | tar xz --strip-components=1 -C /usr/local/bin k6-v0.49.0-linux-amd64/k6"}).
		WithExec([]string{"sh", "-c", "curl -LO https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"}).
		WithExec([]string{"install", "-o", "root", "-g", "root", "-m", "0755", "kubectl", "/usr/local/bin/kubectl"}).
		WithMountedDirectory("/workspace", source).
		WithWorkdir("/workspace")

	// Mount kubeconfig if provided for accessing deployed services
	if kubeconfig != nil {
		testContainer = testContainer.
			WithMountedFile("/root/.kube/config", kubeconfig)
	}

	// Set up port-forward as a background process and run tests
	// Use a wrapper script to start port-forward in background and then run tests
	portForwardCmd := fmt.Sprintf("kubectl port-forward -n %s svc/%s 30303:%s", namespace, serviceName, servicePort)
	testUrl := "http://localhost:30303"

	// Create a test execution script that sets up port-forward and runs tests
	testScript := fmt.Sprintf(`#!/bin/bash
set -e

# Start port-forward in background
%s &
PF_PID=$!

# Wait for port-forward to be ready
echo "Waiting for port-forward to be ready..."
for i in {1..30}; do
  if curl -s %s/health > /dev/null 2>&1; then
    echo "Port-forward is ready"
    break
  fi
  sleep 1
done

# Run integration tests
bash /workspace/tests/integration_test.sh %s

# Cleanup
kill $PF_PID 2>/dev/null || true
`, portForwardCmd, testUrl, testUrl)

	testContainer = testContainer.
		WithNewFile("/tmp/run_tests.sh", testScript).
		WithExec([]string{"chmod", "+x", "/tmp/run_tests.sh"})

	// Execute the test script
	testOutput, err := testContainer.
		WithExec([]string{"bash", "/tmp/run_tests.sh"}).
		Stdout(ctx)

	if err != nil {
		return "", err
	}

	return testOutput, nil
}
