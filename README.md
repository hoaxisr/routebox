# RouteBox — дистрибутивная ветка

Здесь лежат установочные скрипты и пример настроек. Код, документация и релизы — на ветке [`source`](https://github.com/hoaxisr/routebox).

**Полное описание, установка и скриншоты: [README на `source`](https://github.com/hoaxisr/routebox#readme).**

Установка роутера дома:

```bash
curl -fsSL https://raw.githubusercontent.com/hoaxisr/routebox/main/install.sh | sudo bash
```

Установка панели на VPS:

```bash
curl -fsSL https://raw.githubusercontent.com/hoaxisr/routebox/main/vps-install.sh \
  | sudo bash -s -- --domain panel.example.com --email you@example.com --staging
```

Установка панели на VPS в Docker (интерактивно, спрашивает домен и порты, встраивается в имеющийся nginx):

```bash
curl -fsSL https://raw.githubusercontent.com/hoaxisr/routebox/main/docker-install.sh | sudo bash
```

Бинарники — в [релизах](https://github.com/hoaxisr/routebox/releases). Лицензия MIT.
