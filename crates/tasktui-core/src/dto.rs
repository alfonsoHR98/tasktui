//! La tarea en JSON: lo que devuelve `tasktui --json` y lo que intercambia la API.

use chrono::NaiveDate;
use serde::{Deserialize, Serialize};

use crate::model::{Recurrence, Task};

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct TaskDto {
    pub id: Option<String>,
    pub text: String,
    pub done: bool,
    /// `AAAA-MM-DD`.
    pub due: Option<NaiveDate>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub done_date: Option<NaiveDate>,
    /// 1 (máxima) … 4 (sin prioridad).
    pub priority: u8,
    #[serde(default)]
    pub labels: Vec<String>,
    /// En español, como se escribe en la captura rápida: «cada lunes».
    pub recurrence: Option<String>,
    /// La misma regla en el formato de Obsidian Tasks: «every week on Monday».
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub recurrence_rule: Option<String>,
    pub project: String,
    pub section: Option<String>,
    #[serde(default)]
    pub notes: Vec<String>,
}

impl From<&Task> for TaskDto {
    fn from(t: &Task) -> Self {
        TaskDto {
            id: t.id.clone(),
            text: t.text.clone(),
            done: t.done,
            due: t.due,
            done_date: t.done_date,
            priority: t.priority,
            labels: t.labels.clone(),
            recurrence: t.recurrence.as_ref().map(|r| r.to_es()),
            recurrence_rule: t.recurrence.as_ref().map(|r| r.to_obsidian()),
            project: t.project.clone(),
            section: t.section.clone(),
            notes: t.notes.clone(),
        }
    }
}

impl From<&TaskDto> for Task {
    fn from(d: &TaskDto) -> Self {
        let mut t = Task::new(&d.text);
        t.id = d.id.clone();
        t.status = if d.done { 'x' } else { ' ' };
        t.done = d.done;
        t.due = d.due;
        t.done_date = d.done_date;
        t.priority = d.priority.clamp(1, 4);
        t.labels = d.labels.clone();
        t.recurrence = d.recurrence_rule.as_deref().and_then(Recurrence::parse_obsidian);
        t.project = d.project.clone();
        t.section = d.section.clone();
        t.notes = d.notes.clone();
        t
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::markdown::parse_line;

    #[test]
    fn ida_y_vuelta() {
        let mut t = parse_line("- [ ] Pagar luz #casa ⏫ 🔁 every month 📅 2026-11-06 🆔 a1b2c").unwrap();
        t.project = "Finanzas".into();
        t.notes = vec!["recibo en el correo".into()];
        let dto = TaskDto::from(&t);
        let json = serde_json::to_string(&dto).unwrap();
        assert!(json.contains(r#""due":"2026-11-06""#), "{json}");
        assert!(json.contains(r#""recurrence":"cada mes""#), "{json}");
        let back: TaskDto = serde_json::from_str(&json).unwrap();
        let t2 = Task::from(&back);
        assert_eq!(t2.recurrence, t.recurrence);
        assert_eq!(TaskDto::from(&t2), dto);
    }
}
