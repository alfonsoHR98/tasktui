// Package reminders es el espejo hacia la app Recordatorios de Apple (solo macOS).
//
// Los .md (o el servidor) son la fuente de verdad. «tasktui sync»:
//  1. Si completaste en Recordatorios un recordatorio de tasktui, completa la
//     tarea (si se repite, avanza a la siguiente fecha).
//  2. Deja en la lista «tasktui» un recordatorio por cada tarea pendiente con
//     fecha (título, fecha de todo el día, prioridad, proyecto y notas).
//  3. Lo que crees tú en la lista «tasktui» (a mano o con Siri) se importa como
//     tarea nueva. El título acepta la captura rápida («+Proyecto», «#tag», «p1»,
//     «mañana»…); la fecha y prioridad del recordatorio mandan.
//  4. Borra de esa lista los recordatorios de tasktui cuya tarea ya no está
//     pendiente o ya no tiene fecha.
//
// La relación tarea ↔ recordatorio se guarda en ~/.config/tasktui/recordatorios.json
// (los identificadores de EventKit son locales a cada Mac, por eso no van en el vault).
package reminders

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alfonsoHR98/tasktui/internal/core"
	"github.com/alfonsoHR98/tasktui/internal/store"
)

// List es el nombre de la lista de Recordatorios que usa tasktui.
const List = "tasktui"

// Report es lo que hizo una sincronización.
type Report struct {
	Completed []string `json:"completed"` // ids completados desde Recordatorios
	Imported  []string `json:"imported"`  // ids importados desde Recordatorios
	Created   int      `json:"created"`
	Updated   int      `json:"updated"`
	Removed   int      `json:"removed"`
}

// Wanted es cómo debería verse el recordatorio de una tarea.
type Wanted struct {
	TaskID string
	Title  string
	Due    core.Date
	// Time: con hora, el recordatorio avisa a esa hora exacta.
	Time *core.Clock
	// Priority de EventKit: 0 ninguna, 1 alta, 5 media, 9 baja.
	Priority int
	Notes    string
}

// Existing es un recordatorio que ya está en la lista.
type Existing struct {
	EKID      string
	Title     string
	Due       *core.Date
	Time      *core.Clock
	Priority  int
	Notes     string
	Completed bool
}

func sameDate(a *core.Date, b core.Date) bool { return a != nil && a.Equal(b) }

func sameClock(a, b *core.Clock) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func (e Existing) matches(w Wanted) bool {
	return !e.Completed && e.Title == w.Title && sameDate(e.Due, w.Due) && sameClock(e.Time, w.Time) &&
		e.Priority == w.Priority && e.Notes == w.Notes
}

// EKPriority convierte la prioridad de tasktui a la de EventKit.
func EKPriority(p int) int {
	switch p {
	case 1:
		return 1
	case 2:
		return 5
	case 3:
		return 9
	}
	return 0
}

// TaskPriority convierte la prioridad de EventKit a la de tasktui (RFC 5545: 1-4 alta, 5 media, 6-9 baja).
func TaskPriority(ek int) int {
	switch {
	case ek >= 1 && ek <= 4:
		return 1
	case ek == 5:
		return 2
	case ek >= 6 && ek <= 9:
		return 3
	}
	return 4
}

// ImportTask crea una tarea a partir de un recordatorio hecho fuera de tasktui.
// Devuelve la tarea y su proyecto.
func ImportTask(e Existing, today core.Date) (*core.Task, string) {
	q := core.ParseQuick(e.Title, today)
	text := q.Text
	if text == "" {
		text = strings.TrimSpace(e.Title)
	}
	t := core.NewTask(text)
	t.Labels = q.Labels
	t.Priority = 4
	if p := TaskPriority(e.Priority); p != 4 {
		t.Priority = p
	} else if q.Priority != nil {
		t.Priority = *q.Priority
	}
	t.Due = e.Due
	if t.Due == nil {
		t.Due = q.Due
	}
	t.Time = e.Time
	if t.Time == nil {
		t.Time = q.Time
	}
	t.Deadline, t.Recurrence = q.Deadline, q.Recurrence
	if t.Recurrence != nil && t.Due == nil {
		t.Due = t.Recurrence.First(today).Ptr()
	}
	// Las notas del recordatorio son la descripción de la tarea.
	var lines []string
	for _, l := range strings.Split(e.Notes, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	t.Description = strings.Join(lines, "\n")
	project := core.Inbox
	if q.Project != nil {
		project = *q.Project
	}
	return t, project
}

// WantedFor es el recordatorio que le corresponde a una tarea; nil si no debe tener.
func WantedFor(t *core.Task, today core.Date) *Wanted {
	if t.Done || t.ID == nil || t.Due == nil {
		return nil
	}
	// Notas del recordatorio: proyecto, límite, descripción y comentarios.
	notes := []string{"📁 " + t.Project}
	if t.Deadline != nil {
		notes = append(notes, "🎯 Límite: "+core.Long(*t.Deadline, today))
	}
	if strings.TrimSpace(t.Description) != "" {
		notes = append(notes, strings.TrimSpace(t.Description))
	}
	for _, n := range t.Notes {
		if n.Date != nil {
			notes = append(notes, fmt.Sprintf("• %s: %s", core.Rel(*n.Date, today), n.Text))
		} else {
			notes = append(notes, "• "+n.Text)
		}
	}
	return &Wanted{
		TaskID: *t.ID, Title: t.Text, Due: *t.Due, Time: t.Time,
		Priority: EKPriority(t.Priority), Notes: strings.Join(notes, "\n"),
	}
}

// Mapping relaciona el id de la tarea con el identificador del recordatorio.
type Mapping map[string]string

func (m Mapping) keys() []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// Update es un recordatorio por actualizar.
type Update struct {
	EKID string
	Want Wanted
}

// Plan es lo que hay que hacer para dejar la lista como las tareas.
type Plan struct {
	Create []Wanted   // recordatorio nuevo para estas tareas
	Update []Update   // cómo debe quedar cada uno
	Remove []string   // recordatorios de tasktui que sobran
	Import []Existing // creados fuera de tasktui: se vuelven tareas
}

// IsEmpty: no hay nada que hacer.
func (p Plan) IsEmpty() bool {
	return len(p.Create)+len(p.Update)+len(p.Remove)+len(p.Import) == 0
}

// CompletedInReminders devuelve las tareas pendientes cuyo recordatorio marcaste como completado.
func CompletedInReminders(m Mapping, existing []Existing, pending map[string]bool) []string {
	var out []string
	for _, tid := range m.keys() {
		if !pending[tid] {
			continue
		}
		for _, e := range existing {
			if e.EKID == m[tid] && e.Completed {
				out = append(out, tid)
				break
			}
		}
	}
	return out
}

// MakePlan decide qué crear, actualizar, borrar e importar. Actualiza m.
//
//   - Un recordatorio que estaba en m es de tasktui: si su tarea ya no se quiere, se borra.
//   - Uno pendiente que nunca estuvo en m lo creaste tú: se importa (o, si se perdió
//     la relación y coincide el título, se adopta para no duplicar).
//   - Uno completado que nunca estuvo en m se deja en paz.
func MakePlan(want []Wanted, existing []Existing, m Mapping) Plan {
	byID := func(id string) *Existing {
		for i := range existing {
			if existing[i].EKID == id {
				return &existing[i]
			}
		}
		return nil
	}
	ours := map[string]bool{}
	for _, ek := range m {
		ours[ek] = true
	}
	for tid, ek := range m {
		wanted := false
		for _, w := range want {
			wanted = wanted || w.TaskID == tid
		}
		if byID(ek) == nil || !wanted {
			delete(m, tid)
		}
	}
	used := map[string]bool{}
	for _, ek := range m {
		used[ek] = true
	}
	var p Plan
	for _, w := range want {
		if _, ok := m[w.TaskID]; !ok {
			for i := range existing {
				e := &existing[i]
				if !e.Completed && e.Title == w.Title && !used[e.EKID] {
					used[e.EKID] = true
					m[w.TaskID] = e.EKID
					break
				}
			}
		}
		switch e := byID(m[w.TaskID]); {
		case e != nil && e.matches(w):
		case e != nil:
			p.Update = append(p.Update, Update{e.EKID, w})
		default:
			p.Create = append(p.Create, w)
		}
	}
	for _, e := range existing {
		if used[e.EKID] {
			continue
		}
		if ours[e.EKID] {
			p.Remove = append(p.Remove, e.EKID)
		} else if !e.Completed {
			p.Import = append(p.Import, e)
		}
	}
	return p
}

// backend es EventKit (o un doble en las pruebas).
type backend interface {
	Fetch() ([]Existing, error)
	Create(Wanted) (string, error)
	Update(ekID string, w Wanted) error
	Remove(ekID string) error
	Commit() error
}

func statePath() string {
	return filepath.Join(filepath.Dir(store.ConfigPath()), "recordatorios.json")
}

func loadMap() Mapping {
	m := Mapping{}
	if b, err := os.ReadFile(statePath()); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	if m == nil {
		m = Mapping{}
	}
	return m
}

func saveMap(m Mapping) error {
	if err := os.MkdirAll(filepath.Dir(statePath()), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(), b, 0o644)
}

// envOverride: ¿se usa una configuración temporal (TASKTUI_DIR o TASKTUI_SERVER,
// p. ej. en pruebas)? Entonces no se toca Recordatorios: la lista y la relación
// tarea ↔ recordatorio son de tu configuración normal.
func envOverride() bool {
	return os.Getenv("TASKTUI_DIR") != "" || os.Getenv("TASKTUI_SERVER") != ""
}

func entries(s store.Store) []*core.Task {
	var out []*core.Task
	for _, e := range s.Tasks() {
		out = append(out, e.Task)
	}
	return out
}

func wantedList(s store.Store, today core.Date) []Wanted {
	var out []Wanted
	for _, t := range entries(s) {
		if w := WantedFor(t, today); w != nil {
			out = append(out, *w)
		}
	}
	return out
}

func copyMap(m Mapping) Mapping {
	c := Mapping{}
	for k, v := range m {
		c[k] = v
	}
	return c
}

// run sincroniza usando el backend dado.
func run(s store.Store, ek backend) (Report, error) {
	today := core.Today()
	m := loadMap()
	var report Report

	// 1. Lo completado en Recordatorios se completa en la tarea.
	if err := s.Reload(); err != nil {
		return report, err
	}
	pending := map[string]bool{}
	for _, t := range entries(s) {
		if !t.Done && t.ID != nil {
			pending[*t.ID] = true
		}
	}
	existing, err := ek.Fetch()
	if err != nil {
		return report, err
	}
	for _, tid := range CompletedInReminders(m, existing, pending) {
		if _, err := s.Toggle(tid); err != nil {
			return report, err
		}
		report.Completed = append(report.Completed, tid)
	}

	// 2. Lo creado en Recordatorios entra como tarea.
	if err := s.Reload(); err != nil {
		return report, err
	}
	first := MakePlan(wantedList(s, today), existing, copyMap(m))
	for _, e := range first.Import {
		task, project := ImportTask(e, today)
		id, err := s.Add(task, project)
		if err != nil {
			return report, err
		}
		m[id] = e.EKID
		report.Imported = append(report.Imported, id)
	}
	if len(report.Imported) > 0 {
		if err := saveMap(m); err != nil {
			return report, err
		}
	}

	// 3 y 4. Dejar la lista igual a las tareas pendientes con fecha. Los importados
	// ya están en m: se ajustan (o se quitan si no tienen fecha).
	if err := s.Reload(); err != nil {
		return report, err
	}
	p := MakePlan(wantedList(s, today), existing, m)
	for _, u := range p.Update {
		if err := ek.Update(u.EKID, u.Want); err != nil {
			return report, err
		}
	}
	for _, w := range p.Create {
		id, err := ek.Create(w)
		if err != nil {
			return report, err
		}
		m[w.TaskID] = id
	}
	for _, id := range p.Remove {
		if err := ek.Remove(id); err != nil {
			return report, err
		}
	}
	if err := ek.Commit(); err != nil {
		return report, err
	}
	if err := saveMap(m); err != nil {
		return report, err
	}
	report.Created, report.Updated, report.Removed = len(p.Create), len(p.Update), len(p.Remove)
	return report, nil
}

// Sync sincroniza. Se serializa con un candado para que dos «sync» simultáneos
// (TUI + CLI) no creen duplicados.
func Sync(s store.Store) (Report, error) {
	if envOverride() {
		return Report{}, fmt.Errorf("Con TASKTUI_DIR o TASKTUI_SERVER definidos no se sincroniza Recordatorios " +
			"(es solo para tu configuración normal)")
	}
	unlock, err := lockFile(strings.TrimSuffix(statePath(), ".json") + ".lock")
	if err != nil {
		return Report{}, fmt.Errorf("No pude obtener el candado de sincronización: %w", err)
	}
	defer unlock()
	ek, err := openBackend()
	if err != nil {
		return Report{}, err
	}
	return run(s, ek)
}
