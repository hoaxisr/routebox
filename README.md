# RouteBox

[![Release](https://img.shields.io/github/v/release/hoaxisr/routebox)](https://github.com/hoaxisr/routebox/releases/latest)
[![License](https://img.shields.io/badge/license-MIT-blue)](#лицензия)
![Platform](https://img.shields.io/badge/platform-linux%20amd64%20%7C%20arm64-lightgrey)

Веб-панель для [amnezia-box](https://github.com/hoaxisr/amnezia-box) (форк sing-box). Один бинарник со встроенным интерфейсом, два сценария: VPN-роутер для домашней сети и панель управления прокси-сервером на VPS.

![Дашборд: статус amnezia-box, скорость и объём трафика, топ-соединения](assets/01-dashboard.png)

## Два режима

| | Сервер: панель на VPS | Клиент: роутер дома |
|---|---|---|
| Что делает | Управляет VPN-сервером: протоколы, пользователи, подписки | Направляет трафик домашней сети через VPN: целиком или только выбранные сайты |
| Установка | [`vps-install.sh`](https://github.com/hoaxisr/routebox/blob/main/vps-install.sh) или Docker | [`install.sh`](https://github.com/hoaxisr/routebox/blob/main/install.sh) |
| Доступ | `https://домен:8443`, TLS-сертификат Let's Encrypt выпускается автоматически | `http://IP-роутера:8080` |
| Авторизация | Обязательна, пароль администратора генерируется при установке | Опциональна |
| Что подключается | Клиенты пользователей — по ссылке-подписке, QR-коду или файлу конфигурации | AmneziaWG / WireGuard, VLESS, Hysteria2, NaiveProxy, Mieru — по ссылке или файлу |

Режим задаётся ключом `mode` в настройках; интерфейс показывает только разделы своего режима.

<details>
<summary><b>Возможности</b></summary>

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
- Установка «из коробки»: пять протоколов, сайт-заглушка и панель за одним внешним портом 443 — одной командой
- Инбаунды VLESS (в том числе Reality), Hysteria2, NaiveProxy, Mieru; транспорты raw / ws / grpc / httpupgrade / xhttp
- Сервер AmneziaWG (включая версию AWG 3.0) без модуля ядра, с обфускацией и профилями маскировки; выдача клиентам QR-кода, `.conf` и sing-box JSON
- Пользователи: ссылки-подписки, сроки действия, включение и отключение, учёт трафика
- Telegram-прокси (MTProto) внутри самой панели: у каждого клиента свой секрет и своя ссылка `tg://proxy`, отдельный счётчик трафика и срок действия; ссылку можно отозвать, не трогая остальных. В Telegram можно ходить напрямую или через любой outbound/endpoint из конфига

**Мониторинг**
- Трафик в реальном времени с историей, разбивка по аутбаундам
- Активные соединения с флагами стран (GeoIP), цепочкой узлов и объёмами
- Логи amnezia-box и журнал systemd

**Обслуживание**
- Проверка и установка обновлений RouteBox и amnezia-box из панели
- Валидация конфигурации перед применением, черновики и откат изменений

</details>

## Установка: сервер (панель на VPS)

Требования:

- VPS на Linux (amd64 или arm64), права root. Для установки на голое железо — systemd.
- **Домен с A/AAAA-записью на IP сервера** — обязателен: по нему выпускается TLS-сертификат панели. По «голому» IP панель не откроется: сертификат выпускается на домен, и по IP TLS-соединение не установится.
- Порты `8443/tcp` (панель) и `80/tcp` (HTTP-01 проверка Let's Encrypt; должен оставаться открытым — продления идут по нему же).

Четыре способа: всё из коробки за одним портом, скрипт на голое железо, скрипт для Docker и compose вручную. Все поднимают одну и ту же панель, состояние и настройки совместимы.

<details>
<summary><b>Скрипт: vps-install.sh</b></summary>

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

Скрипт проверяет DNS, ставит оба бинарника с проверкой sha256, настраивает systemd и firewall. Порты `8443/tcp` и `80/tcp` он откроет в ufw, если тот активен; порты 443 и 22 не трогает. Сертификат RouteBox выпускает и продлевает сам — nginx и certbot не нужны.

Панель доступна на `https://panel.example.com:8443` — открывайте по домену, не по IP. Логин `admin`, пароль генерируется при первом старте и сохраняется в `/etc/routebox/routebox-initial-password` (также виден в `journalctl -u routebox`). Смените его после входа в разделе App Settings → Security.

</details>

<details>
<summary><b>Всё из коробки: пять протоколов за одним 443</b></summary>

Один запуск на чистом сервере приводит его в конечное рабочее состояние: настроены пять протоколов (VLESS + Reality, VLESS + gRPC, Trojan + WebSocket, NaiveProxy, Mieru), поднят сайт-заглушка, панель доступна по секретному адресу. Наружу открыт ровно один порт — **443, по TCP и по UDP**, — плюс `80`, по которому выпускается и продлевается сертификат.

Снаружи такой сервер выглядит обычным сайтом: на 443 стоит sing-box с Reality, который заимствует имя собственного домена, а весь неопознанный трафик передаёт веб-серверу за собой. Тот отдаёт заглушку, обслуживает NaiveProxy и держит инбаунды с транспортом. Панель прячется за секретной ссылкой: заход по ней ставит cookie, и только с этой cookie домен отдаёт панель, а не заглушку.

Требования:

- чистый сервер: `443/tcp`, `443/udp` и `80/tcp` должны быть свободны — установщик остановится и назовёт, кто их занял;
- домен с A-записью на этот сервер — сверяется до того, как что-либо изменится;
- Docker с compose v2 и `iproute2` (`ss`, им проверяется занятость портов) — чего не хватает, установщик предложит поставить сам; `--install-deps` соглашается заранее, `--no-deps` запрещает.

```bash
curl -fsSL https://raw.githubusercontent.com/hoaxisr/routebox/main/docker-install.sh \
  | sudo bash -s -- --allinone --domain example.com --email you@example.com
```

Без флагов скрипт предложит этот режим сам, если он здесь возможен, и спросит только домен и почту. Флаги: `--domain`, `--email`, `--dir`, `--staging` (тестовый CA на первый прогон), `--stub` (шаблон заглушки, по умолчанию случайный), `--install-deps`, `--awg-port` и `--awg-kernel` (см. ниже). `--dry-run` печатает намерения и будущий `docker-compose.yml`, ничего не создавая.

В конце установщик один раз печатает адрес панели и пароль. Адрес можно спросить снова:

```bash
cd /opt/routebox && docker compose exec routebox routebox panel-url
```

Удаление — как и у остальных режимов, `--uninstall` (с `--purge` удаляет и данные): снимаются оба контейнера, сеть и собственный compose-файл. Чужого скрипт не трогает: в этом режиме он не правит ни одного файла вне своего каталога.

**Что вы принимаете вместе с этим режимом.** Это цена устройства, а не недоделки:

- **Адреса клиентов недоступны.** Фронт на 443 передаёт неаутентифицированный трафик сырыми байтами и адрес клиента до сервера не доносит — у всех соединений источником будет обратная петля. Монитор соединений и дашборд поэтому не показывают адрес вовсе, а объясняют, почему его нет. Это не дефект и не поправимо в этой схеме.
- **Защита от перебора пароля панели считает всех за одного.** Она завязана на адрес клиента, а он здесь один и тот же для всех. Пароль панели поэтому меняйте на длинный, а секретную ссылку не публикуйте.
- **Единая точка отказа.** Остановка sing-box роняет не только VPN: она уносит с собой и сайт-заглушку, и панель — весь трафик домена входит через фронт. Кнопка Stop в панели отключает вас от самой панели.
- **Правки конфигурации не доезжают до уже установленных серверов.** План строится один раз, на пустой конфигурации, и больше никогда её не переписывает — там живут пользователи, ключи и секретные пути. Исправления в схеме получают только новые установки.
- **AmneziaWG требует своего UDP-порта.** За фронт его не спрятать: 443/TCP держит Reality, 443/UDP — Mieru, а мультиплексировать WireGuard-подобный протокол нечем. Порт выбирается на установке (`--awg-port 51820`), публикуется в compose и дальше в панели не меняется: другой порт попал бы на сокет, который никто не пробрасывает. С флагом `--awg-kernel` установщик поставит на хост ядерный модуль (только Debian/Ubuntu, только там, где модули вообще грузятся) и выдаст контейнеру `NET_ADMIN`; без него работает бэкенд на sing-box, которому модуль не нужен.
- **NaiveProxy не показан в списке инбаундов.** Его обслуживает не sing-box, а веб-сервер за фронтом, поэтому инбаунда у него нет: он живёт отдельной карточкой на той же странице, попадает в подписку и в QR-код наравне с остальными.
- **Сертификат может выпуститься не сразу.** Если DNS домена отвечает медленно или с ошибками, первые минуты по `https://` будет тишина: сервер уже поднят, а Let's Encrypt ещё повторяет проверку. Переустанавливать не нужно — выпуск повторяется сам.

</details>

<details>
<summary><b>Docker: установка скриптом</b></summary>

Скрипт спрашивает домен, порты и кто держит TLS, смотрит, что уже есть на хосте, и встраивается в это. Compose-файл и каталог с данными он создаёт сам.

```bash
curl -fsSL https://raw.githubusercontent.com/hoaxisr/routebox/main/docker-install.sh | sudo bash
```

Без nginx на хосте панель держит TLS сама: сертификат выпускает встроенный ACME, наружу открываются порт панели и `80` для проверки Let's Encrypt.

С nginx на хосте панель уходит на loopback, а наружу смотрит nginx: скрипт выпускает сертификат через certbot, пишет свой vhost и перечитывает конфигурацию. Чужие файлы он не правит, а если домен уже кем-то занят — останавливается и просит другой поддомен.

Для nginx-ветки нужен certbot: если его нет и готового сертификата тоже, скрипт остановится и предложит либо поставить certbot, либо вернуться к встроенному ACME.

Если на 443 никто, кроме nginx, не сидит, а модуль stream доступен, скрипт предложит SNI-роутер: тогда на 443 живут и панель, и инбаунд — nginx разводит их по домену из TLS-приветствия. Инбаунд после этого создаётся в панели с портом 443, порт под него уже проброшен. Hysteria2, Mieru и AmneziaWG так не отдать: это UDP, им нужен свой порт.

Если TLS держит Caddy, Traefik или nginx в контейнере, скрипт ставит панель на loopback и печатает готовую конфигурацию для вашего прокси — сам он её не применяет.

Повторный запуск спрашивает только каталог и дальше обновляет образ. Удаление — `sudo bash docker-install.sh --uninstall`, данные при этом остаются (`--purge` удаляет и их); файлы nginx скрипт сносит только свои, по маркеру в первой строке. Посмотреть, что получится, ничего не трогая: `--dry-run`.

</details>

<details>
<summary><b>Docker Compose вручную</b></summary>

Образ собран на [базовом образе LinuxServer.io](https://docs.linuxserver.io/general/containers-101/) и поддерживает только режим панели. Роутеру нужны TUN и роль шлюза локальной сети — в контейнере этого нет, для него есть `install.sh`.

```bash
curl -fsSLO https://raw.githubusercontent.com/hoaxisr/routebox/source/docker-compose.yml
# впишите свой домен в PUBLIC_HOST и почту в ACME_EMAIL
docker compose up -d
```

Панель поднимется на `https://ваш-домен:8443` и сама выпустит сертификат. Пароль администратора она генерирует при первом старте и кладёт в `/config/routebox-initial-password`; он же виден в `docker compose logs routebox`.

Всё состояние лежит в одном volume `/config`: `routebox.toml`, конфиг и бинарник amnezia-box, база GeoIP, `traffic.db`, кэш ACME, ключи сервера AmneziaWG. Внутри контейнера `/etc/routebox` и `/etc/amnezia/amneziawg` — симлинки туда же.

Переменные окружения контейнер читает один раз, когда создаёт `routebox.toml`. Дальше настройки живут в файле, и панель правит его же, так что домен после первого запуска меняйте в `/config/routebox.toml`. Если `PUBLIC_HOST` разойдётся с файлом, контейнер напишет об этом в лог.

| Переменная | Назначение |
|---|---|
| `PUBLIC_HOST` | домен панели; попадает в ссылки-подписки |
| `PUBLIC_PORT` | порт, по которому панель видят клиенты (по умолчанию — из `LISTEN`) |
| `ACME_EMAIL` | контакт для Let's Encrypt |
| `ACME_ENABLED` | `false`, если TLS терминирует прокси |
| `ACME_STAGING` | тестовый CA Let's Encrypt на время проверки |
| `ACME_HTTP_ADDR` | адрес слушателя HTTP-01, если `:80` внутри контейнера занят |
| `LISTEN` | адрес и порт панели внутри контейнера |
| `TRUSTED_PROXIES` | адреса прокси, чьему `X-Forwarded-For` можно верить |
| `AWG_LISTEN_PORT` | UDP-порт AmneziaWG, опубликованный на контейнере; пока переменная задана, панель этот порт не меняет |
| `PUID` / `PGID` | владелец файлов в `/config` |

Домен `panel.example.com` из примера контейнер игнорирует: сертификат на чужой домен всё равно не выпустится, а неудачные проверки расходуют лимит Let's Encrypt.

Бинарник amnezia-box контейнер копирует из образа в `/config/bin` при первом запуске. Дальше он живёт на volume, поэтому обновление из панели работает и переживает пересоздание контейнера; уже установленный бинарник образ не трогает. systemd в контейнере нет, процесс запускает сам RouteBox при каждом старте — если конфиг и бинарник на месте. Start/Stop/Restart в панели работают как обычно.

Наружу открыты `8443` (панель) и `80`: по нему Let's Encrypt проверяет домен, в том числе при продлении. Инбаунды amnezia-box вы настраиваете в панели и публикуете сами через `ports:`. Панель работает не под root, но Docker выставляет в контейнерах `net.ipv4.ip_unprivileged_port_start=0`, так что порт меньше 1024 занимается и без `NET_BIND_SERVICE`. Если ваш runtime так не умеет, добавьте `cap_add: [NET_BIND_SERVICE]` или дайте инбаунду порт от 1024 и пробросьте его на нужный (`"443:8444"`).

Обновление образа:

```bash
docker compose pull && docker compose up -d
```

Раздел Updates работает и здесь: amnezia-box ставится прямо из панели, и обновление остаётся после `docker compose up -d`, потому что бинарник лежит на volume. Свой бинарник RouteBox берёт из образа, поэтому вместо кнопки показывает ту же команду с именем сервиса: `docker compose pull routebox && docker compose up -d routebox`.

</details>

<details>
<summary><b>За обратным прокси: nginx, Caddy, Traefik</b></summary>

Если TLS уже держит nginx, Caddy или Traefik, встроенный ACME не нужен: RouteBox отдаёт панель по обычному HTTP, а сертификат остаётся заботой прокси. Порт `80` наружу не публикуется, `ACME_EMAIL` можно не задавать.

```yaml
environment:
  - PUID=1000
  - PGID=1000
  # Адрес, по которому панель видят клиенты: адрес прокси, а не контейнера.
  - PUBLIC_HOST=panel.example.com
  - PUBLIC_PORT=443
  - ACME_ENABLED=false
  # Чьему X-Forwarded-For верить: только адрес самого прокси.
  - TRUSTED_PROXIES=172.28.0.2/32
networks: [proxynet]
ports: []
expose:
  - "8443"
```

Адрес прокси в той же сети при этом закрепляется:

```yaml
<сервис прокси>:
  networks:
    proxynet:
      ipv4_address: 172.28.0.2

networks:
  proxynet:
    ipam:
      config:
        - subnet: 172.28.0.0/24
          # Docker раздаёт адреса с начала подсети и статику не резервирует,
          # так что динамический диапазон лучше увести от закреплённого .2.
          ip_range: 172.28.0.128/25
```

`TRUSTED_PROXIES` лучше задать сразу. На адресе клиента держатся блокировка перебора пароля и лимит запросов к публичному `/sub/{token}`, а из-за прокси все клиенты приходят с одного адреса и считаются за одного: лимит подписок срабатывает через несколько запросов и накрывает всех. Заголовку RouteBox верит только с перечисленных адресов, с остальных смотрит на реальный адрес соединения — подделать не выйдет. Из цепочки он берёт самый правый адрес, которого нет в списке.

**В списке должен быть адрес прокси, а не подсеть docker-сети.** Подсеть — это все контейнеры в ней: скомпрометированный сосед пришлёт панели поддельный `X-Forwarded-For` напрямую и обойдёт блокировку перебора пароля. Отсюда и фиксированный `ipv4_address` у прокси, и /32 в списке. Если в сети включён IPv6, закрепите и `ipv6_address` и добавьте его тоже: Docker отвечает на DNS-запрос сначала AAAA, прокси придёт по ULA-адресу, а адрес, которого нет в списке, доверенным не считается — и предупреждения об этом не будет. Про `X-Forwarded-For` с неизвестного адреса RouteBox пишет в лог один раз на адрес.

Прокси должен пробрасывать WebSocket — по ним на дашборд идут трафик, логи и список соединений — и передавать `X-Forwarded-Proto`, иначе cookie сессии останется без флага `Secure`.

nginx:

```nginx
server {
    listen 443 ssl http2;
    server_name panel.example.com;

    ssl_certificate     /etc/letsencrypt/live/panel.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/panel.example.com/privkey.pem;

    location / {
        proxy_pass http://routebox:8443;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        # Дашборд держит WebSocket открытым, с таймаутом по умолчанию он рвётся.
        proxy_read_timeout 3600s;
        proxy_buffering off;
    }
}

# в http-блоке:
# map $http_upgrade $connection_upgrade { default upgrade; "" close; }
```

Caddy — `X-Forwarded-*` и WebSocket из коробки:

```caddy
panel.example.com {
    reverse_proxy routebox:8443
}
```

Этот конфиг покрывает только панель и подписки. Инбаунды amnezia-box публикует сам контейнер своими портами: UDP-протоколы и raw-транспорты через HTTP-прокси не пройдут в принципе.

</details>

<details>
<summary><b>Telegram-прокси (MTProto)</b></summary>

Прокси поднимает сама панель, отдельного контейнера и второго бинарника не нужно. Страница «Telegram» есть только в режиме VPS.

У каждого клиента свой секрет: ссылку можно отозвать отдельно, а трафик считается по клиентам.

Порт по умолчанию — `9443`: 443 обычно занят обратным прокси, 8443 — самой панелью. В Docker опубликуйте его:

```yaml
ports:
  - "9443:9443"   # порт из настроек панели
```

Если 443 уже держит nginx с `ssl_preread`, прокси можно завести за ним по SNI и не открывать отдельный порт. Клиенты Telegram отправляют домен маскировки в SNI — по нему и маршрутизируем:

```nginx
stream {
    map $ssl_preread_server_name $backend {
        www.microsoft.com   routebox:9443;   # домен маскировки -> MTProto
        default             panel_backend;
    }

    server {
        listen 443;
        ssl_preread on;
        proxy_pass $backend;
    }
}
```

**Домен маскировки входит в секрет каждого клиента.** Прокси притворяется этим сайтом, и туда же уходит всё, что не прошло проверку. Менять домен после раздачи ссылок нельзя: перестанут работать все выданные. Панель предупреждает об этом перед сохранением. Берите сайт, куда ваши пользователи и так ходят по HTTPS.

В ссылки идут «Внешний хост» и «Внешний порт» из настроек страницы, а не адрес прослушивания: за обратным прокси или SNI-маршрутизатором это разные вещи. Пока хост или домен не заданы, кнопки ссылки и QR неактивны. Ссылка без них выглядит правильной, но в Telegram не сработает.

**Выход в Telegram.** По умолчанию прокси ходит к серверам Telegram напрямую, с адреса этого сервера. В настройках можно выбрать любой outbound или endpoint из конфигурации sing-box — WARP, hysteria2 до другой машины, selector, — и трафик пойдёт туда.

Отдать внешнему процессу конкретный outbound sing-box не умеет, поэтому под выбранный тег RouteBox сам дописывает в конфиг две вещи:

```json
"inbounds": [
  { "type": "socks", "tag": "mtproto-socks", "listen": "127.0.0.1", "listen_port": 1080 }
],
"route": { "rules": [
  { "inbound": ["mtproto-socks"], "outbound": "ionos-hy2" }
] }
```

Правило встаёт первым, чтобы его не перехватило чьё-нибудь общее правило ниже. Оба объекта принадлежат RouteBox: править и удалять их вручную нельзя (как и endpoint `awg-server`), они появляются и исчезают вместе с настройкой. Выбор «напрямую» убирает и inbound, и правило. Слушает только `127.0.0.1` — открытый наружу SOCKS без пароля быстро находят и начинают использовать не по назначению.

Смена выхода переписывает активный конфиг и перезагружает amnezia-box: живые VPN-подключения на секунду рвутся. Пока в конфиге есть неприменённые изменения, настройка не сохраняется — сначала примените или отбросьте черновик. В списке выходов нет `awg-server`: это точка входа для пиров, а не выход, и трафик Telegram в ней просто пропадёт.

Настройку можно задать и руками — `outbound` в блоке `[mtproto]`; при старте RouteBox сам приведёт конфиг sing-box в соответствие.

Если выбранный выход отвалится, подключения к Telegram будут падать, а не молча уходить напрямую: тихий откат означал бы трафик с собственного IP сервера, ровно то, ради чего выход и выбирали. Доменный фронтинг остаётся прямым намеренно — запрос к маскировочному сайту должен выглядеть как обычный визит, а не как визит из чужого VPN-выхода.

Рекламных каналов (adtag) нет: mtglib их не поддерживает.

</details>

<details>
<summary><b>Бэкенд kernel для AmneziaWG в Docker</b></summary>

Бэкенд `kernel` в образе работает, но требует подготовки хоста: модуль, права и порт — всё ниже. По умолчанию сервер AmneziaWG поднимается на `singbox`: ему не нужны ни модуль ядра, ни особые права.

Для `kernel` понадобятся модуль `amneziawg`, загруженный **на хосте** (пакет `amneziawg-dkms` с заголовками ядра, затем `modprobe amneziawg`), права `NET_ADMIN` у контейнера и проброшенный UDP-порт сервера — он не HTTP и через обратный прокси не пойдёт. amneziawg-tools уже в образе. Панель проверяет инструменты и права заранее и при нехватке отказывает, называя, чего именно не хватает.

```yaml
cap_add:
  - NET_ADMIN
sysctls:
  - net.ipv4.ip_forward=1
ports:
  - "51820:51820/udp"   # порт сервера AmneziaWG из настроек панели
```

RouteBox передаёт права через ambient set, поэтому их видят `awg-quick` и запускаемые им `ip` и `iptables`, а сам процесс панели остаётся под пользователем `PUID`. Их наследуют и дочерние процессы, включая amnezia-box, — привилегия расползается шире, чем хотелось бы, поэтому без нужды бэкенд `kernel` не включайте. Без `cap_add` панель работает как раньше, без привилегий.

AWG 3.0 (защита заголовков, CPA/RAT) на этом бэкенде доступна, если и модуль ядра, и amneziawg-tools на хосте версии 3.x. Версию загруженного модуля панель читает из sysfs; если модуль ещё не загружен, она спрашивает `modinfo`, которому нужен смонтированный `/lib/modules:/lib/modules:ro`. Панель проверяет их независимо, так что свежий модуль со старыми инструментами тоже считается неподдерживаемым. На бэкенде `singbox` AWG 3.0 доступна всегда и от версии модуля не зависит.

</details>

![Сервер AmneziaWG: клиенты, выдача QR-кода, .conf и JSON, сроки действия](assets/05-awg.png)

![Пользователи панели: состояние, срок действия, ссылка-подписка и учёт трафика](assets/06-users.png)

## Установка: клиент (роутер дома)

Требования: Linux (amd64 или arm64) с systemd, права root. В Docker этот режим не поднимается — ему нужны TUN и роль шлюза локальной сети.

```bash
curl -fsSL https://raw.githubusercontent.com/hoaxisr/routebox/main/install.sh | sudo bash
```

Скрипт скачивает последние релизы RouteBox и amnezia-box, кладёт настройки в `/etc/routebox/`, скачивает базу GeoIP, включает IP-forwarding и создаёт systemd-сервисы `routebox` и `amnezia-box`.

Дальше:

```bash
sudo systemctl start routebox
```

1. Откройте `http://IP-устройства:8080`.
2. Добавьте подключение: AmneziaWG и WireGuard импортируются из `.conf` в разделе Endpoints; VLESS, Hysteria2, NaiveProxy и Mieru — по ссылке в Outbounds.
3. В разделе Routes выберите, что маршрутизировать через VPN — весь трафик или списки сайтов.
4. На устройствах домашней сети укажите IP роутера как шлюз (или раздайте его по DHCP).

![Маршрутизация: rule-sets с действиями proxy, block, direct и приоритетом перетаскиванием](assets/02-routes.png)

![Активные соединения: страна, цепочка через выбранный узел, объём и время жизни](assets/03-connections.png)

![Монитор трафика: история за 60 секунд и разбивка по аутбаундам](assets/04-traffic.png)

## Обновление

В обоих режимах: раздел Updates в панели проверяет и ставит новые версии RouteBox и amnezia-box. Список изменений — в [CHANGELOG.md](CHANGELOG.md).

Из консоли:

```bash
# Сервер: конфигурация и сертификат сохраняются
sudo bash vps-install.sh --update

# Сервер в Docker
docker compose pull && docker compose up -d

# Клиент: повторный запуск установщика обновляет бинарники, настройки не трогает
curl -fsSL https://raw.githubusercontent.com/hoaxisr/routebox/main/install.sh | sudo bash
```

## Удаление

```bash
# Сервер; --purge дополнительно удаляет /etc/routebox и /etc/amnezia-box
sudo bash vps-install.sh --uninstall

# Сервер в Docker; данные остаются в ./config
docker compose down

# Сервер в Docker, поставленный скриптом (в том числе «из коробки»);
# --purge дополнительно удаляет ./config с данными
sudo bash docker-install.sh --uninstall --dir /opt/routebox

# Клиент (каталоги настроек остаются)
curl -fsSL https://raw.githubusercontent.com/hoaxisr/routebox/main/install.sh | sudo bash -s -- --uninstall
```

## Справочник

<details>
<summary><b>Конфигурация: routebox.toml и GeoIP</b></summary>

Настройки приложения хранятся в `/etc/routebox/routebox.toml` ([пример с комментариями](https://github.com/hoaxisr/routebox/blob/main/routebox.toml)). Большинство параметров также доступно в панели в разделе App Settings.

| Секция | Описание |
|--------|----------|
| `[geoip]` | Путь к MMDB-базе, включение GeoIP |
| `[ui]` | Язык (en/ru), единицы скорости (биты или байты) |
| `[monitoring]` | GeoIP-обогащение соединений |
| `[security]` | Авторизация панели, CORS, таймаут сессии, `trusted_proxies` — чьему `X-Forwarded-For` верить |
| `[network]` | Адрес и порт панели, таймауты, сжатие, TLS (свой сертификат или встроенный ACME/Let's Encrypt) |
| `[server]` | Режим (`router`/`vps`), публичный хост и порт для ссылок-подписок |
| `[singbox]` | Путь к конфигу и бинарнику amnezia-box (`config_path`, `binary_path`; пустые — автопоиск), адрес Clash API, адрес v2ray-статистики. systemd-юнит определяется сам: сначала `amnezia-box`, потом `sing-box` |
| `[updates]` | Ежедневная автопроверка обновлений |
| `[awg]` | Сервер AmneziaWG: интерфейс, подсеть, порт, DNS, бэкенд, обфускация |
| `[mtproto]` | Telegram-прокси: адрес прослушивания, домен маскировки, внешние хост и порт для ссылок, `outbound` для выхода в Telegram и `socks_port` под него |
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
--binary PATH     Путь к бинарнику amnezia-box (переопределяет settings)
--clash ADDR      Адрес Clash API (по умолчанию берётся из конфига amnezia-box)
--mode MODE       Режим панели: router (по умолчанию) или vps
```

Приоритет: флаги командной строки → `routebox.toml` → авто-определение → значения по умолчанию.

В режиме router нужен root: RouteBox создаёт TUN-интерфейс и управляет системными сервисами. В режиме vps root не обязателен — в Docker панель и работает под пользователем `PUID`.

</details>

<details>
<summary><b>REST API</b></summary>

Все эндпоинты, кроме `/api/health` и `/api/auth/login`, `/logout`, `/session`, требуют сессию, если авторизация включена. Основные:

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

Подходит для обоих режимов: разница только в `mode` в `routebox.toml`.

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

Понадобятся Go 1.25+ и Node.js с npm.

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
