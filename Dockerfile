FROM golang:1.22-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /out/syncidian ./cmd/syncidian

FROM alpine:3.20

RUN apk add --no-cache ca-certificates git wget \
    && adduser -D -H -u 10001 syncidian

WORKDIR /app

COPY --from=builder /out/syncidian /app/syncidian

RUN mkdir -p /data && chown -R syncidian:syncidian /data /app

USER syncidian

ENV SYNCIDIAN_DATA=/data
ENV SYNCIDIAN_ADDR=:8080

EXPOSE 8080

VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
  CMD wget -qO- http://127.0.0.1:8080/health || exit 1

ENTRYPOINT ["/app/syncidian"]
