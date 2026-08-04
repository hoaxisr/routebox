#!/bin/bash
#
# RouteBox Docker Installer — интерактивная установка панели в контейнере.
# Спрашивает домен и порты, находит nginx на хосте и встраивается в него,
# либо оставляет TLS самой панели (встроенный ACME).
#
#   curl -fsSL https://raw.githubusercontent.com/hoaxisr/routebox/main/docker-install.sh | sudo bash
#
# Удаление:  sudo bash docker-install.sh --uninstall [--purge]
# Проверка:  sudo bash docker-install.sh --dry-run

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'

IMAGE="ghcr.io/hoaxisr/routebox:latest"
MARKER="# managed by routebox"
NGINX_VHOST_NAME="routebox.conf"
NGINX_STREAM_DIR="/etc/nginx/stream-enabled"
NGINX_STREAM_FILE="${NGINX_STREAM_DIR}/routebox.conf"
CONTAINER_PANEL_PORT="8443"

ACTION="install"; PURGE="false"

# Состояние разведки.
HAS_DOCKER="false"; HAS_NGINX_HOST="false"; HAS_NGINX_CONTAINER="false"
OTHER_PROXY=""; HAS_CERTBOT="false"

# Ответы пользователя.
INSTALL_DIR=""; DOMAIN=""; EMAIL=""; TLS_MODE=""; STAGING="false"
HOST_PORT=""; PUBLIC_PORT=""; SUBNET=""
WANT_SNI="false"; INBOUND_DOMAIN=""; INBOUND_PORT=""; PP_PORT=""; PANEL_HTTPS_PORT=""

err()  { echo -e "${RED}Error: $*${NC}" >&2; }
info() { echo -e "${GREEN}$*${NC}"; }
warn() { echo -e "${YELLOW}$*${NC}"; }

usage() {
	cat <<EOF
RouteBox Docker Installer — интерактивный, флагов установки нет.

  --dry-run     Показать, что будет сделано и какие файлы получатся
  --uninstall   Удалить контейнер и файлы nginx, созданные установщиком
  --purge       Вместе с --uninstall: удалить и каталог ./config с данными
  --help        Эта справка
EOF
}

parse_args() {
	ACTION="install"; PURGE="false"
	while [ $# -gt 0 ]; do
		case "$1" in
			--dry-run)   ACTION="dry-run" ;;
			--uninstall) ACTION="uninstall" ;;
			--purge)     PURGE="true" ;;
			--help|-h)   ACTION="help" ;;
			*) err "неизвестный аргумент: $1"; usage; return 1 ;;
		esac
		shift
	done
	return 0
}

# --- порты -------------------------------------------------------------------

# port_owner PORT -> печатает, кто занял порт; код 0 если занят, 1 если свободен.
# Два источника: ss видит слушающие сокеты, docker ps — опубликованные порты.
# Второй нужен потому, что при userland-proxy: false порт живёт только в
# правилах DNAT и сокета под ним нет — ss его не покажет.
port_owner() {
	local port="$1" line=""
	if command -v ss >/dev/null 2>&1; then
		line=$(ss -ltnpH 2>/dev/null | awk -v p=":${port}\$" '$4 ~ p {print; exit}')
		if [ -n "$line" ]; then
			echo "$line" | sed -n 's/.*users:((\"\([^\"]*\)\".*/\1/p' | head -1
			return 0
		fi
	fi
	if command -v docker >/dev/null 2>&1; then
		# `[^,]*` вместо перечисления символов адреса: в классе GNU grep не
		# понимает экранирование, `[0-9.:\[\]]` там разваливается. Заодно так
		# ловится и IPv6-форма `[::]:8444->`. Записи разделены запятыми,
		# поэтому `[^,]*` не может перескочить на соседний порт.
		if docker ps --format '{{.Ports}}' 2>/dev/null | grep -qE "(^|,| )[^,]*:${port}->"; then
			echo "docker"
			return 0
		fi
	fi
	return 1
}

# pick_free_port START -> первый свободный порт, начиная со START.
pick_free_port() {
	local p="$1"
	while [ "$p" -le 65535 ]; do
		port_owner "$p" >/dev/null || { echo "$p"; return 0; }
		p=$((p + 1))
	done
	err "свободных портов не осталось"
	return 1
}

# --- разбор конфигурации nginx -----------------------------------------------

# nginx_dump -> полная развёрнутая конфигурация nginx.
# Единственная точка чтения: тесты подменяют её через RB_NGINX_T_CMD.
# `|| true` обязателен: потребители читают вывод через `| grep -q`, grep
# закрывает конвейер на первом совпадении, nginx получает SIGPIPE, и под
# pipefail статус 141 превратил бы найденное в ненайденное.
nginx_dump() {
	${RB_NGINX_T_CMD:-nginx -T} 2>/dev/null || true
}

# nginx_config_ok -> 0, если конфигурация nginx вообще читается.
# На сломанном чужом конфиге дамп пустой, и без этой проверки скрипт решил бы,
# что 443 свободен и доменов нет, — и предложил бы nginx-режим вслепую.
nginx_config_ok() {
	[ -n "$(nginx_dump)" ]
}

# nginx_conf_dir -> каталог, куда класть наш vhost.
nginx_conf_dir() {
	if nginx_dump | grep -q 'sites-enabled'; then
		echo "/etc/nginx/sites-available"
	else
		echo "/etc/nginx/conf.d"
	fi
}

# domain_taken DOMAIN -> 0, если домен уже обслуживается чужим блоком.
# Наши собственные блоки помечены маркером и занятыми не считаются: повторный
# запуск установщика должен перезаписывать свой vhost, а не упираться в него.
domain_taken() {
	local domain="$1"
	nginx_dump | awk -v d="$domain" -v marker="$MARKER" '
		/^# configuration file/ { ours = 0 }
		index($0, marker) { ours = 1 }
		/server_name/ && !ours {
			for (i = 2; i <= NF; i++) {
				gsub(/;/, "", $i)
				if ($i == d) { found = 1 }
			}
		}
		END { exit(found ? 0 : 1) }
	'
}

# foreign_443 -> 0, если на 443 слушает хоть один не наш server-блок.
# Пока такой есть, SNI-роутер не предлагается: отдать 443 stream-модулю можно
# было бы только переселив чужие listen, а это ломается молча.
foreign_443() {
	nginx_dump | awk -v marker="$MARKER" '
		/^# configuration file/ { ours = 0 }
		index($0, marker) { ours = 1 }
		/^[[:space:]]*listen[[:space:]]/ && !ours {
			if ($0 ~ /(^|[[:space:]:])443([[:space:]]|;)/) { found = 1 }
		}
		END { exit(found ? 0 : 1) }
	'
}

# stream_module_available -> 0, если stream реально доступен.
# Двух проверок мало по отдельности: на Debian и Ubuntu stream — динамический
# модуль из пакета libnginx-mod-stream, и --with-stream в сборке при
# отсутствующем пакете обманчив.
stream_module_available() {
	nginx -V 2>&1 | grep -q -- '--with-stream' || return 1
	nginx -V 2>&1 | grep -q -- '--with-stream=dynamic' || return 0
	nginx_dump | grep -q 'load_module.*ngx_stream_module'
}

# --- подсеть -----------------------------------------------------------------

# subnet_taken CIDR -> 0, если подсеть уже занята docker-сетью или маршрутом
# хоста. Фиксированная 172.28.0.0/24 без такой проверки роняет compose или,
# хуже, перетягивает на себя чужой маршрут.
subnet_taken() {
	local cidr="$1" nets="" routes=""
	if command -v docker >/dev/null 2>&1; then
		# shellcheck disable=SC2046  # список сетей — именно несколько аргументов
		nets="$(docker network inspect $(docker network ls -q 2>/dev/null) \
			--format '{{range .IPAM.Config}}{{.Subnet}} {{end}}' 2>/dev/null || true)"
		case "$nets" in *"$cidr"*) return 0 ;; esac
	fi
	if command -v ip >/dev/null 2>&1; then
		routes="$(ip -o route 2>/dev/null | awk '{print $1}' || true)"
		case "$routes" in *"$cidr"*) return 0 ;; esac
	fi
	return 1
}

# pick_free_subnet -> первая свободная подсеть из 172.28..172.31, 10.28..10.31.
pick_free_subnet() {
	local c
	for c in 172.28.0.0/24 172.29.0.0/24 172.30.0.0/24 172.31.0.0/24 \
	         10.28.0.0/24 10.29.0.0/24 10.30.0.0/24 10.31.0.0/24; do
		subnet_taken "$c" || { echo "$c"; return 0; }
	done
	err "не нашлось свободной подсети — задайте сеть в docker-compose.yml вручную"
	return 1
}

# gateway_of CIDR -> адрес шлюза (первый адрес подсети).
gateway_of() {
	echo "${1%.*/*}.1"
}

# --- генераторы --------------------------------------------------------------

# gen_compose MODE DOMAIN EMAIL HOST_PORT PUBLIC_PORT SUBNET STAGING INBOUND_PORT
# MODE: standalone (TLS держит RouteBox) | nginx (TLS держит nginx на хосте) |
# proxy (TLS держит чужой прокси — Caddy, Traefik, nginx в контейнере; compose
# такой же, как для nginx, но конфигурацию прокси скрипт только печатает).
# PUBLIC_PORT задаётся всегда: контейнер по умолчанию приравнивает его к
# внутреннему порту прослушивания, и без явного значения подписочные ссылки
# уедут на порт, которого снаружи нет.
gen_compose() {
	local mode="$1" domain="$2" email="$3" host_port="$4" public_port="$5"
	local subnet="$6" staging="$7" inbound_port="$8"
	local gw; gw="$(gateway_of "$subnet")"

	echo "# Создано docker-install.sh. Переменные окружения читаются один раз,"
	echo "# когда контейнер создаёт /config/routebox.toml; дальше правьте файл."
	echo "services:"
	echo "  routebox:"
	echo "    image: ${IMAGE}"
	echo "    container_name: routebox"
	echo "    restart: unless-stopped"
	echo "    environment:"
	echo "      - PUID=1000"
	echo "      - PGID=1000"
	echo "      - TZ=Etc/UTC"
	echo "      - PUBLIC_HOST=${domain}"
	echo "      - PUBLIC_PORT=${public_port}"
	case "$mode" in
		nginx|proxy)
			echo "      - ACME_ENABLED=false"
			echo "      - TRUSTED_PROXIES=${gw}/32"
			;;
		*)
			echo "      - ACME_EMAIL=${email}"
			if [ "$staging" = "true" ]; then echo "      - ACME_STAGING=true"; fi
			;;
	esac
	echo "    ports:"
	case "$mode" in
		nginx|proxy)
			echo "      - \"127.0.0.1:${host_port}:${CONTAINER_PANEL_PORT}\""
			if [ -n "$inbound_port" ]; then
				echo "      - \"127.0.0.1:${inbound_port}:443\"   # инбаунд, создаётся в панели"
			fi
			;;
		*)
			echo "      - \"${host_port}:${CONTAINER_PANEL_PORT}\""
			echo "      - \"80:80\"   # HTTP-01: нужен и для выпуска, и для продлений"
			;;
	esac
	echo "    volumes:"
	echo "      - ./config:/config"
	echo "    networks:"
	echo "      - routebox"
	echo "networks:"
	echo "  routebox:"
	echo "    ipam:"
	echo "      config:"
	echo "        - subnet: ${subnet}"
}

# gen_vhost DOMAIN UPSTREAM_PORT LISTEN_SPEC
# Сертификат выпускается отдельно (certbot certonly), а vhost сразу пишется со
# ссылками на live-каталог: так повторный запуск может перезаписать файл целиком
# и остаться идемпотентным. certbot --nginx в обычном режиме дописывает
# ssl-директивы в наш же файл, и перезапись их бы снесла.
gen_vhost() {
	local domain="$1" upstream="$2" listen_spec="$3"
	cat <<EOF
${MARKER}
map \$http_upgrade \$rb_connection_upgrade { default upgrade; "" close; }

# Блок на :80 нужен webroot-фолбэку certbot: без сервера, отдающего этот
# каталог для домена, проверка HTTP-01 не пройдёт. Он же уводит случайный
# http-заход на https.
server {
    listen 80;
    server_name ${domain};
    location /.well-known/acme-challenge/ { root /var/www/html; }
    location / { return 301 https://\$host\$request_uri; }
}

server {
    listen ${listen_spec};
    server_name ${domain};

    ssl_certificate     /etc/letsencrypt/live/${domain}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/${domain}/privkey.pem;
EOF
	case "$listen_spec" in
		*proxy_protocol*)
			echo "    set_real_ip_from 127.0.0.1;"
			echo "    real_ip_header proxy_protocol;"
			;;
	esac
	cat <<EOF

    location / {
        proxy_pass http://127.0.0.1:${upstream};
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection \$rb_connection_upgrade;
        proxy_set_header Host \$host;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        # Дашборд держит WebSocket открытым, с таймаутом по умолчанию он рвётся.
        proxy_read_timeout 3600s;
        proxy_buffering off;
    }
}
EOF
}

# gen_stream_conf PANEL_SNI PANEL_HTTPS_PORT INBOUND_SNI INBOUND_PORT PP_PORT
# Промежуточный сервер на PP_PORT существует потому, что proxy_protocol в
# stream — директива уровня server, а не upstream: включить её для панели и не
# включить для инбаунда в одном сервере нельзя, а sing-box PROXY-протокол на
# VLESS не ждёт. Без этого хопа панель видела бы всех клиентов как 127.0.0.1,
# а на адресе клиента держатся блокировка перебора пароля и лимит подписок.
gen_stream_conf() {
	local panel_sni="$1" panel_port="$2" inbound_sni="$3" inbound_port="$4" pp_port="$5"
	cat <<EOF
${MARKER}
map \$ssl_preread_server_name \$rb_upstream {
    ${panel_sni}  127.0.0.1:${pp_port};
EOF
	if [ -n "$inbound_sni" ]; then
		echo "    ${inbound_sni}    127.0.0.1:${inbound_port};   # инбаунд: TLS свой"
	fi
	cat <<EOF
    default            127.0.0.1:${pp_port};
}

server {
    listen 443;
    listen [::]:443;
    ssl_preread on;
    proxy_pass \$rb_upstream;
}

server {
    listen 127.0.0.1:${pp_port};
    proxy_pass 127.0.0.1:${panel_port};
    proxy_protocol on;
}
EOF
}

# stream_include_line -> строка, которая добавляется в nginx.conf.
# sites-enabled и conf.d включаются внутри http{}, поэтому stream туда не
# положить: это единственная правка чужого файла во всей схеме.
stream_include_line() {
	echo "stream { include ${NGINX_STREAM_DIR}/*.conf; }  ${MARKER}"
}

# --- разведка и вопросы ------------------------------------------------------

detect() {
	if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
		HAS_DOCKER="true"
	fi
	if command -v nginx >/dev/null 2>&1 && systemctl is-active nginx >/dev/null 2>&1; then
		HAS_NGINX_HOST="true"
	fi
	if command -v docker >/dev/null 2>&1 &&
	   docker ps --format '{{.Image}}' 2>/dev/null | grep -qi nginx; then
		HAS_NGINX_CONTAINER="true"
	fi

	# Конфигурация nginx может быть сломана: тогда дамп пуст, и верить выводам
	# про домены и 443 нельзя — считаем, что nginx для нас недоступен.
	if [ "$HAS_NGINX_HOST" = "true" ] && ! nginx_config_ok; then
		warn "nginx -T ничего не вернул: конфигурация сломана или недоступна"
		HAS_NGINX_HOST="false"
	fi

	local owner80 owner443
	owner80="$(port_owner 80 || true)"; owner443="$(port_owner 443 || true)"
	case "$owner443" in
		caddy|traefik|haproxy|apache2|httpd) OTHER_PROXY="$owner443" ;;
	esac
	if command -v certbot >/dev/null 2>&1; then HAS_CERTBOT="true"; fi

	info "Что найдено на хосте:"
	echo "  docker + compose:   ${HAS_DOCKER}"
	echo "  nginx на хосте:     ${HAS_NGINX_HOST}"
	echo "  nginx в контейнере: ${HAS_NGINX_CONTAINER}"
	echo "  certbot:            ${HAS_CERTBOT}"
	echo "  порт 80:            ${owner80:-свободен}"
	echo "  порт 443:           ${owner443:-свободен}"
	if [ "$HAS_NGINX_HOST" = "true" ]; then
		if foreign_443; then
			echo "  на 443 есть чужие блоки nginx — SNI-роутер предлагаться не будет"
		fi
		local cert
		for cert in /etc/letsencrypt/live/*/; do
			[ -d "$cert" ] || continue
			cert="${cert%/}"
			echo "  сертификат уже есть: ${cert##*/}"
		done
	fi
	if [ -n "$OTHER_PROXY" ]; then
		warn "  на 443 сидит ${OTHER_PROXY} — автоматики для него нет, панель встанет за ним вручную"
	fi
	if [ "$HAS_NGINX_CONTAINER" = "true" ]; then
		warn "  nginx найден в контейнере — его конфигурацию скрипт не правит"
	fi
	return 0
}

# sni_offer_allowed -> 0, если SNI-роутер можно предлагать: nginx на хосте,
# stream доступен и на 443 нет чужих блоков.
sni_offer_allowed() {
	[ "$HAS_NGINX_HOST" = "true" ] || return 1
	stream_module_available || return 1
	! foreign_443
}

# Ввод идёт с дескриптора 3, открытого в main один раз: stdin занят пайпом от
# curl, а перенаправление на каждом read перечитывало бы первую строку файла.
ask() {
	local prompt="$1" default="$2" answer=""
	printf "%s%s: " "$prompt" "${default:+ [$default]}" >&2
	read -r answer <&3 || answer=""
	echo "${answer:-$default}"
}

ask_yn() {
	local prompt="$1" answer=""
	printf "%s [y/N] " "$prompt" >&2
	read -r answer <&3 || answer=""
	case "$answer" in y|Y|yes) return 0 ;; *) return 1 ;; esac
}

# ask_install_dir вынесен отдельно: его зовут и do_install (до проверки на
# повторный запуск), и do_dry_run. Внутри ask_all его быть не должно — иначе
# вопрос задаётся дважды либо INSTALL_DIR остаётся неопределённым под set -u.
ask_install_dir() {
	INSTALL_DIR="$(ask "Каталог установки" "/opt/routebox")"
}

# ask_port PROMPT DEFAULT -> свободный числовой порт; спрашивает, пока не
# получит годный, вместо того чтобы падать на первой опечатке.
ask_port() {
	local prompt="$1" default="$2" p owner
	while :; do
		p="$(ask "$prompt" "$default")"
		case "$p" in ''|*[!0-9]*) err "порт должен быть числом"; continue ;; esac
		if [ "$p" -lt 1 ] || [ "$p" -gt 65535 ]; then err "порт вне диапазона 1..65535"; continue; fi
		owner="$(port_owner "$p" || true)"
		if [ -n "$owner" ]; then err "порт ${p} занят: ${owner:-кто-то}"; continue; fi
		echo "$p"; return 0
	done
}

ask_all() {
	DOMAIN="$(ask "Домен панели (A-запись должна вести сюда)" "")"
	[ -n "$DOMAIN" ] || { err "домен обязателен"; exit 1; }
	check_dns "$DOMAIN"

	if [ "$HAS_NGINX_HOST" = "true" ]; then
		if ask_yn "TLS держит nginx (иначе панель выпустит сертификат сама)?"; then
			TLS_MODE="nginx"
		else
			TLS_MODE="standalone"
		fi
	elif [ -n "$OTHER_PROXY" ] || [ "$HAS_NGINX_CONTAINER" = "true" ]; then
		# Свой прокси у пользователя уже есть, но править его конфигурацию
		# скрипт не умеет. Панель всё равно ставится правильно — на loopback и
		# без встроенного ACME, — а конфигурацию для прокси печатаем.
		TLS_MODE="proxy"
		info "Панель встанет за вашим прокси: слушать будет только loopback, TLS остаётся на прокси"
	else
		TLS_MODE="standalone"
		info "nginx на хосте нет — панель будет держать TLS сама (встроенный ACME)"
	fi

	EMAIL="$(ask "Контакт для Let's Encrypt" "")"
	[ -n "$EMAIL" ] || { err "почта обязательна"; exit 1; }
	if ask_yn "Использовать тестовый сертификат Let's Encrypt для первого прогона?"; then
		STAGING="true"
	else
		STAGING="false"
	fi

	SUBNET="$(pick_free_subnet)"
	WANT_SNI="false"; INBOUND_DOMAIN=""; INBOUND_PORT=""; PP_PORT=""; PANEL_HTTPS_PORT=""

	if [ "$TLS_MODE" = "proxy" ]; then
		HOST_PORT="$(pick_free_port 8443)"
		PUBLIC_PORT="$(ask "Порт, на котором ваш прокси отдаёт панель клиентам" "443")"
	elif [ "$TLS_MODE" = "nginx" ]; then
		if domain_taken "$DOMAIN"; then
			err "домен ${DOMAIN} уже обслуживается другим блоком nginx"
			err "укажите другой поддомен, ведущий на этот сервер, и запустите снова"
			exit 1
		fi
		local owner443; owner443="$(port_owner 443 || true)"
		if [ -n "$owner443" ] && [ "$owner443" != "nginx" ]; then
			err "порт 443 занят: ${owner443}. Освободите его или выберите режим со встроенным ACME."
			exit 1
		fi
		HOST_PORT="$(pick_free_port 8443)"
		PUBLIC_PORT="443"
		if sni_offer_allowed && ask_yn "Отдавать инбаунды на 443 через SNI-роутер nginx?"; then
			WANT_SNI="true"
			INBOUND_DOMAIN="$(ask "Домен для инбаунда (отдельный от панельного)" "")"
			[ -n "$INBOUND_DOMAIN" ] || { err "домен инбаунда обязателен"; exit 1; }
			PANEL_HTTPS_PORT="$(pick_free_port $((HOST_PORT + 1)))"
			INBOUND_PORT="$(pick_free_port $((PANEL_HTTPS_PORT + 1)))"
			PP_PORT="$(pick_free_port $((INBOUND_PORT + 1)))"
		fi
	else
		local owner80; owner80="$(port_owner 80 || true)"
		if [ -n "$owner80" ]; then
			err "порт 80 занят: ${owner80}. Встроенному ACME он нужен именно снаружи —"
			err "HTTP-01 всегда приходит на 80, пробросом это не лечится."
			if [ "$owner80" = "nginx" ]; then
				err "Раз 80 держит nginx, его и стоит сделать держателем TLS: запустите снова и выберите этот режим."
			fi
			exit 1
		fi
		local default_port; default_port="$(pick_free_port 8443)"
		HOST_PORT="$(ask_port "Порт панели (попадёт в ссылки-подписки)" "$default_port")"
		PUBLIC_PORT="$HOST_PORT"
		# ufw-правил скрипт не создаёт: docker публикует порты мимо цепочек ufw,
		# так что правило дало бы ложное чувство контроля, а не доступ.
		warn "Проверьте, что фаервол пропускает ${HOST_PORT}/tcp и 80/tcp — продления идут по 80"
	fi
}

# resolve_ip и server_public_ip перенесены из vps-install.sh без изменений.
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

server_public_ip() {
	local ip=""
	ip=$(curl -fsS4 --max-time 5 https://api.ipify.org 2>/dev/null || true)
	[ -z "$ip" ] && ip=$(hostname -I 2>/dev/null | awk '{print $1}')
	echo "$ip"
}

# check_dns DOMAIN — расхождение A-записи не отказ, а предупреждение с
# подтверждением: неудачная боевая проверка расходует лимит Let's Encrypt.
check_dns() {
	local resolved server
	resolved="$(resolve_ip "$1")"; server="$(server_public_ip)"
	if [ -n "$resolved" ] && [ "$resolved" = "$server" ]; then
		info "DNS OK: $1 -> ${resolved}"
		return 0
	fi
	warn "DNS не сходится: $1 -> '${resolved:-нет записи}', а сервер '${server:-неизвестен}'."
	warn "Выпуск сертификата не пройдёт, пока A-запись не будет вести сюда."
	ask_yn "Всё равно продолжить?" || { err "прервано; поправьте A-запись"; exit 1; }
}

# --- применение --------------------------------------------------------------

require_root() {
	[ "$(id -u)" = "0" ] || { err "нужны права root: sudo bash docker-install.sh"; exit 1; }
}

write_compose() {
	# Чужой compose в этом каталоге перезаписывать нельзя: existing_install
	# опознаёт только наш, всё остальное — чья-то установка.
	if [ -f "${INSTALL_DIR}/docker-compose.yml" ] && ! existing_install "$INSTALL_DIR"; then
		err "в ${INSTALL_DIR} уже лежит чужой docker-compose.yml — выберите другой каталог"
		exit 1
	fi
	mkdir -p "${INSTALL_DIR}/config"
	gen_compose "$TLS_MODE" "$DOMAIN" "$EMAIL" "$HOST_PORT" "$PUBLIC_PORT" \
		"$SUBNET" "$STAGING" "$INBOUND_PORT" > "${INSTALL_DIR}/docker-compose.yml"
	info "Записан ${INSTALL_DIR}/docker-compose.yml"
}

# Один файл, а не по тарболу на прогон: восстанавливаться всё равно будут из
# последнего. Провал бэкапа не должен убивать скрипт молча через set -e.
backup_nginx() {
	if tar czf /root/routebox-nginx-backup.tar.gz -C /etc nginx 2>/dev/null; then
		info "Бэкап конфигурации nginx: /root/routebox-nginx-backup.tar.gz"
	else
		warn "Бэкап конфигурации nginx не удался — продолжаю без него"
	fi
}

install_nginx_files() {
	local dir vhost listen_spec upstream="$HOST_PORT"
	dir="$(nginx_conf_dir)"
	vhost="${dir}/${NGINX_VHOST_NAME}"
	backup_nginx

	if [ "$WANT_SNI" = "true" ]; then
		listen_spec="127.0.0.1:${PANEL_HTTPS_PORT} ssl proxy_protocol"
	else
		listen_spec="443 ssl http2"
	fi
	gen_vhost "$DOMAIN" "$upstream" "$listen_spec" > "$vhost"
	if [ "$dir" = "/etc/nginx/sites-available" ]; then
		ln -sf "$vhost" "/etc/nginx/sites-enabled/${NGINX_VHOST_NAME}"
	fi

	if [ "$WANT_SNI" = "true" ]; then
		mkdir -p "$NGINX_STREAM_DIR"
		gen_stream_conf "$DOMAIN" "$PANEL_HTTPS_PORT" "$INBOUND_DOMAIN" \
			"$INBOUND_PORT" "$PP_PORT" > "$NGINX_STREAM_FILE"
		ensure_stream_include
	fi

	if ! nginx -t >/dev/null 2>&1; then
		err "nginx -t не прошёл — откатываю свои файлы"
		rm -f "$vhost" "/etc/nginx/sites-enabled/${NGINX_VHOST_NAME}" "$NGINX_STREAM_FILE"
		remove_stream_include
		nginx -t >/dev/null 2>&1 ||
			err "конфигурация nginx сломана и до наших правок — разбирайтесь по бэкапу"
		exit 1
	fi
	systemctl reload nginx
	info "nginx перечитал конфигурацию"
}

# ensure_stream_include — единственная правка чужого файла, и та по маркеру.
ensure_stream_include() {
	if grep -qF "$MARKER" /etc/nginx/nginx.conf; then return 0; fi
	if grep -qE '^[[:space:]]*stream[[:space:]]*\{' /etc/nginx/nginx.conf; then
		err "в nginx.conf уже есть блок stream — добавьте в него строку вручную:"
		err "    include ${NGINX_STREAM_DIR}/*.conf;"
		exit 1
	fi
	ask_yn "Добавить в /etc/nginx/nginx.conf строку с include для stream-конфигурации?" ||
		{ err "без неё SNI-роутер не заработает; прервано"; exit 1; }
	stream_include_line >> /etc/nginx/nginx.conf
}

remove_stream_include() {
	sed -i "\|${MARKER}|d" /etc/nginx/nginx.conf 2>/dev/null || true
}

# Останавливаемся сразу, если сертификат взять негде: vhost ссылается на
# /etc/letsencrypt/live/<домен>/, и без сертификата nginx -t упадёт уже на
# нашем файле — с сообщением не о той проблеме.
issue_cert() {
	[ "$TLS_MODE" = "nginx" ] || return 0
	if [ -s "/etc/letsencrypt/live/${DOMAIN}/fullchain.pem" ]; then
		info "Сертификат для ${DOMAIN} уже есть — переиспользую"
		return 0
	fi
	if ! command -v certbot >/dev/null 2>&1; then
		err "certbot не найден, а сертификата для ${DOMAIN} нет."
		err "Поставьте certbot (apt install certbot python3-certbot-nginx) и запустите снова,"
		err "либо выберите режим со встроенным ACME — тогда сертификат выпустит сама панель."
		exit 1
	fi
	local staging_flag=""
	if [ "$STAGING" = "true" ]; then staging_flag="--test-cert"; fi
	certbot certonly --nginx $staging_flag -d "$DOMAIN" -m "$EMAIL" --agree-tos -n ||
		certbot certonly --webroot -w /var/www/html $staging_flag -d "$DOMAIN" -m "$EMAIL" --agree-tos -n ||
		{ err "certbot не выпустил сертификат для ${DOMAIN}"; exit 1; }
}

compose_up() {
	(cd "$INSTALL_DIR" && docker compose pull && docker compose up -d)
}

# Ждём, пока панель ответит на /api/health, а не просто появится файл пароля:
# на повторной установке файла нет вовсе, и цикл по нему тратил бы полминуты
# впустую, чтобы потом сказать невнятное.
wait_healthy() {
	local i=0
	while [ "$i" -lt 60 ]; do
		if curl -fsS --max-time 2 "http://127.0.0.1:${HOST_PORT}/api/health" >/dev/null 2>&1 ||
		   curl -fsSk --max-time 2 "https://127.0.0.1:${HOST_PORT}/api/health" >/dev/null 2>&1; then
			return 0
		fi
		sleep 1; i=$((i + 1))
	done
	warn "Панель не ответила за минуту — смотрите docker compose logs routebox"
	return 1
}

show_password() {
	local f="${INSTALL_DIR}/config/routebox-initial-password"
	if [ -f "$f" ]; then
		info "Логин admin, пароль: $(cat "$f")"
	else
		info "Пароль администратора был выдан ранее; при необходимости смотрите docker compose logs routebox"
	fi
}

# existing_install DIR -> 0, если в каталоге уже стоит наша панель.
existing_install() {
	[ -f "$1/docker-compose.yml" ] || return 1
	grep -qF "$IMAGE" "$1/docker-compose.yml"
}

# do_update — повторный запуск: ничего не переспрашивает, порты и домен уже
# записаны в compose. Чинит свои файлы nginx, если они пропали.
do_update() {
	info "Найдена установка в ${INSTALL_DIR} — обновляю образ"
	(cd "$INSTALL_DIR" && docker compose pull && docker compose up -d)
	local dir
	if [ "$HAS_NGINX_HOST" = "true" ]; then
		dir="$(nginx_conf_dir)"
		if [ ! -f "${dir}/${NGINX_VHOST_NAME}" ]; then
			warn "vhost панели пропал из ${dir} — перенастройте nginx или переустановите из пустого каталога"
		fi
	fi
	info "Готово"
}

do_install() {
	require_root
	detect
	if [ "$HAS_DOCKER" != "true" ]; then
		err "нужен docker с compose v2: curl -fsSL https://get.docker.com | sh"
		exit 1
	fi
	ask_install_dir
	if existing_install "$INSTALL_DIR"; then
		do_update
		exit 0
	fi
	ask_all
	write_compose
	if [ "$TLS_MODE" = "nginx" ]; then
		issue_cert
		install_nginx_files
	fi
	compose_up
	wait_healthy || true
	show_password
	if [ "$TLS_MODE" = "proxy" ]; then
		info "Конфигурация для вашего прокси (панель слушает 127.0.0.1:${HOST_PORT}):"
		gen_vhost "$DOMAIN" "$HOST_PORT" "443 ssl http2"
		info "Прокси должен пробрасывать WebSocket и передавать X-Forwarded-Proto."
		info "Если адрес прокси не $(gateway_of "$SUBNET"), впишите его в trusted_proxies в /config/routebox.toml."
	fi
	if [ "$WANT_SNI" = "true" ]; then
		info "Дальше — в панели: создайте инбаунд с listen_port 443 и доменом ${INBOUND_DOMAIN}."
		info "Порт 127.0.0.1:${INBOUND_PORT} под него уже проброшен."
	fi
	info "Панель: https://${DOMAIN}:${PUBLIC_PORT}"
}

do_dry_run() {
	detect
	ask_install_dir
	ask_all
	echo "# --- docker-compose.yml, который был бы записан в ${INSTALL_DIR} ---"
	gen_compose "$TLS_MODE" "$DOMAIN" "$EMAIL" "$HOST_PORT" "$PUBLIC_PORT" \
		"$SUBNET" "$STAGING" "$INBOUND_PORT"
	if [ "$TLS_MODE" = "proxy" ]; then
		echo
		echo "# --- конфигурация для вашего прокси (панель на 127.0.0.1:${HOST_PORT}) ---"
		gen_vhost "$DOMAIN" "$HOST_PORT" "443 ssl http2"
	elif [ "$TLS_MODE" = "nginx" ]; then
		echo
		echo "# --- vhost в $(nginx_conf_dir)/${NGINX_VHOST_NAME} ---"
		if [ "$WANT_SNI" = "true" ]; then
			gen_vhost "$DOMAIN" "$HOST_PORT" "127.0.0.1:${PANEL_HTTPS_PORT} ssl proxy_protocol"
			echo
			echo "# --- ${NGINX_STREAM_FILE} ---"
			gen_stream_conf "$DOMAIN" "$PANEL_HTTPS_PORT" "$INBOUND_DOMAIN" "$INBOUND_PORT" "$PP_PORT"
			echo
			echo "# --- строка в /etc/nginx/nginx.conf ---"
			stream_include_line
		else
			gen_vhost "$DOMAIN" "$HOST_PORT" "443 ssl http2"
		fi
	fi
}

do_uninstall() {
	require_root
	detect
	ask_install_dir
	if [ -f "${INSTALL_DIR}/docker-compose.yml" ]; then
		(cd "$INSTALL_DIR" && docker compose down) || true
	fi
	local dir="/etc/nginx/conf.d"
	if [ "$HAS_NGINX_HOST" = "true" ]; then dir="$(nginx_conf_dir)"; fi
	rm -f "${dir}/${NGINX_VHOST_NAME}" "/etc/nginx/sites-enabled/${NGINX_VHOST_NAME}" "$NGINX_STREAM_FILE"
	remove_stream_include
	if [ "$HAS_NGINX_HOST" = "true" ] && nginx -t >/dev/null 2>&1; then
		systemctl reload nginx || true
	fi
	if [ "$PURGE" = "true" ]; then
		rm -rf "${INSTALL_DIR}/config"
		warn "Каталог с данными удалён"
	else
		info "Данные остались в ${INSTALL_DIR}/config (удалить: --purge)"
	fi
}

main() {
	parse_args "$@" || exit 1
	set -euo pipefail
	# Дескриптор для ответов пользователя открывается один раз: stdin занят
	# пайпом от curl. RB_TTY_IN подменяется в тестах файлом с ответами.
	exec 3<"${RB_TTY_IN:-/dev/tty}"
	case "$ACTION" in
		help)      usage; exit 0 ;;
		dry-run)   do_dry_run ;;
		uninstall) do_uninstall ;;
		install)   do_install ;;
	esac
}

# main запускается только при прямом вызове; сорсинг для тестов его пропускает.
if [ "${DOCKER_INSTALL_LIB:-0}" != "1" ]; then
	main "$@"
fi
