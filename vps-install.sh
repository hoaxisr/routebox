#!/bin/bash
#
# RouteBox VPS Installer — TLS admin panel with embedded ACME (Let's Encrypt),
# coexisting with vless+Reality on :443. No nginx, no certbot. RouteBox issues
# and hot-renews its own cert (HTTP-01 on :80).
#
# Install (staging cert, recommended first run):
#   curl -fsSL https://raw.githubusercontent.com/hoaxisr/routebox/main/vps-install.sh \
#     | sudo bash -s -- --domain panel.example.com --email you@example.com --staging
# Production (only after staging works):
#   curl -fsSL .../vps-install.sh | sudo bash -s -- --domain panel.example.com --email you@example.com
# Interactive:  sudo bash vps-install.sh
# Update:       sudo bash vps-install.sh --update
# Uninstall:    sudo bash vps-install.sh --uninstall [--purge]

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'

REPO="hoaxisr/routebox"
AMNEZIABOX_REPO="hoaxisr/amnezia-box"
BINARY_NAME="routebox"
AMNEZIABOX_BINARY="amnezia-box"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/routebox"
SINGBOX_CONFIG_DIR="/etc/amnezia-box"
ACME_DIR="${CONFIG_DIR}/acme"
SERVICE_NAME="routebox"
AMNEZIABOX_SERVICE="amnezia-box"
SETTINGS_FILE="routebox.toml"
GEOIP_FILE="geoip.mmdb"
GEOIP_URL="https://github.com/iplocate/ip-address-databases/raw/main/ip-to-country/ip-to-country.mmdb"

# Parsed-arg state (defaults).
DOMAIN=""; EMAIL=""; PORT=""; STAGING="false"
ACTION="install"; PURGE="false"

err()  { echo -e "${RED}Error: $*${NC}" >&2; }
info() { echo -e "${GREEN}$*${NC}"; }
warn() { echo -e "${YELLOW}$*${NC}"; }

usage() {
	cat <<EOF
RouteBox VPS Installer

  --domain <host>   Panel domain (must have an A/AAAA record to this server)
  --email <addr>    Let's Encrypt account contact
  --port <n>        Panel HTTPS port (default 8443)
  --staging         Use LE staging directory (testing; drop for production)
  --update          Re-download binaries (keeps config + cert cache)
  --uninstall       Stop/remove services
  --purge           With --uninstall: also delete ${CONFIG_DIR} and ${SINGBOX_CONFIG_DIR}
  --dry-run         Print the routebox.toml that would be written, then exit
  --help            Show this help
EOF
}

# parse_args populates DOMAIN/EMAIL/PORT/STAGING + ACTION/PURGE. Defaults PORT to 8443.
parse_args() {
	# parse_args is a sourced function (test harness loads it via VPS_INSTALL_LIB);
	# on any error it routes to help via ACTION + return, never exit.
	# Value-flags MUST guard that a value follows: a bare trailing "--domain" would
	# make "shift 2" fail with only 1 arg left. Because parse_args runs before
	# `set -e`, that failure is swallowed and the while-loop never advances —
	# an infinite loop as root. The `$# -lt 2` guards below prevent that.
	while [ "$#" -gt 0 ]; do
		case "$1" in
			--domain)
				if [ "$#" -lt 2 ]; then echo "Error: --domain requires a value" >&2; ACTION="help"; return 0; fi
				DOMAIN="$2"; shift 2 ;;
			--email)
				if [ "$#" -lt 2 ]; then echo "Error: --email requires a value" >&2; ACTION="help"; return 0; fi
				EMAIL="$2"; shift 2 ;;
			--port)
				if [ "$#" -lt 2 ]; then echo "Error: --port requires a value" >&2; ACTION="help"; return 0; fi
				PORT="$2"; shift 2 ;;
			--staging)   STAGING="true";  shift ;;
			--update)    ACTION="update"; shift ;;
			--uninstall) ACTION="uninstall"; shift ;;
			--purge)     PURGE="true";    shift ;;
			--dry-run)   ACTION="dry-run"; shift ;;
			-h|--help)   ACTION="help";   shift ;;
			*) echo "Unknown argument: $1" >&2; ACTION="help"; return 0 ;;
		esac
	done
	[ -z "$PORT" ] && PORT="8443"
}

# dns_matches RESOLVED SERVER -> 0 when non-empty and equal.
dns_matches() { [ -n "$1" ] && [ "$1" = "$2" ]; }

# validate_port PORT -> 0 if usable; nonzero + message on stderr otherwise.
# Rejects non-numeric, out-of-range, 443 (vless+Reality collision — security
# load-bearing), and 80 (reserved for the ACME HTTP-01 challenge). Pure logic,
# sourceable under VPS_INSTALL_LIB for testing without root.
validate_port() {
	local port="$1"
	case "$port" in ''|*[!0-9]*) err "--port must be numeric"; return 1 ;; esac
	if [ "$port" -lt 1 ] || [ "$port" -gt 65535 ]; then err "--port must be 1..65535"; return 1; fi
	[ "$port" = "443" ] && { err "--port 443 collides with vless+Reality; pick another (default 8443)"; return 1; }
	[ "$port" = "80" ]  && { err "--port 80 is reserved for the ACME HTTP-01 challenge; pick another"; return 1; }
	return 0
}

# resolve_ip DOMAIN -> first A record (getent, then dig, then host).
resolve_ip() {
	local d="$1" ip=""
	if command -v getent >/dev/null 2>&1; then
		ip=$(getent ahostsv4 "$d" 2>/dev/null | awk 'NR==1{print $1}')
	fi
	if [ -z "$ip" ] && command -v dig >/dev/null 2>&1; then
		ip=$(dig +short A "$d" 2>/dev/null | head -1)
	fi
	if [ -z "$ip" ] && command -v host >/dev/null 2>&1; then
		ip=$(host -t A "$d" 2>/dev/null | awk '/has address/{print $4; exit}')
	fi
	echo "$ip"
}

# server_public_ip -> best-effort public IPv4 of this host.
server_public_ip() {
	local ip=""
	ip=$(curl -fsS4 --max-time 5 https://api.ipify.org 2>/dev/null || true)
	[ -z "$ip" ] && ip=$(hostname -I 2>/dev/null | awk '{print $1}')
	echo "$ip"
}

# gen_toml DOMAIN EMAIL PORT STAGING(true|false) -> prints the routebox.toml body.
gen_toml() {
	local domain="$1" email="$2" port="$3" staging="$4"
	cat <<EOF
[geoip]
path = "${CONFIG_DIR}/${GEOIP_FILE}"
enabled = true

[network]
listen = "0.0.0.0:${port}"
acme_enabled = true
acme_email = "${email}"
acme_staging = ${staging}
acme_cache_dir = "${ACME_DIR}"

[server]
mode = "vps"
public_host = "${domain}"
public_port = ${port}

[singbox]
config_path = "${SINGBOX_CONFIG_DIR}/config.json"
binary_name = "${AMNEZIABOX_BINARY}"
EOF
}

# detect_arch -> amd64|arm64 (exits on unsupported).
detect_arch() {
	case "$(uname -m)" in
		x86_64) echo "amd64" ;;
		aarch64) echo "arm64" ;;
		*) err "unsupported architecture: $(uname -m)"; exit 1 ;;
	esac
}

# download_verified URL DEST — download then verify <URL>.sha256 sidecar if present.
download_verified() {
	local url="$1" dest="$2" sumfile
	curl -fsSL -o "$dest" "$url" || { err "download failed: $url"; exit 1; }
	sumfile="$(mktemp)"
	if curl -fsSL -o "$sumfile" "${url}.sha256" 2>/dev/null && [ -s "$sumfile" ]; then
		local expected actual
		expected="$(awk '{print $1}' "$sumfile")"
		actual="$(sha256sum "$dest" | awk '{print $1}')"
		if [ -n "$expected" ] && [ "$expected" != "$actual" ]; then
			rm -f "$sumfile"
			err "sha256 mismatch for $url (expected $expected, got $actual)"; exit 1
		fi
		info "sha256 verified: $(basename "$dest")"
	else
		warn "no sha256 sidecar for $(basename "$dest") — skipping checksum"
	fi
	rm -f "$sumfile"
}

require_root() {
	if [ "$(id -u)" -ne 0 ]; then err "run as root (sudo)"; exit 1; fi
}

install_binaries() {
	local arch; arch="$(detect_arch)"
	info "Downloading RouteBox (linux-${arch})..."
	download_verified \
		"https://github.com/${REPO}/releases/latest/download/${BINARY_NAME}-linux-${arch}" \
		"/tmp/${BINARY_NAME}"
	install -m 0755 "/tmp/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"; rm -f "/tmp/${BINARY_NAME}"

	local ab_json ab_url
	ab_json="$(curl -fsSL "https://api.github.com/repos/${AMNEZIABOX_REPO}/releases/latest" 2>/dev/null || true)"
	ab_url="$(echo "$ab_json" | grep -o "https://[^\"]*linux-${arch}\"" | tr -d '"' | head -1)"
	if [ -n "$ab_url" ]; then
		info "Downloading amnezia-box..."
		download_verified "$ab_url" "/tmp/${AMNEZIABOX_BINARY}"
		install -m 0755 "/tmp/${AMNEZIABOX_BINARY}" "${INSTALL_DIR}/${AMNEZIABOX_BINARY}"; rm -f "/tmp/${AMNEZIABOX_BINARY}"
	else
		warn "Could not find amnezia-box release for linux-${arch}; install it manually."
	fi
}

install_geoip() {
	mkdir -p "${CONFIG_DIR}"
	if [ ! -f "${CONFIG_DIR}/${GEOIP_FILE}" ]; then
		info "Downloading GeoIP database..."
		curl -fsSL -o "${CONFIG_DIR}/${GEOIP_FILE}" "${GEOIP_URL}" 2>/dev/null \
			|| warn "GeoIP download failed; country flags disabled."
	fi
}

# write_config — idempotent: never overwrite an existing routebox.toml (it holds
# the bootstrap bcrypt auth hash in [security]). Write it only if absent.
write_config() {
	mkdir -p "${CONFIG_DIR}" "${SINGBOX_CONFIG_DIR}"
	install -d -m 0700 "${ACME_DIR}"
	if [ -f "${CONFIG_DIR}/${SETTINGS_FILE}" ]; then
		info "Existing ${SETTINGS_FILE} found — keeping it (auth/credentials preserved)."
		# Keep the existing config (preserving [security] bcrypt hash), but still
		# reconcile ONLY the acme_staging line so a re-run without --staging actually
		# switches to production certs. A targeted sed touches just that one line —
		# the toml format is `acme_staging = true|false` (see gen_toml).
		sed -i "s/^acme_staging = .*/acme_staging = ${STAGING}/" "${CONFIG_DIR}/${SETTINGS_FILE}"
		return
	fi
	info "Writing ${CONFIG_DIR}/${SETTINGS_FILE}..."
	umask 077
	gen_toml "$DOMAIN" "$EMAIL" "$PORT" "$STAGING" > "${CONFIG_DIR}/${SETTINGS_FILE}"
	chmod 0600 "${CONFIG_DIR}/${SETTINGS_FILE}"
}

install_units() {
	info "Installing systemd units..."
	# routebox runs as ROOT: needed to bind :80 for the ACME HTTP-01 challenge and
	# to manage the amnezia-box service via systemctl. (Mirrors the working router
	# unit; no User=/capabilities — root binds low ports fine.)
	#
	# StartLimit*/Restart guard: a failing ACME issuance (DNS not ready or :80
	# blocked) must NOT hammer Let's Encrypt prod in a tight restart loop. RestartSec
	# of 30s plus a 5-restart-per-10-min cap throttles retries toward LE rate limits.
	cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<EOF
[Unit]
Description=RouteBox - VPN Admin Panel (TLS + embedded ACME)
After=network.target
StartLimitIntervalSec=600
StartLimitBurst=5

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/${BINARY_NAME} --settings ${CONFIG_DIR}/${SETTINGS_FILE}
Restart=on-failure
RestartSec=30

[Install]
WantedBy=multi-user.target
EOF

	if [ -f "${INSTALL_DIR}/${AMNEZIABOX_BINARY}" ]; then
		cat > "/etc/systemd/system/${AMNEZIABOX_SERVICE}.service" <<AEOF
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
	systemctl enable --now "${SERVICE_NAME}"
	if [ -f "/etc/systemd/system/${AMNEZIABOX_SERVICE}.service" ]; then
		# amnezia-box has no config.json yet — the panel writes it post-install
		# (mirrors install.sh). `|| true` tolerates this expected first-start failure.
		systemctl enable --now "${AMNEZIABOX_SERVICE}" || true
	fi
}

configure_firewall() {
	# Open ONLY the panel port + :80. NEVER touch 443 (vless) or 22 (ssh).
	# :80 must stay permanently open: renewals re-run HTTP-01.
	if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q "Status: active"; then
		info "ufw active — allowing ${PORT}/tcp and 80/tcp (443/22 untouched)"
		ufw allow "${PORT}/tcp" >/dev/null || true
		ufw allow 80/tcp >/dev/null || true
	else
		warn "No active ufw — ensure your cloud firewall keeps ${PORT}/tcp AND 80/tcp open"
		warn "(80 is needed for cert renewal). Leave 443/tcp (vless) and 22/tcp (ssh) as they are."
	fi
}

prompt_inputs() {
	if [ -z "$DOMAIN" ]; then printf "Panel domain (A-record must point here): " >/dev/tty; read -r DOMAIN </dev/tty; fi
	if [ -z "$EMAIL" ];  then printf "Let's Encrypt contact email: " >/dev/tty; read -r EMAIL </dev/tty; fi
	[ -n "$DOMAIN" ] || { err "--domain is required"; exit 1; }
	[ -n "$EMAIL" ]  || { err "--email is required"; exit 1; }
}

prechecks() {
	command -v systemctl >/dev/null 2>&1 || { err "systemd is required"; exit 1; }
	command -v curl >/dev/null 2>&1 || { err "curl is required"; exit 1; }
	validate_port "$PORT" || exit 1

	# :80 must be free for HTTP-01 (issuance + every renewal).
	if command -v ss >/dev/null 2>&1 && ss -ltnH 2>/dev/null | awk '{print $4}' | grep -qE '(:|\.)80$'; then
		err ":80 is already in use — the ACME HTTP-01 challenge needs it free. Stop the listener and retry."
		exit 1
	fi

	# DNS precheck: a mismatch means ACME issuance WILL fail (a failed PROD attempt
	# counts toward the 5-certs/domain/week limit). Warn + require confirmation.
	local resolved server
	resolved="$(resolve_ip "$DOMAIN")"
	server="$(server_public_ip)"
	if dns_matches "$resolved" "$server"; then
		info "DNS OK: ${DOMAIN} -> ${resolved}"
	else
		warn "DNS mismatch: ${DOMAIN} -> '${resolved:-<none>}' but this server is '${server:-<unknown>}'."
		warn "ACME issuance will FAIL until the A record points here. A failed PROD attempt counts toward LE limits."
		printf "Continue anyway? [y/N] " >/dev/tty
		local ans; read -r ans </dev/tty || ans=""
		case "$ans" in y|Y) ;; *) err "aborted; fix the A-record first"; exit 1 ;; esac
	fi

	if [ "$STAGING" != "true" ]; then
		warn "PRODUCTION Let's Encrypt selected (no --staging). Limit: 5 certs/domain/week."
		warn "First-time setup should use --staging to validate the flow."
	fi
}

do_install() {
	require_root
	prompt_inputs
	prechecks
	install_binaries
	install_geoip
	write_config
	install_units
	configure_firewall

	echo ""
	info "╔═══════════════════════════════════════════╗"
	info "║         VPS Installation Complete!        ║"
	info "╚═══════════════════════════════════════════╝"
	echo "Panel URL:   https://${DOMAIN}:${PORT}"
	echo "TLS:         embedded ACME ($([ "$STAGING" = "true" ] && echo 'STAGING — untrusted cert' || echo "production Let's Encrypt"))"
	local pwfile="${CONFIG_DIR}/routebox-initial-password"
	if [ -f "$pwfile" ]; then
		echo "Admin password: $(cat "$pwfile")  (username: admin)"
	else
		echo "Admin password: see 'journalctl -u ${SERVICE_NAME}' (generated on first start)"
	fi
	echo ""
	warn "Reminders:"
	warn "  - DNS A-record ${DOMAIN} -> this server must stay valid."
	warn "  - Keep :80 open permanently (cert renewals re-run HTTP-01)."
	warn "  - vless+Reality stays on :443, untouched."
	[ "$STAGING" = "true" ] && warn "  - Staging cert is UNTRUSTED. Re-running this installer WITHOUT --staging flips acme_staging to production (config is otherwise preserved); restart with: systemctl restart ${SERVICE_NAME}"
}

do_update() {
	require_root
	command -v systemctl >/dev/null 2>&1 || { err "systemd required"; exit 1; }
	info "Stopping ${SERVICE_NAME}..."
	systemctl stop "${SERVICE_NAME}" 2>/dev/null || true
	install_binaries   # re-pull verified binaries; config + ACME cache untouched
	systemctl start "${SERVICE_NAME}"
	info "Update complete. Cert/config preserved (autocert renews in-process)."
}

do_uninstall() {
	require_root
	warn "Uninstalling RouteBox (VPS)..."
	for svc in "${SERVICE_NAME}" "${AMNEZIABOX_SERVICE}"; do
		systemctl disable --now "$svc" 2>/dev/null || true
		rm -f "/etc/systemd/system/${svc}.service"
	done
	systemctl daemon-reload
	rm -f "${INSTALL_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${AMNEZIABOX_BINARY}"
	if [ "$PURGE" = "true" ]; then
		warn "--purge: removing ${CONFIG_DIR} (incl. ACME cache) and ${SINGBOX_CONFIG_DIR}"
		rm -rf "${CONFIG_DIR}" "${SINGBOX_CONFIG_DIR}"
	else
		info "Kept configs/cert: ${CONFIG_DIR} (ACME cache in ${ACME_DIR}), ${SINGBOX_CONFIG_DIR}"
		info "Remove with: sudo bash vps-install.sh --uninstall --purge"
	fi
	# NB: firewall rules and 443/22 are intentionally NOT modified on uninstall.
	info "Uninstall complete."
}

do_dry_run() {
	prompt_inputs
	validate_port "$PORT" || exit 1
	echo "# --- routebox.toml that would be written to ${CONFIG_DIR}/${SETTINGS_FILE} ---"
	gen_toml "$DOMAIN" "$EMAIL" "$PORT" "$STAGING"
}

main() {
	parse_args "$@"
	set -euo pipefail
	case "$ACTION" in
		help)      usage; exit 0 ;;
		dry-run)   do_dry_run ;;
		update)    do_update ;;
		uninstall) do_uninstall ;;
		install)   do_install ;;
	esac
}

# Run main only when executed directly; library mode (sourcing for tests) skips it.
if [ "${VPS_INSTALL_LIB:-0}" != "1" ]; then
	main "$@"
fi
