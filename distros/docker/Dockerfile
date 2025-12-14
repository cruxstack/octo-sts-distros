# ------------------------------------------------------------- builder: app ---

FROM golang:1.25rc1-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Copy go.mod files for dependency caching
COPY cmd/go.mod cmd/go.sum* ./cmd/
COPY internal/go.mod internal/go.sum ./internal/
COPY octo-sts-app/go.mod octo-sts-app/go.sum ./octo-sts-app/

# Download dependencies
WORKDIR /build/cmd
RUN go mod download

# Copy source code
WORKDIR /build
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY octo-sts-app/ ./octo-sts-app/

# Build the binaries
WORKDIR /build/cmd
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/sts ./http-sts
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/app ./http-app

# ------------------------------------------------------------------ runtime ---

FROM alpine:3.21

RUN apk add --no-cache ca-certificates wget

# create non-root user
RUN addgroup -g 1000 octo-sts && \
    adduser -u 1000 -G octo-sts -s /bin/sh -D octo-sts

COPY --from=builder /out/sts /usr/local/bin/sts
COPY --from=builder /out/app /usr/local/bin/app

USER octo-sts

EXPOSE 8080
