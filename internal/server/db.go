// Package server es el servidor autoalojado de tasktui: API JSON, página web y MCP,
// con SQLite. Es la misma API que expone el servidor de referencia.
package server

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite" // SQLite en Go puro (sin cgo)

	"github.com/alfonsoHR98/tasktui/internal/core"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB es la base SQLite de tareas. Es segura para uso concurrente.
type DB struct {
	mu sync.Mutex
	db *sql.DB
}

func nowMS() int64 { return time.Now().UnixMilli() }

// Open abre (o crea) la base y aplica las migraciones pendientes.
// `PRAGMA user_version` guarda cuántas se aplicaron.
func Open(path string) (*DB, error) {
	dsn := path
	if path == ":memory:" {
		dsn = "file::memory:?cache=shared&mode=memory"
	}
	sdb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	sdb.SetMaxOpenConns(1)
	for _, p := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON"} {
		if _, err := sdb.Exec(p); err != nil {
			return nil, err
		}
	}
	var applied int
	if err := sdb.QueryRow("PRAGMA user_version").Scan(&applied); err != nil {
		return nil, err
	}
	files, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, f.Name())
	}
	sort.Strings(names)
	for i := applied; i < len(names); i++ {
		sqlText, err := migrationsFS.ReadFile("migrations/" + names[i])
		if err != nil {
			return nil, err
		}
		tx, err := sdb.Begin()
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(string(sqlText)); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("migración %s: %w", names[i], err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}
	return &DB{db: sdb}, nil
}

// Close cierra la base.
func (d *DB) Close() error { return d.db.Close() }

const cols = "id, text, done, due, time, deadline, done_date, priority, labels, recurrence, project, section, description, notes"

type scanner interface{ Scan(...any) error }

func scanTask(r scanner) (*core.Task, error) {
	var (
		id, text, project, labels, description, notes string
		done, priority                                int
		due, tm, deadline, doneDate, rec, section     sql.NullString
	)
	if err := r.Scan(&id, &text, &done, &due, &tm, &deadline, &doneDate, &priority, &labels, &rec, &project, &section, &description, &notes); err != nil {
		return nil, err
	}
	date := func(s sql.NullString) *core.Date {
		if !s.Valid {
			return nil
		}
		if d, ok := core.ParseDate(s.String); ok {
			return d.Ptr()
		}
		return nil
	}
	dto := core.TaskDTO{
		ID: &id, Text: text, Done: done != 0, Due: date(due), Deadline: date(deadline), DoneDate: date(doneDate),
		Priority: priority, Project: project, Description: description,
	}
	if tm.Valid {
		if c, ok := core.ParseClock(tm.String); ok {
			dto.Time = c.Ptr()
		}
	}
	if rec.Valid {
		dto.RecurrenceRule = &rec.String
	}
	if section.Valid {
		dto.Section = &section.String
	}
	_ = json.Unmarshal([]byte(labels), &dto.Labels)
	_ = json.Unmarshal([]byte(notes), &dto.Notes)
	return core.FromDTO(&dto), nil
}

// All devuelve todas las tareas, en orden (proyecto, posición).
func (d *DB) All() ([]*core.Task, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.all()
}

func (d *DB) all() ([]*core.Task, error) {
	rows, err := d.db.Query("SELECT " + cols + " FROM tasks ORDER BY project COLLATE NOCASE, position")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*core.Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Get devuelve una tarea, o nil si no existe.
func (d *DB) Get(id string) (*core.Task, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.get(id)
}

func (d *DB) get(id string) (*core.Task, error) {
	t, err := scanTask(d.db.QueryRow("SELECT "+cols+" FROM tasks WHERE id = ?1", id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

// Exists dice si hay una tarea con ese id.
func (d *DB) Exists(id string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.exists(id)
}

func (d *DB) exists(id string) (bool, error) {
	var x int
	err := d.db.QueryRow("SELECT 1 FROM tasks WHERE id = ?1", id).Scan(&x)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// Projects devuelve los proyectos (los explícitos y los que tienen tareas): Inbox primero.
func (d *DB) Projects() ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.projects()
}

func (d *DB) projects() ([]string, error) {
	rows, err := d.db.Query("SELECT name FROM projects UNION SELECT DISTINCT project FROM tasks")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ps []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		ps = append(ps, n)
	}
	sort.SliceStable(ps, func(i, j int) bool {
		a, b := ps[i] != core.Inbox, ps[j] != core.Inbox
		if a != b {
			return !a
		}
		return strings.ToLower(ps[i]) < strings.ToLower(ps[j])
	})
	out := ps[:0]
	for _, p := range ps {
		if n := len(out); n > 0 && strings.EqualFold(out[n-1], p) {
			continue
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// AddProject crea el proyecto si no existe.
func (d *DB) AddProject(name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.addProject(name)
}

func (d *DB) addProject(name string) error {
	_, err := d.db.Exec("INSERT OR IGNORE INTO projects (name, created_at) VALUES (?1, ?2)", name, nowMS())
	return err
}

// canonicalProject devuelve el nombre del proyecto tal como existe (sin distinguir
// mayúsculas), o el dado.
func (d *DB) canonicalProject(name string) (string, error) {
	ps, err := d.projects()
	if err != nil {
		return "", err
	}
	for _, p := range ps {
		if strings.EqualFold(p, name) {
			return p, nil
		}
	}
	return name, nil
}

func (d *DB) nextPosition(project string) (float64, error) {
	var m sql.NullFloat64
	if err := d.db.QueryRow("SELECT MAX(position) FROM tasks WHERE project = ?1", project).Scan(&m); err != nil {
		return 0, err
	}
	return m.Float64 + 1, nil
}

// save inserta o reemplaza la tarea. Al final de su proyecto si es nueva o cambió
// de proyecto; si no, conserva su posición.
func (d *DB) save(t *core.Task) error {
	id := ""
	if t.ID != nil {
		id = *t.ID
	}
	var curProject string
	var curPos float64
	var curCreated int64
	err := d.db.QueryRow("SELECT project, position, created_at FROM tasks WHERE id = ?1", id).Scan(&curProject, &curPos, &curCreated)
	now := nowMS()
	var position float64
	var created int64
	switch {
	case err == nil && curProject == t.Project:
		position, created = curPos, curCreated
	case err == nil:
		position, created = 0, curCreated
		if position, err = d.nextPosition(t.Project); err != nil {
			return err
		}
	case err == sql.ErrNoRows:
		created = now
		if position, err = d.nextPosition(t.Project); err != nil {
			return err
		}
	default:
		return err
	}
	dto := core.ToDTO(t)
	labels, _ := json.Marshal(dto.Labels)
	notes, _ := json.Marshal(dto.Notes)
	var due, doneDate, deadline, tm any
	if dto.Due != nil {
		due = dto.Due.String()
	}
	if dto.DoneDate != nil {
		doneDate = dto.DoneDate.String()
	}
	if dto.Deadline != nil {
		deadline = dto.Deadline.String()
	}
	if dto.Time != nil {
		tm = dto.Time.String()
	}
	var rec, section any
	if dto.RecurrenceRule != nil {
		rec = *dto.RecurrenceRule
	}
	if dto.Section != nil {
		section = *dto.Section
	}
	done := 0
	if dto.Done {
		done = 1
	}
	_, err = d.db.Exec(`INSERT OR REPLACE INTO tasks
	   (id, text, done, due, done_date, priority, labels, recurrence,
	    project, section, notes, position, created_at, updated_at,
	    description, time, deadline)
	 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17)`,
		id, dto.Text, done, due, doneDate, dto.Priority, string(labels), rec,
		dto.Project, section, string(notes), position, created, now,
		dto.Description, tm, deadline)
	return err
}

// DeleteProject borra el proyecto y todas sus tareas. Devuelve el nombre canónico y
// cuántas tareas se borraron; ok=false si no existe.
func (d *DB) DeleteProject(name string) (string, int, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.deleteProject(name)
}

func (d *DB) deleteProject(name string) (string, int, bool, error) {
	canon, err := d.canonicalProject(name)
	if err != nil {
		return "", 0, false, err
	}
	tx, err := d.db.Begin()
	if err != nil {
		return "", 0, false, err
	}
	defer tx.Rollback()
	rt, err := tx.Exec("DELETE FROM tasks WHERE project = ?1 COLLATE NOCASE", canon)
	if err != nil {
		return "", 0, false, err
	}
	rp, err := tx.Exec("DELETE FROM projects WHERE name = ?1 COLLATE NOCASE", canon)
	if err != nil {
		return "", 0, false, err
	}
	nt, _ := rt.RowsAffected()
	np, _ := rp.RowsAffected()
	if err := tx.Commit(); err != nil {
		return "", 0, false, err
	}
	return canon, int(nt), nt > 0 || np > 0, nil
}

// Delete borra la tarea; false si no existía.
func (d *DB) Delete(id string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	r, err := d.db.Exec("DELETE FROM tasks WHERE id = ?1", id)
	if err != nil {
		return false, err
	}
	n, _ := r.RowsAffected()
	return n > 0, nil
}
