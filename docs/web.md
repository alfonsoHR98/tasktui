# Página web (celular y cualquier navegador)

El mismo servidor sirve una página en la raíz del dominio (por ejemplo
`https://tareas.tu-dominio.com`). La primera vez pide el token del servidor y lo
guarda solo en ese navegador; «Cerrar sesión» en el menú `⋯` lo borra.

- Vistas Hoy, Próximos, Inbox y Proyectos (con etiquetas), y búsqueda.
- Captura rápida con la misma sintaxis que la TUI. Al escribir `+` o `#`
  sugiere proyectos y etiquetas (toca la sugerencia o pulsa Tab). Dentro de un
  proyecto, las tareas nuevas van a ese proyecto.
- Toca el círculo para completar (con «Deshacer», también en las recurrentes).
  Toca la tarea para ver el detalle: cambiar fecha con botones o con captura
  rápida, editar la descripción, añadir notas con fecha y eliminar.
- Sin conexión muestra la última copia de cada vista (solo lectura).
- **iPhone:** ábrela en Safari → Compartir → «Agregar a pantalla de inicio».
  Se abre como app, sin barra del navegador.

No tiene paso de compilación: son archivos estáticos en
`internal/server/web/` (HTML, CSS y JavaScript sin dependencias) que el servidor
incrusta en su binario. Las cabeceras de seguridad (CSP) están en `internal/server/server.go`.
