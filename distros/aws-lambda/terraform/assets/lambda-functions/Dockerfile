# --------------------------------------------------------------------- base ---

FROM golang:1.25-alpine AS base

ARG DISTRO_REPO=https://github.com/cruxstack/octo-sts-distros.git
ARG DISTRO_VERSION=latest

WORKDIR /build

RUN apk add --no-cache git make zip ca-certificates

# Clone the distros repo (contains lambda wrappers)
RUN if [ "${DISTRO_VERSION}" = "latest" ]; then \
      git clone --depth 1 ${DISTRO_REPO} distros; \
    else \
      git clone ${DISTRO_REPO} distros; \
      cd distros; \
      git checkout ${DISTRO_VERSION}; \
    fi

WORKDIR /build/distros/cmd

# Sync dependencies (octo-sts/app is fetched as a regular Go module)
RUN go mod edit -replace github.com/cruxstack/octo-sts-distros/internal=/build/distros/internal \
    && go mod tidy

RUN GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build \
    -ldflags="-s -w" \
    -o /out/bootstrap-sts \
    ./lambda-sts

RUN GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build \
    -ldflags="-s -w" \
    -o /out/bootstrap-webhook \
    ./lambda-webhook

# ------------------------------------------------------------- package: sts ---

FROM alpine:3.21 AS package-sts

COPY --from=base /out/bootstrap-sts /opt/app/dist/bootstrap

RUN apk add --no-cache zip \
    && cd /opt/app/dist \
    && zip -r /tmp/package.zip .

# --------------------------------------------------------- package: webhook ---

FROM alpine:3.21 AS package-webhook

COPY --from=base /out/bootstrap-webhook /opt/app/dist/bootstrap

RUN apk add --no-cache zip \
    && cd /opt/app/dist \
    && zip -r /tmp/package.zip .
