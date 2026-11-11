// Package tui es la interfaz de terminal de tasktui (Bubble Tea).
package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alfonsoHR98/tasktui/internal/core"
	"github.com/alfonsoHR98/tasktui/internal/store"
)

type sideKind int

const (
	sideHeader sideKind = iota
	sideView
)

type sideItem struct {
	label string
	kind  sideKind
	view  core.View
	count int
}

type row struct {
	header string // si no está vacío, es un encabezado de grupo
	pos    store.Pos
}

type focus int

const (
	focusSide focus = iota
	focusList
)

type inputKind int

const (
	inAdd inputKind = iota
	inEdit
	inSchedule
	inNote
	inSearch
)

type mode int

const (
	modeNormal mode = iota
	modeInput
	modeConfirm
	modeHelp
)

type model struct {
	store store.Store
	today core.Date
	w, h  int

	side    []sideItem
	sideIdx int
	view    core.View

	rows     []row
	taskRows []int // índices de rows que son tareas
	sel      int
	selID    string
	scroll   int

	focus    focus
	mode     mode
	inKind   inputKind
	inID     string // id de la tarea de Edit/Schedule/Note/Confirm
	inProj   string // proyecto a eliminar en Confirm (vacío si es una tarea)
	input    input
	query    string
	showDone bool
	msg      string
	pendingG bool
	sugg     int
	undo     []store.Snapshot
	lastKey  time.Time
	quitting bool
}

type tickMsg time.Time

// descMsg llega al terminar de editar la descripción en $EDITOR.
type descMsg struct {
	id, path, before string
	err              error
}

// Run abre la TUI.
func Run(s store.Store) error {
	m := newModel(s)
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func newModel(s store.Store) *model {
	m := &model{store: s, today: core.Today(), view: core.View{Kind: core.ViewToday}, focus: focusList, lastKey: time.Now()}
	m.refresh()
	m.msg = s.Notice()
	return m
}

func tick() tea.Cmd { return tea.Tick(30*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }) }

func (m *model) Init() tea.Cmd { return tick() }

// ---------- estado ----------

func (m *model) selectedPos() (store.Pos, bool) {
	if m.sel < 0 || m.sel >= len(m.taskRows) {
		return store.Pos{}, false
	}
	r := m.rows[m.taskRows[m.sel]]
	if r.header != "" {
		return store.Pos{}, false
	}
	return r.pos, true
}

func (m *model) selected() *core.Task {
	p, ok := m.selectedPos()
	if !ok {
		return nil
	}
	return m.store.TaskAt(p)
}

func (m *model) syncSel() {
	m.selID = ""
	if t := m.selected(); t != nil && t.ID != nil {
		m.selID = *t.ID
	}
}

func (m *model) countOpen(f func(*core.Task) bool) int {
	n := 0
	for _, e := range m.store.Tasks() {
		if !e.Task.Done && f(e.Task) {
			n++
		}
	}
	return n
}

func (m *model) refresh() {
	m.today = core.Today()
	today := m.today
	side := []sideItem{
		{"▣ Inbox", sideView, core.View{Kind: core.ViewInbox}, m.countOpen(func(t *core.Task) bool { return t.Project == core.Inbox })},
		{"◉ Hoy", sideView, core.View{Kind: core.ViewToday}, m.countOpen(func(t *core.Task) bool { return t.Due != nil && !t.Due.After(today) })},
		{"◷ Próximos", sideView, core.View{Kind: core.ViewUpcoming}, m.countOpen(func(t *core.Task) bool { return t.Due != nil && t.Due.After(today) })},
		{"≡ Todas", sideView, core.View{Kind: core.ViewAll}, m.countOpen(func(*core.Task) bool { return true })},
		{"PROYECTOS", sideHeader, core.View{}, 0},
	}
	for _, p := range m.store.Projects() {
		if p == core.Inbox {
			continue
		}
		p := p
		side = append(side, sideItem{"▸ " + p, sideView, core.View{Kind: core.ViewProject, Name: p}, m.countOpen(func(t *core.Task) bool { return t.Project == p })})
	}
	if labels := store.Labels(m.store); len(labels) > 0 {
		side = append(side, sideItem{"ETIQUETAS", sideHeader, core.View{}, 0})
		for _, l := range labels {
			side = append(side, sideItem{"# " + l.Name, sideView, core.View{Kind: core.ViewLabel, Name: l.Name}, l.Open})
		}
	}
	m.side = side
	for i, it := range m.side {
		if it.kind == sideView && it.view == m.view {
			m.sideIdx = i
		}
	}

	m.rows, m.taskRows = nil, nil
	for _, g := range store.View(m.store, m.view, today, m.showDone, m.query) {
		if g.Title != nil {
			m.rows = append(m.rows, row{header: *g.Title})
		}
		for _, p := range g.Items {
			m.taskRows = append(m.taskRows, len(m.rows))
			m.rows = append(m.rows, row{pos: p})
		}
	}
	// Mantener la selección en la misma tarea si sigue visible.
	if m.selID != "" {
		found := false
		for i := range m.taskRows {
			m.sel = i
			if t := m.selected(); t != nil && t.ID != nil && *t.ID == m.selID {
				found = true
				break
			}
		}
		if !found {
			m.sel = min(m.sel, max(len(m.taskRows)-1, 0))
		}
	}
	m.sel = min(m.sel, max(len(m.taskRows)-1, 0))
	m.syncSel()
}

func (m *model) setView(v core.View) {
	m.view = v
	m.sel, m.selID, m.scroll = 0, "", 0
	m.refresh()
}

// countOpenAll cuenta todas las tareas del proyecto, pendientes o no.
func (m *model) countOpenAll(project string) int {
	n := 0
	for _, e := range m.store.Tasks() {
		if strings.EqualFold(e.Task.Project, project) {
			n++
		}
	}
	return n
}

func (m *model) moveSide(delta int) {
	for i := m.sideIdx + delta; i >= 0 && i < len(m.side); i += delta {
		if m.side[i].kind == sideView {
			m.sideIdx = i
			m.setView(m.side[i].view)
			return
		}
	}
}

func (m *model) moveSel(delta int) {
	if len(m.taskRows) == 0 {
		return
	}
	m.sel = min(max(m.sel+delta, 0), len(m.taskRows)-1)
	m.syncSel()
}

// mutate ejecuta un cambio guardando una copia para poder deshacer.
// f devuelve el mensaje y, opcionalmente, el id a seleccionar.
func (m *model) mutate(f func(s store.Store) (string, string, error)) {
	if err := m.store.Reload(); err != nil {
		m.msg = "Error: " + err.Error()
		return
	}
	snap, snapErr := m.store.Snapshot()
	msg, sel, err := f(m.store)
	if err != nil {
		m.msg = "Error: " + err.Error()
	} else {
		if snapErr == nil {
			m.undo = append(m.undo, snap)
			if len(m.undo) > 50 {
				m.undo = m.undo[1:]
			}
		}
		m.msg = msg
		if sel != "" {
			m.selID = sel
		}
	}
	m.refresh()
}

func (m *model) openInput(kind inputKind, id, prefill string) {
	m.input.set(prefill)
	m.inKind, m.inID, m.mode = kind, id, modeInput
	m.sugg = 0
}

func (m *model) defaultProject() string {
	if m.view.Kind == core.ViewProject {
		return m.view.Name
	}
	return core.Inbox
}

// ---------- autocompletado de +Proyecto y #etiqueta ----------

// completions devuelve dónde empieza la palabra en el cursor y las opciones.
func (m *model) completions() (int, []string) {
	if m.mode != modeInput || (m.inKind != inAdd && m.inKind != inEdit) {
		return 0, nil
	}
	buf := m.input.buf[:m.input.cur]
	start := 0
	for i := len(buf) - 1; i >= 0; i-- {
		if buf[i] == ' ' {
			start = i + 1
			break
		}
	}
	word := []rune(string(buf[start:]))
	if len(word) == 0 {
		return 0, nil
	}
	prefix := word[0]
	partial := strings.ToLower(strings.ReplaceAll(string(word[1:]), "_", " "))
	var names []string
	switch prefix {
	case '+':
		names = m.store.Projects()
	case '#', '@':
		for _, l := range store.Labels(m.store) {
			names = append(names, l.Name)
		}
	default:
		return 0, nil
	}
	var found []string
	for _, n := range names {
		if low := strings.ToLower(n); strings.Contains(low, partial) && low != partial {
			found = append(found, n)
		}
	}
	sortStable(found, func(a, b string) bool {
		la, lb := strings.ToLower(a), strings.ToLower(b)
		pa, pb := strings.HasPrefix(la, partial), strings.HasPrefix(lb, partial)
		if pa != pb {
			return pa
		}
		return la < lb
	})
	if len(found) > 6 {
		found = found[:6]
	}
	if len(found) == 0 {
		return 0, nil
	}
	out := make([]string, len(found))
	for i, n := range found {
		out[i] = string(prefix) + strings.ReplaceAll(n, " ", "_")
	}
	return start, out
}

func sortStable(s []string, less func(a, b string) bool) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && less(s[j], s[j-1]); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func (m *model) acceptCompletion() {
	start, opts := m.completions()
	if len(opts) == 0 {
		return
	}
	chosen := []rune(opts[min(m.sugg, len(opts)-1)] + " ")
	rest := append([]rune(nil), m.input.buf[m.input.cur:]...)
	m.input.buf = append(append(append([]rune(nil), m.input.buf[:start]...), chosen...), rest...)
	m.input.cur = start + len(chosen)
	m.sugg = 0
}

// ---------- teclado ----------

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tickMsg:
		// Sin actividad: recarga por si hubo cambios desde el celular.
		if time.Since(m.lastKey) >= 30*time.Second {
			_ = m.store.Reload()
			m.refresh()
		}
		return m, tick()
	case descMsg:
		return m, m.finishDescription(msg)
	case tea.KeyMsg:
		m.lastKey = time.Now()
		cmd := m.onKey(msg)
		if m.quitting {
			return m, tea.Quit
		}
		return m, cmd
	}
	return m, nil
}

func (m *model) onKey(k tea.KeyMsg) tea.Cmd {
	if k.Type == tea.KeyCtrlC {
		m.quitting = true
		return nil
	}
	switch m.mode {
	case modeHelp:
		m.mode = modeNormal
	case modeConfirm:
		id, proj := m.inID, m.inProj
		m.mode, m.inProj = modeNormal, ""
		if k.String() == "y" || k.String() == "Y" {
			if proj != "" {
				m.mutate(func(s store.Store) (string, string, error) {
					n, err := s.DeleteProject(proj)
					return fmt.Sprintf("Proyecto «%s» eliminado (%d tareas) · u deshace", proj, n), "", err
				})
				if !strings.HasPrefix(m.msg, "Error") {
					msg := m.msg
					m.setView(core.View{Kind: core.ViewToday})
					m.msg = msg
				}
				return nil
			}
			m.mutate(func(s store.Store) (string, string, error) {
				return "Eliminada", "", s.Delete(id)
			})
		} else {
			m.msg = "Cancelado"
		}
	case modeInput:
		return m.onInputKey(k)
	default:
		return m.onNormalKey(k)
	}
	return nil
}

func (m *model) onInputKey(k tea.KeyMsg) tea.Cmd {
	switch k.Type {
	case tea.KeyEsc:
		if m.inKind == inSearch {
			m.query = ""
			m.refresh()
		}
		m.mode = modeNormal
	case tea.KeyEnter:
		m.mode = modeNormal
		m.submit()
	case tea.KeyTab:
		m.acceptCompletion()
	case tea.KeyDown, tea.KeyUp:
		if _, opts := m.completions(); len(opts) > 0 {
			n := len(opts)
			if k.Type == tea.KeyDown {
				m.sugg = (m.sugg + 1) % n
			} else {
				m.sugg = (m.sugg + n - 1) % n
			}
		}
	default:
		m.sugg = 0
		m.input.handle(k)
		if m.inKind == inSearch {
			m.query = m.input.text()
			m.sel = 0
			m.refresh()
		}
	}
	return nil
}

func (m *model) onNormalKey(k tea.KeyMsg) tea.Cmd {
	m.msg = ""
	key := k.String()
	if key == " " {
		key = "space"
	}
	if m.pendingG {
		m.pendingG = false
		switch key {
		case "g":
			m.sel = 0
			m.syncSel()
		case "i":
			m.setView(core.View{Kind: core.ViewInbox})
		case "t":
			m.setView(core.View{Kind: core.ViewToday})
		case "u", "p":
			m.setView(core.View{Kind: core.ViewUpcoming})
		case "a":
			m.setView(core.View{Kind: core.ViewAll})
		}
		return nil
	}
	side := m.focus == focusSide
	switch key {
	case "q":
		m.quitting = true
	case "?":
		m.mode = modeHelp
	case "g":
		m.pendingG = true
	case "G":
		m.sel = max(len(m.taskRows)-1, 0)
		m.syncSel()
	case "tab", "shift+tab":
		if side {
			m.focus = focusList
		} else {
			m.focus = focusSide
		}
	case "h", "left":
		m.focus = focusSide
	case "l", "right", "enter":
		m.focus = focusList
	case "j", "down":
		if side {
			m.moveSide(1)
		} else {
			m.moveSel(1)
		}
	case "k", "up":
		if side {
			m.moveSide(-1)
		} else {
			m.moveSel(-1)
		}
	case "a":
		m.openInput(inAdd, "", "")
	case "/":
		m.openInput(inSearch, "", m.query)
	case "esc":
		m.query = ""
		m.refresh()
	case "H":
		m.showDone = !m.showDone
		if m.showDone {
			m.msg = "Mostrando completadas"
		} else {
			m.msg = "Ocultando completadas"
		}
		m.refresh()
	case "u":
		if n := len(m.undo); n > 0 {
			snap := m.undo[n-1]
			m.undo = m.undo[:n-1]
			if err := m.store.Restore(snap); err != nil {
				m.msg = "Error: " + err.Error()
			} else {
				m.msg = "Deshecho"
			}
			m.refresh()
		} else {
			m.msg = "Nada que deshacer"
		}
	case "d":
		if side {
			if m.view.Kind != core.ViewProject {
				m.msg = "Sitúate en un proyecto para eliminarlo"
				return nil
			}
			m.inProj, m.mode = m.view.Name, modeConfirm
			return nil
		}
		return m.onTaskKey(key)
	case "r":
		if err := m.store.Reload(); err != nil {
			m.msg = "Error: " + err.Error()
		} else {
			m.msg = "Recargado"
		}
		m.refresh()
	default:
		return m.onTaskKey(key)
	}
	return nil
}

// onTaskKey: teclas que actúan sobre la tarea seleccionada.
func (m *model) onTaskKey(key string) tea.Cmd {
	t := m.selected()
	if t == nil {
		return nil
	}
	if t.ID == nil {
		m.msg = "Tarea completada sin id: edítala desde Obsidian"
		return nil
	}
	id, today := *t.ID, m.today
	switch key {
	case "x", "space":
		task := t.Clone()
		m.mutate(func(s store.Store) (string, string, error) {
			done, err := s.Toggle(id)
			switch {
			case err != nil:
				return "", "", err
			case done && task.Recurrence != nil:
				return "✓ Hecha · se repite " + task.Recurrence.ToES(), "", nil
			case done:
				return "✓ Completada", "", nil
			}
			return "Reabierta", "", nil
		})
	case "e":
		m.openInput(inEdit, id, core.ToQuick(t, today))
	case "s":
		var parts []string
		if t.Due != nil {
			parts = append(parts, t.Due.String())
		}
		if t.Time != nil {
			parts = append(parts, t.Time.String())
		}
		if t.Deadline != nil {
			parts = append(parts, "limit "+t.Deadline.String())
		}
		m.openInput(inSchedule, id, strings.Join(parts, " "))
	case "t":
		m.mutate(func(s store.Store) (string, string, error) {
			return "Para hoy", "", s.Update(id, func(t *core.Task) { t.Due = today.Ptr() })
		})
	case "1", "2", "3", "4":
		p := int(key[0] - '0')
		m.mutate(func(s store.Store) (string, string, error) {
			return fmt.Sprintf("Prioridad p%d", p), "", s.Update(id, func(t *core.Task) { t.Priority = p })
		})
	case "d":
		m.inID, m.mode = id, modeConfirm
	case "i":
		return m.editDescription(id, t.Description)
	case "n":
		m.openInput(inNote, id, "")
	}
	return nil
}

func (m *model) submit() {
	text := m.input.text()
	today := m.today
	q := core.ParseQuick(text, today)
	id := m.inID
	switch m.inKind {
	case inAdd:
		if q.Text == "" {
			m.msg = "La tarea necesita texto"
			return
		}
		t := core.NewTask(q.Text)
		t.Labels = q.Labels
		t.Priority = 4
		if q.Priority != nil {
			t.Priority = *q.Priority
		}
		t.Recurrence, t.Time, t.Deadline = q.Recurrence, q.Time, q.Deadline
		t.Due = q.Due
		if t.Due == nil && m.view.Kind == core.ViewToday {
			t.Due = today.Ptr()
		}
		if m.view.Kind == core.ViewLabel {
			found := false
			for _, x := range t.Labels {
				found = found || strings.EqualFold(x, m.view.Name)
			}
			if !found {
				t.Labels = append(t.Labels, m.view.Name)
			}
		}
		project := m.defaultProject()
		if q.Project != nil {
			project = *q.Project
		}
		m.mutate(func(s store.Store) (string, string, error) {
			newID, err := s.Add(t, project)
			return "Añadida en " + project, newID, err
		})
	case inEdit:
		if q.Text == "" {
			m.msg = "La tarea necesita texto"
			return
		}
		m.mutate(func(s store.Store) (string, string, error) {
			return "Guardada", "", s.Update(id, func(t *core.Task) { q.ApplyFull(t) })
		})
	case inNote:
		if strings.TrimSpace(text) == "" {
			return
		}
		m.mutate(func(s store.Store) (string, string, error) {
			return "Nota añadida", "", s.AddNote(id, text)
		})
	case inSchedule:
		if strings.TrimSpace(text) != "" && q.Due == nil && q.Deadline == nil {
			m.msg = fmt.Sprintf("No entendí la fecha «%s»", text)
			return
		}
		m.mutate(func(s store.Store) (string, string, error) {
			err := s.Update(id, func(t *core.Task) {
				t.Due, t.Time, t.Deadline = q.Due, q.Time, q.Deadline
				if q.Recurrence != nil {
					t.Recurrence = q.Recurrence
				}
			})
			msg := "Sin fecha"
			if q.Due != nil {
				msg = "Fecha: " + core.Long(*q.Due, today)
			}
			return msg, "", err
		})
	}
}

// ---------- descripción en $EDITOR ----------

func editorCommand() (string, []string) {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "nano"
		for _, e := range []string{"nvim", "vim", "nano"} {
			if _, err := exec.LookPath(e); err == nil {
				editor = e
				break
			}
		}
	}
	parts := strings.Fields(editor)
	return parts[0], parts[1:]
}

func (m *model) editDescription(id, current string) tea.Cmd {
	path := fmt.Sprintf("%s/tasktui-%s.md", os.TempDir(), id)
	if err := os.WriteFile(path, []byte(current), 0o600); err != nil {
		m.msg = "Error: " + err.Error()
		return nil
	}
	name, args := editorCommand()
	c := exec.Command(name, append(args, path)...)
	return tea.ExecProcess(c, func(err error) tea.Msg { return descMsg{id: id, path: path, before: current, err: err} })
}

func (m *model) finishDescription(d descMsg) tea.Cmd {
	defer os.Remove(d.path)
	if d.err != nil {
		m.msg = "Error: El editor terminó con error"
		return nil
	}
	b, err := os.ReadFile(d.path)
	if err != nil {
		m.msg = "Error: " + err.Error()
		return nil
	}
	text := strings.TrimRight(string(b), " \t\r\n")
	if text == strings.TrimRight(d.before, " \t\r\n") {
		m.msg = "Descripción sin cambios"
		return nil
	}
	m.mutate(func(s store.Store) (string, string, error) {
		return "Descripción guardada", "", s.Update(d.id, func(t *core.Task) { t.Description = text })
	})
	return nil
}
