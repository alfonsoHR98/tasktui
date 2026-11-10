//! Servidor MCP (Streamable HTTP, solo JSON) para que Claude y otras IAs
//! manejen las tareas. Las herramientas llaman internamente a la misma API
//! que usan la TUI y la página web, así que se comportan igual.
//!
//! Ruta: `POST /mcp/<MCP_SECRET>`. El secreto va en la URL porque los
//! conectores de claude.ai no envían cabeceras personalizadas.

use serde_json::{json, Map, Value};
use worker::*;

const SINTAXIS: &str = "Captura rápida: fecha («hoy», «mañana», «viernes», «20/11»), hora («18:00»), \
prioridad p1 (máxima) a p4, +Proyecto, #etiqueta, fecha límite con «limit 20/11» y repetición \
(«cada lunes», «cada mes»). Ejemplo: «Pagar luz mañana 18:00 p1 +Finanzas #casa».";

fn tools() -> Value {
    let s = |d: &str| json!({ "type": "string", "description": d });
    json!([
      { "name": "list_tasks",
        "description": "Lista tareas agrupadas. Vistas: today, inbox, upcoming, all, +Proyecto (p. ej. +Niuko Consultores) o #etiqueta. Por defecto solo pendientes.",
        "inputSchema": { "type": "object", "properties": {
            "view": s("today (por defecto), inbox, upcoming, all, +Proyecto o #etiqueta"),
            "query": s("Texto a buscar"),
            "include_done": { "type": "boolean", "description": "Incluir las completadas" } } } },
      { "name": "get_task",
        "description": "Devuelve una tarea completa (descripción y notas incluidas).",
        "inputSchema": { "type": "object", "properties": { "id": s("Id de la tarea") }, "required": ["id"] } },
      { "name": "add_task",
        "description": format!("Crea una tarea. {SINTAXIS} Sin +Proyecto va a Inbox, o al `project` indicado."),
        "inputSchema": { "type": "object", "properties": {
            "text": s("Título con captura rápida"),
            "project": s("Proyecto (opcional)"),
            "description": s("Descripción larga (opcional)") }, "required": ["text"] } },
      { "name": "update_task",
        "description": format!("Edita una tarea. `text` cambia solo lo que escribas (p. ej. «viernes p2» solo cambia fecha y prioridad). {SINTAXIS}"),
        "inputSchema": { "type": "object", "properties": {
            "id": s("Id de la tarea"),
            "text": s("Cambios en captura rápida"),
            "description": s("Reemplaza la descripción"),
            "no_date": { "type": "boolean", "description": "Quita fecha, hora y repetición" },
            "no_deadline": { "type": "boolean", "description": "Quita la fecha límite" } }, "required": ["id"] } },
      { "name": "complete_task",
        "description": "Completa una tarea, o la reabre si ya estaba completada. Si es recurrente crea la siguiente.",
        "inputSchema": { "type": "object", "properties": { "id": s("Id de la tarea") }, "required": ["id"] } },
      { "name": "add_note",
        "description": "Añade una nota de seguimiento con la fecha de hoy.",
        "inputSchema": { "type": "object", "properties": { "id": s("Id de la tarea"), "text": s("Nota") }, "required": ["id", "text"] } },
      { "name": "delete_task",
        "description": "Elimina una tarea definitivamente.",
        "inputSchema": { "type": "object", "properties": { "id": s("Id de la tarea") }, "required": ["id"] } },
      { "name": "list_projects",
        "description": "Lista los proyectos con su número de tareas pendientes.",
        "inputSchema": { "type": "object", "properties": {} } },
      { "name": "create_project",
        "description": "Crea un proyecto.",
        "inputSchema": { "type": "object", "properties": { "name": s("Nombre") }, "required": ["name"] } },
      { "name": "list_labels",
        "description": "Lista las etiquetas con tareas pendientes.",
        "inputSchema": { "type": "object", "properties": {} } }
    ])
}

fn enc(s: &str) -> String {
    js_sys::encode_uri_component(s).as_string().unwrap_or_default()
}

fn str_arg<'a>(a: &'a Map<String, Value>, k: &str) -> Option<&'a str> {
    a.get(k).and_then(Value::as_str)
}

fn need<'a>(a: &'a Map<String, Value>, k: &str) -> std::result::Result<&'a str, String> {
    str_arg(a, k).filter(|s| !s.trim().is_empty()).ok_or_else(|| format!("Falta «{k}»"))
}

/// (método, ruta de la API, cuerpo) de cada herramienta.
fn plan(name: &str, a: &Map<String, Value>) -> std::result::Result<(Method, String, Option<Value>), String> {
    Ok(match name {
        "list_tasks" => {
            let mut p = format!("/api/tasks?view={}", enc(str_arg(a, "view").unwrap_or("today")));
            if let Some(q) = str_arg(a, "query") {
                p += &format!("&q={}", enc(q));
            }
            if a.get("include_done").and_then(Value::as_bool).unwrap_or(false) {
                p += "&all=1";
            }
            (Method::Get, p, None)
        }
        "get_task" => (Method::Get, format!("/api/tasks/{}", enc(need(a, "id")?)), None),
        "add_task" => {
            let mut b = json!({ "text": need(a, "text")? });
            for k in ["project", "description"] {
                if let Some(v) = str_arg(a, k) {
                    b[k] = json!(v);
                }
            }
            (Method::Post, "/api/tasks".into(), Some(b))
        }
        "update_task" => {
            let mut b = Map::new();
            for k in ["text", "description"] {
                if let Some(v) = str_arg(a, k) {
                    b.insert(k.into(), json!(v));
                }
            }
            for k in ["no_date", "no_deadline"] {
                if let Some(v) = a.get(k).and_then(Value::as_bool) {
                    b.insert(k.into(), json!(v));
                }
            }
            (Method::Patch, format!("/api/tasks/{}", enc(need(a, "id")?)), Some(Value::Object(b)))
        }
        "complete_task" => (Method::Post, format!("/api/tasks/{}/done", enc(need(a, "id")?)), None),
        "add_note" => (
            Method::Post,
            format!("/api/tasks/{}/notes", enc(need(a, "id")?)),
            Some(json!({ "text": need(a, "text")? })),
        ),
        "delete_task" => (Method::Delete, format!("/api/tasks/{}", enc(need(a, "id")?)), None),
        "list_projects" => (Method::Get, "/api/projects".into(), None),
        "create_project" => (Method::Post, "/api/projects".into(), Some(json!({ "name": need(a, "name")? }))),
        "list_labels" => (Method::Get, "/api/labels".into(), None),
        _ => return Err(format!("Herramienta desconocida «{name}»")),
    })
}

/// Ejecuta una herramienta llamando a la API interna. `Err` = error para la IA.
async fn call_tool(name: &str, args: &Map<String, Value>, env: &Env) -> std::result::Result<String, String> {
    let (method, path, body) = plan(name, args)?;
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
    let id = msg.get("id")?.clone();
    let method = msg.get("method").and_then(Value::as_str).unwrap_or("");
    let params = msg.get("params").cloned().unwrap_or(Value::Null);
    let result = match method {
        "initialize" => {
            let v = params.get("protocolVersion").and_then(Value::as_str).unwrap_or("2025-03-26");
            json!({
                "protocolVersion": v,
                "capabilities": { "tools": { "listChanged": false } },
                "serverInfo": { "name": "tasktui", "version": env!("CARGO_PKG_VERSION") },
                "instructions": "Tareas personales. Usa list_tasks para consultar y add_task con captura rápida para crear."
            })
        }
        "ping" => json!({}),
        "tools/list" => json!({ "tools": tools() }),
        "tools/call" => {
            let name = params.get("name").and_then(Value::as_str).unwrap_or("");
            let empty = Map::new();
            let args = params.get("arguments").and_then(Value::as_object).unwrap_or(&empty);
            match call_tool(name, args, env).await {
                Ok(t) => json!({ "content": [{ "type": "text", "text": t }] }),
                Err(e) => json!({ "isError": true, "content": [{ "type": "text", "text": e }] }),
            }
        }
        _ => {
            return Some(json!({ "jsonrpc": "2.0", "id": id,
                "error": { "code": -32601, "message": format!("Método desconocido: {method}") } }))
        }
    };
    Some(json!({ "jsonrpc": "2.0", "id": id, "result": result }))
}

/// Atiende `POST /mcp/<secreto>` (el secreto ya está validado).
pub async fn serve(mut req: Request, env: &Env) -> Result<Response> {
    let body: Value = match req.json().await {
        Ok(v) => v,
        Err(_) => {
            return Ok(Response::from_json(&json!({ "jsonrpc": "2.0", "id": null,
                "error": { "code": -32700, "message": "JSON inválido" } }))?
            .with_status(400))
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
