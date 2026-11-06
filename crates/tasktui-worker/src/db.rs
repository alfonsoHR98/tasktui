//! Acceso a D1: filas ↔ `Task` (vía `TaskDto`, el mismo contrato que la API).

use serde::Deserialize;
use tasktui_core::dto::TaskDto;
use tasktui_core::model::Task;
use tasktui_core::INBOX;
use worker::d1::D1Database;
use worker::{query, Result};

#[derive(Deserialize)]
struct Row {
    id: String,
    text: String,
    done: f64,
    due: Option<String>,
    time: Option<String>,
    deadline: Option<String>,
    done_date: Option<String>,
    priority: f64,
    labels: String,
    recurrence: Option<String>,
    project: String,
    section: Option<String>,
    description: String,
    notes: String,
    position: f64,
}

impl Row {
    fn into_task(self) -> (Task, f64) {
        let date = |s: Option<String>| s.and_then(|s| s.parse().ok());
        let dto = TaskDto {
            id: Some(self.id),
            text: self.text,
            done: self.done != 0.0,
            due: date(self.due),
            time: self.time.and_then(|t| chrono::NaiveTime::parse_from_str(&t, "%H:%M").ok()),
            deadline: date(self.deadline),
            done_date: date(self.done_date),
            priority: self.priority as u8,
            labels: serde_json::from_str(&self.labels).unwrap_or_default(),
            recurrence: None,
            recurrence_rule: self.recurrence,
            project: self.project,
            section: self.section,
            description: self.description,
            notes: serde_json::from_str(&self.notes).unwrap_or_default(),
        };
        (Task::from(&dto), self.position)
    }
}

pub struct Db {
    db: D1Database,
    now: f64,
}

impl Db {
    pub fn new(db: D1Database, now_ms: u64) -> Self {
        Db { db, now: now_ms as f64 }
    }

    /// Todas las tareas, en orden (proyecto, posición).
    pub async fn all(&self) -> Result<Vec<Task>> {
        let rows = query!(&self.db, "SELECT * FROM tasks ORDER BY project COLLATE NOCASE, position")
            .all()
            .await?
            .results::<Row>()?;
        Ok(rows.into_iter().map(|r| r.into_task().0).collect())
    }

    pub async fn get(&self, id: &str) -> Result<Option<Task>> {
        let row = query!(&self.db, "SELECT * FROM tasks WHERE id = ?1", id)?
            .first::<Row>(None)
            .await?;
        Ok(row.map(|r| r.into_task().0))
    }

    pub async fn exists(&self, id: &str) -> Result<bool> {
        Ok(query!(&self.db, "SELECT 1 AS x FROM tasks WHERE id = ?1", id)?
            .first::<serde_json::Value>(None)
            .await?
            .is_some())
    }

    /// Proyectos (los explícitos y los que tienen tareas): Inbox primero.
    pub async fn projects(&self) -> Result<Vec<String>> {
        #[derive(Deserialize)]
        struct P {
            name: String,
        }
        let rows = query!(
            &self.db,
            "SELECT name FROM projects UNION SELECT DISTINCT project FROM tasks"
        )
        .all()
        .await?
        .results::<P>()?;
        let mut ps: Vec<String> = rows.into_iter().map(|p| p.name).collect();
        ps.sort_by_key(|p| (p != INBOX, p.to_lowercase()));
        ps.dedup_by(|a, b| a.eq_ignore_ascii_case(b));
        Ok(ps)
    }

    pub async fn add_project(&self, name: &str) -> Result<()> {
        query!(
            &self.db,
            "INSERT OR IGNORE INTO projects (name, created_at) VALUES (?1, ?2)",
            name,
            self.now
        )?
        .run()
        .await?;
        Ok(())
    }

    /// Nombre del proyecto tal como existe (sin distinguir mayúsculas), o el dado.
    pub async fn canonical_project(&self, name: &str) -> Result<String> {
        let found = self
            .projects()
            .await?
            .into_iter()
            .find(|p| p.eq_ignore_ascii_case(name));
        Ok(found.unwrap_or_else(|| name.to_string()))
    }

    async fn next_position(&self, project: &str) -> Result<f64> {
        #[derive(Deserialize)]
        struct M {
            m: Option<f64>,
        }
        let m = query!(
            &self.db,
            "SELECT MAX(position) AS m FROM tasks WHERE project = ?1",
            project
        )?
        .first::<M>(None)
        .await?;
        Ok(m.and_then(|m| m.m).unwrap_or(0.0) + 1.0)
    }

    /// Inserta o reemplaza la tarea. Al final de su proyecto si es nueva o
    /// cambió de proyecto; si no, conserva su posición.
    pub async fn save(&self, t: &Task) -> Result<()> {
        let id = t.id.as_deref().unwrap_or_default();
        #[derive(Deserialize)]
        struct Cur {
            project: String,
            position: f64,
            created_at: f64,
        }
        let cur = query!(
            &self.db,
            "SELECT project, position, created_at FROM tasks WHERE id = ?1",
            id
        )?
        .first::<Cur>(None)
        .await?;
        let (position, created) = match cur {
            Some(c) if c.project == t.project => (c.position, c.created_at),
            Some(c) => (self.next_position(&t.project).await?, c.created_at),
            None => (self.next_position(&t.project).await?, self.now),
        };
        let d = TaskDto::from(t);
        query!(
            &self.db,
            "INSERT OR REPLACE INTO tasks
               (id, text, done, due, done_date, priority, labels, recurrence,
                project, section, notes, position, created_at, updated_at,
                description, time, deadline)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17)",
            id,
            d.text,
            d.done as i32,
            d.due.map(|x| x.to_string()),
            d.done_date.map(|x| x.to_string()),
            d.priority as i32,
            serde_json::to_string(&d.labels).unwrap_or_default(),
            d.recurrence_rule,
            d.project,
            d.section,
            serde_json::to_string(&d.notes).unwrap_or_default(),
            position,
            created,
            self.now,
            d.description,
            d.time.map(|t| t.format("%H:%M").to_string()),
            d.deadline.map(|x| x.to_string())
        )?
        .run()
        .await?;
        Ok(())
    }

    pub async fn delete(&self, id: &str) -> Result<bool> {
        let r = query!(&self.db, "DELETE FROM tasks WHERE id = ?1", id)?.run().await?;
        Ok(r.meta()?.and_then(|m| m.changes).unwrap_or(0) > 0)
    }
}
