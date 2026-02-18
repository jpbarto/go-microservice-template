package main

import (
	"context"
	"strings"

	"dagger/goserv/internal/dagger"
)

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
