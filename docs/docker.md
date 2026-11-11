# Servidor autoalojado con Docker (SQLite)

Un solo contenedor con la API, la página web (PWA) y el MCP. Las tareas viven
en un archivo SQLite dentro de un volumen. Es un solo binario en Go (sin cgo),
y la imagen pesa unos 30 MB. La TUI, la app de iPhone y la web usan su API.

## Requisitos

- Docker con Compose (Docker Desktop, o Docker Engine en un VPS/Raspberry Pi/NAS).
- Un dominio con HTTPS si vas a usarlo fuera de tu casa (ver paso 4).

## 1. Descarga y configura

```sh
git clone https://github.com/alfonsoHR98/tasktui.git
cd tasktui
cp .env.example .env
```

Edita `.env`:

```sh
API_TOKEN=$(openssl rand -hex 24)      # copia aquí el resultado
MCP_SECRET=                            # opcional, otro openssl rand -hex 24
TZ_OFFSET_MINUTES=-360                 # tu zona respecto a UTC (México = -360)
```

`API_TOKEN` es la contraseña de tu servidor: quien la tenga ve y cambia tus
tareas. Mínimo 16 caracteres.

## 2. Arranca

```sh
docker compose up -d --build
curl http://127.0.0.1:8787/api/health      # → {"ok":true,"version":"…"}
```

La primera vez compila el binario (unos minutos). Abre `http://127.0.0.1:8787`:
la web pide el token y ya puedes crear tareas.

## 3. Conecta tus clientes

```sh
tasktui server connect http://127.0.0.1:8787     # la TUI/CLI; pide el token
```

La app de iPhone y la web piden la URL y el token al abrirlas.

## 4. HTTPS para usarlo desde fuera de casa

El contenedor habla HTTP plano y por defecto solo escucha en `127.0.0.1`.
Pon delante algo que ponga HTTPS (necesario para el token, para instalar la web
como app en el iPhone y para el MCP). Tres opciones:

**Caddy** (la más simple si tienes un VPS con el dominio apuntando a él):

```
tareas.tu-dominio.com {
    reverse_proxy 127.0.0.1:8787
}
```

**Cloudflare Tunnel** (sin abrir puertos en casa):
`cloudflared tunnel --url http://127.0.0.1:8787` para probar, o un túnel con
nombre apuntando a `http://127.0.0.1:8787`.

**Tailscale** (solo tus dispositivos): `tailscale serve --bg 8787` te da una
URL `https://…ts.net` privada.

## 5. MCP (opcional)

Con `MCP_SECRET` definido, el servidor atiende `POST /mcp/<MCP_SECRET>`. Mira
[docs/mcp.md](mcp.md) para conectarlo a Claude.

## Datos y respaldos

La base está en el volumen `tasktui-data` (`/data/tasktui.db`).

```sh
# Respaldo en JSON desde la API
curl -H "Authorization: Bearer $API_TOKEN" http://127.0.0.1:8787/api/export > respaldo.json

# O respaldo en .md (Obsidian Tasks) con la CLI
tasktui export ~/Respaldo/Tareas

# O copia el archivo SQLite
docker run --rm -v tasktui_tasktui-data:/data -v "$PWD":/out alpine cp /data/tasktui.db /out/
```

(El nombre del volumen lleva el prefijo del proyecto de compose; míralo con
`docker volume ls`.)

### Pasar de un servidor a otro

```sh
curl -H "Authorization: Bearer $TOKEN_ORIGEN" https://origen/api/export > respaldo.json
curl -H "Authorization: Bearer $TOKEN_DESTINO" -X POST https://destino/api/import \
     -H 'Content-Type: application/json' --data @respaldo.json
```

Conserva los ids.

## Actualizar

```sh
git pull
docker compose up -d --build
```

Las migraciones de la base se aplican solas al arrancar.

## Variables de entorno

| Variable | Por defecto | Qué hace |
|---|---|---|
| `API_TOKEN` | — (obligatoria) | token Bearer de la API |
| `MCP_SECRET` | vacía (MCP apagado) | secreto de la URL del MCP |
| `TZ_OFFSET_MINUTES` | `0` | zona horaria para «hoy» cuando el cliente no manda `?today=` |
| `DATABASE_PATH` | `/data/tasktui.db` | archivo SQLite |
| `PORT` · `BIND` | `8787` · `0.0.0.0` | dónde escucha dentro del contenedor |

## Sin Docker

```sh
go build -o tasktui-server ./cmd/tasktui-server
API_TOKEN=… DATABASE_PATH=./tasktui.db ./tasktui-server
```

## Y si quiero Postgres u otra base

Hoy solo hay SQLite (una persona, un archivo: no necesita más). El acceso a
datos está aislado en `internal/server/db.go`, así que añadir otra
base es cambiar ese archivo. Ideas y pull requests, bienvenidos.
