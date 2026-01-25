# RouteBox

Веб-интерфейс для превращения вашего Linux-устройства в VPN-роутер.

RouteBox управляет [amnezia-box](https://github.com/amnezia-vpn/amnezia-box) — позволяет настроить VPN-подключение и маршрутизировать через него трафик всей домашней сети.

## Возможности

- **Простая настройка** — мастер быстрой настройки за 2 минуты
- **Поддержка протоколов** — AmneziaWG, VLESS, Hysteria2
- **Гибкая маршрутизация** — весь трафик через VPN или только выбранные сайты
- **Готовые списки** — Antizapret и другие rule sets из коробки
- **Мониторинг** — трафик, соединения, логи в реальном времени
- **GeoIP** — флаги стран и информация о провайдерах для соединений
- **Веб-интерфейс** — управление с любого устройства в сети

## Требования

- Linux (amd64/arm64) — Debian, Ubuntu, Arch и др.
- Права root (для TUN-интерфейса)
- Установленный [amnezia-box](https://github.com/amnezia-vpn/amnezia-box)

## Быстрый старт

### Автоматическая установка (рекомендуется)

```bash
curl -fsSL https://raw.githubusercontent.com/hoaxisr/routebox/main/install.sh | sudo bash
```

Скрипт:
- Скачает и установит RouteBox в `/usr/local/bin/`
- Создаст файл настроек `/etc/routebox/routebox.toml`
- Создаст systemd сервис
- Включит IP-forwarding

### Ручная установка

```bash
# Скачать
curl -L -o routebox https://raw.githubusercontent.com/hoaxisr/routebox/main/releases/routebox-linux-amd64
chmod +x routebox

# Запустить
sudo ./routebox
```

### 2. Откройте в браузере

```
http://IP-АДРЕС-УСТРОЙСТВА:8080
```

### 3. Пройдите мастер настройки

1. Вставьте конфигурацию VPN (ссылка или файл .conf)
2. Выберите списки сайтов для маршрутизации
3. Нажмите "Применить"

Готово! Теперь направьте трафик других устройств через этот роутер.

## Удаление

```bash
curl -fsSL https://raw.githubusercontent.com/hoaxisr/routebox/main/install.sh | sudo bash -s -- --uninstall
```

## Конфигурация

RouteBox использует TOML-файл для настроек приложения:

```
/etc/routebox/routebox.toml
```

### Основные секции

| Секция | Описание |
|--------|----------|
| `[geoip]` | Путь к MMDB базе, включение GeoIP |
| `[ui]` | Тема, язык, формат отображения |
| `[monitoring]` | Настройки мониторинга соединений |
| `[network]` | Адрес и порт веб-интерфейса |
| `[singbox]` | Пути к конфигу amnezia-box |

### Пример настройки GeoIP

```toml
[geoip]
path = "/etc/routebox/ipinfo.mmdb"
enabled = true
```

## GeoIP (флаги стран)

RouteBox может показывать флаги стран и информацию о провайдерах для активных соединений.

### Установка GeoIP базы

1. Скачайте бесплатную базу IPInfo:
   https://ipinfo.io/developers/free-ip-database

2. Поместите файл `.mmdb` в `/etc/routebox/`

3. Обновите настройки:
   ```bash
   sudo nano /etc/routebox/routebox.toml
   ```

   Установите путь:
   ```toml
   [geoip]
   path = "/etc/routebox/ipinfo_lite.mmdb"
   enabled = true
   ```

4. Перезапустите сервис:
   ```bash
   sudo systemctl restart routebox
   ```

После этого в списке соединений появятся флаги стран, а при наведении — информация о провайдере.

## Параметры запуска

```
--settings PATH   Путь к routebox.toml (по умолчанию: авто-поиск)
--config PATH     Путь к конфигу amnezia-box (переопределяет settings)
--listen ADDR     Адрес веб-интерфейса (переопределяет settings)
--geoip PATH      Путь к GeoIP MMDB (переопределяет settings)
--clash ADDR      Адрес Clash API (определяется автоматически)
```

### Приоритет настроек

1. Флаги командной строки (высший приоритет)
2. Файл `routebox.toml`
3. Авто-определение
4. Значения по умолчанию

## Установка как сервис (ручная)

```bash
# Скопировать в /usr/local/bin
sudo cp routebox /usr/local/bin/

# Создать директории
sudo mkdir -p /etc/routebox /etc/amnezia-box

# Скопировать настройки
sudo cp routebox.toml /etc/routebox/

# Создать systemd сервис
sudo tee /etc/systemd/system/routebox.service << 'EOF'
[Unit]
Description=RouteBox - VPN Router Web UI
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/routebox --settings /etc/routebox/routebox.toml
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Включить и запустить
sudo systemctl daemon-reload
sudo systemctl enable --now routebox
```

## Подготовка системы

Для работы роутера включите IP-forwarding:

```bash
# Временно (до перезагрузки)
sudo sysctl -w net.ipv4.ip_forward=1

# Постоянно
echo "net.ipv4.ip_forward=1" | sudo tee -a /etc/sysctl.conf
sudo sysctl -p
```

## API

RouteBox предоставляет REST API для интеграции:

| Endpoint | Описание |
|----------|----------|
| `GET /api/status` | Статус amnezia-box |
| `GET /api/settings` | Настройки RouteBox |
| `PUT /api/settings` | Обновить настройки |
| `GET /api/clash/connections` | Активные соединения |
| `GET /api/clash/proxies` | Статус прокси |

Полная документация API: [docs/API.md](docs/API.md)

## Поддерживаемые форматы конфигурации

- **AmneziaWG** — `.conf` файл или текст конфига
- **VLESS** — ссылка `vless://...`
- **Hysteria2** — ссылка `hy2://...`

## Структура проекта

```
routebox/
├── backend/           # Go backend
│   ├── cmd/routebox/  # Main
│   └── internal/      # Packages
├── frontend/          # SvelteKit SPA
├── routebox.toml      # Пример настроек
├── install.sh         # Установщик
└── Makefile           # Сборка
```

## Сборка из исходников

```bash
# Установить зависимости
make deps

# Собрать
make build

# Запустить
sudo ./routebox
```

## Лицензия

MIT
