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

// Deploy installs the Helm chart from a Helm repository to a Kubernetes cluster
// +cache = "never"
func (m *Goserv) Deploy(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// +optional
	// Helm chart repository URL (default: oci://ttl.sh)
	helmRepository string,
	// +optional
	// Container repository URL (default: ttl.sh)
	containerRepository string,
	// +optional
	// Build as release candidate (appends -rc to version tag)
	releaseCandidate bool,
) (*dagger.File, error) {
	if helmRepository == "" {
		helmRepository = "oci://ttl.sh"
	}
	if containerRepository == "" {
		containerRepository = "ttl.sh"
	}

	// Extract configuration from deliveryContext if provided
	var releaseName, namespace, tag, imageReference string
	releaseName = "goserv"
	namespace = "goserv"

	// Read tag from VERSION file
	versionContent, err := source.File("VERSION").Contents(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read VERSION file: %w", err)
	}
	tag = strings.TrimSpace(versionContent)

	// Append -rc suffix for release candidates
	if releaseCandidate {
		tag = tag + "-rc"
	}

	// Construct the chart reference
	chartRef := helmRepository + "/goserv:" + tag

	// Use the privileged HelmUpgrade function to deploy to the Kubernetes cluster.
	// This delegates helm execution to the pre-built privileged wrapper which uses
	// injected kubeconfig secrets rather than building a container inline.
	output, err := cicd.HelmInstall(ctx, dag, releaseName, chartRef, namespace, nil)
	if err != nil {
		return nil, err
	}

	// Construct endpoint URL for the deployed service
	endpoint := fmt.Sprintf("http://%s.%s.svc.cluster.local:8080", releaseName, namespace)

	// Create deployment context JSON
	deploymentContext := map[string]interface{}{
		"timestamp":      time.Now().Format(time.RFC3339),
		"endpoint":       endpoint,
		"releaseName":    releaseName,
		"namespace":      namespace,
		"chartVersion":   tag,
		"imageReference": imageReference,
		"helmOutput":     output,
	}

	contextJSON, err := json.MarshalIndent(deploymentContext, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal deployment context: %w", err)
	}

	// Return deployment context as a file
	return dag.Directory().
		WithNewFile("deploymentContext", string(contextJSON)).
		File("deploymentContext"), nil
}
