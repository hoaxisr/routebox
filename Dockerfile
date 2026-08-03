# RouteBox — VPS panel mode, packaged on a LinuxServer.io baseimage
# (https://docs.linuxserver.io/general/containers-101/). Router mode is not
# supported in Docker: it needs to be the LAN's own gateway (TUN interface,
# host networking) which doesn't fit the container model. Use install.sh on
# bare metal for that.
#
# Every build stage runs on the BUILDPLATFORM and cross-compiles: with the
# multi-arch release building linux/arm64 on amd64 runners, emulating the Go
# and npm compiles under QEMU costs tens of minutes for a binary Go can
# cross-compile in one (CGO_ENABLED=0 throughout — sqlite is modernc, pure Go).

# ---- frontend (arch-independent output: static assets embedded below) ----
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend-builder
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ---- routebox binary ----
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS go-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY backend/ backend/
COPY --from=frontend-builder /src/frontend/build/ backend/internal/embedded/dist/
ARG ROUTEBOX_VERSION=dev
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags "-s -w -X main.Version=${ROUTEBOX_VERSION}" \
    -o /out/routebox ./backend/cmd/routebox

# ---- amnezia-box binary + GeoIP database, fetched for the target arch ----
FROM --platform=$BUILDPLATFORM alpine:3.20 AS fetcher
RUN apk add --no-cache curl jq
ARG TARGETARCH
# Empty = whatever is latest at build time. Either way the resolved tag is
# written next to the binary, so the image says which amnezia-box it shipped
# and cont-init can compare it against the one on the volume.
ARG AMNEZIA_BOX_VERSION=
RUN set -e; \
    case "$TARGETARCH" in \
        amd64) suffix="linux-amd64" ;; \
        arm64) suffix="aarch64-3.10" ;; \
        *) echo "unsupported arch: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    if [ -n "$AMNEZIA_BOX_VERSION" ]; then ref="tags/$AMNEZIA_BOX_VERSION"; else ref="latest"; fi; \
    rel=$(curl -fsSL -H "Accept: application/vnd.github+json" \
        "https://api.github.com/repos/hoaxisr/amnezia-box/releases/$ref"); \
    url=$(echo "$rel" | jq -r --arg suffix "$suffix" \
        '[.assets[] | select(.name | endswith($suffix))][0].browser_download_url'); \
    sha_url=$(echo "$rel" | jq -r --arg suffix "$suffix.sha256" \
        '[.assets[] | select(.name | endswith($suffix))][0].browser_download_url'); \
    if [ -z "$url" ] || [ "$url" = "null" ]; then echo "no amnezia-box asset matching *$suffix" >&2; exit 1; fi; \
    # The bare-metal installer verifies the published checksum before it runs
    # the binary; an image that skipped it would be the weaker of the two paths
    # to the same executable. No sidecar => refuse to build, don't fall back.
    if [ -z "$sha_url" ] || [ "$sha_url" = "null" ]; then echo "no checksum published for *$suffix" >&2; exit 1; fi; \
    mkdir -p /out; \
    curl -fsSL -o /out/amnezia-box "$url"; \
    curl -fsSL -o /tmp/amnezia-box.sha256 "$sha_url"; \
    # Sidecar is "<sha256>  <asset name>"; re-point it at the local filename.
    echo "$(cut -d' ' -f1 /tmp/amnezia-box.sha256)  /out/amnezia-box" | sha256sum -c -; \
    chmod +x /out/amnezia-box; \
    echo "$rel" | jq -r .tag_name > /out/amnezia-box.version
# Same GeoIP DB the installer uses on bare metal — see README's GeoIP section.
# Published without a checksum, so this one is TLS-trust only.
RUN curl -fsSL -o /out/geoip.mmdb \
    https://github.com/iplocate/ip-address-databases/raw/main/ip-to-country/ip-to-country.mmdb

# ---- amneziawg-tools (awg, awg-quick) for the kernel AmneziaWG backend ----
# Alpine does not package these, so they come from upstream. Built for the
# TARGET arch rather than cross-compiled: awg is a small C program and the
# emulated build costs seconds, where a cross toolchain would cost a stage.
# The tools are inert unless the operator selects the kernel backend.
FROM alpine:3.20 AS awg-tools
ARG AMNEZIAWG_TOOLS_VERSION=v3.0.20260730
RUN apk add --no-cache build-base linux-headers curl
RUN set -e; \
    curl -fsSL -o /tmp/tools.tar.gz \
        "https://github.com/amnezia-vpn/amneziawg-tools/archive/refs/tags/${AMNEZIAWG_TOOLS_VERSION}.tar.gz"; \
    mkdir -p /src; tar xzf /tmp/tools.tar.gz -C /src --strip-components=1; \
    make -C /src/src -j"$(nproc)"; \
    # The install target is what renames wg -> awg and wg-quick -> awg-quick,
    # and awg-quick calls `awg` by that name, so the rename is not cosmetic.
    make -C /src/src install DESTDIR=/out PREFIX=/usr \
        WITH_BASHCOMPLETION=no WITH_SYSTEMDUNITS=no WITH_WGQUICK=yes; \
    # awg-quick refuses to run unless it is uid 0 and re-execs itself through
    # sudo. That check predates capabilities: the kernel asks for CAP_NET_ADMIN,
    # not for root, and this image grants exactly that to the panel (see the s6
    # run script). Neutralising the check removes an assumption, not a
    # safeguard — every operation below it is still enforced by the kernel.
    # The replacement is a no-op `:`, not `return 0`: at top level of an
    # executed script bash prints "can only return from a function" on every
    # run, and if upstream ever moves the check into a function, `return`
    # would silently short-circuit that function instead of just this check.
    sed -i "/exec sudo/s|.*|\t: # patched: RouteBox runs this with CAP_NET_ADMIN instead of uid 0|" /out/usr/bin/awg-quick; \
    grep -q "CAP_NET_ADMIN instead of uid 0" /out/usr/bin/awg-quick; \
    if grep -q "exec sudo" /out/usr/bin/awg-quick; then \
        echo "awg-quick root-check patch did not apply" >&2; exit 1; \
    fi; \
    /out/usr/bin/awg --version

# ---- runtime ----
FROM ghcr.io/linuxserver/baseimage-alpine:3.20

LABEL org.opencontainers.image.source="https://github.com/hoaxisr/routebox" \
      org.opencontainers.image.title="routebox" \
      org.opencontainers.image.description="Web panel for amnezia-box (sing-box fork) — VPS panel mode"

# LSIO_FIRST_PARTY=false: this is a third-party image on an LSP baseimage, not
# an official linuxserver.io one — see root/etc/s6-overlay/s6-rc.d/init-adduser/branding
# (https://docs.linuxserver.io/general/container-branding/).
ENV LSIO_FIRST_PARTY=false \
    ROUTEBOX_RUNTIME=docker

# awg-quick is a bash script that drives ip/iptables; bash is already in the
# base image. Everything here is unused by the default singbox backend.
RUN apk add --no-cache iproute2 iptables libcap
COPY --from=awg-tools /out/usr/bin/awg /usr/bin/awg
COPY --from=awg-tools /out/usr/bin/awg-quick /usr/bin/awg-quick

COPY --from=go-builder /out/routebox /usr/bin/routebox
# amnezia-box ships as a /defaults template, not an installed binary: cont-init
# seeds it onto /config/bin, where the panel's updater can replace it as the
# unprivileged run user and the newer binary survives container recreation.
# Installed into the image it would sit in root-owned /usr/bin, where the
# update's write of <path>.new fails with EACCES.
COPY --from=fetcher /out/amnezia-box /defaults/amnezia-box
COPY --from=fetcher /out/amnezia-box.version /defaults/amnezia-box.version
COPY --from=fetcher /out/geoip.mmdb /defaults/geoip.mmdb
COPY root/ /

# /etc/routebox and /etc/amnezia/amneziawg are hardcoded state paths in the
# binary (panel TLS cert mirror, ACME cache default, AmneziaWG server data);
# symlinking them into /config keeps everything RouteBox persists under the
# single volume LSP images conventionally expose, with no code changes needed.
RUN chmod +x /usr/bin/routebox /usr/bin/awg /usr/bin/awg-quick /defaults/amnezia-box /etc/services.d/routebox/run /etc/cont-init.d/10-routebox-config && \
    mkdir -p /config /etc/amnezia && \
    ln -s /config /etc/routebox && \
    ln -s /config/amneziawg /etc/amnezia/amneziawg

EXPOSE 8443 80
VOLUME /config

# /api/health is unauthenticated and answers only once the panel is serving, so
# it reports the thing worth reporting: whether the container is usable. Tries
# HTTPS first (-k: the cert is issued for the public domain, not 127.0.0.1) and
# falls back to HTTP for a panel running without TLS. The port comes from
# /config/routebox.toml at probe time: LISTEN is consumed only on first boot to
# scaffold that file, which is the source of truth from then on — the operator
# (or the panel) can move the listen address in it and the probe must follow.
# Falls back to LISTEN, then the 8443 default, when the file or key is absent.
HEALTHCHECK --interval=30s --timeout=6s --start-period=30s --retries=3 \
    CMD port=$(sed -n 's/^[[:space:]]*listen[[:space:]]*=[[:space:]]*"[^"]*:\([0-9]*\)".*/\1/p' /config/routebox.toml 2>/dev/null | head -n1); \
        port="${port:-${LISTEN##*:}}"; port="${port:-8443}"; \
        curl -fsk --max-time 5 "https://127.0.0.1:$port/api/health" >/dev/null \
        || curl -fs --max-time 5 "http://127.0.0.1:$port/api/health" >/dev/null
