package server

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/alfonsoHR98/tasktui/internal/core"
)

//go:embed web
var webFS embed.FS

// Config son los ajustes del servidor.
type Config struct {
	// APIToken es el token Bearer de la API (obligatorio).
	APIToken string
	// MCPSecret activa el MCP en POST /mcp/<secreto> (vacío = apagado).
	MCPSecret string
	// TZOffsetMinutes es la zona para «hoy» cuando el cliente no manda ?today=.
	TZOffsetMinutes int
}

// Server es el manejador HTTP: API, MCP y página web.
type Server struct {
	db  *DB
	cfg Config
}

// New crea el servidor.
func New(db *DB, cfg Config) *Server { return &Server{db: db, cfg: cfg} }

// sameSecret compara en tiempo constante.
func sameSecret(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// Cabeceras de seguridad de la página (las mismas que web/_headers del Worker).
const csp = "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"

var mimes = map[string]string{
	".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".css": "text/css; charset=utf-8",
	".webmanifest": "application/manifest+json", ".svg": "image/svg+xml", ".png": "image/png",
}

func serveWeb(w http.ResponseWriter, r *http.Request) bool {
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	b, err := webFS.ReadFile("web/" + name)
	if err != nil || strings.Contains(name, "/") {
		return false
	}
	if i := strings.LastIndex(name, "."); i >= 0 {
		w.Header().Set("Content-Type", mimes[name[i:]])
	}
	h := w.Header()
	h.Set("Content-Security-Policy", csp)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	_, _ = w.Write(b)
	return true
}

// ServeHTTP implementa http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "Cuerpo demasiado grande")
		return
	}
	switch {
	case strings.HasPrefix(path, "/mcp/"):
		secret := strings.TrimRight(strings.TrimPrefix(path, "/mcp/"), "/")
		if s.cfg.MCPSecret == "" || !sameSecret(secret, s.cfg.MCPSecret) {
			writeErr(w, 404, "No encontrado")
			return
		}
		if r.Method != http.MethodPost {
			writeErr(w, 405, "Usa POST")
			return
		}
		s.serveMCP(w, body)
	case path == "/api/health":
		writeJSON(w, 200, map[string]any{"ok": true, "version": core.Version})
	case strings.HasPrefix(path, "/api/"):
		token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !found || !sameSecret(strings.TrimSpace(token), s.cfg.APIToken) {
			writeErr(w, 401, "Falta el token o no es válido")
			return
		}
		status, v := s.db.Handle(s.cfg.TZOffsetMinutes, r.Method, r.URL.RequestURI(), body)
		writeJSON(w, status, v)
	default:
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && serveWeb(w, r) {
			return
		}
		writeErr(w, 404, "No encontrado")
	}
}

func (s *Server) callTool(name string, args map[string]any) (string, error) {
	call, err := core.PlanTool(name, args)
	if err != nil {
		return "", err
	}
	var raw []byte
	if call.Body != nil {
		raw, _ = json.Marshal(call.Body)
	}
	status, v := s.db.Handle(s.cfg.TZOffsetMinutes, call.Method, call.Path, raw)
	text, _ := json.MarshalIndent(v, "", "  ")
	if status >= 400 {
		if m, ok := v.(map[string]string); ok {
			return "", &APIError{status, m["error"]}
		}
		return "", &APIError{status, string(text)}
	}
	return string(text), nil
}

func (s *Server) mcpMessage(msg map[string]json.RawMessage) any {
	rpc := core.Dispatch(msg)
	switch {
	case rpc.Notification:
		return nil
	case rpc.Call != nil:
		out, err := s.callTool(rpc.Call.Name, rpc.Call.Args)
		return core.ToolReply(rpc.Call.ID, out, err)
	}
	return rpc.Reply
}

func (s *Server) serveMCP(w http.ResponseWriter, raw []byte) {
	var batch []map[string]json.RawMessage
	var single map[string]json.RawMessage
	var out any
	if json.Unmarshal(raw, &batch) == nil {
		replies := []any{}
		for _, m := range batch {
			if r := s.mcpMessage(m); r != nil {
				replies = append(replies, r)
			}
		}
		if len(replies) > 0 {
			out = replies
		}
	} else if json.Unmarshal(raw, &single) == nil {
		out = s.mcpMessage(single)
	} else {
		writeJSON(w, 400, core.MCPParseError())
		return
	}
	if out == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeJSON(w, 200, out)
}
