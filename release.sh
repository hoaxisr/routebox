#!/bin/bash
#
# RouteBox Release Script
#
# Usage: ./release.sh <version> [message]
# Example: ./release.sh v1.0.0 "Initial release"
#          ./release.sh v1.1.0   # auto-generates message
#

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RELEASES_DIR="${SCRIPT_DIR}/releases"

# Check arguments
if [ -z "$1" ]; then
    echo -e "${RED}Error: Version required${NC}"
    echo ""
    echo "Usage: ./release.sh <version> [message]"
    echo ""
    echo "Examples:"
    echo "  ./release.sh v1.0.0"
    echo "  ./release.sh v1.1.0 \"Add new feature\""
    exit 1
fi

VERSION="$1"
MESSAGE="${2:-Release ${VERSION}}"

# Ensure version starts with 'v'
if [[ ! "$VERSION" =~ ^v ]]; then
    VERSION="v${VERSION}"
fi

# Check gh CLI
if ! command -v gh &> /dev/null; then
    echo -e "${RED}Error: gh CLI not installed${NC}"
    echo "Install: https://cli.github.com/"
    exit 1
fi

# Check gh auth
if ! gh auth status &> /dev/null; then
    echo -e "${RED}Error: Not authenticated with GitHub${NC}"
    echo "Run: gh auth login"
    exit 1
fi

# Check binaries exist
AMD64_BINARY="${RELEASES_DIR}/routebox-linux-amd64"
ARM64_BINARY="${RELEASES_DIR}/routebox-linux-arm64"

if [ ! -f "$AMD64_BINARY" ]; then
    echo -e "${RED}Error: ${AMD64_BINARY} not found${NC}"
    exit 1
fi

if [ ! -f "$ARM64_BINARY" ]; then
    echo -e "${YELLOW}Warning: ${ARM64_BINARY} not found, skipping arm64${NC}"
    ARM64_BINARY=""
fi

echo -e "${GREEN}Creating release ${VERSION}${NC}"
echo "Message: ${MESSAGE}"
echo ""

# Check if release already exists
if gh release view "$VERSION" &> /dev/null; then
    echo -e "${YELLOW}Release ${VERSION} already exists. Deleting...${NC}"
    gh release delete "$VERSION" --yes
    git push origin --delete "$VERSION" 2>/dev/null || true
fi

# Create release with assets
echo "Uploading binaries..."

ASSETS="$AMD64_BINARY"
if [ -n "$ARM64_BINARY" ]; then
    ASSETS="$ASSETS $ARM64_BINARY"
fi

gh release create "$VERSION" \
    --title "$VERSION" \
    --notes "$MESSAGE" \
    $ASSETS

echo ""
echo -e "${GREEN}Release ${VERSION} created successfully!${NC}"
echo ""
echo "Download URLs:"
echo "  https://github.com/hoaxisr/routebox/releases/download/${VERSION}/routebox-linux-amd64"
if [ -n "$ARM64_BINARY" ]; then
    echo "  https://github.com/hoaxisr/routebox/releases/download/${VERSION}/routebox-linux-arm64"
fi
echo ""
echo "Latest release URL (used by install.sh):"
echo "  https://github.com/hoaxisr/routebox/releases/latest/download/routebox-linux-amd64"
