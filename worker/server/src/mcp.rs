//! Servidor MCP (Streamable HTTP, solo JSON) para que Claude y otras IAs
//! manejen las tareas. Las herramientas llaman internamente a la misma API
//! que usan la TUI y la página web, así que se comportan igual.
//! Las herramientas y el protocolo viven en `tasktui_core::mcp`.
//!
//! Ruta: `POST /mcp/<MCP_SECRET>`. El secreto va en la URL porque los
//! conectores de claude.ai no envían cabeceras personalizadas.

use serde_json::{Map, Value};
use tasktui_core::mcp::{self, Rpc};
use worker::*;

/// Ejecuta una herramienta llamando a la API interna. `Err` = error para la IA.
async fn call_tool(name: &str, args: &Map<String, Value>, env: &Env) -> std::result::Result<String, String> {
    let (method, path, body) = mcp::plan(name, args)?;
    let method = match method {
        "POST" => Method::Post,
        "PATCH" => Method::Patch,
        "DELETE" => Method::Delete,
        _ => Method::Get,
    };
    let mut init = RequestInit::new();
    init.with_method(method);
    if let Some(b) = body {
        let headers = Headers::new();
        let _ = headers.set("Content-Type", "application/json");
        init.with_headers(headers);
        init.with_body(Some(b.to_string().into()));
    }
    let req = Request::new_with_init(&format!("https://interno{path}"), &init).map_err(|e| e.to_string())?;
    let mut resp = match crate::route_inner(req, env.clone()).await {
        Ok(r) => r,
        Err(crate::ApiError(_, msg)) => return Err(msg),
    };
    let ok = resp.status_code() < 400;
    let v: Value = resp.json().await.unwrap_or(Value::Null);
    let text = serde_json::to_string_pretty(&v).unwrap_or_default();
    if ok {
        Ok(text)
    } else {
        Err(v.get("error").and_then(Value::as_str).unwrap_or(&text).to_string())
    }
}

/// Responde a un mensaje JSON-RPC; `None` si era una notificación.
async fn handle(msg: &Value, env: &Env) -> Option<Value> {
    match mcp::dispatch(msg) {
        Rpc::Notification => None,
        Rpc::Reply(v) => Some(v),
        Rpc::Call { id, name, args } => Some(mcp::tool_reply(id, call_tool(&name, &args, env).await)),
    }
}

/// Atiende `POST /mcp/<secreto>` (el secreto ya está validado).
pub async fn serve(mut req: Request, env: &Env) -> Result<Response> {
    let body: Value = match req.json().await {
        Ok(v) => v,
        Err(_) => {
            return Ok(Response::from_json(&mcp::parse_error())?.with_status(400))
        }
    };
    if let Some(items) = body.as_array() {
        let mut out = vec![];
        for m in items {
            if let Some(r) = handle(m, env).await {
                out.push(r);
            }
        }
        return if out.is_empty() { Ok(Response::empty()?.with_status(202)) } else { Response::from_json(&out) };
    }
    match handle(&body, env).await {
        Some(r) => Response::from_json(&r),
        None => Ok(Response::empty()?.with_status(202)),
    }
}
