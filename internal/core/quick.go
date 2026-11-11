package core

import (
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Quick es el resultado de interpretar una captura rápida en español:
//
//	Pagar luz mañana p1 #casa +Finanzas cada mes
//
// Etiquetas: #etiqueta o @etiqueta; proyecto: +Proyecto (con _ por espacios);
// prioridad p1..p4; fechas (hoy, mañana, lunes, «en 3 días», 15/11, 2026-11-15,
// también en inglés); hora (18:00, 9am); fecha límite («limit 20/11», «{20/11}»);
// repetición («cada lunes», «every 2 weeks»). «\palabra» fuerza texto literal.
type Quick struct {
	Text       string
	Labels     []string
	Priority   *int
	Due        *Date
	Time       *Clock
	Deadline   *Date
	Recurrence *Recurrence
	Project    *string
}

type effectKind int

const (
	eLabel effectKind = iota
	eProject
	ePriority
	eDue
	eTime
	eDeadline
	eRec
)

type effect struct {
	kind effectKind
	s    string
	n    int
	d    Date
	c    Clock
	rec  Recurrence
}

// timeLit lee 18:00, 9:30, 9am, 6pm, 6:30pm.
func timeLit(s string) (Clock, bool) {
	body := s
	var pm *bool
	if b, ok := strings.CutSuffix(s, "am"); ok {
		body, pm = b, ptrBool(false)
	} else if b, ok := strings.CutSuffix(s, "pm"); ok {
		body, pm = b, ptrBool(true)
	}
	var h, m int
	if hs, ms, ok := strings.Cut(body, ":"); ok {
		if len(ms) != 2 {
			return Clock{}, false
		}
		var e1, e2 error
		h, e1 = atoiU(hs)
		m, e2 = atoiU(ms)
		if e1 != nil || e2 != nil {
			return Clock{}, false
		}
	} else if pm != nil {
		var err error
		if h, err = atoiU(body); err != nil {
			return Clock{}, false
		}
	} else {
		return Clock{}, false
	}
	if pm != nil {
		if h < 1 || h > 12 {
			return Clock{}, false
		}
		if *pm {
			h = h%12 + 12
		} else {
			h %= 12
		}
	}
	return NewClock(h, m)
}

func ptrBool(b bool) *bool { return &b }

// atoiU es como strconv.Atoi pero solo acepta dígitos (sin signo).
func atoiU(s string) (int, error) {
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, strconv.ErrSyntax
		}
	}
	return strconv.Atoi(s)
}

// dateOf lee una fecha escrita con los tokens completos (para «limit …» y «{…}»).
func dateOf(toks []string, today Date) (int, Date, bool) {
	if len(toks) == 0 {
		return 0, Date{}, false // «{}» o «{ }»: no hay fecha dentro
	}
	n, e, ok := trySpecial(toks, 0, today)
	if !ok || e.kind != eDue {
		return 0, Date{}, false
	}
	return n, e.d, true
}

func unitES(s string) (Unit, bool) {
	switch s {
	case "día", "dia", "días", "dias", "day", "days":
		return Day, true
	case "semana", "semanas", "week", "weeks":
		return Week, true
	case "mes", "meses", "month", "months":
		return Month, true
	case "año", "ano", "años", "anos", "year", "years":
		return Year, true
	}
	return 0, false
}

// addUnits suma n unidades a today; ok es false si se sale del calendario.
func addUnits(today Date, n uint32, u Unit) (Date, bool) {
	switch u {
	case Day:
		return today.AddDays(int64(n))
	case Week:
		return today.AddDays(int64(n) * 7)
	case Month:
		return today.AddMonths(n)
	default:
		if n > (1<<32-1)/12 {
			return Date{}, false
		}
		return today.AddMonths(n * 12)
	}
}

func dateLit(s string, today Date) (Date, bool) {
	if d, ok := ParseDate(s); ok && strings.Count(s, "-") == 2 {
		return d, true
	}
	parts := strings.Split(s, "/")
	if len(parts) < 2 {
		return Date{}, false
	}
	day, e1 := atoiU(parts[0])
	month, e2 := atoiU(parts[1])
	if e1 != nil || e2 != nil {
		return Date{}, false
	}
	switch len(parts) {
	case 2:
		d, ok := NewDate(today.Year(), month, day)
		if !ok {
			return Date{}, false
		}
		if d.Before(today) {
			return NewDate(today.Year()+1, month, day)
		}
		return d, true
	case 3:
		y, err := atoiU(parts[2])
		if err != nil {
			return Date{}, false
		}
		if y < 100 {
			y += 2000
		}
		return NewDate(y, month, day)
	}
	return Date{}, false
}

// trySpecial intenta interpretar los tokens desde i. Devuelve cuántos consume.
func trySpecial(toks []string, i int, today Date) (int, effect, bool) {
	tok := toks[i]
	low := strings.ToLower(tok)
	next := func(k int) (string, bool) {
		if i+k < len(toks) {
			return strings.ToLower(toks[i+k]), true
		}
		return "", false
	}
	no := effect{}

	if l, ok := cutPrefix2(tok, "#", "@"); ok {
		if l != "" && !strings.HasPrefix(l, "#") && !strings.HasPrefix(l, "@") && strings.IndexFunc(l, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return 1, effect{kind: eLabel, s: l}, true
		}
		return 0, no, false
	}
	if p, ok := strings.CutPrefix(tok, "+"); ok {
		if strings.IndexFunc(p, unicode.IsLetter) >= 0 {
			return 1, effect{kind: eProject, s: strings.ReplaceAll(p, "_", " ")}, true
		}
		return 0, no, false
	}
	// Fecha límite entre llaves: {20/11}, {viernes}, {en 3 días}
	if strings.HasPrefix(tok, "{") {
		end := -1
		for j := i; j < len(toks); j++ {
			if strings.HasSuffix(toks[j], "}") {
				end = j
				break
			}
		}
		if end < 0 {
			return 0, no, false
		}
		inner := strings.Join(toks[i:end+1], " ")
		inner = strings.TrimRight(strings.TrimLeft(inner, "{"), "}")
		words := strings.Fields(inner)
		if n, d, ok := dateOf(words, today); ok && n == len(words) {
			return end - i + 1, effect{kind: eDeadline, d: d}, true
		}
		return 0, no, false
	}
	if c, ok := timeLit(low); ok {
		return 1, effect{kind: eTime, c: c}, true
	}
	switch low {
	case "limit":
		n, d, ok := dateOf(toks[i+1:], today)
		if !ok {
			return 0, no, false
		}
		return n + 1, effect{kind: eDeadline, d: d}, true
	case "p1", "p2", "p3", "p4":
		return 1, effect{kind: ePriority, n: int(low[1] - '0')}, true
	case "hoy", "today":
		return 1, effect{kind: eDue, d: today}, true
	case "mañana", "manana", "tomorrow":
		if d, ok := today.AddDays(1); ok {
			return 1, effect{kind: eDue, d: d}, true
		}
	case "pasado":
		if n1, ok := next(1); ok && (n1 == "mañana" || n1 == "manana") {
			if d, ok := today.AddDays(2); ok {
				return 2, effect{kind: eDue, d: d}, true
			}
		}
	case "en", "in":
		n1, ok1 := next(1)
		n2, ok2 := next(2)
		if ok1 && ok2 {
			if n, err := strconv.ParseUint(n1, 10, 32); err == nil {
				if u, ok := unitES(n2); ok {
					d, ok := addUnits(today, uint32(n), u)
					if !ok {
						return 0, no, false
					}
					return 3, effect{kind: eDue, d: d}, true
				}
			}
		}
	case "cada", "every":
		if n1, ok := next(1); ok {
			if n, err := strconv.ParseUint(n1, 10, 32); err == nil {
				if n2, ok := next(2); ok && n > 0 {
					if u, ok := unitES(n2); ok {
						return 3, effect{kind: eRec, rec: Recurrence{Interval: uint32(n), Unit: u}}, true
					}
				}
			} else if u, ok := unitES(n1); ok {
				return 2, effect{kind: eRec, rec: Recurrence{Interval: 1, Unit: u}}, true
			} else if w, ok := WeekdayES(n1); ok {
				return 2, effect{kind: eRec, rec: Recurrence{Interval: 1, Unit: Week, Weekday: &w}}, true
			}
		}
	}
	if w, ok := WeekdayES(low); ok {
		if d, ok := NextWeekday(today, w); ok {
			return 1, effect{kind: eDue, d: d}, true
		}
	}
	if d, ok := dateLit(low, today); ok {
		return 1, effect{kind: eDue, d: d}, true
	}
	return 0, no, false
}

func cutPrefix2(s, a, b string) (string, bool) {
	if r, ok := strings.CutPrefix(s, a); ok {
		return r, true
	}
	return strings.CutPrefix(s, b)
}

// ParseQuick interpreta una captura rápida.
func ParseQuick(input string, today Date) Quick {
	toks := strings.Fields(input)
	var q Quick
	var text []string
	for i := 0; i < len(toks); {
		if lit, ok := strings.CutPrefix(toks[i], "\\"); ok && lit != "" {
			text = append(text, lit)
			i++
			continue
		}
		n, e, ok := trySpecial(toks, i, today)
		if !ok {
			text = append(text, toks[i])
			i++
			continue
		}
		switch e.kind {
		case eLabel:
			dup := false
			for _, l := range q.Labels {
				dup = dup || l == e.s
			}
			if !dup {
				q.Labels = append(q.Labels, e.s)
			}
		case eProject:
			s := e.s
			q.Project = &s
		case ePriority:
			p := e.n
			q.Priority = &p
		case eDue:
			q.Due = e.d.Ptr()
		case eTime:
			q.Time = e.c.Ptr()
		case eDeadline:
			q.Deadline = e.d.Ptr()
		case eRec:
			r := e.rec
			q.Recurrence = &r
		}
		i += n
	}
	if q.Recurrence != nil && q.Due == nil {
		q.Due = q.Recurrence.First(today).Ptr()
	}
	if q.Time != nil && q.Due == nil {
		q.Due = today.Ptr()
	}
	q.Text = strings.Join(text, " ")
	return q
}

// IntoTask crea una tarea nueva. Sin +Proyecto, va a defaultProject.
func (q Quick) IntoTask(defaultProject string) *Task {
	t := NewTask(q.Text)
	t.Labels = q.Labels
	t.Priority = 4
	if q.Priority != nil {
		t.Priority = *q.Priority
	}
	t.Due, t.Time, t.Deadline, t.Recurrence = q.Due, q.Time, q.Deadline, q.Recurrence
	t.Project = defaultProject
	if q.Project != nil {
		t.Project = *q.Project
	}
	return t
}

// ApplyPartial cambia solo lo que se escribió («tasktui edit id viernes p2»).
func (q Quick) ApplyPartial(t *Task) {
	if q.Text != "" {
		t.Text = q.Text
	}
	if len(q.Labels) > 0 {
		t.Labels = q.Labels
	}
	if q.Priority != nil {
		t.Priority = *q.Priority
	}
	if q.Due != nil {
		t.Due = q.Due
	}
	if q.Time != nil {
		t.Time = q.Time
	}
	if q.Deadline != nil {
		t.Deadline = q.Deadline
	}
	if q.Recurrence != nil {
		t.Recurrence = q.Recurrence
	}
	if q.Project != nil {
		t.Project = *q.Project
	}
}

// ApplyFull reemplaza todo con lo escrito (editar la línea completa, como en la
// TUI con ToQuick). El proyecto solo cambia si se escribe +Proyecto.
func (q Quick) ApplyFull(t *Task) {
	t.Text = q.Text
	t.Labels = q.Labels
	t.Priority = 4
	if q.Priority != nil {
		t.Priority = *q.Priority
	}
	t.Due, t.Time, t.Deadline, t.Recurrence = q.Due, q.Time, q.Deadline, q.Recurrence
	if q.Project != nil {
		t.Project = *q.Project
	}
}

// ToQuick es la representación editable de una tarea (lo inverso de ParseQuick).
func ToQuick(t *Task, today Date) string {
	words := strings.Fields(t.Text)
	var out []string
	for i, w := range words {
		if _, _, special := trySpecial(words, i, today); strings.HasPrefix(w, "\\") || special {
			out = append(out, "\\"+w)
		} else {
			out = append(out, w)
		}
	}
	for _, l := range t.Labels {
		out = append(out, "#"+l)
	}
	if t.Priority < 4 {
		out = append(out, "p"+strconv.Itoa(t.Priority))
	}
	if t.Due != nil {
		out = append(out, t.Due.String())
	}
	if t.Time != nil {
		out = append(out, t.Time.String())
	}
	if t.Deadline != nil {
		out = append(out, "limit "+t.Deadline.String())
	}
	if t.Recurrence != nil {
		out = append(out, t.Recurrence.ToES())
	}
	if t.Project != "" {
		out = append(out, "+"+strings.ReplaceAll(t.Project, " ", "_"))
	}
	return strings.Join(out, " ")
}

var _ = time.Monday
