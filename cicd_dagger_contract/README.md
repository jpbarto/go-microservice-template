# Dagger CI Function Contract

This directory contains the standardized function signatures (contract) for the Dagger CI/CD pipeline. These function signatures define the interface that any implementation must adhere to, ensuring consistency across different deployment strategies (e.g., Helm-based, ArgoCD-based).

## Overview

The CI/CD pipeline consists of six core functions that represent different stages of the software delivery lifecycle. Each function has a well-defined signature with specific parameters and return types that enable composability and flexibility.

## Function Execution Order

Based on the actual pipeline scripts in the `cicd-local` directory, these functions are invoked in the following stages and order:

### 1. **CI Pipeline** (`local_ci_pipeline.sh`)
**Commit Pipeline**:
```
Build → UnitTest
```

**PR Merge Pipeline**:
```
Build → UnitTest → Deliver
```

- **Purpose**: Validates code changes on branch commits and prepares artifacts on PR merges
- **Build**: Compiles multi-architecture container image and exports as OCI tarball to `./build/goserv-image.tar`
- **UnitTest**: Executes unit tests against the built image tarball
- **Deliver**: (PR merge only) Publishes container images and Helm charts to repositories
- **Notes**: PR merges automatically set `--release-candidate=true`

### 2. **IAT Pipeline** (`local_iat_pipeline.sh`) - Integration Acceptance Testing
```
Deploy → Validate → IntegrationTest
```

- **Purpose**: Deploys application to local Kubernetes (Colima) and runs comprehensive integration tests
- **Deploy**: Deploys the application using Helm with `--release-candidate=true`
- **Validate**: Verifies deployment health and version correctness
- **IntegrationTest**: Executes integration tests against the deployed instance via port-forward
- **Notes**: 
  - Always uses release candidate builds
  - Sets up kubectl port-forward to enable testing from Dagger containers
  - Uses `host.docker.internal` as target host for integration tests

### 3. **Staging Pipeline** (`local_staging_pipeline.sh`) - Blue-Green Deployment Testing
```
Phase 1: Deploy (current-rc) → Validate (current-rc)
Phase 2: Deploy (previous-release) → Validate (previous-release)
Phase 3: Deploy (current-rc) → Validate (current-rc)
```

- **Purpose**: Validates blue-green deployment scenarios and rollback capabilities
- **Phase 1**: Deploy and validate current version as release candidate (Green)
- **Phase 2**: Deploy and validate previous git tag release (Blue) - tests rollback
- **Phase 3**: Re-deploy and validate current version (Green) - tests re-deployment stability
- **Notes**:
  - Current version always uses `--release-candidate=true`
  - Previous release uses `--release-candidate=false`
  - Uses git checkout to switch between versions
  - Validates Helm revision tracking across deployments

### 4. **Delivery Pipeline** (`local_deliver_pipeline.sh`)
```
Build → Deliver
```

- **Purpose**: Builds and publishes artifacts to container and Helm chart repositories
- **Build**: Creates multi-architecture container image (can be skipped with `--skip-build`)
- **Deliver**: Publishes container images (multi-arch) and Helm charts to repositories
- **Notes**: 
  - Optionally use `--skip-build` to deliver existing tarball
  - Supports custom repository URLs via parameters

### 5. **Deploy Pipeline** (`local_deploy_pipeline.sh`)
```
Deploy → Validate
```

- **Purpose**: Deploys application to Kubernetes cluster and validates deployment
- **Deploy**: Installs/upgrades Helm release in target cluster
- **Validate**: Verifies deployment health, version, and functionality
- **Notes**: 
  - Can skip validation with `--skip-validation`
  - Works with Colima or any Kubernetes cluster
  - Uses kubeconfig from `~/.kube/config`

---

## Function Signatures

### 1. Build

**Purpose**: Builds a multi-architecture Docker image and exports it as an OCI tarball.

**Signature**:
```go
func (m *Goserv) Build(
    ctx context.Context,
    source *dagger.Directory,
    releaseCandidate bool,
) (*dagger.File, error)
```

**Parameters**:
- `ctx context.Context` - Go context for cancellation and timeout control
- `source *dagger.Directory` - Source directory containing the project files (Dockerfile, source code, etc.)
- `releaseCandidate bool` - (Optional) Whether to build as a release candidate (typically appends `-rc` suffix to version)

**Return Values**:
- `*dagger.File` - An OCI tarball containing the multi-architecture container image
- `error` - Error if build fails

**Usage Notes**:
- The function should read the version from a `VERSION` file in the source directory
- Should support multi-platform builds (e.g., `linux/amd64`, `linux/arm64`)
- The OCI tarball can be passed to subsequent functions or imported for testing
- If `releaseCandidate` is true, append `-rc` to the version tag

---

### 2. UnitTest

**Purpose**: Runs unit tests against the built application container.

**Signature**:
```go
func (m *Goserv) UnitTest(
    ctx context.Context,
    source *dagger.Directory,
    imageTarball *dagger.File,
) (string, error)
```

**Parameters**:
- `ctx context.Context` - Go context for cancellation and timeout control
- `source *dagger.Directory` - Source directory containing test scripts and fixtures
- `imageTarball *dagger.File` - (Optional) Pre-built OCI image tarball. If not provided, should call `Build()` internally

**Return Values**:
- `string` - Test output (stdout) from test execution
- `error` - Error if tests fail or cannot be executed

**Usage Notes**:
- Should start the application container as a service
- Execute test scripts (e.g., `tests/unit_test.sh`) against the running service
- If no `imageTarball` is provided, build from source automatically
- Tests should verify basic application functionality

---

### 3. IntegrationTest

**Purpose**: Runs integration tests against a deployed application instance.

**Signature**:
```go
func (m *Goserv) IntegrationTest(
    ctx context.Context,
    source *dagger.Directory,
    targetHost string,
    targetPort string,
) (string, error)
```

**Parameters**:
- `ctx context.Context` - Go context for cancellation and timeout control
- `source *dagger.Directory` - Source directory containing integration test scripts
- `targetHost string` - (Optional, default: `localhost`) Hostname where the application is deployed
- `targetPort string` - (Optional, default: `8080`) Port where the application is listening

**Return Values**:
- `string` - Test output (stdout) from test execution
- `error` - Error if tests fail or cannot be executed

**Usage Notes**:
- Tests run against an already-deployed instance (not a service started by this function)
- Should execute integration test scripts (e.g., `tests/integration_test.sh`)
- May include performance tests, acceptance tests, and end-to-end tests
- Commonly used with load testing tools (e.g., k6)

---

### 4. Deliver

**Purpose**: Publishes container images and Helm charts to artifact repositories.

**Signature**:
```go
func (m *Goserv) Deliver(
    ctx context.Context,
    source *dagger.Directory,
    containerRepository string,
    helmRepository string,
    imageTarball *dagger.File,
    releaseCandidate bool,
) (string, error)
```

**Parameters**:
- `ctx context.Context` - Go context for cancellation and timeout control
- `source *dagger.Directory` - Source directory containing Helm charts and application files
- `containerRepository string` - (Optional, default: `ttl.sh`) Container registry URL (e.g., `ghcr.io/org`, `docker.io/username`)
- `helmRepository string` - (Optional, default: `oci://ttl.sh`) Helm chart repository URL (OCI or classic HTTP)
- `imageTarball *dagger.File` - (Optional) Pre-built OCI image tarball. If not provided, should build from source
- `releaseCandidate bool` - (Optional) Whether this is a release candidate (affects version tagging)

**Return Values**:
- `string` - Delivery summary including published container image reference and Helm chart reference
- `error` - Error if publishing fails

**Usage Notes**:
- Should publish multi-architecture container images to the specified registry
- Should package and publish Helm charts to the chart repository
- Update Helm chart values with the correct image repository and tag
- Read version from `VERSION` file and append `-rc` if `releaseCandidate` is true
- Return references to published artifacts for traceability

---

### 5. Deploy

**Purpose**: Deploys the application to a Kubernetes cluster using Helm or other deployment mechanism.

**Signature**:
```go
func (m *Goserv) Deploy(
    ctx context.Context,
    source *dagger.Directory,
    kubeconfig *dagger.Secret,
    helmRepository string,
    releaseName string,
    namespace string,
    releaseCandidate bool,
) (string, error)
```

**Annotations**:
- `// +cache = "never"` - Disables Dagger's caching for this function to ensure fresh deployments

**Parameters**:
- `ctx context.Context` - Go context for cancellation and timeout control
- `source *dagger.Directory` - Source directory containing deployment manifests
- `kubeconfig *dagger.Secret` - Kubernetes configuration file content (as a secret for security)
- `helmRepository string` - (Optional, default: `oci://ttl.sh`) Helm chart repository URL
- `releaseName string` - (Optional, default: `goserv`) Helm release name
- `namespace string` - (Optional, default: `goserv`) Kubernetes namespace for deployment
- `releaseCandidate bool` - (Optional) Whether to deploy release candidate version

**Return Values**:
- `string` - Deployment output (e.g., Helm release info, kubectl output)
- `error` - Error if deployment fails

**Usage Notes**:
- Should use `kubectl` and/or `helm` to deploy the application
- For Helm-based deployments: `helm upgrade --install` with appropriate flags
- For ArgoCD-based deployments: `kubectl apply -f application.yaml`
- Read version from `VERSION` file to determine which chart/image version to deploy
- Should wait for deployment to be ready before returning
- The `kubeconfig` secret allows deployment to different clusters (local, staging, production)

---

### 6. Validate

**Purpose**: Validates that a deployment is healthy and functioning correctly.

**Signature**:
```go
func (m *Goserv) Validate(
    ctx context.Context,
    source *dagger.Directory,
    kubeconfig *dagger.Secret,
    releaseName string,
    namespace string,
    expectedVersion string,
    releaseCandidate bool,
) (string, error)
```

**Parameters**:
- `ctx context.Context` - Go context for cancellation and timeout control
- `source *dagger.Directory` - Source directory containing validation scripts
- `kubeconfig *dagger.Secret` - Kubernetes configuration file content
- `releaseName string` - (Optional, default: `goserv`) Helm release name to validate
- `namespace string` - (Optional, default: `goserv`) Kubernetes namespace to validate
- `expectedVersion string` - (Optional) Expected version to validate. If not provided, read from `VERSION` file
- `releaseCandidate bool` - (Optional) Whether validating a release candidate

**Return Values**:
- `string` - Validation output (test results, health check output)
- `error` - Error if validation fails

**Usage Notes**:
- Should verify deployment exists and is healthy (pods running, replicas ready)
- Should verify correct version is deployed
- Should check Kubernetes resources (Deployment, Service, Endpoints)
- Should verify application endpoints are responding correctly
- May execute validation scripts (e.g., `tests/validate.sh`)
- Should check both Helm release status and actual pod/application health

---

## Common Dagger Types

### `*dagger.Directory`
Represents a directory in the Dagger pipeline. Can be:
- The source code directory
- A mounted filesystem
- A generated directory from a previous step

### `*dagger.File`
Represents a single file in the Dagger pipeline. Examples:
- OCI tarball from the Build function
- Configuration files
- Build artifacts

### `*dagger.Secret`
Represents a secret value that should be handled securely. Common uses:
- Kubeconfig files
- Registry credentials
- API tokens

Secrets are never logged or exposed in plain text.

---

## Parameter Conventions

### Optional Parameters
Parameters marked with `// +optional` can be omitted when calling the function. Implementations should provide sensible defaults:
- `containerRepository`: `ttl.sh`
- `helmRepository`: `oci://ttl.sh`
- `releaseName`: `goserv`
- `namespace`: `goserv`
- `targetHost`: `localhost`
- `targetPort`: `8080`

### Version Handling
All functions that work with versions should:
1. Read the version from a `VERSION` file in the source directory
2. If `releaseCandidate` is `true`, append `-rc` to the version
3. Use this version for tagging, naming, and validation

### Error Handling
All functions return an `error` as the second return value. Errors should be:
- Descriptive (include context about what failed)
- Wrapped with additional context using `fmt.Errorf("context: %w", err)`
- Returned immediately when encountered (fail-fast)

---

## Implementation Guidelines

When implementing these functions:

1. **Maintain Signature Compatibility**: Never change the function signatures without updating all implementations
2. **Use Consistent Defaults**: Apply the same default values across all implementations
3. **Handle Optional Parameters**: Check for empty strings/nil values and apply defaults
4. **Read VERSION File**: Use `source.File("VERSION").Contents(ctx)` to read version
5. **Respect releaseCandidate Flag**: Append `-rc` to versions when this flag is true
6. **Use Dagger Containers**: Leverage `dag.Container()` for containerized operations
7. **Install Required Tools**: Each function should install its own dependencies (kubectl, helm, etc.)
8. **Return Meaningful Output**: Return stdout/results that can be logged or used by callers
9. **Enable Debugging**: Include sufficient logging for troubleshooting failures

---

## Example Invocations

### Building for Release Candidate
```bash
dagger call build --source=. --release-candidate=true
```

### Running Tests with Pre-built Image
```bash
dagger call build --source=. | dagger call unit-test --source=. --image-tarball=-
```

### Full Deployment Pipeline
```bash
# Build
IMAGE=$(dagger call build --source=. --release-candidate=false)

# Test
dagger call unit-test --source=. --image-tarball="$IMAGE"
dagger call integration-test --source=. --target-host=staging.example.com

# Deliver
dagger call deliver --source=. --image-tarball="$IMAGE" \
  --container-repository=ghcr.io/myorg \
  --helm-repository=oci://ghcr.io/myorg/charts

# Deploy & Validate
dagger call deploy --source=. \
  --kubeconfig=env:KUBECONFIG \
  --helm-repository=oci://ghcr.io/myorg/charts \
  --namespace=production

dagger call validate --source=. \
  --kubeconfig=env:KUBECONFIG \
  --namespace=production
```

---

## Integration with Shell Scripts

The `cicd-local` directory contains pipeline scripts that demonstrate how to compose these Dagger functions:

### Pipeline Scripts

- **`local_ci_pipeline.sh`**: CI pipeline with two modes:
  - **Commit mode**: `Build → UnitTest` (validates changes)
  - **PR merge mode**: `Build → UnitTest → Deliver` (publishes artifacts)
  - Usage: `./local_ci_pipeline.sh --pipeline-trigger=commit|pr-merge`

- **`local_iat_pipeline.sh`**: Integration Acceptance Testing
  - **Flow**: `Deploy → Validate → IntegrationTest`
  - Deploys to local Kubernetes (Colima) and runs comprehensive tests
  - Always uses release candidate builds
  - Usage: `./local_iat_pipeline.sh`

- **`local_staging_pipeline.sh`**: Blue-Green Deployment Testing
  - **Flow**: Three-phase blue-green testing
  - Phase 1: Deploy current (RC) → Validate
  - Phase 2: Deploy previous → Validate (rollback test)
  - Phase 3: Deploy current (RC) → Validate (re-deployment test)
  - Usage: `./local_staging_pipeline.sh`

- **`local_deliver_pipeline.sh`**: Artifact Delivery
  - **Flow**: `Build → Deliver`
  - Publishes to container and Helm repositories
  - Usage: `./local_deliver_pipeline.sh --release-candidate`

- **`local_deploy_pipeline.sh`**: Production Deployment
  - **Flow**: `Deploy → Validate`
  - Deploys to Kubernetes and validates
  - Usage: `./local_deploy_pipeline.sh --namespace=production`

### Key Patterns from Scripts

**Passing Build Artifacts Between Functions**:
```bash
# Build and export to file
dagger -m cicd call build --source=. \
  --release-candidate=true \
  export --path=./build/goserv-image.tar

# Use tarball in subsequent functions
dagger -m cicd call unit-test --source=. \
  --image-tarball=./build/goserv-image.tar

dagger -m cicd call deliver --source=. \
  --image-tarball=./build/goserv-image.tar \
  --release-candidate=true
```

**Passing Kubeconfig as Secret**:
```bash
dagger -m cicd call deploy --source=. \
  --kubeconfig=file:${HOME}/.kube/config \
  --release-name=goserv \
  --namespace=goserv
```

**Conditional Release Candidate Flag**:
```bash
# For PR merges and staging
--release-candidate=true

# For production and previous releases
--release-candidate=false  # or omit the flag
```

**Integration Testing with Port Forward**:
```bash
# Use host.docker.internal to reach localhost from Dagger container
dagger -m cicd call integration-test --source=. \
  --target-host=host.docker.internal \
  --target-port=8080
```

### When Migrating from Shell Scripts to Dagger Functions

1. Replace `dagger call build` commands with function invocations
2. Pass kubeconfig via secrets: `--kubeconfig=file:~/.kube/config`
3. Chain functions using pipes or exported files (tarballs)
4. Handle boolean flags explicitly (`--release-candidate=true`)
5. Use `host.docker.internal` for integration tests targeting localhost
6. Export build artifacts to files when they need to be reused across multiple function calls

---

## Testing Your Implementation

To verify your implementation matches this contract:

1. **Check Function Signatures**: Ensure parameters and return types match exactly
2. **Test Optional Parameters**: Verify defaults are applied when parameters are omitted
3. **Test with releaseCandidate**: Verify `-rc` suffix is added to versions
4. **Test Error Cases**: Verify meaningful errors are returned
5. **Test Integration**: Verify output from one function can be piped to another
6. **Verify Version Reading**: Confirm VERSION file is read correctly

---

## Notes

- All functions are methods on the `Goserv` struct (receiver `m *Goserv`)
- Import path: `dagger/goserv/internal/dagger`
- Package: `main`
- These signatures support both Helm-based and ArgoCD-based deployment strategies
- Parameters unused by a specific implementation should still be accepted (for compatibility)
