#!/usr/bin/env bash
# Builds the Amnezia schema oracle. Requires Qt6 dev headers and a clone of
# amnezia-vpn/amnezia-client.
#
#   AMNEZIA_CLIENT=/path/to/amnezia-client ./scripts/qtoracle/build.sh
#
# Produces ./scripts/qtoracle/oracle.
set -euo pipefail

: "${AMNEZIA_CLIENT:?set AMNEZIA_CLIENT to an amnezia-client clone}"
SRC="$AMNEZIA_CLIENT/client"
OUT="$(cd "$(dirname "$0")" && pwd)"

# Must be the Qt6 moc: a bare `moc` on PATH is often Qt5, and the generated file
# then refuses to compile against Qt6 headers.
MOC="${MOC:-$(pkg-config --variable=libexecdir Qt6Core)/moc}"
[ -x "$MOC" ] || { echo "Qt6 moc not found at $MOC — set MOC=" >&2; exit 1; }

"$MOC" -I"$SRC" "$SRC/core/utils/protocolEnum.h" -o "$OUT/moc_protocolEnum.cpp"

g++ -fPIC -std=c++17 -I"$SRC" -I"$OUT" \
    "$OUT/oracle.cpp" \
    "$SRC/core/models/protocols/awgProtocolConfig.cpp" \
    "$SRC/core/protocols/protocolUtils.cpp" \
    "$OUT/moc_protocolEnum.cpp" \
    -o "$OUT/oracle" \
    $(pkg-config --cflags --libs Qt6Core)

echo "built $OUT/oracle"
