#!/bin/bash
# Тесты docker-install.sh. Запуск: bash tests/docker-install.test.sh
# Ни root, ни docker, ни nginx не требуются: всё внешнее подменяется стабами
# в PATH, а вывод `nginx -T` — переменной RB_NGINX_T_CMD.
HERE="$(cd "$(dirname "$0")" && pwd)"
FIXTURES="$HERE/fixtures"
FAILS=0

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
assert_fails 'parse_args --domain x' "неизвестный флаг отвергается"

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
printf '%s\n' "$WORK/rb" "panel.example.com" "y" "you@example.com" "n" "n" > "$WORK/answers"
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
printf '%s\n' "$WORK/rb" "panel.example.com" "y" "you@example.com" "n" "y" "vpn.example.com" > "$WORK/answers-sni"
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
# Ответы: каталог, домен, почта, тестовый сертификат — да, порт панели (дефолт).
printf '%s\n' "$WORK/rb" "panel.example.com" "you@example.com" "y" "" > "$WORK/answers-bare"
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
