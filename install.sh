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
BINARY_NAME="routebox"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/routebox"
SINGBOX_CONFIG_DIR="/etc/amnezia-box"
SERVICE_NAME="routebox"
SETTINGS_FILE="routebox.toml"

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

    # Remove binary
    if [ -f ${INSTALL_DIR}/${BINARY_NAME} ]; then
        echo "Removing ${INSTALL_DIR}/${BINARY_NAME}..."
        rm -f ${INSTALL_DIR}/${BINARY_NAME}
    fi

    echo ""
    echo -e "${GREEN}RouteBox uninstalled.${NC}"
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

# Download binary
echo ""
echo "Downloading RouteBox..."
DOWNLOAD_URL="https://raw.githubusercontent.com/${REPO}/main/releases/${BINARY_NAME}-linux-${ARCH}"

if command -v curl &> /dev/null; then
    curl -fsSL -o /tmp/${BINARY_NAME} "${DOWNLOAD_URL}"
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
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable ${SERVICE_NAME}

    echo ""
    echo -e "${YELLOW}Start the service with:${NC}"
    echo "  sudo systemctl start routebox"
fi

# Get IP address
IP=$(hostname -I | awk '{print $1}')

echo ""
echo -e "${GREEN}╔═══════════════════════════════════════════╗"
echo "║         Installation Complete!            ║"
echo "╚═══════════════════════════════════════════╝${NC}"
echo ""
echo "Binary:   ${INSTALL_DIR}/${BINARY_NAME}"
echo "Settings: ${CONFIG_DIR}/${SETTINGS_FILE}"
echo "Config:   ${SINGBOX_CONFIG_DIR}"
echo ""
echo -e "${GREEN}Next steps:${NC}"
echo ""
echo "1. Start RouteBox:"
echo "   sudo systemctl start routebox"
echo ""
echo "2. Open in browser:"
echo -e "   ${GREEN}http://${IP}:8080${NC}"
echo ""
echo "3. Follow the setup wizard"
echo ""
echo -e "${BLUE}Optional: GeoIP (country flags in connections)${NC}"
echo ""
echo "   Download IPInfo database (free):"
echo "   https://ipinfo.io/developers/free-ip-database"
echo ""
echo "   Place the .mmdb file and update settings:"
echo "   nano ${CONFIG_DIR}/${SETTINGS_FILE}"
echo ""
echo "   Set geoip.path = \"/path/to/ipinfo.mmdb\""
echo "   Then restart: sudo systemctl restart routebox"
echo ""
