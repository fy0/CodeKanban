# Build stage for frontend
FROM node:22.12-alpine AS frontend-builder

WORKDIR /app/ui

# Install pnpm using npm
RUN npm install -g pnpm@latest

# Copy frontend package files
COPY ui/package.json ui/pnpm-lock.yaml ./

# Install dependencies
RUN pnpm install --frozen-lockfile

# Copy frontend source code
COPY ui/ ./

# Build frontend
RUN pnpm build

# Build stage for backend
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS backend-builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev sqlite-dev

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Copy built frontend assets
COPY --from=frontend-builder /app/ui/dist ./static

# Build backend with cross-compilation
# Use build platform (amd64) to compile for target platform (amd64/arm64)
ARG TARGETARCH
ARG TARGETOS
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w" \
    -tags "sqlite_omit_load_extension" \
    -trimpath \
    -o CodeKanban

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    sqlite-libs

WORKDIR /app

# Copy binary from builder
COPY --from=backend-builder /app/CodeKanban .

# Create data directory
RUN mkdir -p /app/data

# Expose default port
EXPOSE 3007

# Set environment variables
ENV GIN_MODE=release

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:3007/api/v1/health || exit 1

# Run the application
CMD ["./CodeKanban"]
