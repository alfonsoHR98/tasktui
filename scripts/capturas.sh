#!/bin/sh
# Regenera las capturas de la TUI que usan el README y la página web.
#   scripts/capturas.sh [carpeta-de-salida]      (por defecto: docs/img)
# Necesita Go, Python 3 y Google Chrome (o Chromium en $CHROME).
set -eu
cd "$(dirname "$0")/.."
out="${1:-docs/img}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
chrome="${CHROME:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}"

TT_SHOT="$tmp" go test ./internal/tui/ -run TestCapturas -count=1 >/dev/null
mkdir -p "$out"
for f in "$tmp"/*.ansi; do
  name="$(basename "$f" .ansi)"
  set -- $(python3 scripts/ansi2html.py "$f" "$tmp/$name.html")   # columnas y filas
  w=$(( $1 * 843 / 100 + 56 ))
  h=$(( $2 * 1932 / 100 + 48 ))
  "$chrome" --headless=new --disable-gpu --hide-scrollbars --force-device-scale-factor=2 \
    --window-size="$w,$h" \
    --screenshot="$out/$name.png" "file://$tmp/$name.html" >/dev/null 2>&1
  echo "→ $out/$name.png"
done
