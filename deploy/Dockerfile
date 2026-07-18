# CAR server container image.
#
# CAR binds to the homelab's loopback/WireGuard interface, NEVER to the public
# Internet. This image runs the server binary as a non-root service identity.
# Workspace and artifact data LIVE OUTSIDE the container (volume-mounted), so
# the image remains stateless transport.

FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Reproducible build: strip debug info, embed version.
ARG VERSION=dev
RUN CGO_ENABLED=1 GOOS=linux go build \
    -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/car ./cmd/server

FROM alpine:3.20
# sqlite needs libc compat for the CGO build.
RUN apk add --no-cache ca-certificates sqlite-libs && \
    addgroup -S car && adduser -S -G car -h /var/lib/car car

COPY --from=build /out/car /usr/local/bin/car

# Configuration and data are volume-mounted; the image carries no secrets.
VOLUME ["/etc/car", "/var/lib/car"]
USER car
EXPOSE 8080

# Health: process liveness. Readiness: storage reachable (probed by the binary).
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:${CAR_PORT:-8080}/health || exit 1

ENTRYPOINT ["/usr/local/bin/car"]
CMD ["--config", "/etc/car/config.json"]
