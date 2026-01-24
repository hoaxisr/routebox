# RouteBox

Веб-интерфейс для превращения вашего Linux-устройства в VPN-роутер.

RouteBox управляет [amnezia-box](https://github.com/amnezia-vpn/amnezia-box) — позволяет настроить VPN-подключение и маршрутизировать через него трафик всей домашней сети.

## Возможности

- **Простая настройка** — мастер быстрой настройки за 2 минуты
- **Поддержка протоколов** — AmneziaWG, VLESS, Hysteria2
- **Гибкая маршрутизация** — весь трафик через VPN или только выбранные сайты
- **Готовые списки** — Antizapret и другие rule sets из коробки
- **Мониторинг** — трафик, соединения, логи в реальном времени
- **Веб-интерфейс** — управление с любого устройства в сети

## Требования

- Linux (amd64) — Debian, Ubuntu, Arch и др.
- Права root (для TUN-интерфейса)
- Установленный [amnezia-box](https://github.com/amnezia-vpn/amnezia-box)

## Быстрый старт

### Автоматическая установка (рекомендуется)

```bash
curl -fsSL https://raw.githubusercontent.com/hoaxisr/routebox/main/install.sh | sudo bash
```

Скрипт:
- Скачает и установит RouteBox в `/usr/local/bin/`
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

## Установка как сервис (ручная)

Для автозапуска при загрузке системы:

```bash
# Скопировать в /usr/local/bin
sudo cp routebox /usr/local/bin/

# Создать systemd сервис
sudo tee /etc/systemd/system/routebox.service << 'EOF'
[Unit]
Description=RouteBox - VPN Router Web UI
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/routebox --config /etc/amnezia-box/config.json
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

## Параметры запуска

```
--config PATH   Путь к конфигу amnezia-box (по умолчанию: /etc/amnezia-box/config.json)
--listen ADDR   Адрес веб-интерфейса (по умолчанию: 0.0.0.0:8080)
--clash ADDR    Адрес Clash API (определяется автоматически)
```

## Скриншоты

*Dashboard с мониторингом трафика*

*Мастер быстрой настройки*

*Управление маршрутами*

## Поддерживаемые форматы конфигурации

- **AmneziaWG** — `.conf` файл или текст конфига
- **VLESS** — ссылка `vless://...`
- **Hysteria2** — ссылка `hy2://...`

## Лицензия

MIT
