package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// Colores de la interfaz. Los fija applyTheme; por defecto, Aurora.
var (
	selBG    lipgloss.Color // fondo de la fila elegida
	accent   lipgloss.Color // foco, marca y títulos
	onAccent lipgloss.Color // texto sobre un fondo de color
	borderC  lipgloss.Color // marcos sin foco y reglas
	dimC     lipgloss.Color // datos secundarios
	hintC    lipgloss.Color // ayudas que sí deben leerse
	redC     lipgloss.Color
	greenC   lipgloss.Color
	yellowC  lipgloss.Color
	blueC    lipgloss.Color
	magentaC lipgloss.Color
	cyanC    lipgloss.Color
	grayC    lipgloss.Color

	// Rampas de tonos vecinos para los degradados. En el tema ansi quedan
	// vacías y todo se pinta en un solo color.
	auroraRamp []string // avance: de verde azulado a verde claro
	fadeRamp   []string // reglas: del borde hacia el fondo
)

func init() { applyTheme("") }

// applyTheme elige la paleta: "aurora" (por defecto, color verdadero) o "ansi",
// que usa los 16 colores de la terminal y así sigue el tema que tengas puesto.
func applyTheme(name string) {
	if strings.EqualFold(strings.TrimSpace(name), "ansi") {
		selBG, accent, onAccent, borderC = "237", "2", "0", "8"
		dimC, hintC = "8", "7"
		redC, greenC, yellowC, blueC, magentaC, cyanC, grayC = "1", "2", "3", "4", "5", "6", "7"
		auroraRamp, fadeRamp = nil, nil
		return
	}
	selBG, accent, onAccent, borderC = "#2A4A3E", "#59C98D", "#0a0a0c", "#3A3A44"
	dimC, hintC = "#6E6787", "#8FA89F"
	redC, greenC, yellowC, blueC = "#E0607F", "#59C98D", "#C5D98A", "#9AA8DC"
	magentaC, cyanC, grayC = "#B06BC4", "#6FD9C4", "#9DBCB1"
	auroraRamp = []string{"#3FA894", "#59C98D", "#8FE7AB"}
	fadeRamp = []string{"#4A4A58", "#3A3A44", "#1C1C20"}
}

// mix interpola dentro de una rampa de colores «#rrggbb»; t va de 0 a 1.
func mix(stops []string, t float64) lipgloss.Color {
	t = min(max(t, 0), 1) * float64(len(stops)-1)
	i := min(int(t), len(stops)-2)
	f := t - float64(i)
	a, _ := strconv.ParseUint(stops[i][1:], 16, 32)
	b, _ := strconv.ParseUint(stops[i+1][1:], 16, 32)
	ch := func(shift uint) uint64 {
		x, y := float64(a>>shift&0xff), float64(b>>shift&0xff)
		return uint64(x + (y-x)*f + 0.5)
	}
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", ch(16), ch(8), ch(0)))
}

// gradient repite ch a lo largo de n columnas recorriendo la rampa desde la
// posición from hasta to (de 0 a 1). Sin rampa usa el color fallback.
func gradient(ch string, n int, stops []string, from, to float64, fallback lipgloss.Color) string {
	if n <= 0 {
		return ""
	}
	if len(stops) < 2 {
		return st(strings.Repeat(ch, n), fallback, false, false, false)
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		t := from
		if n > 1 {
			t += (to - from) * float64(i) / float64(n-1)
		}
		b.WriteString(st(ch, mix(stops, t), false, false, false))
	}
	return b.String()
}

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

// pill es una etiqueta con fondo de color, como el modo de una barra de estado.
func pill(s string, bg lipgloss.Color) string {
	return lipgloss.NewStyle().Foreground(onAccent).Background(bg).Bold(true).Render(" " + s + " ")
}

// rule es una línea horizontal tenue de w columnas que se desvanece hacia la
// derecha.
func rule(w int) string {
	return gradient("─", w, fadeRamp, 0, 1, borderC)
}

// center pone una línea (con ANSI) al centro de w columnas.
func center(s string, w int) string {
	return strings.Repeat(" ", max((w-width(s))/2, 0)) + s
}

// box dibuja un marco redondeado con título; lines se ajustan a w-2 columnas y
// h-2 filas. El marco con foco va en el color de acento.
func box(title string, w, h int, lines []string, focused bool) []string {
	if w < 4 || h < 2 {
		return nil
	}
	color, titleC := borderC, grayC
	if focused {
		color, titleC = accent, accent
	}
	b := func(s string) string { return lipgloss.NewStyle().Foreground(color).Render(s) }
	t := ""
	if title = strings.TrimSpace(title); title != "" && w >= 8 {
		t = lipgloss.NewStyle().Foreground(titleC).Bold(true).Render(" " + truncate(title, w-6) + " ")
	}
	lead := "─"
	if t == "" {
		lead = ""
	}
	top := b("╭"+lead) + t + b(strings.Repeat("─", max(0, w-2-width(lead)-width(t)))+"╮")
	out := []string{top}
	for i := 0; i < h-2; i++ {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		out = append(out, b("│")+fit(l, w-2)+b("│"))
	}
	return append(out, b("╰"+strings.Repeat("─", w-2)+"╯"))
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
