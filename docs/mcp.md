# Servidor MCP (para Claude y otras IAs)

El servidor expone `POST /mcp/<MCP_SECRET>` (MCP por HTTP,
solo JSON). Así una IA puede consultar y manejar tus tareas con las mismas
reglas que la TUI (captura rápida, repeticiones…).

Herramientas: `list_tasks`, `get_task`, `add_task`, `update_task`,
`complete_task`, `add_note`, `delete_task`, `list_projects`, `create_project`
y `list_labels`.

> **El secreto va en la URL** porque los conectores de claude.ai no envían
> cabeceras personalizadas. Trátalo como una contraseña: usa siempre HTTPS, no
> lo compartas y genera uno largo (`openssl rand -hex 32`). Si se filtra,
> cámbialo y vuelve a desplegar.

## Activarlo

- **Docker:** pon `MCP_SECRET=…` en `.env` y `docker compose up -d`.

Sin `MCP_SECRET`, el MCP está apagado (la ruta responde 404).

## Conectarlo

- **claude.ai:** Ajustes → Conectores → Añadir conector personalizado →
  `https://tu-servidor/mcp/<MCP_SECRET>`. (En planes de equipo, un administrador
  debe permitir los conectores personalizados.)
- **Claude Code:**
  `claude mcp add --transport http tasktui https://tu-servidor/mcp/<MCP_SECRET>`

Prueba: «¿Qué tengo para hoy?» o «Añade pagar luz mañana 18:00 p1 +Finanzas».
