# Contribuir a tasktui

Gracias por querer mejorar tasktui. Es un proyecto pequeño y personal, así que
el proceso es sencillo.

## Antes de escribir código

- **¿Un error?** Abre un issue con lo que hiciste, lo que esperabas y lo que
  pasó. Si es de la captura rápida, pega el texto exacto.
- **¿Una idea?** Abre un issue antes de un cambio grande, para acordar el
  enfoque y que no trabajes en vano.
- **¿Una vulnerabilidad?** No abras un issue público: lee [SECURITY.md](SECURITY.md).

## Preparar el entorno

Necesitas [Go](https://go.dev/dl/) 1.27 o superior. Nada más: SQLite va en Go
puro, sin cgo.

```sh
git clone https://github.com/alfonsoHR98/tasktui.git && cd tasktui
make test     # go vet + go test ./...
make demo     # abre la TUI con tareas de ejemplo
```

Para probar tus cambios sin tocar tus tareas reales:

```sh
TASKTUI_DIR=/tmp/tasktui-pruebas go run ./cmd/tasktui
```

La estructura del código está en [docs/desarrollo.md](docs/desarrollo.md).

## Cómo trabajamos

- **El núcleo no hace E/S.** `internal/core` no lee el reloj, archivos ni red;
  quien lo llama le pasa «hoy». Así la TUI, la CLI y el servidor se comportan
  igual y todo se puede probar.
- **Cada cambio de comportamiento lleva su prueba.** Las de la TUI mandan
  teclas al modelo y revisan la pantalla; las del servidor levantan un servidor
  real con una base temporal.
- **Ninguna entrada debe tumbar el programa.** Si tocas la captura rápida,
  `TestSinPanico` tiene que seguir pasando.
- **Español.** La interfaz, los comentarios, los mensajes de commit y la
  documentación están en español. Los comandos de la CLI y las rutas de la API
  van en inglés.
- **Sin dependencias de más.** La página web del servidor y la del proyecto no
  tienen paso de compilación; mantengámoslo así.
- **Si cambias la interfaz de la TUI**, regenera las capturas con
  `make capturas` y súbelas en el mismo commit.

## Enviar un pull request

1. Crea una rama desde `main`.
2. Haz commits pequeños con un mensaje que diga qué cambia y por qué
   (`TUI: la barra de estado muestra el modo`).
3. Comprueba que `make test` pasa y que `gofmt -l .` no lista nada.
4. Actualiza la documentación y [CHANGELOG.md](CHANGELOG.md) si el cambio lo
   nota quien usa tasktui.
5. Abre el pull request explicando el cambio. La CI corre las pruebas en Linux
   y macOS, y construye la imagen de Docker.

## Convivencia

Trata a las demás personas con respeto y asume buena fe. Las críticas van al
código, no a quien lo escribió. Quien mantiene el proyecto puede editar o
cerrar aportes que no sigan esto.
