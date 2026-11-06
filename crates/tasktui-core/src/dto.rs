//! La tarea en JSON: lo que devuelve `tasktui --json` y lo que intercambia la API.

use chrono::{NaiveDate, NaiveTime};
use serde::{Deserialize, Deserializer, Serialize, Serializer};

use crate::model::{Note, Recurrence, Task};

/// Hora como `"18:00"`.
mod hhmm {
    use super::*;

    pub fn serialize<S: Serializer>(t: &Option<NaiveTime>, s: S) -> Result<S::Ok, S::Error> {
        match t {
            Some(t) => s.serialize_str(&t.format("%H:%M").to_string()),
            None => s.serialize_none(),
        }
    }

    pub fn deserialize<'de, D: Deserializer<'de>>(d: D) -> Result<Option<NaiveTime>, D::Error> {
        let v: Option<String> = Option::deserialize(d)?;
        v.map(|v| {
            NaiveTime::parse_from_str(&v, "%H:%M")
                .or_else(|_| NaiveTime::parse_from_str(&v, "%H:%M:%S"))
                .map_err(serde::de::Error::custom)
        })
        .transpose()
    }
}

/// Comentario de seguimiento.
#[derive(Debug, Clone, PartialEq, Serialize)]
pub struct NoteDto {
    pub date: Option<NaiveDate>,
    pub text: String,
}

/// Acepta también el formato anterior (solo el texto).
impl<'de> Deserialize<'de> for NoteDto {
    fn deserialize<D: Deserializer<'de>>(d: D) -> Result<Self, D::Error> {
        #[derive(Deserialize)]
        #[serde(untagged)]
        enum Any {
            Text(String),
            Full { date: Option<NaiveDate>, text: String },
        }
        Ok(match Any::deserialize(d)? {
            Any::Text(text) => NoteDto { date: None, text },
            Any::Full { date, text } => NoteDto { date, text },
        })
    }
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct TaskDto {
    pub id: Option<String>,
    pub text: String,
    pub done: bool,
    /// Fecha (`AAAA-MM-DD`): cuándo piensas hacerla.
    pub due: Option<NaiveDate>,
    /// Hora de `due` (`"18:00"`).
    #[serde(default, with = "hhmm")]
    pub time: Option<NaiveTime>,
    /// Fecha límite (`AAAA-MM-DD`): cuándo debe estar hecha.
    #[serde(default)]
    pub deadline: Option<NaiveDate>,
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
    pub description: String,
    #[serde(default)]
    pub notes: Vec<NoteDto>,
}

impl From<&Task> for TaskDto {
    fn from(t: &Task) -> Self {
        TaskDto {
            id: t.id.clone(),
            text: t.text.clone(),
            done: t.done,
            due: t.due,
            time: t.time,
            deadline: t.deadline,
            done_date: t.done_date,
            priority: t.priority,
            labels: t.labels.clone(),
            recurrence: t.recurrence.as_ref().map(|r| r.to_es()),
            recurrence_rule: t.recurrence.as_ref().map(|r| r.to_obsidian()),
            project: t.project.clone(),
            section: t.section.clone(),
            description: t.description.clone(),
            notes: t.notes.iter().map(|n| NoteDto { date: n.date, text: n.text.clone() }).collect(),
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
        t.time = d.time;
        t.deadline = d.deadline;
        t.done_date = d.done_date;
        t.priority = d.priority.clamp(1, 4);
        t.labels = d.labels.clone();
        t.recurrence = d.recurrence_rule.as_deref().and_then(Recurrence::parse_obsidian);
        t.project = d.project.clone();
        t.section = d.section.clone();
        t.description = d.description.clone();
        t.notes = d.notes.iter().map(|n| Note { date: n.date, text: n.text.clone() }).collect();
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
        t.time = NaiveTime::from_hms_opt(18, 0, 0);
        t.deadline = NaiveDate::from_ymd_opt(2026, 11, 20);
        t.description = "Recibo de CFE\nPagar en línea".into();
        t.notes = vec![Note::new("ya llegó el recibo", NaiveDate::from_ymd_opt(2026, 11, 6))];
        let dto = TaskDto::from(&t);
        let json = serde_json::to_string(&dto).unwrap();
        assert!(json.contains(r#""due":"2026-11-06""#), "{json}");
        assert!(json.contains(r#""recurrence":"cada mes""#), "{json}");
        assert!(json.contains(r#""time":"18:00""#), "{json}");
        assert!(json.contains(r#""notes":[{"date":"2026-11-06","text":"ya llegó el recibo"}]"#), "{json}");
        let back: TaskDto = serde_json::from_str(&json).unwrap();
        let t2 = Task::from(&back);
        assert_eq!(t2.recurrence, t.recurrence);
        assert_eq!(TaskDto::from(&t2), dto);
    }
}

#[cfg(test)]
mod compat {
    use super::*;

    #[test]
    fn notas_en_formato_anterior() {
        let d: TaskDto = serde_json::from_str(
            r#"{"id":"a","text":"x","done":false,"due":null,"priority":4,"recurrence":null,"project":"Inbox","section":null,"notes":["vieja"]}"#,
        )
        .unwrap();
        assert_eq!(d.notes, vec![NoteDto { date: None, text: "vieja".into() }]);
        assert_eq!((d.time, d.deadline, d.description.as_str()), (None, None, ""));
    }
}
