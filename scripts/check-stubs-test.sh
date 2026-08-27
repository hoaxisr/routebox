#!/usr/bin/env bash
# Проверка самой проверки: check-stubs.sh дважды тихо пропускал всё —
# один раз из-за условия, ложного для каждого шаблона, второй из-за
# развалившегося sed в конвейере. Тест ломает шаблон и ждёт отказа.
set -u
cd "$(dirname "$0")/.."

pass=0; fail=0
# Каждый случай: правка одного шаблона и ожидаемый исход проверки.
run() { # run <имя> <ok|fail> <sed-выражение>
  local name=$1 want=$2 edit=$3
  local dir; dir=$(mktemp -d)
  cp -r stubs "$dir/stubs"
  cp scripts/check-stubs.sh "$dir/check.sh"
  mkdir -p "$dir/scripts" && mv "$dir/check.sh" "$dir/scripts/check-stubs.sh"
  [ -z "$edit" ] || sed -i -E "$edit" "$dir/stubs/stash/index.html"
  local got=ok
  bash "$dir/scripts/check-stubs.sh" >/dev/null 2>&1 || got=fail
  rm -rf "$dir"
  if [ "$got" = "$want" ]; then
    pass=$((pass+1))
  else
    fail=$((fail+1)); echo "FAIL: $name — проверка сказала $got, ожидалось $want" >&2
  fi
}

run "нетронутые шаблоны"       ok   ''
run "внешний скрипт"           fail 's@</body>@<script src="/tracker.js"></script></body>@'
run "скрипт с CDN"             fail 's@</body>@<script src="https://cdn.example.com/a.js"></script></body>@'
run "схемо-относительный шрифт" fail 's@</head>@<link rel="stylesheet" href="//fonts.example.com/x.css"></head>@'
run "внешняя картинка"         fail 's@</body>@<img src="//img.example.com/a.png"></body>@'
run "комментарий //TODO в скрипте" ok   's@</body>@<script>//TODO: подсказка\nvoid 0;</script></body>@'
run "снятый favicon"           fail 's@rel="icon"@rel="shortcut"@'
run "снятый title"             fail 's@<title>@<titlex>@'

echo "check-stubs-test: $pass ok, $fail fail"
[ "$fail" = 0 ]
