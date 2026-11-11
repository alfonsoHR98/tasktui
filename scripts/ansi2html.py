#!/usr/bin/env python3
"""Convierte una pantalla ANSI de la TUI (colores de 24 bits) en una página HTML.

Uso: ansi2html.py entrada.ansi salida.html
Solo entiende lo que emite lipgloss: color de frente y fondo, negrita, tachado
e inverso. Lo usa scripts/capturas.sh para las imágenes de la documentación.
"""
import html
import re
import sys

ANSI16 = ["#1c1c20", "#E0607F", "#4FA86F", "#C5D98A", "#6E7FB8", "#B06BC4", "#3FA894", "#9DBCB1",
          "#544E6B", "#F08098", "#8FE7AB", "#DDEDA6", "#9AA8DC", "#D08FE0", "#6FD9C4", "#FBFFFC"]
BG, FG = "#0a0a0c", "#DFEDE5"


def color256(n):
    if n < 16:
        return ANSI16[n]
    if n < 232:
        n -= 16
        r, g, b = n // 36, (n // 6) % 6, n % 6
        return "#%02x%02x%02x" % tuple(0 if v == 0 else 55 + 40 * v for v in (r, g, b))
    v = 8 + 10 * (n - 232)
    return "#%02x%02x%02x" % (v, v, v)


def convert(text):
    out, state = [], {}
    for part in re.split(r"(\x1b\[[0-9;]*m)", text):
        if part.startswith("\x1b["):
            codes = [int(c) if c else 0 for c in part[2:-1].split(";")]
            i = 0
            while i < len(codes):
                c = codes[i]
                if c == 0:
                    state = {}
                elif c == 1:
                    state["bold"] = True
                elif c == 7:
                    state["rev"] = True
                elif c == 9:
                    state["strike"] = True
                elif c in (38, 48):
                    key = "fg" if c == 38 else "bg"
                    if codes[i + 1] == 2:
                        state[key] = "#%02x%02x%02x" % tuple(codes[i + 2:i + 5])
                        i += 4
                    else:
                        state[key] = color256(codes[i + 2])
                        i += 2
                elif 30 <= c <= 37:
                    state["fg"] = ANSI16[c - 30]
                elif 90 <= c <= 97:
                    state["fg"] = ANSI16[c - 82]
                elif 40 <= c <= 47:
                    state["bg"] = ANSI16[c - 40]
                i += 1
            continue
        if not part:
            continue
        fg, bg = state.get("fg", FG), state.get("bg")
        if state.get("rev"):
            fg, bg = bg or BG, fg
        css = "color:%s;" % fg
        if bg:
            css += "background:%s;" % bg
        if state.get("bold"):
            css += "font-weight:700;"
        if state.get("strike"):
            css += "text-decoration:line-through;"
        out.append('<span style="%s">%s</span>' % (css, html.escape(part)))
    return "".join(out)


PAGE = """<!doctype html><meta charset="utf-8"><style>
html,body{margin:0;background:%s}
pre{margin:0;padding:24px 28px;font:14px/1.38 'JetBrainsMono Nerd Font Mono','JetBrains Mono',Menlo,monospace;
color:%s;font-variant-ligatures:none}
</style><pre>%s</pre>"""

if __name__ == "__main__":
    src, dst = sys.argv[1], sys.argv[2]
    with open(src, encoding="utf-8") as f:
        body = convert(f.read())
    with open(dst, "w", encoding="utf-8") as f:
        f.write(PAGE % (BG, FG, body))
    # tamaño de la pantalla (columnas y filas), para que quien llama ajuste la ventana
    lines = re.sub(r"\x1b\[[0-9;]*m", "", open(src, encoding="utf-8").read()).split("\n")
    print(max(len(l) for l in lines), len(lines))
