# Desarrollo

Go 1.27+. No hace falta cgo ni herramientas de Xcode (SQLite en Go puro).

```sh
make test        # go vet + go test ./...
make install     # instala tasktui en ~/go/bin (o donde diga GOBIN)
make server      # compila ./tasktui-server
make docker      # construye la imagen
make demo        # abre la TUI con tareas de ejemplo
make capturas    # regenera docs/img/*.png (necesita Python 3 y Google Chrome)
make site        # sirve la página del proyecto en http://127.0.0.1:8931
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
| `site/` | La página del proyecto (HTML, CSS y JS sin compilación); se publica en GitHub Pages |
| `scripts/` | Utilidades: capturas de la TUI para la documentación |
| `ios/` | App de iPhone, en desarrollo |

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

## Interfaz y colores

Los colores de la TUI están en `internal/tui/style.go` (`applyTheme`): Aurora
en color verdadero y `ansi` con los 16 colores de la terminal. La página web
(`site/style.css`) usa la misma paleta. Si cambias la interfaz, regenera las
capturas con `make capturas` y súbelas en el mismo commit.

| Color | Hex | Uso |
|---|---|---|
| acento | `#59C98D` | foco, marca de selección, modo NORMAL |
| texto | `#DFEDE5` | texto principal |
| secundario | `#9DBCB1` · `#8FA89F` | títulos sin foco, ayudas |
| tenue | `#6E6787` | datos secundarios |
| marco | `#3A3A44` | marcos sin foco y reglas |
| selección | `#2A4A3E` | fondo de la fila elegida |
| rojo · amarillo · azul | `#E0607F` · `#C5D98A` · `#9AA8DC` | p1/vencido · p2 · p3/grupos |
| malva · cian | `#B06BC4` · `#6FD9C4` | etiquetas · próximos y repeticiones |

## Publicar una versión

1. Actualiza `Version` en `internal/core/mcp.go` y `CHANGELOG.md`.
2. `git tag v0.4.0 && git push --tags`.
3. El flujo `release` compila los binarios con GoReleaser, crea la versión en
   GitHub y publica la imagen en `ghcr.io/alfonsohr98/tasktui`.

La página se publica sola con cada cambio en `site/` (flujo `pages`). La
primera vez hay que activar Pages en el repositorio: Settings → Pages →
Source: **GitHub Actions**.
