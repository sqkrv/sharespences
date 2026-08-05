# syntax=docker/dockerfile:1

# --- SPA build: vite writes to ../internal/web/dist (see web/vite.config.ts) ---
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN mkdir -p /src/internal/web && npm run build

# --- Admin UI build: vite writes to ../internal/adminweb/dist (ADR-0008) ---
FROM node:22-alpine AS adminweb
WORKDIR /src/admin
COPY admin/package.json admin/package-lock.json ./
RUN npm ci
COPY admin/ ./
RUN mkdir -p /src/internal/adminweb && npm run build

# --- Go build: app binary with the SPA embedded (ADR-0002) plus the admin
# sidecar with its UI (ADR-0008) — one image carries both binaries ---
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY "cmd/" "cmd/"
COPY internal/ internal/
COPY --from=web /src/internal/web/dist internal/web/dist
COPY --from=adminweb /src/internal/adminweb/dist internal/adminweb/dist
# VERSION stamps the build (ADR-0006 CalVer) into GET /api/v1/version and the
# «О приложении» card. Left at "dev" when the build arg is not passed — the
# git tree is not in this stage, so it cannot be derived here:
#   docker build --build-arg VERSION="$(git describe --tags --always --dirty)" .
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/sharespences "./cmd/sharespences" \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/sharespences-admin "./cmd/sharespences-admin"

# --- Runtime ---
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 65532 sharespences \
    && mkdir -p /data/attachments \
    && chown sharespences:sharespences /data/attachments
COPY --from=build /out/sharespences /usr/local/bin/sharespences
COPY --from=build /out/sharespences-admin /usr/local/bin/sharespences-admin
USER sharespences
ENV ATTACHMENTS_DIR=/data/attachments
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/sharespences"]
CMD ["serve"]
