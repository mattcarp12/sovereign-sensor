# ─── Stage 1: Build the Go Binary ─────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

WORKDIR /workspace

# Cache dependencies first (this speeds up subsequent builds)
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire source code
# (This is critical because it copies the pre-built React frontend in internal/api/dist!)
COPY . .

# Build the Operator binary targeting cmd/controller/main.go
# Name it 'manager' so it matches the Kubernetes Deployment YAML
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o manager cmd/controller/main.go

# ─── Stage 2: The Minimal Runtime Image ───────────────────────────────────────
FROM alpine:3.19

WORKDIR /

# Copy the compiled manager binary from the builder stage
COPY --from=builder /workspace/manager .

# Run as an unprivileged user to satisfy Kubernetes security contexts
USER 65532:65532

# Execute the manager
ENTRYPOINT ["/manager"]