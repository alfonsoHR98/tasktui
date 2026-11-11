# API

La API del servidor (`tasktui-server`).

Todas las rutas salvo `/api/health` piden `Authorization: Bearer <API_TOKEN>`.

| Método | Ruta | Qué hace |
|---|---|---|
| GET | `/api/tasks?view=today&all=1&q=texto&today=AAAA-MM-DD` | vista agrupada (`today`, `inbox`, `upcoming`, `all`, `+Proyecto`, `#etiqueta`) |
| GET | `/api/tasks/:id` | una tarea |
| POST | `/api/tasks` `{"text": "Pagar luz mañana 18:00 p1 +Finanzas limit 20/11", "description": "…"}` | añadir con captura rápida |
| PATCH | `/api/tasks/:id` `{"text": "viernes p2"}` | cambia solo lo escrito (`"full": true` reemplaza todo; `"no_date"`, `"no_deadline"`, `"description"`) |
| PUT | `/api/tasks/:id` (tarea completa en JSON) | crear o reemplazar (lo usa la TUI) |
| POST | `/api/tasks/:id/done` | completar / reabrir |
| POST | `/api/tasks/:id/notes` `{"text": "…"}` | añadir nota con la fecha de hoy |
| DELETE | `/api/tasks/:id` | eliminar |
| GET / POST | `/api/projects` | listar / crear `{"name": "…"}` |
| GET | `/api/labels` | etiquetas con pendientes |
| GET | `/api/export` · POST `/api/import` | respaldo y carga (conserva ids) |

«Hoy» usa `TZ_OFFSET_MINUTES` si el cliente no manda `?today=`.
Los nombres anteriores en español (`vista`, `texto`…) se aceptan como alias.
Los errores devuelven `{"error": "mensaje en español"}`.

```sh
curl -H "Authorization: Bearer $API_TOKEN" "$SERVIDOR/api/tasks?view=today"
curl -H "Authorization: Bearer $API_TOKEN" -X POST "$SERVIDOR/api/tasks" \
     -d '{"text": "Pagar luz mañana 18:00 p1 +Finanzas"}'
```
