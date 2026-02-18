package main

import (
	"context"
	"fmt"
	"strings"

	"dagger/goserv/internal/dagger"
)

// Deliver publishes the goserv container and Helm chart to repositories
func (m *Goserv) Deliver(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// +optional
	// Container repository (default: ttl.sh)
	containerRepository string,
	// +optional
	// Helm chart repository URL (default: oci://ttl.sh)
	helmRepository string,
	// +optional
	// Pre-built OCI image tarball (if not provided, will build from source)
	imageTarball *dagger.File,
	// +optional
	// Build as release candidate (appends -rc to version tag)
	releaseCandidate bool,
) (string, error) {
	// Apply defaults
	if containerRepository == "" {
		containerRepository = "ttl.sh"
	}
	if helmRepository == "" {
		helmRepository = "oci://ttl.sh"
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
