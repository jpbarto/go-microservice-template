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
	"fmt"
	"strings"

	"dagger/goserv/internal/dagger"
)

type Goserv struct{}

// Returns a container that echoes whatever string argument is provided
func (m *Goserv) ContainerEcho(stringArg string) *dagger.Container {
	return dag.Container().From("alpine:latest").WithExec([]string{"echo", stringArg})
}

// Build builds the Docker image using the Dockerfile in the project directory
func (m *Goserv) Build(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// +optional
	// Whether to build as release candidate (appends -rc to version)
	releaseCandidate bool,
) (*dagger.Container, error) {
	// Read version from VERSION file
	versionContent, err := source.File("VERSION").Contents(ctx)
	if err != nil {
		return nil, err
	}

	tag := strings.TrimSpace(versionContent)

	// Append -rc if this is a release candidate
	if releaseCandidate {
		tag = tag + "-rc"
	}

	// Build the container with VERSION as build arg
	container := source.DockerBuild(dagger.DirectoryDockerBuildOpts{
		BuildArgs: []dagger.BuildArg{
			{Name: "VERSION", Value: tag},
		},
	})

	return container, nil
}

// UnitTest runs the goserv container and executes unit tests against it
func (m *Goserv) UnitTest(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
) (string, error) {
	// Build the application container
	appContainer, err := m.Build(ctx, source, false)
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

// Deliver publishes the goserv container to ttl.sh registry
func (m *Goserv) Deliver(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// +optional
	// Build as release candidate (appends -rc to version tag)
	releaseCandidate bool,
) (string, error) {
	// Read version from VERSION file
	versionContent, err := source.File("VERSION").Contents(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to read VERSION file: %w", err)
	}
	tag := strings.TrimSpace(versionContent)

	// Append -rc suffix for release candidates
	if releaseCandidate {
		tag = tag + "-rc"
	}

	// Build the application container with the version tag
	container, err := m.Build(ctx, source, releaseCandidate)
	if err != nil {
		return "", err
	}

	// Publish to ttl.sh (anonymous registry with automatic expiration)
	// Format: ttl.sh/goserv-version:1h (combines version tag with expiration)
	imageRef := "ttl.sh/goserv-" + tag + ":1h"

	address, err := container.Publish(ctx, imageRef)
	if err != nil {
		return "", err
	}

	return address, nil
}

// Deploy installs the Helm chart to a Kubernetes cluster
func (m *Goserv) Deploy(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// Kubernetes config file content
	kubeconfig *dagger.Secret,
	// +optional
	// Release name (default: goserv)
	releaseName string,
	// +optional
	// Kubernetes namespace (default: default)
	namespace string,
	// +optional
	// Image repository (default: ttl.sh/goserv)
	imageRepository string,
	// +optional
	// Image tag (default: reads from VERSION file)
	imageTag string,
) (string, error) {
	if releaseName == "" {
		releaseName = "goserv"
	}
	if namespace == "" {
		namespace = "goserv"
	}
	if imageRepository == "" {
		imageRepository = "ttl.sh/goserv-latest"
		versionContent, err := source.File("VERSION").Contents(ctx)
		if err == nil {
			imageRepository = "ttl.sh/goserv-" + strings.TrimSpace(versionContent)
		}
	}
	if imageTag == "" {
		imageTag = "1h"
	}

	// Create a container with kubectl and helm installed
	output, err := dag.Container().
		From("debian:bookworm-slim").
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{"apt-get", "install", "-y", "curl", "gnupg", "apt-transport-https"}).
		WithExec([]string{"sh", "-c", "curl -fsSL https://packages.buildkite.com/helm-linux/helm-debian/gpgkey | gpg --dearmor | tee /usr/share/keyrings/helm.gpg > /dev/null"}).
		WithExec([]string{"sh", "-c", "echo \"deb [signed-by=/usr/share/keyrings/helm.gpg] https://packages.buildkite.com/helm-linux/helm-debian/any/ any main\" | tee /etc/apt/sources.list.d/helm-stable-debian.list"}).
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{"apt-get", "install", "-y", "helm"}).
		WithSecretVariable("KUBECONFIG_CONTENT", kubeconfig).
		WithExec([]string{"sh", "-c", "mkdir -p /root/.kube && echo \"$KUBECONFIG_CONTENT\" > /root/.kube/config"}).
		WithMountedDirectory("/workspace", source).
		WithWorkdir("/workspace").
		WithExec([]string{
			"helm", "upgrade", "--install", releaseName,
			"./helm/goserv",
			"--namespace", namespace,
			"--create-namespace",
			"--set", "image.repository=" + imageRepository,
			"--set", "image.tag=" + imageTag,
			"--wait",
		}).
		Stdout(ctx)

	if err != nil {
		return "", err
	}

	return output, nil
}
