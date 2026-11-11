package server

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/alfonsoHR98/tasktui/internal/core"
)

// APIError es un error de la API con su código HTTP. Los mensajes van en español.
type APIError struct {
	Status int
	Msg    string
}

func (e *APIError) Error() string { return e.Msg }

func bad(msg string) *APIError { return &APIError{400, msg} }

func notFound(id string) *APIError {
	return &APIError{404, fmt.Sprintf("No existe la tarea «%s»", id)}
}

func internal(err error) *APIError {
	fmt.Printf("{\"nivel\":\"error\",\"error\":%q}\n", err.Error())
	return &APIError{500, "Error interno"}
}

type result struct {
	status int
	body   any
}

func ok(v any) (result, *APIError) { return result{200, v}, nil }

type query map[string]string

// param acepta también el nombre anterior en español.
func (q query) param(key string) (string, bool) {
	alias := map[string]string{"view": "vista", "all": "todas", "today": "hoy"}[key]
	if v, ok := q[key]; ok {
		return v, true
	}
	if alias != "" {
		v, ok := q[alias]
		return v, ok
	}
	return "", false
}

// today es el «hoy» del usuario: ?today= si viene; si no, la hora actual con el desfase de la zona.
func today(q query, tzMin int) core.Date {
	if s, ok := q.param("today"); ok {
		if d, ok := core.ParseDate(s); ok {
			return d
		}
	}
	tzMin = min(max(tzMin, -1440), 1440)
	n := time.Now().UTC().Add(time.Duration(tzMin) * time.Minute)
	d, _ := core.NewDate(n.Year(), int(n.Month()), n.Day())
	return d
}

// uniqueID genera un id corto de 5 caracteres (no es secreto).
func (d *DB) uniqueID() (string, error) {
	const alpha = "0123456789abcdefghijklmnopqrstuvwxyz"
	for {
		b := make([]byte, 5)
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		for i := range b {
			b[i] = alpha[int(b[i])%36]
		}
		exists, err := d.exists(string(b))
		if err != nil {
			return "", err
		}
		if !exists {
			return string(b), nil
		}
	}
}

func decode(raw []byte, v any, msg string) *APIError {
	if err := json.Unmarshal(raw, v); err != nil {
		return bad(msg)
	}
	return nil
}

// first devuelve el primer valor no vacío (para los alias en español).
func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func dto(t *core.Task) any { return core.ToDTO(t) }

// Handle atiende una llamada a /api/* (el token ya está validado). No conoce HTTP:
// recibe método, ruta con consulta y cuerpo, y devuelve (código, JSON). Lo reutiliza
// también el servidor MCP. pathAndQuery es, por ejemplo, «/api/tasks?view=today».
func (d *DB) Handle(tzMin int, method, pathAndQuery string, raw []byte) (int, any) {
	r, err := d.route(tzMin, method, pathAndQuery, raw)
	if err != nil {
		return err.Status, map[string]string{"error": err.Msg}
	}
	return r.status, r.body
}

func (d *DB) route(tzMin int, method, pathAndQuery string, raw []byte) (result, *APIError) {
	u, perr := url.Parse(pathAndQuery)
	if perr != nil {
		return result{}, bad("Ruta inválida")
	}
	q := query{}
	for k, v := range u.Query() {
		q[k] = v[0]
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	parts := strings.Split(strings.TrimRight(strings.TrimPrefix(u.Path, "/api/"), "/"), "/")
	switch {
	case method == "GET" && len(parts) == 1 && parts[0] == "tasks":
		return d.list(q, tzMin)
	case method == "POST" && len(parts) == 1 && parts[0] == "tasks":
		return d.add(q, tzMin, raw)
	case len(parts) == 2 && parts[0] == "tasks":
		id := parts[1]
		switch method {
		case "GET":
			t, err := d.get(id)
			if err != nil {
				return result{}, internal(err)
			}
			if t == nil {
				return result{}, notFound(id)
			}
			return ok(dto(t))
		case "PATCH":
			return d.edit(q, tzMin, id, raw)
		case "PUT":
			return d.put(id, raw)
		case "DELETE":
			del, err := d.db.Exec("DELETE FROM tasks WHERE id = ?1", id)
			if err != nil {
				return result{}, internal(err)
			}
			if n, _ := del.RowsAffected(); n == 0 {
				return result{}, notFound(id)
			}
			return ok(map[string]string{"deleted": id})
		}
	case method == "POST" && len(parts) == 3 && parts[0] == "tasks" && parts[2] == "done":
		return d.toggle(q, tzMin, parts[1])
	case method == "POST" && len(parts) == 3 && parts[0] == "tasks" && parts[2] == "notes":
		return d.addNote(q, tzMin, parts[1], raw)
	case method == "GET" && len(parts) == 1 && parts[0] == "projects":
		return d.projectsOut()
	case method == "POST" && len(parts) == 1 && parts[0] == "projects":
		return d.createProject(raw)
	case method == "DELETE" && len(parts) == 1 && parts[0] == "projects":
		return d.deleteProjectAPI(q)
	case method == "GET" && len(parts) == 1 && parts[0] == "labels":
		return d.labels()
	case method == "GET" && len(parts) == 1 && parts[0] == "export":
		return d.export()
	case method == "POST" && len(parts) == 1 && parts[0] == "import":
		return d.importAll(raw)
	}
	return result{}, &APIError{404, "Ruta no encontrada"}
}

func (d *DB) list(q query, tzMin int) (result, *APIError) {
	name, has := q.param("view")
	if !has {
		name = "today"
	}
	view, okView := core.ParseView(name)
	if !okView {
		return result{}, bad(fmt.Sprintf("Vista desconocida «%s». Usa: today, inbox, upcoming, all, +Proyecto, #etiqueta", name))
	}
	allS, _ := q.param("all")
	showDone := allS == "1" || allS == "true"
	text := q["q"]
	hoy := today(q, tzMin)
	tasks, err := d.all()
	if err != nil {
		return result{}, internal(err)
	}
	projects, err := d.projects()
	if err != nil {
		return result{}, internal(err)
	}
	items := make([]core.Item, len(tasks))
	for i, t := range tasks {
		items[i] = core.Item{Key: i, Task: t}
	}
	groups := []map[string]any{}
	for _, g := range core.BuildView(items, projects, view, hoy, showDone, text) {
		ts := make([]core.TaskDTO, len(g.Items))
		for i, k := range g.Items {
			ts[i] = core.ToDTO(tasks[k])
		}
		groups = append(groups, map[string]any{"title": g.Title, "tasks": ts})
	}
	return ok(map[string]any{"view": view.Title(), "today": hoy, "groups": groups})
}

func (d *DB) add(q query, tzMin int, raw []byte) (result, *APIError) {
	var b struct {
		Text, Texto       string
		Project, Proyecto *string
		Description       string
	}
	if e := decode(raw, &b, "Se esperaba {\"text\": \"…\"}"); e != nil {
		return result{}, e
	}
	parsed := core.ParseQuick(first(b.Text, b.Texto), today(q, tzMin))
	if parsed.Text == "" {
		return result{}, bad("La tarea necesita texto")
	}
	project := core.Inbox
	if b.Project != nil {
		project = *b.Project
	} else if b.Proyecto != nil {
		project = *b.Proyecto
	}
	t := parsed.IntoTask(project)
	t.Description = strings.TrimSpace(b.Description)
	var err error
	if t.Project, err = d.canonicalProject(t.Project); err != nil {
		return result{}, internal(err)
	}
	id, err := d.uniqueID()
	if err != nil {
		return result{}, internal(err)
	}
	t.ID = &id
	if err := d.addProject(t.Project); err != nil {
		return result{}, internal(err)
	}
	if err := d.save(t); err != nil {
		return result{}, internal(err)
	}
	return result{201, dto(t)}, nil
}

func (d *DB) edit(q query, tzMin int, id string, raw []byte) (result, *APIError) {
	var b struct {
		Text, Texto                  *string
		Full, Completo               bool
		NoDate, SinFecha, NoDeadline bool
		Description                  *string
	}
	// Los campos con alias en español: json no admite alias, así que se leen por separado.
	var m map[string]json.RawMessage
	if e := decode(raw, &m, "Cuerpo JSON inválido"); e != nil {
		return result{}, e
	}
	get := func(keys ...string) json.RawMessage {
		for _, k := range keys {
			if v, ok := m[k]; ok {
				return v
			}
		}
		return nil
	}
	if v := get("text", "texto"); v != nil {
		var s string
		if json.Unmarshal(v, &s) == nil {
			b.Text = &s
		}
	}
	flag := func(keys ...string) bool {
		var f bool
		if v := get(keys...); v != nil {
			_ = json.Unmarshal(v, &f)
		}
		return f
	}
	b.Full, b.NoDate, b.NoDeadline = flag("full", "completo"), flag("no_date", "sin_fecha"), flag("no_deadline")
	if v := get("description"); v != nil {
		var s string
		if json.Unmarshal(v, &s) == nil {
			b.Description = &s
		}
	}
	t, err := d.get(id)
	if err != nil {
		return result{}, internal(err)
	}
	if t == nil {
		return result{}, notFound(id)
	}
	if b.Text != nil {
		parsed := core.ParseQuick(*b.Text, today(q, tzMin))
		if b.Full {
			if parsed.Text == "" {
				return result{}, bad("La tarea necesita texto")
			}
			parsed.ApplyFull(t)
		} else {
			parsed.ApplyPartial(t)
		}
	}
	if b.NoDate {
		t.Due, t.Time, t.Recurrence = nil, nil, nil
	}
	if b.NoDeadline {
		t.Deadline = nil
	}
	if b.Description != nil {
		t.Description = strings.TrimSpace(*b.Description)
	}
	if t.Project, err = d.canonicalProject(t.Project); err != nil {
		return result{}, internal(err)
	}
	if err := d.addProject(t.Project); err != nil {
		return result{}, internal(err)
	}
	if err := d.save(t); err != nil {
		return result{}, internal(err)
	}
	return ok(dto(t))
}

// put crea o reemplaza la tarea completa con el id de la ruta.
func (d *DB) put(id string, raw []byte) (result, *APIError) {
	var in core.TaskDTO
	if e := decode(raw, &in, "Se esperaba una tarea en JSON"); e != nil {
		return result{}, e
	}
	t := core.FromDTO(&in)
	if strings.TrimSpace(t.Text) == "" {
		return result{}, bad("La tarea necesita texto")
	}
	if id == "" || len(id) > 32 || strings.IndexFunc(id, func(r rune) bool {
		return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z')
	}) >= 0 {
		return result{}, bad("Id inválido")
	}
	t.ID = &id
	if strings.TrimSpace(t.Project) == "" {
		t.Project = core.Inbox
	}
	var err error
	if t.Project, err = d.canonicalProject(t.Project); err != nil {
		return result{}, internal(err)
	}
	if err := d.addProject(t.Project); err != nil {
		return result{}, internal(err)
	}
	if err := d.save(t); err != nil {
		return result{}, internal(err)
	}
	return ok(dto(t))
}

func (d *DB) toggle(q query, tzMin int, id string) (result, *APIError) {
	hoy := today(q, tzMin)
	t, err := d.get(id)
	if err != nil {
		return result{}, internal(err)
	}
	if t == nil {
		return result{}, notFound(id)
	}
	if t.Done {
		t.SetDone(false, hoy)
		if err := d.save(t); err != nil {
			return result{}, internal(err)
		}
		return ok(map[string]any{"task": dto(t), "completed": false})
	}
	doneID, err := d.uniqueID()
	if err != nil {
		return result{}, internal(err)
	}
	c := t.Complete(hoy, doneID)
	if !c.Recurring() {
		if err := d.save(c.Simple); err != nil {
			return result{}, internal(err)
		}
		return ok(map[string]any{"task": dto(c.Simple), "completed": true})
	}
	if err := d.save(c.Done); err != nil {
		return result{}, internal(err)
	}
	if err := d.save(c.Next); err != nil {
		return result{}, internal(err)
	}
	return ok(map[string]any{"task": dto(c.Next), "completed": true, "occurrence": dto(c.Done)})
}

func (d *DB) addNote(q query, tzMin int, id string, raw []byte) (result, *APIError) {
	var b struct{ Text, Texto string }
	if e := decode(raw, &b, "Se esperaba {\"text\": \"…\"}"); e != nil {
		return result{}, e
	}
	text := first(b.Text, b.Texto)
	if strings.TrimSpace(text) == "" {
		return result{}, bad("La nota está vacía")
	}
	t, err := d.get(id)
	if err != nil {
		return result{}, internal(err)
	}
	if t == nil {
		return result{}, notFound(id)
	}
	t.Notes = append(t.Notes, core.NewNote(text, today(q, tzMin).Ptr()))
	if err := d.save(t); err != nil {
		return result{}, internal(err)
	}
	return ok(dto(t))
}

func (d *DB) projectsOut() (result, *APIError) {
	tasks, err := d.all()
	if err != nil {
		return result{}, internal(err)
	}
	ps, err := d.projects()
	if err != nil {
		return result{}, internal(err)
	}
	out := []map[string]any{}
	for _, p := range ps {
		open := 0
		for _, t := range tasks {
			if !t.Done && strings.EqualFold(t.Project, p) {
				open++
			}
		}
		out = append(out, map[string]any{"name": p, "open": open})
	}
	return ok(out)
}

func (d *DB) createProject(raw []byte) (result, *APIError) {
	var b struct{ Name, Nombre string }
	if e := decode(raw, &b, "Se esperaba {\"name\": \"…\"}"); e != nil {
		return result{}, e
	}
	name := strings.TrimSpace(first(b.Name, b.Nombre))
	if name == "" {
		return result{}, bad("El proyecto necesita nombre")
	}
	if err := d.addProject(name); err != nil {
		return result{}, internal(err)
	}
	return result{201, map[string]string{"name": name}}, nil
}

func (d *DB) deleteProjectAPI(q query) (result, *APIError) {
	name, _ := q.param("name")
	name = strings.TrimSpace(name)
	if name == "" {
		return result{}, bad("Falta ?name= con el proyecto a eliminar")
	}
	if strings.EqualFold(name, core.Inbox) {
		return result{}, bad("El proyecto Inbox no se puede eliminar")
	}
	canon, n, found, err := d.deleteProject(name)
	if err != nil {
		return result{}, internal(err)
	}
	if !found {
		return result{}, &APIError{404, fmt.Sprintf("No existe el proyecto «%s»", name)}
	}
	return ok(map[string]any{"deleted": canon, "tasks": n})
}

func (d *DB) labels() (result, *APIError) {
	tasks, err := d.all()
	if err != nil {
		return result{}, internal(err)
	}
	count := map[string]int{}
	var names []string
	for _, t := range tasks {
		if t.Done {
			continue
		}
		for _, l := range t.Labels {
			if _, seen := count[l]; !seen {
				names = append(names, l)
			}
			count[l]++
		}
	}
	sortStrings(names)
	out := []map[string]any{}
	for _, n := range names {
		out = append(out, map[string]any{"name": n, "open": count[n]})
	}
	return ok(out)
}

func (d *DB) export() (result, *APIError) {
	tasks, err := d.all()
	if err != nil {
		return result{}, internal(err)
	}
	ps, err := d.projects()
	if err != nil {
		return result{}, internal(err)
	}
	out := make([]core.TaskDTO, len(tasks))
	for i, t := range tasks {
		out[i] = core.ToDTO(t)
	}
	return ok(map[string]any{"projects": ps, "tasks": out})
}

func (d *DB) importAll(raw []byte) (result, *APIError) {
	var m map[string]json.RawMessage
	if e := decode(raw, &m, "Se esperaba {\"tasks\": […]}"); e != nil {
		return result{}, e
	}
	var projects []string
	var tasks []core.TaskDTO
	for k, v := range m {
		switch k {
		case "projects", "proyectos":
			_ = json.Unmarshal(v, &projects)
		case "tasks", "tareas":
			if err := json.Unmarshal(v, &tasks); err != nil {
				return result{}, bad("Se esperaba {\"tasks\": […]}")
			}
		}
	}
	for _, p := range projects {
		if err := d.addProject(p); err != nil {
			return result{}, internal(err)
		}
	}
	n := 0
	for i := range tasks {
		t := core.FromDTO(&tasks[i])
		if t.ID == nil {
			id, err := d.uniqueID()
			if err != nil {
				return result{}, internal(err)
			}
			t.ID = &id
		}
		if t.Project == "" {
			t.Project = core.Inbox
		}
		if err := d.addProject(t.Project); err != nil {
			return result{}, internal(err)
		}
		if err := d.save(t); err != nil {
			return result{}, internal(err)
		}
		n++
	}
	return ok(map[string]int{"imported": n})
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
