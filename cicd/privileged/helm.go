package privileged

import (
	"context"
	"fmt"
	"os"

	"dagger.io/dagger"
)

// HelmInstall installs or upgrades a Helm chart.
// This function performs a Helm install/upgrade operation with the provided configuration.
//
// Parameters:
//   - ctx: Context for the operation
//   - client: Dagger client instance
//   - releaseName: Name of the Helm release
//   - chartPath: Directory containing the Helm chart or chart reference (e.g., "stable/nginx")
//   - namespace: Kubernetes namespace to install into
//   - valuesFile: Optional directory containing values.yaml file (can be nil)
//   - kubeconfig: Dagger secret containing kubeconfig content
//
// Environment variables:
//   - HELM_TIMEOUT: Timeout for helm operations (default: 5m)
//   - KUBECTL_CONTEXT: Kubernetes context to use (optional)
//
// Returns the helm install output as a string.
//
// Example usage:
//
//	kubeconfigSecret, err := privileged.LoadKubeconfig(ctx, client, "")
//	output, err := privileged.HelmInstall(ctx, client, "myapp", chartDir, "default", valuesDir, kubeconfigSecret)
//	if err != nil {
//	    return "", fmt.Errorf("helm install failed: %w", err)
//	}
func HelmInstall(
	ctx context.Context,
	client *dagger.Client,
	releaseName string,
	chartPath *dagger.Directory,
	namespace string,
	valuesFile *dagger.Directory,
	kubeconfig *dagger.Secret,
) (string, error) {
	if releaseName == "" {
		return "", fmt.Errorf("release name is required")
	}
	if chartPath == nil {
		return "", fmt.Errorf("chart path is required")
	}
	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}
	if kubeconfig == nil {
		return "", fmt.Errorf("kubeconfig secret is required")
	}

	// Start with Helm container
	container := client.Container().
		From("alpine/helm:latest").
		WithMountedDirectory("/chart", chartPath).
		WithMountedSecret("/root/.kube/config", kubeconfig)

	// Add values file if provided
	if valuesFile != nil {
		container = container.WithMountedDirectory("/values", valuesFile)
	}

	// Build helm command
	args := []string{
		"helm", "upgrade", "--install",
		releaseName, "/chart",
		"-n", namespace,
		"--create-namespace",
	}

	// Add values file if provided
	if valuesFile != nil {
		args = append(args, "-f", "/values/values.yaml")
	}

	// Add timeout if specified
	if timeout := os.Getenv("HELM_TIMEOUT"); timeout != "" {
		args = append(args, "--timeout", timeout)
	} else {
		args = append(args, "--timeout", "5m")
	}

	// Add context if specified
	if kubectlContext := os.Getenv("KUBECTL_CONTEXT"); kubectlContext != "" {
		args = append(args, "--kube-context", kubectlContext)
	}

	// Execute helm install/upgrade
	output, err := container.WithExec(args).Stdout(ctx)
	if err != nil {
		return "", fmt.Errorf("helm install failed: %w", err)
	}

	return output, nil
}

// HelmUpgrade upgrades an existing Helm release.
// This function performs a Helm upgrade operation for an already installed release.
//
// Parameters:
//   - ctx: Context for the operation
//   - client: Dagger client instance
//   - releaseName: Name of the Helm release to upgrade
//   - chartReference: Chart reference (can be a repo/chart or local path)
//   - namespace: Kubernetes namespace containing the release
//   - kubeconfig: Dagger secret containing kubeconfig content
//
// Environment variables:
//   - HELM_TIMEOUT: Timeout for helm operations (default: 5m)
//   - KUBECTL_CONTEXT: Kubernetes context to use (optional)
//
// Returns the helm upgrade output as a string.
//
// Example usage:
//
//	kubeconfigSecret, err := privileged.LoadKubeconfig(ctx, client, "")
//	output, err := privileged.HelmUpgrade(ctx, client, "myapp", "bitnami/nginx", "production", kubeconfigSecret)
//	if err != nil {
//	    return "", fmt.Errorf("helm upgrade failed: %w", err)
//	}
func HelmUpgrade(
	ctx context.Context,
	client *dagger.Client,
	releaseName string,
	chartReference string,
	namespace string,
	kubeconfig *dagger.Secret,
) (string, error) {
	if releaseName == "" {
		return "", fmt.Errorf("release name is required")
	}
	if chartReference == "" {
		return "", fmt.Errorf("chart reference is required")
	}
	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}
	if kubeconfig == nil {
		return "", fmt.Errorf("kubeconfig secret is required")
	}

	// Start with Helm container
	container := client.Container().
		From("alpine/helm:latest").
		WithMountedSecret("/root/.kube/config", kubeconfig)

	// Build helm upgrade command
	args := []string{
		"helm", "upgrade",
		releaseName, chartReference,
		"-n", namespace,
	}

	// Add timeout if specified
	if timeout := os.Getenv("HELM_TIMEOUT"); timeout != "" {
		args = append(args, "--timeout", timeout)
	} else {
		args = append(args, "--timeout", "5m")
	}

	// Add context if specified
	if kubectlContext := os.Getenv("KUBECTL_CONTEXT"); kubectlContext != "" {
		args = append(args, "--kube-context", kubectlContext)
	}

	// Execute helm upgrade
	output, err := container.WithExec(args).Stdout(ctx)
	if err != nil {
		return "", fmt.Errorf("helm upgrade failed: %w", err)
	}

	return output, nil
}
