# ---- Build stage ----
FROM golang:1.25-alpine AS build

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /src

# Cache dependency downloads
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .

ARG VERSION=dev
ARG COMMIT=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /hivy ./cmd/server

# ---- Runtime stage ----
FROM alpine:3.21

ARG VERSION=dev
ARG COMMIT=unknown

LABEL org.opencontainers.image.source="https://github.com/usehivy/hivy"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.revision="${COMMIT}"

# su-exec: lightweight privilege-drop tool (replaces gosu; no PAM dependency).
# nginx: proxies :80 → Go API on :8080 and :8081 (MCP).
RUN apk add --no-cache ca-certificates tzdata nginx su-exec \
    # Create a dedicated non-root user for the Go binary.
    # nginx workers already drop to the "nginx" system user (uid 100) via the
    # "user nginx;" directive in /etc/nginx/nginx.conf; the master stays root
    # only long enough to bind port 80.
    && addgroup -S hivy \
    && adduser  -S -G hivy hivy \
    # Redirect nginx logs to the container stdout/stderr so the platform
    # log pipeline (Railway, Docker, k8s) can ingest them without log rotation.
    && ln -sf /dev/stdout /var/log/nginx/access.log \
    && ln -sf /dev/stderr /var/log/nginx/error.log

COPY --from=build /hivy /hivy
COPY --from=build /src/global /global
COPY proxy.nginx.conf /etc/nginx/http.d/default.conf
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 80 8080

# The entrypoint script runs as root (required to start nginx which must bind
# port 80); it then drops the Go binary to the "hivy" user via su-exec.
# See docker/entrypoint.sh for the supervision and SIGTERM-drain logic.
ENTRYPOINT ["/entrypoint.sh"]
