// Package store es el almacén de tareas de la TUI y la CLI: archivos .md locales
// (FileStore) o el servidor (RemoteStore). Quien lo usa no sabe cuál hay detrás.
//
// Cada tarea se identifica por su posición (A, B): en archivos es (documento,
// línea); en remoto es (0, índice).
package store

import (
	"fmt"
	"sort"

	"github.com/alfonsoHR98/tasktui/internal/core"
)

// Pos identifica una tarea dentro del almacén.
type Pos struct{ A, B int }

// Entry es una tarea con su posición.
type Entry struct {
	Pos  Pos
	Task *core.Task
}

// Snapshot es una copia para deshacer (su contenido depende del almacén).
type Snapshot any

// Store es la interfaz común de los dos almacenes.
type Store interface {
	// Notice es un aviso para mostrar al abrir (p. ej. sin conexión).
	Notice() string
	// Dir es la carpeta del modo local ("" en remoto).
	Dir() string
	Reload() error
	// Tasks devuelve todas las tareas en orden (archivo y línea).
	Tasks() []Entry
	TaskAt(Pos) *core.Task
	Find(id string) (Pos, bool)
	Projects() []string
	// Add crea la tarea en el proyecto y devuelve su id.
	Add(t *core.Task, project string) (string, error)
	// Update modifica una tarea. Si cambia Project, se mueve.
	Update(id string, f func(*core.Task)) error
	// Toggle completa o reabre; devuelve true si quedó completada.
	Toggle(id string) (bool, error)
	Delete(id string) error
	AddNote(id, text string) error
	Snapshot() (Snapshot, error)
	Restore(Snapshot) error
}

// View arma una vista con los criterios del núcleo.
func View(s Store, view core.View, today core.Date, showDone bool, query string) []Group {
	entries := s.Tasks()
	items := make([]core.Item, len(entries))
	for i, e := range entries {
		items[i] = core.Item{Key: i, Task: e.Task}
	}
	var out []Group
	for _, g := range core.BuildView(items, s.Projects(), view, today, showDone, query) {
		keys := make([]Pos, len(g.Items))
		for i, k := range g.Items {
			keys[i] = entries[k].Pos
		}
		out = append(out, Group{Title: g.Title, Items: keys})
	}
	return out
}

// Group es un grupo de una vista, con las posiciones de sus tareas.
type Group struct {
	Title *string
	Items []Pos
}

// LabelCount es una etiqueta con su número de tareas pendientes.
type LabelCount struct {
	Name string
	Open int
}

// Labels devuelve las etiquetas con sus pendientes, en orden alfabético.
func Labels(s Store) []LabelCount {
	m := map[string]int{}
	for _, e := range s.Tasks() {
		for _, l := range e.Task.Labels {
			if !e.Task.Done {
				m[l]++
			} else if _, ok := m[l]; !ok {
				m[l] = 0
			}
		}
	}
	out := make([]LabelCount, 0, len(m))
	for n, c := range m {
		out = append(out, LabelCount{n, c})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// FindTask busca una tarea por id.
func FindTask(s Store, id string) (*core.Task, error) {
	p, ok := s.Find(id)
	if !ok {
		return nil, fmt.Errorf("No encontré la tarea «%s»", id)
	}
	t := s.TaskAt(p)
	if t == nil {
		return nil, fmt.Errorf("No encontré la tarea «%s»", id)
	}
	return t, nil
}
