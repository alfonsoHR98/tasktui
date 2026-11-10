# Servidor en Cloudflare Workers + D1

Corre en el plan gratuito de Cloudflare, sin servidores que mantener. La base es
D1 (SQLite administrado). Si prefieres tu propia máquina, usa
[Docker](docker.md).

## Requisitos

- Cuenta de Cloudflare (gratis) y, para dominio propio, un dominio en Cloudflare.
- Rust con el target wasm: `rustup target add wasm32-unknown-unknown`
- Node.js (para `npx wrangler`).

## Paso a paso

```sh
cargo install worker-build --version 0.8.7 --locked
cd crates/tasktui-worker

cp wrangler.example.jsonc wrangler.jsonc     # tu copia local (no se sube al repo)
npx wrangler login
npx wrangler d1 create tasktui               # imprime un database_id
```

1. Pega el `database_id` en `wrangler.jsonc`.
2. En `routes` pon tu dominio (`"tareas.tu-dominio.com"`). Sin dominio propio,
   borra `routes` y pon `"workers_dev": true`: te dará una URL `*.workers.dev`.
3. Ajusta `TZ_OFFSET_MINUTES` a tu zona (México = `-360`).

```sh
npx wrangler d1 migrations apply tasktui --remote
openssl rand -hex 24                          # cópialo
npx wrangler secret put API_TOKEN             # y pégalo aquí
npx wrangler deploy
curl https://tareas.tu-dominio.com/api/health
```

Abre la URL en el navegador: la web pide el token. Luego conecta la TUI con
`tasktui server connect https://tareas.tu-dominio.com`.

## Actualizar

```sh
git pull
cd crates/tasktui-worker
CI=true npx wrangler d1 migrations apply tasktui --remote   # antes de desplegar
npx wrangler deploy
```

Sin `CI=true`, wrangler pide confirmación y en una sesión no interactiva la
cancela sin avisar.

## Probar en local

```sh
npx wrangler d1 migrations apply tasktui --local
echo 'API_TOKEN=un-token-de-prueba' > .dev.vars
npx wrangler dev
```

## MCP

`npx wrangler secret put MCP_SECRET` y `npx wrangler deploy`. Ver [mcp.md](mcp.md).

## API

Referencia completa en [docs/api.md](api.md).
