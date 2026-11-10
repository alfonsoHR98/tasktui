//! SQLite: filas ↔ `Task` (vía `TaskDto`, el mismo contrato que la API).
//! El esquema es el de las migraciones de D1 (`tasktui-worker/migrations`), así
//! que ambos servidores guardan los datos igual.

use rusqlite::{params, Connection, OptionalExtension};
use tasktui_core::dto::TaskDto;
use tasktui_core::model::Task;
use tasktui_core::INBOX;

/// Migraciones en orden; `PRAGMA user_version` guarda cuántas se aplicaron.
const MIGRATIONS: [&str; 2] = [
    include_str!("../../tasktui-worker/migrations/0001_init.sql"),
    include_str!("../../tasktui-worker/migrations/0002_descripcion_hora_limite.sql"),
];

pub type Result<T> = std::result::Result<T, rusqlite::Error>;

pub struct Db {
    conn: Connection,
}

fn now_ms() -> i64 {
    chrono::Utc::now().timestamp_millis()
}

fn task_from_row(r: &rusqlite::Row) -> Result<Task> {
    let date = |c: &str| -> Result<Option<chrono::NaiveDate>> {
        Ok(r.get::<_, Option<String>>(c)?.and_then(|s| s.parse().ok()))
    };
    let time: Option<String> = r.get("time")?;
    let labels: String = r.get("labels")?;
    let notes: String = r.get("notes")?;
    let dto = TaskDto {
        id: Some(r.get("id")?),
        text: r.get("text")?,
        done: r.get::<_, i64>("done")? != 0,
        due: date("due")?,
        time: time.and_then(|t| chrono::NaiveTime::parse_from_str(&t, "%H:%M").ok()),
        deadline: date("deadline")?,
        done_date: date("done_date")?,
        priority: r.get::<_, i64>("priority")? as u8,
        labels: serde_json::from_str(&labels).unwrap_or_default(),
        recurrence: None,
        recurrence_rule: r.get("recurrence")?,
        project: r.get("project")?,
        section: r.get("section")?,
        description: r.get("description")?,
        notes: serde_json::from_str(&notes).unwrap_or_default(),
    };
    Ok(Task::from(&dto))
}

impl Db {
    /// Abre (o crea) la base y aplica las migraciones pendientes.
    pub fn open(path: &str) -> Result<Self> {
        let conn = Connection::open(path)?;
        conn.pragma_update(None, "journal_mode", "WAL")?;
        conn.pragma_update(None, "busy_timeout", 5000)?;
        let applied: usize = conn.pragma_query_value(None, "user_version", |r| r.get(0))?;
        for (i, sql) in MIGRATIONS.iter().enumerate().skip(applied) {
            conn.execute_batch(&format!("BEGIN;\n{sql}\nPRAGMA user_version = {};\nCOMMIT;", i + 1))?;
        }
        Ok(Db { conn })
    }

    pub fn all(&self) -> Result<Vec<Task>> {
        let mut st = self
            .conn
            .prepare("SELECT * FROM tasks ORDER BY project COLLATE NOCASE, position")?;
        let rows = st.query_map([], task_from_row)?;
        rows.collect()
    }

    pub fn get(&self, id: &str) -> Result<Option<Task>> {
        self.conn
            .query_row("SELECT * FROM tasks WHERE id = ?1", [id], task_from_row)
            .optional()
    }

    pub fn exists(&self, id: &str) -> Result<bool> {
        Ok(self
            .conn
            .query_row("SELECT 1 FROM tasks WHERE id = ?1", [id], |_| Ok(()))
            .optional()?
            .is_some())
    }

    /// Proyectos (los explícitos y los que tienen tareas): Inbox primero.
    pub fn projects(&self) -> Result<Vec<String>> {
        let mut st = self
            .conn
            .prepare("SELECT name FROM projects UNION SELECT DISTINCT project FROM tasks")?;
        let mut ps: Vec<String> = st.query_map([], |r| r.get(0))?.collect::<Result<_>>()?;
        ps.sort_by_key(|p| (p != INBOX, p.to_lowercase()));
        ps.dedup_by(|a, b| a.eq_ignore_ascii_case(b));
        Ok(ps)
    }

    pub fn add_project(&self, name: &str) -> Result<()> {
        self.conn.execute(
            "INSERT OR IGNORE INTO projects (name, created_at) VALUES (?1, ?2)",
            params![name, now_ms()],
        )?;
        Ok(())
    }

    /// Nombre del proyecto tal como existe (sin distinguir mayúsculas), o el dado.
    pub fn canonical_project(&self, name: &str) -> Result<String> {
        let found = self.projects()?.into_iter().find(|p| p.eq_ignore_ascii_case(name));
        Ok(found.unwrap_or_else(|| name.to_string()))
    }

    fn next_position(&self, project: &str) -> Result<f64> {
        let m: Option<f64> = self.conn.query_row(
            "SELECT MAX(position) FROM tasks WHERE project = ?1",
            [project],
            |r| r.get(0),
        )?;
        Ok(m.unwrap_or(0.0) + 1.0)
    }

    /// Inserta o reemplaza la tarea. Al final de su proyecto si es nueva o
    /// cambió de proyecto; si no, conserva su posición.
    pub fn save(&self, t: &Task) -> Result<()> {
        let id = t.id.as_deref().unwrap_or_default();
        let cur: Option<(String, f64, i64)> = self
            .conn
            .query_row(
                "SELECT project, position, created_at FROM tasks WHERE id = ?1",
                [id],
                |r| Ok((r.get(0)?, r.get(1)?, r.get(2)?)),
            )
            .optional()?;
        let now = now_ms();
        let (position, created) = match cur {
            Some((p, pos, c)) if p == t.project => (pos, c),
            Some((_, _, c)) => (self.next_position(&t.project)?, c),
            None => (self.next_position(&t.project)?, now),
        };
        let d = TaskDto::from(t);
        self.conn.execute(
            "INSERT OR REPLACE INTO tasks
               (id, text, done, due, done_date, priority, labels, recurrence,
                project, section, notes, position, created_at, updated_at,
                description, time, deadline)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17)",
            params![
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
                now,
                d.description,
                d.time.map(|t| t.format("%H:%M").to_string()),
                d.deadline.map(|x| x.to_string()),
            ],
        )?;
        Ok(())
    }

    pub fn delete(&self, id: &str) -> Result<bool> {
        Ok(self.conn.execute("DELETE FROM tasks WHERE id = ?1", [id])? > 0)
    }
}
