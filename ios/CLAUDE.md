# App iPhone de tasktui (SwiftUI)

Responde siempre en español (código, comentarios y textos de la interfaz).

## Qué es
Cliente nativo (iOS 17+, SwiftUI, sin dependencias) de un servidor propio de tareas
en tu servidor (`https://tasks.tu-dominio.com`) (binario Go + SQLite). Reemplaza a Todoist.
El proyecto se genera con `xcodegen` desde `project.yml`; **añade archivos en `TaskTUI/`**
(o en `TaskTUIWidget/` para el widget) y ejecuta `xcodegen` (no edites el `.xcodeproj`, está en .gitignore).
`Support/` tiene los Info.plist y entitlements que genera xcodegen desde `project.yml`.

## Archivos
- `API.swift`: cliente HTTP (Bearer token en Llavero) · `Models.swift`: `TaskItem`, `TaskGroup`, `ProjectInfo`, `Fmt` (fechas)
- `Store.swift`: sesión, proyectos, etiquetas, contadores y colores de listas (en `UserDefaults`; el servidor no guarda colores)
- `Theme.swift`: `ListTint`, `ListStyle` (color + SF Symbol), `SmartList` (Hoy, Próximos, Inbox, Todas), `Route`, `Priority`
- `Notifications.swift`: avisos locales

## Widget (`TaskTUIWidget/`)
- Widget «Hoy» (pequeño, mediano, grande y pantalla de bloqueo). Comparte con la app
  `API.swift`, `Models.swift` y `Theme.swift` (ver `project.yml`).
- Sesión compartida por el App Group `group.dev.madaradev.tasktui`: token en el Llavero
  con ese grupo y URL/colores en `Shared.defaults` (`API.swift`, enum `Shared`).
- El círculo usa `CompleteTaskIntent` (App Intent) para completar sin abrir la app.
- Enlaces: `tasktui://hoy` y `tasktui://tarea/<id>` (los maneja `RootView.open`).
- La app recarga el widget en `Store.syncReminders()`, que se llama tras cada cambio.
- `RootView` (inicio con tarjetas + «Mis listas», `ListSheet` para crear lista o cambiar color/ícono)
- `TaskListView` (encabezado de color con contador, filas, deslizar, «Mostrar completadas», captura rápida + búsqueda)
- `TaskEditorView` (hoja de crear y editar con chips; crear arma el texto de captura rápida con el título escapado con `\`, editar usa `PUT`)

## Modelo de datos (tabla `tasks` en SQLite; JSON `TaskItem`)
| Campo | Tipo | Notas |
|---|---|---|
| id | string | 5 caracteres, p. ej. `096uw` |
| text | string | título |
| done | bool | |
| due | `AAAA-MM-DD`? | cuándo la harás |
| time | `HH:MM`? | hora de `due` |
| deadline | `AAAA-MM-DD`? | fecha límite (distinta de `due`) |
| priority | 1…4 | 1 máxima, 4 = sin prioridad |
| labels | [string] | etiquetas (`#etiqueta`) |
| recurrence | string? | en español: «cada lunes»; `recurrence_rule` = formato Obsidian |
| project | string | `Inbox` por defecto; los proyectos viven en la tabla `projects` |
| section | string? | sección dentro del proyecto |
| description | string | texto largo |
| notes | [{date?, text}] | comentarios de seguimiento con fecha |

## API (todas con `Authorization: Bearer <token>`; añadir `?today=AAAA-MM-DD`)
- `GET /api/tasks?view=today|inbox|upcoming|all|+Proyecto|#etiqueta&q=texto&all=1` → `{view, groups:[{title, tasks}]}` (`all=1` incluye completadas)
- `POST /api/tasks` `{text, project?, description?}`: **captura rápida**, el servidor interpreta el texto («Pagar luz mañana 18:00 p1 +Finanzas #casa limit 20/11 cada mes»)
- `PATCH /api/tasks/:id` `{text?, description?, full?, no_date?, no_deadline?}`: `text` cambia solo lo escrito (fecha, prioridad…)
- `PUT /api/tasks/:id` (tarea completa) · `DELETE /api/tasks/:id`
- `POST /api/tasks/:id/done` (alterna; si es recurrente devuelve `occurrence` y crea la siguiente)
- `POST /api/tasks/:id/notes` `{text}` · `GET/POST /api/projects` · `GET /api/labels`

## Objetivo de diseño
Que se sienta como la app **Recordatorios de Apple**: listas con color e ícono (SF Symbols),
encabezado grande con contador, filas limpias con círculo, nueva tarea con hoja inferior
(título, notas, y chips/botones para fecha, hora, prioridad, proyecto y etiquetas en lugar de
escribir la sintaxis), edición en línea, deslizar para completar/eliminar, completadas
ocultas con «Mostrar completadas». Mantén modo oscuro, Dynamic Type y texto en español.
Para fecha/hora/prioridad desde UI, construye el `text` de captura rápida o usa `PUT` con la tarea completa.
Sin emojis en la interfaz: usa SF Symbols.
