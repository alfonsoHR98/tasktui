package store

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alfonsoHR98/tasktui/internal/core"
)

// FileStore es el almacén local: una carpeta de archivos Markdown.
//
//   - Cada archivo .md es un proyecto («Trabajo/Cliente.md» → proyecto «Trabajo/Cliente»).
//   - Los encabezados «##» dentro del archivo son secciones.
//   - Las líneas que no son tareas se conservan intactas, igual que las tareas que
//     no se modifican (no se reformatea lo que escribiste en Obsidian).
//   - Las líneas con más sangría justo debajo de una tarea son su bloque: el texto
//     sin viñeta es la descripción y las viñetas («- 2026-11-06: texto») son notas.
//     El bloque se mueve y se borra junto con la tarea.
type FileStore struct {
	dir  string
	docs []*doc
}

type line struct {
	task  *core.Task // nil si no es tarea
	raw   *string    // texto original; nil si la tarea se modificó y hay que regenerarla
	other string
}

func (l line) isTask() bool { return l.task != nil }

type doc struct {
	path    string
	project string
	lines   []line
}

func (d *doc) render() string {
	parts := make([]string, len(d.lines))
	for i, l := range d.lines {
		switch {
		case l.isTask() && l.raw != nil:
			parts[i] = *l.raw
		case l.isTask():
			parts[i] = core.RenderLine(l.task)
		default:
			parts[i] = l.other
		}
	}
	return strings.Join(parts, "\n") + "\n"
}

func (d *doc) hasTasks() bool {
	for _, l := range d.lines {
		if l.isTask() {
			return true
		}
	}
	return false
}

// fileSnapshot es el contenido de los archivos, para deshacer.
type fileSnapshot []struct{ path, content string }

func writeAtomic(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp")
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func walk(dir string, out *[]string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("No pude leer %s: %w", dir, err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue // .obsidian, .trash, temporales
		}
		p := filepath.Join(dir, e.Name())
		if e.IsDir() {
			if err := walk(p, out); err != nil {
				return err
			}
		} else if filepath.Ext(p) == ".md" {
			*out = append(*out, p)
		}
	}
	return nil
}

// indentWidth es el ancho de la sangría (un tabulador cuenta como 4).
func indentWidth(s string) int {
	n := 0
	for _, r := range s {
		switch {
		case r == '\t':
			n += 4
		case r == ' ' || r == ' ':
			n++
		default:
			return n
		}
	}
	return n
}

// isNoteOf: ¿la línea es nota/descripción de una tarea con esa sangría?
func isNoteOf(s, taskIndent string) bool {
	_, _, isHeading := heading(s)
	return strings.TrimSpace(s) != "" && !isHeading && indentWidth(s) > indentWidth(taskIndent)
}

// NoteLine es la línea de nota tal como se escribe en disco, bajo una tarea con indent.
func NoteLine(indent string, n core.Note) string {
	if n.Date != nil {
		return fmt.Sprintf("%s    - %s: %s", indent, n.Date, strings.TrimSpace(n.Text))
	}
	return fmt.Sprintf("%s    - %s", indent, strings.TrimSpace(n.Text))
}

// BlockLines es el bloque completo bajo la tarea: descripción y luego notas.
func BlockLines(t *core.Task) []string {
	var out []string
	for _, l := range strings.Split(t.Description, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, t.Indent+"    "+l)
		}
	}
	for _, n := range t.Notes {
		out = append(out, NoteLine(t.Indent, n))
	}
	return out
}

func heading(s string) (int, string, bool) {
	n := 0
	for n < len(s) && s[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || !strings.HasPrefix(s[n:], " ") {
		return 0, "", false
	}
	return n, strings.TrimSpace(s[n:]), true
}

// GenID crea un id corto de 5 caracteres que no esté en existing.
func GenID(existing map[string]bool) string {
	const alpha = "0123456789abcdefghijklmnopqrstuvwxyz"
	for {
		b := make([]byte, 5)
		_, _ = rand.Read(b)
		for i := range b {
			b[i] = alpha[int(b[i])%36]
		}
		if id := string(b); !existing[id] {
			return id
		}
	}
}

// OpenFiles abre (o crea) la carpeta de tareas.
func OpenFiles(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("No pude crear %s: %w", dir, err)
	}
	inbox := filepath.Join(dir, core.Inbox+".md")
	if _, err := os.Stat(inbox); errors.Is(err, fs.ErrNotExist) {
		if err := writeAtomic(inbox, "# "+core.Inbox+"\n\n"); err != nil {
			return nil, err
		}
	}
	s := &FileStore{dir: dir}
	return s, s.Reload()
}

func (s *FileStore) Notice() string { return "" }
func (s *FileStore) Dir() string    { return s.dir }

// Reload vuelve a leer todo desde disco (por si cambió desde Obsidian o el celular).
func (s *FileStore) Reload() error {
	var files []string
	if err := walk(s.dir, &files); err != nil {
		return err
	}
	sort.Strings(files)
	docs := make([]*doc, 0, len(files))
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("No pude leer %s: %w", p, err)
		}
		docs = append(docs, s.parseDoc(p, string(b)))
	}
	s.docs = docs
	return s.normalizeIDs()
}

func splitLines(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimSuffix(content, "\n")
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}

func (s *FileStore) parseDoc(path, content string) *doc {
	rel, err := filepath.Rel(s.dir, path)
	if err != nil {
		rel = path
	}
	project := filepath.ToSlash(strings.TrimSuffix(rel, filepath.Ext(rel)))
	var section *string
	var lines []line
	owner := -1 // última tarea a la que todavía se le pueden pegar notas
	for _, raw := range splitLines(content) {
		raw := raw
		if task, ok := core.ParseLine(raw); ok {
			task.Project = project
			task.Section = section
			owner = len(lines)
			lines = append(lines, line{task: task, raw: &raw})
			continue
		}
		if owner >= 0 {
			if t := lines[owner].task; t != nil && isNoteOf(raw, t.Indent) {
				trimmed := strings.TrimSpace(raw)
				if rest, ok := strings.CutPrefix(trimmed, "- "); ok {
					t.Notes = append(t.Notes, parseNote(rest))
				} else if rest, ok := strings.CutPrefix(trimmed, "* "); ok {
					t.Notes = append(t.Notes, parseNote(rest))
				} else {
					if t.Description != "" {
						t.Description += "\n"
					}
					t.Description += trimmed
				}
				lines = append(lines, line{other: raw})
				continue
			}
			owner = -1
		}
		if level, title, ok := heading(raw); ok {
			if level >= 2 {
				s := title
				section = &s
			} else {
				section = nil
			}
		}
		lines = append(lines, line{other: raw})
	}
	return &doc{path: path, project: project, lines: lines}
}

func parseNote(rest string) core.Note {
	if d, txt, ok := strings.Cut(rest, ": "); ok {
		if date, ok := core.ParseDate(d); ok {
			return core.NewNote(txt, date.Ptr())
		}
	}
	return core.NewNote(rest, nil)
}

// normalizeIDs: toda tarea pendiente necesita un 🆔 para poder referirse a ella.
func (s *FileStore) normalizeIDs() error {
	ids := s.ids()
	for di, d := range s.docs {
		dirty := false
		for i := range d.lines {
			l := &d.lines[i]
			if l.isTask() && !l.task.Done && l.task.ID == nil {
				id := GenID(ids)
				ids[id] = true
				l.task.ID = &id
				l.raw = nil
				dirty = true
			}
		}
		if dirty {
			if err := s.write(di); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *FileStore) write(di int) error {
	d := s.docs[di]
	return writeAtomic(d.path, d.render())
}

func (s *FileStore) Tasks() []Entry {
	var out []Entry
	for di, d := range s.docs {
		for li, l := range d.lines {
			if l.isTask() {
				out = append(out, Entry{Pos{di, li}, l.task})
			}
		}
	}
	return out
}

func (s *FileStore) TaskAt(p Pos) *core.Task {
	if p.A < 0 || p.A >= len(s.docs) || p.B < 0 || p.B >= len(s.docs[p.A].lines) {
		return nil
	}
	return s.docs[p.A].lines[p.B].task
}

func (s *FileStore) Find(id string) (Pos, bool) {
	for _, e := range s.Tasks() {
		if e.Task.ID != nil && *e.Task.ID == id {
			return e.Pos, true
		}
	}
	return Pos{}, false
}

func (s *FileStore) findOrErr(id string) (Pos, error) {
	p, ok := s.Find(id)
	if !ok {
		return Pos{}, fmt.Errorf("No encontré la tarea «%s»", id)
	}
	return p, nil
}

func (s *FileStore) ids() map[string]bool {
	m := map[string]bool{}
	for _, e := range s.Tasks() {
		if e.Task.ID != nil {
			m[*e.Task.ID] = true
		}
	}
	return m
}

// Projects devuelve los proyectos: Inbox primero y el resto en orden alfabético.
func (s *FileStore) Projects() []string {
	var ps []string
	for _, d := range s.docs {
		if d.project != core.Inbox {
			ps = append(ps, d.project)
		}
	}
	sort.SliceStable(ps, func(i, j int) bool { return strings.ToLower(ps[i]) < strings.ToLower(ps[j]) })
	return append([]string{core.Inbox}, ps...)
}

func (s *FileStore) docIndex(project string) int {
	p := strings.ToLower(project)
	for i, d := range s.docs {
		if strings.ToLower(d.project) == p {
			return i
		}
	}
	return -1
}

func (s *FileStore) createDoc(project string) (int, error) {
	name := strings.Trim(strings.TrimSpace(project), "/")
	invalid := name == ""
	for _, c := range strings.Split(name, "/") {
		if strings.TrimSpace(c) == "" || c == ".." || strings.HasPrefix(c, ".") {
			invalid = true
		}
	}
	if invalid {
		return 0, fmt.Errorf("Nombre de proyecto inválido: «%s»", project)
	}
	title := name[strings.LastIndex(name, "/")+1:]
	s.docs = append(s.docs, &doc{
		path:    filepath.Join(s.dir, filepath.FromSlash(name)+".md"),
		project: name,
		lines:   []line{{other: "# " + title}, {other: ""}},
	})
	di := len(s.docs) - 1
	return di, s.write(di)
}

func (s *FileStore) docOrCreate(project string) (int, error) {
	if i := s.docIndex(project); i >= 0 {
		return i, nil
	}
	return s.createDoc(project)
}

// blockEnd es el fin (exclusivo) del bloque de la tarea: la tarea y sus notas.
func (s *FileStore) blockEnd(p Pos) int {
	lines := s.docs[p.A].lines
	if p.B >= len(lines) || !lines[p.B].isTask() {
		return p.B + 1
	}
	indent := lines[p.B].task.Indent
	j := p.B + 1
	for j < len(lines) && !lines[j].isTask() && isNoteOf(lines[j].other, indent) {
		j++
	}
	return j
}

func insertLines(lines []line, at int, ins ...line) []line {
	out := make([]line, 0, len(lines)+len(ins))
	out = append(out, lines[:at]...)
	out = append(out, ins...)
	return append(out, lines[at:]...)
}

// insertTask inserta tras la última tarea del archivo (y sus notas), o al final.
// Las notas de la tarea se escriben debajo.
func (s *FileStore) insertTask(di int, t *core.Task) {
	d := s.docs[di]
	t.Project = d.project
	at := len(d.lines)
	for i := len(d.lines) - 1; i >= 0; i-- {
		if d.lines[i].isTask() {
			at = s.blockEnd(Pos{di, i})
			break
		}
	}
	ins := []line{{task: t}}
	for _, b := range BlockLines(t) {
		ins = append(ins, line{other: b})
	}
	d.lines = insertLines(d.lines, at, ins...)
}

// removeBlock quita la tarea en p junto con sus notas.
func (s *FileStore) removeBlock(p Pos) {
	end := s.blockEnd(p)
	d := s.docs[p.A]
	d.lines = append(d.lines[:p.B:p.B], d.lines[end:]...)
}

func (s *FileStore) AddNote(id, text string) error {
	if strings.TrimSpace(text) == "" {
		return errors.New("La nota está vacía")
	}
	if err := s.Reload(); err != nil {
		return err
	}
	p, err := s.findOrErr(id)
	if err != nil {
		return err
	}
	end := s.blockEnd(p)
	indent := s.TaskAt(p).Indent
	note := core.NewNote(text, core.Today().Ptr())
	d := s.docs[p.A]
	d.lines = insertLines(d.lines, end, line{other: NoteLine(indent, note)})
	return s.write(p.A)
}

func (s *FileStore) setTask(p Pos, t *core.Task) {
	s.docs[p.A].lines[p.B] = line{task: t}
}

func (s *FileStore) Add(t *core.Task, project string) (string, error) {
	if err := s.Reload(); err != nil {
		return "", err
	}
	id := GenID(s.ids())
	t.ID = &id
	di, err := s.docOrCreate(project)
	if err != nil {
		return "", err
	}
	s.insertTask(di, t)
	return id, s.write(di)
}

func notesEqual(a, b []core.Note) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Text != b[i].Text || (a[i].Date == nil) != (b[i].Date == nil) || (a[i].Date != nil && !a[i].Date.Equal(*b[i].Date)) {
			return false
		}
	}
	return true
}

// Update modifica una tarea. Si cambia Project, se mueve a otro archivo.
func (s *FileStore) Update(id string, f func(*core.Task)) error {
	if err := s.Reload(); err != nil {
		return err
	}
	p, err := s.findOrErr(id)
	if err != nil {
		return err
	}
	before := s.TaskAt(p)
	t := before.Clone()
	f(t)
	if strings.EqualFold(t.Project, s.docs[p.A].project) {
		t.Project = s.docs[p.A].project
		// Si cambió la descripción o las notas, se reescribe el bloque.
		if t.Description != before.Description || !notesEqual(t.Notes, before.Notes) {
			end := s.blockEnd(p)
			d := s.docs[p.A]
			var nb []line
			for _, b := range BlockLines(t) {
				nb = append(nb, line{other: b})
			}
			rest := append([]line(nil), d.lines[end:]...)
			d.lines = append(append(d.lines[:p.B+1:p.B+1], nb...), rest...)
		}
		s.setTask(p, t)
		return s.write(p.A)
	}
	target, err := s.docOrCreate(t.Project)
	if err != nil {
		return err
	}
	// docOrCreate puede añadir un documento al final: p sigue valiendo.
	s.removeBlock(p)
	if err := s.write(p.A); err != nil {
		return err
	}
	t.Section = nil
	t.Indent = ""
	s.insertTask(target, t)
	return s.write(target)
}

// Toggle alterna completada/pendiente; devuelve true si quedó completada. Si se
// repite, la tarea pendiente conserva el id y avanza de fecha (como Obsidian Tasks,
// la ocurrencia hecha queda debajo).
func (s *FileStore) Toggle(id string) (bool, error) {
	if err := s.Reload(); err != nil {
		return false, err
	}
	p, err := s.findOrErr(id)
	if err != nil {
		return false, err
	}
	t := s.TaskAt(p).Clone()
	today := core.Today()
	if t.Done {
		t.SetDone(false, today)
		s.setTask(p, t)
		return false, s.write(p.A)
	}
	c := t.Complete(today, GenID(s.ids()))
	if !c.Recurring() {
		s.setTask(p, c.Simple)
	} else {
		end := s.blockEnd(p)
		s.setTask(p, c.Next)
		d := s.docs[p.A]
		d.lines = insertLines(d.lines, end, line{task: c.Done})
	}
	return true, s.write(p.A)
}

func (s *FileStore) Delete(id string) error {
	if err := s.Reload(); err != nil {
		return err
	}
	p, err := s.findOrErr(id)
	if err != nil {
		return err
	}
	s.removeBlock(p)
	return s.write(p.A)
}

func (s *FileStore) Snapshot() (Snapshot, error) {
	var snap fileSnapshot
	for _, d := range s.docs {
		b, err := os.ReadFile(d.path)
		if err != nil {
			return nil, err
		}
		snap = append(snap, struct{ path, content string }{d.path, string(b)})
	}
	return snap, nil
}

// Restore restaura una copia. Los archivos creados después solo se borran si no
// tienen tareas (para no perder nada escrito desde otro dispositivo).
func (s *FileStore) Restore(sn Snapshot) error {
	snap, ok := sn.(fileSnapshot)
	if !ok {
		return errors.New("La copia para deshacer es de otro almacén")
	}
	if err := s.Reload(); err != nil {
		return err
	}
	for _, d := range s.docs {
		found := false
		for _, f := range snap {
			found = found || f.path == d.path
		}
		if !found && !d.hasTasks() {
			if err := os.Remove(d.path); err != nil {
				return err
			}
		}
	}
	for _, f := range snap {
		if cur, err := os.ReadFile(f.path); err != nil || string(cur) != f.content {
			if err := writeAtomic(f.path, f.content); err != nil {
				return err
			}
		}
	}
	return s.Reload()
}

// ExportMarkdown escribe las tareas de cualquier almacén como .md (un archivo por
// proyecto, «##» por sección, notas con sangría). Devuelve cuántos proyectos escribió.
func ExportMarkdown(s Store, dir string) (int, error) {
	projects := s.Projects()
	for _, p := range projects {
		title := p[strings.LastIndex(p, "/")+1:]
		var sb strings.Builder
		sb.WriteString("# " + title + "\n")
		var tasks []*core.Task
		for _, e := range s.Tasks() {
			if e.Task.Project == p {
				tasks = append(tasks, e.Task)
			}
		}
		// Secciones en orden de aparición, primero las tareas sin sección.
		sections := []*string{nil}
		same := func(a, b *string) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
		for _, t := range tasks {
			found := false
			for _, sec := range sections {
				found = found || same(sec, t.Section)
			}
			if !found {
				sections = append(sections, t.Section)
			}
		}
		for _, sec := range sections {
			var group []*core.Task
			for _, t := range tasks {
				if same(t.Section, sec) {
					group = append(group, t)
				}
			}
			if len(group) == 0 && sec != nil {
				continue
			}
			sb.WriteString("\n")
			if sec != nil {
				sb.WriteString("## " + *sec + "\n\n")
			}
			for _, t := range group {
				sb.WriteString(core.RenderLine(t) + "\n")
				for _, l := range BlockLines(t) {
					sb.WriteString(l + "\n")
				}
			}
		}
		if err := writeAtomic(filepath.Join(dir, filepath.FromSlash(p)+".md"), sb.String()); err != nil {
			return 0, err
		}
	}
	return len(projects), nil
}
