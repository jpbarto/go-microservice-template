package privileged

import (
	"context"
	"fmt"
	"os"

	"dagger.io/dagger"
)

// KubectlApply applies Kubernetes manifests using kubectl.
// This function executes kubectl apply with the provided manifests directory.
//
// Parameters:
//   - ctx: Context for the operation
//   - client: Dagger client instance
//   - manifestsDir: Directory containing Kubernetes YAML manifests
//   - namespace: Kubernetes namespace to apply to (optional, uses manifest default if empty)
//   - kubeconfig: Dagger secret containing kubeconfig content
//
// Environment variables:
//   - KUBECTL_CONTEXT: Kubernetes context to use (optional)
//
// Returns the kubectl apply output as a string.
//
// Example usage:
//
//	kubeconfigSecret, err := privileged.LoadKubeconfig(ctx, client, "")
//	output, err := privileged.KubectlApply(ctx, client, manifestsDir, "default", kubeconfigSecret)
//	if err != nil {
//	    return "", fmt.Errorf("kubectl apply failed: %w", err)
//	}
func KubectlApply(
	ctx context.Context,
	client *dagger.Client,
	manifestsDir *dagger.Directory,
	namespace string,
	kubeconfig *dagger.Secret,
) (string, error) {
	if manifestsDir == nil {
		return "", fmt.Errorf("manifests directory is required")
	}
	if kubeconfig == nil {
		return "", fmt.Errorf("kubeconfig secret is required")
	}

	// Start with kubectl container
	container := client.Container().
		From("bitnami/kubectl:latest").
		WithMountedDirectory("/manifests", manifestsDir).
		WithMountedSecret("/root/.kube/config", kubeconfig)

	// Build kubectl command
	args := []string{"kubectl", "apply", "-f", "/manifests"}

	// Add namespace if specified
	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	// Add context if specified in environment
	if kubectlContext := os.Getenv("KUBECTL_CONTEXT"); kubectlContext != "" {
		args = append(args, "--context", kubectlContext)
	}

	// Execute kubectl apply
	output, err := container.WithExec(args).Stdout(ctx)
	if err != nil {
		return "", fmt.Errorf("kubectl apply failed: %w", err)
	}

	return output, nil
}

// KubectlGet retrieves a Kubernetes resource and returns it as JSON.
// This function executes kubectl get with the specified resource and namespace.
//
// Parameters:
//   - ctx: Context for the operation
//   - client: Dagger client instance
//   - namespace: Kubernetes namespace containing the resource
//   - resourceName: Resource to get (e.g., "pod/mypod", "deployment/myapp", "service/mysvc")
//   - kubeconfig: Dagger secret containing kubeconfig content
//
// Environment variables:
//   - KUBECTL_CONTEXT: Kubernetes context to use (optional)
//
// Returns the resource as JSON string.
//
// Example usage:
//
//	kubeconfigSecret, err := privileged.LoadKubeconfig(ctx, client, "")
//	podJSON, err := privileged.KubectlGet(ctx, client, "default", "pod/mypod", kubeconfigSecret)
//	if err != nil {
//	    return "", fmt.Errorf("kubectl get failed: %w", err)
//	}
func KubectlGet(
	ctx context.Context,
	client *dagger.Client,
	namespace string,
	resourceName string,
	kubeconfig *dagger.Secret,
) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}
	if resourceName == "" {
		return "", fmt.Errorf("resource name is required")
	}
	if kubeconfig == nil {
		return "", fmt.Errorf("kubeconfig secret is required")
	}

	// Start with kubectl container
	container := client.Container().
		From("bitnami/kubectl:latest").
		WithMountedSecret("/root/.kube/config", kubeconfig)

	// Build kubectl command with JSON output
	args := []string{"kubectl", "get", resourceName, "-n", namespace, "-o", "json"}

	// Add context if specified in environment
	if kubectlContext := os.Getenv("KUBECTL_CONTEXT"); kubectlContext != "" {
		args = append(args, "--context", kubectlContext)
	}

	// Execute kubectl get
	output, err := container.WithExec(args).Stdout(ctx)
	if err != nil {
		return "", fmt.Errorf("kubectl get failed: %w", err)
	}

	return output, nil
}

// KubectlPortForward creates a port-forwarding tunnel to a Kubernetes resource.
// This function returns a Service that can be used by other Dagger functions to connect
// to the forwarded port. The port forwarding runs in the background as a service.
//
// Parameters:
//   - ctx: Context for the operation
//   - client: Dagger client instance
//   - namespace: Kubernetes namespace containing the resource
//   - resourceName: Resource to forward to (e.g., "pod/mypod", "deployment/myapp", "service/mysvc")
//   - ports: Port mapping in format "localPort:remotePort" (e.g., "8080:80", "3000:3000")
//   - kubeconfig: Dagger secret containing kubeconfig content
//
// Environment variables:
//   - KUBECTL_CONTEXT: Kubernetes context to use (optional)
//
// Returns a Dagger Service that forwards the specified ports.
//
// Example usage:
//
//	kubeconfigSecret, err := privileged.LoadKubeconfig(ctx, client, "")
//	portForwardSvc, err := privileged.KubectlPortForward(ctx, client, "default", "pod/mypod", "8080:80", kubeconfigSecret)
//	if err != nil {
//	    return err
//	}
//	// Use the service in another container
//	testContainer := client.Container().
//	    From("curlimages/curl:latest").
//	    WithServiceBinding("app", portForwardSvc).
//	    WithExec([]string{"curl", "http://app:8080/health"})
func KubectlPortForward(
	ctx context.Context,
	client *dagger.Client,
	namespace string,
	resourceName string,
	ports string,
	kubeconfig *dagger.Secret,
) (*dagger.Service, error) {
	if namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}
	if resourceName == "" {
		return nil, fmt.Errorf("resource name is required")
	}
	if ports == "" {
		return nil, fmt.Errorf("ports are required (format: localPort:remotePort)")
	}
	if kubeconfig == nil {
		return nil, fmt.Errorf("kubeconfig secret is required")
	}

	// Start with kubectl container
	container := client.Container().
		From("bitnami/kubectl:latest").
		WithMountedSecret("/root/.kube/config", kubeconfig)

	// Build kubectl port-forward command
	args := []string{
		"kubectl", "port-forward",
		resourceName,
		ports,
		"-n", namespace,
		"--address", "0.0.0.0", // Listen on all interfaces so Dagger can access it
	}

	// Add context if specified in environment
	if kubectlContext := os.Getenv("KUBECTL_CONTEXT"); kubectlContext != "" {
		args = append(args, "--context", kubectlContext)
	}

	// Extract the local port from the ports string (e.g., "8080:80" -> 8080)
	localPort := ports
	for i, c := range ports {
		if c == ':' {
			localPort = ports[:i]
			break
		}
	}

	// Create and return the service
	// The service will run kubectl port-forward in the background
	service := container.
		WithExec(args).
		WithExposedPort(parsePort(localPort)).
		AsService()

	return service, nil
}

// parsePort converts a port string to an integer for WithExposedPort
func parsePort(portStr string) int {
	port := 0
	for _, c := range portStr {
		if c >= '0' && c <= '9' {
			port = port*10 + int(c-'0')
		}
	}
	if port == 0 {
		return 8080 // default fallback
	}
	return port
}
