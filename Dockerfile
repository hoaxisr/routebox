# RouteBox — VPS panel mode, packaged on a LinuxServer.io baseimage
# (https://docs.linuxserver.io/general/containers-101/). Router mode is not
# supported in Docker: it needs to be the LAN's own gateway (TUN interface,
# host networking) which doesn't fit the container model. Use install.sh on
# bare metal for that.

# ---- frontend ----
FROM node:22-alpine AS frontend-builder
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ---- routebox binary ----
FROM golang:1.25-alpine AS go-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY backend/ backend/
COPY --from=frontend-builder /src/frontend/build/ backend/internal/embedded/dist/
ARG ROUTEBOX_VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.Version=${ROUTEBOX_VERSION}" \
    -o /out/routebox ./backend/cmd/routebox

# ---- amnezia-box binary + GeoIP database, fetched for the target arch ----
FROM alpine:3.20 AS fetcher
RUN apk add --no-cache curl jq
ARG TARGETARCH
RUN set -e; \
    case "$TARGETARCH" in \
        amd64) suffix="linux-amd64" ;; \
        arm64) suffix="aarch64-3.10" ;; \
        *) echo "unsupported arch: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    rel=$(curl -fsSL -H "Accept: application/vnd.github+json" \
        https://api.github.com/repos/hoaxisr/amnezia-box/releases/latest); \
    url=$(echo "$rel" | jq -r --arg suffix "$suffix" \
        '[.assets[] | select(.name | endswith($suffix)) | select(.name | endswith(".sha256") | not)][0].browser_download_url'); \
    if [ -z "$url" ] || [ "$url" = "null" ]; then echo "no amnezia-box asset matching *$suffix" >&2; exit 1; fi; \
    mkdir -p /out; \
    curl -fsSL -o /out/amnezia-box "$url"; \
    chmod +x /out/amnezia-box
# Same GeoIP DB the installer uses on bare metal — see README's GeoIP section.
RUN curl -fsSL -o /out/geoip.mmdb \
    https://github.com/iplocate/ip-address-databases/raw/main/ip-to-country/ip-to-country.mmdb

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

COPY --from=go-builder /out/routebox /usr/bin/routebox
COPY --from=fetcher /out/amnezia-box /usr/bin/amnezia-box
COPY --from=fetcher /out/geoip.mmdb /defaults/geoip.mmdb
COPY root/ /

# /etc/routebox and /etc/amnezia/amneziawg are hardcoded state paths in the
# binary (panel TLS cert mirror, ACME cache default, AmneziaWG server data);
# symlinking them into /config keeps everything RouteBox persists under the
# single volume LSP images conventionally expose, with no code changes needed.
RUN chmod +x /usr/bin/routebox /usr/bin/amnezia-box /etc/services.d/routebox/run /etc/cont-init.d/10-routebox-config && \
    mkdir -p /config /etc/amnezia && \
    ln -s /config /etc/routebox && \
    ln -s /config/amneziawg /etc/amnezia/amneziawg

EXPOSE 8443 80
VOLUME /config
