# Servidor autoalojado de tasktui (API + página web + MCP) con SQLite.
#   docker build -t tasktui .
#   docker run -p 8787:8787 -e API_TOKEN=... -v tasktui-data:/data tasktui
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/tasktui-server ./cmd/tasktui-server
COPY internal/core ./internal/core
COPY internal/server ./internal/server
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/tasktui-server ./cmd/tasktui-server

FROM alpine:3
RUN adduser -S -u 10001 -h /data tasktui && mkdir -p /data && chown tasktui /data
COPY --from=build /out/tasktui-server /usr/local/bin/tasktui-server
USER tasktui
ENV DATABASE_PATH=/data/tasktui.db PORT=8787
VOLUME /data
EXPOSE 8787
HEALTHCHECK --interval=30s --timeout=3s CMD ["tasktui-server", "healthcheck"]
CMD ["tasktui-server"]
