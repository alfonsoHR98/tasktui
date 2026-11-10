//! Parte común del servidor MCP (JSON-RPC sobre HTTP): herramientas, su
//! traducción a llamadas de la API y las respuestas del protocolo.
//!
//! No hace E/S: el Worker (Cloudflare) y el servidor nativo (Docker) solo
//! ejecutan la llamada HTTP interna y devuelven el resultado.

use serde_json::{json, Map, Value};

const SINTAXIS: &str = "Captura rápida: fecha («hoy», «mañana», «viernes», «20/11»), hora («18:00»), \
prioridad p1 (máxima) a p4, +Proyecto, #etiqueta, fecha límite con «limit 20/11» y repetición \
(«cada lunes», «cada mes»). Ejemplo: «Pagar luz mañana 18:00 p1 +Finanzas #casa».";

pub fn tools() -> Value {
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

/// Codifica un componente de URL (como `encodeURIComponent`).
fn enc(s: &str) -> String {
    let mut out = String::new();
    for b in s.bytes() {
        if b.is_ascii_alphanumeric() || b"-_.!~*'()".contains(&b) {
            out.push(b as char);
        } else {
            out.push_str(&format!("%{b:02X}"));
        }
    }
    out
}

fn str_arg<'a>(a: &'a Map<String, Value>, k: &str) -> Option<&'a str> {
    a.get(k).and_then(Value::as_str)
}

fn need<'a>(a: &'a Map<String, Value>, k: &str) -> std::result::Result<&'a str, String> {
    str_arg(a, k).filter(|s| !s.trim().is_empty()).ok_or_else(|| format!("Falta «{k}»"))
}

/// (método HTTP, ruta de la API, cuerpo) de cada herramienta.
pub fn plan(name: &str, a: &Map<String, Value>) -> Result<(&'static str, String, Option<Value>), String> {
    Ok(match name {
        "list_tasks" => {
            let mut p = format!("/api/tasks?view={}", enc(str_arg(a, "view").unwrap_or("today")));
            if let Some(q) = str_arg(a, "query") {
                p += &format!("&q={}", enc(q));
            }
            if a.get("include_done").and_then(Value::as_bool).unwrap_or(false) {
                p += "&all=1";
            }
            ("GET", p, None)
        }
        "get_task" => ("GET", format!("/api/tasks/{}", enc(need(a, "id")?)), None),
        "add_task" => {
            let mut b = json!({ "text": need(a, "text")? });
            for k in ["project", "description"] {
                if let Some(v) = str_arg(a, k) {
                    b[k] = json!(v);
                }
            }
            ("POST", "/api/tasks".into(), Some(b))
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
            ("PATCH", format!("/api/tasks/{}", enc(need(a, "id")?)), Some(Value::Object(b)))
        }
        "complete_task" => ("POST", format!("/api/tasks/{}/done", enc(need(a, "id")?)), None),
        "add_note" => (
            "POST",
            format!("/api/tasks/{}/notes", enc(need(a, "id")?)),
            Some(json!({ "text": need(a, "text")? })),
        ),
        "delete_task" => ("DELETE", format!("/api/tasks/{}", enc(need(a, "id")?)), None),
        "list_projects" => ("GET", "/api/projects".into(), None),
        "create_project" => ("POST", "/api/projects".into(), Some(json!({ "name": need(a, "name")? }))),
        "list_labels" => ("GET", "/api/labels".into(), None),
        _ => return Err(format!("Herramienta desconocida «{name}»")),
    })
}

/// Qué hacer con un mensaje JSON-RPC.
pub enum Rpc {
    /// Notificación (sin `id`): se responde 202 sin cuerpo.
    Notification,
    /// Respuesta lista.
    Reply(Value),
    /// Ejecutar una herramienta y responder con [`tool_reply`].
    Call { id: Value, name: String, args: Map<String, Value> },
}

/// Interpreta un mensaje JSON-RPC.
pub fn dispatch(msg: &Value) -> Rpc {
    let Some(id) = msg.get("id").cloned() else {
        return Rpc::Notification;
    };
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
            return Rpc::Call {
                id,
                name: params.get("name").and_then(Value::as_str).unwrap_or("").to_string(),
                args: params.get("arguments").and_then(Value::as_object).cloned().unwrap_or_default(),
            };
        }
        _ => {
            return Rpc::Reply(json!({ "jsonrpc": "2.0", "id": id,
                "error": { "code": -32601, "message": format!("Método desconocido: {method}") } }))
        }
    };
    Rpc::Reply(json!({ "jsonrpc": "2.0", "id": id, "result": result }))
}

/// Respuesta JSON-RPC al resultado de una herramienta (`Err` = error para la IA).
pub fn tool_reply(id: Value, out: Result<String, String>) -> Value {
    let result = match out {
        Ok(t) => json!({ "content": [{ "type": "text", "text": t }] }),
        Err(e) => json!({ "isError": true, "content": [{ "type": "text", "text": e }] }),
    };
    json!({ "jsonrpc": "2.0", "id": id, "result": result })
}

/// Error de JSON-RPC para un cuerpo que no es JSON.
pub fn parse_error() -> Value {
    json!({ "jsonrpc": "2.0", "id": null, "error": { "code": -32700, "message": "JSON inválido" } })
}
