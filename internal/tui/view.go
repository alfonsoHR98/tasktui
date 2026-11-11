package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/alfonsoHR98/tasktui/internal/core"
	"github.com/alfonsoHR98/tasktui/internal/store"
)

func (m *model) View() string {
	if m.w < 20 || m.h < 6 {
		return ""
	}
	mainH := m.h - 1
	sideW := min(m.w/3, 26)
	rightW := m.w - sideW

	var listBox, detailBox []string
	if rightW >= 120 {
		dw := min(max(rightW*2/5, 44), 70)
		listBox = m.drawList(rightW-dw, mainH)
		detailBox = m.drawDetail(dw, mainH)
	} else {
		lines := 0
		if t := m.selected(); t != nil {
			lines = len(m.detailLines(t, rightW-2))
		}
		dh := min(max(lines+2, 6), max(mainH/2, 3))
		listBox = m.drawList(rightW, mainH-dh)
		detailBox = m.drawDetail(rightW, dh)
	}
	var rightCol []string
	if rightW >= 120 {
		for i := range listBox {
			rightCol = append(rightCol, listBox[i]+detailBox[i])
		}
	} else {
		rightCol = append(append(rightCol, listBox...), detailBox...)
	}
	sideBox := m.drawSide(sideW, mainH)
	frame := make([]string, 0, m.h)
	for i := 0; i < mainH; i++ {
		l, r := "", ""
		if i < len(sideBox) {
			l = sideBox[i]
		}
		if i < len(rightCol) {
			r = rightCol[i]
		}
		frame = append(frame, fit(l, sideW)+fit(r, rightW))
	}
	frame = append(frame, fit(m.statusLine(), m.w))

	switch m.mode {
	case modeInput:
		frame = m.drawInput(frame)
	case modeHelp:
		frame = m.drawHelp(frame)
	}
	return strings.Join(frame, "\n")
}

// ---------- barra lateral ----------

func (m *model) drawSide(w, h int) []string {
	inner := w - 2
	visible := h - 2
	off := scrollFor(m.sideIdx, 0, visible)
	var lines []string
	focused := m.focus == focusSide
	for i := off; i < len(m.side) && len(lines) < visible; i++ {
		it := m.side[i]
		if it.kind == sideHeader {
			lines = append(lines, st(" "+it.label, dimC, true, false, false))
			continue
		}
		count := ""
		if it.count > 0 {
			count = fmt.Sprint(it.count)
		}
		label := " " + it.label
		pad := max(inner-runewidth.StringWidth(label)-runewidth.StringWidth(count)-2, 0)
		sel := i == m.sideIdx
		switch {
		case sel && focused:
			lines = append(lines, fitBG(st(label+strings.Repeat(" ", pad)+count, accent, true, false, true), inner, true))
		case sel:
			lines = append(lines, st(label+strings.Repeat(" ", pad), accent, false, false, false)+st(count, dimC, false, false, false))
		default:
			lines = append(lines, plain(label+strings.Repeat(" ", pad), false)+st(count, dimC, false, false, false))
		}
	}
	title := " tasktui "
	if m.store.Dir() == "" {
		title = " tasktui ☁ "
		if m.store.Notice() != "" {
			title = " tasktui ⚠ "
		}
	}
	return box(title, w, h, lines, focused)
}

// scrollFor mantiene el elemento sel visible en una ventana de `visible` filas.
func scrollFor(sel, off, visible int) int {
	if visible <= 0 {
		return 0
	}
	if sel < off {
		return sel
	}
	if sel >= off+visible {
		return sel - visible + 1
	}
	return off
}

// ---------- lista ----------

func relColor(d core.Date, today core.Date, done bool) lipgloss.Color {
	switch {
	case done:
		return dimC
	case d.Before(today):
		return redC
	case d.Equal(today):
		return greenC
	}
	return cyanC
}

// deadlineStyle: rojo si ya pasó o es hoy, amarillo si faltan 2 días o menos.
func deadlineText(s string, d, today core.Date, sel bool) string {
	switch n := d.DaysSince(today); {
	case n <= 0:
		return st(s, redC, true, false, sel)
	case n <= 2:
		return st(s, yellowC, false, false, sel)
	}
	return st(s, dimC, false, false, sel)
}

func (m *model) drawList(w, h int) []string {
	title := fmt.Sprintf(" %s · %d ", m.view.Title(), len(m.taskRows))
	if m.query != "" {
		title += fmt.Sprintf("· /%s ", m.query)
	}
	focused := m.focus == focusList
	if len(m.taskRows) == 0 {
		msg := "  Nada pendiente por aquí ✨   (a = añadir tarea)"
		if m.query != "" {
			msg = "  Sin resultados   (Esc = limpiar búsqueda)"
		}
		return box(title, w, h, []string{"", st(msg, dimC, false, false, false)}, focused)
	}
	showProject := m.view.Kind != core.ViewProject && m.view.Kind != core.ViewInbox && m.view.Kind != core.ViewAll
	today := m.today
	var tasks []*core.Task
	for _, r := range m.rows {
		if r.header == "" {
			if t := m.store.TaskAt(r.pos); t != nil {
				tasks = append(tasks, t)
			}
		}
	}
	// Cada columna mide lo que necesita su contenido más largo.
	whenW, dlW, projW, iconW := 0, 0, 0, 0
	for _, t := range tasks {
		if t.Due != nil {
			n := runewidth.StringWidth(core.Rel(*t.Due, today))
			if t.Time != nil {
				n += 6
			}
			whenW = max(whenW, n)
		}
		if !t.Done && t.Deadline != nil {
			dlW = max(dlW, runewidth.StringWidth(core.Rel(*t.Deadline, today))+3)
		}
		if showProject {
			projW = min(max(projW, runewidth.StringWidth(t.Project)), 18)
		}
		n := 0
		if strings.TrimSpace(t.Description) != "" {
			n += 2
		}
		if len(t.Notes) > 0 {
			n += 2 + len(fmt.Sprint(len(t.Notes)))
		}
		if t.Recurrence != nil {
			n++
		}
		iconW = max(iconW, n)
	}
	inner := w - 2
	cols := 3
	if showProject {
		cols = 4
	}
	textW := max(inner-(whenW+dlW+projW+iconW+cols+1), 10)

	var rows []string
	selRow := -1
	if m.sel < len(m.taskRows) {
		selRow = m.taskRows[m.sel]
	}
	for i, r := range m.rows {
		if r.header != "" {
			rows = append(rows, st(" "+r.header, accent, true, false, false))
			continue
		}
		t := m.store.TaskAt(r.pos)
		if t == nil {
			rows = append(rows, "")
			continue
		}
		sel := focused && i == selRow
		rows = append(rows, m.taskRow(t, textW, whenW, dlW, projW, iconW, showProject, sel, inner))
	}
	visible := h - 2
	m.scroll = scrollFor(selRow, m.scroll, visible)
	if m.scroll > len(rows) {
		m.scroll = 0
	}
	end := min(m.scroll+visible, len(rows))
	return box(title, w, h, rows[m.scroll:end], focused)
}

func (m *model) taskRow(t *core.Task, textW, whenW, dlW, projW, iconW int, showProject, sel bool, inner int) string {
	today := m.today
	indent := strings.Repeat(" ", len(strings.ReplaceAll(t.Indent, "\t", "  "))/2*2)
	check := "○ "
	if t.Done {
		check = "✓ "
	}
	// Texto + etiquetas; si no caben, las etiquetas se quitan y el texto se corta con «…».
	var lb strings.Builder
	for _, l := range t.Labels {
		lb.WriteString("  #" + l)
	}
	labels := lb.String()
	room := max(textW-runewidth.StringWidth(indent)-runewidth.StringWidth(check), 0)
	text := t.Text
	if runewidth.StringWidth(text)+runewidth.StringWidth(labels) > room {
		text, labels = truncate(text, room), ""
	}
	textFG := lipgloss.Color("")
	if t.Done {
		textFG = dimC
	}
	cell := plain(indent, sel) + st(check, prioColor(t.Priority), true, false, sel) +
		st(text, textFG, false, t.Done, sel) + st(labels, magentaC, false, false, sel)

	sp := plain(" ", sel)
	var sb strings.Builder
	sb.WriteString(fitBG(cell, textW, sel) + sp)
	when := ""
	if t.Due != nil {
		s := core.Rel(*t.Due, today)
		if t.Time != nil {
			s += " " + t.Time.String()
		}
		when = st(s, relColor(*t.Due, today, t.Done), false, false, sel)
	}
	sb.WriteString(fitBG(when, whenW, sel) + sp)
	dl := ""
	if t.Deadline != nil && !t.Done {
		dl = deadlineText("🎯 "+core.Rel(*t.Deadline, today), *t.Deadline, today, sel)
	}
	sb.WriteString(fitBG(dl, dlW, sel) + sp)
	if showProject {
		sb.WriteString(fitBG(st(truncate(t.Project, 18), dimC, false, false, sel), projW, sel) + sp)
	}
	var icons string
	if strings.TrimSpace(t.Description) != "" {
		icons += plain("¶", sel)
	}
	if len(t.Notes) > 0 {
		icons += st(fmt.Sprintf("✎%d", len(t.Notes)), dimC, false, false, sel)
	}
	if t.Recurrence != nil {
		icons += st("↻", cyanC, false, false, sel)
	}
	sb.WriteString(fitBG(icons, iconW, sel))
	return fitBG(sb.String(), inner, sel)
}

// ---------- detalle ----------

func relativeText(d, today core.Date, done bool) string {
	if done {
		return ""
	}
	switch n := d.DaysSince(today); {
	case n == 0:
		return "  · hoy"
	case n == 1:
		return "  · mañana"
	case n == -1:
		return "  · venció ayer"
	case n < 0:
		return fmt.Sprintf("  · venció hace %d días", -n)
	default:
		return fmt.Sprintf("  · en %d días", n)
	}
}

func (m *model) drawDetail(w, h int) []string {
	t := m.selected()
	if t == nil {
		return box(" Detalle ", w, h, nil, false)
	}
	return box(" Detalle ", w, h, m.detailLines(t, w-2), false)
}

// detailLines es el detalle de la tarea: encabezado, campos con etiqueta,
// descripción y notas, ya ajustado al ancho.
func (m *model) detailLines(t *core.Task, w int) []string {
	today := m.today
	dim := func(s string) string { return st(s, dimC, false, false, false) }
	label := func(s string) string {
		return dim(s + strings.Repeat(" ", max(14-runewidth.StringWidth(s), 0)))
	}
	section := func(s string) string { return st(s, accent, true, false, false) }
	where := t.Project
	if t.Section != nil {
		where += " › " + *t.Section
	}
	lines := []string{st(t.Text, "", true, false, false), dim(where), ""}
	short := func(d core.Date) string { return strings.SplitN(core.Long(d, today), " · ", 2)[0] }
	if t.Due != nil {
		s := short(*t.Due)
		if t.Time != nil {
			s += " · " + t.Time.String()
		}
		lines = append(lines, label("◷ Fecha")+st(s, relColor(*t.Due, today, t.Done), false, false, false)+dim(relativeText(*t.Due, today, t.Done)))
	}
	if t.Deadline != nil {
		lines = append(lines, label("🎯 Límite")+deadlineText(short(*t.Deadline), *t.Deadline, today, false)+dim(relativeText(*t.Deadline, today, t.Done)))
	}
	if t.Priority < 4 && t.Priority >= 1 {
		name := [...]string{"alta", "media", "baja"}[t.Priority-1]
		lines = append(lines, label("⚑ Prioridad")+st(fmt.Sprintf("p%d · %s", t.Priority, name), prioColor(t.Priority), false, false, false))
	}
	if t.Recurrence != nil {
		lines = append(lines, label("↻ Repite")+t.Recurrence.ToES())
	}
	if len(t.Labels) > 0 {
		ls := make([]string, len(t.Labels))
		for i, l := range t.Labels {
			ls[i] = "#" + l
		}
		lines = append(lines, label("# Etiquetas")+st(strings.Join(ls, "  "), magentaC, false, false, false))
	}
	if t.Done && t.DoneDate != nil {
		lines = append(lines, label("✓ Hecha")+core.Long(*t.DoneDate, today))
	}
	if strings.TrimSpace(t.Description) != "" {
		lines = append(lines, "", section("Descripción"))
		lines = append(lines, strings.Split(t.Description, "\n")...)
	}
	if len(t.Notes) > 0 {
		lines = append(lines, "", section(fmt.Sprintf("Notas (%d)", len(t.Notes))))
		for _, n := range t.Notes {
			when := ""
			if n.Date != nil {
				when = core.Rel(*n.Date, today) + " · "
			}
			lines = append(lines, dim("› "+when)+n.Text)
		}
	}
	id := "—"
	if t.ID != nil {
		id = *t.ID
	}
	foot := "id " + id
	if len(t.Extra) > 0 {
		foot += "   " + strings.Join(t.Extra, " ")
	}
	lines = append(lines, "", dim(foot))
	var out []string
	for _, l := range lines {
		out = append(out, strings.Split(ansi.Wrap(l, max(w, 1), ""), "\n")...)
	}
	return out
}

// ---------- barra de estado ----------

func (m *model) statusLine() string {
	switch {
	case m.mode == modeConfirm:
		name := ""
		if m.inProj != "" {
			n := m.countOpenAll(m.inProj)
			return lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(redC).Render(fit(fmt.Sprintf(" ¿Eliminar el proyecto «%s» y sus %d tareas? (y/n)", m.inProj, n), m.w))
		}
		if t := m.selected(); t != nil {
			name = t.Text
		}
		return lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(redC).Render(fit(fmt.Sprintf(" ¿Eliminar «%s»? (y/n)", name), m.w))
	case m.msg != "":
		return st(" "+m.msg, yellowC, false, false, false)
	}
	return st(" a añadir · x completar · e editar · s fecha · i descripción · n nota · d borrar · / buscar · ? ayuda", dimC, false, false, false)
}

// ---------- ventana de captura ----------

func (m *model) drawInput(frame []string) []string {
	W, H := m.w, m.h
	w := min(W-4, 80)
	_, opts := m.completions()
	nSugg := 0
	if len(opts) > 0 {
		nSugg = len(opts) + 1
	}
	h := min(6+nSugg, H)
	x, y := (W-w)/2, min(H/3, H-h)
	title := ""
	switch m.inKind {
	case inAdd:
		title = fmt.Sprintf(" Nueva tarea → %s ", m.defaultProject())
	case inEdit:
		title = " Editar tarea "
	case inSchedule:
		title = " Fecha (mañana 18:00, viernes, 15/11 limit 20/11; vacío = sin fecha) "
	case inNote:
		title = " Nueva nota (se guarda con la fecha de hoy) "
	case inSearch:
		title = " Buscar "
	}
	inner := w - 2
	lines := []string{m.inputLine(inner - 1), "", "", ""}
	dim := func(s string) string { return st(s, dimC, false, false, false) }
	var preview, hint string
	switch m.inKind {
	case inAdd, inEdit:
		preview = m.preview(core.ParseQuick(m.input.text(), m.today), m.inKind == inAdd)
		hint = dim(`#etiqueta  +Proyecto  p1-p4  mañana 18:00  limit 20/11  cada lunes  \literal`)
	case inSchedule:
		q := core.ParseQuick(m.input.text(), m.today)
		var desc []string
		if q.Due != nil {
			s := "◷ " + core.Long(*q.Due, m.today)
			if q.Time != nil {
				s += " · " + q.Time.String()
			}
			desc = append(desc, s)
		}
		if q.Deadline != nil {
			desc = append(desc, "🎯 límite "+core.Long(*q.Deadline, m.today))
		}
		switch {
		case len(m.input.buf) == 0:
			preview = dim("→ sin fecha")
		case len(desc) > 0:
			preview = st("→ "+strings.Join(desc, "   "), greenC, false, false, false)
		default:
			preview = st("→ ?", redC, false, false, false)
		}
		hint = dim("Enter guardar · Esc cancelar")
	case inNote:
		hint = dim("Enter guardar · Esc cancelar")
	case inSearch:
		hint = dim("Enter fijar · Esc limpiar")
	}
	lines[2], lines[3] = preview, hint
	if len(opts) > 0 {
		sel := min(m.sugg, len(opts)-1)
		lines = append(lines, dim("Tab completar · ↑↓ elegir"))
		for i, o := range opts {
			mark := " "
			if i == sel {
				mark = "›"
				lines = append(lines, st(fmt.Sprintf(" %s %s ", mark, o), lipgloss.Color("15"), false, false, true))
			} else {
				lines = append(lines, st(fmt.Sprintf(" %s %s ", mark, o), grayC, false, false, false))
			}
		}
	}
	return overlay(frame, box(title, w, h, lines, true), x, y)
}

// inputLine dibuja el texto con desplazamiento horizontal y el cursor en vídeo
// inverso, para que siempre se vea dónde se escribe.
func (m *model) inputLine(avail int) string {
	buf, cur := m.input.buf, m.input.cur
	start := 0
	for start < cur && runewidth.StringWidth(string(buf[start:cur])) > avail-1 {
		start++
	}
	cursor := lipgloss.NewStyle().Reverse(true)
	before := string(buf[start:cur])
	if cur < len(buf) {
		return before + cursor.Render(string(buf[cur])) + string(buf[cur+1:])
	}
	return before + cursor.Render(" ")
}

// preview muestra cómo se interpretará la captura rápida.
func (m *model) preview(q core.Quick, adding bool) string {
	out := st("→ ", dimC, false, false, false)
	push := func(s string, c lipgloss.Color) { out += st(s, c, false, false, false) + "  " }
	due := q.Due
	if due == nil && adding && m.view.Kind == core.ViewToday {
		due = m.today.Ptr()
	}
	if due != nil {
		s := "◷ " + core.Long(*due, m.today)
		if q.Time != nil {
			s += " · " + q.Time.String()
		}
		push(s, greenC)
	}
	if q.Deadline != nil {
		push("🎯 límite "+core.Long(*q.Deadline, m.today), redC)
	}
	if q.Priority != nil && *q.Priority < 4 {
		push(fmt.Sprintf("p%d", *q.Priority), prioColor(*q.Priority))
	}
	if q.Recurrence != nil {
		push("↻ "+q.Recurrence.ToES(), cyanC)
	}
	for _, l := range q.Labels {
		push("#"+l, magentaC)
	}
	if q.Project != nil {
		push("▸ "+*q.Project, blueC)
	}
	return out
}

// ---------- ayuda ----------

func (m *model) drawHelp(frame []string) []string {
	keys := [][2]string{
		{"Navegación", ""},
		{"j / k  ↓ / ↑", "moverse"},
		{"h / l  Tab", "barra lateral ↔ lista"},
		{"gi gt gu ga", "ir a Inbox · Hoy · Próximos · Todas"},
		{"gg / G", "inicio / final"},
		{"/  Esc", "buscar · limpiar búsqueda"},
		{"H", "mostrar/ocultar completadas"},
		{"", ""},
		{"Tareas", ""},
		{"a", "añadir (captura rápida)"},
		{"Tab", "al escribir: completar +proyecto o #etiqueta"},
		{"e", "editar"},
		{"x  espacio", "completar / reabrir"},
		{"s", "fecha, hora y límite (mañana 18:00 limit 20/11)"},
		{"i", "editar la descripción en $EDITOR"},
		{"n", "añadir una nota con fecha"},
		{"t", "poner para hoy"},
		{"1 2 3 4", "prioridad"},
		{"d", "eliminar (en el panel de proyectos: el proyecto)"},
		{"u", "deshacer"},
		{"r", "recargar desde disco"},
		{"q", "salir"},
	}
	var lines []string
	for _, k := range keys {
		if k[1] == "" {
			lines = append(lines, st(" "+k[0], accent, true, false, false))
		} else {
			lines = append(lines, st(fmt.Sprintf("   %-14s", k[0]), yellowC, false, false, false)+k[1])
		}
	}
	w, h := min(56, m.w), min(len(lines)+2, m.h)
	return overlay(frame, box(" Atajos · cualquier tecla para cerrar ", w, h, lines, true), (m.w-w)/2, (m.h-h)/2)
}

var _ store.Store
