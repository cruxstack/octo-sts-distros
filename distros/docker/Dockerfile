# Octo-STS Docker Image
# Builds both the STS exchange service and webhook validator into a single image

# Build stage
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Copy go mod files first for better caching
COPY octo-sts-app/go.mod octo-sts-app/go.sum ./
RUN go mod download

# Copy the source code
COPY octo-sts-app/ ./

# Build both binaries with optimizations
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/sts ./cmd/app
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/webhook ./cmd/webhook

# Runtime stage
FROM alpine:3.21

RUN apk add --no-cache ca-certificates wget

# Create non-root user
RUN addgroup -g 1000 octo-sts && \
    adduser -u 1000 -G octo-sts -s /bin/sh -D octo-sts

# Copy binaries from builder
COPY --from=builder /out/sts /usr/local/bin/sts
COPY --from=builder /out/webhook /usr/local/bin/webhook

# Default to non-root user
USER octo-sts

# Default port
EXPOSE 8080

# Default entrypoint - override with command in docker-compose
ENTRYPOINT ["/usr/local/bin/sts"]
