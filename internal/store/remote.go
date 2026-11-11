package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alfonsoHR98/tasktui/internal/core"
)

// Export es lo que devuelve GET /api/export.
type Export struct {
	Projects []string       `json:"projects"`
	Tasks    []core.TaskDTO `json:"tasks"`
}

// NetError: no se pudo hablar con el servidor (red, DNS, tiempo agotado). Solo en
// este caso se usa la copia en caché; un token inválido u otro error del servidor
// se informa tal cual.
type NetError struct{ Msg string }

func (e *NetError) Error() string { return e.Msg }

// Client es el cliente HTTP del servidor.
type Client struct {
	base, token string
	http        *http.Client
}

// NewClient crea el cliente (15 s de tiempo máximo por llamada).
func NewClient(s Server) *Client {
	return &Client{base: strings.TrimRight(s.URL, "/"), token: s.Token, http: &http.Client{Timeout: 15 * time.Second}}
}

// URL es la dirección base del servidor.
func (c *Client) URL() string { return c.base }

func (c *Client) do(method, path string, body any) (map[string]any, []byte, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+"/api/"+path, rd)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, &NetError{fmt.Sprintf("Sin conexión con %s: %v", c.base, err)}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var v map[string]any
	_ = json.Unmarshal(raw, &v)
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return v, raw, nil
	case resp.StatusCode == 401:
		return nil, nil, errors.New("El servidor rechazó el token. Vuelve a conectar con `tasktui server connect`.")
	}
	if msg, ok := v["error"].(string); ok {
		return nil, nil, errors.New(msg)
	}
	return nil, nil, fmt.Errorf("Error %d del servidor", resp.StatusCode)
}

// Get hace GET /api/<path>.
func (c *Client) Get(path string) (map[string]any, error) {
	v, _, err := c.do("GET", path, nil)
	return v, err
}

// Post hace POST /api/<path> (body puede ser nil).
func (c *Client) Post(path string, body any) (map[string]any, error) {
	v, _, err := c.do("POST", path, body)
	return v, err
}

// Put hace PUT /api/<path>.
func (c *Client) Put(path string, body any) error {
	_, _, err := c.do("PUT", path, body)
	return err
}

// Delete hace DELETE /api/<path>.
func (c *Client) Delete(path string) error {
	_, _, err := c.do("DELETE", path, nil)
	return err
}

// Export descarga todas las tareas y proyectos.
func (c *Client) Export() (Export, error) {
	_, raw, err := c.do("GET", "export", nil)
	if err != nil {
		return Export{}, err
	}
	var ex Export
	err = json.Unmarshal(raw, &ex)
	return ex, err
}

// Import sube tareas conservando sus ids (no borra nada en el servidor).
func (c *Client) Import(projects []string, tasks []core.TaskDTO) (int, error) {
	v, err := c.Post("import", map[string]any{"projects": projects, "tasks": tasks})
	if err != nil {
		return 0, err
	}
	n, _ := v["imported"].(float64)
	return int(n), nil
}

// RemoteStore es el almacén remoto: el servidor de tasktui. Cada cambio va directo
// al servidor y después se vuelve a leer todo (son pocas tareas). La última lectura
// se guarda en caché: sin internet se pueden ver las tareas, pero los cambios se
// rechazan con un mensaje claro en vez de perderse.
type RemoteStore struct {
	client   *Client
	tasks    []*core.Task
	projects []string
	cache    string
	// Offline: la última lectura falló y se muestran datos de la caché.
	Offline bool
}

// cachePath: una caché por servidor.
func cachePath(u string) string {
	host := u
	if i := strings.LastIndex(u, "://"); i >= 0 {
		host = u[i+3:]
	}
	var sb strings.Builder
	for _, r := range host {
		if r < 128 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-') {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "tasktui", "remoto-"+strings.TrimRight(sb.String(), "_")+".json")
}

// OpenRemote abre el servidor. Si no responde pero hay caché, abre sin conexión.
func OpenRemote(s Server) (*RemoteStore, error) {
	r := &RemoteStore{client: NewClient(s), cache: cachePath(s.URL)}
	if err := r.Reload(); err != nil {
		var ne *NetError
		if !errors.As(err, &ne) {
			return nil, err
		}
		b, rerr := os.ReadFile(r.cache)
		var ex Export
		if rerr != nil || json.Unmarshal(b, &ex) != nil {
			return nil, err
		}
		r.set(ex)
		r.Offline = true
	}
	return r, nil
}

// Client devuelve el cliente HTTP (para exportar/importar).
func (r *RemoteStore) Client() *Client { return r.client }

func (r *RemoteStore) Notice() string {
	if r.Offline {
		return fmt.Sprintf("Sin conexión con %s: mostrando la última copia (solo lectura)", r.client.URL())
	}
	return ""
}

func (r *RemoteStore) Dir() string { return "" }

func (r *RemoteStore) set(ex Export) {
	r.tasks = make([]*core.Task, len(ex.Tasks))
	for i := range ex.Tasks {
		r.tasks[i] = core.FromDTO(&ex.Tasks[i])
	}
	r.projects = ex.Projects
}

func (r *RemoteStore) Reload() error {
	ex, err := r.client.Export()
	if err != nil {
		return err
	}
	if b, err := json.Marshal(ex); err == nil {
		_ = os.MkdirAll(filepath.Dir(r.cache), 0o755)
		_ = os.WriteFile(r.cache, b, 0o644)
	}
	r.set(ex)
	r.Offline = false
	return nil
}

func (r *RemoteStore) Tasks() []Entry {
	out := make([]Entry, len(r.tasks))
	for i, t := range r.tasks {
		out[i] = Entry{Pos{0, i}, t}
	}
	return out
}

func (r *RemoteStore) TaskAt(p Pos) *core.Task {
	if p.B < 0 || p.B >= len(r.tasks) {
		return nil
	}
	return r.tasks[p.B]
}

func (r *RemoteStore) Projects() []string { return append([]string(nil), r.projects...) }

func (r *RemoteStore) find(id string) int {
	for i, t := range r.tasks {
		if t.ID != nil && *t.ID == id {
			return i
		}
	}
	return -1
}

func (r *RemoteStore) Find(id string) (Pos, bool) {
	i := r.find(id)
	return Pos{0, i}, i >= 0
}

func (r *RemoteStore) getOrErr(id string) (*core.Task, error) {
	i := r.find(id)
	if i < 0 {
		return nil, fmt.Errorf("No existe la tarea %s", id)
	}
	return r.tasks[i].Clone(), nil
}

func (r *RemoteStore) put(t *core.Task) error {
	if t.ID == nil {
		return errors.New("La tarea no tiene id")
	}
	return r.client.Put("tasks/"+url.PathEscape(*t.ID), core.ToDTO(t))
}

func (r *RemoteStore) Add(t *core.Task, project string) (string, error) {
	if err := r.Reload(); err != nil {
		return "", err
	}
	ids := map[string]bool{}
	for _, x := range r.tasks {
		if x.ID != nil {
			ids[*x.ID] = true
		}
	}
	id := GenID(ids)
	t.ID = &id
	t.Project = project
	if err := r.put(t); err != nil {
		return "", err
	}
	return id, r.Reload()
}

func (r *RemoteStore) Update(id string, f func(*core.Task)) error {
	if err := r.Reload(); err != nil {
		return err
	}
	t, err := r.getOrErr(id)
	if err != nil {
		return err
	}
	f(t)
	if err := r.put(t); err != nil {
		return err
	}
	return r.Reload()
}

func (r *RemoteStore) Toggle(id string) (bool, error) {
	v, err := r.client.Post("tasks/"+url.PathEscape(id)+"/done", nil)
	if err != nil {
		return false, err
	}
	done, _ := v["completed"].(bool)
	return done, r.Reload()
}

func (r *RemoteStore) Delete(id string) error {
	if err := r.client.Delete("tasks/" + url.PathEscape(id)); err != nil {
		return err
	}
	return r.Reload()
}

func (r *RemoteStore) DeleteProject(name string) (int, error) {
	if strings.EqualFold(name, core.Inbox) {
		return 0, errors.New("El proyecto Inbox no se puede eliminar")
	}
	v, _, err := r.client.do("DELETE", "projects?name="+url.QueryEscape(name), nil)
	if err != nil {
		return 0, err
	}
	n, _ := v["tasks"].(float64)
	return int(n), r.Reload()
}

func (r *RemoteStore) AddNote(id, text string) error {
	if strings.TrimSpace(text) == "" {
		return errors.New("La nota está vacía")
	}
	if _, err := r.client.Post("tasks/"+url.PathEscape(id)+"/notes", map[string]string{"text": text}); err != nil {
		return err
	}
	return r.Reload()
}

func (r *RemoteStore) Snapshot() (Snapshot, error) {
	out := make([]core.TaskDTO, len(r.tasks))
	for i, t := range r.tasks {
		out[i] = core.ToDTO(t)
	}
	return out, nil
}

// Restore deja el servidor como estaba en la copia: repone lo cambiado o borrado y
// quita lo que se creó después.
func (r *RemoteStore) Restore(sn Snapshot) error {
	snap, ok := sn.([]core.TaskDTO)
	if !ok {
		return errors.New("La copia para deshacer es de otro almacén")
	}
	if err := r.Reload(); err != nil {
		return err
	}
	keep := map[string]bool{}
	for i := range snap {
		d := snap[i]
		if d.ID != nil {
			keep[*d.ID] = true
		}
		var cur *core.TaskDTO
		if d.ID != nil {
			if j := r.find(*d.ID); j >= 0 {
				c := core.ToDTO(r.tasks[j])
				cur = &c
			}
		}
		a, _ := json.Marshal(cur)
		b, _ := json.Marshal(&d)
		if cur == nil || !bytes.Equal(a, b) {
			if err := r.put(core.FromDTO(&d)); err != nil {
				return err
			}
		}
	}
	for _, t := range r.tasks {
		if t.ID != nil && !keep[*t.ID] {
			if err := r.client.Delete("tasks/" + url.PathEscape(*t.ID)); err != nil {
				return err
			}
		}
	}
	return r.Reload()
}
