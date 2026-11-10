//! Rutas de la API: mismo contrato que el Worker (ver `tasktui-worker/src/lib.rs`).
//! `handle` es síncrono y no conoce HTTP: recibe método, ruta, consulta y cuerpo,
//! y devuelve (código, JSON). Así lo reutiliza también el servidor MCP.

use std::collections::HashMap;

use chrono::{Duration, NaiveDate};
use rand::Rng;
use serde::Deserialize;
use serde_json::{json, Value};
use tasktui_core::dto::TaskDto;
use tasktui_core::model::{Completion, Note, Task};
use tasktui_core::quick::parse_quick;
use tasktui_core::views::{self, View};
use tasktui_core::INBOX;

use crate::db::Db;

pub struct ApiError(pub u16, pub String);

pub type Out = Result<(u16, Value), ApiError>;

impl From<rusqlite::Error> for ApiError {
    fn from(e: rusqlite::Error) -> Self {
        eprintln!("{{\"nivel\":\"error\",\"error\":{:?}}}", e.to_string());
        ApiError(500, "Error interno".into())
    }
}

fn bad(msg: impl Into<String>) -> ApiError {
    ApiError(400, msg.into())
}

fn not_found(id: &str) -> ApiError {
    ApiError(404, format!("No existe la tarea «{id}»"))
}

fn ok(v: Value) -> Out {
    Ok((200, v))
}

fn dto(t: &Task) -> Value {
    serde_json::to_value(TaskDto::from(t)).unwrap_or(Value::Null)
}

fn body<T: for<'de> Deserialize<'de>>(raw: &[u8], msg: &str) -> Result<T, ApiError> {
    serde_json::from_slice(raw).map_err(|_| bad(msg))
}

/// Parámetro de la consulta; acepta también su nombre anterior en español.
fn param(q: &HashMap<String, String>, key: &str) -> Option<String> {
    let alias = match key {
        "view" => "vista",
        "all" => "todas",
        "today" => "hoy",
        _ => key,
    };
    q.get(key).or_else(|| q.get(alias)).cloned()
}

/// «Hoy» del usuario: `?today=` si viene; si no, la hora actual con el desfase de la zona.
fn today(q: &HashMap<String, String>, tz_offset_min: i64) -> NaiveDate {
    if let Some(d) = param(q, "today").and_then(|s| s.parse().ok()) {
        return d;
    }
    (chrono::Utc::now() + Duration::minutes(tz_offset_min.clamp(-1440, 1440))).date_naive()
}

/// Id corto de 5 caracteres, como en los `.md`. No es secreto.
fn unique_id(db: &Db) -> Result<String, ApiError> {
    const ALPHA: &[u8] = b"0123456789abcdefghijklmnopqrstuvwxyz";
    let mut rng = rand::rng();
    loop {
        let id: String = (0..5).map(|_| ALPHA[rng.random_range(0..36)] as char).collect();
        if !db.exists(&id)? {
            return Ok(id);
        }
    }
}

#[derive(Deserialize)]
struct AddBody {
    #[serde(alias = "texto")]
    text: String,
    #[serde(alias = "proyecto")]
    project: Option<String>,
    #[serde(default)]
    description: String,
}

#[derive(Deserialize)]
struct EditBody {
    #[serde(alias = "texto")]
    text: Option<String>,
    #[serde(default, alias = "completo")]
    full: bool,
    #[serde(default, alias = "sin_fecha")]
    no_date: bool,
    #[serde(default)]
    no_deadline: bool,
    description: Option<String>,
}

#[derive(Deserialize)]
struct NoteBody {
    #[serde(alias = "texto")]
    text: String,
}

#[derive(Deserialize)]
struct ProjectBody {
    #[serde(alias = "nombre")]
    name: String,
}

#[derive(Deserialize)]
struct ImportBody {
    #[serde(default, alias = "proyectos")]
    projects: Vec<String>,
    #[serde(alias = "tareas")]
    tasks: Vec<TaskDto>,
}

fn list(db: &Db, q: &HashMap<String, String>, tz: i64) -> Out {
    let name = param(q, "view").unwrap_or_else(|| "today".into());
    let view = View::parse(&name).ok_or_else(|| {
        bad(format!("Vista desconocida «{name}». Usa: today, inbox, upcoming, all, +Proyecto, #etiqueta"))
    })?;
    let show_done = param(q, "all").is_some_and(|v| v == "1" || v == "true");
    let text = param(q, "q").unwrap_or_default();
    let today = today(q, tz);
    let tasks = db.all()?;
    let projects = db.projects()?;
    let indexed: Vec<(usize, &Task)> = tasks.iter().enumerate().collect();
    let groups: Vec<Value> = views::build(&indexed, &projects, &view, today, show_done, &text)
        .into_iter()
        .map(|g| json!({ "title": g.title, "tasks": g.items.iter().map(|&i| dto(&tasks[i])).collect::<Vec<_>>() }))
        .collect();
    ok(json!({ "view": view.title(), "today": today, "groups": groups }))
}

fn add(db: &Db, q: &HashMap<String, String>, tz: i64, raw: &[u8]) -> Out {
    let b: AddBody = body(raw, "Se esperaba {\"text\": \"…\"}")?;
    let parsed = parse_quick(&b.text, today(q, tz));
    if parsed.text.is_empty() {
        return Err(bad("La tarea necesita texto"));
    }
    let mut t = parsed.into_task(b.project.as_deref().unwrap_or(INBOX));
    t.description = b.description.trim().to_string();
    t.project = db.canonical_project(&t.project)?;
    t.id = Some(unique_id(db)?);
    db.add_project(&t.project)?;
    db.save(&t)?;
    Ok((201, dto(&t)))
}

fn edit(db: &Db, q: &HashMap<String, String>, tz: i64, id: &str, raw: &[u8]) -> Out {
    let b: EditBody = body(raw, "Cuerpo JSON inválido")?;
    let mut t = db.get(id)?.ok_or_else(|| not_found(id))?;
    if let Some(text) = &b.text {
        let parsed = parse_quick(text, today(q, tz));
        if b.full {
            if parsed.text.is_empty() {
                return Err(bad("La tarea necesita texto"));
            }
            parsed.apply_full(&mut t);
        } else {
            parsed.apply_partial(&mut t);
        }
    }
    if b.no_date {
        t.due = None;
        t.time = None;
        t.recurrence = None;
    }
    if b.no_deadline {
        t.deadline = None;
    }
    if let Some(d) = &b.description {
        t.description = d.trim().to_string();
    }
    t.project = db.canonical_project(&t.project)?;
    db.add_project(&t.project)?;
    db.save(&t)?;
    ok(dto(&t))
}

fn put(db: &Db, id: &str, raw: &[u8]) -> Out {
    let d: TaskDto = body(raw, "Se esperaba una tarea en JSON")?;
    let mut t = Task::from(&d);
    if t.text.trim().is_empty() {
        return Err(bad("La tarea necesita texto"));
    }
    if id.is_empty() || id.len() > 32 || !id.chars().all(|c| c.is_ascii_alphanumeric()) {
        return Err(bad("Id inválido"));
    }
    t.id = Some(id.to_string());
    if t.project.trim().is_empty() {
        t.project = INBOX.into();
    }
    t.project = db.canonical_project(&t.project)?;
    db.add_project(&t.project)?;
    db.save(&t)?;
    ok(dto(&t))
}

fn toggle(db: &Db, q: &HashMap<String, String>, tz: i64, id: &str) -> Out {
    let today = today(q, tz);
    let mut t = db.get(id)?.ok_or_else(|| not_found(id))?;
    if t.done {
        t.set_done(false, today);
        db.save(&t)?;
        return ok(json!({ "task": dto(&t), "completed": false }));
    }
    match t.complete(today, unique_id(db)?) {
        Completion::Simple(done) => {
            db.save(&done)?;
            ok(json!({ "task": dto(&done), "completed": true }))
        }
        Completion::Recurring { next, done } => {
            db.save(&done)?;
            db.save(&next)?;
            ok(json!({ "task": dto(&next), "completed": true, "occurrence": dto(&done) }))
        }
    }
}

fn add_note(db: &Db, q: &HashMap<String, String>, tz: i64, id: &str, raw: &[u8]) -> Out {
    let b: NoteBody = body(raw, "Se esperaba {\"text\": \"…\"}")?;
    if b.text.trim().is_empty() {
        return Err(bad("La nota está vacía"));
    }
    let mut t = db.get(id)?.ok_or_else(|| not_found(id))?;
    t.notes.push(Note::new(&b.text, Some(today(q, tz))));
    db.save(&t)?;
    ok(dto(&t))
}

fn projects(db: &Db) -> Out {
    let tasks = db.all()?;
    let out: Vec<Value> = db
        .projects()?
        .into_iter()
        .map(|p| {
            let open = tasks.iter().filter(|t| !t.done && t.project.eq_ignore_ascii_case(&p)).count();
            json!({ "name": p, "open": open })
        })
        .collect();
    ok(json!(out))
}

fn create_project(db: &Db, raw: &[u8]) -> Out {
    let b: ProjectBody = body(raw, "Se esperaba {\"name\": \"…\"}")?;
    let name = b.name.trim();
    if name.is_empty() {
        return Err(bad("El proyecto necesita nombre"));
    }
    db.add_project(name)?;
    Ok((201, json!({ "name": name })))
}

fn labels(db: &Db) -> Out {
    let mut m: std::collections::BTreeMap<String, usize> = Default::default();
    for t in db.all()?.iter().filter(|t| !t.done) {
        for l in &t.labels {
            *m.entry(l.clone()).or_default() += 1;
        }
    }
    ok(json!(m.into_iter().map(|(l, n)| json!({ "name": l, "open": n })).collect::<Vec<_>>()))
}

fn export(db: &Db) -> Out {
    let tasks: Vec<TaskDto> = db.all()?.iter().map(TaskDto::from).collect();
    ok(json!({ "projects": db.projects()?, "tasks": tasks }))
}

fn import(db: &Db, raw: &[u8]) -> Out {
    let b: ImportBody = body(raw, "Se esperaba {\"tasks\": […]}")?;
    for p in &b.projects {
        db.add_project(p)?;
    }
    let mut n = 0;
    for d in &b.tasks {
        let mut t = Task::from(d);
        if t.id.is_none() {
            t.id = Some(unique_id(db)?);
        }
        if t.project.is_empty() {
            t.project = INBOX.into();
        }
        db.add_project(&t.project)?;
        db.save(&t)?;
        n += 1;
    }
    ok(json!({ "imported": n }))
}

/// Atiende una llamada a `/api/*` (el token ya está validado).
/// `path_and_query` es, por ejemplo, `/api/tasks?view=today`.
pub fn handle(db: &Db, tz: i64, method: &str, path_and_query: &str, raw: &[u8]) -> Out {
    let url = url::Url::parse(&format!("http://interno{path_and_query}")).map_err(|_| bad("Ruta inválida"))?;
    let q: HashMap<String, String> = url.query_pairs().map(|(k, v)| (k.into_owned(), v.into_owned())).collect();
    let path = url.path().to_string();
    let parts: Vec<&str> = path.trim_start_matches("/api/").trim_end_matches('/').split('/').collect();
    match (method, parts.as_slice()) {
        ("GET", ["tasks"]) => list(db, &q, tz),
        ("POST", ["tasks"]) => add(db, &q, tz, raw),
        ("GET", ["tasks", id]) => ok(dto(&db.get(id)?.ok_or_else(|| not_found(id))?)),
        ("PATCH", ["tasks", id]) => edit(db, &q, tz, id, raw),
        ("PUT", ["tasks", id]) => put(db, id, raw),
        ("DELETE", ["tasks", id]) => {
            if db.delete(id)? {
                ok(json!({ "deleted": id }))
            } else {
                Err(not_found(id))
            }
        }
        ("POST", ["tasks", id, "done"]) => toggle(db, &q, tz, id),
        ("POST", ["tasks", id, "notes"]) => add_note(db, &q, tz, id, raw),
        ("GET", ["projects"]) => projects(db),
        ("POST", ["projects"]) => create_project(db, raw),
        ("GET", ["labels"]) => labels(db),
        ("GET", ["export"]) => export(db),
        ("POST", ["import"]) => import(db, raw),
        _ => Err(ApiError(404, "Ruta no encontrada".into())),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn call(db: &Db, method: &str, path: &str, body: Value) -> (u16, Value) {
        match handle(db, 0, method, path, body.to_string().as_bytes()) {
            Ok(r) => r,
            Err(ApiError(s, m)) => (s, json!({ "error": m })),
        }
    }

    #[test]
    fn ciclo_completo() {
        let db = Db::open(":memory:").unwrap();
        let (s, t) = call(&db, "POST", "/api/tasks?today=2026-11-10", json!({ "text": "Pagar luz mañana p1 +Finanzas #casa" }));
        assert_eq!(s, 201);
        assert_eq!(t["due"], "2026-11-11");
        assert_eq!(t["project"], "Finanzas");
        let id = t["id"].as_str().unwrap().to_string();

        let (_, l) = call(&db, "GET", "/api/tasks?view=%2BFinanzas&today=2026-11-10", json!(null));
        assert_eq!(l["groups"][0]["tasks"][0]["id"], id.as_str());

        let (_, d) = call(&db, "POST", &format!("/api/tasks/{id}/done?today=2026-11-10"), json!(null));
        assert_eq!(d["completed"], true);
        let (_, p) = call(&db, "GET", "/api/projects", json!(null));
        assert_eq!(p[1]["open"], 0);

        let (s, _) = call(&db, "DELETE", &format!("/api/tasks/{id}"), json!(null));
        assert_eq!(s, 200);
        let (s, _) = call(&db, "GET", &format!("/api/tasks/{id}"), json!(null));
        assert_eq!(s, 404);
    }

    #[test]
    fn repeticion_avanza() {
        let db = Db::open(":memory:").unwrap();
        let (_, t) = call(&db, "POST", "/api/tasks?today=2026-11-10", json!({ "text": "Regar cada lunes" }));
        let id = t["id"].as_str().unwrap();
        let (_, d) = call(&db, "POST", &format!("/api/tasks/{id}/done?today=2026-11-10"), json!(null));
        assert_eq!(d["task"]["done"], false);
        assert_eq!(d["task"]["due"], "2026-11-23");
        assert_eq!(d["occurrence"]["done"], true);
    }

    #[test]
    fn errores_en_espanol() {
        let db = Db::open(":memory:").unwrap();
        let (s, e) = call(&db, "POST", "/api/tasks", json!({ "text": "  " }));
        assert_eq!((s, e["error"].as_str().unwrap()), (400, "La tarea necesita texto"));
        let (s, _) = call(&db, "GET", "/api/tasks?view=nada", json!(null));
        assert_eq!(s, 400);
    }
}
