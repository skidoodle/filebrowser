# syntax=docker/dockerfile:1

FROM oven/bun:1-alpine AS frontend
WORKDIR /app/frontend

COPY frontend/package.json frontend/bun.lock ./
RUN bun install --frozen-lockfile

COPY frontend/ ./
RUN bun run build

FROM golang:alpine AS backend
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend /app/frontend/dist ./frontend/dist

ARG VERSION=dev
ARG COMMIT=none
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /bin/filebrowser .

FROM alpine:3.21

ARG VERSION=dev
ARG COMMIT=none

LABEL org.opencontainers.image.title="filebrowser" \
      org.opencontainers.image.description="Web file browser" \
      org.opencontainers.image.url="https://github.com/skidoodle/filebrowser" \
      org.opencontainers.image.source="https://github.com/skidoodle/filebrowser" \
      org.opencontainers.image.licenses="BSD-3-Clause" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}"

RUN apk add --no-cache ca-certificates mailcap tzdata su-exec && \
    mkdir -p /data /cache /app /db && \
    chown -R 1000:1000 /data /cache /app /db

COPY --from=backend /bin/filebrowser /usr/local/bin/filebrowser
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

ENV FILEBROWSER_ROOT=/data \
    FILEBROWSER_CACHEDIR=/cache \
    FILEBROWSER_DATABASE=/db/filebrowser.db

VOLUME ["/data", "/cache", "/db"]

WORKDIR /app

EXPOSE 8080

ENTRYPOINT ["docker-entrypoint.sh"]
