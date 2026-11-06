# tasktui

Gestor de tareas para la terminal, inspirado en Todoist: proyectos, secciones,
etiquetas, prioridades, fechas en lenguaje natural, repeticiones y atajos de
teclado estilo vim. Las tareas se guardan como **Markdown compatible con el
plugin Obsidian Tasks**, así que se sincronizan por iCloud y se ven/editan desde
Obsidian en el iPhone.

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

## Instalación

```sh
cargo install --path crates/tasktui
tasktui            # abre la TUI
```

## Dónde se guardan las tareas

Dos modos, con la misma TUI y la misma CLI:

- **Remoto** (recomendado si quieres usarlo desde varios lugares): las tareas
  viven en tu servidor (ver «Servidor» más abajo).
- **Local**: una carpeta de archivos `.md`.

```sh
tasktui servidor conectar https://tasks.ejemplo.com   # pide el token; activa el modo remoto
tasktui servidor                                      # muestra el modo y si el servidor responde
tasktui servidor local | remoto                       # cambiar de modo
tasktui servidor subir                                # sube los .md locales al servidor (conserva ids)
tasktui exportar ~/Respaldo/Tareas                    # respaldo en .md desde cualquier modo
```

En modo remoto cada cambio va directo al servidor. La última lectura queda en
caché: sin internet ves tus tareas (la TUI muestra ⚠) y los cambios se rechazan
con un aviso en lugar de perderse. Deshacer (`u`) también funciona en remoto.
`TASKTUI_SERVER` + `TASKTUI_TOKEN` fuerzan el modo remoto; `TASKTUI_DIR`, el local.

### Modo local

Por defecto en `~/.local/share/tasktui`. Para verlas en el celular, apunta a una
carpeta dentro de tu vault de Obsidian (en iCloud):

```sh
tasktui dir "~/Library/Mobile Documents/iCloud~md~obsidian/Documents/<TuVault>/Tareas"
```

Eso se guarda en `~/.config/tasktui/config.toml`. La variable `TASKTUI_DIR`
tiene prioridad (útil para pruebas).

### Formato

| Concepto | En disco |
|---|---|
| Proyecto | un archivo `.md` (`Trabajo/Cliente.md` → proyecto `Trabajo/Cliente`) |
| Sección | encabezado `##` dentro del archivo |
| Tarea | `- [ ] Pagar luz #casa 🔺 🔁 every month 📅 2026-11-05 🆔 frlyy` |
| Etiqueta | `#etiqueta` (las de Obsidian) |
| Prioridad | 🔺 p1 · ⏫ p2 · 🔼 p3 |
| Subtarea | tarea con sangría |
| Nota | línea con sangría bajo la tarea (`    - texto`); se mueve y se borra con ella |

- Lo que no es tarea (notas, texto libre) se respeta tal cual.
- Las tareas que no tocas no se reformatean.
- Cada tarea pendiente recibe un `🆔` corto y estable para que la CLI pueda
  referirse a ella (también las que escribas desde Obsidian).
- Al completar una tarea que se repite, la pendiente avanza de fecha y conserva
  su id; la ocurrencia hecha queda debajo con `✅ fecha` (igual que Obsidian Tasks).

## Recordatorios de Apple (avisos en Mac e iPhone)

```sh
tasktui sync              # sincroniza una vez
tasktui sync --auto on    # además, sincroniza en segundo plano tras cada cambio
tasktui sync --agente on  # y cada 10 min con launchd (--cada N para cambiarlo)
```

- Crea la lista **tasktui** en Recordatorios con un recordatorio por cada tarea
  pendiente **con fecha** (título, fecha de todo el día, prioridad, proyecto y
  notas). Te avisa a la hora de «Notificación de hoy» de Recordatorios.
- Los `.md` mandan: si cambias un recordatorio de tasktui, el siguiente `sync`
  lo deja como la tarea. Dos excepciones:
  - si lo **completas** en Recordatorios, la tarea se completa (si se repite, avanza);
  - si **creas** un recordatorio en la lista tasktui (a mano o con Siri), se
    importa como tarea. El título acepta la captura rápida
    (`Comprar leche +Casa #super p1`); la fecha, prioridad y notas del
    recordatorio se respetan. Si no tiene fecha, pasa a tasktui y sale de
    Recordatorios (ahí solo viven las tareas con fecha).
- La primera vez macOS pide permiso de acceso a Recordatorios (el agente de
  launchd tiene su propio permiso; el registro queda en
  `~/Library/Logs/tasktui-sync.log`).
- La relación tarea ↔ recordatorio vive en `~/.config/tasktui/recordatorios.json`.

## Servidor (Cloudflare Workers + D1)

`crates/tasktui-worker` publica una API JSON con la misma lógica que la TUI
(captura rápida, vistas, completar con repeticiones). Corre en el plan gratuito
de Cloudflare y está siempre disponible.

```sh
cargo install worker-build --version 0.8.7 --locked
cd crates/tasktui-worker
npx wrangler d1 create tasktui                 # copia el database_id a wrangler.jsonc
npx wrangler d1 migrations apply tasktui --remote
npx wrangler secret put API_TOKEN              # un token largo y aleatorio
npx wrangler deploy                            # cambia `routes` por tu dominio
```

Todas las rutas salvo `/api/health` piden `Authorization: Bearer <API_TOKEN>`.

| Método | Ruta | Qué hace |
|---|---|---|
| GET | `/api/tasks?vista=hoy&todas=1&q=texto&hoy=AAAA-MM-DD` | vista agrupada (`hoy`, `inbox`, `proximos`, `todas`, `+Proyecto`, `#etiqueta`) |
| GET | `/api/tasks/:id` | una tarea |
| POST | `/api/tasks` `{"texto": "Pagar luz mañana p1 +Finanzas"}` | añadir con captura rápida |
| PATCH | `/api/tasks/:id` `{"texto": "viernes p2"}` | cambia solo lo escrito (`"completo": true` reemplaza todo; `"sin_fecha": true`) |
| POST | `/api/tasks/:id/done` | completar / reabrir |
| POST | `/api/tasks/:id/notes` `{"texto": "…"}` | añadir nota |
| DELETE | `/api/tasks/:id` | eliminar |
| GET / POST | `/api/projects` | listar / crear `{"nombre": "…"}` |
| GET | `/api/labels` | etiquetas con pendientes |
| GET | `/api/export` · POST `/api/import` | respaldo y carga (conserva ids) |

«Hoy» usa `TZ_OFFSET_MINUTES` (en `wrangler.jsonc`) si el cliente no manda `?hoy=`.
Prueba local: `npx wrangler d1 migrations apply tasktui --local`, crea
`.dev.vars` con `API_TOKEN=...` y `npx wrangler dev`.

## Captura rápida

```
Pagar luz mañana p1 #casa +Finanzas cada mes
```

| Escribe | Significa |
|---|---|
| `#casa` o `@casa` | etiqueta |
| `+Trabajo` · `+Mis_Cuentas` | proyecto (`_` = espacio; si no existe, se crea) |
| `p1` … `p4` | prioridad |
| `hoy`, `mañana`, `pasado mañana` | fecha |
| `lunes` … `domingo` | el próximo ese día |
| `en 3 días`, `en 2 semanas`, `en 1 mes` | fecha relativa |
| `15/11`, `15/11/2026`, `2026-11-15` | fecha exacta |
| `cada día`, `cada 2 semanas`, `cada lunes`, `cada mes`, `cada año` | repetición |
| `\hoy` | la palabra como texto literal |

## Atajos (TUI)

| Tecla | Acción |
|---|---|
| `j` `k` / flechas | moverse |
| `h` `l` / `Tab` | barra lateral ↔ lista |
| `gi` `gt` `gu` `ga` | Inbox · Hoy · Próximos · Todas |
| `gg` `G` | inicio / final |
| `a` | añadir (en Hoy pone fecha de hoy; en un proyecto o etiqueta, la usa) |
| `e` | editar con la misma sintaxis |
| `x` / espacio | completar / reabrir |
| `s` | cambiar fecha · `t` para hoy |
| `1`–`4` | prioridad |
| `d` | eliminar (pide confirmación) |
| `u` | deshacer (hasta 50 pasos) |
| `/` · `Esc` | buscar · limpiar |
| `H` | mostrar completadas |
| `r` | recargar (también se recarga solo cada 30 s sin actividad) |
| `?` · `q` | ayuda · salir |

## CLI (scripts y Claude)

```sh
tasktui add Pagar luz mañana p1 #casa      # comillas si usas #: zsh lo trata como comentario a veces
tasktui list                               # hoy + vencidas
tasktui list proximos | inbox | todas | +Trabajo | '#casa'
tasktui list +Trabajo --all --buscar PR    # incluir completadas, filtrar texto
tasktui done frlyy                         # completar / reabrir
tasktui edit frlyy viernes p2              # cambia solo lo indicado
tasktui edit frlyy --sin-fecha
tasktui note frlyy Llamaron, sigue pendiente # añade una nota
tasktui rm frlyy
tasktui projects | labels
tasktui sync                               # espejo hacia Recordatorios
tasktui list --json                        # cualquier comando acepta --json
```

## Código

Workspace de Cargo con tres crates (el Worker solo compila a wasm y `cargo test` no lo incluye):

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

**`crates/tasktui-worker`**: el servidor (API sobre Cloudflare Workers + D1, ver arriba).

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
- **Hecho:** `tasktui exportar` a `.md` como respaldo.
- **Siguiente:** página web instalable (PWA) para el celular y cualquier
  navegador, con avisos push.
- **Después:** horas en las fechas (`mañana 18:00`) para avisos a una hora exacta.
- **Fase 3:** abrir la tarea en Obsidian,
  temas de color.
- **Fase 4:** notificaciones en macOS/iPhone (puente opcional con Recordatorios
  vía EventKit), servidor MCP, publicación (`brew`, crates.io).
