# Build stage
FROM golang:1.21-bookworm AS builder

# Install ca-certificates and git
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    git \
    && rm -rf /var/lib/apt/lists/*

# Copy any private certs to the container's trust store and update the CA certificates
COPY certs/*.cr[t] /usr/local/share/ca-certificates/
RUN update-ca-certificates

WORKDIR /app

# Copy go mod files
COPY src/go.mod src/go.sum* ./

# Download dependencies
RUN go mod download

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
