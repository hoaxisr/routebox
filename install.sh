#!/bin/bash
#
# RouteBox Installer
#
# Install:   curl -fsSL https://raw.githubusercontent.com/hoaxisr/routebox/main/install.sh | sudo bash
# Uninstall: curl -fsSL https://raw.githubusercontent.com/hoaxisr/routebox/main/install.sh | sudo bash -s -- --uninstall
#

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

REPO="hoaxisr/routebox"
AMNEZIABOX_REPO="hoaxisr/amnezia-box"
BINARY_NAME="routebox"
AMNEZIABOX_BINARY="amnezia-box"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/routebox"
SINGBOX_CONFIG_DIR="/etc/amnezia-box"
SERVICE_NAME="routebox"
AMNEZIABOX_SERVICE="amnezia-box"
SETTINGS_FILE="routebox.toml"
GEOIP_FILE="geoip.mmdb"
GEOIP_URL="https://github.com/iplocate/ip-address-databases/raw/main/ip-to-country/ip-to-country.mmdb"

# ============================================
# Version helper functions
# ============================================

# Get version of installed amnezia-box or sing-box binary
get_installed_version() {
    local binary=""
    if [ -f "${INSTALL_DIR}/${AMNEZIABOX_BINARY}" ]; then
        binary="${INSTALL_DIR}/${AMNEZIABOX_BINARY}"
    elif [ -f "${INSTALL_DIR}/sing-box" ]; then
        binary="${INSTALL_DIR}/sing-box"
    fi
    if [ -n "$binary" ]; then
        "$binary" version 2>/dev/null | head -1 | grep -oP '\d+\.\d+\.\d+\S*'
    fi
}

# Fetch latest release JSON from GitHub (cached in AMNEZIABOX_RELEASE_JSON)
fetch_release_json() {
    if [ -z "${AMNEZIABOX_RELEASE_JSON:-}" ]; then
        local api_url="https://api.github.com/repos/${AMNEZIABOX_REPO}/releases/latest"
        if command -v curl &> /dev/null; then
            AMNEZIABOX_RELEASE_JSON=$(curl -fsSL "$api_url" 2>/dev/null || true)
        elif command -v wget &> /dev/null; then
            AMNEZIABOX_RELEASE_JSON=$(wget -qO- "$api_url" 2>/dev/null || true)
        fi
    fi
}

# Get latest version from GitHub releases (strips v prefix)
get_latest_version() {
    fetch_release_json
    local tag=""
    # GitHub pretty-prints its JSON: "tag_name": "1.14.0-..." — the space is mandatory here
    tag=$(echo "$AMNEZIABOX_RELEASE_JSON" | grep -o '"tag_name":[[:space:]]*"[^"]*"' | cut -d'"' -f4)
    echo "${tag#v}"
}

# Asset URL of the amnezia-box binary for arch $1 (amd64|arm64), empty if absent.
# The fork ships aarch64 builds as singbox-<ver>-aarch64-<kernel>, not linux-arm64.
# ponytail: head -1 picks the first aarch64 asset; if the fork ever ships several
# kernel variants, match the wanted one here.
amneziabox_asset_url() {
    local re="linux-$1"
    if [ "$1" = "arm64" ]; then re="linux-arm64|aarch64"; fi
    echo "$AMNEZIABOX_RELEASE_JSON" | grep -oE "https://[^\"]*(${re})[^\"]*" | grep -v '\.sha256$' | head -1 || true
}

# Returns true if $1 < $2 (version comparison via sort -V)
version_lt() {
    [ "$1" != "$2" ] && [ "$(printf '%s\n%s' "$1" "$2" | sort -V | head -1)" = "$1" ]
}

# Find which service (amnezia-box or sing-box) is currently running
get_running_service() {
    for svc in "${AMNEZIABOX_SERVICE}" "sing-box"; do
        if systemctl is-active --quiet "$svc" 2>/dev/null; then
            echo "$svc"
            return
        fi
    done
}

# Check root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Error: Please run as root (sudo)${NC}"
    exit 1
fi

# ============================================
# Uninstall function
# ============================================
do_uninstall() {
    echo -e "${YELLOW}"
    echo "╔═══════════════════════════════════════════╗"
    echo "║         RouteBox Uninstaller              ║"
    echo "╚═══════════════════════════════════════════╝"
    echo -e "${NC}"

    # Stop and disable service
    if command -v systemctl &> /dev/null; then
        if systemctl is-active --quiet ${SERVICE_NAME} 2>/dev/null; then
            echo "Stopping ${SERVICE_NAME} service..."
            systemctl stop ${SERVICE_NAME}
        fi
        if systemctl is-enabled --quiet ${SERVICE_NAME} 2>/dev/null; then
            echo "Disabling ${SERVICE_NAME} service..."
            systemctl disable ${SERVICE_NAME}
        fi
        if [ -f /etc/systemd/system/${SERVICE_NAME}.service ]; then
            echo "Removing systemd service file..."
            rm -f /etc/systemd/system/${SERVICE_NAME}.service
            systemctl daemon-reload
        fi
    fi

    # Stop and disable amnezia-box service
    if command -v systemctl &> /dev/null; then
        if systemctl is-active --quiet ${AMNEZIABOX_SERVICE} 2>/dev/null; then
            echo "Stopping ${AMNEZIABOX_SERVICE} service..."
            systemctl stop ${AMNEZIABOX_SERVICE}
        fi
        if systemctl is-enabled --quiet ${AMNEZIABOX_SERVICE} 2>/dev/null; then
            echo "Disabling ${AMNEZIABOX_SERVICE} service..."
            systemctl disable ${AMNEZIABOX_SERVICE}
        fi
        if [ -f /etc/systemd/system/${AMNEZIABOX_SERVICE}.service ]; then
            echo "Removing ${AMNEZIABOX_SERVICE} systemd service file..."
            rm -f /etc/systemd/system/${AMNEZIABOX_SERVICE}.service
            systemctl daemon-reload
        fi
    fi

    # Remove binaries
    if [ -f ${INSTALL_DIR}/${BINARY_NAME} ]; then
        echo "Removing ${INSTALL_DIR}/${BINARY_NAME}..."
        rm -f ${INSTALL_DIR}/${BINARY_NAME}
    fi
    if [ -f ${INSTALL_DIR}/${AMNEZIABOX_BINARY} ]; then
        echo "Removing ${INSTALL_DIR}/${AMNEZIABOX_BINARY}..."
        rm -f ${INSTALL_DIR}/${AMNEZIABOX_BINARY}
    fi

    echo ""
    echo -e "${GREEN}RouteBox and amnezia-box uninstalled.${NC}"
    echo ""
    echo -e "${YELLOW}Note: Config directories were kept:${NC}"
    echo "      ${CONFIG_DIR}"
    echo "      ${SINGBOX_CONFIG_DIR}"
    echo ""
    echo "      Remove manually if no longer needed:"
    echo "      sudo rm -rf ${CONFIG_DIR} ${SINGBOX_CONFIG_DIR}"
    echo ""
    exit 0
}

# Parse arguments
case "${1:-}" in
    --uninstall|-u|uninstall|remove)
        do_uninstall
        ;;
esac

# ============================================
# Install
# ============================================
echo -e "${GREEN}"
echo "╔═══════════════════════════════════════════╗"
echo "║         RouteBox Installer                ║"
echo "╚═══════════════════════════════════════════╝"
echo -e "${NC}"

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64)
        ARCH="amd64"
        ;;
    aarch64)
        ARCH="arm64"
        ;;
    *)
        echo -e "${RED}Error: Unsupported architecture: $ARCH${NC}"
        exit 1
        ;;
esac

echo "Detected architecture: $ARCH"

# Check for systemd
if ! command -v systemctl &> /dev/null; then
    echo -e "${YELLOW}Warning: systemd not found. Service will not be installed.${NC}"
    NO_SYSTEMD=1
fi

# Download binary from GitHub Releases (latest)
echo ""
echo "Downloading RouteBox (latest release)..."
DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${BINARY_NAME}-linux-${ARCH}"

if command -v curl &> /dev/null; then
    curl -fsSL -L -o /tmp/${BINARY_NAME} "${DOWNLOAD_URL}"
elif command -v wget &> /dev/null; then
    wget -q -O /tmp/${BINARY_NAME} "${DOWNLOAD_URL}"
else
    echo -e "${RED}Error: curl or wget required${NC}"
    exit 1
fi

# Install binary
echo "Installing to ${INSTALL_DIR}/${BINARY_NAME}..."
mv /tmp/${BINARY_NAME} ${INSTALL_DIR}/${BINARY_NAME}
chmod +x ${INSTALL_DIR}/${BINARY_NAME}

# Check amnezia-box version and install/upgrade if needed
echo ""
INSTALLED_VER=$(get_installed_version)
LATEST_VER=$(get_latest_version)
RUNNING_SVC=$(get_running_service)
SKIP_AMNEZIABOX=false

if [ -n "$INSTALLED_VER" ] && [ -n "$LATEST_VER" ]; then
    if version_lt "$INSTALLED_VER" "$LATEST_VER"; then
        echo -e "Upgrading amnezia-box: ${YELLOW}${INSTALLED_VER}${NC} → ${GREEN}${LATEST_VER}${NC}"
        if [ -n "$RUNNING_SVC" ]; then
            echo "Stopping ${RUNNING_SVC} service..."
            systemctl stop "$RUNNING_SVC"
        fi
    else
        echo -e "${GREEN}amnezia-box is up to date (${INSTALLED_VER})${NC}"
        SKIP_AMNEZIABOX=true
    fi
elif [ -z "$INSTALLED_VER" ]; then
    echo "amnezia-box not found, installing..."
else
    echo "Could not determine latest version, skipping amnezia-box update"
    SKIP_AMNEZIABOX=true
fi

if [ "$SKIP_AMNEZIABOX" = false ]; then
    fetch_release_json
    AMNEZIABOX_URL=$(amneziabox_asset_url "${ARCH}")

    if [ -z "${AMNEZIABOX_URL}" ]; then
        echo -e "${YELLOW}Warning: Could not find amnezia-box release for ${ARCH}${NC}"
        echo -e "${YELLOW}You can install amnezia-box manually to ${INSTALL_DIR}/${AMNEZIABOX_BINARY}${NC}"
    else
        echo "Downloading amnezia-box..."
        if command -v curl &> /dev/null; then
            curl -fsSL -L -o /tmp/${AMNEZIABOX_BINARY} "${AMNEZIABOX_URL}"
        elif command -v wget &> /dev/null; then
            wget -q -O /tmp/${AMNEZIABOX_BINARY} "${AMNEZIABOX_URL}"
        fi

        echo "Installing to ${INSTALL_DIR}/${AMNEZIABOX_BINARY}..."
        mv /tmp/${AMNEZIABOX_BINARY} ${INSTALL_DIR}/${AMNEZIABOX_BINARY}
        chmod +x ${INSTALL_DIR}/${AMNEZIABOX_BINARY}
    fi

    # Restart service if it was running before upgrade
    if [ -n "$RUNNING_SVC" ]; then
        echo "Starting ${AMNEZIABOX_SERVICE} service..."
        systemctl start "${AMNEZIABOX_SERVICE}"
    fi
fi

# Create config directories
mkdir -p ${CONFIG_DIR}
mkdir -p ${SINGBOX_CONFIG_DIR}

# Download and install settings file if not exists
if [ ! -f ${CONFIG_DIR}/${SETTINGS_FILE} ]; then
    echo "Downloading default settings..."
    SETTINGS_URL="https://raw.githubusercontent.com/${REPO}/main/routebox.toml"
    if command -v curl &> /dev/null; then
        curl -fsSL -o ${CONFIG_DIR}/${SETTINGS_FILE} "${SETTINGS_URL}" 2>/dev/null || true
    elif command -v wget &> /dev/null; then
        wget -q -O ${CONFIG_DIR}/${SETTINGS_FILE} "${SETTINGS_URL}" 2>/dev/null || true
    fi

    if [ -f ${CONFIG_DIR}/${SETTINGS_FILE} ]; then
        echo "Settings installed: ${CONFIG_DIR}/${SETTINGS_FILE}"
    else
        echo -e "${YELLOW}Note: Could not download settings template. Using defaults.${NC}"
    fi
else
    echo "Settings file exists, keeping: ${CONFIG_DIR}/${SETTINGS_FILE}"
fi

# Download GeoIP database if not exists
if [ ! -f ${CONFIG_DIR}/${GEOIP_FILE} ]; then
    echo "Downloading GeoIP database..."
    if command -v curl &> /dev/null; then
        curl -fsSL -o ${CONFIG_DIR}/${GEOIP_FILE} "${GEOIP_URL}" 2>/dev/null || true
    elif command -v wget &> /dev/null; then
        wget -q -O ${CONFIG_DIR}/${GEOIP_FILE} "${GEOIP_URL}" 2>/dev/null || true
    fi

    if [ -f ${CONFIG_DIR}/${GEOIP_FILE} ]; then
        echo "GeoIP database installed: ${CONFIG_DIR}/${GEOIP_FILE}"
    else
        echo -e "${YELLOW}Note: Could not download GeoIP database. Flags will not be shown.${NC}"
    fi
else
    echo "GeoIP database exists, keeping: ${CONFIG_DIR}/${GEOIP_FILE}"
fi

# Enable IP forwarding
echo ""
echo "Configuring system..."
if [ "$(sysctl -n net.ipv4.ip_forward)" != "1" ]; then
    echo "Enabling IPv4 forwarding..."
    sysctl -w net.ipv4.ip_forward=1 > /dev/null
    if ! grep -q "net.ipv4.ip_forward=1" /etc/sysctl.conf 2>/dev/null; then
        echo "net.ipv4.ip_forward=1" >> /etc/sysctl.conf
    fi
fi

# Install systemd service
if [ -z "$NO_SYSTEMD" ]; then
    echo "Installing systemd service..."
    cat > /etc/systemd/system/${SERVICE_NAME}.service << EOF
[Unit]
Description=RouteBox - VPN Router Web UI
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/${BINARY_NAME} --settings ${CONFIG_DIR}/${SETTINGS_FILE}
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
EOF

    # Install amnezia-box systemd service (if binary was installed)
    if [ -f ${INSTALL_DIR}/${AMNEZIABOX_BINARY} ]; then
        cat > /etc/systemd/system/${AMNEZIABOX_SERVICE}.service << AEOF
[Unit]
Description=amnezia-box Service
Documentation=https://sing-box.sagernet.org
After=network.target nss-lookup.target

[Service]
Type=simple
WorkingDirectory=${SINGBOX_CONFIG_DIR}
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW CAP_NET_BIND_SERVICE CAP_SYS_PTRACE CAP_DAC_READ_SEARCH
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW CAP_NET_BIND_SERVICE CAP_SYS_PTRACE CAP_DAC_READ_SEARCH
ExecStart=${INSTALL_DIR}/${AMNEZIABOX_BINARY} run -c ${SINGBOX_CONFIG_DIR}/config.json
ExecReload=/bin/kill -HUP \$MAINPID
Restart=on-failure
RestartSec=10s
LimitNOFILE=infinity

[Install]
WantedBy=multi-user.target
AEOF
    fi

    systemctl daemon-reload
    systemctl enable ${SERVICE_NAME}
    # amnezia-box must come back on its own after a reboot (#94). Not --now:
    # there is no config.json until the panel writes one.
    if [ -f /etc/systemd/system/${AMNEZIABOX_SERVICE}.service ]; then
        systemctl enable ${AMNEZIABOX_SERVICE}
    fi

    echo ""
    echo -e "${YELLOW}Start the service with:${NC}"
    echo "  sudo systemctl start routebox"
fi

# Get IP address
IP=$(hostname -I | awk '{print $1}')

echo ""
echo -e "${GREEN}╔═══════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║         Installation Complete!            ║${NC}"
echo -e "${GREEN}╚═══════════════════════════════════════════╝${NC}"
echo ""
echo "RouteBox:    ${INSTALL_DIR}/${BINARY_NAME}"
if [ -f ${INSTALL_DIR}/${AMNEZIABOX_BINARY} ]; then
echo "amnezia-box: ${INSTALL_DIR}/${AMNEZIABOX_BINARY}"
fi
echo "Settings:    ${CONFIG_DIR}/${SETTINGS_FILE}"
echo "GeoIP:       ${CONFIG_DIR}/${GEOIP_FILE}"
echo "Config:      ${SINGBOX_CONFIG_DIR}"
echo ""
echo -e "${GREEN}Next steps:${NC}"
echo ""
echo "1. Start RouteBox:"
echo "   sudo systemctl start routebox"
echo ""
echo "2. Open in browser:"
echo -e "   ${GREEN}http://${IP}:8080${NC}"
echo ""
