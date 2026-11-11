package core

import (
	"sort"
	"strings"
)

// ViewKind distingue las vistas.
type ViewKind int

const (
	ViewInbox ViewKind = iota
	ViewToday
	ViewUpcoming
	ViewAll
	ViewProject
	ViewLabel
)

// View es qué tareas se muestran: Inbox, Hoy, Próximos, Todas, un proyecto o una etiqueta.
type View struct {
	Kind ViewKind
	Name string // proyecto o etiqueta
}

// Title es el nombre de la vista.
func (v View) Title() string {
	switch v.Kind {
	case ViewInbox:
		return Inbox
	case ViewToday:
		return "Hoy"
	case ViewUpcoming:
		return "Próximos"
	case ViewAll:
		return "Todas"
	case ViewProject:
		return v.Name
	default:
		return "#" + v.Name
	}
}

// ParseView lee «hoy», «inbox», «proximos», «todas», «+Proyecto», «#etiqueta».
func ParseView(s string) (View, bool) {
	switch strings.ToLower(s) {
	case "hoy", "today":
		return View{Kind: ViewToday}, true
	case "inbox", "entrada":
		return View{Kind: ViewInbox}, true
	case "proximos", "próximos", "upcoming":
		return View{Kind: ViewUpcoming}, true
	case "todas", "todo", "all":
		return View{Kind: ViewAll}, true
	}
	if p, ok := strings.CutPrefix(s, "+"); ok {
		return View{Kind: ViewProject, Name: strings.ReplaceAll(p, "_", " ")}, true
	}
	if l, ok := cutPrefix2(s, "#", "@"); ok {
		return View{Kind: ViewLabel, Name: l}, true
	}
	return View{}, false
}

// Item identifica una tarea para quien arma la vista (posición, índice…).
type Item struct {
	Key  int
	Task *Task
}

// Group es un grupo de la vista.
type Group struct {
	Title *string
	Items []int // claves (Item.Key)
}

func title(s string) *string { return &s }

func matchesQuery(t *Task, q string) bool {
	if q == "" || strings.Contains(strings.ToLower(t.Text), q) || strings.Contains(strings.ToLower(t.Project), q) {
		return true
	}
	for _, l := range t.Labels {
		if strings.Contains(strings.ToLower(l), q) {
			return true
		}
	}
	return false
}

func pushGrouped(groups *[]Group, t *string, key int) {
	if n := len(*groups); n > 0 {
		g := &(*groups)[n-1]
		if (g.Title == nil && t == nil) || (g.Title != nil && t != nil && *g.Title == *t) {
			g.Items = append(g.Items, key)
			return
		}
	}
	*groups = append(*groups, Group{Title: t, Items: []int{key}})
}

// cmpClock: las que tienen hora van antes; entre ellas, por hora.
func cmpClock(a, b *Clock) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	case a.Less(*b):
		return -1
	case b.Less(*a):
		return 1
	}
	return 0
}

// cmpDate: sin fecha antes que con fecha (como Option en Rust).
func cmpDate(a, b *Date) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	case a.Before(*b):
		return -1
	case a.After(*b):
		return 1
	}
	return 0
}

// byPriority: primero las que tienen hora (en orden), luego por prioridad y fecha.
func byPriority(v []Item) {
	sort.SliceStable(v, func(i, j int) bool {
		a, b := v[i].Task, v[j].Task
		if c := cmpClock(a.Time, b.Time); c != 0 {
			return c < 0
		}
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		return cmpDate(a.Due, b.Due) < 0
	})
}

// Projects devuelve los proyectos: Inbox primero y luego alfabético, sin repetir.
func Projects(tasks []*Task) []string {
	ps := make([]string, 0, len(tasks))
	for _, t := range tasks {
		ps = append(ps, t.Project)
	}
	sort.SliceStable(ps, func(i, j int) bool {
		a, b := ps[i] != Inbox, ps[j] != Inbox
		if a != b {
			return !a
		}
		return strings.ToLower(ps[i]) < strings.ToLower(ps[j])
	})
	out := ps[:0]
	for i, p := range ps {
		if i == 0 || p != ps[i-1] {
			out = append(out, p)
		}
	}
	return out
}

// BuildView arma una vista. tasks va en el orden en que aparecen (archivo y línea);
// projects en el orden de la vista Todas (ver Projects).
func BuildView(tasks []Item, projects []string, view View, today Date, showDone bool, query string) []Group {
	q := strings.ToLower(query)
	visible := func(t *Task) bool { return (showDone || !t.Done) && matchesQuery(t, q) }
	var groups []Group

	switch view.Kind {
	case ViewInbox, ViewProject:
		name := strings.ToLower(Inbox)
		if view.Kind == ViewProject {
			name = strings.ToLower(view.Name)
		}
		for _, it := range tasks {
			if strings.ToLower(it.Task.Project) == name && visible(it.Task) {
				pushGrouped(&groups, it.Task.Section, it.Key)
			}
		}
	case ViewToday:
		var over, tod, done []Item
		for _, it := range tasks {
			t := it.Task
			if !matchesQuery(t, q) {
				continue
			}
			if !t.Done {
				// La fecha límite también cuenta: si es hoy o ya pasó, aparece.
				late := (t.Due != nil && t.Due.Before(today)) || (t.Deadline != nil && t.Deadline.Before(today))
				now := (t.Due != nil && t.Due.Equal(today)) || (t.Deadline != nil && t.Deadline.Equal(today))
				if late {
					over = append(over, it)
				} else if now {
					tod = append(tod, it)
				}
			} else if showDone && t.DoneDate != nil && t.DoneDate.Equal(today) {
				done = append(done, it)
			}
		}
		byPriority(over)
		byPriority(tod)
		for _, g := range []struct {
			title string
			items []Item
		}{{"Vencidas", over}, {Long(today, today), tod}, {"Completadas hoy", done}} {
			keys := make([]int, len(g.items))
			for i, it := range g.items {
				keys[i] = it.Key
			}
			groups = append(groups, Group{Title: title(g.title), Items: keys})
		}
	case ViewUpcoming:
		var v []Item
		for _, it := range tasks {
			t := it.Task
			if !t.Done && t.Due != nil && t.Due.After(today) && matchesQuery(t, q) {
				v = append(v, it)
			}
		}
		sort.SliceStable(v, func(i, j int) bool {
			a, b := v[i].Task, v[j].Task
			if c := cmpDate(a.Due, b.Due); c != 0 {
				return c < 0
			}
			if c := cmpClock(a.Time, b.Time); c != 0 {
				return c < 0
			}
			return a.Priority < b.Priority
		})
		for _, it := range v {
			pushGrouped(&groups, title(Long(*it.Task.Due, today)), it.Key)
		}
	case ViewAll:
		for _, p := range projects {
			var keys []int
			for _, it := range tasks {
				if it.Task.Project == p && visible(it.Task) {
					keys = append(keys, it.Key)
				}
			}
			groups = append(groups, Group{Title: title(p), Items: keys})
		}
	case ViewLabel:
		l := strings.ToLower(view.Name)
		for _, it := range tasks {
			if !visible(it.Task) {
				continue
			}
			for _, x := range it.Task.Labels {
				if strings.ToLower(x) == l {
					pushGrouped(&groups, title(it.Task.Project), it.Key)
					break
				}
			}
		}
	}
	out := groups[:0]
	for _, g := range groups {
		if len(g.Items) > 0 {
			out = append(out, g)
		}
	}
	return out
}
