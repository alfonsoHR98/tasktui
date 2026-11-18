# Servidor en Cloudflare Workers + D1

Además del servidor en Go (Docker o binario), tasktui se puede servir desde un
Worker de Cloudflare con la base en D1. Es lo que usa la instancia del autor y
cabe sin problema en el plan gratis (el Worker pesa unos 235 KB comprimido).

El código está en [`worker/`](../worker): un workspace de Rust con el núcleo
(`worker/core`) y el Worker (`worker/server`). Expone la misma API, la misma
página web y el mismo MCP que el servidor en Go.

## Dos lenguajes, un mismo comportamiento

La TUI y la CLI están en Go y el Worker en Rust, así que la captura rápida y el
MCP existen dos veces. Para que no se desalineen:

- Los casos de [`testdata/captura.json`](../testdata/captura.json) se prueban en
  los dos (`go test ./internal/core` y `cargo test` en `worker/`). Si cambias el
  parser en uno, añade el caso ahí y la prueba del otro fallará hasta portarlo.
- Los textos del MCP (`internal/core/mcp.go` y `worker/core/src/mcp.rs`) y los
  mensajes de la API son iguales a propósito.
- La página web es la misma: `worker/server/web` es una copia de
  `internal/server/web`. Si cambias una, copia la otra.

La CI corre las pruebas de los dos.

## Instalarlo

Necesitas Rust con el destino `wasm32-unknown-unknown`, `worker-build`
(`cargo install worker-build`) y una cuenta de Cloudflare.

```sh
cd worker/server
cp wrangler.example.jsonc wrangler.jsonc        # no se sube al repo
npx wrangler d1 create tasktui                  # pega el database_id en wrangler.jsonc
npx wrangler d1 migrations apply tasktui --remote
npx wrangler secret put API_TOKEN               # openssl rand -hex 24
npx wrangler secret put MCP_SECRET              # openssl rand -hex 32 (opcional)
npx wrangler deploy
```

## Probarlo en local antes de desplegar

```sh
cd worker
cargo test                                      # núcleo + casos compartidos
cd server
npx wrangler d1 export tasktui --remote --output=/tmp/copia.sql   # copia (y respaldo) de producción
npx wrangler d1 execute tasktui --local --file=/tmp/copia.sql
printf 'API_TOKEN=prueba\nMCP_SECRET=prueba-mcp\n' > .dev.vars
npx wrangler dev --local --port 8788
```

El export también sirve como respaldo antes de cada despliegue.

## Comprobar qué versión está desplegada

```sh
curl -s https://tu-dominio/api/health          # {"ok":true,"version":"…"}
```

Después de desplegar cambios en las herramientas del MCP, desconecta y vuelve a
conectar el conector en claude.ai para que recargue la lista.
