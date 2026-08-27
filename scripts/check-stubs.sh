#!/usr/bin/env bash
# Проверяет заглушки в stubs/: самодостаточность (никакой сети) и различимость.
# ponytail: grep вместо HTML-парсера — заглушки правим руками, разметка простая.
set -u
cd "$(dirname "$0")/.."

fail=0
err() { echo "FAIL: $*" >&2; fail=1; }

stubs=(stubs/*/index.html)
[ "${#stubs[@]}" -ge 3 ] || err "нужно минимум 3 шаблона, найдено ${#stubs[@]}"

titles=""; icons=""
for f in "${stubs[@]}"; do
  [ -f "$f" ] || { err "$f не файл"; continue; }

  # xmlns SVG — не сетевая ссылка, вырезаем перед проверкой на внешние ресурсы
  body=$(sed 's|xmlns=.http://www.w3.org[^"'"'"']*.||g' "$f")

  # Схемо-относительный адрес ловим только там, где он адрес: голое // ловило
  # любой комментарий // в инлайновом скрипте или CSS.
  echo "$body" | grep -qE 'https?://|(="|url\()//[a-zA-Z0-9.-]+' && err "$f: внешняя ссылка (CDN/шрифт/аналитика)"
  # data:-адрес — не внешний ресурс, поэтому вырезаем его ИЗ СВОЕГО ТЕГА, а не
  # снимаем проверку со всего файла: снятая со всего файла, она не срабатывала
  # никогда — favicon в виде data: обязателен в каждом шаблоне.
  echo "$body" | sed -E 's@(src|href)="data:[^"]*"@\1="#"@g' \
    | grep -qiE '<(script|link|img|iframe)[^>]+(src|href)="[^"#]' && err "$f: внешний ресурс в теге"
  grep -qiE '<link[^>]+rel="icon"[^>]+href="data:' "$f" || err "$f: нет встроенного favicon"
  grep -qiE '<meta[^>]+name="description"' "$f" || err "$f: нет meta description"
  grep -qiE 'type="password"' "$f" || err "$f: нет формы входа"

  t=$(grep -oiE '<title>[^<]*' "$f" | head -1)
  [ -n "$t" ] || err "$f: нет <title>"
  case "$titles" in *"[$t]"*) err "$f: заголовок повторяется — это отпечаток";; esac
  titles="$titles[$t]"

  i=$(grep -oiE 'rel="icon" href="[^"]*' "$f" | head -1)
  case "$icons" in *"[$i]"*) err "$f: favicon повторяется — это отпечаток";; esac
  icons="$icons[$i]"
done

[ "$fail" = 0 ] && echo "OK: ${#stubs[@]} шаблона(ов) самодостаточны и различимы"
exit "$fail"
