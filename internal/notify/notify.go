// Package notify avisa en la Mac cuando vence una tarea. La alerta es un diálogo
// de macOS que se queda en pantalla hasta que la cierras (o la completas/pospones).
package notify

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/alfonsoHR98/tasktui/internal/core"
	"github.com/alfonsoHR98/tasktui/internal/store"
)

const (
	// DefaultHour es la hora del aviso para tareas con fecha pero sin hora.
	DefaultHour = 9
	// Gracia: no se avisa de lo que venció hace más de esto (evita ráfagas al arrancar).
	Gracia = 2 * time.Hour
	Snooze = 10 * time.Minute
)

// Due es un aviso pendiente de una tarea.
type Due struct {
	Key  string // identifica la tarea y su hora de aviso
	ID   string // vacío si la tarea no tiene id (no se puede completar desde la alerta)
	Text string
	Proj string
	When time.Time
}

// When calcula cuándo debe sonar el aviso de la tarea (hora local); ok=false si no aplica.
func When(t *core.Task, loc *time.Location) (time.Time, bool) {
	if t.Done || t.Due == nil {
		return time.Time{}, false
	}
	h, m := DefaultHour, 0
	if t.Time != nil {
		h, m = t.Time.H, t.Time.M
	}
	return time.Date(t.Due.Year(), time.Month(t.Due.Month()), t.Due.Day(), h, m, 0, 0, loc), true
}

// Pending devuelve los avisos que ya tocan en «now», sin los ya mostrados (seen) y
// respetando los pospuestos (snoozed: clave → nueva hora).
func Pending(entries []store.Entry, now time.Time, seen map[string]bool, snoozed map[string]time.Time) []Due {
	var out []Due
	for _, e := range entries {
		when, ok := When(e.Task, now.Location())
		if !ok {
			continue
		}
		key := fmt.Sprintf("%s|%s|%s", e.Task.Project, e.Task.Text, when.Format(time.RFC3339))
		if e.Task.ID != nil {
			key = *e.Task.ID + "|" + when.Format(time.RFC3339)
		}
		if at, ok := snoozed[key]; ok {
			when = at
		} else if seen[key] {
			continue
		}
		if now.Before(when) || now.Sub(when) > Gracia+Snooze*6 {
			continue
		}
		d := Due{Key: key, Text: e.Task.Text, Proj: e.Task.Project, When: when}
		if e.Task.ID != nil {
			d.ID = *e.Task.ID
		}
		out = append(out, d)
	}
	return out
}

// Run revisa las tareas cada «every» y muestra las alertas hasta que ctx termine.
func Run(ctx context.Context, s store.Store, every time.Duration, log func(string)) error {
	seen := map[string]bool{}
	snoozed := map[string]time.Time{}
	var mu sync.Mutex // protege seen y snoozed (las alertas corren en goroutines)
	start := time.Now()
	tick := func() {
		if err := s.Reload(); err != nil {
			log("no pude leer las tareas: " + err.Error())
			return
		}
		now := time.Now()
		mu.Lock()
		pend := Pending(s.Tasks(), now, seen, snoozed)
		mu.Unlock()
		for _, d := range pend {
			mu.Lock()
			// Lo que ya había vencido antes de arrancar el demonio no se avisa.
			if _, sn := snoozed[d.Key]; !sn && d.When.Before(start.Add(-time.Minute)) {
				seen[d.Key] = true
				mu.Unlock()
				continue
			}
			seen[d.Key] = true
			delete(snoozed, d.Key)
			mu.Unlock()
			go func(d Due) {
				switch Alert(d) {
				case "Completar":
					if _, err := s.Toggle(d.ID); err != nil {
						log("no pude completar: " + err.Error())
					}
				case "Posponer 10 min":
					mu.Lock()
					seen[d.Key] = false
					snoozed[d.Key] = time.Now().Add(Snooze)
					mu.Unlock()
				}
			}(d)
		}
	}
	tick()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			tick()
		}
	}
}

// Alert muestra el aviso y devuelve el botón pulsado ("" si se cierra o falla).
// Usa una notificación nativa con botones (terminal-notifier); si no está instalado,
// un diálogo fijo de macOS.
func Alert(d Due) string {
	if bin := findNotifier(); bin != "" {
		return alertNative(bin, d)
	}
	return alertDialog(d)
}

// findNotifier busca terminal-notifier. launchd arranca con un PATH mínimo que no
// incluye Homebrew, así que también se prueban sus rutas habituales.
func findNotifier() string {
	if bin, err := exec.LookPath("terminal-notifier"); err == nil {
		return bin
	}
	for _, p := range []string{"/opt/homebrew/bin/terminal-notifier", "/usr/local/bin/terminal-notifier"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// alertNative envía una notificación de macOS con botones y espera la respuesta.
// Para que se quede fija hay que poner terminal-notifier en «Alertas» en
// Ajustes del Sistema → Notificaciones.
func alertNative(bin string, d Due) string {
	msg := d.Proj
	if msg == "" {
		msg = "tasktui"
	}
	actions := "Completar,Posponer 10 min"
	if d.ID == "" {
		actions = "Posponer 10 min"
	}
	out, _ := exec.Command(bin, "-title", "Vence: "+d.Text, "-message", msg+" · "+d.When.Format("15:04"),
		"-sound", "default", "-group", "tasktui-"+d.Key, "-action", actions).Output()
	s := strings.TrimSpace(string(out))
	if s == "Completar" || s == "Posponer 10 min" {
		return s
	}
	return ""
}

// alertDialog es el respaldo: un diálogo fijo con los botones.
func alertDialog(d Due) string {
	buttons := `{"Cerrar", "Posponer 10 min", "Completar"}`
	if d.ID == "" {
		buttons = `{"Cerrar", "Posponer 10 min"}`
	}
	msg := d.Proj
	if msg == "" {
		msg = "tasktui"
	}
	script := fmt.Sprintf(`display alert %s message %s buttons %s default button "Cerrar" as critical`,
		asQuote("Vence: "+d.Text), asQuote(msg+" · "+d.When.Format("15:04")), buttons)
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return ""
	}
	// Salida: «button returned:Completar»
	s := strings.TrimSpace(string(out))
	if i := strings.Index(s, "button returned:"); i >= 0 {
		s = s[i+len("button returned:"):]
	}
	return strings.TrimSpace(strings.SplitN(s, ",", 2)[0])
}

// asQuote escapa un texto para AppleScript.
func asQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
