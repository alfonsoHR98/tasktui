package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alfonsoHR98/tasktui/internal/core"
)

func tempStore(t *testing.T) *FileStore {
	t.Helper()
	s, err := OpenFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestFlujoBasico(t *testing.T) {
	s := tempStore(t)
	write(t, filepath.Join(s.dir, "Casa.md"), "# Casa\nNotas libres\n\n## Compras\n- [ ]   Leche   #super\n")
	if err := s.Reload(); err != nil {
		t.Fatal(err)
	}
	var leche *core.Task
	for _, e := range s.Tasks() {
		if e.Task.Text == "Leche" {
			leche = e.Task
		}
	}
	if leche == nil || leche.Section == nil || *leche.Section != "Compras" || leche.ID == nil {
		t.Fatalf("leche: %+v", leche)
	}
	idLeche := *leche.ID

	id, err := s.Add(core.NewTask("Pan"), "casa")
	if err != nil {
		t.Fatal(err)
	}
	content := read(t, filepath.Join(s.dir, "Casa.md"))
	if !strings.HasPrefix(content, "# Casa\nNotas libres\n") || !strings.Contains(content, "- [ ] Pan 🆔 "+id) {
		t.Errorf("contenido: %q", content)
	}

	// Mover de proyecto crea el archivo
	if err := s.Update(idLeche, func(tk *core.Task) { tk.Project = "Super/Semanal" }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.dir, "Super/Semanal.md")); err != nil {
		t.Error("no se creó Super/Semanal.md")
	}
	p, _ := s.Find(idLeche)
	if s.TaskAt(p).Project != "Super/Semanal" {
		t.Error("el proyecto no cambió")
	}

	// Deshacer
	snap, _ := s.Snapshot()
	if err := s.Delete(id); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Find(id); ok {
		t.Error("debió borrarse")
	}
	if err := s.Restore(snap); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Find(id); !ok {
		t.Error("debió restaurarse")
	}
}

func TestCompletarRecurrente(t *testing.T) {
	s := tempStore(t)
	tk := core.NewTask("Gym")
	tk.Recurrence, _ = core.ParseObsidian("every day")
	tk.Due = core.Today().Ptr()
	id, _ := s.Add(tk, core.Inbox)
	if done, err := s.Toggle(id); err != nil || !done {
		t.Fatalf("%v %v", done, err)
	}
	p, _ := s.Find(id)
	pend := s.TaskAt(p)
	tomorrow, _ := core.Today().AddDays(1)
	if pend.Done || !pend.Due.Equal(tomorrow) {
		t.Errorf("pendiente: %+v", pend)
	}
	n := 0
	for _, e := range s.Tasks() {
		if e.Task.Done {
			n++
		}
	}
	if n != 1 {
		t.Errorf("hechas = %d", n)
	}
}

func TestNotasViajanConLaTarea(t *testing.T) {
	s := tempStore(t)
	write(t, filepath.Join(s.dir, "Inbox.md"), "# Inbox\n\n- [ ] Uno 🆔 aaaaa\n    - nota 1\n  texto suelto\n- [ ] Dos 🆔 bbbbb\n\nFin\n")
	if err := s.Reload(); err != nil {
		t.Fatal(err)
	}
	p, _ := s.Find("aaaaa")
	uno := s.TaskAt(p)
	if len(uno.Notes) != 1 || uno.Notes[0].Text != "nota 1" || uno.Description != "texto suelto" {
		t.Fatalf("uno: %+v", uno)
	}
	if err := s.AddNote("bbbbb", "nota de dos"); err != nil {
		t.Fatal(err)
	}
	if err := s.Update("aaaaa", func(tk *core.Task) { tk.Project = "Otro" }); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("# Inbox\n\n- [ ] Dos 🆔 bbbbb\n    - %s: nota de dos\n\nFin\n", core.Today())
	if got := read(t, filepath.Join(s.dir, "Inbox.md")); got != want {
		t.Errorf("inbox:\n%q\nquería\n%q", got, want)
	}
	if otro := read(t, filepath.Join(s.dir, "Otro.md")); !strings.Contains(otro, "- [ ] Uno 🆔 aaaaa\n    texto suelto\n    - nota 1\n") {
		t.Errorf("otro: %q", otro)
	}
	// Cambiar la descripción reescribe el bloque y conserva las notas.
	if err := s.Update("aaaaa", func(tk *core.Task) { tk.Description = "línea 1\nlínea 2" }); err != nil {
		t.Fatal(err)
	}
	if otro := read(t, filepath.Join(s.dir, "Otro.md")); !strings.Contains(otro, "- [ ] Uno 🆔 aaaaa\n    línea 1\n    línea 2\n    - nota 1\n") {
		t.Errorf("otro: %q", otro)
	}
	p, _ = s.Find("aaaaa")
	if s.TaskAt(p).Description != "línea 1\nlínea 2" {
		t.Error("descripción")
	}
	if err := s.Delete("aaaaa"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(read(t, filepath.Join(s.dir, "Otro.md")), "nota") {
		t.Error("las notas deben borrarse con la tarea")
	}
}
