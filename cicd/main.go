// A Dagger module for goserv CI/CD pipeline
//
// This module provides CI/CD functions for building, testing, validating,
// deploying, and delivering the goserv microservice.

package main

import (
	"context"
	"fmt"

	"dagger/goserv/internal/dagger"
)

type Goserv struct{}

// Build builds the Docker image using build.sh script with Docker-in-Docker
func (m *Goserv) Build(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// +optional
	// Container registry to push to (e.g., docker.io/username)
	registry string,
	// +optional
	// Image tag (default: latest)
	tag string,
) (string, error) {
	if tag == "" {
		tag = "latest"
	}

	// Start a Docker engine service
	dockerEngine := dag.Container().
		From("docker:dind").
		WithMountedCache("/var/lib/docker", dag.CacheVolume("docker-lib")).
		WithExposedPort(2375).
		WithExec([]string{
			"dockerd",
			"--host=tcp://0.0.0.0:2375",
			"--host=unix:///var/run/docker.sock",
			"--tls=false",
		}, dagger.ContainerWithExecOpts{
			InsecureRootCapabilities: true,
		}).
		AsService()

	// Build environment variables for the script
	buildEnv := fmt.Sprintf("TAG=%s", tag)
	if registry != "" {
		buildEnv = fmt.Sprintf("REGISTRY=%s TAG=%s", registry, tag)
	}

	// Use the Docker engine service in our build container
	output, err := getBaseContainer(source).
		WithServiceBinding("docker", dockerEngine).
		WithEnvVariable("DOCKER_HOST", "tcp://docker:2375").
		WithExec([]string{"sh", "-c", buildEnv + " ./cicd/build.sh"}).
		Stdout(ctx)

	if err != nil {
		return "", err
	}

	return output, nil
}

// BuildAndPublish is an alias for Build that includes publishing logic via the build.sh script
func (m *Goserv) BuildAndPublish(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// +optional
	// Container registry to push the image to (e.g., "docker.io/myorg")
	registry string,
	// +optional
	// Image tag (default: latest)
	tag string,
	// +optional
	// Whether to push the image to the registry
	push bool,
) (string, error) {
	// The build.sh script handles pushing if REGISTRY is set
	// Set PUSH=true environment variable to enable pushing
	if push && registry != "" {
		return m.Build(ctx, source, registry, tag)
	}
	return m.Build(ctx, source, "", tag)
}

// UnitTest executes the unit_test.sh script to run unit tests
func (m *Goserv) UnitTest(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
) (string, error) {
	output, err := getBaseContainer(source).
		WithExec([]string{"sh", "-c", "./cicd/unit_test.sh"}).
		Stdout(ctx)

	if err != nil {
		return "", fmt.Errorf("unit tests failed: %w", err)
	}

	return output, nil
}

// IntegrationTest executes the integration_test.sh script to run integration tests
func (m *Goserv) IntegrationTest(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// +optional
	// Kubernetes context to use for testing
	kubeContext string,
) (string, error) {
	container := getBaseContainer(source)

	if kubeContext != "" {
		container = container.WithEnvVariable("KUBE_CONTEXT", kubeContext)
	}

	output, err := container.
		WithExec([]string{"sh", "-c", "./cicd/integration_test.sh"}).
		Stdout(ctx)

	if err != nil {
		return "", fmt.Errorf("integration tests failed: %w", err)
	}

	return output, nil
}

// Validate executes the validate.sh script to validate the build and configuration
func (m *Goserv) Validate(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
) (string, error) {
	output, err := getBaseContainer(source).
		WithExec([]string{"sh", "-c", "./cicd/validate.sh"}).
		Stdout(ctx)

	if err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}

	return output, nil
}

// Deploy executes the deploy.sh script to deploy to Kubernetes
func (m *Goserv) Deploy(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// Environment to deploy to (dev, staging, prod)
	environment string,
	// +optional
	// Kubernetes namespace
	namespace string,
	// +optional
	// Image tag to deploy
	imageTag string,
) (string, error) {
	if imageTag == "" {
		imageTag = "latest"
	}
	if namespace == "" {
		namespace = "default"
	}

	container := getBaseContainer(source).
		WithEnvVariable("ENVIRONMENT", environment).
		WithEnvVariable("NAMESPACE", namespace).
		WithEnvVariable("IMAGE_TAG", imageTag)

	output, err := container.
		WithExec([]string{"sh", "-c", "./cicd/deploy.sh"}).
		Stdout(ctx)

	if err != nil {
		return "", fmt.Errorf("deployment failed: %w", err)
	}

	return output, nil
}

// Deliver executes the deliver.sh script to deliver the application
func (m *Goserv) Deliver(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// +optional
	// Version to deliver
	version string,
	// +optional
	// Release notes
	releaseNotes string,
) (string, error) {
	container := getBaseContainer(source)

	if version != "" {
		container = container.WithEnvVariable("VERSION", version)
	}
	if releaseNotes != "" {
		container = container.WithEnvVariable("RELEASE_NOTES", releaseNotes)
	}

	output, err := container.
		WithExec([]string{"sh", "-c", "./cicd/deliver.sh"}).
		Stdout(ctx)

	if err != nil {
		return "", fmt.Errorf("delivery failed: %w", err)
	}

	return output, nil
}

// Pipeline executes the full CI/CD pipeline: build, validate, unit test, integration test, deploy
func (m *Goserv) Pipeline(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// Environment to deploy to (dev, staging, prod)
	environment string,
	// +optional
	// Image tag (default: latest)
	tag string,
	// +optional
	// Skip deployment step
	skipDeploy bool,
) (string, error) {
	var output string

	// Build
	buildOutput, err := m.Build(ctx, source, "", tag)
	if err != nil {
		return "", err
	}
	output += "=== BUILD ===\n" + buildOutput + "\n\n"

	// Validate
	validateOutput, err := m.Validate(ctx, source)
	if err != nil {
		return "", err
	}
	output += "=== VALIDATE ===\n" + validateOutput + "\n\n"

	// Unit Tests
	unitTestOutput, err := m.UnitTest(ctx, source)
	if err != nil {
		return "", err
	}
	output += "=== UNIT TESTS ===\n" + unitTestOutput + "\n\n"

	// Integration Tests
	integrationTestOutput, err := m.IntegrationTest(ctx, source, "")
	if err != nil {
		return "", err
	}
	output += "=== INTEGRATION TESTS ===\n" + integrationTestOutput + "\n\n"

	// Deploy
	if !skipDeploy {
		deployOutput, err := m.Deploy(ctx, source, environment, "", tag)
		if err != nil {
			return "", err
		}
		output += "=== DEPLOY ===\n" + deployOutput + "\n\n"
	}

	output += "Pipeline completed successfully!\n"
	return output, nil
}

// getBaseContainer returns a base container with necessary tools installed
func getBaseContainer(source *dagger.Directory) *dagger.Container {
	return dag.Container().
		From("golang:1.21-bookworm").
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{"apt-get", "install", "-y", "docker.io", "curl", "git", "bash"}).
		WithMountedDirectory("/src", source).
		WithWorkdir("/src").
		WithExec([]string{"chmod", "+x", "./cicd/build.sh"}).
		WithExec([]string{"chmod", "+x", "./cicd/deploy.sh"}).
		WithExec([]string{"chmod", "+x", "./cicd/unit_test.sh"}).
		WithExec([]string{"chmod", "+x", "./cicd/integration_test.sh"}).
		WithExec([]string{"chmod", "+x", "./cicd/validate.sh"}).
		WithExec([]string{"chmod", "+x", "./cicd/deliver.sh"})
}
