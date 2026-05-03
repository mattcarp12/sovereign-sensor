# Build the agent binary
FROM golang:1.25-alpine AS builder
WORKDIR /workspace
# Copy Modules
COPY go.mod go.sum ./
RUN go mod download
# Copy Code
COPY cmd/agent/ cmd/agent/
COPY internal/ internal/
COPY api/ api/
# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o agent cmd/agent/main.go

# Package the agent
FROM alpine:3.19
WORKDIR /
COPY --from=builder /workspace/agent .
# Ensure the MaxMind DB gets copied into the image
# COPY pkg/geo/GeoLite2-Country.mmdb /pkg/geo/GeoLite2-Country.mmdb
USER root
ENTRYPOINT ["/agent"]