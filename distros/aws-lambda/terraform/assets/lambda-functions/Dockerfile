# --------------------------------------------------------------------- base ---

FROM golang:1.25-alpine AS base

ARG APP_REPO=https://github.com/octo-sts/app.git
ARG APP_VERSION=latest
ARG DISTRO_REPO=https://github.com/cruxstack/octo-sts-distros.git
ARG DISTRO_VERSION=latest

WORKDIR /build

RUN apk add --no-cache git make zip ca-certificates

# clone the distros repo (contains lambda wrappers)
RUN if [ "${DISTRO_VERSION}" = "latest" ]; then \
      git clone --depth 1 ${DISTRO_REPO} distros; \
    else \
      git clone --depth 1 --branch ${DISTRO_VERSION} ${DISTRO_REPO} distros; \
    fi

# clone the octo-sts/app repo
RUN if [ "${APP_VERSION}" = "latest" ]; then \
      git clone --depth 1 ${APP_REPO} octo-sts-app; \
    else \
      git clone --depth 1 --branch ${APP_VERSION} ${APP_REPO} octo-sts-app; \
    fi

WORKDIR /build/distros/distros/aws-lambda

# Update go.mod to point to the cloned app repo and sync dependencies
RUN go mod edit -replace github.com/octo-sts/app=../../../octo-sts-app \
    && go mod tidy

RUN GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build \
    -ldflags="-s -w" \
    -o /out/bootstrap-sts \
    ./cmd/lambda-sts

RUN GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build \
    -ldflags="-s -w" \
    -o /out/bootstrap-webhook \
    ./cmd/lambda-webhook

# ------------------------------------------------------------- package: sts ---

FROM alpine:latest AS package-sts

COPY --from=base /out/bootstrap-sts /opt/app/dist/bootstrap

RUN apk add --no-cache zip \
    && cd /opt/app/dist \
    && zip -r /tmp/package.zip .

# --------------------------------------------------------- package: webhook ---

FROM alpine:latest AS package-webhook

COPY --from=base /out/bootstrap-webhook /opt/app/dist/bootstrap

RUN apk add --no-cache zip \
    && cd /opt/app/dist \
    && zip -r /tmp/package.zip .
