# Cambios

Los cambios que nota quien usa tasktui. El formato sigue
[Keep a Changelog](https://keepachangelog.com/es/1.1.0/) y las versiones,
[SemVer](https://semver.org/lang/es/).

## Sin publicar — será la 0.3.0, primera versión pública

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
