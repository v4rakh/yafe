FROM node:24-alpine AS builder-frontend

RUN apk --update upgrade && \
    apk add pnpm && \
    rm -rf /var/cache/apk/*

WORKDIR /src/web
COPY internal/frontend/app/package*.json ./
RUN pnpm install
COPY internal/frontend/app/ ./
RUN pnpm build

FROM golang:1.26-alpine AS builder-server

# Enable automatic toolchain download for Go 1.25+
ENV GOTOOLCHAIN=auto

WORKDIR /src

# Download dependencies first (better layer caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and frontend build
COPY . .
COPY --from=builder-frontend /src/web/dist ./internal/frontend/app/dist
RUN CGO_ENABLED=0 GOOS=linux go build -tags embed -trimpath -ldflags="-s -w" -o /yafe ./cmd/yafe

FROM debian:trixie-slim

# Runtime dependencies
RUN apt-get update && \
    apt-get install -y --no-install-recommends bash ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Labels
LABEL org.opencontainers.image.title="YaFE" \
    org.opencontainers.image.description="Yet another Flow Engine"  \
    org.opencontainers.image.source="https://git.myservermanager.com/varakh/yafe"

# Copy binary
COPY --from=builder-server /yafe /usr/local/bin/yafe

# Create data directory structure and set up non-root user
# OpenShift compatible: use root group (GID 0) with group write permissions
RUN useradd -u 65532 -g 0 -M -d /data nonroot && \
    mkdir -p /data && \
    chgrp -R 0 /data && \
    chmod -R g=u /data

USER nonroot
WORKDIR /data

# Expose HTTP port
EXPOSE 8080

# Default configuration - see README.md for all environment variables
ENV YAFE_SOCKET_ENABLED=false \
    YAFE_HTTP_ENABLED=true \
    YAFE_HTTP_LISTEN=:8080 \
    YAFE_QUEUE_DIR=/data/queue \
    YAFE_FLOWS_DIR=/data/flows \
    YAFE_SCHEDULES_DIR=/data/schedules

# Default command: run daemon
ENTRYPOINT ["/usr/local/bin/yafe"]
CMD ["serve"]
