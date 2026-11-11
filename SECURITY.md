# Seguridad

## Reportar una vulnerabilidad

No abras un issue público. Usa el reporte privado de GitHub:
**[Security → Report a vulnerability](https://github.com/alfonsoHR98/tasktui/security/advisories/new)**.

Incluye qué versión usas, cómo reproducirlo y qué impacto tiene. Recibirás
respuesta en cuanto sea posible; es un proyecto mantenido por una persona en su
tiempo libre. Se corrige sobre la última versión publicada.

## Modelo de seguridad

tasktui es una herramienta **personal**: un usuario, un token.

- **`API_TOKEN` es la única llave.** Quien lo tenga puede leer y cambiar todas
  las tareas. Genera uno largo (`openssl rand -hex 24`) y guárdalo fuera del
  repositorio; `.env` está en `.gitignore`.
- **El servidor habla HTTP plano** y por defecto solo escucha en `127.0.0.1`.
  Fuera de tu red, ponle delante HTTPS (Caddy, Cloudflare Tunnel o Tailscale:
  ver [docs/docker.md](docs/docker.md)).
- **El secreto del MCP va en la URL**, porque algunos clientes no envían
  cabeceras propias. Trátalo como una contraseña y cámbialo si se filtra. Sin
  `MCP_SECRET`, el MCP está apagado.
- **En tu equipo**, el token se guarda en `~/.config/tasktui/servidor.toml` con
  permisos 600.
- No hay cuentas, roles ni aislamiento entre usuarios. No lo expongas como
  servicio compartido.
