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

## Project Structure

```
.
├── src/
│   ├── main.go             # Go application code
│   ├── go.mod              # Go module definition
│   └── go.sum              # Go module checksums
├── Dockerfile              # Multi-stage Docker build
├── .dockerignore           # Docker build exclusions
├── .gitignore              # Git exclusions
└── helm/
    └── goserv/
        ├── Chart.yaml      # Helm chart metadata
        ├── values.yaml     # Default configuration values
        └── templates/      # Kubernetes resource templates
            ├── _helpers.tpl
            ├── deployment.yaml
            ├── service.yaml
            ├── serviceaccount.yaml
            ├── ingress.yaml
            └── hpa.yaml
```

## Prerequisites

- Go 1.21+ (for local development)
- Docker (for building container images)
- Kubernetes cluster (for deployment)
- Helm 3+ (for deployment)

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

```bash
# Build the Docker image
docker build -t goserv:latest .

# Test the container locally
docker run -p 8080:8080 goserv:latest

# With environment variables
docker run -p 8080:8080 \
  -e SERVICE_NAME="my-service" \
  -e SERVICE_VERSION="2.0.0" \
  -e DEPENDENCY_URL="https://httpbin.org/headers" \
  goserv:latest
```

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

```bash
# 1. Build the image
docker build -t goserv:v1.0.0 .

# 2. Tag for your registry (if using a remote registry)
docker tag goserv:v1.0.0 your-registry/goserv:v1.0.0

# 3. Push to registry
docker push your-registry/goserv:v1.0.0

# 4. Deploy with Helm
helm install my-service ./helm/goserv \
  --set image.repository="your-registry/goserv" \
  --set image.tag="v1.0.0" \
  --set application.serviceName="my-service" \
  --set application.dependencyUrl="http://httpbin.default.svc.cluster.local/headers"

# 5. Verify deployment
kubectl get pods
kubectl get svc

# 6. Test the service
kubectl port-forward svc/my-service-goserv 8080:80
curl http://localhost:8080/
```

## License

MIT
