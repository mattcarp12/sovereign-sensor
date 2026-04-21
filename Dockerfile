# ─── Stage 1: Build the Go Binary ─────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Cache dependencies first (this speeds up subsequent builds)
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build a statically linked binary (CGO_ENABLED=0 ensures it runs on Alpine/Scratch)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /bin/sovereign-sensor ./cmd/agent/main.go

# ─── Stage 2: The Minimal Runtime Image ───────────────────────────────────────
FROM alpine:3.19

# Install CA certificates in case we ever need to make outbound HTTPS calls
# RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the compiled binary from the builder stage
COPY --from=builder /bin/sovereign-sensor /app/sovereign-sensor

# Run the agent
CMD ["/app/sovereign-sensor"]