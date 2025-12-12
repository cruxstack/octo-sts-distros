# ------------------------------------------------------------- builder: app ---

FROM golang:1.25rc1-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

COPY octo-sts-app/go.mod octo-sts-app/go.sum ./
RUN go mod download

COPY octo-sts-app/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/sts ./cmd/app
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/webhook ./cmd/webhook

# ------------------------------------------------------- builder: installer ---

FROM golang:1.25rc1-alpine AS installer-builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

COPY app-installer/go.mod ./
RUN go mod download

COPY app-installer/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/app-installer ./cmd/app-installer

# ------------------------------------------------------------------ runtime ---

FROM alpine:3.21

RUN apk add --no-cache ca-certificates wget

# create non-root user
RUN addgroup -g 1000 octo-sts && \
    adduser -u 1000 -G octo-sts -s /bin/sh -D octo-sts

COPY --from=builder /out/sts /usr/local/bin/sts
COPY --from=builder /out/webhook /usr/local/bin/webhook
COPY --from=installer-builder /out/app-installer /usr/local/bin/app-installer

USER octo-sts

EXPOSE 8080

