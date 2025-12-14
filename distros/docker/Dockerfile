# ------------------------------------------------------------- builder: app ---

FROM golang:1.25rc1-alpine AS builder

ARG APP_REPO=https://github.com/octo-sts/app.git
ARG APP_VERSION=latest

RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Copy go.mod files for dependency caching
COPY cmd/go.mod cmd/go.sum* ./cmd/
COPY internal/go.mod internal/go.sum ./internal/

# Clone the octo-sts/app repo
RUN if [ "${APP_VERSION}" = "latest" ]; then \
      git clone --depth 1 ${APP_REPO} octo-sts-app; \
    else \
      git clone --depth 1 --branch ${APP_VERSION} ${APP_REPO} octo-sts-app; \
    fi

# Download dependencies
WORKDIR /build/cmd
RUN go mod edit -replace github.com/octo-sts/app=../octo-sts-app \
    && go mod tidy \
    && go mod download

# Copy source code
WORKDIR /build
COPY cmd/ ./cmd/
COPY internal/ ./internal/

# Re-apply replace directive after copying source
WORKDIR /build/cmd
RUN go mod edit -replace github.com/octo-sts/app=../octo-sts-app

# Build the binaries
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
