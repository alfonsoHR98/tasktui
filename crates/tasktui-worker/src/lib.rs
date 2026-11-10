//! Servidor de tasktui: API JSON sobre Cloudflare Workers + D1.
//!
//! Toda la lógica (captura rápida, vistas, completar, repeticiones) viene de
//! `tasktui-core`, así que se comporta igual que la TUI.
//!
//! Autenticación: `Authorization: Bearer <API_TOKEN>` (secreto del Worker).
//!
//! | Método | Ruta | Qué hace |
//! |---|---|---|
//! | POST | `/mcp/<MCP_SECRET>` | servidor MCP (ver `mcp.rs`) |
//! | GET | `/api/health` | estado (sin token) |
//! | GET | `/api/tasks?view=today&all=1&q=texto&today=AAAA-MM-DD` | vista agrupada |
//! | GET | `/api/tasks/:id` | una tarea |
//! | POST | `/api/tasks` `{text, project?}` | añadir con captura rápida |
//! | PATCH | `/api/tasks/:id` `{text?, full?, no_date?, no_deadline?, description?}` | editar |
//! | PUT | `/api/tasks/:id` `TaskDto` | crear o reemplazar la tarea completa (lo usa la TUI) |
//! | POST | `/api/tasks/:id/done` | completar / reabrir |
//! | POST | `/api/tasks/:id/notes` `{text}` | añadir nota (con la fecha de hoy) |
//! | DELETE | `/api/tasks/:id` | eliminar |
//! | GET/POST | `/api/projects` | listar / crear `{name}` |
//! | GET | `/api/labels` | etiquetas con pendientes |
//! | GET | `/api/export` | todas las tareas (respaldo) |
//! | POST | `/api/import` `{tasks, projects}` | cargar tareas conservando ids |
//!
//! Los mensajes de error van en español. Los nombres anteriores en español
//! (`vista`, `texto`…) se siguen aceptando como alias.

mod db;
mod mcp;

use chrono::{Duration, NaiveDate};
use serde::{Deserialize, Serialize};
use serde_json::json;
use tasktui_core::dto::TaskDto;
use tasktui_core::model::{Completion, Note, Task};
use tasktui_core::quick::parse_quick;
use tasktui_core::views::{self, View};
use tasktui_core::INBOX;
use worker::*;

use db::Db;

/// Error de la API con su código HTTP.
struct ApiError(u16, String);

type ApiResult = std::result::Result<Response, ApiError>;

impl From<worker::Error> for ApiError {
    fn from(e: worker::Error) -> Self {
        console_error!("{{\"nivel\":\"error\",\"error\":{:?}}}", e.to_string());
        ApiError(500, "Error interno".into())
    }
}

fn bad(msg: impl Into<String>) -> ApiError {
    ApiError(400, msg.into())
}

fn not_found(id: &str) -> ApiError {
    ApiError(404, format!("No existe la tarea «{id}»"))
}

fn json_ok<T: Serialize>(v: &T) -> ApiResult {
    Ok(Response::from_json(v)?)
}

/// Comparación en tiempo constante (no revela cuántos caracteres coinciden).
fn same_secret(a: &str, b: &str) -> bool {
    let (a, b) = (a.as_bytes(), b.as_bytes());
    let mut diff = (a.len() ^ b.len()) as u8;
    for i in 0..a.len().max(b.len()) {
        diff |= a.get(i).copied().unwrap_or(0) ^ b.get(i).copied().unwrap_or(0xff);
    }
    diff == 0
}

fn authorized(req: &Request, env: &Env) -> bool {
    let Ok(secret) = env.secret("API_TOKEN") else {
        return false;
    };
    let header = req.headers().get("Authorization").ok().flatten().unwrap_or_default();
    match header.strip_prefix("Bearer ") {
        Some(token) => same_secret(token.trim(), &secret.to_string()),
        None => false,
    }
}

/// «Hoy» del usuario: `?today=` si viene, si no la hora actual con `TZ_OFFSET_MINUTES`
/// (Ciudad de México = -360, sin horario de verano).
fn today(url: &Url, env: &Env) -> NaiveDate {
    if let Some(d) = query_param(url, "today").and_then(|s| s.parse().ok()) {
        return d;
    }
    let offset: i64 = env
        .var("TZ_OFFSET_MINUTES")
        .ok()
        .and_then(|v| v.to_string().parse().ok())
        .unwrap_or(0);
    let now = chrono::DateTime::from_timestamp_millis(Date::now().as_millis() as i64).unwrap_or_default();
    (now + Duration::minutes(offset.clamp(-1440, 1440))).date_naive()
}

/// Parámetro de la URL; acepta también su nombre anterior en español.
fn query_param(url: &Url, key: &str) -> Option<String> {
    let alias = match key {
        "view" => "vista",
        "all" => "todas",
        "today" => "hoy",
        _ => key,
    };
    url.query_pairs().find(|(k, _)| k == key || k == alias).map(|(_, v)| v.into_owned())
}

/// Id corto de 5 caracteres, como en los `.md` (`🆔 a1b2c`). No es secreto.
fn new_id() -> String {
    const ALPHA: &[u8] = b"0123456789abcdefghijklmnopqrstuvwxyz";
    (0..5)
        .map(|_| ALPHA[(js_sys::Math::random() * 36.0) as usize % 36] as char)
        .collect()
}

async fn unique_id(db: &Db) -> Result<String> {
    loop {
        let id = new_id();
        if !db.exists(&id).await? {
            return Ok(id);
        }
    }
}

fn db(env: &Env) -> Result<Db> {
    Ok(Db::new(env.d1("DB")?, Date::now().as_millis()))
}

#[derive(Serialize)]
struct GroupOut {
    title: Option<String>,
    tasks: Vec<TaskDto>,
}

async fn list(req: &Request, env: &Env) -> ApiResult {
    let url = req.url()?;
    let name = query_param(&url, "view").unwrap_or_else(|| "today".into());
    let view = View::parse(&name).ok_or_else(|| {
        bad(format!("Vista desconocida «{name}». Usa: today, inbox, upcoming, all, +Proyecto, #etiqueta"))
    })?;
    let show_done = query_param(&url, "all").is_some_and(|v| v == "1" || v == "true");
    let q = query_param(&url, "q").unwrap_or_default();
    let today = today(&url, env);
    let db = db(env)?;
    let tasks = db.all().await?;
    let projects = db.projects().await?;
    let indexed: Vec<(usize, &Task)> = tasks.iter().enumerate().collect();
    let groups: Vec<GroupOut> = views::build(&indexed, &projects, &view, today, show_done, &q)
        .into_iter()
        .map(|g| GroupOut {
            title: g.title,
            tasks: g.items.iter().map(|&i| TaskDto::from(&tasks[i])).collect(),
        })
        .collect();
    json_ok(&json!({ "view": view.title(), "today": today, "groups": groups }))
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

async fn add(mut req: Request, env: &Env) -> ApiResult {
    let body: AddBody = req.json().await.map_err(|_| bad("Se esperaba {\"text\": \"…\"}"))?;
    let today = today(&req.url()?, env);
    let q = parse_quick(&body.text, today);
    if q.text.is_empty() {
        return Err(bad("La tarea necesita texto"));
    }
    let db = db(env)?;
    let mut t = q.into_task(body.project.as_deref().unwrap_or(INBOX));
    t.description = body.description.trim().to_string();
    t.project = db.canonical_project(&t.project).await?;
    t.id = Some(unique_id(&db).await?);
    db.add_project(&t.project).await?;
    db.save(&t).await?;
    Ok(Response::from_json(&TaskDto::from(&t))?.with_status(201))
}

#[derive(Deserialize)]
struct EditBody {
    /// Captura rápida. Por defecto cambia solo lo escrito.
    #[serde(alias = "texto")]
    text: Option<String>,
    /// `true`: reemplaza todo con `text` (como editar la línea en la TUI).
    #[serde(default, alias = "completo")]
    full: bool,
    /// Quita fecha, hora y repetición.
    #[serde(default, alias = "sin_fecha")]
    no_date: bool,
    /// Quita la fecha límite.
    #[serde(default)]
    no_deadline: bool,
    /// Reemplaza la descripción.
    description: Option<String>,
}

async fn edit(mut req: Request, env: &Env, id: &str) -> ApiResult {
    let body: EditBody = req.json().await.map_err(|_| bad("Cuerpo JSON inválido"))?;
    let today = today(&req.url()?, env);
    let db = db(env)?;
    let mut t = db.get(id).await?.ok_or_else(|| not_found(id))?;
    if let Some(text) = &body.text {
        let q = parse_quick(text, today);
        if body.full {
            if q.text.is_empty() {
                return Err(bad("La tarea necesita texto"));
            }
            q.apply_full(&mut t);
        } else {
            q.apply_partial(&mut t);
        }
    }
    if body.no_date {
        t.due = None;
        t.time = None;
        t.recurrence = None;
    }
    if body.no_deadline {
        t.deadline = None;
    }
    if let Some(d) = &body.description {
        t.description = d.trim().to_string();
    }
    t.project = db.canonical_project(&t.project).await?;
    db.add_project(&t.project).await?;
    db.save(&t).await?;
    json_ok(&TaskDto::from(&t))
}

/// Crea o reemplaza la tarea completa con el id de la ruta.
async fn put(mut req: Request, env: &Env, id: &str) -> ApiResult {
    let dto: TaskDto = req.json().await.map_err(|_| bad("Se esperaba una tarea en JSON"))?;
    let mut t = Task::from(&dto);
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
    let db = db(env)?;
    t.project = db.canonical_project(&t.project).await?;
    db.add_project(&t.project).await?;
    db.save(&t).await?;
    json_ok(&TaskDto::from(&t))
}

async fn toggle(req: &Request, env: &Env, id: &str) -> ApiResult {
    let today = today(&req.url()?, env);
    let db = db(env)?;
    let t = db.get(id).await?.ok_or_else(|| not_found(id))?;
    if t.done {
        let mut t = t;
        t.set_done(false, today);
        db.save(&t).await?;
        return json_ok(&json!({ "task": TaskDto::from(&t), "completed": false }));
    }
    match t.complete(today, unique_id(&db).await?) {
        Completion::Simple(done) => {
            db.save(&done).await?;
            json_ok(&json!({ "task": TaskDto::from(&done), "completed": true }))
        }
        Completion::Recurring { next, done } => {
            db.save(&done).await?;
            db.save(&next).await?;
            json_ok(&json!({
                "task": TaskDto::from(&next),
                "completed": true,
                "occurrence": TaskDto::from(&done),
            }))
        }
    }
}

#[derive(Deserialize)]
struct NoteBody {
    #[serde(alias = "texto")]
    text: String,
}

async fn add_note(mut req: Request, env: &Env, id: &str) -> ApiResult {
    let body: NoteBody = req.json().await.map_err(|_| bad("Se esperaba {\"text\": \"…\"}"))?;
    if body.text.trim().is_empty() {
        return Err(bad("La nota está vacía"));
    }
    let today = today(&req.url()?, env);
    let db = db(env)?;
    let mut t = db.get(id).await?.ok_or_else(|| not_found(id))?;
    t.notes.push(Note::new(&body.text, Some(today)));
    db.save(&t).await?;
    json_ok(&TaskDto::from(&t))
}

async fn projects(env: &Env) -> ApiResult {
    let db = db(env)?;
    let tasks = db.all().await?;
    let out: Vec<_> = db
        .projects()
        .await?
        .into_iter()
        .map(|p| {
            let open = tasks.iter().filter(|t| !t.done && t.project.eq_ignore_ascii_case(&p)).count();
            json!({ "name": p, "open": open })
        })
        .collect();
    json_ok(&out)
}

#[derive(Deserialize)]
struct ProjectBody {
    #[serde(alias = "nombre")]
    name: String,
}

async fn create_project(mut req: Request, env: &Env) -> ApiResult {
    let body: ProjectBody = req.json().await.map_err(|_| bad("Se esperaba {\"name\": \"…\"}"))?;
    let name = body.name.trim();
    if name.is_empty() {
        return Err(bad("El proyecto necesita nombre"));
    }
    db(env)?.add_project(name).await?;
    Ok(Response::from_json(&json!({ "name": name }))?.with_status(201))
}

async fn labels(env: &Env) -> ApiResult {
    let tasks = db(env)?.all().await?;
    let mut m: std::collections::BTreeMap<String, usize> = Default::default();
    for t in tasks.iter().filter(|t| !t.done) {
        for l in &t.labels {
            *m.entry(l.clone()).or_default() += 1;
        }
    }
    let out: Vec<_> = m.into_iter().map(|(l, n)| json!({ "name": l, "open": n })).collect();
    json_ok(&out)
}

async fn export(env: &Env) -> ApiResult {
    let db = db(env)?;
    let tasks: Vec<TaskDto> = db.all().await?.iter().map(TaskDto::from).collect();
    json_ok(&json!({ "projects": db.projects().await?, "tasks": tasks }))
}

#[derive(Deserialize)]
struct ImportBody {
    #[serde(default, alias = "proyectos")]
    projects: Vec<String>,
    #[serde(alias = "tareas")]
    tasks: Vec<TaskDto>,
}

async fn import(mut req: Request, env: &Env) -> ApiResult {
    let body: ImportBody = req.json().await.map_err(|_| bad("Se esperaba {\"tasks\": […]}"))?;
    let db = db(env)?;
    for p in &body.projects {
        db.add_project(p).await?;
    }
    let mut n = 0;
    for dto in &body.tasks {
        let mut t = Task::from(dto);
        if t.id.is_none() {
            t.id = Some(unique_id(&db).await?);
        }
        if t.project.is_empty() {
            t.project = INBOX.into();
        }
        db.add_project(&t.project).await?;
        db.save(&t).await?;
        n += 1;
    }
    json_ok(&json!({ "imported": n }))
}

async fn route(req: Request, env: Env) -> ApiResult {
    let path = req.path();
    let method = req.method();
    if let Some(secret) = path.strip_prefix("/mcp/") {
        let ok = env.secret("MCP_SECRET").map(|s| same_secret(secret.trim_end_matches('/'), &s.to_string())).unwrap_or(false);
        if !ok {
            return Err(ApiError(404, "No encontrado".into()));
        }
        if method != Method::Post {
            return Err(ApiError(405, "Usa POST".into()));
        }
        return Ok(mcp::serve(req, &env).await?);
    }
    if path == "/api/health" {
        return json_ok(&json!({ "ok": true, "version": env!("CARGO_PKG_VERSION") }));
    }
    if !path.starts_with("/api/") {
        return Err(ApiError(404, "No encontrado".into()));
    }
    if !authorized(&req, &env) {
        return Err(ApiError(401, "Falta el token o no es válido".into()));
    }
    route_inner(req, env).await
}

/// Las rutas de la API sin comprobar el token (ya validado, o llamada interna del MCP).
async fn route_inner(req: Request, env: Env) -> ApiResult {
    let path = req.path();
    let method = req.method();
    let parts: Vec<&str> = path.trim_start_matches("/api/").trim_end_matches('/').split('/').collect();
    match (method, parts.as_slice()) {
        (Method::Get, ["tasks"]) => list(&req, &env).await,
        (Method::Post, ["tasks"]) => add(req, &env).await,
        (Method::Get, ["tasks", id]) => {
            let t = db(&env)?.get(id).await?.ok_or_else(|| not_found(id))?;
            json_ok(&TaskDto::from(&t))
        }
        (Method::Patch, ["tasks", id]) => {
            let id = id.to_string();
            edit(req, &env, &id).await
        }
        (Method::Put, ["tasks", id]) => {
            let id = id.to_string();
            put(req, &env, &id).await
        }
        (Method::Delete, ["tasks", id]) => {
            if db(&env)?.delete(id).await? {
                json_ok(&json!({ "deleted": id }))
            } else {
                Err(not_found(id))
            }
        }
        (Method::Post, ["tasks", id, "done"]) => toggle(&req, &env, id).await,
        (Method::Post, ["tasks", id, "notes"]) => {
            let id = id.to_string();
            add_note(req, &env, &id).await
        }
        (Method::Get, ["projects"]) => projects(&env).await,
        (Method::Post, ["projects"]) => create_project(req, &env).await,
        (Method::Get, ["labels"]) => labels(&env).await,
        (Method::Get, ["export"]) => export(&env).await,
        (Method::Post, ["import"]) => import(req, &env).await,
        _ => Err(ApiError(404, "Ruta no encontrada".into())),
    }
}

#[event(fetch)]
async fn fetch(req: Request, env: Env, _ctx: Context) -> Result<Response> {
    match route(req, env).await {
        Ok(r) => Ok(r),
        Err(ApiError(status, msg)) => Ok(Response::from_json(&json!({ "error": msg }))?.with_status(status)),
    }
}
