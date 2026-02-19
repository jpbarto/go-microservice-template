package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"dagger/goserv/internal/dagger"
)

// Validate runs the validation script to verify that the deployment is healthy and functioning correctly
func (m *Goserv) Validate(
	ctx context.Context,
	// Source directory containing the project
	source *dagger.Directory,
	// Kubernetes config file content
	kubeconfig *dagger.Secret,
	// +optional
	// AWS configuration
	awsconfig *dagger.Secret,
	// Deployment context from Deploy function
	deploymentContext *dagger.File,
	// +optional
	// Build as release candidate (appends -rc to version)
	releaseCandidate bool,
) (*dagger.File, error) {
	// Extract deployment information from context if provided
	releaseName := "goserv"
	namespace := "goserv"

	var endpoint string
	if deploymentContext != nil {
		contextContent, err := deploymentContext.Contents(ctx)
		if err == nil {
			var depContext map[string]interface{}
			if err := json.Unmarshal([]byte(contextContent), &depContext); err == nil {
				if ep, ok := depContext["endpoint"].(string); ok {
					endpoint = ep
				}
				if rn, ok := depContext["releaseName"].(string); ok && releaseName == "goserv" {
					releaseName = rn
				}
				if ns, ok := depContext["namespace"].(string); ok && namespace == "goserv" {
					namespace = ns
				}
			}
		}
	}

	var expectedVersion string
	versionContent, err := source.File("VERSION").Contents(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read VERSION file: %w", err)
	}
	expectedVersion = strings.TrimSpace(versionContent)

	// Append -rc suffix for release candidates
	if releaseCandidate {
		expectedVersion = expectedVersion + "-rc"
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
		WithEnvVariable("EXPECTED_VERSION", expectedVersion).
		WithExec([]string{"bash", "/workspace/tests/validate.sh"}).
		Stdout(ctx)

	if err != nil {
		return nil, err
	}

	// Determine validation status from output
	status := "healthy"
	if err != nil {
		status = "failed"
	}

	// Create validation context JSON
	validationContext := map[string]interface{}{
		"timestamp":        time.Now().Format(time.RFC3339),
		"releaseName":      releaseName,
		"endpoint":         endpoint,
		"namespace":        namespace,
		"expectedVersion":  expectedVersion,
		"status":           status,
		"healthChecks":     []string{"pod-ready", "service-available"},
		"readinessChecks":  []string{"http-200"},
		"validationOutput": validationOutput,
	}

	contextJSON, err := json.MarshalIndent(validationContext, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal validation context: %w", err)
	}

	// Return validation context as a file
	return dag.Directory().
		WithNewFile("validationContext", string(contextJSON)).
		File("validationContext"), nil
}
