#!/bin/bash
# Тесты docker-install.sh. Запуск: bash tests/docker-install.test.sh
# Ни root, ни docker, ни nginx не требуются: всё внешнее подменяется стабами
# в PATH, а вывод `nginx -T` — переменной RB_NGINX_T_CMD.
HERE="$(cd "$(dirname "$0")" && pwd)"
FIXTURES="$HERE/fixtures"
FAILS=0
# Настоящий docker, если он тут есть, — запоминается ДО того, как PATH накроют
# стабы: им проверяется схема сгенерированного compose. Отсутствует — проверка
# пропускается, стенд от docker не зависит.
REAL_DOCKER="$(command -v docker 2>/dev/null || true)"

assert_eq() {
	if [ "$1" = "$2" ]; then printf '  ok   %s\n' "$3"
	else printf '  FAIL %s\n       ожидалось: %s\n       получено:  %s\n' "$3" "$1" "$2"; FAILS=$((FAILS+1)); fi
}
assert_contains() {
	case "$1" in *"$2"*) printf '  ok   %s\n' "$3" ;;
	*) printf '  FAIL %s\n       не найдено: %s\n' "$3" "$2"; FAILS=$((FAILS+1)) ;; esac
}
assert_not_contains() {
	case "$1" in *"$2"*) printf '  FAIL %s\n       найдено лишнее: %s\n' "$3" "$2"; FAILS=$((FAILS+1)) ;;
	*) printf '  ok   %s\n' "$3" ;; esac
}
assert_fails() {
	if eval "$1" >/dev/null 2>&1; then printf '  FAIL %s (команда завершилась успешно)\n' "$2"; FAILS=$((FAILS+1))
	else printf '  ok   %s\n' "$2"; fi
}

DOCKER_INSTALL_LIB=1 . "$HERE/../docker-install.sh"

echo "parse_args"
parse_args --help;      assert_eq "help"      "$ACTION" "--help -> help"
parse_args --dry-run;   assert_eq "dry-run"   "$ACTION" "--dry-run -> dry-run"
parse_args --uninstall; assert_eq "uninstall" "$ACTION" "--uninstall -> uninstall"
parse_args --uninstall --purge; assert_eq "true" "$PURGE" "--purge -> PURGE=true"
parse_args;             assert_eq "install"   "$ACTION" "без флагов -> install"
assert_fails 'parse_args --tls-mode x' "неизвестный флаг отвергается"

# --- стабы внешних команд ----------------------------------------------------
STUBS="$(mktemp -d)"
trap 'rm -rf "$STUBS"' EXIT

cat >"$STUBS/ss" <<'EOF'
#!/bin/bash
# 8443 занят nginx, 80 занят nginx, остальное свободно
echo 'LISTEN 0 511 0.0.0.0:8443 0.0.0.0:* users:(("nginx",pid=1,fd=6))'
echo 'LISTEN 0 511 0.0.0.0:80 0.0.0.0:* users:(("nginx",pid=1,fd=7))'
EOF
cat >"$STUBS/docker" <<'EOF'
#!/bin/bash
case "$1" in
	ps)      echo '0.0.0.0:8444->8444/tcp, [::]:8555->8555/tcp, 0.0.0.0:18666->18666/tcp' ;;
	network) echo '172.28.0.0/24' ;;
	compose) exit 0 ;;
esac
exit 0
EOF
cat >"$STUBS/ip" <<'EOF'
#!/bin/bash
echo "172.29.0.0/24 dev br-abc proto kernel scope link src 172.29.0.1"
EOF
cat >"$STUBS/nginx" <<'EOF'
#!/bin/bash
[ "$1" = "-V" ] && echo "configure arguments: --with-stream" >&2
exit 0
EOF
cat >"$STUBS/systemctl" <<'EOF'
#!/bin/bash
exit 0
EOF
cat >"$STUBS/curl" <<'EOF'
#!/bin/bash
echo "203.0.113.10"
EOF
cat >"$STUBS/getent" <<'EOF'
#!/bin/bash
echo "203.0.113.10 STREAM panel.example.com"
EOF
chmod +x "$STUBS"/*
PATH="$STUBS:$PATH"

echo
echo "порты"
assert_eq "8445" "$(pick_free_port 8443)" "8443 и 8444 заняты -> 8445"
assert_eq "9000" "$(pick_free_port 9000)" "свободный порт возвращается как есть"
assert_contains "$(port_owner 8443)" "nginx" "владелец 8443 определяется как nginx"
assert_fails 'port_owner 9000' "свободный порт -> ненулевой код"
assert_contains "$(port_owner 8444)" "docker" "порт из docker ps опознаётся"
assert_contains "$(port_owner 8555)" "docker" "порт в IPv6-форме [::]:8555-> опознаётся"
assert_fails 'port_owner 8666' "18666 не принимается за 8666"

# Порт, занятый чужим процессом, `ss` без root показывает без имени владельца.
# Решать должен код возврата, иначе занятый порт сойдёт за свободный.
cat >"$STUBS/ss" <<'EOF'
#!/bin/bash
echo 'LISTEN 0 511 0.0.0.0:8443 0.0.0.0:* users:(("nginx",pid=1,fd=6))'
echo 'LISTEN 0 511 0.0.0.0:80 0.0.0.0:* users:(("nginx",pid=1,fd=7))'
echo 'LISTEN 0 128 0.0.0.0:2049 0.0.0.0:*'
EOF
chmod +x "$STUBS/ss"
assert_eq "0" "$(port_owner 2049 >/dev/null; echo $?)" "порт без имени владельца всё равно занят"
assert_eq ""  "$(port_owner 2049)" "имени владельца при этом нет"

# Диапазон портов из docker ps занимает всё, что внутри него.
cat >"$STUBS/docker" <<'EOF'
#!/bin/bash
case "$1" in
	ps)      echo '0.0.0.0:8444->8444/tcp, [::]:8555->8555/tcp, 0.0.0.0:18666->18666/tcp, 0.0.0.0:9100-9110->9100-9110/tcp' ;;
	network) echo '172.28.0.0/24' ;;
	compose) exit 0 ;;
esac
exit 0
EOF
chmod +x "$STUBS/docker"
assert_eq "0" "$(port_owner 9105 >/dev/null; echo $?)" "порт внутри диапазона 9100-9110 занят"
assert_fails 'port_owner 9111' "порт за границей диапазона свободен"

# ask_port должен отвергать занятый порт даже тогда, когда имя владельца
# неизвестно, и спрашивать снова. Ответы подаются тем же дескриптором 3.
ANS="$(mktemp)"
printf '2049\nне-число\n9111\n' > "$ANS"
exec 3<"$ANS"
assert_eq "9111" "$(ask_port "порт" "9111" 2>/dev/null)" "занятый порт без владельца и мусор отвергаются, спрашивается снова"
exec 3<&-
rm -f "$ANS"

echo
echo "проверка доменных имён"
assert_eq "0" "$(valid_domain panel.example.com; echo $?)" "обычный домен принимается"
assert_fails 'valid_domain "panel.example.com; rm -rf /"' "точка с запятой отвергается"
assert_fails 'valid_domain "panel example.com"'           "пробел отвергается"
assert_fails 'valid_domain localhost'                     "имя без точки отвергается"
assert_fails 'valid_domain ""'                            "пустой ответ отвергается"

echo
echo "строка include в nginx.conf"
# Файл без завершающего перевода строки: наша строка не должна приклеиться к
# чужой закрывающей скобке, иначе откат по маркеру унесёт её с собой.
NCONF="$(mktemp -d)"
NGINX_CONF="$NCONF/nginx.conf"
printf 'events {}\nhttp {\n    server { listen 80; }\n}' > "$NGINX_CONF"   # без перевода строки в конце
ensure_stream_include
# Точное сравнение, а не поиск подстроки: приклеенная строка вида
# `}stream { include ...` содержит и то и другое и прошла бы мягкую проверку.
assert_eq "$(stream_include_line)" "$(tail -1 "$NGINX_CONF")" "наша строка — отдельная последняя строка"
assert_eq "}" "$(tail -2 "$NGINX_CONF" | head -1)" "чужая закрывающая скобка осталась целой строкой"
ensure_stream_include
assert_eq "1" "$(grep -c 'stream { include' "$NGINX_CONF")" "повторный вызов не дублирует строку"
remove_stream_include
assert_eq "}" "$(tail -1 "$NGINX_CONF")" "после отката чужая скобка на месте"
assert_eq "0" "$(grep -c "$MARKER" "$NGINX_CONF")" "после отката нашей строки не осталось"
rm -rf "$NCONF"
NGINX_CONF="/etc/nginx/nginx.conf"

echo
echo "разбор nginx"
export RB_NGINX_T_CMD="cat $FIXTURES/nginx-clean.conf"
assert_fails 'foreign_443' "чистый конфиг: чужих блоков на 443 нет"
assert_fails 'domain_taken panel.example.com' "чистый конфиг: домен свободен"
assert_eq "/etc/nginx/conf.d" "$(nginx_conf_dir)" "раскладка conf.d опознана"

export RB_NGINX_T_CMD="cat $FIXTURES/nginx-busy443.conf"
assert_eq "0" "$(foreign_443; echo $?)" "чужой server на 443 найден"
assert_eq "/etc/nginx/sites-available" "$(nginx_conf_dir)" "раскладка sites-enabled опознана"

export RB_NGINX_T_CMD="cat $FIXTURES/nginx-domain-taken.conf"
assert_eq "0" "$(domain_taken panel.example.com; echo $?)" "занятый домен найден"
assert_fails 'domain_taken other.example.com' "чужой домен не считается занятым"

export RB_NGINX_T_CMD="cat $FIXTURES/nginx-commented.conf"
assert_fails 'domain_taken panel.example.com' "закомментированный server_name не блокирует установку"
assert_eq "0" "$(domain_taken other.example.com; echo $?)" "живой server_name в том же файле находится"
unset RB_NGINX_T_CMD

echo
echo "подсети"
assert_eq "0" "$(subnet_taken 172.28.0.0/24; echo $?)" "занятая docker-сетью подсеть отвергается"
assert_eq "0" "$(subnet_taken 172.29.0.0/24; echo $?)" "подсеть из таблицы маршрутов отвергается"
assert_eq "172.30.0.0/24" "$(pick_free_subnet)" "берётся первая свободная"
assert_eq "172.30.0.1" "$(gateway_of 172.30.0.0/24)" "шлюз подсети"

# Пересечения, которых не видно строковым сравнением: docker раздаёт сетям /16,
# а VPN может держать маршрут /12 поверх всех кандидатов сразу.
assert_eq "0" "$(cidr_overlap 172.28.0.0/24 172.28.0.0/16; echo $?)" "/16 накрывает кандидата /24"
assert_eq "0" "$(cidr_overlap 172.28.0.0/24 172.16.0.0/12; echo $?)" "/12 накрывает кандидата /24"
assert_fails 'cidr_overlap 172.28.0.0/24 172.29.0.0/24' "соседние /24 не пересекаются"
assert_fails 'cidr_overlap 172.28.0.0/24 10.0.0.0/8'    "чужой блок адресов не пересекается"
assert_fails 'cidr_overlap 172.28.0.0/24 fd00::/64'     "IPv6 не ломает сравнение"

cat >"$STUBS/docker" <<'EOF'
#!/bin/bash
case "$1" in
	ps)      echo '0.0.0.0:8444->8444/tcp, [::]:8555->8555/tcp, 0.0.0.0:18666->18666/tcp' ;;
	network) echo '172.28.0.0/16' ;;
	compose) exit 0 ;;
esac
exit 0
EOF
chmod +x "$STUBS/docker"
assert_eq "0" "$(subnet_taken 172.28.0.0/24; echo $?)" "существующая /16 делает кандидата /24 занятым"
# Возвращаем стаб к /24, чтобы дальнейшие проверки шли в прежних условиях.
cat >"$STUBS/docker" <<'EOF'
#!/bin/bash
case "$1" in
	ps)      echo '0.0.0.0:8444->8444/tcp, [::]:8555->8555/tcp, 0.0.0.0:18666->18666/tcp' ;;
	network) echo '172.28.0.0/24' ;;
	compose) exit 0 ;;
esac
exit 0
EOF
chmod +x "$STUBS/docker"

echo
echo "генератор compose"
C_STANDALONE="$(gen_compose standalone panel.example.com you@example.com 9443 9443 172.28.0.0/24 true "")"
assert_contains "$C_STANDALONE" "PUBLIC_HOST=panel.example.com" "standalone: PUBLIC_HOST"
assert_contains "$C_STANDALONE" "PUBLIC_PORT=9443"              "standalone: PUBLIC_PORT равен внешнему порту"
assert_contains "$C_STANDALONE" "ACME_EMAIL=you@example.com"    "standalone: почта ACME"
assert_contains "$C_STANDALONE" "ACME_STAGING=true"             "standalone: тестовый CA"
assert_contains "$C_STANDALONE" '"9443:8443"'                   "standalone: панель проброшена наружу"
assert_contains "$C_STANDALONE" '"80:80"'                       "standalone: порт 80 для HTTP-01"
assert_not_contains "$C_STANDALONE" "ACME_ENABLED=false"        "standalone: ACME не выключен"

C_NGINX="$(gen_compose nginx panel.example.com you@example.com 8445 443 172.28.0.0/24 false 8446)"
assert_contains "$C_NGINX" "PUBLIC_PORT=443"                "nginx: клиенты видят 443"
assert_contains "$C_NGINX" "ACME_ENABLED=false"             "nginx: встроенный ACME выключен"
assert_contains "$C_NGINX" "TRUSTED_PROXIES=172.28.0.1/32"  "nginx: доверяем шлюзу сети"
assert_contains "$C_NGINX" '"127.0.0.1:8445:8443"'          "nginx: панель только на loopback"
assert_contains "$C_NGINX" '"127.0.0.1:8446:443"'           "nginx: порт под будущий инбаунд"
assert_not_contains "$C_NGINX" '"80:80"'                    "nginx: порт 80 не публикуется"

C_PROXY="$(gen_compose proxy panel.example.com you@example.com 8443 443 172.28.0.0/24 false "")"
assert_contains "$C_PROXY" "ACME_ENABLED=false"             "proxy: TLS у чужого прокси"
assert_contains "$C_PROXY" '"127.0.0.1:8443:8443"'          "proxy: панель только на loopback"

echo
echo "генераторы nginx"
V="$(gen_vhost panel.example.com 8445 "443 ssl http2")"
assert_contains "$V" "$MARKER"                           "vhost помечен маркером"
assert_contains "$V" "listen 443 ssl http2;"             "vhost слушает заданное"
assert_contains "$V" "proxy_pass http://127.0.0.1:8445;" "vhost проксирует в контейнер"
assert_contains "$V" "proxy_set_header Upgrade"          "vhost пробрасывает WebSocket"
assert_contains "$V" "X-Forwarded-Proto"                 "vhost передаёт схему"
assert_contains "$V" "proxy_read_timeout 3600s;"         "vhost не рвёт долгие WebSocket"
assert_contains "$V" "letsencrypt/live/panel.example.com" "vhost ссылается на сертификат"
assert_contains "$V" "location /.well-known/acme-challenge/" "vhost отдаёт каталог для проверки certbot"

VPP="$(gen_vhost panel.example.com 8445 "127.0.0.1:8444 ssl proxy_protocol")"
assert_contains "$VPP" "set_real_ip_from 127.0.0.1;"    "PP-режим: доверяем локальному источнику"
assert_contains "$VPP" "real_ip_header proxy_protocol;" "PP-режим: адрес клиента берётся из PROXY-протокола"

S="$(gen_stream_conf panel.example.com 8444 vpn.example.com 8446 8447)"
assert_contains "$S" "ssl_preread on;"     "stream: чтение SNI включено"
assert_contains "$S" "listen [::]:443;"    "stream: IPv6 не забыт"
assert_contains "$S" "proxy_protocol on;"  "stream: PP включён на промежуточном сервере"
assert_eq "1" "$(echo "$S" | grep -c 'proxy_protocol on;')" "stream: PP ровно на одном сервере"
assert_contains "$(echo "$S" | tr -s ' ')" "panel.example.com 127.0.0.1:8447" "stream: панель идёт через PROXY-протокол"
assert_contains "$(echo "$S" | tr -s ' ')" "vpn.example.com 127.0.0.1:8446"   "stream: инбаунд идёт напрямую"

echo
echo "разведка"
export RB_NGINX_T_CMD="cat $FIXTURES/nginx-busy443.conf"
detect >/dev/null
assert_eq "true" "$HAS_DOCKER"     "docker обнаружен"
assert_eq "true" "$HAS_NGINX_HOST" "nginx на хосте обнаружен"
assert_fails 'sni_offer_allowed' "чужой блок на 443 -> SNI-роутер не предлагается"
export RB_NGINX_T_CMD="cat $FIXTURES/nginx-clean.conf"
assert_eq "0" "$(sni_offer_allowed; echo $?)" "чистый 443 -> SNI-роутер можно предлагать"
export RB_NGINX_T_CMD="true"
assert_fails 'nginx_config_ok' "пустой дамп -> конфигурация нечитаема"
export RB_NGINX_T_CMD="cat $FIXTURES/nginx-clean.conf"
assert_eq "203.0.113.10" "$(resolve_ip panel.example.com)" "DNS-резолв читается из getent"
unset RB_NGINX_T_CMD

echo
echo "повторный запуск"
D="$(mktemp -d)"; mkdir -p "$D/x"
assert_fails "existing_install $D" "пустой каталог не считается установкой"
printf 'image: %s\n' "$IMAGE" > "$D/x/docker-compose.yml"
assert_eq "0" "$(existing_install "$D/x"; echo $?)" "каталог с нашим compose опознан"
printf 'image: nginx\n' > "$D/x/docker-compose.yml"
assert_fails "existing_install $D/x" "чужой compose не считается нашим"
rm -rf "$D"

echo
echo "dry-run целиком"
WORK="$(mktemp -d)"
# Порядок ответов: каталог, домен, «TLS держит nginx» — да, почта,
# тестовый сертификат — нет, SNI-роутер — нет.
printf '%s\n' "$WORK/rb" "panel.example.com" "y" "you@example.com" "n" "0" "n" > "$WORK/answers"
OUT="$(RB_TTY_IN="$WORK/answers" RB_NGINX_T_CMD="cat $FIXTURES/nginx-clean.conf" \
	PATH="$STUBS:$PATH" bash "$HERE/../docker-install.sh" --dry-run 2>&1 || true)"
assert_contains "$OUT" "PUBLIC_HOST=panel.example.com" "dry-run печатает compose"
assert_contains "$OUT" "PUBLIC_PORT=443"               "dry-run: клиенты видят 443"
assert_contains "$OUT" "ACME_ENABLED=false"            "dry-run: TLS остаётся nginx"
assert_contains "$OUT" "server_name panel.example.com" "dry-run печатает vhost"
assert_not_contains "$OUT" "ssl_preread"               "SNI-роутер не запрошен — его конфига нет"
assert_eq "0" "$(ls -A "$WORK/rb" 2>/dev/null | wc -l)" "dry-run не создал ни одного файла"

# Тот же прогон, но с запрошенным SNI-роутером: каталог, домен, «TLS у nginx»,
# почта, тестовый сертификат — нет, SNI-роутер — да, домен инбаунда.
printf '%s\n' "$WORK/rb" "panel.example.com" "y" "you@example.com" "n" "0" "y" "vpn.example.com" > "$WORK/answers-sni"
OUT_SNI="$(RB_TTY_IN="$WORK/answers-sni" RB_NGINX_T_CMD="cat $FIXTURES/nginx-clean.conf" \
	PATH="$STUBS:$PATH" bash "$HERE/../docker-install.sh" --dry-run 2>&1 || true)"
assert_contains "$OUT_SNI" "ssl_preread on;"            "SNI: stream-конфигурация напечатана"
assert_contains "$OUT_SNI" "listen [::]:443;"           "SNI: IPv6-слушатель на месте"
assert_contains "$OUT_SNI" "proxy_protocol on;"         "SNI: промежуточный сервер с PROXY-протоколом"
assert_contains "$OUT_SNI" "real_ip_header proxy_protocol;" "SNI: vhost берёт адрес клиента из PROXY-протокола"
assert_contains "$OUT_SNI" "vpn.example.com"            "SNI: домен инбаунда попал в map"
assert_contains "$OUT_SNI" ":443\"   # инбаунд"         "SNI: порт под инбаунд проброшен"
assert_contains "$OUT_SNI" "stream { include"           "SNI: строка include для nginx.conf напечатана"

# Домен уже обслуживается чужим блоком: отказ, ни одного файла.
printf '%s\n' "$WORK/rb" "panel.example.com" "y" "you@example.com" "n" > "$WORK/answers-taken"
OUT_TAKEN="$(RB_TTY_IN="$WORK/answers-taken" RB_NGINX_T_CMD="cat $FIXTURES/nginx-domain-taken.conf" \
	PATH="$STUBS:$PATH" bash "$HERE/../docker-install.sh" --dry-run 2>&1)"; CODE_TAKEN=$?
assert_eq "1" "$CODE_TAKEN" "занятый домен -> ненулевой код возврата"
assert_contains "$OUT_TAKEN" "уже обслуживается" "занятый домен -> объяснение, а не молчание"
assert_not_contains "$OUT_TAKEN" "PUBLIC_HOST=" "занятый домен -> compose не печатается"

# Standalone при занятом порту 80: отказ вместо нерабочего конфига.
# Ответы: каталог, домен, «TLS у nginx» — нет, почта, тестовый сертификат — нет.
printf '%s\n' "$WORK/rb" "panel.example.com" "n" "you@example.com" "n" > "$WORK/answers-80"
OUT_80="$(RB_TTY_IN="$WORK/answers-80" RB_NGINX_T_CMD="cat $FIXTURES/nginx-clean.conf" \
	PATH="$STUBS:$PATH" bash "$HERE/../docker-install.sh" --dry-run 2>&1)"; CODE_80=$?
assert_eq "1" "$CODE_80" "занятый 80 в standalone -> ненулевой код возврата"
assert_contains "$OUT_80" "порт 80 занят" "занятый 80 -> названа причина"
assert_not_contains "$OUT_80" "PUBLIC_HOST=" "занятый 80 -> compose не печатается"

# Сценарий без nginx: панель держит TLS сама. Нужен отдельный набор стабов —
# общий ss держит 80 занятым, и standalone-путь в нём не доходит до конца.
BARE="$(mktemp -d)"
cat >"$BARE/ss" <<'EOF'
#!/bin/bash
echo 'LISTEN 0 128 0.0.0.0:22 0.0.0.0:* users:(("sshd",pid=1,fd=3))'
EOF
cat >"$BARE/docker" <<'EOF'
#!/bin/bash
case "$1" in
	ps)      echo '' ;;
	network) echo '10.0.0.0/24' ;;
	compose) exit 0 ;;
esac
exit 0
EOF
printf '#!/bin/bash\necho "203.0.113.10"\n' > "$BARE/curl"
printf '#!/bin/bash\necho "203.0.113.10 STREAM panel.example.com"\n' > "$BARE/getent"
chmod +x "$BARE"/*
# Ответы: каталог, «из коробки» — нет, домен, почта, тестовый сертификат — да,
# порт панели (дефолт). На этом стенде 80 и 443 свободны, поэтому новый режим
# предлагается и от него надо явно отказаться.
printf '%s\n' "$WORK/rb" "n" "panel.example.com" "you@example.com" "y" "0" "" > "$WORK/answers-bare"
OUT_BARE="$(RB_TTY_IN="$WORK/answers-bare" RB_NGINX_T_CMD="true" \
	PATH="$BARE:$PATH" bash "$HERE/../docker-install.sh" --dry-run 2>&1 || true)"
assert_contains "$OUT_BARE" "ACME_EMAIL=you@example.com" "без nginx: панель выпускает сертификат сама"
assert_contains "$OUT_BARE" "ACME_STAGING=true"          "без nginx: выбран тестовый CA"
assert_contains "$OUT_BARE" '"8443:8443"'                "без nginx: панель проброшена наружу"
assert_contains "$OUT_BARE" '"80:80"'                    "без nginx: порт 80 под HTTP-01"
assert_contains "$OUT_BARE" "PUBLIC_PORT=8443"           "без nginx: PUBLIC_PORT равен внешнему порту"
assert_not_contains "$OUT_BARE" "TRUSTED_PROXIES"        "без nginx: доверенных прокси нет"
assert_not_contains "$OUT_BARE" "server_name"            "без nginx: vhost не печатается"
rm -rf "$BARE" "$WORK"

echo
echo "режим «из коробки»: флаги"
parse_args --allinone
assert_eq "allinone" "$MODE_FLAG" "--allinone выбирает четвёртый режим"
parse_args --domain panel.example.com --email you@example.com --dir /opt/rb --staging
assert_eq "panel.example.com" "$ARG_DOMAIN"  "--domain разобран"
assert_eq "you@example.com"   "$ARG_EMAIL"   "--email разобран"
assert_eq "/opt/rb"           "$ARG_DIR"     "--dir разобран"
assert_eq "true"              "$ARG_STAGING" "--staging разобран"
assert_eq ""                  "$MODE_FLAG"   "режим сбрасывается между разборами"
parse_args
assert_eq ""        "$ARG_DOMAIN" "флаги сбрасываются между разборами"
assert_fails 'parse_args --domain'          "--domain без значения отвергается"
assert_fails 'parse_args --email'           "--email без значения отвергается"
parse_args --dry-run --allinone --domain panel.example.com
assert_eq "dry-run"   "$ACTION"    "--dry-run сочетается с --allinone"
assert_eq "allinone"  "$MODE_FLAG" "режим при этом сохраняется"

echo
echo "режим «из коробки»: занятость портов"
# Внешний порт нужен и по TCP, и по UDP: фронт держит 443/tcp, mieru — 443/udp.
cat >"$STUBS/ss" <<'EOF'
#!/bin/bash
case "$*" in
	*u*) echo 'UNCONN 0 0 0.0.0.0:443 0.0.0.0:* users:(("mieru",pid=9,fd=3))' ;;
	*)   echo 'LISTEN 0 511 0.0.0.0:8443 0.0.0.0:* users:(("nginx",pid=1,fd=6))' ;;
esac
EOF
chmod +x "$STUBS/ss"
assert_contains "$(port_owner 443 udp)" "mieru" "владелец 443/udp виден"
assert_fails 'port_owner 443'                   "443/tcp при этом свободен"
assert_fails 'port_owner 8443 udp'              "занятый TCP-порт не считается занятым по UDP"

# --- стенд под режим «из коробки»: 80 и 443 свободны, docker есть -----------
OOB="$(mktemp -d)"
oob_stubs() {   # oob_stubs SS_BODY
	cat >"$OOB/ss" <<EOF
#!/bin/bash
$1
EOF
	cat >"$OOB/docker" <<'EOF'
#!/bin/bash
case "$1" in
	ps)      echo '' ;;
	network) echo '10.0.0.0/24' ;;
	compose) exit 0 ;;
esac
exit 0
EOF
	printf '#!/bin/bash\necho "203.0.113.10"\n' > "$OOB/curl"
	printf '#!/bin/bash\necho "203.0.113.10 STREAM panel.example.com"\n' > "$OOB/getent"
	printf '#!/bin/bash\nexit 1\n' > "$OOB/nginx"
	printf '#!/bin/bash\nexit 1\n' > "$OOB/systemctl"
	chmod +x "$OOB"/*
}
oob_run() {     # oob_run ARGS... -> вывод на stdout, код возврата установщика
	local out rc
	out="$(RB_TTY_IN="$OOB/answers" RB_NGINX_T_CMD="true" PATH="$OOB:$PATH" \
		bash "$HERE/../docker-install.sh" "$@" 2>&1)"; rc=$?
	printf '%s\n' "$out"
	return "$rc"
}
FREE_SS='echo "LISTEN 0 128 0.0.0.0:22 0.0.0.0:* users:((\"sshd\",pid=1,fd=3))"'

echo
echo "режим «из коробки»: предварительный показ"
oob_stubs "$FREE_SS"
: > "$OOB/answers"   # ни одного ответа: всё пришло флагами
OUT_OOB="$(oob_run --dry-run --allinone --domain panel.example.com \
	--email you@example.com --dir "$OOB/rb")"; OOB_CODE=$?
assert_eq "0" "$OOB_CODE"                        "предварительный показ проходит"
assert_contains "$OUT_OOB" "из коробки"          "показ называет режим"
assert_contains "$OUT_OOB" "panel.example.com"   "показ называет домен"
assert_contains "$OUT_OOB" "443/udp"             "показ называет внешний порт по UDP"
assert_contains "$OUT_OOB" "$OOB/rb"             "показ называет каталог установки"
assert_not_contains "$OUT_OOB" "Домен панели"    "с флагами вопросы не задаются"
assert_not_contains "$OUT_OOB" "Контакт для"     "почта из флага не переспрашивается"
assert_eq "0" "$(ls -A "$OOB/rb" 2>/dev/null | wc -l)" "показ не создал ни одного файла"

echo
echo "режим «из коробки»: предусловия"
oob_stubs 'echo "LISTEN 0 511 0.0.0.0:443 0.0.0.0:* users:((\"caddy\",pid=7,fd=5))"'
OUT_443="$(oob_run --dry-run --allinone --domain panel.example.com --email you@example.com --dir "$OOB/rb")"; OOB_CODE=$?
assert_eq "1" "$OOB_CODE"                   "занятый 443/tcp -> остановка"
assert_contains "$OUT_443" "443/tcp"        "занятый 443/tcp -> назван порт"
assert_contains "$OUT_443" "caddy"          "занятый 443/tcp -> назван владелец"
assert_not_contains "$OUT_443" "из коробки: что будет сделано" "занятый порт -> плана нет"

oob_stubs 'case "$*" in *u*) echo "UNCONN 0 0 0.0.0.0:443 0.0.0.0:* users:((\"mieru\",pid=9,fd=3))" ;; esac'
OUT_UDP="$(oob_run --dry-run --allinone --domain panel.example.com --email you@example.com --dir "$OOB/rb")"; OOB_CODE=$?
assert_eq "1" "$OOB_CODE"              "занятый 443/udp -> остановка"
assert_contains "$OUT_UDP" "443/udp"   "занятый 443/udp -> назван порт"
assert_contains "$OUT_UDP" "mieru"     "занятый 443/udp -> назван владелец"

oob_stubs 'echo "LISTEN 0 511 0.0.0.0:80 0.0.0.0:* users:((\"nginx\",pid=1,fd=7))"'
OUT_80="$(oob_run --dry-run --allinone --domain panel.example.com --email you@example.com --dir "$OOB/rb")"; OOB_CODE=$?
assert_eq "1" "$OOB_CODE"            "занятый 80 -> остановка: сертификат выпустить нечем"
assert_contains "$OUT_80" "80/tcp"   "занятый 80 -> назван порт"

# Домен, указывающий не сюда, останавливает установку — без «всё равно
# продолжить?»: в этом режиме сертификат выпускает dest, и промах A-записи
# означает сервер, который не поднимется.
oob_stubs "$FREE_SS"
printf '#!/bin/bash\necho "198.51.100.7 STREAM panel.example.com"\n' > "$OOB/getent"
chmod +x "$OOB/getent"
OUT_DNS="$(oob_run --dry-run --allinone --domain panel.example.com --email you@example.com --dir "$OOB/rb")"; OOB_CODE=$?
assert_eq "1" "$OOB_CODE"                     "домен ведёт не сюда -> остановка"
assert_contains "$OUT_DNS" "198.51.100.7"     "названо, куда ведёт домен"
assert_contains "$OUT_DNS" "203.0.113.10"     "названо, где сервер"
assert_not_contains "$OUT_DNS" "Всё равно продолжить" "подтверждения не спрашиваем"

echo
echo "режим «из коробки»: интерактивный выбор"
oob_stubs "$FREE_SS"
# Ответы: каталог, «из коробки» — да, домен, почта.
printf '%s\n' "$OOB/rb" "y" "panel.example.com" "you@example.com" > "$OOB/answers"
OUT_ASK="$(oob_run --dry-run)"; OOB_CODE=$?
assert_eq "0" "$OOB_CODE"                     "интерактивный выбор проходит"
assert_contains "$OUT_ASK" "из коробки"       "режим предложен и выбран"
assert_contains "$OUT_ASK" "panel.example.com" "домен спрошен"

# Отказ от нового режима возвращает на прежнюю дорогу: панель держит TLS сама.
# Ответы: каталог, «из коробки» — нет, домен, почта, тестовый CA — нет, порт.
printf '%s\n' "$OOB/rb" "n" "panel.example.com" "you@example.com" "n" "0" "" > "$OOB/answers"
OUT_NO="$(oob_run --dry-run)"; OOB_CODE=$?
assert_eq "0" "$OOB_CODE"                          "отказ от нового режима не ломает прежний путь"
assert_contains "$OUT_NO" "ACME_EMAIL=you@example.com" "отказ -> прежний standalone-режим"
assert_not_contains "$OUT_NO" "из коробки: что будет сделано" "отказ -> плана нового режима нет"

# Когда 443 занят, новый режим не предлагается вовсе: предлагать невозможное
# значит просить оператора отвечать на вопрос, у которого нет годного ответа.
# Установка при этом уходит в режим «за чужим прокси» (на 443 сидит caddy), и
# что именно она там доспросит — этой проверки не касается: она ровно о том,
# что вопроса про «из коробки» не прозвучало.
oob_stubs 'echo "LISTEN 0 511 0.0.0.0:443 0.0.0.0:* users:((\"caddy\",pid=7,fd=5))"'
printf '%s\n' "$OOB/rb" "panel.example.com" "you@example.com" "n" "" > "$OOB/answers"
OUT_BUSY="$(oob_run --dry-run)"; OOB_CODE=$?
assert_not_contains "$OUT_BUSY" "из коробки" "занятый 443 -> режим не предлагается"
# Домен с несколькими A-записями: адрес сервера не первый, но он в списке —
# это правильный домен, и останавливать установку не за что.
oob_stubs "$FREE_SS"
printf '#!/bin/bash\necho "198.51.100.7 STREAM panel.example.com"\necho "203.0.113.10 STREAM panel.example.com"\n' > "$OOB/getent"
chmod +x "$OOB/getent"
OUT_MULTI="$(oob_run --dry-run --allinone --domain panel.example.com --email you@example.com --dir "$OOB/rb")"; OOB_CODE=$?
assert_eq "0" "$OOB_CODE"                    "адрес сервера среди нескольких A-записей -> установка идёт"
assert_contains "$OUT_MULTI" "DNS OK"        "совпадение названо вслух"

# Без ss занятость хостовых сокетов не видна: это отказ, а не «свободно».
# Проверяется на самих функциях: подсунуть скрипту PATH вообще без ss нельзя,
# не отобрав у него заодно awk и mktemp.
NOSS="$(mktemp -d)"
cp "$OOB/docker" "$NOSS/docker"
HAS_DOCKER="true"
assert_fails '(PATH="$NOSS"; allinone_possible)' "нет ss -> режим не предлагается"
OUT_NOSS="$( (PATH="$NOSS"; ask_allinone) 2>&1 )"; NOSS_CODE=$?
assert_eq "1" "$NOSS_CODE"             "явный --allinone без ss -> отказ"
assert_contains "$OUT_NOSS" "iproute2" "названо, чего не хватает"
rm -rf "$NOSS"
rm -rf "$OOB"

echo
echo "compose режима «из коробки»"
C_OOB="$(gen_compose_allinone panel.example.com you@example.com 172.30.0.0/24 false)"
assert_contains "$C_OOB" 'PUBLIC_HOST=panel.example.com' "домен уезжает в контейнер"
assert_contains "$C_OOB" 'PUBLIC_PORT=443'               "клиенты видят 443"
assert_contains "$C_OOB" 'BOOTSTRAP_ALLINONE=1'          "режим включается явно"
assert_contains "$C_OOB" 'ACME_ENABLED=false'            "свой ACME панели выключен"
assert_contains "$C_OOB" 'ACME_EMAIL=you@example.com'    "почта уезжает в контейнер"
assert_contains "$C_OOB" 'TRUSTED_PROXIES=127.0.0.1/32'  "панель доверяет dest на общей петле"
assert_contains "$C_OOB" '"443:443/tcp"'                 "наружу 443 по TCP"
assert_contains "$C_OOB" '"443:443/udp"'                 "наружу 443 по UDP"
assert_contains "$C_OOB" '"80:80"'                       "80 под HTTP-01"
assert_not_contains "$C_OOB" '8443:'                     "порт панели наружу не опубликован"
assert_not_contains "$C_OOB" '9443:'                     "порт dest наружу не опубликован"
assert_contains "$C_OOB" 'network_mode: "service:routebox"' "dest живёт в сетевой области панели"
assert_contains "$C_OOB" '/config/bin/dest run --config /config/Caddyfile' "dest запускается с планом"
assert_contains "$C_OOB" 'until [ -s /config/Caddyfile ]' "dest дожидается первого старта панели"
assert_contains "$C_OOB" 'XDG_DATA_HOME=/config/dest'    "сертификаты dest переживают пересоздание"
assert_eq "2" "$(echo "$C_OOB" | grep -c 'restart: unless-stopped')" "обе службы поднимаются после перезагрузки"
assert_not_contains "$C_OOB" "ACME_STAGING"              "боевой CA по умолчанию"
assert_contains "$(gen_compose_allinone panel.example.com you@example.com 172.30.0.0/24 true)" \
	"ACME_STAGING=true"                                  "тестовый CA доезжает до контейнера"

echo
echo "сверка контрольных сумм"
REL="$(mktemp -d)"; DL="$(mktemp -d)"
echo "содержимое-бинаря" > "$REL/artifact"
sha256sum "$REL/artifact" | awk '{print $1}' > "$REL/artifact.sha256"
echo "подделка" > "$REL/tampered"
echo "0000000000000000000000000000000000000000000000000000000000000000" > "$REL/tampered.sha256"
echo "без-суммы" > "$REL/nosum"
cat >"$STUBS/curl" <<EOF
#!/bin/bash
# Понимает ровно то, чем пользуется скрипт: -o DEST URL, и запрос своего адреса.
dest=""; url=""
while [ \$# -gt 0 ]; do
	case "\$1" in
		-o) shift; dest="\$1" ;;
		-*) ;;
		*)  url="\$1" ;;
	esac
	shift
done
if [ -z "\$dest" ]; then echo "203.0.113.10"; exit 0; fi
src="$REL/\$(basename "\$url")"
[ -f "\$src" ] || exit 1
cp "\$src" "\$dest"
EOF
chmod +x "$STUBS/curl"
OUT_SUM="$( (fetch_verified "file://x/artifact" "$DL/artifact") 2>&1 )"
assert_contains "$OUT_SUM" "sha256 сверен" "сумма сошлась — артефакт принят"
assert_eq "содержимое-бинаря" "$(cat "$DL/artifact")" "скачано именно то, что лежало в релизе"
OUT_BAD="$( (fetch_verified "file://x/tampered" "$DL/tampered") 2>&1 )"
assert_fails '(fetch_verified "file://x/tampered" "$DL/tampered")' "несовпадение суммы прерывает установку"
assert_contains "$OUT_BAD" "не сошлась" "несовпадение суммы названо вслух"
assert_fails '(fetch_verified "file://x/nosum" "$DL/nosum")' "отсутствие файла суммы тоже прерывает"
assert_fails '(fetch_verified "file://x/missing" "$DL/missing")' "не скачалось -> отказ"

echo
echo "выбор шаблона заглушки"
ARCH_DIR="$(mktemp -d)"
mkdir -p "$ARCH_DIR/stubs/vaultline" "$ARCH_DIR/stubs/stash" "$ARCH_DIR/stubs/driftbox"
for t in vaultline stash driftbox; do echo "<html>$t</html>" > "$ARCH_DIR/stubs/$t/index.html"; done
ARG_STUB=""
case " vaultline stash driftbox " in
	*" $(pick_stub "$ARCH_DIR/stubs") "*) printf '  ok   %s\n' "случайный выбор даёт шаблон из архива" ;;
	*) printf '  FAIL %s\n' "случайный выбор даёт шаблон из архива"; FAILS=$((FAILS+1)) ;;
esac
ARG_STUB="stash"
assert_eq "stash" "$(pick_stub "$ARCH_DIR/stubs")" "флаг задаёт шаблон явно"
ARG_STUB="нетакого"
assert_fails '(pick_stub "$ARCH_DIR/stubs")' "неизвестный шаблон -> отказ"
ARG_STUB=""

echo
echo "доставка артефактов"
# Собираем «релиз» так, как его публикует CI: бинарь под обе архитектуры и
# архив шаблонов с каталогом stubs/ внутри.
for a in amd64 arm64; do
	echo "dest-$a" > "$REL/routebox-dest-linux-$a"
	sha256sum "$REL/routebox-dest-linux-$a" | awk '{print $1}' > "$REL/routebox-dest-linux-$a.sha256"
done
tar czf "$REL/routebox-stubs.tar.gz" -C "$ARCH_DIR" stubs
sha256sum "$REL/routebox-stubs.tar.gz" | awk '{print $1}' > "$REL/routebox-stubs.tar.gz.sha256"
INSTALL_DIR="$(mktemp -d)/rb"
ARG_STUB="driftbox"
OUT_ART="$(RELEASE_BASE="https://example.invalid/releases" install_dest_and_stub 2>&1)"
assert_contains "$OUT_ART" "Заглушка: driftbox" "выбранный шаблон виден оператору"
assert_eq "0" "$(test -x "${INSTALL_DIR}/config/bin/dest"; echo $?)" "бинарь dest на месте и исполняемый"
assert_contains "$(cat "${INSTALL_DIR}/config/stub/index.html")" "driftbox" "файлы выбранной заглушки распакованы"
assert_eq "" "$(ls "${INSTALL_DIR}/config/stub" | grep -v index.html || true)" "в корне заглушки только её файлы"
# Подменённый архив не должен доехать до тома.
echo "мусор" > "$REL/routebox-stubs.tar.gz"
INSTALL_DIR="$(mktemp -d)/rb2"
assert_fails '(RELEASE_BASE="https://example.invalid/releases" install_dest_and_stub)' \
	"подменённый архив прерывает доставку"
assert_eq "0" "$(ls -A "${INSTALL_DIR}/config/stub" 2>/dev/null | wc -l)" "после отказа заглушка не разложена"
rm -rf "$REL" "$DL" "$ARCH_DIR"
ARG_STUB=""; INSTALL_DIR=""

echo
echo "установка «из коробки» целиком, на стабах"
# Сквозной прогон настоящего do_install: артефакты -> compose -> запуск ->
# адрес панели -> пароль. Всё внешнее подменено, root подделан стабом id.
E2E="$(mktemp -d)"; E2EREL="$E2E/release"; mkdir -p "$E2EREL"
for a in amd64 arm64; do
	echo "dest-$a" > "$E2EREL/routebox-dest-linux-$a"
	sha256sum "$E2EREL/routebox-dest-linux-$a" | awk '{print $1}' > "$E2EREL/routebox-dest-linux-$a.sha256"
done
mkdir -p "$E2E/pack/stubs/vaultline" "$E2E/pack/stubs/stash"
echo "<html>vaultline</html>" > "$E2E/pack/stubs/vaultline/index.html"
echo "<html>stash</html>"     > "$E2E/pack/stubs/stash/index.html"
tar czf "$E2EREL/routebox-stubs.tar.gz" -C "$E2E/pack" stubs
sha256sum "$E2EREL/routebox-stubs.tar.gz" | awk '{print $1}' > "$E2EREL/routebox-stubs.tar.gz.sha256"

E2EBIN="$E2E/bin"; mkdir -p "$E2EBIN"
printf '#!/bin/bash\necho "LISTEN 0 128 0.0.0.0:22 0.0.0.0:* users:((\\"sshd\\",pid=1,fd=3))"\n' > "$E2EBIN/ss"
printf '#!/bin/bash\necho 0\n' > "$E2EBIN/id"
printf '#!/bin/bash\nexit 0\n' > "$E2EBIN/systemctl"
printf '#!/bin/bash\necho "203.0.113.10 STREAM panel.example.com"\n' > "$E2EBIN/getent"
cat >"$E2EBIN/curl" <<EOF
#!/bin/bash
dest=""; url=""
while [ \$# -gt 0 ]; do
	case "\$1" in
		-o) shift; dest="\$1" ;;
		-*) ;;
		*)  url="\$1" ;;
	esac
	shift
done
if [ -z "\$dest" ]; then echo "203.0.113.10"; exit 0; fi
src="$E2EREL/\$(basename "\$url")"
[ -f "\$src" ] || exit 1
cp "\$src" "\$dest"
EOF
# Панель «стартует» только к моменту первого `compose exec`: пароль появляется
# тогда же, когда становится известен адрес. Порядок в do_install обязан это
# выдерживать, иначе оператор остаётся без пароля.
cat >"$E2EBIN/docker" <<'EOF'
#!/bin/bash
case "$1" in
	ps)      echo '' ;;
	network) echo '10.0.0.0/24' ;;
	compose)
		case "$2" in
			exec)
				mkdir -p ./config
				echo "secret-password" > ./config/routebox-initial-password
				echo "https://panel.example.com/deadbeef"
				;;
			*) ;;
		esac
		;;
esac
exit 0
EOF
chmod +x "$E2EBIN"/*

E2E_DIR="$E2E/opt/routebox"
OUT_E2E="$(PATH="$E2EBIN:$PATH" RB_TTY_IN=/dev/null RB_NGINX_T_CMD="true" \
	RB_RELEASE_BASE="https://example.invalid/releases" \
	bash "$HERE/../docker-install.sh" --allinone --domain panel.example.com \
	--email you@example.com --dir "$E2E_DIR" --stub stash 2>&1)"; E2E_CODE=$?
assert_eq "0" "$E2E_CODE"                              "установка целиком проходит"
assert_contains "$OUT_E2E" "Заглушка: stash"           "выбранный шаблон назван"
assert_contains "$OUT_E2E" "Панель: https://panel.example.com/deadbeef" "адрес панели напечатан"
assert_contains "$OUT_E2E" "secret-password"           "пароль напечатан, а не «выдан ранее»"
assert_not_contains "$OUT_E2E" "выдан ранее"           "ложного «пароль выдан ранее» нет"
assert_contains "$OUT_E2E" "Сайт: https://panel.example.com" "адрес сайта назван"
assert_contains "$(cat "$E2E_DIR/docker-compose.yml")" "BOOTSTRAP_ALLINONE=1" "compose записан"
assert_eq "0" "$(test -x "$E2E_DIR/config/bin/dest"; echo $?)" "бинарь dest доставлен"
assert_contains "$(cat "$E2E_DIR/config/stub/index.html")" "stash" "заглушка разложена"
assert_eq "0" "$(ls -A "$E2E_DIR/config/.download" 2>/dev/null | wc -l)" "временный каталог за собой убран"

# Повторный запуск: ничего не переспрашивает и не упирается в собственный 443.
OUT_AGAIN="$(PATH="$E2EBIN:$PATH" RB_TTY_IN=/dev/null RB_NGINX_T_CMD="true" \
	RB_RELEASE_BASE="https://example.invalid/releases" \
	bash "$HERE/../docker-install.sh" --allinone --dir "$E2E_DIR" 2>&1)"; AGAIN_CODE=$?
assert_eq "0" "$AGAIN_CODE"                        "повторный запуск проходит"
assert_contains "$OUT_AGAIN" "обновляю образ"      "повторный запуск уходит в обновление"
assert_contains "$OUT_AGAIN" "Готово"              "обновление доходит до конца"
assert_not_contains "$OUT_AGAIN" "занят"           "свой же 443 не считается помехой"
assert_contains "$(cat "$E2E_DIR/docker-compose.yml")" "BOOTSTRAP_ALLINONE=1" "compose не переписан"

# Реестр может лежать, а образ быть на месте: обновление обязано дойти до конца.
cat >"$E2EBIN/docker" <<'EOF'
#!/bin/bash
case "$1" in
	ps)      echo '' ;;
	network) echo '10.0.0.0/24' ;;
	compose)
		case "$2" in
			pull) echo "pull access denied" >&2; exit 1 ;;
			exec)
				mkdir -p ./config
				echo "secret-password" > ./config/routebox-initial-password
				echo "https://panel.example.com/deadbeef"
				;;
			*) ;;
		esac
		;;
esac
exit 0
EOF
chmod +x "$E2EBIN/docker"
OUT_NOPULL="$(PATH="$E2EBIN:$PATH" RB_TTY_IN=/dev/null RB_NGINX_T_CMD="true" \
	RB_RELEASE_BASE="https://example.invalid/releases" \
	bash "$HERE/../docker-install.sh" --allinone --dir "$E2E_DIR" 2>&1)"; NOPULL_CODE=$?
assert_eq "0" "$NOPULL_CODE"                        "провал pull не обрывает обновление"
assert_contains "$OUT_NOPULL" "поднимаю на том образе" "сказано, что поднимаемся на скачанном"
assert_contains "$OUT_NOPULL" "Готово"              "обновление всё равно доходит до конца"

# Пропавший с тома dest восстанавливается, а не оставляет контейнер в перезапусках.
rm -f "$E2E_DIR/config/bin/dest"
OUT_REPAIR="$(PATH="$E2EBIN:$PATH" RB_TTY_IN=/dev/null RB_NGINX_T_CMD="true" \
	RB_RELEASE_BASE="https://example.invalid/releases" \
	bash "$HERE/../docker-install.sh" --allinone --dir "$E2E_DIR" 2>&1 || true)"
assert_contains "$OUT_REPAIR" "доставляю заново" "пропавший dest замечен"
assert_eq "0" "$(test -x "$E2E_DIR/config/bin/dest"; echo $?)" "и доставлен обратно"

echo
echo "удаление режима «из коробки»"
OUT_RM="$(PATH="$E2EBIN:$PATH" RB_TTY_IN=/dev/null RB_NGINX_T_CMD="true" \
	RB_NGINX_CONF="$E2E/nginx.conf" \
	bash "$HERE/../docker-install.sh" --uninstall --dir "$E2E_DIR" 2>&1)"; RM_CODE=$?
assert_eq "0" "$RM_CODE"                           "удаление проходит"
assert_fails "test -f '$E2E_DIR/docker-compose.yml'" "свой compose снесён"
assert_eq "0" "$(test -f "$E2E_DIR/config/bin/dest"; echo $?)" "данные без --purge остаются"
assert_contains "$OUT_RM" "Данные остались"        "сказано, что данные на месте"

# Чужой compose в том же каталоге удаление не трогает.
printf 'services:\n  other:\n    image: nginx\n' > "$E2E_DIR/docker-compose.yml"
OUT_FOREIGN="$(PATH="$E2EBIN:$PATH" RB_TTY_IN=/dev/null RB_NGINX_T_CMD="true" \
	RB_NGINX_CONF="$E2E/nginx.conf" \
	bash "$HERE/../docker-install.sh" --uninstall --dir "$E2E_DIR" 2>&1)"; FOREIGN_CODE=$?
assert_eq "1" "$FOREIGN_CODE"                        "чужой compose -> отказ"
assert_eq "0" "$(test -f "$E2E_DIR/docker-compose.yml"; echo $?)" "чужой compose на месте"
assert_contains "$OUT_FOREIGN" "чужой docker-compose.yml" "названа причина отказа"
rm -rf "$E2E"

echo
echo "предупреждение о AAAA"
# getent показывает обычную A-запись как ::ffff:… — на домене без AAAA
# предупреждение срабатывать не должно.
NOAAAA="$(mktemp -d)"
cat >"$NOAAAA/getent" <<'EOF'
#!/bin/bash
case "$1" in
	ahostsv6) echo "::ffff:203.0.113.10 STREAM panel.example.com" ;;
	*)        echo "203.0.113.10 STREAM panel.example.com" ;;
esac
EOF
chmod +x "$NOAAAA/getent"
# Под `set -euo pipefail`, как в main: без этого не видно, что пустой вывод
# grep роняет весь скрипт на домене без AAAA.
OUT_V4ONLY="$( (set -euo pipefail; PATH="$NOAAAA:$PATH"; warn_stray_aaaa panel.example.com; echo "дожили") 2>&1 )"
assert_eq "дожили" "$OUT_V4ONLY" "домен без AAAA: ни предупреждения, ни падения"
cat >"$NOAAAA/getent" <<'EOF'
#!/bin/bash
case "$1" in
	ahostsv6) echo "2001:db8::1 STREAM panel.example.com" ;;
	*)        echo "203.0.113.10 STREAM panel.example.com" ;;
esac
EOF
chmod +x "$NOAAAA/getent"
OUT_V6="$( (set -euo pipefail; PATH="$NOAAAA:$PATH"; warn_stray_aaaa panel.example.com) 2>&1 )"
assert_contains "$OUT_V6" "2001:db8::1" "настоящая AAAA-запись названа"
assert_contains "$OUT_V6" "IPv6"        "сказано, почему это важно"
rm -rf "$NOAAAA"

echo
echo "порт AmneziaWG"
# Порт выбирается на установке и попадает и в проброс, и в переменную, из
# которой панель узнаёт, что менять его нельзя.
AWG_PORT="51820"
C_AWG="$(gen_compose_allinone d.example.com a@b.c 172.30.0.0/24 false)"
assert_contains "$C_AWG" '"51820:51820/udp"'   "порт AmneziaWG опубликован"
assert_contains "$C_AWG" "AWG_LISTEN_PORT=51820" "панель узнаёт порт из окружения"
C_AWG_N="$(gen_compose nginx d.example.com a@b.c 8445 443 172.30.0.0/24 false "")"
assert_contains "$C_AWG_N" '"51820:51820/udp"'   "и в остальных режимах тоже"
assert_contains "$C_AWG_N" "AWG_LISTEN_PORT=51820" "и переменная тоже"
AWG_PORT=""
assert_not_contains "$(gen_compose_allinone d.example.com a@b.c 172.30.0.0/24 false)" "AWG_LISTEN_PORT" \
	"без порта переменной нет"
assert_not_contains "$(gen_compose_allinone d.example.com a@b.c 172.30.0.0/24 false)" "/udp\"   # AmneziaWG" \
	"без порта проброса нет"

# Ответ «0» — это отказ, а не порт 0.
ANSP="$(mktemp)"; printf '0\n' > "$ANSP"; exec 3<"$ANSP"
ARG_AWG_PORT=""; AWG_PORT="сторож"
ask_awg_port
assert_eq "" "$AWG_PORT" "ноль означает «не публиковать»"
exec 3<&-; rm -f "$ANSP"

# Занятый UDP-порт отвергается, и спрашивается снова — как и порт панели.
ANSP="$(mktemp)"; printf '443\nне-число\n51820\n' > "$ANSP"; exec 3<"$ANSP"
cat >"$STUBS/ss" <<'EOF'
#!/bin/bash
case "$*" in
	*u*) echo 'UNCONN 0 0 0.0.0.0:443 0.0.0.0:* users:(("mieru",pid=9,fd=3))' ;;
esac
EOF
chmod +x "$STUBS/ss"
AWG_PORT=""
OUT_AWGP="$( (PATH="$STUBS:$PATH"; ask_awg_port; echo "порт=$AWG_PORT") 2>&1 )"
assert_contains "$OUT_AWGP" "занят"        "занятый UDP-порт назван занятым"
assert_contains "$OUT_AWGP" "порт=51820"   "после отказа спрашивается снова"
exec 3<&-; rm -f "$ANSP"

# Флагом — то же самое, но без переспрашивания: занятый порт это остановка.
ARG_AWG_PORT="443"
assert_fails '(PATH="$STUBS:$PATH"; ask_awg_port)' "--awg-port на занятый порт -> отказ"
ARG_AWG_PORT=""; AWG_PORT=""

echo
echo "ядерный модуль AmneziaWG на хосте"
AWGH="$(mktemp -d)"
mkdir -p "$AWGH/bin" "$AWGH/etc"
# os-release семейства Debian и каталог модулей running-ядра — два условия, при
# которых модуль тут вообще можно поставить.
printf 'ID=ubuntu\nID_LIKE=debian\nVERSION_CODENAME=noble\n' > "$AWGH/os-release"
# Переменные читаются при сорсинге, поэтому подменяем их сами, а не через окружение.
AWG_OS_RELEASE="$AWGH/os-release"
assert_eq "0" "$(awg_kernel_possible; echo $?)" "Debian-семейство с каталогом модулей -> можно"
printf 'ID=alpine\n' > "$AWGH/os-release"
assert_fails 'awg_kernel_possible' "не-Debian хост -> нельзя"
printf 'ID=ubuntu\nID_LIKE=debian\n' > "$AWGH/os-release"
assert_fails 'awg_kernel_possible' "без кодового имени выпуска -> нельзя (не из чего собрать suite PPA)"
printf 'ID=ubuntu\nID_LIKE=debian\nVERSION_CODENAME=noble\n' > "$AWGH/os-release"

# Явный --awg-kernel там, где модуль невозможен, — это остановка с причиной, а
# не тихий пропуск: оператор попросил то, чего здесь не будет.
printf 'ID=alpine\n' > "$AWGH/os-release"
ARG_AWG_KERNEL="true"
OUT_IMP="$( (awg_kernel_wanted) 2>&1 )"; IMP_CODE=$?
assert_eq "1" "$IMP_CODE"                       "--awg-kernel на неподходящем хосте -> отказ"
assert_contains "$OUT_IMP" "Debian/Ubuntu"      "названо, какой хост нужен"
assert_contains "$OUT_IMP" "singbox"            "названа рабочая альтернатива"
printf 'ID=ubuntu\nID_LIKE=debian\nVERSION_CODENAME=noble\n' > "$AWGH/os-release"
ARG_AWG_KERNEL="false"
assert_fails 'awg_kernel_wanted' "--no-awg-kernel -> не спрашиваем и не ставим"
ARG_AWG_KERNEL=""

# Установка на стабах: apt, gpg и modprobe подменены, ключ отдаёт правильный
# отпечаток. Проверяем, что дошли до конца и написали источник apt.
cat >"$AWGH/bin/apt-get" <<'EOF'
#!/bin/bash
exit 0
EOF
cat >"$AWGH/bin/gpg" <<EOF
#!/bin/bash
case "\$*" in
	*--fingerprint*) echo "      75C9 DD72 C799 870E 3105  42E2 4166 F2C2 5729 0828" ;;
	*--export*)      out=""; prev=""; for a in "\$@"; do [ "\$prev" = "--output" ] && out="\$a"; prev="\$a"; done; [ -n "\$out" ] && echo key > "\$out" ;;
esac
exit 0
EOF
printf '#!/bin/bash\nexit 0\n' > "$AWGH/bin/modprobe"
chmod +x "$AWGH/bin"/*
RB_AWG_KEYRING="$AWGH/etc/amnezia.gpg"
RB_AWG_SOURCES="$AWGH/etc/amnezia.sources"
AWG_KEYRING="$RB_AWG_KEYRING"; AWG_SOURCES="$RB_AWG_SOURCES"
OUT_MOD="$( (PATH="$AWGH/bin:$PATH"; install_awg_module) 2>&1 )"; MOD_CODE=$?
assert_eq "0" "$MOD_CODE"                            "установка модуля проходит"
assert_contains "$OUT_MOD" "DKMS"                    "сказано, что обновления берёт на себя DKMS"
assert_contains "$(cat "$RB_AWG_SOURCES")" "Suites: noble" "suite PPA взят из выпуска хоста"
assert_contains "$(cat "$RB_AWG_SOURCES")" "Signed-By: $RB_AWG_KEYRING" "источник подписан проверенной связкой"

# Подменённый ключ не должен доехать до доверенного каталога.
rm -f "$RB_AWG_KEYRING"
cat >"$AWGH/bin/gpg" <<'EOF'
#!/bin/bash
case "$*" in
	*--fingerprint*) echo "      DEAD BEEF C799 870E 3105  42E2 4166 F2C2 5729 0828" ;;
esac
exit 0
EOF
chmod +x "$AWGH/bin/gpg"
OUT_BADKEY="$( (PATH="$AWGH/bin:$PATH"; install_awg_module) 2>&1 )"; BADKEY_CODE=$?
assert_eq "1" "$BADKEY_CODE"                     "чужой отпечаток -> модуль не ставится"
assert_contains "$OUT_BADKEY" "подмена"          "названа причина отказа"
assert_fails "test -f '$RB_AWG_KEYRING'"         "чужой ключ не попал в доверенные"

# Привилегия появляется в compose только вместе с установленным модулем.
WANT_CAP_NET_ADMIN="false"
assert_not_contains "$(gen_compose_allinone d.example.com a@b.c 172.30.0.0/24 false)" "NET_ADMIN" \
	"без модуля привилегии в compose нет"
WANT_CAP_NET_ADMIN="true"
assert_contains "$(gen_compose_allinone d.example.com a@b.c 172.30.0.0/24 false)" "- NET_ADMIN" \
	"с модулем привилегия выдаётся"
assert_contains "$(gen_compose nginx d.example.com a@b.c 8445 443 172.30.0.0/24 false "")" "- NET_ADMIN" \
	"и в остальных режимах тоже"
WANT_CAP_NET_ADMIN="false"
rm -rf "$AWGH"
unset RB_AWG_KEYRING RB_AWG_SOURCES
AWG_OS_RELEASE="/etc/os-release"

# Схему compose проверяет сам docker, если он на машине есть: набор ключей у
# compose меняется от версии к версии, и «yaml разобрался» об этом не говорит.
if [ -n "$REAL_DOCKER" ] && "$REAL_DOCKER" compose version >/dev/null 2>&1; then
	CY="$(mktemp -d)"
	gen_compose_allinone panel.example.com you@example.com 172.30.0.0/24 true > "$CY/docker-compose.yml"
	CY_OUT="$("$REAL_DOCKER" compose -f "$CY/docker-compose.yml" config 2>&1)"; CY_CODE=$?
	assert_eq "0" "$CY_CODE" "docker compose принимает сгенерированный файл"
	assert_contains "$CY_OUT" "443" "в разобранном виде внешний порт на месте"
	rm -rf "$CY"
else
	echo "  --   схема compose: пропущено, docker compose недоступен"
fi

# Если nginx на машине есть, сгенерированные конфигурации проверяются им самим.
if command -v nginx >/dev/null 2>&1; then
	echo
	echo "nginx -t на сгенерированном"
	NG="$(mktemp -d)"
	gen_vhost panel.example.com 8445 "443 ssl http2" > "$NG/vhost.conf"
	gen_stream_conf panel.example.com 8444 vpn.example.com 8446 8447 > "$NG/stream.conf"
	cat > "$NG/nginx.conf" <<EOF
events {}
http {
    include ${NG}/vhost.conf;
}
$(cat "$NG/stream.conf")
EOF
	# Сертификата нет, поэтому смотрим только на синтаксические ошибки.
	NG_OUT="$(nginx -t -c "$NG/nginx.conf" -p "$NG" 2>&1 || true)"
	assert_not_contains "$NG_OUT" "unknown directive"  "nginx не знает лишних директив"
	assert_not_contains "$NG_OUT" "unexpected"         "структура блоков верна"
	rm -rf "$NG"
else
	echo
	echo "nginx -t на сгенерированном: пропущено, nginx не установлен"
fi

echo
if [ "$FAILS" -gt 0 ]; then printf '%d проверок упало\n' "$FAILS"; exit 1; fi
printf 'все проверки прошли\n'
