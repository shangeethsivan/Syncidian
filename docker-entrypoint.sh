#!/bin/sh
set -e

# Persist data on a runtime mount, not a Dockerfile VOLUME (Railway forbids it).
if [ -z "${SYNCIDIAN_DATA}" ]; then
  if [ -n "${RAILWAY_VOLUME_MOUNT_PATH}" ]; then
    SYNCIDIAN_DATA="${RAILWAY_VOLUME_MOUNT_PATH}"
  else
    SYNCIDIAN_DATA=/data
  fi
  export SYNCIDIAN_DATA
fi

mkdir -p "${SYNCIDIAN_DATA}"

run_as_app_user() {
  export HOME=/home/syncidian
  exec su-exec syncidian /app/syncidian "$@"
}

if [ "$(id -u)" = "0" ]; then
  chown syncidian:syncidian "${SYNCIDIAN_DATA}" 2>/dev/null || true
  if ! su-exec syncidian sh -c "test -w \"${SYNCIDIAN_DATA}\""; then
    chown -R syncidian:syncidian "${SYNCIDIAN_DATA}" 2>/dev/null || true
  fi
  if su-exec syncidian sh -c "test -w \"${SYNCIDIAN_DATA}\""; then
    run_as_app_user "$@"
  fi
  # Volume not writable by uid 10001 (some hosts remount as root-only).
  export HOME=/root
  exec /app/syncidian "$@"
fi

exec /app/syncidian "$@"
