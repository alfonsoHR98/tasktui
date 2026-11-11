package core

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Inbox es el proyecto por defecto.
const Inbox = "Inbox"

var weekdays = [7]time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday}

var wdEN = [7]string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}
var wdES = [7]string{"lunes", "martes", "miércoles", "jueves", "viernes", "sábado", "domingo"}

// wdIndex: lunes = 0 … domingo = 6.
func wdIndex(w time.Weekday) int { return (int(w) + 6) % 7 }

func weekdayEN(s string) (time.Weekday, bool) {
	for i, n := range wdEN {
		if strings.EqualFold(n, s) {
			return weekdays[i], true
		}
	}
	return 0, false
}

// Unit es la unidad de una repetición.
type Unit int

const (
	Day Unit = iota
	Week
	Month
	Year
)

// Recurrence es una regla de repetición. Se guarda en el formato del plugin
// Obsidian Tasks («every week on Monday») y se muestra en español («cada lunes»).
type Recurrence struct {
	Interval uint32
	Unit     Unit
	Weekday  *time.Weekday
}

// ParseObsidian lee «every 2 weeks», «every week on Monday», «every monday»…
func ParseObsidian(s string) (*Recurrence, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	rest, ok := strings.CutPrefix(s, "every")
	if !ok {
		return nil, false
	}
	rest = strings.TrimSpace(rest)
	main, on, hasOn := strings.Cut(rest, " on ")
	main = strings.TrimSpace(main)
	var weekday *time.Weekday
	if hasOn {
		first := strings.FieldsFunc(strings.TrimSpace(on), func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
		name := ""
		if len(first) > 0 {
			name = first[0]
		}
		w, ok := weekdayEN(name)
		if !ok {
			return nil, false
		}
		weekday = &w
	}
	parts := strings.Fields(main)
	if len(parts) == 0 {
		return nil, false
	}
	var interval uint32
	var unitS string
	if n, err := strconv.ParseUint(parts[0], 10, 32); err == nil {
		if n == 0 || len(parts) < 2 {
			return nil, false
		}
		interval, unitS = uint32(n), parts[1]
		parts = parts[2:]
	} else {
		interval, unitS = 1, parts[0]
		parts = parts[1:]
	}
	if len(parts) > 0 {
		return nil, false
	}
	var unit Unit
	if w, ok := weekdayEN(unitS); ok {
		weekday = &w
		unit = Week
	} else {
		switch strings.TrimSuffix(unitS, "s") {
		case "day":
			unit = Day
		case "week":
			unit = Week
		case "month":
			unit = Month
		case "year":
			unit = Year
		default:
			return nil, false
		}
	}
	if weekday != nil && unit != Week {
		return nil, false
	}
	return &Recurrence{Interval: interval, Unit: unit, Weekday: weekday}, true
}

func (r Recurrence) unitEN() string {
	return [...]string{"day", "week", "month", "year"}[r.Unit]
}

// ToObsidian devuelve la regla en el formato de Obsidian Tasks.
func (r Recurrence) ToObsidian() string {
	s := "every " + r.unitEN()
	if r.Interval != 1 {
		s = fmt.Sprintf("every %d %ss", r.Interval, r.unitEN())
	}
	if r.Weekday != nil {
		s += " on " + wdEN[wdIndex(*r.Weekday)]
	}
	return s
}

// ToES devuelve la regla en español («cada 2 semanas»).
func (r Recurrence) ToES() string {
	if r.Weekday != nil && r.Interval == 1 {
		return "cada " + wdES[wdIndex(*r.Weekday)]
	}
	names := [...][2]string{{"día", "días"}, {"semana", "semanas"}, {"mes", "meses"}, {"año", "años"}}[r.Unit]
	if r.Interval == 1 {
		return "cada " + names[0]
	}
	return fmt.Sprintf("cada %d %s", r.Interval, names[1])
}

// Next devuelve la siguiente fecha a partir de base; si se sale del calendario, base.
func (r Recurrence) Next(base Date) Date {
	if d, ok := r.tryNext(base); ok {
		return d
	}
	return base
}

func (r Recurrence) tryNext(base Date) (Date, bool) {
	n := int64(r.Interval)
	switch r.Unit {
	case Day:
		return base.AddDays(n)
	case Week:
		if r.Weekday != nil {
			d, ok := base.AddDays(1)
			for ok && d.Weekday() != *r.Weekday {
				d, ok = d.AddDays(1)
			}
			if !ok {
				return Date{}, false
			}
			return d.AddDays((n - 1) * 7)
		}
		return base.AddDays(n * 7)
	case Month:
		return base.AddMonths(r.Interval)
	default:
		if r.Interval > (1<<32-1)/12 {
			return Date{}, false
		}
		return base.AddMonths(r.Interval * 12)
	}
}

// First es la primera fecha cuando la tarea se crea con repetición pero sin fecha.
func (r Recurrence) First(today Date) Date {
	if r.Weekday == nil {
		return today
	}
	d := today
	for d.Weekday() != *r.Weekday {
		d, _ = d.AddDays(1)
	}
	return d
}

// Note es un comentario de seguimiento, con la fecha en que se escribió.
type Note struct {
	Date *Date
	Text string
}

// NewNote recorta el texto.
func NewNote(text string, date *Date) Note {
	return Note{Date: date, Text: strings.TrimSpace(text)}
}

// Task es una tarea.
type Task struct {
	ID *string
	// Status es el carácter dentro de «[ ]»: ' ', 'x', '-', '/'…
	Status rune
	Done   bool
	Text   string
	Labels []string
	// Priority: 1 = máxima … 4 = sin prioridad.
	Priority int
	// Due es cuándo piensas hacerla; Time, la hora de Due.
	Due  *Date
	Time *Clock
	// Deadline es cuándo debe estar hecha.
	Deadline   *Date
	DoneDate   *Date
	Recurrence *Recurrence
	// Extra son campos de Obsidian Tasks que no manejamos, preservados tal cual.
	Extra       []string
	Description string
	Notes       []Note
	Indent      string
	// Project y Section los asigna el almacén.
	Project string
	Section *string
}

// NewTask crea una tarea pendiente sin fechas.
func NewTask(text string) *Task {
	return &Task{Status: ' ', Text: text, Priority: 4}
}

// Clone copia la tarea (con sus listas).
func (t *Task) Clone() *Task {
	c := *t
	c.Labels = append([]string(nil), t.Labels...)
	c.Extra = append([]string(nil), t.Extra...)
	c.Notes = append([]Note(nil), t.Notes...)
	return &c
}

// SetDone marca o desmarca la tarea.
func (t *Task) SetDone(done bool, today Date) {
	t.Done = done
	if done {
		t.Status = 'x'
		t.DoneDate = today.Ptr()
	} else {
		t.Status = ' '
		t.DoneDate = nil
	}
}

// IsOverdue: pendiente con fecha o fecha límite anterior a hoy.
func (t *Task) IsOverdue(today Date) bool {
	if t.Done {
		return false
	}
	return (t.Due != nil && t.Due.Before(today)) || (t.Deadline != nil && t.Deadline.Before(today))
}

// Completion es el resultado de Complete.
type Completion struct {
	// Simple: la tarea ya completada (cuando no se repite).
	Simple *Task
	// Next: la misma tarea (mismo id) con la siguiente fecha; Done: la ocurrencia
	// completada, con id nuevo. Solo en tareas que se repiten.
	Next, Done *Task
}

// Complete completa una tarea pendiente. Si se repite, la pendiente conserva su
// id y avanza a la siguiente fecha, y se crea una ocurrencia hecha con doneID
// (como Obsidian Tasks). Las notas se quedan en la pendiente.
func (t *Task) Complete(today Date, doneID string) Completion {
	if t.Recurrence == nil {
		c := t.Clone()
		c.SetDone(true, today)
		return Completion{Simple: c}
	}
	next := t.Clone()
	base := today
	if t.Due != nil {
		base = *t.Due
	}
	next.Due = t.Recurrence.Next(base).Ptr()
	done := t.Clone()
	done.ID = &doneID
	done.Description = ""
	done.Notes = nil
	done.SetDone(true, today)
	return Completion{Next: next, Done: done}
}

// Recurring indica si el resultado es de una tarea que se repite.
func (c Completion) Recurring() bool { return c.Simple == nil }
