FROM golang:1.22-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /out/syncidian ./cmd/syncidian

FROM alpine:3.20

# Do not add a Docker VOLUME instruction. Railway rejects it ("use Railway
# Volumes"). Persist /data at runtime instead: Railway Volume, Compose, or -v.
RUN apk add --no-cache ca-certificates git wget su-exec \
    && adduser -D -u 10001 syncidian

WORKDIR /app

COPY --from=builder /out/syncidian /app/syncidian
COPY docker-entrypoint.sh /app/docker-entrypoint.sh

RUN mkdir -p /data \
    && chown -R syncidian:syncidian /data /app /home/syncidian \
    && chmod 0755 /app/docker-entrypoint.sh

# Image defaults. Railway injects PORT (and RAILWAY_* vars) at runtime.
# SYNCIDIAN_DATA=/data is the image default; an attached Railway volume
# (RAILWAY_VOLUME_MOUNT_PATH) wins over this path at startup.
ENV PORT=8080
ENV SYNCIDIAN_DATA=/data
ENV HOME=/home/syncidian

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
  CMD /bin/sh -c 'wget -qO- "http://127.0.0.1:${PORT:-8080}/health" || exit 1'

# Start as root so a Railway/host volume mounted on /data can be chowned,
# then drop to the syncidian user. `docker run --user` still works.
ENTRYPOINT ["/app/docker-entrypoint.sh"]
