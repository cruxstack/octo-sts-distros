# Octo-STS Docker Image
# Builds the STS exchange service, webhook validator, and app installer into a single image

# Build stage for octo-sts-app binaries
FROM golang:1.25rc1-alpine AS builder

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

# Build stage for app-installer
FROM golang:1.25rc1-alpine AS installer-builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Copy go mod files first for better caching
COPY app-installer/go.mod ./
RUN go mod download

# Copy the source code
COPY app-installer/ ./

# Build the app-installer binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/app-installer ./cmd/app-installer

# Runtime stage
FROM alpine:3.21

RUN apk add --no-cache ca-certificates wget

# Create non-root user
RUN addgroup -g 1000 octo-sts && \
    adduser -u 1000 -G octo-sts -s /bin/sh -D octo-sts

# Copy binaries from builders
COPY --from=builder /out/sts /usr/local/bin/sts
COPY --from=builder /out/webhook /usr/local/bin/webhook
COPY --from=installer-builder /out/app-installer /usr/local/bin/app-installer

# Default to non-root user
USER octo-sts

# Default port
EXPOSE 8080

# No default entrypoint - specify command in docker-compose for each service
# This allows flexible selection of which binary to run (sts, webhook, app-installer)
