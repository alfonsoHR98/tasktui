# Modos de almacenamiento y formato Markdown

Dos modos, con la misma TUI y la misma CLI:

- **Remoto** (recomendado si quieres usarlo desde varios lugares): las tareas
  viven en tu servidor (ver [docker.md](docker.md)).
- **Local**: una carpeta de archivos `.md`.

```sh
tasktui server connect https://tareas.tu-dominio.com   # pide el token; activa el modo remoto
tasktui server                                     # muestra el modo y si el servidor responde
tasktui server local | remote                      # cambiar de modo
tasktui server push                                # sube los .md locales al servidor (conserva ids)
tasktui export ~/Respaldo/Tareas                   # respaldo en .md desde cualquier modo
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
| Tarea | `- [ ] Pagar luz #casa 🔺 🔁 every month 📅 2026-11-05 ⏰ 18:00 🎯 2026-11-20 🆔 frlyy` |
| Hora · fecha límite | `⏰ 18:00` · `🎯 2026-11-20` (propios de tasktui; Obsidian Tasks los deja como texto) |
| Etiqueta | `#etiqueta` (las de Obsidian) |
| Prioridad | 🔺 p1 · ⏫ p2 · 🔼 p3 |
| Subtarea | tarea con sangría |
| Descripción | texto con sangría bajo la tarea (`    Recibo de CFE`) |
| Nota | viñeta con sangría bajo la tarea (`    - 2026-11-06: llamé, sigue pendiente`) |

La descripción y las notas se mueven y se borran junto con su tarea.

- Lo que no es tarea (notas, texto libre) se respeta tal cual.
- Las tareas que no tocas no se reformatean.
- Cada tarea pendiente recibe un `🆔` corto y estable para que la CLI pueda
  referirse a ella (también las que escribas desde Obsidian).
- Al completar una tarea que se repite, la pendiente avanza de fecha y conserva
  su id; la ocurrencia hecha queda debajo con `✅ fecha` (igual que Obsidian Tasks).
