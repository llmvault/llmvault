# Hivy app sandbox image: minimal Debian + hivy-appd. None of the agent
# image's node/Chromium/ffmpeg bulk — app sandboxes only run the deployed
# app binary plus the appd control daemon.
#
# Process model (mirrors Dockerfile.runtime) — the image supports two boot
# modes and must keep working in both:
#   - tini mode (flag off / providers that run the image ENTRYPOINT): tini +
#     an entrypoint script run hivy-appd directly; hivy-appd supervises the
#     app process itself (its "direct" process manager).
#   - systemd-as-PID1 mode (docker provider with HIVY_SANDBOX_DOCKER_SYSTEMD,
#     and microsandbox VMs): the container/VM boots /sbin/init, the enabled
#     hivy-appd.service starts appd (ExecStartPre covers the entrypoint's dir
#     setup), the hivy-sandbox-env-generator forwards the container env to
#     units, and hivy-appd detects the booted systemd and switches to
#     systemctl-driven control of hivy-app.service.
#
# Build context is assembled by build_image.sh (go.mod/go.sum + the hivy-appd
# source tree + these docker assets).

FROM golang:1.25-trixie AS build

ARG VERSION=dev
ARG COMMIT=unknown

WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/hivy-appd ./cmd/hivy-appd

FROM debian:13.4

ENV DEBIAN_FRONTEND=noninteractive
ENV container=microsandbox
ARG VERSION=dev
ARG COMMIT=unknown

LABEL org.opencontainers.image.source="https://github.com/usehivy/hivy"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.revision="${COMMIT}"

RUN apt-get update \
  && apt-get install -y --no-install-recommends \
    ca-certificates \
    dbus \
    systemd \
    systemd-sysv \
    tini \
  && systemctl mask \
    dev-hugepages.mount \
    sys-fs-fuse-connections.mount \
    systemd-hwdb-update.service \
    systemd-journal-flush.service \
    systemd-modules-load.service \
    systemd-remount-fs.service \
    systemd-sysctl.service \
    systemd-tmpfiles-setup-dev.service \
    systemd-udev-trigger.service \
    systemd-udevd.service \
  && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/hivy-appd /usr/local/bin/hivy-appd
COPY docker/app/entrypoint /usr/local/bin/hivy-app-entrypoint
COPY docker/app/hivy-appd.service /etc/systemd/system/hivy-appd.service
COPY docker/app/hivy-app.service /etc/systemd/system/hivy-app.service
COPY docker/app/hivy-sandbox-env-generator /usr/lib/systemd/system-generators/hivy-sandbox-env-generator

RUN chmod +x /usr/local/bin/hivy-appd \
  && chmod +x /usr/local/bin/hivy-app-entrypoint \
  && chmod +x /usr/lib/systemd/system-generators/hivy-sandbox-env-generator \
  && mkdir -p /workspace/app/releases /workspace/app/tmp /workspace/logs /var/run \
  && systemctl enable hivy-appd.service \
  && systemctl enable hivy-app.service

ENV HOME=/workspace
ENV HIVY_APPD_PORT=7080
ENV APP_ROOT=/workspace/app
ENV LOGS_DIR=/workspace/logs

# 7080: hivy-appd control port (same reserved slot as the agent runtime).
# 8080: the app process itself.
EXPOSE 7080 8080

STOPSIGNAL SIGRTMIN+4
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/hivy-app-entrypoint", "/usr/local/bin/hivy-appd"]
