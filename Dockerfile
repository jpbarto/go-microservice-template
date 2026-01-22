# Build stage
FROM golang:1.21-bookworm AS builder

# Install ca-certificates and git
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    git \
    && rm -rf /var/lib/apt/lists/*

# Copy your local CA bundle that already includes Netskope cert (using wildcard makes it optional)
# This is the same bundle your local Go uses
COPY .ca-bundle.pe[m] /etc/ssl/certs/ca-bundle.pem

# Verify the bundle was copied and set environment to use it
RUN ls -lh /etc/ssl/certs/ca-bundle.pem && \
    echo "CA bundle size:" && wc -l /etc/ssl/certs/ca-bundle.pem

# Set environment to use the CA bundle
ENV SSL_CERT_FILE=/etc/ssl/certs/ca-bundle.pem \
    GIT_SSL_CAINFO=/etc/ssl/certs/ca-bundle.pem

WORKDIR /app

# Copy go mod files
COPY src/go.mod src/go.sum* ./

# Download dependencies - explicitly set the cert file inline to ensure it's used
RUN SSL_CERT_FILE=/etc/ssl/certs/ca-bundle.pem go mod download

# Copy source code
COPY src/ .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o goserv .

# Runtime stage
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/goserv .

# Expose port
EXPOSE 8080

# Run the application
CMD ["./goserv"]
