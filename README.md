# goserv

A containerized Go microservice that responds to HTTP requests with service metadata and dependency information, deployed using Helm.

## Features

- **HTTP JSON API**: Responds to GET requests on `/` with:
  - Service name
  - Service version
  - IP address of the running instance
  - Unique UUID for the instance
  - Headers from a configurable dependency service
- **Health Checks**: `/health` and `/ready` endpoints for Kubernetes probes
- **Configurable Dependency**: Call an external service and include its response headers
- **Container Ready**: Multi-stage Docker build for minimal image size
- **Kubernetes Deployment**: Full Helm chart with configurable values
- **CI/CD Pipeline**: Dagger-based CI/CD automation for build, test, and deployment

## Project Structure

```
.
├── src/
│   ├── main.go             # Go application code
│   ├── go.mod              # Go module definition
│   └── go.sum              # Go module checksums
├── cicd/
│   ├── main.go             # Dagger CI/CD module with build, test, and delivery functions
│   ├── dagger.json         # Dagger module configuration
│   ├── go.mod              # Dagger module dependencies
│   └── internal/           # Generated Dagger SDK code
├── hooks/
│   ├── pre-commit          # Git hook to sync VERSION to Helm chart
│   ├── install.sh          # Script to install Git hooks
│   └── README.md           # Documentation for Git hooks
├── tests/
│   └── unit_test.sh        # Automated unit test script for goserv endpoints
├── helm/
│   └── goserv/
│       ├── Chart.yaml      # Helm chart metadata
│       ├── values.yaml     # Default configuration values
│       └── templates/      # Kubernetes resource templates
├── VERSION                 # Single source of truth for version number
├── Dockerfile              # Multi-stage Docker build with version injection
├── .dockerignore           # Docker build exclusions
├── .gitignore              # Git exclusions
└── README.md               # This file
```

## Version Management

This project uses a `VERSION` file as the single source of truth for version numbering. The version flows through:

- **Go application**: Injected at build time via `-ldflags`
- **Docker image**: Passed as `VERSION` build argument
- **Helm chart**: Automatically synced via Git hook
- **Dagger builds**: Reads from VERSION file

### Git Hooks

The `hooks/` directory contains Git hooks that automate version management:

**Installation:**
```bash
./hooks/install.sh
```

**What it does:**
- When you commit a change to the `VERSION` file, the pre-commit hook automatically updates the `appVersion` in `helm/goserv/Chart.yaml` to match
- This ensures version consistency across the Go code, Docker images, and Helm charts without manual updates

See `hooks/README.md` for more details.

## CI/CD Pipeline

This repository uses [Dagger](https://dagger.io) for CI/CD automation. The `cicd/` directory contains a Dagger module written in Go that provides functions for building, testing, and delivering the application.

### Available Dagger Functions

- **`build`**: Builds the Docker image using the Dockerfile, automatically reading version from VERSION file
- **`unit-test`**: Runs the goserv container and executes automated tests against it
- **`deliver`**: Publishes the container image to ttl.sh registry (temporary registry for testing)

### Prerequisites for Dagger

1. **Install Dagger CLI**: Follow [Dagger installation guide](https://docs.dagger.io/install)
2. **Corporate Proxy Setup** (if behind Netskope or similar): 
   - Add certificates to `~/Library/Application Support/dagger/ca-certificates` (macOS) or `/etc/dagger/certs` (Linux)
   - Restart the Dagger engine: `docker restart $(docker ps -q --filter name=dagger-engine)`
   - See [Dagger custom CA documentation](https://docs.dagger.io/reference/configuration/custom-ca/)

### Running Dagger Commands

All Dagger commands must be run from the **repository root** with the `-m cicd` flag:

```bash
# Build the container image (automatically uses version from VERSION file)
dagger -m cicd call build --source=.

# Build with specific version tag
dagger -m cicd call build --source=. --tag="1.2.3"

# Run unit tests (builds and tests the application)
dagger -m cicd call unit-test --source=.

# Deliver to ttl.sh registry (publishes for 1 hour)
dagger -m cicd call deliver --source=.

# Deliver with specific tag
dagger -m cicd call deliver --source=. --tag="v1.0.0"
```

### Exporting Built Images

To save a built image locally:

```bash
# Export to Docker
dagger -m cicd call build --source=. export --path=/tmp/goserv.tar

# Load into Docker
docker load < /tmp/goserv.tar
```

### CI/CD Integration

The Dagger functions can be called from any CI/CD system that has Docker available:

**GitHub Actions example:**
```yaml
- name: Build
  run: dagger -m cicd call build --source=.
  
- name: Test
  run: dagger -m cicd call unit-test --source=.
```

**GitLab CI example:**
```yaml
build:
  script:
    - dagger -m cicd call build --source=.
```

## Prerequisites

- Go 1.21+ (for local development)
- Docker (for building container images)
- Kubernetes cluster (for deployment)
- Helm 3+ (for deployment)
- Dagger CLI (for running CI/CD pipelines locally)

## Local Development

### Run Locally

```bash
# Download dependencies
cd src
go mod download

# Run the application
go run main.go
```

The service will start on port 8080 by default.

### Test the Service

```bash
# Test the main endpoint
curl http://localhost:8080/

# Test health endpoint
curl http://localhost:8080/health

# Test ready endpoint
curl http://localhost:8080/ready
```

### Environment Variables

- `SERVICE_NAME`: Name of the service (default: "goserv")
- `SERVICE_VERSION`: Version of the service (default: "1.0.0")
- `PORT`: Port to listen on (default: "8080")
- `DEPENDENCY_URL`: URL of dependency service to call (optional)

Example with dependency:
```bash
cd src
export DEPENDENCY_URL="https://httpbin.org/headers"
go run main.go
```

## Build Container Image

### Using Dagger (Recommended)

```bash
# Build using Dagger (automatically uses VERSION file)
dagger -m cicd call build --source=.

# Build with specific version
dagger -m cicd call build --source=. --tag="1.2.3"

# Export to local Docker
dagger -m cicd call build --source=. export --path=/tmp/goserv.tar
docker load < /tmp/goserv.tar
```

### Using Docker Directly

```bash
# Build with version from VERSION file
docker build --build-arg VERSION=$(cat VERSION) -t goserv:$(cat VERSION) .

# Build with specific version
docker build --build-arg VERSION=1.0.0 -t goserv:1.0.0 .

# Test the container locally
docker run -p 8080:8080 goserv:latest

# With environment variables
docker run -p 8080:8080 \
  -e SERVICE_NAME="my-service" \
  -e DEPENDENCY_URL="https://httpbin.org/headers" \
  goserv:latest
```

## Running Tests

### Automated Testing with Dagger

```bash
# Run all unit tests (builds container and tests it)
dagger -m cicd call unit-test --source=.
```

This will:
1. Build the goserv container
2. Start it as a service
3. Run the test script (`tests/unit_test.sh`) against the running service
4. Verify all endpoints and responses

### Manual Testing

If you have the application running locally or in Docker:

```bash
# Set test environment variables (optional)
export TEST_HOST=localhost
export TEST_PORT=8080

# Run the test script
./tests/unit_test.sh
```

The test script checks:
- HTTP 200 response from root endpoint
- Valid JSON response format
- Presence of all required fields
- Service name correctness
- UUID format validation
- UUID consistency across requests
- Timestamp format (RFC3339)
- Content-Type header
- 404 for invalid paths

## Deploy to Kubernetes with Helm

### Install the Chart

```bash
# Basic installation
helm install my-service ./helm/goserv

# With custom values
helm install my-service ./helm/goserv \
  --set application.serviceName="my-custom-service" \
  --set application.serviceVersion="1.2.3" \
  --set application.dependencyUrl="http://another-service:80/" \
  --set image.repository="your-registry/goserv" \
  --set image.tag="v1.0.0"
```

### Upgrade the Release

```bash
helm upgrade my-service ./helm/goserv \
  --set application.dependencyUrl="http://new-service:80/"
```

### Uninstall the Release

```bash
helm uninstall my-service
```

## Helm Configuration

Key configuration options in `values.yaml`:

### Application Settings
```yaml
application:
  serviceName: "goserv"      # Name of the service
  serviceVersion: "1.0.0"              # Version of the service
  port: "8080"                         # Application port
  dependencyUrl: ""                    # URL of dependency service (optional)
```

### Image Settings
```yaml
image:
  repository: goserv          # Docker image repository
  pullPolicy: IfNotPresent             # Image pull policy
  tag: "latest"                        # Image tag
```

### Deployment Settings
```yaml
replicaCount: 2                        # Number of pod replicas

resources:
  limits:
    cpu: 200m
    memory: 128Mi
  requests:
    cpu: 100m
    memory: 64Mi
```

### Service Settings
```yaml
service:
  type: ClusterIP                      # Service type (ClusterIP, NodePort, LoadBalancer)
  port: 80                             # Service port
  targetPort: 8080                     # Container port
```

### Enable Ingress
```yaml
ingress:
  enabled: true
  className: nginx
  hosts:
    - host: my-service.example.com
      paths:
        - path: /
          pathType: Prefix
```

### Enable Auto-scaling
```yaml
autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 80
```

## API Response Example

Request:
```bash
curl http://localhost:8080/
```

Response without dependency:
```json
{
  "service_name": "goserv",
  "service_version": "1.0.0",
  "ip_address": "192.168.1.100",
  "instance_uuid": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "timestamp": "2026-01-22T10:30:00Z"
}
```

Response with dependency:
```json
{
  "service_name": "goserv",
  "service_version": "1.0.0",
  "ip_address": "192.168.1.100",
  "instance_uuid": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "dependency_headers": {
    "Content-Type": ["application/json"],
    "Date": ["Wed, 22 Jan 2026 10:30:00 GMT"],
    "Server": ["nginx/1.21.0"]
  },
  "timestamp": "2026-01-22T10:30:00Z"
}
```

## Testing in Kubernetes

After deploying with Helm:

```bash
# Get the service name
kubectl get svc

# Port forward to access locally
kubectl port-forward svc/my-service-goserv 8080:80

# Test the service
curl http://localhost:8080/
```

## Complete Deployment Example

### Using Dagger and Helm

```bash
# 1. Build and test with Dagger
dagger -m cicd call build --source=.
dagger -m cicd call unit-test --source=.

# 2. Deliver to ttl.sh (temporary registry for testing)
dagger -m cicd call deliver --source=. --tag="v1.0.0"

# 3. Deploy with Helm using the published image
helm install my-service ./helm/goserv \
  --set image.repository="ttl.sh/goserv-v1.0.0" \
  --set image.tag="1h" \
  --set application.dependencyUrl="http://httpbin.default.svc.cluster.local/headers"

# 4. Verify deployment
kubectl get pods
kubectl get svc

# 5. Test the service
kubectl port-forward svc/my-service-goserv 8080:80
curl http://localhost:8080/
```

### Using Docker and Helm

```bash
# 1. Build the image
docker build --build-arg VERSION=$(cat VERSION) -t goserv:$(cat VERSION) .

# 2. Tag for your registry
docker tag goserv:$(cat VERSION) your-registry/goserv:$(cat VERSION)

# 3. Push to registry
docker push your-registry/goserv:$(cat VERSION)

# 4. Deploy with Helm
helm install my-service ./helm/goserv \
  --set image.repository="your-registry/goserv" \
  --set image.tag="$(cat VERSION)" \
  --set application.serviceName="my-service" \
  --set application.dependencyUrl="http://httpbin.default.svc.cluster.local/headers"

# 5. Verify deployment
kubectl get pods
kubectl get svc

# 6. Test the service
kubectl port-forward svc/my-service-goserv 8080:80
curl http://localhost:8080/
```

## Quick Start

```bash
# 1. Clone and setup
git clone https://github.com/jpbarto/go-microservice-template.git
cd go-microservice-template
./hooks/install.sh  # Install Git hooks for version management

# 2. Build and test
dagger -m cicd call build --source=.
dagger -m cicd call unit-test --source=.

# 3. Run locally
docker run -p 8080:8080 goserv:latest

# 4. Test
curl http://localhost:8080/
```

## License

MIT
