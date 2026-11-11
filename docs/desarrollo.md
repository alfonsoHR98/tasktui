# Desarrollo

Go 1.27+. No hace falta cgo ni herramientas de Xcode (SQLite en Go puro).

```sh
make test        # go vet + go test ./...
make install     # instala tasktui en ~/go/bin (o donde diga GOBIN)
make server      # compila ./tasktui-server
make docker      # construye la imagen
```

## Estructura

| Carpeta | Qué hace |
|---|---|
| `internal/core` | El núcleo, sin reloj, archivos ni red (quien llama pasa «hoy») |
| `internal/server` | API JSON, MCP y la página web (incrustada), con SQLite |
| `internal/store` | Almacén de la TUI/CLI: archivos `.md` (`files.go`), servidor (`remote.go`) y `config.go` |
| `internal/tui` | La interfaz (Bubble Tea) |
| `cmd/tasktui` | La CLI (cobra); sin argumentos abre la TUI |
| `cmd/tasktui-server` | El servidor |

### `internal/core`

| Archivo | Qué hace |
|---|---|
| `model.go` | `Task`, `Recurrence`, completar tareas que se repiten |
| `quick.go` | captura rápida en español |
| `dates.go`, `date.go` | fechas en español («hoy», «vie», «15 nov») y aritmética segura |
| `markdown.go` | leer/escribir una línea en formato Obsidian Tasks |
| `views.go` | vistas Inbox/Hoy/Próximos/Todas/Proyecto/Etiqueta |
| `dto.go` | la tarea en JSON: contrato de `--json` y de la API |
| `mcp.go` | herramientas y protocolo del servidor MCP (sin E/S) |

La captura rápida se prueba con miles de combinaciones raras de texto y números
enormes para que ninguna entrada pueda tumbar el programa
(`TestSinPanico` en `internal/core/core_test.go`).

Las pruebas del servidor levantan un servidor HTTP real con una base temporal;
las de la TUI mandan teclas al modelo y revisan la pantalla.
