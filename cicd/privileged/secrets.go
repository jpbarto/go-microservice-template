package privileged

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"dagger.io/dagger"
)

// INJECTED SECRETS - These values are replaced at runtime by manage_privileged.sh
// DO NOT modify these constants manually - they are template placeholders
const (
	// __INJECTED_KUBECONFIG__ will be replaced with the actual kubeconfig content
	injectedKubeconfig = `__INJECTED_KUBECONFIG__`

	// __INJECTED_KUBECTL_CONTEXT__ will be replaced with the kubectl context
	injectedKubectlContext = `__INJECTED_KUBECTL_CONTEXT__`

	// __INJECTED_HELM_TIMEOUT__ will be replaced with the helm timeout
	injectedHelmTimeout = `__INJECTED_HELM_TIMEOUT__`
)

// k8sConfig holds shared Kubernetes configuration for kubectl and helm operations.
// This is a private struct used internally by privileged functions to avoid
// passing kubeconfig repeatedly across function calls.
type k8sConfig struct {
	kubeconfig *dagger.Secret
	context    string
}

// newK8sConfig creates a new Kubernetes configuration using injected secrets.
// The secrets are injected at runtime before Dagger execution, so user-defined
// Dagger functions never have direct access to the sensitive credentials.
func newK8sConfig(ctx context.Context, client *dagger.Client) (*k8sConfig, error) {
	// Check if kubeconfig was injected
	if injectedKubeconfig == "__INJECTED_KUBECONFIG__" || injectedKubeconfig == "" {
		return nil, fmt.Errorf("kubeconfig not injected - this function must be called after runtime injection")
	}

	// Create Dagger secret from injected kubeconfig
	secret := client.SetSecret("kubeconfig", injectedKubeconfig)

	// Use injected context (empty string is valid)
	kubectlContext := injectedKubectlContext
	if kubectlContext == "__INJECTED_KUBECTL_CONTEXT__" {
		kubectlContext = ""
	}

	return &k8sConfig{
		kubeconfig: secret,
		context:    kubectlContext,
	}, nil
}

// GetKubectlContext returns the injected kubectl context.
// Returns empty string if no context was set.
func GetKubectlContext() string {
	if injectedKubectlContext == "__INJECTED_KUBECTL_CONTEXT__" {
		return ""
	}
	return injectedKubectlContext
}

// GetHelmTimeout returns the injected Helm timeout value.
// Returns "5m" as default if not set.
func GetHelmTimeout() string {
	if injectedHelmTimeout == "__INJECTED_HELM_TIMEOUT__" || injectedHelmTimeout == "" {
		return "5m" // default
	}
	return injectedHelmTimeout
}

// LoadKubeconfig returns the injected kubeconfig as a Dagger secret.
// The kubeconfig is injected at runtime before Dagger execution.
//
// Parameters:
//   - ctx: Context for the operation
//   - client: Dagger client instance
//
// Returns a Dagger secret containing the kubeconfig content.
//
// Example usage:
//
//	kubeconfigSecret, err := privileged.LoadKubeconfig(ctx, client)
//	if err != nil {
//	    return err
//	}
func LoadKubeconfig(ctx context.Context, client *dagger.Client) (*dagger.Secret, error) {
	cfg, err := newK8sConfig(ctx, client)
	if err != nil {
		return nil, err
	}
	return cfg.kubeconfig, nil
}

// GetSecretPath returns the path to a secret file in ~/.cicd-local/secrets/.
// This is useful for storing sensitive values outside the project directory.
//
// Parameters:
//   - secretName: Name of the secret file
//
// Returns the full path to the secret file.
//
// Example usage:
//
//	secretPath, err := privileged.GetSecretPath("api-token")
//	if err != nil {
//	    return err
//	}
func GetSecretPath(secretName string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	secretsDir := filepath.Join(homeDir, ".cicd-local", "secrets")
	secretPath := filepath.Join(secretsDir, secretName)

	if _, err := os.Stat(secretPath); os.IsNotExist(err) {
		return "", fmt.Errorf("secret file not found: %s", secretPath)
	}

	return secretPath, nil
}

// LoadSecretFile reads a secret from ~/.cicd-local/secrets/.
// Returns the secret content as a byte slice.
//
// Example usage:
//
//	content, err := privileged.LoadSecretFile("api-token")
//	if err != nil {
//	    return err
//	}
func LoadSecretFile(secretName string) ([]byte, error) {
	secretPath, err := GetSecretPath(secretName)
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(secretPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read secret file: %w", err)
	}

	return content, nil
}

// LoadSecretAsDaggerSecret loads a secret file as a Dagger secret.
// This is useful for passing secrets to Dagger containers.
//
// Parameters:
//   - client: Dagger client instance
//   - secretName: Name of the secret file in ~/.cicd-local/secrets/
//
// Returns a Dagger secret.
//
// Example usage:
//
//	apiToken, err := privileged.LoadSecretAsDaggerSecret(client, "api-token")
//	if err != nil {
//	    return err
//	}
func LoadSecretAsDaggerSecret(client *dagger.Client, secretName string) (*dagger.Secret, error) {
	content, err := LoadSecretFile(secretName)
	if err != nil {
		return nil, err
	}

	return client.SetSecret(secretName, string(content)), nil
}

// GetEnvOrSecret attempts to get a value from environment variable first,
// then falls back to reading from a secret file in ~/.cicd-local/secrets/.
//
// Parameters:
//   - envVar: Environment variable name to check first
//   - secretName: Secret file name to fall back to
//
// Returns the value as a string.
//
// Example usage:
//
//	apiKey, err := privileged.GetEnvOrSecret("API_KEY", "api-key")
//	if err != nil {
//	    return err
//	}
func GetEnvOrSecret(envVar, secretName string) (string, error) {
	// Try environment variable first
	if value := os.Getenv(envVar); value != "" {
		return value, nil
	}

	// Fall back to secret file
	content, err := LoadSecretFile(secretName)
	if err != nil {
		return "", fmt.Errorf("failed to get value from env var %s or secret file %s: %w", envVar, secretName, err)
	}

	return string(content), nil
}
