# Desarrollo

Workspace de Cargo con cuatro crates (el Worker solo compila a wasm y `cargo test` no lo incluye):

**`crates/tasktui-core`**: el núcleo. No usa reloj, archivos ni red (quien
llama pasa la fecha de hoy), así que compila igual a nativo que a wasm para el
servidor.

| Archivo | Qué hace |
|---|---|
| `model.rs` | `Task`, `Recurrence` |
| `quick.rs` | captura rápida en español |
| `dates.rs` | fechas en español («hoy», «vie», «15 nov») |
| `markdown.rs` | leer/escribir una línea en formato Obsidian Tasks |
| `views.rs` | vistas Inbox/Hoy/Próximos/Todas/Proyecto/Etiqueta |
| `dto.rs` | la tarea en JSON: contrato de `--json` y de la API |
| `mcp.rs` | herramientas y protocolo del servidor MCP (sin E/S) |

**`crates/tasktui-worker`**: el servidor (API sobre Cloudflare Workers + D1, ver arriba)
y la página web en `web/` (`app.js` cliente de la API, `sw.js` para abrir sin conexión).

**`crates/tasktui-server`**: el mismo servidor como binario nativo con SQLite (axum + rusqlite), para Docker. Incrusta la web y comparte con el Worker el MCP (`tasktui_core::mcp`). Las rutas están duplicadas respecto al Worker (`api.rs` ↔ `lib.rs`): si cambias una, cambia la otra.

**`crates/tasktui`**: la TUI y la CLI.

| Archivo | Qué hace |
|---|---|
| `store.rs` | elige el modo (local o remoto) con una sola interfaz |
| `files.rs` | modo local: carpeta de `.md` (cargar, añadir, editar, mover, completar, deshacer, notas) y exportar |
| `remote.rs` | modo remoto: cliente HTTP del servidor, caché sin conexión y deshacer |
| `tui.rs` | interfaz ratatui |
| `reminders.rs` | espejo con Recordatorios (EventKit, solo macOS) y agente de launchd |
| `config.rs` | `~/.config/tasktui/config.toml` |
| `main.rs` | CLI (clap) |

```sh
cargo test                                              # todas las pruebas
TASKTUI_SERVER=http://localhost:8787 TASKTUI_TOKEN=… cargo test -p tasktui remoto   # integración con un servidor
cargo build -p tasktui-core --target wasm32-unknown-unknown   # el núcleo compila a wasm
```


Las pruebas del servidor Docker (`cargo test -p tasktui-server`) cubren el ciclo
de una tarea, las repeticiones y los errores. El Worker se prueba con
`wrangler dev` (ver [cloudflare.md](cloudflare.md)).
