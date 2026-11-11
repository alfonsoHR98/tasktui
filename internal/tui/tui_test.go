package tui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/alfonsoHR98/tasktui/internal/core"
	"github.com/alfonsoHR98/tasktui/internal/store"
)

func newTestModel(t *testing.T, w, h int) *model {
	t.Helper()
	s, err := store.OpenFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Pagar luz mañana 18:00 p1 +Finanzas #casa limit 20/11", "Revisar contrato hoy cada lunes", "Llamar al dentista"} {
		q := core.ParseQuick(text, core.Today())
		tk := q.IntoTask(core.Inbox)
		if _, err := s.Add(tk, tk.Project); err != nil {
			t.Fatal(err)
		}
	}
	m := newModel(s)
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

func key(m *model, k string) {
	switch k {
	case "enter":
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	case "esc":
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	case "backspace":
		m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	case "tab":
		m.Update(tea.KeyMsg{Type: tea.KeyTab})
	case "space":
		m.Update(tea.KeyMsg{Type: tea.KeySpace})
	default:
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	}
}

func typeText(m *model, s string) {
	for _, r := range s {
		if r == ' ' {
			key(m, "space")
		} else {
			key(m, string(r))
		}
	}
}

func screen(m *model) string { return ansi.Strip(m.View()) }

func TestPantallaBasica(t *testing.T) {
	m := newTestModel(t, 130, 30)
	out := screen(m)
	for _, want := range []string{"tasktui", "Inbox", "Hoy", "Revisar contrato", "Detalle", "a añadir"} {
		if !strings.Contains(out, want) {
			t.Errorf("falta %q en la pantalla:\n%s", want, out)
		}
	}
	if os.Getenv("TT_SHOW") != "" {
		t.Log("\n" + out)
	}
}

func TestEscribirLlavesYTNoCierra(t *testing.T) {
	m := newTestModel(t, 100, 30)
	key(m, "a")
	typeText(m, "t {} t { } {t} limit {")
	if m.quitting || m.mode != modeInput {
		t.Fatalf("la captura se cerró: quitting=%v mode=%v", m.quitting, m.mode)
	}
	if !strings.Contains(screen(m), "Nueva tarea") {
		t.Error("no se ve la ventana de captura")
	}
	key(m, "esc")
}

func TestAnadirCompletarYBorrar(t *testing.T) {
	m := newTestModel(t, 100, 30)
	key(m, "g")
	key(m, "i") // ir a Inbox
	key(m, "a")
	typeText(m, "Nueva tarea de prueba p2")
	key(m, "enter")
	if !strings.Contains(screen(m), "Nueva tarea de prueba") {
		t.Fatalf("no apareció la tarea:\n%s", screen(m))
	}
	key(m, "x")
	if !strings.Contains(m.msg, "Completada") {
		t.Errorf("msg = %q", m.msg)
	}
	key(m, "u") // deshacer
	if m.msg != "Deshecho" {
		t.Errorf("msg = %q", m.msg)
	}
	key(m, "d")
	if m.mode != modeConfirm {
		t.Fatal("debía pedir confirmación")
	}
	key(m, "n")
	if m.mode != modeNormal {
		t.Error("n debía cancelar")
	}
}

func TestVariosTamanos(t *testing.T) {
	for _, sz := range [][2]int{{20, 6}, {40, 10}, {80, 24}, {119, 30}, {120, 30}, {200, 50}} {
		m := newTestModel(t, sz[0], sz[1])
		key(m, "a")
		typeText(m, "x +")
		_ = m.View()
		key(m, "esc")
		key(m, "?")
		_ = m.View()
		key(m, "esc")
	}
}

func TestEliminarProyectoDesdePanel(t *testing.T) {
	m := newTestModel(t, 100, 30)
	key(m, "h") // foco al panel
	for i := 0; i < 10 && m.view.Kind != core.ViewProject; i++ {
		key(m, "j")
	}
	if m.view.Kind != core.ViewProject || m.view.Name != "Finanzas" {
		t.Fatalf("vista = %+v", m.view)
	}
	key(m, "d")
	if m.mode != modeConfirm || !strings.Contains(screen(m), "proyecto «Finanzas»") {
		t.Fatalf("debía pedir confirmación:\n%s", screen(m))
	}
	key(m, "n")
	if len(m.store.Projects()) != 2 {
		t.Error("cancelar no debía borrar")
	}
	key(m, "d")
	key(m, "y")
	for _, p := range m.store.Projects() {
		if p == "Finanzas" {
			t.Error("Finanzas sigue existiendo")
		}
	}
	if m.view.Kind != core.ViewToday {
		t.Errorf("debía volver a Hoy: %+v", m.view)
	}
	key(m, "u")
	if len(m.store.Projects()) != 2 {
		t.Error("deshacer debía devolver el proyecto")
	}
}

// TestCapturas escribe pantallas de ejemplo con color (ANSI) en la carpeta
// TT_SHOT, para las imágenes de la documentación: ver scripts/capturas.sh.
func TestCapturas(t *testing.T) {
	dir := os.Getenv("TT_SHOT")
	if dir == "" {
		t.Skip("define TT_SHOT=<carpeta> para generar las capturas")
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	applyTheme(os.Getenv("TASKTUI_THEME"))
	shot := func(name string, w, h int, keys ...string) {
		s, err := store.OpenFiles(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SeedDemo(s, core.Today()); err != nil {
			t.Fatal(err)
		}
		m := newModel(s)
		m.Update(tea.WindowSizeMsg{Width: w, Height: h})
		for _, k := range keys {
			if len([]rune(k)) > 1 && k != "enter" && k != "esc" && k != "tab" && k != "space" {
				typeText(m, k)
			} else {
				key(m, k)
			}
		}
		if err := os.WriteFile(dir+"/"+name+".ansi", []byte(m.View()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	shot("hoy", 150, 34)
	shot("captura", 150, 34, "a", "Mandar el reporte viernes 9:00 p1 +Trabajo #cliente cada mes")
	shot("ayuda", 150, 34, "?")
	shot("todas", 150, 34, "g", "a", "j", "j")
	shot("estrecha", 84, 30, "j")
	shot("vacia", 150, 34, "/", "zzz", "enter")
	shot("lateral", 150, 34, "h", "j", "j", "j", "j")
}
