package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	db, err := Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	srv := httptest.NewServer(New(db, Config{APIToken: "pruebapruebaprueba1", MCPSecret: "secretosecretosecreto", TZOffsetMinutes: -360}))
	t.Cleanup(srv.Close)
	return srv
}

func call(t *testing.T, srv *httptest.Server, method, path, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer pruebapruebaprueba1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var v map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&v)
	return resp.StatusCode, v
}

func TestCicloCompleto(t *testing.T) {
	srv := newTestServer(t)
	s, task := call(t, srv, "POST", "/api/tasks?today=2026-11-10", `{"text":"Pagar luz mañana p1 +Finanzas #casa"}`)
	if s != 201 || task["due"] != "2026-11-11" || task["project"] != "Finanzas" {
		t.Fatalf("%d %v", s, task)
	}
	id := task["id"].(string)
	_, l := call(t, srv, "GET", "/api/tasks?view=%2BFinanzas&today=2026-11-10", "")
	groups := l["groups"].([]any)
	if len(groups) != 1 || groups[0].(map[string]any)["tasks"].([]any)[0].(map[string]any)["id"] != id {
		t.Fatalf("lista: %v", l)
	}
	_, d := call(t, srv, "POST", "/api/tasks/"+id+"/done?today=2026-11-10", "")
	if d["completed"] != true {
		t.Fatalf("%v", d)
	}
	if s, _ := call(t, srv, "DELETE", "/api/tasks/"+id, ""); s != 200 {
		t.Errorf("delete %d", s)
	}
	if s, _ := call(t, srv, "GET", "/api/tasks/"+id, ""); s != 404 {
		t.Errorf("get %d", s)
	}
}

func TestRepeticionAvanza(t *testing.T) {
	srv := newTestServer(t)
	_, task := call(t, srv, "POST", "/api/tasks?today=2026-11-10", `{"text":"Regar cada lunes"}`)
	_, d := call(t, srv, "POST", "/api/tasks/"+task["id"].(string)+"/done?today=2026-11-10", "")
	next, occ := d["task"].(map[string]any), d["occurrence"].(map[string]any)
	if next["done"] != false || next["due"] != "2026-11-23" || occ["done"] != true {
		t.Errorf("next=%v occ=%v", next, occ)
	}
}

func TestErroresYAuth(t *testing.T) {
	srv := newTestServer(t)
	if s, e := call(t, srv, "POST", "/api/tasks", `{"text":"  "}`); s != 400 || e["error"] != "La tarea necesita texto" {
		t.Errorf("%d %v", s, e)
	}
	if s, _ := call(t, srv, "GET", "/api/tasks?view=nada", ""); s != 400 {
		t.Errorf("vista: %d", s)
	}
	resp, _ := http.Get(srv.URL + "/api/tasks")
	if resp.StatusCode != 401 {
		t.Errorf("sin token: %d", resp.StatusCode)
	}
	resp, _ = http.Post(srv.URL+"/mcp/otro", "application/json", strings.NewReader("{}"))
	if resp.StatusCode != 404 {
		t.Errorf("mcp mal: %d", resp.StatusCode)
	}
	resp, _ = http.Get(srv.URL + "/")
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		t.Errorf("web: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}

func TestMCP(t *testing.T) {
	srv := newTestServer(t)
	post := func(body string) (int, string) {
		resp, err := http.Post(srv.URL+"/mcp/secretosecretosecreto", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if s, b := post(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`); s != 200 || !strings.Contains(b, "list_tasks") {
		t.Errorf("tools/list: %d %s", s, b)
	}
	if s, b := post(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"add_task","arguments":{"text":"Hola mañana"}}}`); s != 200 || strings.Contains(b, "isError") {
		t.Errorf("add_task: %d %s", s, b)
	}
	if s, b := post(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_task","arguments":{"id":"nope"}}}`); s != 200 || !strings.Contains(b, "isError") {
		t.Errorf("get_task error: %d %s", s, b)
	}
	if s, _ := post(`{"jsonrpc":"2.0","method":"notifications/initialized"}`); s != 202 {
		t.Errorf("notificación: %d", s)
	}
}

func TestEliminarProyecto(t *testing.T) {
	srv := newTestServer(t)
	call(t, srv, "POST", "/api/tasks", `{"text":"Pagar luz","project":"Finanzas"}`)
	call(t, srv, "POST", "/api/tasks", `{"text":"Otra","project":"Casa/Cocina"}`)
	if s, _ := call(t, srv, "DELETE", "/api/projects?name=Inbox", ""); s != 400 {
		t.Errorf("Inbox: %d", s)
	}
	if s, _ := call(t, srv, "DELETE", "/api/projects", ""); s != 400 {
		t.Errorf("sin nombre: %d", s)
	}
	if s, _ := call(t, srv, "DELETE", "/api/projects?name=nada", ""); s != 404 {
		t.Errorf("inexistente: %d", s)
	}
	s, v := call(t, srv, "DELETE", "/api/projects?name=finanzas", "")
	if s != 200 || v["tasks"] != float64(1) || v["deleted"] != "Finanzas" {
		t.Errorf("borrar: %d %v", s, v)
	}
	if s, _ := call(t, srv, "DELETE", "/api/projects?name=Casa%2FCocina", ""); s != 200 {
		t.Errorf("con barra: %d", s)
	}
	_, list := call(t, srv, "GET", "/api/tasks?view=all", "")
	if b, _ := json.Marshal(list); strings.Contains(string(b), "Pagar luz") || strings.Contains(string(b), "Otra") {
		t.Errorf("las tareas debían borrarse: %s", b)
	}
}

func projectNames(t *testing.T, srv *httptest.Server) []string {
	t.Helper()
	req, _ := http.NewRequest("GET", srv.URL+"/api/projects", nil)
	req.Header.Set("Authorization", "Bearer pruebapruebaprueba1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var ps []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&ps)
	var out []string
	for _, p := range ps {
		out = append(out, p["name"].(string))
	}
	return out
}

// Lo que duplicaba proyectos: «+Niuko Consultores» en el texto, comillas y nombres
// sin acentos. Ninguno debe crear un proyecto nuevo.
func TestSinProyectosDuplicados(t *testing.T) {
	srv := newTestServer(t)
	if s, _ := call(t, srv, "POST", "/api/projects", `{"name":"Niuko Consultores"}`); s != 201 {
		t.Fatalf("crear %d", s)
	}
	call(t, srv, "POST", "/api/projects", `{"name":"Ramírez Medellín"}`)
	for _, body := range []string{
		`{"text":"VPN +Niuko Consultores"}`,
		`{"text":"VPN +\"Niuko Consultores\""}`,
		`{"text":"VPN +niuko_consultores"}`,
		`{"text":"VPN","project":"niuko consultores"}`,
		`{"text":"Checador","project":"Ramirez Medellin"}`,
	} {
		s, task := call(t, srv, "POST", "/api/tasks", body)
		if s != 201 || (task["project"] != "Niuko Consultores" && task["project"] != "Ramírez Medellín") {
			t.Errorf("%s → %d %v", body, s, task)
		}
		if strings.Contains(task["text"].(string), "Consultores") {
			t.Errorf("%s → el nombre quedó en el texto: %v", body, task["text"])
		}
	}
	// Crear uno equivalente devuelve el existente.
	if s, p := call(t, srv, "POST", "/api/projects", `{"name":"ramirez medellin"}`); s != 200 || p["name"] != "Ramírez Medellín" || p["existed"] != true {
		t.Errorf("equivalente: %d %v", s, p)
	}
	if ps := projectNames(t, srv); len(ps) != 3 {
		t.Errorf("proyectos: %v", ps)
	}
}

// En modo estricto (el del MCP) un proyecto desconocido es un error con sugerencias.
func TestProyectoEstricto(t *testing.T) {
	srv := newTestServer(t)
	call(t, srv, "POST", "/api/projects", `{"name":"Niuko Consultores"}`)
	s, e := call(t, srv, "POST", "/api/tasks", `{"text":"VPN +Niuko","strict_project":true}`)
	if s != 400 || !strings.Contains(e["error"].(string), "¿Quisiste decir «Niuko Consultores»?") {
		t.Fatalf("%d %v", s, e)
	}
	s, task := call(t, srv, "POST", "/api/tasks", `{"text":"VPN","project":"Niuko Consultores","strict_project":true}`)
	if s != 201 {
		t.Fatalf("%d %v", s, task)
	}
	id := task["id"].(string)
	if s, _ := call(t, srv, "PATCH", "/api/tasks/"+id, `{"project":"Otro","strict_project":true}`); s != 400 {
		t.Errorf("mover a uno inexistente: %d", s)
	}
	if s, _ := call(t, srv, "PATCH", "/api/tasks/"+id, `{"text":"p2","strict_project":true}`); s != 200 {
		t.Errorf("editar sin tocar el proyecto: %d", s)
	}
	if ps := projectNames(t, srv); len(ps) != 2 {
		t.Errorf("proyectos: %v", ps)
	}
}

// Un proyecto creado de paso por una tarea desaparece al quedarse vacío; uno creado
// a propósito se queda.
func TestProyectoImplicitoNoQuedaVacio(t *testing.T) {
	srv := newTestServer(t)
	call(t, srv, "POST", "/api/projects", `{"name":"Niuko Consultores"}`)
	_, task := call(t, srv, "POST", "/api/tasks", `{"text":"VPN +Niuko"}`)
	if task["project"] != "Niuko" {
		t.Fatalf("%v", task)
	}
	call(t, srv, "PATCH", "/api/tasks/"+task["id"].(string), `{"project":"Niuko Consultores"}`)
	ps := projectNames(t, srv)
	for _, p := range ps {
		if p == "Niuko" {
			t.Errorf("quedó el proyecto vacío: %v", ps)
		}
	}
}
