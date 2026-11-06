//! Almacén remoto: el servidor de tasktui (`crates/tasktui-worker`).
//!
//! Tiene la misma interfaz que el almacén de archivos. Cada cambio va directo
//! al servidor y después se vuelve a leer todo (son pocas tareas). La última
//! lectura se guarda en caché: sin internet se pueden ver las tareas, pero los
//! cambios se rechazan con un mensaje claro en vez de perderse.

use std::collections::HashSet;
use std::fs;
use std::path::PathBuf;
use std::time::Duration;

use anyhow::{anyhow, bail, Context, Result};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use tasktui_core::dto::TaskDto;

use crate::config::Server;
use crate::files::gen_id;
use crate::model::Task;

/// Lo que devuelve `GET /api/export`.
#[derive(Default, Serialize, Deserialize)]
pub struct Export {
    pub proyectos: Vec<String>,
    pub tareas: Vec<TaskDto>,
}

pub struct Client {
    base: String,
    token: String,
    agent: ureq::Agent,
}

type Reply = std::result::Result<ureq::http::Response<ureq::Body>, ureq::Error>;

/// No se pudo hablar con el servidor (red, DNS, tiempo agotado). Solo en este
/// caso se usa la copia en caché; un token inválido u otro error del servidor
/// se informa tal cual.
#[derive(Debug)]
pub struct NetError(pub String);

impl std::fmt::Display for NetError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str(&self.0)
    }
}

impl std::error::Error for NetError {}

impl Client {
    pub fn new(server: &Server) -> Self {
        let agent: ureq::Agent = ureq::Agent::config_builder()
            .timeout_global(Some(Duration::from_secs(15)))
            .http_status_as_error(false)
            .build()
            .into();
        Client {
            base: server.url.trim_end_matches('/').to_string(),
            token: server.token.clone(),
            agent,
        }
    }

    pub fn url(&self) -> &str {
        &self.base
    }

    fn endpoint(&self, path: &str) -> String {
        format!("{}/api/{path}", self.base)
    }

    fn auth(&self) -> String {
        format!("Bearer {}", self.token)
    }

    fn finish(&self, reply: Reply) -> Result<Value> {
        let resp = reply.map_err(|e| NetError(format!("Sin conexión con {}: {e}", self.base)))?;
        let status = resp.status().as_u16();
        let v: Value = resp.into_body().read_json().unwrap_or(Value::Null);
        match status {
            200..=299 => Ok(v),
            401 => bail!("El servidor rechazó el token. Vuelve a conectar con `tasktui servidor conectar`."),
            _ => bail!(
                "{}",
                v.get("error").and_then(Value::as_str).map(String::from).unwrap_or(format!("Error {status} del servidor"))
            ),
        }
    }

    pub fn get(&self, path: &str) -> Result<Value> {
        self.finish(self.agent.get(self.endpoint(path)).header("Authorization", self.auth()).call())
    }

    pub fn post(&self, path: &str, body: Option<Value>) -> Result<Value> {
        let req = self.agent.post(self.endpoint(path)).header("Authorization", self.auth());
        self.finish(match body {
            Some(b) => req.send_json(b),
            None => req.send_empty(),
        })
    }

    pub fn put(&self, path: &str, body: Value) -> Result<Value> {
        self.finish(self.agent.put(self.endpoint(path)).header("Authorization", self.auth()).send_json(body))
    }

    pub fn delete(&self, path: &str) -> Result<Value> {
        self.finish(self.agent.delete(self.endpoint(path)).header("Authorization", self.auth()).call())
    }

    /// Todas las tareas y proyectos.
    pub fn export(&self) -> Result<Export> {
        Ok(serde_json::from_value(self.get("export")?)?)
    }

    /// Sube tareas conservando sus ids (no borra nada en el servidor).
    pub fn import(&self, proyectos: &[String], tareas: &[TaskDto]) -> Result<u64> {
        let v = self.post("import", Some(json!({ "proyectos": proyectos, "tareas": tareas })))?;
        Ok(v.get("importadas").and_then(Value::as_u64).unwrap_or(0))
    }
}

pub struct RemoteStore {
    client: Client,
    tasks: Vec<Task>,
    projects: Vec<String>,
    cache: PathBuf,
    /// La última lectura falló: se muestran datos de la caché.
    pub offline: bool,
}

/// Una caché por servidor: `remoto-tasks.ejemplo.com.json`.
fn cache_path(url: &str) -> PathBuf {
    let host: String = url
        .split("://")
        .last()
        .unwrap_or(url)
        .chars()
        .map(|c| if c.is_ascii_alphanumeric() || c == '.' || c == '-' { c } else { '_' })
        .collect();
    dirs::cache_dir()
        .unwrap_or_else(std::env::temp_dir)
        .join(format!("tasktui/remoto-{}.json", host.trim_end_matches('_')))
}

impl RemoteStore {
    /// Abre el servidor. Si no responde pero hay caché, abre en modo sin conexión.
    pub fn open(server: &Server) -> Result<Self> {
        let mut s = RemoteStore {
            client: Client::new(server),
            tasks: vec![],
            projects: vec![],
            cache: cache_path(&server.url),
            offline: false,
        };
        if let Err(e) = s.reload() {
            if e.downcast_ref::<NetError>().is_none() {
                return Err(e);
            }
            let cached = fs::read_to_string(&s.cache)
                .ok()
                .and_then(|c| serde_json::from_str::<Export>(&c).ok())
                .ok_or(e)?;
            s.set(cached);
            s.offline = true;
        }
        Ok(s)
    }

    pub fn url(&self) -> &str {
        self.client.url()
    }

    fn set(&mut self, ex: Export) {
        self.tasks = ex.tareas.iter().map(Task::from).collect();
        self.projects = ex.proyectos;
    }

    pub fn reload(&mut self) -> Result<()> {
        let ex = self.client.export()?;
        if let Some(dir) = self.cache.parent() {
            let _ = fs::create_dir_all(dir);
        }
        let _ = fs::write(&self.cache, serde_json::to_string(&ex)?);
        self.set(ex);
        self.offline = false;
        Ok(())
    }

    pub fn tasks(&self) -> &[Task] {
        &self.tasks
    }

    pub fn projects(&self) -> Vec<String> {
        self.projects.clone()
    }

    pub fn find(&self, id: &str) -> Option<usize> {
        self.tasks.iter().position(|t| t.id.as_deref() == Some(id))
    }

    fn get_or_err(&self, id: &str) -> Result<Task> {
        self.find(id)
            .map(|i| self.tasks[i].clone())
            .ok_or_else(|| anyhow!("No existe la tarea {id}"))
    }

    fn put(&self, t: &Task) -> Result<()> {
        let id = t.id.as_deref().context("La tarea no tiene id")?;
        self.client.put(&format!("tasks/{id}"), serde_json::to_value(TaskDto::from(t))?)?;
        Ok(())
    }

    pub fn add(&mut self, mut task: Task, project: &str) -> Result<String> {
        self.reload()?;
        let ids: HashSet<String> = self.tasks.iter().filter_map(|t| t.id.clone()).collect();
        let id = gen_id(&ids);
        task.id = Some(id.clone());
        task.project = project.to_string();
        self.put(&task)?;
        self.reload()?;
        Ok(id)
    }

    pub fn update(&mut self, id: &str, f: impl FnOnce(&mut Task)) -> Result<()> {
        self.reload()?;
        let mut t = self.get_or_err(id)?;
        f(&mut t);
        self.put(&t)?;
        self.reload()
    }

    pub fn toggle(&mut self, id: &str) -> Result<bool> {
        let v = self.client.post(&format!("tasks/{id}/done"), None)?;
        self.reload()?;
        Ok(v.get("completada").and_then(Value::as_bool).unwrap_or(false))
    }

    pub fn delete(&mut self, id: &str) -> Result<()> {
        self.client.delete(&format!("tasks/{id}"))?;
        self.reload()
    }

    pub fn add_note(&mut self, id: &str, text: &str) -> Result<()> {
        if text.trim().is_empty() {
            bail!("La nota está vacía");
        }
        self.client.post(&format!("tasks/{id}/notes"), Some(json!({ "texto": text })))?;
        self.reload()
    }

    pub fn snapshot(&self) -> Vec<Task> {
        self.tasks.clone()
    }

    /// Deja el servidor como estaba en `snap`: repone lo cambiado o borrado y
    /// quita lo que se creó después.
    pub fn restore(&mut self, snap: &[Task]) -> Result<()> {
        self.reload()?;
        for t in snap {
            let current = t.id.as_deref().and_then(|id| self.find(id)).map(|i| &self.tasks[i]);
            if current != Some(t) {
                self.put(t)?;
            }
        }
        let keep: HashSet<&str> = snap.iter().filter_map(|t| t.id.as_deref()).collect();
        for t in &self.tasks {
            if let Some(id) = t.id.as_deref().filter(|id| !keep.contains(id)) {
                self.client.delete(&format!("tasks/{id}"))?;
            }
        }
        self.reload()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Prueba contra un servidor real. Solo corre con `TASKTUI_SERVER` y
    /// `TASKTUI_TOKEN` definidos (p. ej. `npx wrangler dev` en local):
    ///
    /// ```sh
    /// TASKTUI_SERVER=http://localhost:8787 TASKTUI_TOKEN=… cargo test -p tasktui remoto
    /// ```
    #[test]
    fn remoto_deshacer_restaura_el_servidor() {
        let Some(server) = crate::config::server().filter(|_| std::env::var("TASKTUI_SERVER").is_ok()) else {
            eprintln!("(omitida: define TASKTUI_SERVER y TASKTUI_TOKEN)");
            return;
        };
        let mut s = RemoteStore::open(&server).unwrap();
        assert!(!s.offline);
        let antes = s.snapshot();

        // Varios cambios: añadir, editar, nota, completar y borrar.
        let mut t = Task::new("Prueba de deshacer");
        t.priority = 2;
        let nueva = s.add(t, "Pruebas").unwrap();
        s.update(&nueva, |t| t.text = "Prueba editada".into()).unwrap();
        s.add_note(&nueva, "una nota").unwrap();
        assert!(s.toggle(&nueva).unwrap());
        let t = &s.tasks()[s.find(&nueva).unwrap()];
        assert_eq!((t.text.as_str(), t.done, t.notes.len()), ("Prueba editada", true, 1));
        if let Some(otra) = antes.iter().find(|t| !t.done).and_then(|t| t.id.clone()) {
            s.delete(&otra).unwrap();
            assert!(s.find(&otra).is_none());
        }

        // Deshacer todo de golpe deja el servidor como estaba.
        s.restore(&antes).unwrap();
        let mut a: Vec<_> = antes.iter().map(TaskDto::from).map(|d| serde_json::to_string(&d).unwrap()).collect();
        let mut b: Vec<_> = s.tasks().iter().map(TaskDto::from).map(|d| serde_json::to_string(&d).unwrap()).collect();
        a.sort();
        b.sort();
        assert_eq!(a, b);
    }
}
