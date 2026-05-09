# Stage 1: Build the React Frontend
FROM node:20-alpine AS frontend-builder
WORKDIR /build
# Copy package files first for layer caching
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build # Now outputs to /build/dist

# Stage 2: Build the Go Manager
FROM golang:1.25 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY api/ api/
COPY internal/ internal/
COPY cmd/controller/ cmd/controller/

# FIX: Move the built frontend into the path Go expects for embedding
# Go's //go:embed dist/* in internal/api/server.go looks for internal/api/dist/
COPY --from=frontend-builder /build/dist ./internal/api/dist

# Build the binary with embedded assets
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -a -o manager cmd/controller/main.go

# Stage 3: Final Production Image
FROM gcr.io/distroless/static:nonroot
WORKDIR /
# Only the binary is needed now; assets are inside it.
COPY --from=builder /workspace/manager .

USER 65532:65532
ENTRYPOINT ["/manager"]