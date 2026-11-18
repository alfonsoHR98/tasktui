# Cambios

Los cambios que nota quien usa tasktui. El formato sigue
[Keep a Changelog](https://keepachangelog.com/es/1.1.0/) y las versiones,
[SemVer](https://semver.org/lang/es/).

## Sin publicar

### Servidor
- Vuelve el servidor para **Cloudflare Workers + D1** (Rust, en `worker/`), con
  las mismas correcciones y herramientas que el servidor en Go, incluida
  `delete_project`. Los casos de `testdata/captura.json` se prueban en Go y en
  Rust para que los dos interpreten igual la captura rápida.

### Corregido
- Ya no se duplican proyectos al capturar: `+Niuko Consultores` (con espacio)
  llega al proyecto que existe en vez de crear «Niuko» y dejar «Consultores» en
  el título; `+"Niuko Consultores"` ya no crea el proyecto `"Niuko`; y los
  nombres se comparan sin mayúsculas, acentos ni `_` («ramirez medellin» es
  «Ramírez Medellín»), en la TUI, la CLI, el servidor y el modo local.
- Un proyecto que se creó de paso al guardar una tarea desaparece cuando se
  queda sin tareas; solo se conservan vacíos los creados a propósito.
- `create_project` con un nombre equivalente devuelve el que ya existe.

### MCP
- `add_task` y `update_task` ya no crean proyectos: si el proyecto no existe
  responden con un error que sugiere los parecidos («¿Quisiste decir «Niuko
  Consultores»?»). Los nuevos se crean con `create_project`.
- `update_task` acepta `project` para mover una tarea.
- Instrucciones y descripciones más claras para la IA: el proyecto va en
  `project` con el nombre exacto de `list_projects`, no en el texto.

### Interfaz
- Al escribir una tarea, la vista previa marca en amarillo «proyecto nuevo»
  cuando `+Proyecto` no existe, antes de guardarla.
- El autocompletado de `+proyecto` no distingue acentos.

## 0.3.0 — 2026-11-11 (primera versión pública)

### Interfaz
- TUI rediseñada: marcos redondeados, tema **Aurora** en color verdadero, barra
  de estado con el modo (`NORMAL`, `ESCRIBIR`, `BUSCAR`, `ELIMINAR`), grupos
  con regla, marca de selección y avance del día en la barra lateral.
- Tema `ansi` para seguir los colores de la terminal (`theme` en
  `config.toml` o `TASKTUI_THEME`).
- La fecha límite se marca con `⚑` en lugar de un emoji, para que las columnas
  queden alineadas en cualquier terminal.
- `tasktui demo`: abre la TUI con tareas de ejemplo sin tocar las tuyas.

### Proyecto
- Página del proyecto con una réplica interactiva de la TUI (`site/`).
- Flujos para publicar binarios y la imagen de Docker con cada versión.

### Lo que ya traía
- TUI y CLI con captura rápida, proyectos, secciones, etiquetas, prioridades,
  repeticiones, vistas, búsqueda y deshacer.
- Descripción, notas con fecha, hora y fecha límite en las tareas.
- Modo local sobre archivos Markdown compatibles con Obsidian Tasks, y modo
  remoto con caché sin conexión.
- Servidor autoalojado (Docker o binario) con SQLite, API JSON, página web
  instalable (PWA) y servidor MCP.
- Avisos en la Mac con `tasktui notify`.
- `tasktui export` a Markdown y respaldo en JSON por la API.

### Historia
- El proyecto empezó en Rust con un servidor en Cloudflare Workers; se migró
  por completo a Go y el Worker se retiró.
- Se retiró la sincronización con Recordatorios de Apple.
