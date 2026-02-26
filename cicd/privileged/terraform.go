package privileged

import (
	"context"
	"fmt"
	"os"

	"dagger.io/dagger"
)

// baseImage is the minimal Debian container used as a base for OpenTofu operations.
const baseImage = "debian:bookworm-slim"

// openTofuContainer returns a Dagger container with the OpenTofu binary installed.
// It starts from a minimal Debian image, installs required dependencies, and then
// uses the official OpenTofu install script to install the tofu CLI.
func openTofuContainer(client *dagger.Client) *dagger.Container {
	return client.Container().
		From(baseImage).
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{"apt-get", "install", "-y", "--no-install-recommends",
			"curl", "gnupg", "software-properties-common", "git", "unzip", "ca-certificates"}).
		WithExec([]string{"sh", "-c",
			"curl --proto '=https' --tlsv1.2 -fsSL https://get.opentofu.org/install-opentofu.sh -o install-opentofu.sh && " +
				"chmod +x install-opentofu.sh && " +
				"./install-opentofu.sh --install-method deb && " +
				"rm -f install-opentofu.sh"}).
		WithExec([]string{"apt-get", "clean"}).
		WithExec([]string{"rm", "-rf", "/var/lib/apt/lists/*"}).
		WithExec([]string{"tofu", "--version"})
}

// TerraformPlan runs tofu plan and returns the generated plan file.
// This function executes tofu init and plan in the provided directory,
// producing a binary plan file that can be inspected or applied later.
//
// Parameters:
//   - ctx: Context for the operation
//   - client: Dagger client instance
//   - terraformDir: Directory containing Terraform/OpenTofu configuration files
//   - varFile: Optional directory containing terraform.tfvars file (can be nil)
//
// Environment variables:
//   - TF_VAR_*: Terraform variables (e.g., TF_VAR_region=us-east-1)
//   - AWS_ACCESS_KEY_ID: AWS access key (if using AWS provider)
//   - AWS_SECRET_ACCESS_KEY: AWS secret key (if using AWS provider)
//   - AWS_SESSION_TOKEN: AWS session token (optional)
//   - AWS_REGION: AWS region (optional)
//
// Returns the plan as a *dagger.File (binary plan file).
//
// Example usage:
//
//	planFile, err := privileged.TerraformPlan(ctx, client, terraformDir, varFileDir)
//	if err != nil {
//	    return nil, fmt.Errorf("terraform plan failed: %w", err)
//	}
func TerraformPlan(
	ctx context.Context,
	client *dagger.Client,
	terraformDir *dagger.Directory,
	varFile *dagger.Directory,
) (*dagger.File, error) {
	if terraformDir == nil {
		return nil, fmt.Errorf("terraform directory is required")
	}

	// Start with Debian container with OpenTofu installed
	container := openTofuContainer(client).
		WithMountedDirectory("/terraform", terraformDir).
		WithWorkdir("/terraform")

	// Add var file if provided
	if varFile != nil {
		container = container.WithMountedDirectory("/vars", varFile)
	}

	// Pass through environment variables for Terraform/OpenTofu
	container = passThroughTerraformEnv(container)

	// Initialize OpenTofu
	container = container.WithExec([]string{"tofu", "init"})

	// Build tofu plan command — write plan to a file
	planArgs := []string{"tofu", "plan", "-no-color", "-out=/terraform/tfplan"}

	// Add var file if provided
	if varFile != nil {
		planArgs = append(planArgs, "-var-file=/vars/terraform.tfvars")
	}

	// Execute tofu plan
	container = container.WithExec(planArgs)

	// Return the binary plan file
	planFile := container.File("/terraform/tfplan")

	return planFile, nil
}

// TerraformApply runs tofu apply and returns the resulting state file.
// This function executes tofu init and apply in the provided directory,
// then returns the terraform.tfstate file produced by the apply.
//
// Parameters:
//   - ctx: Context for the operation
//   - client: Dagger client instance
//   - terraformDir: Directory containing Terraform/OpenTofu configuration files
//   - varFile: Optional directory containing terraform.tfvars file (can be nil)
//
// Environment variables:
//   - TF_VAR_*: Terraform variables (e.g., TF_VAR_region=us-east-1)
//   - AWS_ACCESS_KEY_ID: AWS access key (if using AWS provider)
//   - AWS_SECRET_ACCESS_KEY: AWS secret key (if using AWS provider)
//   - AWS_SESSION_TOKEN: AWS session token (optional)
//   - AWS_REGION: AWS region (optional)
//
// Returns the state file as a *dagger.File (terraform.tfstate).
//
// Example usage:
//
//	stateFile, err := privileged.TerraformApply(ctx, client, terraformDir, varFileDir)
//	if err != nil {
//	    return nil, fmt.Errorf("terraform apply failed: %w", err)
//	}
func TerraformApply(
	ctx context.Context,
	client *dagger.Client,
	terraformDir *dagger.Directory,
	varFile *dagger.Directory,
) (*dagger.File, error) {
	if terraformDir == nil {
		return nil, fmt.Errorf("terraform directory is required")
	}

	// Start with Debian container with OpenTofu installed
	container := openTofuContainer(client).
		WithMountedDirectory("/terraform", terraformDir).
		WithWorkdir("/terraform")

	// Add var file if provided
	if varFile != nil {
		container = container.WithMountedDirectory("/vars", varFile)
	}

	// Pass through environment variables for Terraform/OpenTofu
	container = passThroughTerraformEnv(container)

	// Initialize OpenTofu
	container = container.WithExec([]string{"tofu", "init"})

	// Build tofu apply command
	applyArgs := []string{"tofu", "apply", "-no-color", "-auto-approve"}

	// Add var file if provided
	if varFile != nil {
		applyArgs = append(applyArgs, "-var-file=/vars/terraform.tfvars")
	}

	// Execute tofu apply
	container = container.WithExec(applyArgs)

	// Return the state file produced by apply
	stateFile := container.File("/terraform/terraform.tfstate")

	return stateFile, nil
}

// passThroughTerraformEnv passes through relevant environment variables to the OpenTofu container.
func passThroughTerraformEnv(container *dagger.Container) *dagger.Container {
	// AWS credentials
	if val := os.Getenv("AWS_ACCESS_KEY_ID"); val != "" {
		container = container.WithEnvVariable("AWS_ACCESS_KEY_ID", val)
	}
	if val := os.Getenv("AWS_SECRET_ACCESS_KEY"); val != "" {
		container = container.WithEnvVariable("AWS_SECRET_ACCESS_KEY", val)
	}
	if val := os.Getenv("AWS_SESSION_TOKEN"); val != "" {
		container = container.WithEnvVariable("AWS_SESSION_TOKEN", val)
	}
	if val := os.Getenv("AWS_REGION"); val != "" {
		container = container.WithEnvVariable("AWS_REGION", val)
	}

	// Pass through all TF_VAR_* environment variables
	for _, env := range os.Environ() {
		if len(env) > 7 && env[:7] == "TF_VAR_" {
			// Split on first '='
			parts := splitOnce(env, "=")
			if len(parts) == 2 {
				container = container.WithEnvVariable(parts[0], parts[1])
			}
		}
	}

	return container
}

// splitOnce splits a string on the first occurrence of sep.
func splitOnce(s, sep string) []string {
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			return []string{s[:i], s[i+len(sep):]}
		}
	}
	return []string{s}
}
