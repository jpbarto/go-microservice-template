package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"dagger/goserv/internal/cicd"
	"dagger/goserv/internal/dagger"
)

// Deliver publishes the goserv container and Helm chart to repositories
func (m *Goserv) Deliver(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// +optional
	// Pre-built OCI image tarball (if not provided, will build from source)
	buildArtifact *dagger.File,
	// +optional
	// Build as release candidate (appends -rc to version tag)
	releaseCandidate bool,
) (*dagger.File, error) {
	// Read version from VERSION file
	versionContent, err := source.File("VERSION").Contents(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read VERSION file: %w", err)
	}
	tag := strings.TrimSpace(versionContent)

	// Append -rc suffix for release candidates
	if releaseCandidate {
		tag = tag + "-rc"
	}

	// Import the pre-built OCI tarball
	// The Import method preserves the multi-architecture manifest from the tarball
	if buildArtifact == nil {
		return nil, fmt.Errorf("buildArtifact is required; use Build function to create OCI tarball")
	}

	// Publish the container image tarball to the injected registry using cicd.ContainerPush.
	// This reads the registry URL from the injected CONTAINER_REPOSITORY_URL constant.
	// "latest" is added as an additional tag alongside the versioned tag.
	address, err := cicd.ContainerPush(ctx, dag, buildArtifact, "goserv", tag, []string{"latest"})
	if err != nil {
		return nil, fmt.Errorf("failed to publish container: %w", err)
	}

	// Split the published image reference (e.g. "registry.example.com/goserv:1.2.3")
	// into the repository and tag parts for use in the Helm chart values.
	imageRepository, imageTag, _ := strings.Cut(address, ":")

	// Package the Helm chart and extract the resulting .tgz so it can be passed
	// to cicd.HelmPush, which handles the push to the injected Helm repository.
	chartTgz := dag.Container().
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
		WithExec([]string{"yq", "eval", ".image.repository = \"" + imageRepository + "\"", "-i", "./helm/goserv/values.yaml"}).
		WithExec([]string{"yq", "eval", ".image.tag = \"" + imageTag + "\"", "-i", "./helm/goserv/values.yaml"}).
		WithExec([]string{"helm", "package", "./helm/goserv", "--version", tag, "--app-version", tag}).
		File("goserv-" + tag + ".tgz")

	// Push the packaged chart using cicd.HelmPush.
	// This reads the repository URL from the injected HELM_REPOSITORY_URL constant
	// and returns the fully-qualified chart reference.
	chartRef, err := cicd.HelmPush(ctx, dag, chartTgz)
	if err != nil {
		return nil, fmt.Errorf("failed to publish Helm chart: %w", err)
	}

	// Create delivery context JSON
	deliveryContext := map[string]interface{}{
		"timestamp":        time.Now().Format(time.RFC3339),
		"imageReference":   address,
		"chartReference":   chartRef,
		"version":          tag,
		"releaseCandidate": releaseCandidate,
		"architectures":    []string{"linux/amd64", "linux/arm64"},
	}

	contextJSON, err := json.MarshalIndent(deliveryContext, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal delivery context: %w", err)
	}

	// Return delivery context as a file
	return dag.Directory().
		WithNewFile("deliveryContext", string(contextJSON)).
		File("deliveryContext"), nil
}
