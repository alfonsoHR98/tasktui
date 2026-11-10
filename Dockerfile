# Servidor autoalojado de tasktui (API + página web + MCP) con SQLite.
#   docker build -t tasktui .
#   docker run -p 8787:8787 -e API_TOKEN=... -v tasktui-data:/data tasktui
FROM rust:1-slim AS build
WORKDIR /src
COPY Cargo.toml Cargo.lock ./
COPY crates ./crates
RUN cargo build --release -p tasktui-server

FROM debian:stable-slim
RUN useradd --system --uid 10001 --home /data tasktui \
 && mkdir /data && chown tasktui /data \
 && apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
 && rm -rf /var/lib/apt/lists/*
COPY --from=build /src/target/release/tasktui-server /usr/local/bin/tasktui-server
USER tasktui
ENV DATABASE_PATH=/data/tasktui.db PORT=8787
VOLUME /data
EXPOSE 8787
HEALTHCHECK --interval=30s --timeout=3s CMD curl -fs http://127.0.0.1:8787/api/health || exit 1
CMD ["tasktui-server"]
