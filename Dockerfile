# syntax=docker/dockerfile:1

# --- SPA build: vite writes to ../internal/web/dist (see web/vite.config.ts) ---
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN mkdir -p /src/internal/web && npm run build

# --- Go build: single binary with the SPA embedded (ADR-0002) ---
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY --from=web /src/internal/web/dist internal/web/dist
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/sharespences ./cmd/sharespences

# --- Runtime ---
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 65532 sharespences \
    && mkdir -p /data/attachments \
    && chown sharespences:sharespences /data/attachments
COPY --from=build /out/sharespences /usr/local/bin/sharespences
USER sharespences
ENV ATTACHMENTS_DIR=/data/attachments
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/sharespences"]
CMD ["serve"]
