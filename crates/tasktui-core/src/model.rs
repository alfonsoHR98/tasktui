//! Modelo de datos: tareas y recurrencias.

use chrono::{Datelike, Duration, Months, NaiveDate, NaiveTime, Weekday};

pub const WEEKDAYS: [Weekday; 7] = [
    Weekday::Mon,
    Weekday::Tue,
    Weekday::Wed,
    Weekday::Thu,
    Weekday::Fri,
    Weekday::Sat,
    Weekday::Sun,
];
const WD_EN: [&str; 7] = [
    "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday",
];
pub const WD_ES: [&str; 7] = [
    "lunes", "martes", "miércoles", "jueves", "viernes", "sábado", "domingo",
];

fn weekday_en(s: &str) -> Option<Weekday> {
    WD_EN
        .iter()
        .position(|w| w.eq_ignore_ascii_case(s))
        .map(|i| WEEKDAYS[i])
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Unit {
    Day,
    Week,
    Month,
    Year,
}

/// Regla de repetición. Se guarda en el formato del plugin Obsidian Tasks
/// (`every week on Monday`) y se muestra/captura en español (`cada lunes`).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Recurrence {
    pub interval: u32,
    pub unit: Unit,
    pub weekday: Option<Weekday>,
}

impl Recurrence {
    pub fn parse_obsidian(s: &str) -> Option<Self> {
        let s = s.trim().to_lowercase();
        let rest = s.strip_prefix("every")?.trim().to_string();
        let (main, on) = match rest.split_once(" on ") {
            Some((a, b)) => (a.trim().to_string(), Some(b.trim().to_string())),
            None => (rest.clone(), None),
        };
        let mut weekday = match on {
            Some(w) => Some(weekday_en(
                w.split(|c: char| c == ',' || c.is_whitespace())
                    .next()
                    .unwrap_or(""),
            )?),
            None => None,
        };
        let mut parts = main.split_whitespace();
        let first = parts.next()?;
        let (interval, unit_s) = match first.parse::<u32>() {
            Ok(n) if n > 0 => (n, parts.next()?),
            Ok(_) => return None,
            Err(_) => (1, first),
        };
        if parts.next().is_some() {
            return None;
        }
        let unit = if let Some(w) = weekday_en(unit_s) {
            weekday = Some(w);
            Unit::Week
        } else {
            match unit_s.trim_end_matches('s') {
                "day" => Unit::Day,
                "week" => Unit::Week,
                "month" => Unit::Month,
                "year" => Unit::Year,
                _ => return None,
            }
        };
        if weekday.is_some() && unit != Unit::Week {
            return None;
        }
        Some(Recurrence { interval, unit, weekday })
    }

    fn unit_en(&self) -> &'static str {
        match self.unit {
            Unit::Day => "day",
            Unit::Week => "week",
            Unit::Month => "month",
            Unit::Year => "year",
        }
    }

    pub fn to_obsidian(&self) -> String {
        let mut s = if self.interval == 1 {
            format!("every {}", self.unit_en())
        } else {
            format!("every {} {}s", self.interval, self.unit_en())
        };
        if let Some(w) = self.weekday {
            s.push_str(" on ");
            s.push_str(WD_EN[w.num_days_from_monday() as usize]);
        }
        s
    }

    pub fn to_es(&self) -> String {
        if let (Some(w), 1) = (self.weekday, self.interval) {
            return format!("cada {}", WD_ES[w.num_days_from_monday() as usize]);
        }
        let (sing, plur) = match self.unit {
            Unit::Day => ("día", "días"),
            Unit::Week => ("semana", "semanas"),
            Unit::Month => ("mes", "meses"),
            Unit::Year => ("año", "años"),
        };
        if self.interval == 1 {
            format!("cada {sing}")
        } else {
            format!("cada {} {plur}", self.interval)
        }
    }

    /// Siguiente fecha a partir de `base` (la fecha de la ocurrencia actual).
    pub fn next(&self, base: NaiveDate) -> NaiveDate {
        let n = self.interval;
        match self.unit {
            Unit::Day => base + Duration::days(n as i64),
            Unit::Week => match self.weekday {
                Some(w) => {
                    let mut d = base + Duration::days(1);
                    while d.weekday() != w {
                        d += Duration::days(1);
                    }
                    d + Duration::weeks(n as i64 - 1)
                }
                None => base + Duration::weeks(n as i64),
            },
            Unit::Month => base.checked_add_months(Months::new(n)).unwrap_or(base),
            Unit::Year => base.checked_add_months(Months::new(12 * n)).unwrap_or(base),
        }
    }

    /// Primera fecha cuando la tarea se crea con repetición pero sin fecha.
    pub fn first(&self, today: NaiveDate) -> NaiveDate {
        match self.weekday {
            Some(w) => {
                let mut d = today;
                while d.weekday() != w {
                    d += Duration::days(1);
                }
                d
            }
            None => today,
        }
    }
}

/// Comentario de seguimiento, con la fecha en que se escribió.
#[derive(Debug, Clone, PartialEq)]
pub struct Note {
    pub date: Option<NaiveDate>,
    pub text: String,
}

impl Note {
    pub fn new(text: &str, date: Option<NaiveDate>) -> Self {
        Note { date, text: text.trim().to_string() }
    }
}

#[derive(Debug, Clone, PartialEq)]
pub struct Task {
    /// Identificador estable (campo `🆔` de Obsidian Tasks).
    pub id: Option<String>,
    /// Carácter dentro de `[ ]`: ' ', 'x', '-', '/'...
    pub status: char,
    pub done: bool,
    pub text: String,
    pub labels: Vec<String>,
    /// 1 = máxima ... 4 = sin prioridad (como Todoist).
    pub priority: u8,
    /// Fecha: cuándo piensas hacerla.
    pub due: Option<NaiveDate>,
    /// Hora opcional de `due` (con hora, Recordatorios avisa a esa hora).
    pub time: Option<NaiveTime>,
    /// Fecha límite: cuándo debe estar hecha.
    pub deadline: Option<NaiveDate>,
    pub done_date: Option<NaiveDate>,
    pub recurrence: Option<Recurrence>,
    /// Campos de Obsidian Tasks que no manejamos, preservados tal cual.
    pub extra: Vec<String>,
    /// Descripción: texto libre, puede tener varias líneas.
    pub description: String,
    /// Comentarios de seguimiento, del más viejo al más nuevo.
    pub notes: Vec<Note>,
    pub indent: String,
    /// Lo asigna el almacén: nombre del archivo (proyecto) y encabezado (sección).
    pub project: String,
    pub section: Option<String>,
}

impl Task {
    pub fn new(text: &str) -> Self {
        Task {
            id: None,
            status: ' ',
            done: false,
            text: text.to_string(),
            labels: vec![],
            priority: 4,
            due: None,
            time: None,
            deadline: None,
            done_date: None,
            recurrence: None,
            extra: vec![],
            description: String::new(),
            notes: vec![],
            indent: String::new(),
            project: String::new(),
            section: None,
        }
    }

    pub fn set_done(&mut self, done: bool, today: NaiveDate) {
        self.done = done;
        self.status = if done { 'x' } else { ' ' };
        self.done_date = if done { Some(today) } else { None };
    }

    pub fn is_overdue(&self, today: NaiveDate) -> bool {
        !self.done && (self.due.is_some_and(|d| d < today) || self.deadline.is_some_and(|d| d < today))
    }

    /// Completa una tarea pendiente. Si se repite, la pendiente conserva su id
    /// y avanza a la siguiente fecha, y se crea una ocurrencia hecha con
    /// `done_id` (como Obsidian Tasks). Las notas se quedan en la pendiente.
    pub fn complete(&self, today: NaiveDate, done_id: String) -> Completion {
        let Some(rec) = &self.recurrence else {
            let mut t = self.clone();
            t.set_done(true, today);
            return Completion::Simple(t);
        };
        let mut next = self.clone();
        next.due = Some(rec.next(self.due.unwrap_or(today)));
        let mut done = self.clone();
        done.id = Some(done_id);
        done.description.clear();
        done.notes.clear();
        done.set_done(true, today);
        Completion::Recurring { next, done }
    }
}

/// Resultado de [`Task::complete`].
#[derive(Debug, Clone, PartialEq)]
pub enum Completion {
    /// La tarea, ya completada.
    Simple(Task),
    /// `next`: la misma tarea (mismo id) con la siguiente fecha.
    /// `done`: la ocurrencia completada, con id nuevo.
    Recurring { next: Task, done: Task },
}

#[cfg(test)]
mod tests {
    use super::*;

    fn d(s: &str) -> NaiveDate {
        NaiveDate::parse_from_str(s, "%Y-%m-%d").unwrap()
    }

    #[test]
    fn recurrencia_ida_y_vuelta() {
        for s in ["every day", "every 2 weeks", "every week on Monday", "every month", "every 3 years"] {
            assert_eq!(Recurrence::parse_obsidian(s).unwrap().to_obsidian(), s);
        }
        let r = Recurrence::parse_obsidian("every monday").unwrap();
        assert_eq!(r.to_obsidian(), "every week on Monday");
        assert_eq!(r.to_es(), "cada lunes");
    }

    #[test]
    fn completar() {
        let mut t = Task::new("Gym");
        t.id = Some("aaaaa".into());
        t.notes = vec![Note::new("rutina A", None)];
        t.due = Some(d("2026-11-05"));
        match t.complete(d("2026-11-06"), "bbbbb".into()) {
            Completion::Simple(h) => assert!(h.done && h.done_date == Some(d("2026-11-06"))),
            c => panic!("{c:?}"),
        }
        t.recurrence = Recurrence::parse_obsidian("every week on Monday");
        match t.complete(d("2026-11-06"), "bbbbb".into()) {
            Completion::Recurring { next, done } => {
                assert_eq!((next.id.as_deref(), next.done, next.due), (Some("aaaaa"), false, Some(d("2026-11-09"))));
                assert_eq!(next.notes, vec![Note::new("rutina A", None)]);
                assert_eq!((done.id.as_deref(), done.done), (Some("bbbbb"), true));
                assert!(done.notes.is_empty());
            }
            c => panic!("{c:?}"),
        }
    }

    #[test]
    fn siguiente_fecha() {
        let lunes = Recurrence::parse_obsidian("every week on Monday").unwrap();
        // 2026-11-05 es jueves
        assert_eq!(lunes.next(d("2026-11-05")), d("2026-11-09"));
        assert_eq!(lunes.next(d("2026-11-09")), d("2026-11-16"));
        let mes = Recurrence::parse_obsidian("every month").unwrap();
        assert_eq!(mes.next(d("2026-01-31")), d("2026-02-28"));
    }
}
