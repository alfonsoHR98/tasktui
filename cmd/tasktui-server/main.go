// tasktui-server es el servidor autoalojado de tasktui: API, página web y MCP con SQLite.
//
// Variables de entorno:
//
//	API_TOKEN          (obligatoria) token Bearer de la API, mínimo 16 caracteres.
//	MCP_SECRET         (opcional) activa el MCP en POST /mcp/<MCP_SECRET>.
//	DATABASE_PATH      archivo SQLite (por defecto tasktui.db; en Docker /data/tasktui.db).
//	TZ_OFFSET_MINUTES  zona para «hoy» (por defecto 0; México = -360).
//	PORT, BIND         dónde escucha (por defecto 8787 y 0.0.0.0).
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/alfonsoHR98/tasktui/internal/core"
	"github.com/alfonsoHR98/tasktui/internal/server"
)

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}

// healthcheck consulta /api/health del propio servidor (lo usa HEALTHCHECK de Docker).
func healthcheck() {
	resp, err := http.Get("http://127.0.0.1:" + getenv("PORT", "8787") + "/api/health")
	if err != nil || resp.StatusCode != 200 {
		os.Exit(1)
	}
	os.Exit(0)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		healthcheck()
	}
	token := os.Getenv("API_TOKEN")
	if len(token) < 16 {
		fail("Define API_TOKEN con al menos 16 caracteres (por ejemplo: openssl rand -hex 24).")
	}
	secret := os.Getenv("MCP_SECRET")
	if secret != "" && len(secret) < 16 {
		fail("MCP_SECRET debe tener al menos 16 caracteres.")
	}
	path := getenv("DATABASE_PATH", "tasktui.db")
	db, err := server.Open(path)
	if err != nil {
		fail("No se pudo abrir la base «%s»: %v", path, err)
	}
	defer db.Close()
	tz, _ := strconv.Atoi(os.Getenv("TZ_OFFSET_MINUTES"))

	addr := getenv("BIND", "0.0.0.0") + ":" + getenv("PORT", "8787")
	srv := &http.Server{
		Addr:              addr,
		Handler:           server.New(db, server.Config{APIToken: token, MCPSecret: secret, TZOffsetMinutes: tz}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	fmt.Printf("tasktui-server %s escuchando en http://%s (base: %s)\n", core.Version, addr, path)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fail("Error del servidor: %v", err)
	}
}
