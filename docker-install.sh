#!/bin/bash
#
# RouteBox Docker Installer — интерактивная установка панели в контейнере.
# Спрашивает домен и порты, находит nginx на хосте и встраивается в него,
# либо оставляет TLS самой панели (встроенный ACME). Четвёртый режим,
# «из коробки», ставит сервер целиком за одним внешним 443.
#
#   curl -fsSL https://raw.githubusercontent.com/hoaxisr/routebox/main/docker-install.sh | sudo bash
#
# Удаление:  sudo bash docker-install.sh --uninstall [--purge]
# Проверка:  sudo bash docker-install.sh --dry-run

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'

# Образ вынесен в переменную ради прогонов на стенде: без этого проверить
# собранный из проверяемого кода образ можно только опубликовав его.
IMAGE="${RB_IMAGE:-ghcr.io/hoaxisr/routebox:latest}"
MARKER="# managed by routebox"
NGINX_VHOST_NAME="routebox.conf"
NGINX_STREAM_DIR="/etc/nginx/stream-enabled"
NGINX_STREAM_FILE="${NGINX_STREAM_DIR}/routebox.conf"
# Путь вынесен в переменную ради тестов: правка nginx.conf — единственное, что
# скрипт делает с чужим файлом, и проверять это надо на копии, а не на системном.
NGINX_CONF="${RB_NGINX_CONF:-/etc/nginx/nginx.conf}"
CONTAINER_PANEL_PORT="8443"
# Откуда берутся артефакты режима «из коробки». Переменная — ради тестов:
# скачивание подменяется локальным каталогом, сеть в стенде недоступна.
RELEASE_BASE="${RB_RELEASE_BASE:-https://github.com/hoaxisr/routebox/releases/latest/download}"

ACTION="install"; PURGE="false"

# Ответы, пришедшие флагами. Пустое значение означает «спросим».
MODE_FLAG=""; ARG_DOMAIN=""; ARG_EMAIL=""; ARG_DIR=""; ARG_STAGING=""; ARG_STUB=""
# Ядерный бэкенд AmneziaWG: "" — спросить, true/false — ответ уже дан флагом.
ARG_AWG_KERNEL=""
# Порт AmneziaWG: "" — спросить, "0" — не публиковать, число — публиковать его.
ARG_AWG_PORT=""
# Доставка недостающего на хост: "" — спросить, true/false — ответ дан флагом.
ARG_INSTALL_DEPS=""

# Состояние разведки.
HAS_DOCKER="false"; HAS_NGINX_HOST="false"; HAS_NGINX_CONTAINER="false"
OTHER_PROXY=""; HAS_CERTBOT="false"

# Ответы пользователя.
INSTALL_DIR=""; DOMAIN=""; EMAIL=""; TLS_MODE=""; STAGING="false"
HOST_PORT=""; PUBLIC_PORT=""; SUBNET=""
WANT_SNI="false"; INBOUND_DOMAIN=""; INBOUND_PORT=""; PP_PORT=""; PANEL_HTTPS_PORT=""
# Привилегия выдаётся ТОЛЬКО когда на хосте поднялся модуль: она нужна ровно для
# создания интерфейса, которого без модуля не будет, а лишняя привилегия у
# службы, торчащей в интернет, — это плата без покупки.
WANT_CAP_NET_ADMIN="false"
# Порт AmneziaWG, опубликованный на контейнере. Пусто — не публикуется.
AWG_PORT=""
# Ответ на вопрос о ядерном модуле, полученный на этапе вопросов.
WANT_AWG_KERNEL="false"

err()  { echo -e "${RED}Error: $*${NC}" >&2; }
info() { echo -e "${GREEN}$*${NC}"; }
warn() { echo -e "${YELLOW}$*${NC}"; }

usage() {
	cat <<EOF
RouteBox Docker Installer. Без флагов — задаёт вопросы.

  --allinone       Режим «из коробки»: все инбаунды и сайт-заглушка на одном 443
  --domain NAME    Домен (иначе спросим)
  --email ADDR     Контакт для Let's Encrypt (иначе спросим)
  --dir PATH       Каталог установки (по умолчанию /opt/routebox)
  --staging        Тестовый CA Let's Encrypt — для первого прогона
  --stub NAME      Шаблон заглушки (по умолчанию случайный из архива)
  --install-deps   Поставить недостающее на хост (Docker, iproute2) без вопросов
  --no-deps        Не ставить ничего: не хватает — остановиться
  --awg-port N     Опубликовать UDP-порт AmneziaWG (0 — не публиковать)
  --awg-kernel     Поставить на ХОСТ ядерный модуль AmneziaWG (Debian/Ubuntu)
  --no-awg-kernel  Не спрашивать про него и не ставить
  --dry-run        Показать, что будет сделано и какие файлы получатся
  --uninstall      Удалить контейнер и файлы nginx, созданные установщиком
  --purge          Вместе с --uninstall: удалить и каталог ./config с данными
  --help           Эта справка
EOF
}

parse_args() {
	ACTION="install"; PURGE="false"
	MODE_FLAG=""; ARG_DOMAIN=""; ARG_EMAIL=""; ARG_DIR=""; ARG_STAGING=""; ARG_STUB=""
# Ядерный бэкенд AmneziaWG: "" — спросить, true/false — ответ уже дан флагом.
ARG_AWG_KERNEL=""
# Порт AmneziaWG: "" — спросить, "0" — не публиковать, число — публиковать его.
ARG_AWG_PORT=""
# Доставка недостающего на хост: "" — спросить, true/false — ответ дан флагом.
ARG_INSTALL_DEPS=""
	while [ $# -gt 0 ]; do
		case "$1" in
			--dry-run)   ACTION="dry-run" ;;
			--uninstall) ACTION="uninstall" ;;
			--purge)     PURGE="true" ;;
			--allinone)  MODE_FLAG="allinone" ;;
			--staging)   ARG_STAGING="true" ;;
			--stub)      shift; [ $# -gt 0 ] || { err "--stub без значения"; return 1; }; ARG_STUB="$1" ;;
			--install-deps)  ARG_INSTALL_DEPS="true" ;;
			--no-deps)       ARG_INSTALL_DEPS="false" ;;
			--awg-port)  shift; [ $# -gt 0 ] || { err "--awg-port без значения"; return 1; }; ARG_AWG_PORT="$1" ;;
			--awg-kernel)    ARG_AWG_KERNEL="true" ;;
			--no-awg-kernel) ARG_AWG_KERNEL="false" ;;
			--domain)    shift; [ $# -gt 0 ] || { err "--domain без значения"; return 1; }; ARG_DOMAIN="$1" ;;
			--email)     shift; [ $# -gt 0 ] || { err "--email без значения"; return 1; }; ARG_EMAIL="$1" ;;
			--dir)       shift; [ $# -gt 0 ] || { err "--dir без значения"; return 1; }; ARG_DIR="$1" ;;
			--help|-h)   ACTION="help" ;;
			*) err "неизвестный аргумент: $1"; usage; return 1 ;;
		esac
		shift
	done
	if [ "$PURGE" = "true" ] && [ "$ACTION" != "uninstall" ]; then
		err "--purge имеет смысл только вместе с --uninstall"
		return 1
	fi
	return 0
}

# --- порты -------------------------------------------------------------------

# port_owner PORT [PROTO] -> печатает, кто занял порт; код 0 если занят, 1 если
# свободен. PROTO — tcp (по умолчанию) или udp: режиму «из коробки» нужен один
# и тот же номер порта по обоим протоколам, фронт держит TCP, mieru — UDP.
# Два источника: ss видит слушающие сокеты, docker ps — опубликованные порты.
# Второй нужен потому, что при userland-proxy: false порт живёт только в
# правилах DNAT и сокета под ним нет — ss его не покажет.
port_owner() {
	local port="$1" proto="${2:-tcp}" line="" ss_flags="-ltnpH"
	# Не `[ ... ] && ...`: под set -e ложное условие в конце AND-списка уронило
	# бы функцию, и занятый порт сошёл бы за свободный.
	if [ "$proto" = "udp" ]; then ss_flags="-lunpH"; fi
	if command -v ss >/dev/null 2>&1; then
		line=$(ss $ss_flags 2>/dev/null | awk -v p=":${port}\$" '$4 ~ p {print; exit}')
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
		if docker ps --format '{{.Ports}}' 2>/dev/null |
				grep -qE "(^|,| )[^,]*:${port}->[^,]*/${proto}"; then
			echo "docker"
			return 0
		fi
		# Диапазон `0.0.0.0:8440-8450->8440-8450/tcp` занимает всё, что внутри.
		local spec lo hi
		for spec in $(docker ps --format '{{.Ports}}' 2>/dev/null |
				grep -oE ":[0-9]+-[0-9]+->[0-9]+-[0-9]+/${proto}" || true); do
			spec="${spec#:}"; spec="${spec%%->*}"
			lo="${spec%%-*}"; hi="${spec##*-}"
			if [ "$port" -ge "$lo" ] && [ "$port" -le "$hi" ]; then
				echo "docker"
				return 0
			fi
		done
	fi
	return 1
}

# busy_note START CHOSEN -> " (8443 занят nginx, 8444 занят docker)" или пусто.
# Молчаливый выбор порта выглядит как произвол; строка объясняет, почему не
# взят ожидаемый 8443.
busy_note() {
	local p="$1" chosen="$2" owner note=""
	while [ "$p" -lt "$chosen" ]; do
		owner="$(port_owner "$p" || true)"
		note="${note:+${note}, }${p} занят ${owner:-кем-то}"
		p=$((p + 1))
	done
	[ -n "$note" ] && echo " (${note})"
	return 0
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
		/^[[:space:]]*#/ { next }
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

# ip_to_int A.B.C.D -> целое. Нужно для проверки пересечения диапазонов.
ip_to_int() {
	local a b c d
	IFS=. read -r a b c d <<<"$1"
	echo $(((a << 24) + (b << 16) + (c << 8) + d))
}

# cidr_overlap A/N B/M -> 0, если диапазоны пересекаются.
# Строкового сравнения мало: docker раздаёт сетям /16, и существующая
# 172.28.0.0/16 не совпадает со строкой 172.28.0.0/24, хотя накрывает её
# целиком — compose потом падает с «Pool overlaps». То же с маршрутом
# 172.16.0.0/12 от VPN, который накрывает все кандидаты сразу.
cidr_overlap() {
	local a="${1%%/*}" alen="${1##*/}" b="${2%%/*}" blen="${2##*/}"
	case "$a$b" in *:*) return 1 ;; esac   # IPv6 не сравниваем
	case "$1$2" in *[!0-9./]*) return 1 ;; esac
	local astart bstart amask bmask aend bend
	astart="$(ip_to_int "$a")"; bstart="$(ip_to_int "$b")"
	amask=$(( alen == 0 ? 0 : (0xFFFFFFFF << (32 - alen)) & 0xFFFFFFFF ))
	bmask=$(( blen == 0 ? 0 : (0xFFFFFFFF << (32 - blen)) & 0xFFFFFFFF ))
	astart=$((astart & amask)); bstart=$((bstart & bmask))
	aend=$((astart + (0xFFFFFFFF & ~amask)))
	bend=$((bstart + (0xFFFFFFFF & ~bmask)))
	[ "$astart" -le "$bend" ] && [ "$bstart" -le "$aend" ]
}

subnet_taken() {
	local cidr="$1" existing="" one=""
	if command -v docker >/dev/null 2>&1; then
		# shellcheck disable=SC2046  # список сетей — именно несколько аргументов
		existing="$(docker network inspect $(docker network ls -q 2>/dev/null) \
			--format '{{range .IPAM.Config}}{{.Subnet}} {{end}}' 2>/dev/null || true)"
	fi
	if command -v ip >/dev/null 2>&1; then
		existing="${existing} $(ip -o route 2>/dev/null | awk '{print $1}' || true)"
	fi
	for one in $existing; do
		case "$one" in */*) ;; *) continue ;; esac
		if cidr_overlap "$cidr" "$one"; then return 0; fi
	done
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
	if [ -n "$AWG_PORT" ]; then echo "      - AWG_LISTEN_PORT=${AWG_PORT}"; fi
	echo "    ports:"
	if [ -n "$AWG_PORT" ]; then
		echo "      - \"${AWG_PORT}:${AWG_PORT}/udp\"   # AmneziaWG"
	fi
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
	if [ "$WANT_CAP_NET_ADMIN" = "true" ]; then
		echo "    cap_add:"
		echo "      - NET_ADMIN   # ядерный бэкенд AmneziaWG: создание интерфейса"
	fi
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

# gen_compose_allinone DOMAIN EMAIL SUBNET STAGING
# Две службы в одной сетевой области видимости: sing-box держит наружный 443 по
# TCP (фронт) и по UDP (mieru), dest сидит на обратной петле и получает от фронта
# всё, что не разобрали инбаунды. `network_mode: service:routebox` — это и есть
# «dest на loopback»: 127.0.0.1 у них общий, поэтому адрес dest из плана
# (127.0.0.1:9443) работает в обе стороны, а сам порт наружу не опубликован.
# Наружу публикуется только 443/tcp, 443/udp и 80 — последний нужен dest для
# HTTP-01: TLS-ALPN-01 отвечал бы на 443, а он занят фронтом.
gen_compose_allinone() {
	local domain="$1" email="$2" subnet="$3" staging="$4"

	echo "# Создано docker-install.sh, режим «из коробки». Переменные окружения"
	echo "# читаются один раз, когда контейнер создаёт /config/routebox.toml."
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
	echo "      - PUBLIC_PORT=443"
	echo "      - BOOTSTRAP_ALLINONE=1"
	# Свой ACME панели здесь выключен: 443 держит sing-box, сертификат для домена
	# выпускает dest, и второй претендент на выпуск только жёг бы лимит выдачи.
	echo "      - ACME_ENABLED=false"
	echo "      - ACME_EMAIL=${email}"
	if [ "$staging" = "true" ]; then echo "      - ACME_STAGING=true"; fi
	# Панель стоит за dest, а тот приходит с общей обратной петли. Без этой
	# строки каждый вход в панель выглядел бы приходом с 127.0.0.1, и блокировка
	# перебора пароля считала бы попытки всех сразу.
	echo "      - TRUSTED_PROXIES=127.0.0.1/32"
	# Панель узнаёт из этой переменной, что порт задан установкой, пишет его в
	# настройки при первом старте и дальше не даёт менять: другой порт попал бы
	# на сокет, который никто не пробрасывает.
	if [ -n "$AWG_PORT" ]; then echo "      - AWG_LISTEN_PORT=${AWG_PORT}"; fi
	echo "    ports:"
	echo "      - \"443:443/tcp\"   # фронт: vless + reality"
	echo "      - \"443:443/udp\"   # mieru"
	echo "      - \"80:80\"         # HTTP-01: выпуск и продление сертификата"
	if [ -n "$AWG_PORT" ]; then
		echo "      - \"${AWG_PORT}:${AWG_PORT}/udp\"   # AmneziaWG"
	fi
	if [ "$WANT_CAP_NET_ADMIN" = "true" ]; then
		echo "    cap_add:"
		echo "      - NET_ADMIN   # ядерный бэкенд AmneziaWG: создание интерфейса"
	fi
	echo "    volumes:"
	echo "      - ./config:/config"
	echo "    networks:"
	echo "      - routebox"
	echo "  dest:"
	# Тот же образ, а не отдельный: он уже скачан, а бинарь dest статический и
	# лежит на томе — второй образ добавил бы зависимость от чужого реестра
	# ради /bin/sh.
	echo "    image: ${IMAGE}"
	echo "    container_name: routebox-dest"
	echo "    restart: unless-stopped"
	echo "    depends_on:"
	echo "      - routebox"
	echo "    network_mode: \"service:routebox\""
	echo "    environment:"
	# Каталог данных Caddy: без него сертификаты легли бы в файловую систему
	# контейнера и выпускались бы заново при каждом пересоздании — прямо в
	# лимит выдачи Let's Encrypt.
	echo "      - XDG_DATA_HOME=/config/dest"
	echo "      - XDG_CONFIG_HOME=/config/dest"
	# Caddyfile пишет сама панель на первом старте, поэтому dest его дожидается.
	# Без ожидания служба падала бы в цикл перезапусков до конца первого старта.
	echo "    entrypoint:"
	echo "      - /bin/sh"
	echo "      - -c"
	echo "      - until [ -s /config/Caddyfile ]; do sleep 1; done; exec /config/bin/dest run --config /config/Caddyfile --adapter caddyfile"
	# Проверка здоровья из образа опрашивает панель, а не dest: в общей сетевой
	# области она проходит всегда и говорила бы не о той службе.
	echo "    healthcheck:"
	echo "      disable: true"
	echo "    volumes:"
	echo "      - ./config:/config"
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
	# Имя образа, а не подстрока: иначе nginx-prometheus-exporter сойдёт за
	# обратный прокси и панель молча уедет на loopback без того, кто её отдаёт.
	if command -v docker >/dev/null 2>&1 &&
	   docker ps --format '{{.Image}}' 2>/dev/null |
	   grep -qiE '(^|/)(nginx|openresty|jc21/nginx-proxy-manager|jwilder/nginx-proxy|linuxserver/swag)(:|$)'; then
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

# ask_yn PROMPT [DEFAULT] — DEFAULT это y или n (по умолчанию n).
ask_yn() {
	local prompt="$1" default="${2:-n}" hint="y/N" answer=""
	if [ "$default" = "y" ]; then hint="Y/n"; fi
	printf "%s [%s] " "$prompt" "$hint" >&2
	# Пустая строка — это умолчание, а конец ввода — нет: с умолчанием «y»
	# оборванный ввод иначе означал бы согласие, которого никто не давал.
	if ! read -r answer <&3; then answer="n"; else answer="${answer:-$default}"; fi
	case "$answer" in y|Y|yes) return 0 ;; *) return 1 ;; esac
}

# ask_install_dir вынесен отдельно: его зовут и do_install (до проверки на
# повторный запуск), и do_dry_run. Внутри ask_all его быть не должно — иначе
# вопрос задаётся дважды либо INSTALL_DIR остаётся неопределённым под set -u.
ask_install_dir() {
	INSTALL_DIR="${ARG_DIR:-$(ask "Каталог установки" "/opt/routebox")}"
}

# ask_port PROMPT DEFAULT -> свободный числовой порт; спрашивает, пока не
# получит годный, вместо того чтобы падать на первой опечатке.
ask_port() {
	local prompt="$1" default="$2" p owner
	while :; do
		p="$(ask "$prompt" "$default")"
		case "$p" in ''|*[!0-9]*) err "порт должен быть числом"; continue ;; esac
		if [ "$p" -lt 1 ] || [ "$p" -gt 65535 ]; then err "порт вне диапазона 1..65535"; continue; fi
		# Решает код возврата, а не наличие имени: `ss` без root показывает
		# чужие сокеты без владельца, и проверка по имени приняла бы занятый
		# порт за свободный.
		if owner="$(port_owner "$p")"; then
			err "порт ${p} занят${owner:+: $owner}"; continue
		fi
		echo "$p"; return 0
	done
}

# valid_domain NAME -> 0, если это похоже на доменное имя. Пробел или `;` в
# ответе иначе уезжает в server_name и в пути сертификата, а ломается всё это
# позже и с невнятной диагностикой.
valid_domain() {
	case "$1" in
		''|*[!A-Za-z0-9.-]*) return 1 ;;
		.*|-*|*-|*.) return 1 ;;
		*.*) return 0 ;;
		*) return 1 ;;
	esac
}

ask_all() {
	if want_allinone; then
		ask_allinone
		return 0
	fi
	DOMAIN="${ARG_DOMAIN:-$(ask "Домен панели (A-запись должна вести сюда)" "")}"
	if ! valid_domain "$DOMAIN"; then
		err "нужно доменное имя вида panel.example.com"
		exit 1
	fi
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

	# За чужим прокси сертификат выпускает он сам: ни почта, ни выбор CA нам
	# не нужны — не спрашиваем то, что некуда положить.
	if [ "$TLS_MODE" != "proxy" ]; then
		EMAIL="${ARG_EMAIL:-$(ask "Контакт для Let's Encrypt" "")}"
		[ -n "$EMAIL" ] || { err "почта обязательна"; exit 1; }
		if [ -n "$ARG_STAGING" ]; then
			STAGING="$ARG_STAGING"
		elif ask_yn "Использовать тестовый сертификат Let's Encrypt для первого прогона?"; then
			STAGING="true"
		else
			STAGING="false"
		fi
	fi

	ask_awg_port
	ask_awg_kernel
	SUBNET="$(pick_free_subnet)"
	WANT_SNI="false"; INBOUND_DOMAIN=""; INBOUND_PORT=""; PP_PORT=""; PANEL_HTTPS_PORT=""

	if [ "$TLS_MODE" = "proxy" ]; then
		HOST_PORT="$(pick_free_port 8443)"
		while :; do
			PUBLIC_PORT="$(ask "Порт, на котором ваш прокси отдаёт панель клиентам" "443")"
			case "$PUBLIC_PORT" in ''|*[!0-9]*) err "порт должен быть числом" ;; *) break ;; esac
		done
	elif [ "$TLS_MODE" = "nginx" ]; then
		if domain_taken "$DOMAIN"; then
			err "домен ${DOMAIN} уже обслуживается другим блоком nginx"
			err "укажите другой поддомен, ведущий на этот сервер, и запустите снова"
			exit 1
		fi
		local owner443=""
		if owner443="$(port_owner 443)" && [ "$owner443" != "nginx" ]; then
			err "порт 443 занят${owner443:+: $owner443}. Освободите его или выберите режим со встроенным ACME."
			exit 1
		fi
		HOST_PORT="$(pick_free_port 8443)"
		PUBLIC_PORT="443"
		info "панель → 127.0.0.1:${HOST_PORT}$(busy_note 8443 "$HOST_PORT")"
		if sni_offer_allowed && ask_yn "Отдавать инбаунды на 443 через SNI-роутер nginx?"; then
			WANT_SNI="true"
			INBOUND_DOMAIN="$(ask "Домен для инбаунда (отдельный от панельного)" "")"
			if ! valid_domain "$INBOUND_DOMAIN"; then
				err "нужно доменное имя вида vpn.example.com"
				exit 1
			fi
			PANEL_HTTPS_PORT="$(pick_free_port $((HOST_PORT + 1)))"
			INBOUND_PORT="$(pick_free_port $((PANEL_HTTPS_PORT + 1)))"
			PP_PORT="$(pick_free_port $((INBOUND_PORT + 1)))"
			info "https-слушатель nginx → 127.0.0.1:${PANEL_HTTPS_PORT}, инбаунд → 127.0.0.1:${INBOUND_PORT}, PROXY-протокол → 127.0.0.1:${PP_PORT}"
		fi
	else
		local owner80=""
		if owner80="$(port_owner 80)"; then
			err "порт 80 занят${owner80:+: $owner80}. Встроенному ACME он нужен именно снаружи —"
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

# resolve_ips DOMAIN -> все A-записи домена, по одной в строке.
# Отдельно от resolve_ip: у домена с несколькими A-записями адрес этого сервера
# может не быть первым, и сверка «по первой» остановила бы установку на
# совершенно правильном домене.
resolve_ips() {
	local d="$1" out=""
	if command -v getent >/dev/null 2>&1; then
		out="$(getent ahostsv4 "$d" 2>/dev/null | awk '{print $1}')"
	fi
	if [ -z "$out" ] && command -v dig >/dev/null 2>&1; then
		out="$(dig +short A "$d" 2>/dev/null)"
	fi
	if [ -z "$out" ] && command -v host >/dev/null 2>&1; then
		out="$(host -t A "$d" 2>/dev/null | awk '/has address/{print $4}')"
	fi
	echo "$out" | awk 'NF' | sort -u
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

# --- режим «из коробки» -------------------------------------------------------
#
# Четвёртый режим рядом с standalone, nginx и proxy: наружу открыт ровно один
# порт — 443 по TCP (фронт) и по UDP (mieru), — а панель, dest и остальные
# инбаунды сидят на обратной петле. nginx в этой схеме не участвует, поэтому ни
# одного чужого файла скрипт здесь не правит.

# require_free PORT PROTO — предусловие «внешний порт свободен».
# Опираемся на состояние сокетов, а не на проверку конфигурации веб-сервера:
# `nginx -t` на чужом сервере, уже слушающем 443, скажет, что всё в порядке, и
# конфликт владельцев вылезет только при запуске — на половине установки.
require_free() {
	local port="$1" proto="$2" owner
	if owner="$(port_owner "$port" "$proto")"; then
		err "внешний порт ${port}/${proto} занят${owner:+: $owner}"
		err "в режиме «из коробки» этот порт держит сам сервер; освободите его"
		err "или выберите другой режим установки"
		exit 1
	fi
	return 0
}

# require_dns_match DOMAIN — домен обязан вести на этот сервер.
# Здесь это отказ, а не предупреждение с подтверждением, как в остальных
# режимах: сертификат выпускает dest по HTTP-01, и промах A-записи означает
# сервер, который не поднимется вообще, — а понять это оператор сможет только
# по логам контейнера.
require_dns_match() {
	local server ips one
	server="$(server_public_ip)"
	ips="$(resolve_ips "$1")"
	if [ -n "$server" ]; then
		for one in $ips; do
			if [ "$one" = "$server" ]; then
				info "DNS OK: $1 -> ${one}"
				warn_stray_aaaa "$1"
				return 0
			fi
		done
	fi
	local where; where="${ips//$'\n'/ }"
	err "домен $1 ведёт на '${where:-нет A-записи}', а этот сервер — '${server:-неизвестен}'"
	err "поправьте A-запись и запустите снова: без неё сертификат не выпустится"
	exit 1
}

# warn_stray_aaaa DOMAIN — предупреждение о AAAA-записи.
# Проверка выше сверяет IPv4, а Let's Encrypt для HTTP-01 предпочитает IPv6:
# посторонняя AAAA-запись уводит проверку на чужую машину, и выпуск падает уже
# после установки — ровно тот случай, ради которого сверка сделана жёсткой.
# Именно предупреждение: убедиться, что AAAA ведёт сюда, скрипт не может —
# своего IPv6 он не знает.
warn_stray_aaaa() {
	command -v getent >/dev/null 2>&1 || return 0
	# `::ffff:` отбрасывается: getent показывает так обычные A-записи, и без
	# этого предупреждение срабатывало бы на каждом домене без AAAA.
	# Именно awk, а не `grep -v`: под pipefail grep, не нашедший ни строки,
	# возвращает 1, подстановка команд наследует этот код, и присваивание роняет
	# весь скрипт — на домене без AAAA, то есть на самом обычном.
	local v6; v6="$(getent ahostsv6 "$1" 2>/dev/null |
		awk '$1 !~ /^::ffff:/ {print $1}' | sort -u | head -3)"
	[ -n "$v6" ] || return 0
	warn "у домена есть AAAA-запись (${v6//$'\n'/ }): Let's Encrypt пойдёт по IPv6."
	warn "Убедитесь, что она ведёт на этот сервер, иначе сертификат не выпустится."
	return 0
}

# allinone_possible -> 0, если режим здесь вообще имеет смысл.
# Предлагать невозможное — значит просить оператора ответить на вопрос, у
# которого нет годного ответа. Явный --allinone при занятом порте, наоборот,
# останавливает установку с именем владельца: там оператор уже выбрал.
allinone_possible() {
	if [ "$HAS_DOCKER" != "true" ]; then return 1; fi
	# Без ss занятость хостовых сокетов не видна, и «свободен» стало бы просто
	# другим именем для «не смогли проверить».
	if ! command -v ss >/dev/null 2>&1; then return 1; fi
	local spec
	for spec in "443 tcp" "443 udp" "80 tcp"; do
		# shellcheck disable=SC2086  # пара «порт протокол» — именно два аргумента
		if port_owner $spec >/dev/null; then return 1; fi
	done
	return 0
}

want_allinone() {
	if [ "$MODE_FLAG" = "allinone" ]; then return 0; fi
	if ! allinone_possible; then return 1; fi
	ask_yn "Поставить всё из коробки: пять инбаундов и сайт-заглушка на одном 443?" y
}

ask_allinone() {
	TLS_MODE="allinone"
	if ! command -v ss >/dev/null 2>&1; then
		err "нет ss (пакет iproute2) — нечем проверить, свободны ли 443 и 80"
		err "поставьте iproute2 и запустите снова: молча подвинуть чужой сокет"
		err "хуже, чем остановиться"
		exit 1
	fi
	# Порты проверяются до вопросов: занятый 443 не лечится ни одним ответом.
	require_free 443 tcp
	require_free 443 udp
	# 80 нужен снаружи для HTTP-01: сертификат выпускает dest, а пробросом
	# HTTP-01 не лечится — проверка всегда приходит именно на 80.
	require_free 80 tcp

	DOMAIN="${ARG_DOMAIN:-$(ask "Домен (он же имя, которое заимствует Reality)" "")}"
	if ! valid_domain "$DOMAIN"; then
		err "нужно доменное имя вида example.com"
		exit 1
	fi
	require_dns_match "$DOMAIN"

	EMAIL="${ARG_EMAIL:-$(ask "Контакт для Let's Encrypt" "")}"
	[ -n "$EMAIL" ] || { err "почта обязательна"; exit 1; }
	STAGING="${ARG_STAGING:-false}"

	# Панель наружу не публикуется вовсе: до неё ходит dest по сети контейнера,
	# а клиентские ссылки собираются от внешнего 443.
	HOST_PORT="$CONTAINER_PANEL_PORT"
	PUBLIC_PORT="443"
	ask_awg_port
	ask_awg_kernel
	SUBNET="$(pick_free_subnet)"
	WANT_SNI="false"; INBOUND_DOMAIN=""; INBOUND_PORT=""; PP_PORT=""; PANEL_HTTPS_PORT=""
}

# detect_arch -> amd64|arm64. Артефакты релиза собраны только под них.
detect_arch() {
	case "$(uname -m)" in
		x86_64)  echo "amd64" ;;
		aarch64) echo "arm64" ;;
		*) err "архитектура $(uname -m) не поддерживается: артефакты собираются под amd64 и arm64"; exit 1 ;;
	esac
}

# fetch_verified URL DEST — скачать и сверить контрольную сумму.
# Отсутствие файла суммы — тоже отказ, а не «продолжим без проверки»: все наши
# артефакты публикуются с ним, поэтому его отсутствие означает не старый релиз,
# а что скачано не то и не оттуда.
fetch_verified() {
	local url="$1" dest="$2" sum expected actual
	curl -fsSL -o "$dest" "$url" || { err "не скачалось: ${url}"; exit 1; }
	sum="$(mktemp)"
	if ! curl -fsSL -o "$sum" "${url}.sha256" || [ ! -s "$sum" ]; then
		rm -f "$sum"
		err "нет файла контрольной суммы для ${url} — установка прервана"
		exit 1
	fi
	expected="$(awk '{print $1}' "$sum")"; rm -f "$sum"
	actual="$(sha256sum "$dest" | awk '{print $1}')"
	if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
		err "контрольная сумма не сошлась для ${url}"
		err "ожидалось ${expected:-пусто}, получено ${actual}"
		exit 1
	fi
	info "sha256 сверен: $(basename "$dest")"
}

# pick_stub DIR -> имя шаблона заглушки.
# Случайный, если не задан флагом: одна и та же заглушка на всех установках сама
# по себе становится приметой, по которой их видно списком.
pick_stub() {
	local dir="$1" names=() one
	for one in "$dir"/*/; do
		[ -d "$one" ] || continue
		one="${one%/}"
		names+=("${one##*/}")
	done
	if [ "${#names[@]}" -eq 0 ]; then
		err "в архиве шаблонов нет ни одного каталога"
		exit 1
	fi
	if [ -n "$ARG_STUB" ]; then
		for one in "${names[@]}"; do
			if [ "$one" = "$ARG_STUB" ]; then echo "$one"; return 0; fi
		done
		err "шаблона ${ARG_STUB} в архиве нет; есть: ${names[*]}"
		exit 1
	fi
	echo "${names[$((RANDOM % ${#names[@]}))]}"
}

# install_dest_and_stub — доставка dest и файлов заглушки на том контейнера.
# Оба артефакта кладутся туда, где их ждёт план: бинарь — /config/bin/dest,
# файлы заглушки — /config/stub (корень, который планировщик прописал в
# Caddyfile). Всё это делается ДО записи compose: иначе оборванная закачка
# оставила бы каталог, который повторный запуск примет за готовую установку.
install_dest_and_stub() {
	local dir="${INSTALL_DIR}/config" arch tmp chosen
	arch="$(detect_arch)"
	# Скачивается рядом с установкой, а не в /tmp: любой отказ ниже — это выход
	# из скрипта, и через ловушку на EXIT это не решить (в тестах скрипт
	# сорсится, и такая ловушка затёрла бы чужую). Каталог с известным именем
	# сносится в начале следующего запуска и уходит вместе с --purge.
	tmp="${dir}/.download"
	rm -rf "$tmp"
	mkdir -p "$tmp" "${dir}/bin"

	info "Скачиваю dest (linux-${arch})..."
	fetch_verified "${RELEASE_BASE}/routebox-dest-linux-${arch}" "${tmp}/dest"
	install -m 0755 "${tmp}/dest" "${dir}/bin/dest"

	info "Скачиваю шаблоны заглушки..."
	fetch_verified "${RELEASE_BASE}/routebox-stubs.tar.gz" "${tmp}/stubs.tar.gz"
	mkdir -p "${tmp}/unpacked"
	tar xzf "${tmp}/stubs.tar.gz" -C "${tmp}/unpacked" ||
		{ err "архив шаблонов не распаковался"; exit 1; }
	# В архиве один каталог stubs/, внутри — по каталогу на шаблон.
	chosen="$(pick_stub "${tmp}/unpacked/stubs")"
	rm -rf "${dir}/stub"
	mkdir -p "${dir}/stub"
	cp -r "${tmp}/unpacked/stubs/${chosen}/." "${dir}/stub/"
	rm -rf "$tmp"
	info "Заглушка: ${chosen}"
}

# announce_panel_url — путь к панели печатается один раз, когда он появился.
# Спрашиваем у самого бинаря, а не собираем адрес здесь: секрет живёт в
# настройках, и второе место, где он складывается в URL, рано или поздно
# разойдётся с первым.
announce_panel_url() {
	local i=0 url=""
	while [ "$i" -lt 120 ]; do
		url="$( (cd "$INSTALL_DIR" && docker compose exec -T routebox routebox panel-url) 2>/dev/null | tr -d '\r')"
		if [ -n "$url" ]; then
			info "Панель: ${url}"
			info "Заходить по этому адресу: он ставит cookie, без неё на домене отдаётся заглушка."
			info "Адрес можно спросить снова: docker compose exec routebox routebox panel-url"
			return 0
		fi
		if [ "$i" = "20" ]; then
			info "Жду выпуск сертификата и первый старт, это занимает до минуты..."
		fi
		sleep 1; i=$((i + 1))
	done
	warn "Панель не отозвалась за две минуты — смотрите docker compose logs"
	return 1
}

# allinone_plan — перечисление намерений. Ничего не читает и ничего не меняет.
allinone_plan() {
	local acme="боевой Let's Encrypt"
	if [ "$STAGING" = "true" ]; then acme="тестовый Let's Encrypt"; fi
	echo "# --- режим «из коробки»: что будет сделано ---"
	echo "домен:           ${DOMAIN}"
	echo "каталог:         ${INSTALL_DIR}"
	echo "наружу:          443/tcp — фронт, 443/udp — mieru, 80/tcp — только выпуск сертификата"
	echo "внутрь:          dest, панель и остальные инбаунды слушают обратную петлю"
	echo "сеть контейнера: ${SUBNET}"
	echo "сертификат:      выпускает dest, ${acme}"
	echo "почта:           ${EMAIL} — уезжает в настройки панели"
	echo "AmneziaWG:       ${AWG_PORT:+порт ${AWG_PORT}/udp наружу}${AWG_PORT:-не публикуется}"
	echo "заглушка:        ${ARG_STUB:-случайный шаблон из архива релиза}"
	echo "артефакты:       dest и шаблоны — из релиза, со сверкой sha256"
	echo "чужие файлы:     не правятся ни одного"
	echo "панель:          по секретному пути, он будет напечатан один раз после старта"
}

# --- предусловия хоста --------------------------------------------------------

# missing_host_tools -> список недостающего, человеческим языком, или пусто.
# Два обязательных: docker с compose v2 (без него ставить некуда) и ss из
# iproute2 (им проверяется занятость портов; без него «свободен» было бы просто
# другим именем для «не смогли проверить»).
missing_host_tools() {
	local miss=""
	if [ "$HAS_DOCKER" != "true" ]; then miss="docker с compose v2"; fi
	if ! command -v ss >/dev/null 2>&1; then miss="${miss:+${miss}, }iproute2 (ss)"; fi
	echo "$miss"
}

# pkg_install PKG — поставить пакет тем, что есть на хосте.
pkg_install() {
	if command -v apt-get >/dev/null 2>&1; then
		apt-get update >/dev/null 2>&1 && apt-get install -y "$1" >/dev/null 2>&1
	elif command -v dnf >/dev/null 2>&1; then
		dnf install -y "$1" >/dev/null 2>&1
	elif command -v yum >/dev/null 2>&1; then
		yum install -y "$1" >/dev/null 2>&1
	elif command -v apk >/dev/null 2>&1; then
		apk add --no-cache "$1" >/dev/null 2>&1
	elif command -v pacman >/dev/null 2>&1; then
		pacman -Sy --noconfirm "$1" >/dev/null 2>&1
	else
		return 1
	fi
}

# ensure_host_tools — доставить недостающее, спросив один раз.
# Установка Docker — заметное изменение системы, поэтому по умолчанию она
# спрашивается, а не делается молча; get.docker.com — тот же путь, который
# скрипт до сих пор советовал выполнить руками. Заодно он ставит плагин
# compose там, где docker есть, а плагина нет.
ensure_host_tools() {
	local miss; miss="$(missing_host_tools)"
	[ -n "$miss" ] || return 0
	if [ "$ARG_INSTALL_DEPS" = "false" ]; then return 0; fi
	if [ "$ARG_INSTALL_DEPS" != "true" ]; then
		info "На хосте не хватает: ${miss}"
		ask_yn "Поставить это сейчас?" y || return 0
	fi

	if [ "$HAS_DOCKER" != "true" ]; then
		info "Ставлю Docker (get.docker.com)..."
		if ! curl -fsSL https://get.docker.com | sh >/dev/null 2>&1; then
			warn "установка Docker не удалась"
		fi
	fi
	if ! command -v ss >/dev/null 2>&1; then
		info "Ставлю iproute2..."
		pkg_install iproute2 || warn "не удалось поставить iproute2 — поставьте его сами"
	fi

	# Перепроверяем: дальше решения принимаются по состоянию хоста, а не по
	# тому, что мы пытались сделать.
	detect >/dev/null
	miss="$(missing_host_tools)"
	if [ -n "$miss" ]; then
		warn "всё ещё не хватает: ${miss}"
	fi
	return 0
}

# --- ядерный модуль AmneziaWG на хосте ----------------------------------------
#
# Из контейнера модуль не собрать и не загрузить: ядро принадлежит хосту,
# загрузка требует CAP_SYS_MODULE (по сути root на хосте), а наш установщик
# модуля умеет только apt и DKMS — образ же собран на Alpine. Зато ЭТОТ скрипт
# уже root на хосте, поэтому ставит он. Дальше DKMS пересобирает модуль сам на
# каждом обновлении ядра — отдельного механизма обновления не нужно.
#
# Шаги повторяют backend/internal/awg/module.go один в один, включая сверку
# отпечатка ключа PPA до того, как ключ попадёт в доверенные.
AWG_KEY_FINGERPRINT="75C9DD72C799870E310542E24166F2C257290828"
AWG_PPA_URI="https://ppa.launchpadcontent.net/amnezia/ppa/ubuntu/"
AWG_KEYRING="${RB_AWG_KEYRING:-/usr/share/keyrings/amnezia-awg.gpg}"
AWG_SOURCES="${RB_AWG_SOURCES:-/etc/apt/sources.list.d/amnezia-awg.sources}"
AWG_KEYSERVER="keyserver.ubuntu.com"
AWG_OS_RELEASE="${RB_OS_RELEASE:-/etc/os-release}"

# os_field KEY -> значение из os-release без кавычек.
os_field() {
	sed -n "s/^$1=//p" "$AWG_OS_RELEASE" 2>/dev/null | head -1 | tr -d '"'
}

# awg_kernel_possible -> 0, если ядерный модуль тут вообще можно поставить.
# Три условия: семейство Debian (наш путь установки — apt и PPA Amnezia),
# наличие каталога модулей running-ядра (в OpenVZ и LXC его нет, и модули туда
# не грузятся) и известное кодовое имя выпуска — из него собирается suite PPA.
awg_kernel_possible() {
	local id id_like
	id="$(os_field ID)"; id_like="$(os_field ID_LIKE)"
	case " ${id} ${id_like} " in
		*" debian "*|*" ubuntu "*) ;;
		*) return 1 ;;
	esac
	[ -d "/lib/modules/$(uname -r)" ] || return 1
	[ -n "$(os_field VERSION_CODENAME)" ] || return 1
	return 0
}

# awg_kernel_wanted -> 0, если модуль надо ставить. Вопрос задаётся только там,
# где ответ «да» к чему-то приведёт: предлагать невозможное — значит просить
# оператора выбрать из одного варианта.
# ask_awg_kernel — задаётся вместе с остальными вопросами, ответ запоминается.
ask_awg_kernel() {
	if awg_kernel_wanted; then WANT_AWG_KERNEL="true"; else WANT_AWG_KERNEL="false"; fi
	return 0
}

awg_kernel_wanted() {
	if [ "$ARG_AWG_KERNEL" = "true" ]; then
		if awg_kernel_possible; then return 0; fi
		err "ядерный модуль AmneziaWG здесь поставить нельзя:"
		err "нужен хост семейства Debian/Ubuntu с каталогом модулей своего ядра"
		err "(в OpenVZ и LXC модули не грузятся). Остаётся бэкенд singbox — ему модуль не нужен."
		exit 1
	fi
	[ "$ARG_AWG_KERNEL" = "false" ] && return 1
	awg_kernel_possible || return 1
	ask_yn "Поставить на хост ядерный модуль AmneziaWG? Он быстрее, но не обязателен"
}

# install_awg_module — установка на ХОСТ. Провал не валит установку панели:
# ядерный бэкенд ускоряет AmneziaWG, но без него всё работает на singbox.
# Возвращает 1, если модуль не поднялся, — тогда привилегия контейнеру не
# выдаётся: она нужна ровно для интерфейса, которого без модуля не будет.
install_awg_module() {
	local ver codename scratch
	ver="$(uname -r)"; codename="$(os_field VERSION_CODENAME)"
	info "Ставлю ядерный модуль AmneziaWG на хост (${codename}, ядро ${ver})..."

	local step
	for step in "update" "install -y gnupg2" "install -y dirmngr" "install -y linux-headers-${ver}"; do
		# shellcheck disable=SC2086  # шаг — это список аргументов, не одна строка
		if ! apt-get $step >/dev/null 2>&1; then
			warn "apt-get ${step} не прошёл — модуль не ставится, остаётся singbox"
			return 1
		fi
	done

	# Сверка ДО доверия: ключ принимается во временную связку, его отпечаток
	# сверяется с прошитым, и только после этого он попадает в доверенный
	# каталог. Несовпадение — это подмена ключа или репозитория, и тогда мы
	# не ставим ничего.
	scratch="$(mktemp -d)"
	if ! gpg --no-default-keyring --keyring "${scratch}/awg.gpg" \
			--keyserver "$AWG_KEYSERVER" --recv-keys "$AWG_KEY_FINGERPRINT" >/dev/null 2>&1; then
		rm -rf "$scratch"
		warn "ключ PPA Amnezia не получен — модуль не ставится, остаётся singbox"
		return 1
	fi
	local got
	got="$(gpg --no-default-keyring --keyring "${scratch}/awg.gpg" --fingerprint 2>/dev/null |
		tr -d ' ' | tr 'a-f' 'A-F')"
	case "$got" in
		*"$AWG_KEY_FINGERPRINT"*) ;;
		*)
			rm -rf "$scratch"
			err "отпечаток ключа PPA не совпал с прошитым (${AWG_KEY_FINGERPRINT})"
			err "это подмена ключа или репозитория — ничего не ставлю"
			return 1
			;;
	esac
	if ! gpg --no-default-keyring --keyring "${scratch}/awg.gpg" \
			--output "$AWG_KEYRING" --export "$AWG_KEY_FINGERPRINT" >/dev/null 2>&1; then
		rm -rf "$scratch"
		warn "ключ PPA не экспортировался — модуль не ставится, остаётся singbox"
		return 1
	fi
	rm -rf "$scratch"

	mkdir -p "$(dirname "$AWG_SOURCES")"
	cat > "$AWG_SOURCES" <<EOF
Types: deb
URIs: ${AWG_PPA_URI}
Suites: ${codename}
Components: main
Signed-By: ${AWG_KEYRING}
EOF

	if ! apt-get update >/dev/null 2>&1 || ! apt-get install -y amneziawg >/dev/null 2>&1; then
		warn "пакет amneziawg не установился — модуль не ставится, остаётся singbox"
		return 1
	fi
	if ! modprobe amneziawg >/dev/null 2>&1; then
		warn "modprobe amneziawg не прошёл — модуль не загрузился, остаётся singbox"
		return 1
	fi
	info "Модуль amneziawg загружен. DKMS пересоберёт его сам при обновлении ядра."
	return 0
}

# ask_awg_port — порт AmneziaWG выбирается ЗДЕСЬ, а не потом в панели.
# Публикацию порта нельзя добавить работающему контейнеру: нужно править compose
# и пересоздавать его. Панель этого сделать не может (для этого ей нужен
# docker.sock, то есть root на хосте у службы, торчащей в интернет), поэтому
# порт фиксируется на установке — и панель дальше не даёт его менять.
#
# Второй внешний UDP-порт стоит дёшево: AmneziaWG 3.1 на неаутентифицированный
# пакет не отвечает вовсе, а молчащий UDP-порт снаружи неотличим от
# отфильтрованного.
ask_awg_port() {
	local p
	while :; do
		if [ -n "$ARG_AWG_PORT" ]; then
			p="$ARG_AWG_PORT"
		else
			# Умолчание — «не нужен»: Enter не должен открывать наружу порт,
			# которого не просили, тем более в режиме, весь смысл которого в
			# одном внешнем порте. Кому AmneziaWG нужен, тот вводит номер.
			p="$(ask "Порт для AmneziaWG, если он нужен (0 — нет; потом не изменить)" "0")"
		fi
		case "$p" in
			0|"") AWG_PORT=""; return 0 ;;
			*[!0-9]*)
				err "порт должен быть числом"
				[ -n "$ARG_AWG_PORT" ] && exit 1
				continue ;;
		esac
		if [ "$p" -lt 1 ] || [ "$p" -gt 65535 ]; then
			err "порт вне диапазона 1..65535"
			[ -n "$ARG_AWG_PORT" ] && exit 1
			continue
		fi
		local owner
		# Проверяется UDP: AmneziaWG живёт по UDP, и занятость TCP-порта с тем же
		# номером ему не мешает.
		if owner="$(port_owner "$p" udp)"; then
			err "порт ${p}/udp занят${owner:+: $owner}"
			[ -n "$ARG_AWG_PORT" ] && exit 1
			continue
		fi
		AWG_PORT="$p"
		return 0
	done
}

# maybe_install_awg_module — поставить (если на вопрос ответили «да») и только
# при успехе выдать контейнеру привилегию. Спрашивает не она: все вопросы
# задаются до того, как что-то делается, иначе оператор, отошедший от терминала
# после ответов, возвращается к скрипту, который ничего не сделал и ждёт.
maybe_install_awg_module() {
	[ "$WANT_AWG_KERNEL" = "true" ] || return 0
	if install_awg_module; then
		WANT_CAP_NET_ADMIN="true"
		if [ -z "$AWG_PORT" ]; then
			# Модуль есть, а порта наружу нет: сервер включится и не пустит никого,
			# и заметить это можно будет только по молчащему клиенту.
			warn "Порт AmneziaWG не публикуется: сервер, включённый в панели, будет"
			warn "недостижим снаружи. Допишите порт в ${INSTALL_DIR}/docker-compose.yml"
			warn "и в переменную AWG_LISTEN_PORT, затем: cd ${INSTALL_DIR} && docker compose up -d"
		fi
	fi
	return 0
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
	if [ "$TLS_MODE" = "allinone" ]; then
		gen_compose_allinone "$DOMAIN" "$EMAIL" "$SUBNET" "$STAGING" \
			> "${INSTALL_DIR}/docker-compose.yml"
	else
		gen_compose "$TLS_MODE" "$DOMAIN" "$EMAIL" "$HOST_PORT" "$PUBLIC_PORT" \
			"$SUBNET" "$STAGING" "$INBOUND_PORT" > "${INSTALL_DIR}/docker-compose.yml"
	fi
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

	# Согласие на правку nginx.conf спрашивается ДО того, как что-то записано:
	# иначе отказ на этом шаге оставлял бы vhost, слушающий внутренний порт, и
	# stream-файл, который никто не включает, — панель снаружи недоступна,
	# и выяснится это на ближайшем reload от продления сертификата.
	if [ "$WANT_SNI" = "true" ]; then
		confirm_stream_include
	fi
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

# confirm_stream_include — только спрашивает и проверяет, ничего не пишет.
# Вызывается до записи любых файлов, чтобы отказ не оставлял полуконфигурации.
confirm_stream_include() {
	if grep -qF "$MARKER" "$NGINX_CONF"; then return 0; fi
	if grep -qE '^[[:space:]]*stream[[:space:]]*\{' "$NGINX_CONF"; then
		err "в nginx.conf уже есть блок stream — добавьте в него строку вручную:"
		err "    include ${NGINX_STREAM_DIR}/*.conf;"
		err "затем запустите установщик снова"
		exit 1
	fi
	if ! ask_yn "Добавить в ${NGINX_CONF} строку с include для stream-конфигурации?"; then
		err "без неё SNI-роутер не заработает; ничего не изменено"
		exit 1
	fi
}

# ensure_stream_include — единственная правка чужого файла, и та по маркеру.
ensure_stream_include() {
	if grep -qF "$MARKER" "$NGINX_CONF"; then return 0; fi
	# Ведущий перевод строки обязателен: если файл не заканчивается переводом,
	# наша строка приклеится к последней — к чужой закрывающей `}`. Тогда откат
	# по маркеру снёс бы эту `}` вместе с нашей строкой и оставил nginx.conf
	# сломанным навсегда, с сообщением «оно и до нас было сломано».
	printf '\n%s\n' "$(stream_include_line)" >> "$NGINX_CONF"
}

# remove_if_ours PATH — удаляет файл, только если он начинается нашим маркером.
# Совпадения имени мало: routebox.conf в conf.d мог написать и человек.
remove_if_ours() {
	local f="$1"
	[ -f "$f" ] || return 0
	if head -1 "$f" | grep -qF "$MARKER"; then
		rm -f "$f"
		info "Удалён ${f}"
	else
		warn "${f} писали не мы (нет маркера) — оставляю как есть"
	fi
}

remove_stream_include() {
	sed -i "\|${MARKER}|d" "$NGINX_CONF" 2>/dev/null || true
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
	prepare_challenge_vhost
	# webroot первым, а не --nginx: после prepare_challenge_vhost каталог
	# проверки гарантированно отдаётся именно для нашего домена, тогда как
	# --nginx полагается на то, что подходящий server-блок уже есть.
	# shellcheck disable=SC2086  # staging_flag — либо пусто, либо один флаг
	certbot certonly --webroot -w /var/www/html $staging_flag \
			-d "$DOMAIN" -m "$EMAIL" --agree-tos -n ||
		certbot certonly --nginx $staging_flag -d "$DOMAIN" -m "$EMAIL" --agree-tos -n ||
		{ err "certbot не выпустил сертификат для ${DOMAIN}"; exit 1; }
}

# prepare_challenge_vhost — минимальный :80-блок под HTTP-01, до выпуска
# сертификата. Без него certbot нечем проверить домен: ssl-части настоящего
# vhost'а ссылаются на сертификат, которого ещё нет, а `--nginx` на хосте без
# подходящего server-блока (conf.d-раскладки, CentOS) просто падает.
# Файл потом перезапишется полным vhost'ом — путь тот же, мусора не остаётся.
prepare_challenge_vhost() {
	local dir vhost
	dir="$(nginx_conf_dir)"
	vhost="${dir}/${NGINX_VHOST_NAME}"
	mkdir -p /var/www/html
	backup_nginx
	cat > "$vhost" <<EOF
${MARKER}
server {
    listen 80;
    server_name ${DOMAIN};
    location /.well-known/acme-challenge/ { root /var/www/html; }
    location / { return 404; }
}
EOF
	if [ "$dir" = "/etc/nginx/sites-available" ]; then
		ln -sf "$vhost" "/etc/nginx/sites-enabled/${NGINX_VHOST_NAME}"
	fi
	if ! nginx -t >/dev/null 2>&1; then
		err "nginx -t не прошёл на блоке для проверки домена — откатываю"
		rm -f "$vhost" "/etc/nginx/sites-enabled/${NGINX_VHOST_NAME}"
		exit 1
	fi
	systemctl reload nginx
}

compose_up() {
	# Провал обновления образа — не повод не поднимать тот, что уже есть:
	# реестр может лежать или упереться в лимит, а образ быть на месте. Если его
	# нет вовсе, `up -d` скажет об этом сам и сообщением по делу.
	if ! (cd "$INSTALL_DIR" && docker compose pull); then
		warn "docker compose pull не прошёл — поднимаю на том образе, что уже скачан"
	fi
	if ! (cd "$INSTALL_DIR" && docker compose up -d); then
		err "docker compose не поднял контейнер. Конфигурация записана в ${INSTALL_DIR},"
		err "так что можно поправить её и повторить: cd ${INSTALL_DIR} && docker compose up -d"
		exit 1
	fi
}

# Ждём, пока панель ответит на /api/health, а не просто появится файл пароля:
# на повторной установке файла нет вовсе, и цикл по нему тратил бы полминуты
# впустую, чтобы потом сказать невнятное.
# Обращаться нужно по имени домена, а не по 127.0.0.1: в standalone панель
# держит TLS через autocert с белым списком доменов, и запрос по IP не несёт
# SNI — рукопожатие не состоится никогда, сколько ни жди. `--resolve` даёт
# нужное имя в TLS и при этом идёт на локальный адрес. Первый такой запрос
# заодно запускает выпуск сертификата, поэтому ждём до двух минут.
wait_healthy() {
	local i=0
	while [ "$i" -lt 120 ]; do
		if curl -fsSk --max-time 3 --resolve "${DOMAIN}:${HOST_PORT}:127.0.0.1" \
			"https://${DOMAIN}:${HOST_PORT}/api/health" >/dev/null 2>&1 ||
		   curl -fsS --max-time 3 "http://127.0.0.1:${HOST_PORT}/api/health" >/dev/null 2>&1; then
			return 0
		fi
		if [ "$i" = "20" ] && [ "$TLS_MODE" = "standalone" ]; then
			info "Ждём выпуск сертификата Let's Encrypt, это занимает до минуты..."
		fi
		sleep 1; i=$((i + 1))
	done
	warn "Панель не ответила за две минуты — смотрите docker compose logs routebox"
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
# allinone_install DIR -> 0, если в каталоге лежит наш compose режима «из
# коробки». Признак — переменная, которая только в нём и встречается.
allinone_install() {
	[ -f "$1/docker-compose.yml" ] || return 1
	grep -qF "BOOTSTRAP_ALLINONE" "$1/docker-compose.yml"
}

do_update() {
	info "Найдена установка в ${INSTALL_DIR} — обновляю образ"
	# Бинарь dest и файлы заглушки живут на томе, а не в образе: если их снесли
	# (например, `--uninstall --purge`, а потом установка заново), контейнер dest
	# уходит в вечный перезапуск на несуществующем файле, и домен молчит.
	# Дешевле восстановить их здесь, чем объяснять это по логам.
	if allinone_install "$INSTALL_DIR" &&
			{ [ ! -x "${INSTALL_DIR}/config/bin/dest" ] ||
			  [ ! -f "${INSTALL_DIR}/config/stub/index.html" ]; }; then
		warn "dest или заглушка пропали с тома — доставляю заново"
		install_dest_and_stub
	fi
	# Тот же compose_up, что и при установке: провалившееся обновление образа не
	# должно рвать запуск на полпути, оставляя контейнеры как есть и без единого
	# слова о том, чем всё кончилось.
	compose_up
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
	ensure_host_tools
	if [ "$HAS_DOCKER" != "true" ]; then
		err "нужен docker с compose v2: curl -fsSL https://get.docker.com | sh"
		exit 1
	fi
	ask_install_dir
	if existing_install "$INSTALL_DIR"; then
		do_update
		exit 0
	fi
	# Чужой compose ловим здесь, а не в write_compose: иначе можно было бы
	# выпустить сертификат и настроить nginx, чтобы упереться в это в конце.
	if [ -f "${INSTALL_DIR}/docker-compose.yml" ]; then
		err "в ${INSTALL_DIR} уже лежит чужой docker-compose.yml — выберите другой каталог"
		exit 1
	fi
	ask_all
	if [ "$TLS_MODE" = "allinone" ]; then
		allinone_plan
		maybe_install_awg_module
		# Артефакты первыми: оборванная закачка после записи compose оставила бы
		# каталог, который повторный запуск примет за готовую установку и уйдёт
		# обновлять то, что ещё не собрано.
		install_dest_and_stub
		write_compose
		compose_up
		# Сначала ожидание, потом пароль: файл с первым паролем пишет сама панель
		# на первом старте, и до него show_password сказал бы «пароль выдан
		# ранее» — на свежей установке это просто неправда.
		announce_panel_url || true
		show_password
		info "Сайт: https://${DOMAIN}"
		return 0
	fi
	# Сначала nginx и сертификат, и только потом compose. Обратный порядок
	# оставлял бы после любого провала каталог с нашим compose, который при
	# повторном запуске опознаётся как готовая установка и уводит в do_update —
	# то есть настройку уже не доделать, только сносить и начинать заново.
	if [ "$TLS_MODE" = "nginx" ]; then
		issue_cert
		install_nginx_files
	fi
	maybe_install_awg_module
	write_compose
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
	if [ "$TLS_MODE" = "allinone" ]; then
		allinone_plan
		echo
		echo "# --- docker-compose.yml, который был бы записан в ${INSTALL_DIR} ---"
		gen_compose_allinone "$DOMAIN" "$EMAIL" "$SUBNET" "$STAGING"
		return 0
	fi
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
	# Опечатка в пути не должна класть чужой стек: сносим только свой compose.
	if [ -f "${INSTALL_DIR}/docker-compose.yml" ]; then
		if existing_install "$INSTALL_DIR"; then
			(cd "$INSTALL_DIR" && docker compose down) || true
			# Свой compose — тоже свой файл, и его надо унести. Оставленный, он
			# при следующей установке опознаётся как готовый и уводит в
			# обновление: контейнеры поднимутся, а снесённых артефактов уже
			# никто не доставит.
			rm -f "${INSTALL_DIR}/docker-compose.yml"
			info "Удалён ${INSTALL_DIR}/docker-compose.yml"
		else
			err "в ${INSTALL_DIR} лежит чужой docker-compose.yml — не трогаю его"
			exit 1
		fi
	fi
	local dir="/etc/nginx/conf.d"
	if [ "$HAS_NGINX_HOST" = "true" ]; then dir="$(nginx_conf_dir)"; fi
	remove_if_ours "${dir}/${NGINX_VHOST_NAME}"
	remove_if_ours "$NGINX_STREAM_FILE"
	# Симлинк удаляем только вместе с целью, которую сами и создавали.
	if [ -L "/etc/nginx/sites-enabled/${NGINX_VHOST_NAME}" ] && [ ! -e "${dir}/${NGINX_VHOST_NAME}" ]; then
		rm -f "/etc/nginx/sites-enabled/${NGINX_VHOST_NAME}"
	fi
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
	# --help терминала не требует: открывать дескриптор до него значило бы
	# падать с «/dev/tty: No such device» в CI и в ssh без -t.
	if [ "$ACTION" = "help" ]; then usage; exit 0; fi
	# Дескриптор для ответов пользователя открывается один раз: stdin занят
	# пайпом от curl. RB_TTY_IN подменяется в тестах файлом с ответами.
	exec 3<"${RB_TTY_IN:-/dev/tty}" || {
		err "нет терминала для вопросов: запустите скрипт из интерактивной сессии"
		exit 1
	}
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
