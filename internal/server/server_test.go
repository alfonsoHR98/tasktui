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
