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

// Build builds a multi-architecture Docker image and exports it as an OCI tarball
func (m *Goserv) Build(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// +optional
	// Whether to build as release candidate (appends -rc to version)
	releaseCandidate bool,
) (*dagger.File, error) {
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

	// Define target platforms
	platforms := []dagger.Platform{
		"linux/amd64",
		"linux/arm64",
	}

	// Build multi-platform variants
	platformVariants := make([]*dagger.Container, 0, len(platforms))
	for _, platform := range platforms {
		variant := source.DockerBuild(dagger.DirectoryDockerBuildOpts{
			Platform: platform,
			BuildArgs: []dagger.BuildArg{
				{Name: "VERSION", Value: tag},
			},
		})
		platformVariants = append(platformVariants, variant)
	}

	// Export as multi-platform OCI tarball
	// Use the first variant as base and pass only the remaining variants
	// to avoid duplicate platform error
	tarball := platformVariants[0].AsTarball(dagger.ContainerAsTarballOpts{
		PlatformVariants: platformVariants[1:],
	})

	return tarball, nil
}

// UnitTest runs the goserv container and executes unit tests against it
func (m *Goserv) UnitTest(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// +optional
	// Pre-built OCI image tarball (if not provided, will build from source)
	imageTarball *dagger.File,
) (string, error) {
	var appContainer *dagger.Container

	if imageTarball != nil {
		// Import the pre-built OCI image
		// This will automatically select the appropriate platform variant for the host
		appContainer = dag.Container().Import(imageTarball)
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
	// Target host where goserv is deployed
	targetHost string,
	// +optional
	// Target port (default: 8080)
	targetPort string,
) (string, error) {
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

// Deliver publishes the goserv container and Helm chart to repositories
func (m *Goserv) Deliver(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// Container repository (example: ttl.sh)
	containerRepository string,
	// Helm chart repository URL (example: oci://registry-1.docker.io/myuser)
	helmRepository string,
	// +optional
	// Pre-built OCI image tarball (if not provided, will build from source)
	imageTarball *dagger.File,
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

	// Get or build the container images
	// If tarball is provided, we need to rebuild from source to get platform variants
	// since Import() doesn't preserve multi-arch manifests
	var platformVariants []*dagger.Container

	if imageTarball != nil {
		// Even if tarball exists, we need to rebuild to get proper platform variants for publishing
		// The tarball is useful for testing, but publishing requires the container objects
		platforms := []dagger.Platform{
			"linux/amd64",
			"linux/arm64",
		}

		platformVariants = make([]*dagger.Container, 0, len(platforms))
		for _, platform := range platforms {
			variant := source.DockerBuild(dagger.DirectoryDockerBuildOpts{
				Platform: platform,
				BuildArgs: []dagger.BuildArg{
					{Name: "VERSION", Value: tag},
				},
			})
			platformVariants = append(platformVariants, variant)
		}
	} else {
		// Build from source
		platforms := []dagger.Platform{
			"linux/amd64",
			"linux/arm64",
		}

		platformVariants = make([]*dagger.Container, 0, len(platforms))
		for _, platform := range platforms {
			variant := source.DockerBuild(dagger.DirectoryDockerBuildOpts{
				Platform: platform,
				BuildArgs: []dagger.BuildArg{
					{Name: "VERSION", Value: tag},
				},
			})
			platformVariants = append(platformVariants, variant)
		}
	}

	// Publish multi-architecture container to registry
	imageRef := containerRepository + "/goserv:" + tag

	address, err := platformVariants[0].Publish(ctx, imageRef, dagger.ContainerPublishOpts{
		PlatformVariants: platformVariants[1:],
	})
	if err != nil {
		return "", fmt.Errorf("failed to publish container: %w", err)
	}

	// Package and publish Helm chart
	helmOutput, err := dag.Container().
		From("debian:bookworm-slim").
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{"apt-get", "install", "-y", "curl", "gnupg", "apt-transport-https", "wget"}).
		WithExec([]string{"sh", "-c", "curl -fsSL https://packages.buildkite.com/helm-linux/helm-debian/gpgkey | gpg --dearmor | tee /usr/share/keyrings/helm.gpg > /dev/null"}).
		WithExec([]string{"sh", "-c", "echo \"deb [signed-by=/usr/share/keyrings/helm.gpg] https://packages.buildkite.com/helm-linux/helm-debian/any/ any main\" | tee /etc/apt/sources.list.d/helm-stable-debian.list"}).
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{"apt-get", "install", "-y", "helm"}).
		WithExec([]string{"sh", "-c", "wget -qO /usr/local/bin/yq https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64 && chmod +x /usr/local/bin/yq"}).
		WithMountedDirectory("/workspace", source).
		WithWorkdir("/workspace").
		WithExec([]string{"yq", "eval", ".image.repository = \"" + containerRepository + "/goserv\"", "-i", "./helm/goserv/values.yaml"}).
		WithExec([]string{"yq", "eval", ".image.tag = \"" + tag + "\"", "-i", "./helm/goserv/values.yaml"}).
		WithExec([]string{"helm", "package", "./helm/goserv", "--version", tag, "--app-version", tag}).
		WithExec([]string{"helm", "push", "goserv-" + tag + ".tgz", helmRepository + "/charts"}).
		Stdout(ctx)

	if err != nil {
		return "", fmt.Errorf("failed to publish Helm chart: %w", err)
	}

	chartRef := helmRepository + "/charts/goserv:" + tag
	return fmt.Sprintf("Container: %s (multi-arch: linux/amd64, linux/arm64)\nHelm chart: %s\nHelm output: %s", address, chartRef, helmOutput), nil
}

// Deploy installs the Helm chart from a Helm repository to a Kubernetes cluster
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
	if releaseName == "" {
		releaseName = "goserv"
	}

	if helmRepository == "" {
		helmRepository = "oci://ttl.sh"
	}

	if namespace == "" {
		namespace = "goserv"
	}

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

	// Construct the chart reference
	chartRef := helmRepository + "/charts/goserv:" + tag

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
		WithWorkdir("/workspace").
		WithExec([]string{
			"helm", "upgrade", "--install", releaseName,
			chartRef,
			"--namespace", namespace,
			"--create-namespace",
			"--wait",
		}).
		Stdout(ctx)

	if err != nil {
		return "", err
	}

	return output, nil
}

// Validate runs the validation script to verify that the deployment is healthy and functioning correctly
func (m *Goserv) Validate(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// Kubernetes config file content
	kubeconfig *dagger.Secret,
	// +optional
	// Release name (default: goserv)
	releaseName string,
	// +optional
	// Kubernetes namespace (default: goserv)
	namespace string,
) (string, error) {
	if releaseName == "" {
		releaseName = "goserv"
	}

	if namespace == "" {
		namespace = "goserv"
	}

	// Run the validation script in a container with kubectl, helm, and other dependencies
	// Install kubectl and helm as binaries instead of from apt repositories
	validationOutput, err := dag.Container().
		From("debian:bookworm-slim").
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{"apt-get", "install", "-y", "bash", "curl", "jq", "ca-certificates", "wget"}).
		WithExec([]string{"sh", "-c", "curl -LO https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"}).
		WithExec([]string{"install", "-o", "root", "-g", "root", "-m", "0755", "kubectl", "/usr/local/bin/kubectl"}).
		WithExec([]string{"sh", "-c", "curl -fsSL https://get.helm.sh/helm-v3.13.3-linux-amd64.tar.gz | tar xz --strip-components=1 -C /usr/local/bin linux-amd64/helm"}).
		WithSecretVariable("KUBECONFIG_CONTENT", kubeconfig).
		WithExec([]string{"sh", "-c", "mkdir -p /root/.kube && echo \"$KUBECONFIG_CONTENT\" > /root/.kube/config"}).
		WithMountedDirectory("/workspace", source).
		WithWorkdir("/workspace").
		WithEnvVariable("RELEASE_NAME", releaseName).
		WithEnvVariable("NAMESPACE", namespace).
		WithExec([]string{"bash", "/workspace/tests/validate.sh"}).
		Stdout(ctx)

	if err != nil {
		return "", err
	}

	return validationOutput, nil
}
