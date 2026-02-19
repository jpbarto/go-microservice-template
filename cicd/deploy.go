package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"dagger/goserv/internal/dagger"
)

// Deploy installs the Helm chart from a Helm repository to a Kubernetes cluster
// +cache = "never"
func (m *Goserv) Deploy(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// +optional
	// AWS configuration
	awsconfig *dagger.Secret,
	// +optional
	// Kubernetes config file content
	kubeconfig *dagger.Secret,
	// +optional
	// Helm chart repository URL (default: oci://ttl.sh)
	helmRepository string,
	// +optional
	// Container repository URL (default: ttl.sh)
	containerRepository string,
	// +optional
	// Build as release candidate (appends -rc to version tag)
	releaseCandidate bool,
	// +optional
	// Delivery context from Deliver function
	deliveryContext *dagger.File,
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

	if deliveryContext != nil {
		contextContent, err := deliveryContext.Contents(ctx)
		if err == nil {
			var delContext map[string]interface{}
			if err := json.Unmarshal([]byte(contextContent), &delContext); err == nil {
				if version, ok := delContext["version"].(string); ok {
					tag = version
				}
				if imgRef, ok := delContext["imageReference"].(string); ok {
					imageReference = imgRef
				}
			}
		}
	}

	// If tag wasn't in deliveryContext, read from VERSION file
	if tag == "" {
		versionContent, err := source.File("VERSION").Contents(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to read VERSION file: %w", err)
		}
		tag = strings.TrimSpace(versionContent)

		// Append -rc suffix for release candidates
		if releaseCandidate {
			tag = tag + "-rc"
		}
	}

	// Construct the chart reference
	chartRef := helmRepository + "/charts/goserv:" + tag

	// Create a container with kubectl and helm installed
	container := dag.Container().
		From("debian:bookworm-slim").
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{"apt-get", "install", "-y", "curl", "gnupg", "apt-transport-https"}).
		WithExec([]string{"sh", "-c", "curl -fsSL https://packages.buildkite.com/helm-linux/helm-debian/gpgkey | gpg --dearmor | tee /usr/share/keyrings/helm.gpg > /dev/null"}).
		WithExec([]string{"sh", "-c", "echo \"deb [signed-by=/usr/share/keyrings/helm.gpg] https://packages.buildkite.com/helm-linux/helm-debian/any/ any main\" | tee /etc/apt/sources.list.d/helm-stable-debian.list"}).
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{"apt-get", "install", "-y", "helm", "wget"}).
		WithExec([]string{"sh", "-c", "curl -LO https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"}).
		WithExec([]string{"install", "-o", "root", "-g", "root", "-m", "0755", "kubectl", "/usr/local/bin/kubectl"})

	// Mount kubeconfig if provided
	if kubeconfig != nil {
		container = container.
			WithSecretVariable("KUBECONFIG_CONTENT", kubeconfig).
			WithExec([]string{"sh", "-c", "mkdir -p /root/.kube && echo \"$KUBECONFIG_CONTENT\" > /root/.kube/config"})
	}

	// Mount awsconfig if provided
	if awsconfig != nil {
		container = container.
			WithSecretVariable("AWS_CONFIG_CONTENT", awsconfig).
			WithExec([]string{"sh", "-c", "mkdir -p /root/.aws && echo \"$AWS_CONFIG_CONTENT\" > /root/.aws/config"})
	}

	container = container.WithWorkdir("/workspace")

	// Perform the helm upgrade/install
	// Using --force to ensure each deployment creates a new revision,
	// even when downgrading to an older version (e.g., in rollback scenarios)
	output, err := container.
		WithEnvVariable("CACHE_BUSTER", time.Now().String()).
		WithExec([]string{
			"helm", "upgrade", "--install", releaseName,
			chartRef,
			"--namespace", namespace,
			"--create-namespace",
			"--force",
			"--wait",
		}).
		Stdout(ctx)

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
