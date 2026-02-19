package main

import (
	"context"
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
	// AWS configuration file content
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
) (string, error) {
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

	// Construct the chart reference
	chartRef := helmRepository + "/charts/goserv:" + tag

	// Create a container with kubectl and helm installed
	// Note: We need to install kubectl to list namespaces
	container := dag.Container().
		From("debian:bookworm-slim").
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{"apt-get", "install", "-y", "curl", "gnupg", "apt-transport-https"}).
		WithExec([]string{"sh", "-c", "curl -fsSL https://packages.buildkite.com/helm-linux/helm-debian/gpgkey | gpg --dearmor | tee /usr/share/keyrings/helm.gpg > /dev/null"}).
		WithExec([]string{"sh", "-c", "echo \"deb [signed-by=/usr/share/keyrings/helm.gpg] https://packages.buildkite.com/helm-linux/helm-debian/any/ any main\" | tee /etc/apt/sources.list.d/helm-stable-debian.list"}).
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{"apt-get", "install", "-y", "helm", "wget"}).
		WithExec([]string{"sh", "-c", "curl -LO https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"}).
		WithExec([]string{"install", "-o", "root", "-g", "root", "-m", "0755", "kubectl", "/usr/local/bin/kubectl"}).
		WithSecretVariable("KUBECONFIG_CONTENT", kubeconfig).
		WithExec([]string{"sh", "-c", "mkdir -p /root/.kube && echo \"$KUBECONFIG_CONTENT\" > /root/.kube/config"}).
		WithWorkdir("/workspace")

	// Perform the helm upgrade/install
	// Using --force to ensure each deployment creates a new revision,
	// even when downgrading to an older version (e.g., in rollback scenarios)
	releaseName := "goserv"
	namespace := "goserv"
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
		return "", err
	}

	return output, nil
}
