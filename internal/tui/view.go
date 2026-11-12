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

// sideIconColor da a cada vista su color en la barra lateral.
func sideIconColor(v core.View) lipgloss.Color {
	switch v.Kind {
	case core.ViewInbox:
		return blueC
	case core.ViewToday:
		return greenC
	case core.ViewUpcoming:
		return cyanC
	case core.ViewLabel:
		return magentaC
	case core.ViewProject:
		return yellowC
	}
	return grayC
}

func (m *model) drawSide(w, h int) []string {
	inner := w - 2
	visible := h - 2
	focused := m.focus == focusSide
	var lines []string
	selLine := 0
	for i, it := range m.side {
		if it.kind == sideHeader {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, st("  "+it.label, dimC, true, false, false))
			continue
		}
		count := ""
		if it.count > 0 {
			count = fmt.Sprint(it.count)
		}
		icon, name, _ := strings.Cut(it.label, " ")
		name = truncate(name, max(inner-6-runewidth.StringWidth(count), 4))
		pad := strings.Repeat(" ", max(inner-4-runewidth.StringWidth(icon)-runewidth.StringWidth(name)-runewidth.StringWidth(count), 1))
		sel := i == m.sideIdx
		bg := sel && focused
		if sel {
			selLine = len(lines)
		}
		mark, nameC, countC := plain(" ", false), lipgloss.Color(""), dimC
		switch {
		case bg:
			mark, nameC, countC = st("▌", accent, false, false, true), "", grayC
		case sel:
			mark, nameC = st("▌", borderC, false, false, false), accent
		}
		line := mark + st(icon+" ", sideIconColor(it.view), false, false, bg) + st(name, nameC, sel, false, bg) +
			plain(pad, bg) + st(count, countC, false, false, bg) + plain(" ", bg)
		lines = append(lines, fitBG(line, inner, bg))
	}
	// Pie: avance del día (hechas hoy frente a lo que toca hoy).
	foot := m.todayProgress(inner)
	room := visible
	if len(foot) > 0 && visible >= len(lines)+len(foot)+1 {
		room = visible - len(foot)
	} else {
		foot = nil
	}
	off := scrollFor(selLine, 0, room)
	if off > 0 && lines[off] == "" {
		off++
	}
	shown := lines[off:min(off+room, len(lines))]
	if foot != nil {
		shown = append(append([]string(nil), shown...), make([]string, room-len(shown))...)
		shown = append(shown, foot...)
	}
	title := "tasktui"
	if m.store.Dir() == "" {
		title = "tasktui ☁"
		if m.store.Notice() != "" {
			title = "tasktui ⚠"
		}
	}
	return box(title, w, h, shown, focused)
}

// todayProgress dibuja la barra de avance de hoy; nil si hoy no hay nada.
func (m *model) todayProgress(w int) []string {
	done, open := 0, 0
	for _, e := range m.store.Tasks() {
		t := e.Task
		switch {
		case t.Done && t.DoneDate != nil && t.DoneDate.Equal(m.today):
			done++
		case !t.Done && t.Due != nil && !t.Due.After(m.today):
			open++
		}
	}
	total := done + open
	if total == 0 || w < 16 {
		return nil
	}
	tag := fmt.Sprintf("%d/%d", done, total)
	barW := w - 4
	fill := done * barW / total
	// Lo avanzado recorre la rampa: cuanto más completas, más claro llega.
	bar := gradient("━", fill, auroraRamp, 0, float64(fill)/float64(max(barW, 1)), accent) +
		st(strings.Repeat("━", barW-fill), borderC, false, false, false)
	head := st("  hoy", dimC, true, false, false)
	return []string{
		head + strings.Repeat(" ", max(w-2-width(head)-len(tag), 1)) + st(tag, grayC, false, false, false),
		"  " + bar,
	}
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
	title := fmt.Sprintf("%s · %d", m.view.Title(), len(m.taskRows))
	if m.query != "" {
		title += " · /" + m.query
	}
	focused := m.focus == focusList
	inner := w - 2
	if len(m.taskRows) == 0 {
		return box(title, w, h, m.emptyList(inner, h-2), focused)
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
			dlW = max(dlW, runewidth.StringWidth(core.Rel(*t.Deadline, today))+2)
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
	rowW := inner - 1 // la primera columna es la marca de selección
	cols := 3
	if showProject {
		cols = 4
	}
	textW := max(rowW-(whenW+dlW+projW+iconW+cols+1), 10)

	var rows []string
	selTask, selRow := -1, 0
	if m.sel < len(m.taskRows) {
		selTask = m.taskRows[m.sel]
	}
	for i, r := range m.rows {
		if r.header != "" {
			if len(rows) > 0 {
				rows = append(rows, "")
			}
			rows = append(rows, groupHeader(r.header, inner))
			continue
		}
		t := m.store.TaskAt(r.pos)
		if t == nil {
			rows = append(rows, "")
			continue
		}
		isSel := i == selTask
		sel := focused && isSel
		mark := plain(" ", false)
		switch {
		case sel:
			mark = st("▌", accent, false, false, true)
		case isSel:
			mark = st("▌", borderC, false, false, false)
		}
		if isSel {
			selRow = len(rows)
		}
		rows = append(rows, mark+m.taskRow(t, textW, whenW, dlW, projW, iconW, showProject, sel, rowW))
	}
	visible := h - 2
	m.scroll = scrollFor(selRow, m.scroll, visible)
	// Al subir a la primera tarea de un grupo, que se vea también su encabezado.
	if m.scroll > 0 && m.scroll == selRow && selTask > 0 && m.rows[selTask-1].header != "" {
		m.scroll--
	}
	if m.scroll > len(rows) {
		m.scroll = 0
	}
	end := min(m.scroll+visible, len(rows))
	return box(title, w, h, rows[m.scroll:end], focused)
}

// groupHeader es el encabezado de un grupo: nombre y una regla hasta el borde.
func groupHeader(name string, w int) string {
	c := blueC
	if strings.HasPrefix(name, "Vencidas") {
		c = redC
	}
	h := st(" "+name+" ", c, true, false, false)
	return h + rule(w-width(h)-1)
}

// emptyList es lo que se ve cuando la vista no tiene tareas.
func (m *model) emptyList(w, h int) []string {
	dim := func(s string) string { return st(s, dimC, false, false, false) }
	keyHint := func(k, what string) string { return st(k, accent, true, false, false) + dim("  "+what) }
	body := []string{
		st("✓", accent, true, false, false),
		"",
		st("Nada pendiente por aquí", grayC, true, false, false),
		"",
		keyHint("a", "añadir tarea"),
	}
	if m.query != "" {
		body = []string{
			st("∅", yellowC, true, false, false),
			"",
			st("Sin resultados para «"+truncate(m.query, max(w-24, 4))+"»", grayC, true, false, false),
			"",
			keyHint("Esc", "limpiar búsqueda"),
		}
	}
	out := make([]string, max((h-len(body))/3, 1))
	for _, l := range body {
		out = append(out, center(l, w))
	}
	return out
}

func (m *model) taskRow(t *core.Task, textW, whenW, dlW, projW, iconW int, showProject, sel bool, inner int) string {
	today := m.today
	indent := strings.Repeat(" ", len(strings.ReplaceAll(t.Indent, "\t", "  "))/2*2)
	check, checkC := "○ ", prioColor(t.Priority)
	switch {
	case t.Done:
		check, checkC = "✓ ", dimC
	case t.Priority == 1:
		check = "◉ "
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
	cell := plain(indent, sel) + st(check, checkC, true, false, sel) +
		st(text, textFG, sel, t.Done, sel) + st(labels, magentaC, false, false, sel)

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
		dl = deadlineText("⚑ "+core.Rel(*t.Deadline, today), *t.Deadline, today, sel)
	}
	sb.WriteString(fitBG(dl, dlW, sel) + sp)
	if showProject {
		sb.WriteString(fitBG(st(truncate(t.Project, 18), dimC, false, false, sel), projW, sel) + sp)
	}
	var icons string
	if strings.TrimSpace(t.Description) != "" {
		icons += st("¶", hintC, false, false, sel)
	}
	if len(t.Notes) > 0 {
		icons += st(fmt.Sprintf("✎%d", len(t.Notes)), hintC, false, false, sel)
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
		empty := append(make([]string, max((h-2)/3, 1)), center(st("Elige una tarea para ver su detalle", dimC, false, false, false), w-2))
		return box("Detalle", w, h, empty, false)
	}
	return box("Detalle", w, h, m.detailLines(t, w-2), false)
}

// detailLines es el detalle de la tarea: encabezado, campos con etiqueta,
// descripción y notas, ya ajustado al ancho.
func (m *model) detailLines(t *core.Task, w int) []string {
	today := m.today
	w = max(w-2, 1) // un margen a cada lado
	dim := func(s string) string { return st(s, dimC, false, false, false) }
	label := func(icon string, c lipgloss.Color, s string) string {
		return st(icon+" ", c, false, false, false) + dim(s+strings.Repeat(" ", max(11-runewidth.StringWidth(s), 0)))
	}
	section := func(s string) string {
		h := st(s+" ", accent, true, false, false)
		return h + rule(w-width(h))
	}
	where := "▸ " + t.Project
	if t.Section != nil {
		where += " › " + *t.Section
	}
	title := st(t.Text, "", true, t.Done, false)
	lines := []string{title, st(where, blueC, false, false, false), ""}
	short := func(d core.Date) string { return strings.SplitN(core.Long(d, today), " · ", 2)[0] }
	if t.Due != nil {
		s := short(*t.Due)
		if t.Time != nil {
			s += " · " + t.Time.String()
		}
		c := relColor(*t.Due, today, t.Done)
		lines = append(lines, label("◷", c, "Fecha")+st(s, c, false, false, false)+dim(relativeText(*t.Due, today, t.Done)))
	}
	if t.Deadline != nil {
		lines = append(lines, label("⚑", redC, "Límite")+deadlineText(short(*t.Deadline), *t.Deadline, today, false)+dim(relativeText(*t.Deadline, today, t.Done)))
	}
	if t.Priority < 4 && t.Priority >= 1 {
		name := [...]string{"alta", "media", "baja"}[t.Priority-1]
		c := prioColor(t.Priority)
		lines = append(lines, label("▲", c, "Prioridad")+st(fmt.Sprintf("p%d · %s", t.Priority, name), c, false, false, false))
	}
	if t.Recurrence != nil {
		lines = append(lines, label("↻", cyanC, "Repite")+t.Recurrence.ToES())
	}
	if len(t.Labels) > 0 {
		ls := make([]string, len(t.Labels))
		for i, l := range t.Labels {
			ls[i] = "#" + l
		}
		lines = append(lines, label("#", magentaC, "Etiquetas")+st(strings.Join(ls, "  "), magentaC, false, false, false))
	}
	if t.Done && t.DoneDate != nil {
		lines = append(lines, label("✓", greenC, "Hecha")+core.Long(*t.DoneDate, today))
	}
	if strings.TrimSpace(t.Description) != "" {
		lines = append(lines, "", section("Descripción"))
		lines = append(lines, strings.Split(t.Description, "\n")...)
	}
	if len(t.Notes) > 0 {
		lines = append(lines, "", section(fmt.Sprintf("Notas · %d", len(t.Notes))))
		for _, n := range t.Notes {
			when := ""
			if n.Date != nil {
				when = core.Rel(*n.Date, today) + "  "
			}
			// Cada renglón de la nota conserva la barra de la izquierda.
			for j, part := range strings.Split(ansi.Wrap(n.Text, max(w-2-runewidth.StringWidth(when), 8), ""), "\n") {
				lead := dim(when)
				if j > 0 {
					lead = strings.Repeat(" ", runewidth.StringWidth(when))
				}
				lines = append(lines, st("│ ", borderC, false, false, false)+lead+part)
			}
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
		for _, part := range strings.Split(ansi.Wrap(l, w, ""), "\n") {
			out = append(out, " "+part)
		}
	}
	return out
}

// ---------- barra de estado ----------

// statusLine es la barra de abajo: modo, atajos (o un mensaje) y, a la derecha,
// dónde viven las tareas y la fecha.
func (m *model) statusLine() string {
	if m.mode == modeConfirm {
		q := ""
		if m.inProj != "" {
			q = fmt.Sprintf("¿Eliminar el proyecto «%s» y sus %d tareas?", m.inProj, m.countOpenAll(m.inProj))
		} else if t := m.selected(); t != nil {
			q = fmt.Sprintf("¿Eliminar «%s»?", t.Text)
		}
		keys := "  " + st("y", redC, true, false, false) + st(" sí · ", dimC, false, false, false) +
			st("n", accent, true, false, false) + st(" no", dimC, false, false, false)
		left := pill("ELIMINAR", redC) + " "
		return left + st(truncate(q, max(m.w-width(left)-width(keys)-1, 8)), redC, true, false, false) + keys
	}
	name, bg := "NORMAL", accent
	switch m.mode {
	case modeInput:
		name, bg = "ESCRIBIR", blueC
		if m.inKind == inSearch {
			name, bg = "BUSCAR", yellowC
		}
	case modeHelp:
		name, bg = "AYUDA", magentaC
	}
	left := pill(name, bg) + " "
	where := "▤ local"
	whereC := dimC
	if m.store.Dir() == "" {
		where = "☁ remoto"
		if m.store.Notice() != "" {
			where, whereC = "⚠ sin conexión", yellowC
		}
	}
	right := st(where, whereC, false, false, false) + st("  "+strings.SplitN(core.Long(m.today, m.today), " · ", 2)[0]+" ", hintC, false, false, false)
	mid := ""
	if m.msg != "" {
		mid = st(m.msg, yellowC, false, false, false)
	} else {
		hints := [][2]string{{"a", "añadir"}, {"x", "completar"}, {"e", "editar"}, {"s", "fecha"}, {"i", "descripción"},
			{"n", "nota"}, {"d", "borrar"}, {"/", "buscar"}, {"?", "ayuda"}}
		// Solo los atajos que caben enteros (y dejando sitio a la derecha si se puede).
		limit := m.w - width(left) - width(right) - 2
		if limit < 30 {
			limit = m.w - width(left)
		}
		for i, h := range hints {
			item := st(h[0], accent, true, false, false) + st(" "+h[1], hintC, false, false, false)
			if i > 0 {
				item = st(" · ", borderC, false, false, false) + item
			}
			if width(mid)+width(item) > limit {
				break
			}
			mid += item
		}
	}
	room := m.w - width(left)
	if room-width(mid) < width(right)+2 {
		return left + fit(mid, max(room, 0))
	}
	return left + fit(mid, room-width(right)) + right
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
	lines := []string{" " + st("❯ ", accent, true, false, false) + m.inputLine(inner-4), "", "", ""}
	dim := func(s string) string { return st(s, dimC, false, false, false) }
	var preview, hint string
	switch m.inKind {
	case inAdd, inEdit:
		preview = m.preview(core.ParseQuick(m.input.text(), m.today), m.inKind == inAdd)
		hint = dim(` #etiqueta  +Proyecto  p1-p4  mañana 18:00  limit 20/11  cada lunes  \literal`)
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
			desc = append(desc, "⚑ límite "+core.Long(*q.Deadline, m.today))
		}
		switch {
		case len(m.input.buf) == 0:
			preview = dim(" → sin fecha")
		case len(desc) > 0:
			preview = st(" → "+strings.Join(desc, "   "), greenC, false, false, false)
		default:
			preview = st(" → no entiendo esa fecha", redC, false, false, false)
		}
		hint = dim(" Enter guardar · Esc cancelar")
	case inNote:
		hint = dim(" Enter guardar · Esc cancelar")
	case inSearch:
		hint = dim(" Enter fijar · Esc limpiar")
	}
	lines[2], lines[3] = preview, hint
	if len(opts) > 0 {
		sel := min(m.sugg, len(opts)-1)
		lines = append(lines, dim(" Tab completar · ↑↓ elegir"))
		for i, o := range opts {
			mark := " "
			if i == sel {
				mark = "›"
				lines = append(lines, st(" "+mark, accent, true, false, true)+st(" "+o+" ", "", true, false, true))
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
	out := st(" → ", dimC, false, false, false)
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
		push("⚑ límite "+core.Long(*q.Deadline, m.today), redC)
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
		if k[0] == "" {
			lines = append(lines, "")
		} else if k[1] == "" {
			lines = append(lines, st(" "+k[0], accent, true, false, false)+" "+rule(68-runewidth.StringWidth(k[0])))
		} else {
			lines = append(lines, st(fmt.Sprintf("   %-14s", k[0]), yellowC, true, false, false)+st(k[1], grayC, false, false, false))
		}
	}
	w, h := min(74, m.w), min(len(lines)+2, m.h)
	return overlay(frame, box(" Atajos · cualquier tecla para cerrar ", w, h, lines, true), (m.w-w)/2, (m.h-h)/2)
}

var _ store.Store
