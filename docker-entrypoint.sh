#!/bin/sh
set -e

PUID="${PUID:-1000}"
PGID="${PGID:-1000}"

if [ "$(id -u)" = "0" ]; then
    if ! getent group "$PGID" > /dev/null 2>&1; then
        addgroup -g "$PGID" filebrowser
    fi
    if ! getent passwd "$PUID" > /dev/null 2>&1; then
        adduser -u "$PUID" -G "$(getent group "$PGID" | cut -d: -f1)" \
            -D -H -s /sbin/nologin filebrowser
    fi

    chown "$PUID:$PGID" /data /cache /db /app 2>/dev/null || true

    exec su-exec "$PUID:$PGID" filebrowser "$@"
fi

exec filebrowser "$@"
