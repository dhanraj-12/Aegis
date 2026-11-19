# syntax=docker/dockerfile:1

# Stage 1: Build the statically linked Go binary
FROM golang:1.24-alpine AS builder

WORKDIR /src

# Pre-fetch dependencies for Docker cache optimization
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY api/ api/
COPY cmd/ cmd/
COPY pkg/ pkg/

# Compile static binary with optimizations and stripped debug info
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/aegis cmd/proxy/main.go

# Stage 2: Ultra-minimal distroless runtime
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /

# Copy the static binary from builder stage
COPY --from=builder /bin/aegis /aegis

# Copy default config file
COPY config.yaml /etc/aegis/config.yaml

# Expose default HTTP reverse proxy port and UDP gossip port
EXPOSE 8080/tcp
EXPOSE 8081/udp

# Run as nonroot user (UID 65532)
USER nonroot:nonroot

ENTRYPOINT ["/aegis"]
CMD ["-config", "/etc/aegis/config.yaml"]
