//! Servidor de tasktui autoalojado: la misma API, la misma página web y el mismo
//! MCP que el Worker de Cloudflare, pero en un solo binario con SQLite.
//!
//! Variables de entorno:
//! - `API_TOKEN` (obligatoria): token Bearer de la API.
//! - `MCP_SECRET` (opcional): activa el MCP en `POST /mcp/<MCP_SECRET>`.
//! - `DATABASE_PATH` (por defecto `tasktui.db`; en Docker `/data/tasktui.db`).
//! - `TZ_OFFSET_MINUTES` (por defecto 0): zona para «hoy» (México = -360).
//! - `PORT` (por defecto 8787), `BIND` (por defecto `0.0.0.0`).

mod api;
mod db;
mod web;

use std::sync::{Arc, Mutex};

use axum::body::{to_bytes, Body};
use axum::extract::State;
use axum::http::{header, HeaderValue, Method, Request, StatusCode};
use axum::response::{IntoResponse, Response};
use axum::Router;
use serde_json::{json, Value};
use tasktui_core::mcp::{self, Rpc};

use api::ApiError;
use db::Db;

struct App {
    db: Mutex<Db>,
    token: String,
    mcp_secret: Option<String>,
    tz: i64,
}

type Shared = Arc<App>;

/// Comparación en tiempo constante (no revela cuántos caracteres coinciden).
fn same_secret(a: &str, b: &str) -> bool {
    let (a, b) = (a.as_bytes(), b.as_bytes());
    let mut diff = (a.len() ^ b.len()) as u8;
    for i in 0..a.len().max(b.len()) {
        diff |= a.get(i).copied().unwrap_or(0) ^ b.get(i).copied().unwrap_or(0xff);
    }
    diff == 0
}

fn json_response(status: u16, v: &Value) -> Response {
    let mut r = (
        StatusCode::from_u16(status).unwrap_or(StatusCode::OK),
        [(header::CONTENT_TYPE, "application/json")],
        v.to_string(),
    )
        .into_response();
    r.headers_mut().insert(header::CACHE_CONTROL, HeaderValue::from_static("no-store"));
    r
}

fn err(status: u16, msg: &str) -> Response {
    json_response(status, &json!({ "error": msg }))
}

fn run(app: &App, method: &str, path_and_query: &str, raw: &[u8]) -> Result<(u16, Value), ApiError> {
    let db = app.db.lock().unwrap_or_else(|e| e.into_inner());
    api::handle(&db, app.tz, method, path_and_query, raw)
}

fn call_tool(app: &App, name: &str, args: &serde_json::Map<String, Value>) -> Result<String, String> {
    let (method, path, body) = mcp::plan(name, args)?;
    let raw = body.map(|b| b.to_string().into_bytes()).unwrap_or_default();
    match run(app, method, &path, &raw) {
        Ok((_, v)) => Ok(serde_json::to_string_pretty(&v).unwrap_or_default()),
        Err(ApiError(_, msg)) => Err(msg),
    }
}

fn mcp_message(app: &App, msg: &Value) -> Option<Value> {
    match mcp::dispatch(msg) {
        Rpc::Notification => None,
        Rpc::Reply(v) => Some(v),
        Rpc::Call { id, name, args } => Some(mcp::tool_reply(id, call_tool(app, &name, &args))),
    }
}

fn serve_mcp(app: &App, raw: &[u8]) -> Response {
    let Ok(body) = serde_json::from_slice::<Value>(raw) else {
        return json_response(400, &mcp::parse_error());
    };
    let out = match body.as_array() {
        Some(items) => {
            let replies: Vec<Value> = items.iter().filter_map(|m| mcp_message(app, m)).collect();
            (!replies.is_empty()).then(|| json!(replies))
        }
        None => mcp_message(app, &body),
    };
    match out {
        Some(v) => json_response(200, &v),
        None => StatusCode::ACCEPTED.into_response(),
    }
}

async fn handler(State(app): State<Shared>, req: Request<Body>) -> Response {
    let method = req.method().clone();
    let path = req.uri().path().to_string();
    let path_and_query = req.uri().path_and_query().map(|p| p.as_str().to_string()).unwrap_or_else(|| path.clone());
    let authorization = req
        .headers()
        .get(header::AUTHORIZATION)
        .and_then(|v| v.to_str().ok())
        .unwrap_or_default()
        .to_string();
    let Ok(raw) = to_bytes(req.into_body(), 2 * 1024 * 1024).await else {
        return err(413, "Cuerpo demasiado grande");
    };

    if let Some(secret) = path.strip_prefix("/mcp/") {
        let valid = app.mcp_secret.as_deref().is_some_and(|s| same_secret(secret.trim_end_matches('/'), s));
        if !valid {
            return err(404, "No encontrado");
        }
        if method != Method::POST {
            return err(405, "Usa POST");
        }
        return serve_mcp(&app, &raw);
    }
    if path == "/api/health" {
        return json_response(200, &json!({ "ok": true, "version": env!("CARGO_PKG_VERSION") }));
    }
    if path.starts_with("/api/") {
        let authorized = authorization
            .strip_prefix("Bearer ")
            .is_some_and(|t| same_secret(t.trim(), &app.token));
        if !authorized {
            return err(401, "Falta el token o no es válido");
        }
        return match run(&app, method.as_str(), &path_and_query, &raw) {
            Ok((status, v)) => json_response(status, &v),
            Err(ApiError(status, msg)) => err(status, &msg),
        };
    }
    if method == Method::GET || method == Method::HEAD {
        if let Some(r) = web::serve(&path) {
            return r;
        }
    }
    err(404, "No encontrado")
}

#[tokio::main]
async fn main() {
    let token = std::env::var("API_TOKEN").unwrap_or_default();
    if token.len() < 16 {
        eprintln!("Define API_TOKEN con al menos 16 caracteres (por ejemplo: openssl rand -hex 24).");
        std::process::exit(1);
    }
    let path = std::env::var("DATABASE_PATH").unwrap_or_else(|_| "tasktui.db".into());
    let db = Db::open(&path).unwrap_or_else(|e| {
        eprintln!("No se pudo abrir la base «{path}»: {e}");
        std::process::exit(1);
    });
    let mcp_secret = std::env::var("MCP_SECRET").ok().filter(|s| !s.is_empty());
    if mcp_secret.as_ref().is_some_and(|s| s.len() < 16) {
        eprintln!("MCP_SECRET debe tener al menos 16 caracteres.");
        std::process::exit(1);
    }
    let tz = std::env::var("TZ_OFFSET_MINUTES").ok().and_then(|v| v.parse().ok()).unwrap_or(0);
    let app = Arc::new(App { db: Mutex::new(db), token, mcp_secret, tz });

    let port = std::env::var("PORT").unwrap_or_else(|_| "8787".into());
    let bind = std::env::var("BIND").unwrap_or_else(|_| "0.0.0.0".into());
    let listener = tokio::net::TcpListener::bind(format!("{bind}:{port}")).await.unwrap_or_else(|e| {
        eprintln!("No se pudo abrir {bind}:{port}: {e}");
        std::process::exit(1);
    });
    println!("tasktui-server {} escuchando en http://{bind}:{port} (base: {path})", env!("CARGO_PKG_VERSION"));

    let router = Router::new().fallback(handler).with_state(app);
    axum::serve(listener, router)
        .with_graceful_shutdown(async {
            let _ = tokio::signal::ctrl_c().await;
        })
        .await
        .unwrap_or_else(|e| eprintln!("Error del servidor: {e}"));
}
