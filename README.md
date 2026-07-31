# RouteBox

[![Release](https://img.shields.io/github/v/release/hoaxisr/routebox)](https://github.com/hoaxisr/routebox/releases/latest)
[![License](https://img.shields.io/badge/license-MIT-blue)](#лицензия)
![Platform](https://img.shields.io/badge/platform-linux%20amd64%20%7C%20arm64-lightgrey)

Веб-панель для [amnezia-box](https://github.com/hoaxisr/amnezia-box) (форк sing-box). Один бинарник со встроенным интерфейсом, два сценария: VPN-роутер для домашней сети и панель управления прокси-сервером на VPS.

![Дашборд: статус amnezia-box, скорость и объём трафика, топ-соединения](assets/01-dashboard.png)

## Два режима

| | Роутер дома | Панель на VPS |
|---|---|---|
| Что делает | Направляет трафик домашней сети через VPN: целиком или только выбранные сайты | Управляет VPN-сервером: протоколы, пользователи, подписки |
| Установка | [`install.sh`](https://github.com/hoaxisr/routebox/blob/main/install.sh) | [`vps-install.sh`](https://github.com/hoaxisr/routebox/blob/main/vps-install.sh) |
| Доступ | `http://IP-роутера:8080` | `https://домен:8443`, TLS-сертификат Let's Encrypt выпускается автоматически |
| Авторизация | Опциональна | Обязательна, пароль администратора генерируется при установке |
| Что подключается | AmneziaWG / WireGuard, VLESS, Hysteria2, NaiveProxy, Mieru — по ссылке или файлу | Клиенты пользователей — по ссылке-подписке, QR-коду или файлу конфигурации |

Режим задаётся ключом `mode` в настройках; интерфейс показывает только разделы своего режима.

## Возможности

**Маршрутизация**
- Готовые списки: Antizapret, geosite/geoip rule-sets; свои списки доменов
- Приоритет правил перетаскиванием, продвинутые правила по домену, IP, порту, процессу
- Настройка DNS-серверов и DNS-правил
- Route Inspector: проверка, каким правилом и куда уйдёт запрос

**Подключения (клиентская сторона)**
- Эндпоинты AmneziaWG (включая обфускацию AWG 1.0/2.0/3.0) и WireGuard — импорт из `.conf`
- Аутбаунды VLESS, Hysteria2, NaiveProxy, Mieru — импорт по ссылкам `vless://`, `hy2://`, `naive+https://`, `mierus://`
- Группы selector и urltest, подписки с автообновлением

**Сервер (режим VPS)**
- Инбаунды VLESS (в том числе Reality), Hysteria2, NaiveProxy, Mieru; транспорты raw / ws / grpc / httpupgrade / xhttp
- Сервер AmneziaWG (включая версию AWG 3.0) без модуля ядра, с обфускацией и профилями маскировки; выдача клиентам QR-кода, `.conf` и sing-box JSON
- Пользователи: ссылки-подписки, сроки действия, включение и отключение, учёт трафика

**Мониторинг**
- Трафик в реальном времени с историей, разбивка по исходящим узлам
- Активные соединения с флагами стран (GeoIP), цепочкой узлов и объёмами
- Логи amnezia-box и журнал systemd

**Обслуживание**
- Проверка и установка обновлений RouteBox и amnezia-box из панели
- Валидация конфигурации перед применением, черновики и откат изменений

## Быстрый старт: роутер дома

Требования: Linux (amd64 или arm64) с systemd, права root.

```bash
curl -fsSL https://raw.githubusercontent.com/hoaxisr/routebox/main/install.sh | sudo bash
```

Скрипт скачивает последние релизы RouteBox и amnezia-box, кладёт настройки в `/etc/routebox/`, скачивает базу GeoIP, включает IP-forwarding и создаёт systemd-сервисы `routebox` и `amnezia-box`.

Дальше:

```bash
sudo systemctl start routebox
```

1. Откройте `http://IP-устройства:8080`.
2. Добавьте подключение: вставьте ссылку или файл конфигурации вашего VPN в разделе Endpoints или Outbounds.
3. В разделе Routes выберите, что маршрутизировать через VPN — весь трафик или списки сайтов.
4. На устройствах домашней сети укажите IP роутера как шлюз (или раздайте его по DHCP).

![Маршрутизация: rule-sets с действиями proxy, block, direct и приоритетом перетаскиванием](assets/02-routes.png)

![Активные соединения: страна, цепочка через выбранный узел, объём и время жизни](assets/03-connections.png)

![Монитор трафика: история за 60 секунд и разбивка по исходящим узлам](assets/04-traffic.png)

## Быстрый старт: панель на VPS

Требования:

- VPS на Linux (amd64 или arm64) с systemd, права root.
- **Домен с A/AAAA-записью на IP сервера** — обязателен: по нему встроенный ACME выпускает TLS-сертификат панели. По «голому» IP панель не откроется (TLS-сертификату нужен SNI домена).
- Порты `8443/tcp` (панель) и `80/tcp` (HTTP-01 проверка Let's Encrypt; должен оставаться открытым — продления идут по нему же). Установщик откроет их в ufw, если он активен. Порты 443 и 22 скрипт не трогает.

Первый запуск лучше делать с флагом `--staging` — он использует тестовый каталог Let's Encrypt и не расходует лимит выпуска сертификатов:

```bash
curl -fsSL https://raw.githubusercontent.com/hoaxisr/routebox/main/vps-install.sh \
  | sudo bash -s -- --domain panel.example.com --email you@example.com --staging
```

Когда всё работает, повторный запуск без `--staging` переключает панель на боевой сертификат:

```bash
curl -fsSL https://raw.githubusercontent.com/hoaxisr/routebox/main/vps-install.sh \
  | sudo bash -s -- --domain panel.example.com --email you@example.com
```

Флаги:

- `--domain` — домен панели (без него установщик спросит интерактивно);
- `--email` — контакт для Let's Encrypt;
- `--port <n>` — порт панели (по умолчанию `8443`);
- `--staging` — тестовый CA Let's Encrypt.

Скрипт проверяет DNS, ставит оба бинарника с проверкой sha256, настраивает systemd и firewall. Сертификат RouteBox выпускает и продлевает сам — nginx и certbot не нужны.

Панель доступна на `https://panel.example.com:8443` — открывайте по домену, не по IP. Логин `admin`, пароль генерируется при первом старте и сохраняется в `/etc/routebox/routebox-initial-password` (также виден в `journalctl -u routebox`). Смените его после входа в разделе App Settings → Security.

![Сервер AmneziaWG: клиенты, выдача QR-кода, .conf и JSON, сроки действия](assets/05-awg.png)

![Пользователи панели: состояние, срок действия, ссылка-подписка и учёт трафика](assets/06-users.png)

## Docker (панель на VPS)

Образ собран на [баз-имидже LinuxServer.io](https://docs.linuxserver.io/general/containers-101/) — только режим панели на VPS; режим роутера в Docker не поддерживается (ему нужен TUN и роль шлюза локальной сети — используйте `install.sh` на голом железе).

```bash
curl -fsSLO https://raw.githubusercontent.com/hoaxisr/routebox/main/docker-compose.yml
# отредактируйте PUBLIC_HOST и ACME_EMAIL в docker-compose.yml
docker compose up -d
```

- `/config` — единственный volume: `routebox.toml`, конфиг amnezia-box, GeoIP-база, `traffic.db`, ACME-кэш, данные сервера AmneziaWG. Всё, что панель обычно хранит в `/etc/routebox` и `/etc/amnezia/amneziawg`, живёт здесь через символические ссылки внутри контейнера.
- При первом запуске без `routebox.toml` в `/config` контейнер создаёт минимальный конфиг из переменных окружения `PUBLIC_HOST`, `ACME_EMAIL`, `ACME_STAGING` — аналог флагов `vps-install.sh`. Пароль администратора панель генерирует сама при первом старте и пишет в `/config/routebox-initial-password` (виден также в `docker compose logs routebox`).
- Порт `8443` (панель) и `80` (ACME HTTP-01, должен оставаться открытым для продлений) публикуются по умолчанию. Инбаунды amnezia-box настраиваются в панели и публикуются по отдельности через `ports:`; контейнер работает без root, поэтому для порта <1024 внутри контейнера либо перенаправьте его на непривилегированный (`"443:8444"`), либо добавьте `cap_add: [NET_BIND_SERVICE]` — оба варианта закомментированы в `docker-compose.yml`.
- `PUID`/`PGID` — как в любом образе LinuxServer.io, задают владельца файлов в `/config`.

Обновление образа:

```bash
docker compose pull && docker compose up -d
```

Раздел Updates в панели по-прежнему проверяет новые версии RouteBox и amnezia-box: обновление amnezia-box ставится прямо из панели, а для самого RouteBox панель показывает эту же команду вместо кнопки — образ является источником истины для собственного бинарника.

## Обновление

В обоих режимах: раздел Updates в панели проверяет и ставит новые версии RouteBox и amnezia-box. Список изменений — в [CHANGELOG.md](CHANGELOG.md).

Из консоли:

```bash
# Роутер: повторный запуск установщика обновляет бинарники, настройки не трогает
curl -fsSL https://raw.githubusercontent.com/hoaxisr/routebox/main/install.sh | sudo bash

# VPS: конфигурация и сертификат сохраняются
sudo bash vps-install.sh --update
```

## Удаление

```bash
# Роутер (каталоги настроек остаются)
curl -fsSL https://raw.githubusercontent.com/hoaxisr/routebox/main/install.sh | sudo bash -s -- --uninstall

# VPS; --purge дополнительно удаляет /etc/routebox и /etc/amnezia-box
sudo bash vps-install.sh --uninstall
```

<details>
<summary><b>Конфигурация: routebox.toml и GeoIP</b></summary>

Настройки приложения хранятся в `/etc/routebox/routebox.toml` ([пример с комментариями](https://github.com/hoaxisr/routebox/blob/main/routebox.toml)). Большинство параметров также доступно в панели в разделе App Settings.

| Секция | Описание |
|--------|----------|
| `[geoip]` | Путь к MMDB-базе, включение GeoIP |
| `[ui]` | Тема, язык (en/ru), единицы скорости, формат времени |
| `[monitoring]` | GeoIP-обогащение соединений, история закрытых соединений, интервалы опроса |
| `[security]` | Авторизация панели, CORS, таймаут сессии |
| `[network]` | Адрес и порт панели, таймауты, сжатие, TLS (свой сертификат или встроенный ACME/Let's Encrypt) |
| `[server]` | Режим (`router`/`vps`), публичный хост и порт для ссылок-подписок |
| `[singbox]` | Путь к конфигу amnezia-box, адрес Clash API, имя systemd-сервиса, адрес v2ray-статистики. Путь к бинарнику не настраивается: он ищется в `/usr/local/bin`, `/usr/bin`, `/opt/amnezia-box` и `PATH` |
| `[updates]` | Ежедневная автопроверка обновлений |
| `[awg]` | Сервер AmneziaWG: интерфейс, подсеть, порт, DNS, бэкенд, обфускация |
| `[advanced]` | Интервалы WebSocket-пинга |

### GeoIP

Установщик скачивает базу [IPLocate ip-to-country](https://github.com/iplocate/ip-address-databases) в `/etc/routebox/geoip.mmdb` — флаги стран в мониторе соединений работают сразу. Ручное обновление базы:

```bash
sudo curl -fsSL -o /etc/routebox/geoip.mmdb \
  https://github.com/iplocate/ip-address-databases/raw/main/ip-to-country/ip-to-country.mmdb
sudo systemctl restart routebox
```

Обновление можно повесить на cron (с перезапуском сервиса). Подходит любая MMDB-база (IPInfo, MaxMind GeoLite2) — укажите путь в `path`.

</details>

<details>
<summary><b>Параметры запуска и приоритет настроек</b></summary>

```
routebox [флаги]
routebox version      # версия

--settings PATH   Путь к routebox.toml (по умолчанию: авто-поиск)
--config PATH     Путь к конфигу amnezia-box (переопределяет settings)
--listen ADDR     Адрес веб-интерфейса (переопределяет settings)
--geoip PATH      Путь к GeoIP MMDB (переопределяет settings)
--clash ADDR      Адрес Clash API (по умолчанию берётся из конфига amnezia-box)
--mode MODE       Режим панели: router (по умолчанию) или vps
```

Приоритет: флаги командной строки → `routebox.toml` → авто-определение → значения по умолчанию.

RouteBox требует запуска от root — он создаёт TUN-интерфейсы и управляет системными сервисами.

</details>

<details>
<summary><b>REST API</b></summary>

Все эндпоинты, кроме `/api/health` и `/api/auth/*`, требуют сессию, если авторизация включена. Основные:

| Endpoint | Описание |
|----------|----------|
| `GET /api/status` | Статус процесса amnezia-box |
| `GET` / `PUT /api/config` | Полный конфиг sing-box |
| `POST /api/config/apply` | Применить изменения (reload/restart) |
| `POST /api/config/fix-unit` | Перенацелить systemd-юнит на конфиг, которым управляет RouteBox (drop-in) |
| `DELETE /api/config/unit-dropin` | Снять этот drop-in — обратная операция к `fix-unit` |
| `POST /api/config/adopt-unit-path` | Перейти на путь конфига из `ExecStart` юнита (второе лечение расхождения) |
| CRUD `/api/endpoints`, `/api/outbounds`, `/api/inbounds` | Подключения |
| CRUD `/api/route/rules`, `/api/route/rule-sets` | Маршрутизация |
| CRUD `/api/dns/servers`, `/api/dns/rules` | DNS |
| CRUD `/api/users` | Пользователи панели (режим vps) |
| `/api/awg/*` | Сервер AmneziaWG: статус, пиры, конфиги |
| `GET` / `PUT /api/settings` | Настройки RouteBox |
| `GET /api/clash/*` | Прокси к Clash API |
| WS `/api/clash/traffic`, `/api/clash/connections`, `/api/clash/logs` | Данные в реальном времени |
| `GET /sub/{token}` | Публичная ссылка-подписка пользователя |

</details>

<details>
<summary><b>Ручная установка и systemd</b></summary>

Бинарники публикуются в [GitHub Releases](https://github.com/hoaxisr/routebox/releases):

```bash
# amd64; для arm64 замените суффикс
curl -fsSL -o routebox \
  https://github.com/hoaxisr/routebox/releases/latest/download/routebox-linux-amd64
curl -fsSL -O \
  https://github.com/hoaxisr/routebox/releases/latest/download/routebox-linux-amd64.sha256
sha256sum -c routebox-linux-amd64.sha256 --ignore-missing || echo "checksum mismatch"
chmod +x routebox

sudo cp routebox /usr/local/bin/
sudo mkdir -p /etc/routebox /etc/amnezia-box
```

Также понадобится бинарник amnezia-box из [релизов форка](https://github.com/hoaxisr/amnezia-box/releases) в `/usr/local/bin/amnezia-box`.

Systemd-сервис:

```bash
sudo tee /etc/systemd/system/routebox.service << 'EOF'
[Unit]
Description=RouteBox - VPN Router Web UI
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/routebox --settings /etc/routebox/routebox.toml
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now routebox
```

Для работы в режиме роутера включите IP-forwarding:

```bash
sudo sysctl -w net.ipv4.ip_forward=1
echo "net.ipv4.ip_forward=1" | sudo tee -a /etc/sysctl.conf
```

</details>

<details>
<summary><b>Сборка из исходников</b></summary>

Понадобятся Go 1.21+ и Node.js с npm.

```bash
git clone https://github.com/hoaxisr/routebox
cd routebox

# Фронтенд (SvelteKit -> статика, встраивается в бинарник)
cd frontend && npm install && npm run build && cd ..
rm -rf backend/internal/embedded/dist/*
cp -r frontend/build/* backend/internal/embedded/dist/

# Бэкенд
go build -ldflags "-X main.Version=$(sed -n 's/.*"version": "\(.*\)".*/\1/p' frontend/package.json)" \
  -o routebox ./backend/cmd/routebox

sudo ./routebox
```

</details>

## Лицензия

MIT
