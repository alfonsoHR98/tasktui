# tasktui

Gestor de tareas personal para reemplazar a Todoist, **tuyo y en tu propio
servidor**: una TUI/CLI de terminal (Rust + Ratatui), una página web instalable
(PWA), una app nativa de iPhone, y un servidor MCP para que Claude y otras IAs
manejen tus tareas. Captura rápida en lenguaje natural (`Pagar luz mañana
18:00 p1 +Finanzas #casa cada mes`), proyectos, etiquetas, prioridades,
repeticiones, fecha límite, descripción y notas.

```
┌ tasktui ─────────────┐┌ Hoy · 3 ─────────────────────────────────────┐
│ ▣ Inbox           2  ││ Vencidas                                     │
│ ◉ Hoy             3  ││ ○ Renovar dominio  ayer  #work  · Trabajo    │
│ ◷ Próximos        5  ││ jue 5 nov · hoy                              │
│ ≡ Todas          12  ││ ○ Pagar luz  hoy ↻  #casa  · Finanzas        │
│ PROYECTOS            ││ ○ Llamar dentista  hoy  · Inbox              │
│ ▸ Finanzas        1  │├ Detalle ─────────────────────────────────────┤
│ ▸ Trabajo         4  ││ Pagar luz · Finanzas                         │
│ ETIQUETAS            ││ 📅 jue 5 nov · hoy   p1   ↻ cada mes   #casa │
│ # casa            2  ││ id frlyy                                     │
└──────────────────────┘└──────────────────────────────────────────────┘
 a añadir · x completar · e editar · s fecha · d borrar · / buscar · ? ayuda
```

## Cómo funciona

```
 TUI / CLI ─┐
 Web (PWA) ─┼──  HTTPS + token  ──►  Servidor (API + web + MCP)  ──►  Base de datos
 iPhone    ─┤                         · Cloudflare Worker + D1
 Claude    ─┘  (MCP)                  · o contenedor Docker + SQLite
```

Un servidor es la fuente de verdad y todos los clientes hablan la misma API.
Toda la lógica (captura rápida, vistas, repeticiones) vive en `tasktui-core` y
es la misma en la TUI y en el servidor. También puedes usarlo **sin servidor**,
guardando las tareas como Markdown compatible con Obsidian Tasks.

## Empieza

Elige dónde vivirá tu servidor:

| Opción | Ideal si… | Guía |
|---|---|---|
| **Docker + SQLite** | tienes un VPS, Raspberry Pi, NAS o tu Mac siempre encendida | [docs/docker.md](docs/docker.md) |
| **Cloudflare Workers + D1** | quieres algo gratis, sin mantenimiento y siempre disponible | [docs/cloudflare.md](docs/cloudflare.md) |
| **Sin servidor** (archivos `.md`) | solo usas la terminal, sincronizas con iCloud/Obsidian | [docs/modos-y-recordatorios.md](docs/modos-y-recordatorios.md) |

### Docker en tres comandos

```sh
git clone https://github.com/alfonsoHR98/tasktui.git && cd tasktui
cp .env.example .env        # pon un API_TOKEN (openssl rand -hex 24)
docker compose up -d --build
```

Abre `http://127.0.0.1:8787`, entra con tu token y listo. Para usarlo fuera de
casa necesitas HTTPS (la guía explica Caddy, Cloudflare Tunnel y Tailscale).

### Instala la TUI/CLI

Requiere [Rust](https://rustup.rs).

```sh
cargo install --path crates/tasktui
tasktui server connect https://tareas.tu-dominio.com    # pide el token
tasktui                                                  # abre la TUI
tasktui add Pagar luz mañana 18:00 p1 +Finanzas          # o desde la CLI
```

## Qué incluye

- **TUI y CLI** (`tasktui`): atajos estilo vim, deshacer, `--json` en cada comando. → [uso](docs/uso.md)
- **Servidor** con API JSON: Cloudflare o Docker, con la misma API. → [API](docs/api.md)
- **Página web (PWA)**, servida por el mismo servidor, funciona sin conexión en lectura. → [web](docs/web.md)
- **App de iPhone** (SwiftUI) con widget y avisos locales. → [iOS](docs/ios.md)
- **Servidor MCP** para Claude y otras IAs. → [MCP](docs/mcp.md)
- **Recordatorios de Apple** (macOS): espejo en una lista, captura con Siri. → [docs](docs/modos-y-recordatorios.md)
- **Respaldo**: `tasktui export` a `.md`, y `/api/export` en JSON.

## Documentación

| | |
|---|---|
| [docs/docker.md](docs/docker.md) | servidor autoalojado, HTTPS, respaldos, variables |
| [docs/cloudflare.md](docs/cloudflare.md) | servidor en Cloudflare Workers + D1 |
| [docs/mcp.md](docs/mcp.md) | conectar Claude u otra IA |
| [docs/uso.md](docs/uso.md) | captura rápida, atajos de la TUI, CLI |
| [docs/api.md](docs/api.md) | rutas de la API |
| [docs/web.md](docs/web.md) | la página web / PWA |
| [docs/ios.md](docs/ios.md) | la app de iPhone |
| [docs/modos-y-recordatorios.md](docs/modos-y-recordatorios.md) | modo local, formato Markdown, Recordatorios |
| [docs/desarrollo.md](docs/desarrollo.md) | estructura del código y pruebas |

## Seguridad

- El token es la única llave: genera uno largo y no lo subas a ningún repo
  (`.env` y `wrangler.jsonc` están en `.gitignore`).
- Usa siempre HTTPS fuera de tu red. El secreto del MCP va en la URL; trátalo
  como una contraseña.
- Es una herramienta **personal** (un usuario, un token), no un servicio
  multiusuario.

## Hoja de ruta

- **Hecho (MVP):** proyectos, secciones, etiquetas, prioridades, fechas
  naturales, repeticiones, vistas, atajos, deshacer, CLI con `--json`.
- **Fase 2:** filtros con consulta y guardados en la barra lateral
  (`(hoy | vencidas) & #work & !#esperando`), mover tareas entre secciones,
  crear subtareas desde la TUI, reordenar, editar notas desde la TUI.
- **Hecho:** notas por tarea (CLI `note`, se ven en el detalle), migración
  inicial desde Todoist.
- **Hecho:** espejo hacia Recordatorios (`tasktui sync`).
- **Hecho:** captura desde Recordatorios/Siri y sync periódico con launchd.
- **Hecho:** núcleo `tasktui-core` separado, compila a wasm.
- **Hecho:** servidor con API en Cloudflare Workers + D1.
- **Hecho:** TUI/CLI en modo remoto, con caché sin conexión.
- **Hecho:** `tasktui export` a `.md` como respaldo.
- **Hecho:** descripción, notas con fecha, hora y fecha límite; CLI y API en inglés.
- **Hecho:** página web instalable (PWA) para el celular y cualquier navegador.
- **Hecho:** repo público con guías, servidor Docker + SQLite y servidor MCP.
- **Siguiente:** avisos push en la página web; imagen de Docker publicada; otras bases (Postgres).
- **Fase 3:** abrir la tarea en Obsidian,
  temas de color.
- **Fase 4:** notificaciones en macOS/iPhone (puente opcional con Recordatorios
  vía EventKit), publicación (`brew`, crates.io).

## Licencia

[MIT](LICENSE)
