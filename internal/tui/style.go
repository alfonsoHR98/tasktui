package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

var (
	selBG    = lipgloss.Color("237")
	accent   = lipgloss.Color("1") // rojo
	dimC     = lipgloss.Color("8")
	redC     = lipgloss.Color("1")
	greenC   = lipgloss.Color("2")
	yellowC  = lipgloss.Color("3")
	blueC    = lipgloss.Color("4")
	magentaC = lipgloss.Color("5")
	cyanC    = lipgloss.Color("6")
	grayC    = lipgloss.Color("7")
)

func prioColor(p int) lipgloss.Color {
	switch p {
	case 1:
		return redC
	case 2:
		return yellowC
	case 3:
		return blueC
	}
	return grayC
}

// st escribe texto con color, negrita o tachado, y con fondo si sel es true.
func st(s string, fg lipgloss.Color, bold, strike, sel bool) string {
	if s == "" {
		return ""
	}
	style := lipgloss.NewStyle()
	if fg != "" {
		style = style.Foreground(fg)
	}
	if bold {
		style = style.Bold(true)
	}
	if strike {
		style = style.Strikethrough(true)
	}
	if sel {
		style = style.Background(selBG)
	}
	return style.Render(s)
}

func plain(s string, sel bool) string { return st(s, "", false, false, sel) }

func width(s string) int { return lipgloss.Width(s) }

// fit corta o rellena una línea (con ANSI) a exactamente w columnas.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if n := width(s); n > w {
		return ansi.Truncate(s, w, "")
	} else if n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

// fitBG es como fit pero rellena con fondo de selección.
func fitBG(s string, w int, sel bool) string {
	n := width(s)
	if n >= w || !sel {
		return fit(s, w)
	}
	return s + lipgloss.NewStyle().Background(selBG).Render(strings.Repeat(" ", w-n))
}

// truncate corta a w columnas con «…» si no cabe.
func truncate(s string, w int) string {
	if runewidth.StringWidth(s) <= w {
		return s
	}
	return runewidth.Truncate(s, w, "…")
}

// box dibuja un marco con título; lines se ajustan a w-2 columnas y h-2 filas.
func box(title string, w, h int, lines []string, focused bool) []string {
	if w < 4 || h < 2 {
		return nil
	}
	color := dimC
	if focused {
		color = accent
	}
	b := func(s string) string { return lipgloss.NewStyle().Foreground(color).Render(s) }
	t := lipgloss.NewStyle().Foreground(color).Bold(true).Render(truncate(title, w-4))
	top := b("┌") + t + b(strings.Repeat("─", max(0, w-2-width(t)))+"┐")
	out := []string{top}
	for i := 0; i < h-2; i++ {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		out = append(out, b("│")+fit(l, w-2)+b("│"))
	}
	return append(out, b("└"+strings.Repeat("─", w-2)+"┘"))
}

// overlay pone popup sobre base en (x, y).
func overlay(base []string, popup []string, x, y int) []string {
	out := append([]string(nil), base...)
	for i, pl := range popup {
		row := y + i
		if row < 0 || row >= len(out) {
			continue
		}
		pw := width(pl)
		left := ansi.Truncate(out[row], x, "")
		left += strings.Repeat(" ", max(0, x-width(left)))
		right := ansi.TruncateLeft(out[row], x+pw, "")
		out[row] = left + pl + right
	}
	return out
}

// wrap parte un texto largo en líneas de w columnas (sin ANSI).
func wrap(s string, w int) []string {
	if w <= 0 {
		return []string{s}
	}
	return strings.Split(ansi.Wrap(s, w, ""), "\n")
}
