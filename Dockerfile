FROM golang:1.26.2 AS builder-glibc

RUN apt-get update && apt-get install -y build-essential

WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build for glibc-based Linux (Ubuntu/Debian), Windows, and Darwin
RUN for platform in linux/amd64 linux/arm64 windows/amd64 darwin/amd64 darwin/arm64; do \
        os=$(echo $platform | cut -d/ -f1); \
        arch=$(echo $platform | cut -d/ -f2); \
        ext=""; [ "$os" = "windows" ] && ext=".exe"; \
        echo "Building $os/$arch (glibc/standard)..."; \
        # Enable CGO only for native Linux AMD64 to ensure BoringCrypto/FIPS compliance
        cgo=0; [ "$os" = "linux" ] && [ "$arch" = "amd64" ] && cgo=1; \
        CGO_ENABLED=$cgo GOEXPERIMENT=boringcrypto GOOS=$os GOARCH=$arch go build -trimpath -buildvcs=false -ldflags="-buildid=" -o veo3-mcp-$os-$arch$ext .; \
    done

# --- MUSL BUILDER ---
FROM golang:1.26.2-alpine AS builder-musl

RUN apk add --no-cache build-base

WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build for musl-based Linux (Alpine)
RUN echo "Building linux/amd64 (musl/alpine)..." && \
    CGO_ENABLED=1 GOEXPERIMENT=boringcrypto GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags="-buildid=" -o veo3-mcp-linux-amd64-musl .

# --- FINAL STAGE (NAMED 'builder' for Makefile compatibility) ---
FROM debian:bookworm-slim AS builder

WORKDIR /app

# Install CA certificates for HTTPS
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

# Copy ALL binaries from both builders for distribution
RUN mkdir -p /app/dist
COPY --from=builder-glibc /app/veo3-mcp-linux-amd64 /app/dist/
COPY --from=builder-glibc /app/veo3-mcp-linux-arm64 /app/dist/
COPY --from=builder-glibc /app/veo3-mcp-windows-amd64.exe /app/dist/
COPY --from=builder-glibc /app/veo3-mcp-darwin-amd64 /app/dist/
COPY --from=builder-glibc /app/veo3-mcp-darwin-arm64 /app/dist/
COPY --from=builder-musl /app/veo3-mcp-linux-amd64-musl /app/dist/

# Generate checksums inside the final image
RUN cd /app/dist && sha256sum veo3-mcp-* > checksums.txt

# Set the DEFAULT binary for the container
RUN cp /app/dist/veo3-mcp-linux-amd64 /app/veo3-mcp && chmod +x /app/veo3-mcp

# Expose port for SSE
EXPOSE 8080

# Set the entrypoint
ENTRYPOINT ["/app/veo3-mcp"]
