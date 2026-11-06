//! Captura rápida en lenguaje natural (español), al estilo Todoist:
//!
//! `Pagar luz mañana p1 #casa +Finanzas cada mes`
//!
//! - `#etiqueta` o `@etiqueta`, `+Proyecto` (usa `_` para espacios), `p1`..`p4`
//! - Fechas: hoy, mañana, pasado mañana, lunes..domingo, `en 3 días`,
//!   `15/11`, `15/11/2026`, `2026-11-15`
//! - Hora: `18:00`, `9:30`, `9am`, `6:30pm` (sin fecha, es para hoy)
//! - Fecha límite: `limit 20/11`, `limit viernes`, `{20/11}`, `{en 3 días}`
//! - Repetición: `cada día`, `cada 2 semanas`, `cada lunes`, `cada mes`
//! - `\palabra` fuerza a que la palabra sea texto literal.

use chrono::{Datelike, Duration, Months, NaiveDate, NaiveTime};

use crate::dates;
use crate::model::{Recurrence, Task, Unit};

#[derive(Debug, Default, Clone, PartialEq)]
pub struct Quick {
    pub text: String,
    pub labels: Vec<String>,
    pub priority: Option<u8>,
    pub due: Option<NaiveDate>,
    pub time: Option<NaiveTime>,
    pub deadline: Option<NaiveDate>,
    pub recurrence: Option<Recurrence>,
    pub project: Option<String>,
}

enum Effect {
    Label(String),
    Project(String),
    Priority(u8),
    Due(NaiveDate),
    Time(NaiveTime),
    Deadline(NaiveDate),
    Rec(Recurrence),
}

/// `18:00`, `9:30`, `9am`, `6pm`, `6:30pm`.
fn time_lit(s: &str) -> Option<NaiveTime> {
    let (body, pm) = match s.strip_suffix("am") {
        Some(b) => (b, Some(false)),
        None => match s.strip_suffix("pm") {
            Some(b) => (b, Some(true)),
            None => (s, None),
        },
    };
    let (h, m) = match body.split_once(':') {
        Some((h, m)) if m.len() == 2 => (h.parse::<u32>().ok()?, m.parse::<u32>().ok()?),
        Some(_) => return None,
        None if pm.is_some() => (body.parse::<u32>().ok()?, 0),
        None => return None,
    };
    let h = match pm {
        Some(_) if !(1..=12).contains(&h) => return None,
        Some(true) => h % 12 + 12,
        Some(false) => h % 12,
        None => h,
    };
    NaiveTime::from_hms_opt(h, m, 0)
}

/// Fecha escrita con los tokens `toks` completos (para `limit …` y `{…}`).
fn date_of(toks: &[&str], today: NaiveDate) -> Option<(usize, NaiveDate)> {
    match try_special(toks, 0, today)? {
        (n, Effect::Due(d)) => Some((n, d)),
        _ => None,
    }
}

fn unit_es(s: &str) -> Option<Unit> {
    Some(match s {
        "día" | "dia" | "días" | "dias" => Unit::Day,
        "semana" | "semanas" => Unit::Week,
        "mes" | "meses" => Unit::Month,
        "año" | "ano" | "años" | "anos" => Unit::Year,
        _ => return None,
    })
}

fn add(today: NaiveDate, n: u32, u: Unit) -> NaiveDate {
    match u {
        Unit::Day => today + Duration::days(n as i64),
        Unit::Week => today + Duration::weeks(n as i64),
        Unit::Month => today.checked_add_months(Months::new(n)).unwrap_or(today),
        Unit::Year => today.checked_add_months(Months::new(12 * n)).unwrap_or(today),
    }
}

fn date_lit(s: &str, today: NaiveDate) -> Option<NaiveDate> {
    if let Ok(d) = NaiveDate::parse_from_str(s, "%Y-%m-%d") {
        return Some(d);
    }
    let parts: Vec<&str> = s.split('/').collect();
    let day: u32 = parts.first()?.parse().ok()?;
    let month: u32 = parts.get(1)?.parse().ok()?;
    match parts.len() {
        2 => {
            let d = NaiveDate::from_ymd_opt(today.year(), month, day)?;
            if d < today {
                NaiveDate::from_ymd_opt(today.year() + 1, month, day)
            } else {
                Some(d)
            }
        }
        3 => {
            let mut y: i32 = parts[2].parse().ok()?;
            if y < 100 {
                y += 2000;
            }
            NaiveDate::from_ymd_opt(y, month, day)
        }
        _ => None,
    }
}

/// Intenta interpretar los tokens desde `i`. Devuelve cuántos consume.
fn try_special(toks: &[&str], i: usize, today: NaiveDate) -> Option<(usize, Effect)> {
    let tok = toks[i];
    let low = tok.to_lowercase();
    let next = |k: usize| toks.get(i + k).map(|s| s.to_lowercase());

    if let Some(l) = tok.strip_prefix('#').or_else(|| tok.strip_prefix('@')) {
        if !l.is_empty() && !l.starts_with(['#', '@']) && l.chars().any(|c| !c.is_ascii_digit()) {
            return Some((1, Effect::Label(l.to_string())));
        }
        return None;
    }
    if let Some(p) = tok.strip_prefix('+') {
        if p.chars().any(|c| c.is_alphabetic()) {
            return Some((1, Effect::Project(p.replace('_', " "))));
        }
        return None;
    }
    // Fecha límite entre llaves: {20/11}, {viernes}, {en 3 días}
    if tok.starts_with('{') {
        let end = (i..toks.len()).find(|&j| toks[j].ends_with('}'))?;
        let inner = toks[i..=end].join(" ");
        let inner = inner.trim_start_matches('{').trim_end_matches('}');
        let words: Vec<&str> = inner.split_whitespace().collect();
        return match date_of(&words, today) {
            Some((n, d)) if n == words.len() => Some((end - i + 1, Effect::Deadline(d))),
            _ => None,
        };
    }
    if let Some(t) = time_lit(&low) {
        return Some((1, Effect::Time(t)));
    }
    match low.as_str() {
        "limit" => {
            let (n, d) = date_of(&toks[i + 1..], today)?;
            return Some((n + 1, Effect::Deadline(d)));
        }
        "p1" | "p2" | "p3" | "p4" => {
            return Some((1, Effect::Priority(low[1..].parse().unwrap())));
        }
        "hoy" => return Some((1, Effect::Due(today))),
        "mañana" | "manana" => return Some((1, Effect::Due(today + Duration::days(1)))),
        "pasado" => {
            if matches!(next(1).as_deref(), Some("mañana" | "manana")) {
                return Some((2, Effect::Due(today + Duration::days(2))));
            }
        }
        "en" => {
            let n = next(1).and_then(|s| s.parse::<u32>().ok());
            let u = next(2).and_then(|s| unit_es(&s));
            if let (Some(n), Some(u)) = (n, u) {
                return Some((3, Effect::Due(add(today, n, u))));
            }
        }
        "cada" => {
            if let Some(n1) = next(1) {
                if let Ok(n) = n1.parse::<u32>() {
                    if let (true, Some(u)) = (n > 0, next(2).and_then(|s| unit_es(&s))) {
                        let r = Recurrence { interval: n, unit: u, weekday: None };
                        return Some((3, Effect::Rec(r)));
                    }
                } else if let Some(u) = unit_es(&n1) {
                    let r = Recurrence { interval: 1, unit: u, weekday: None };
                    return Some((2, Effect::Rec(r)));
                } else if let Some(w) = dates::weekday_es(&n1) {
                    let r = Recurrence { interval: 1, unit: Unit::Week, weekday: Some(w) };
                    return Some((2, Effect::Rec(r)));
                }
            }
        }
        _ => {}
    }
    if let Some(w) = dates::weekday_es(&low) {
        return Some((1, Effect::Due(dates::next_weekday(today, w))));
    }
    date_lit(&low, today).map(|d| (1, Effect::Due(d)))
}

impl Quick {
    /// Tarea nueva. Sin `+Proyecto`, va a `default_project`.
    pub fn into_task(self, default_project: &str) -> Task {
        let mut t = Task::new(&self.text);
        t.labels = self.labels;
        t.priority = self.priority.unwrap_or(4);
        t.due = self.due;
        t.time = self.time;
        t.deadline = self.deadline;
        t.recurrence = self.recurrence;
        t.project = self.project.unwrap_or_else(|| default_project.to_string());
        t
    }

    /// Cambia solo lo que se escribió (`tasktui edit id viernes p2`).
    pub fn apply_partial(self, t: &mut Task) {
        if !self.text.is_empty() {
            t.text = self.text;
        }
        if !self.labels.is_empty() {
            t.labels = self.labels;
        }
        if let Some(p) = self.priority {
            t.priority = p;
        }
        if self.due.is_some() {
            t.due = self.due;
        }
        if self.time.is_some() {
            t.time = self.time;
        }
        if self.deadline.is_some() {
            t.deadline = self.deadline;
        }
        if self.recurrence.is_some() {
            t.recurrence = self.recurrence;
        }
        if let Some(p) = self.project {
            t.project = p;
        }
    }

    /// Reemplaza todo con lo escrito (editar la línea completa, como en la
    /// TUI con [`to_quick`]). El proyecto solo cambia si se escribe `+Proyecto`.
    pub fn apply_full(self, t: &mut Task) {
        t.text = self.text;
        t.labels = self.labels;
        t.priority = self.priority.unwrap_or(4);
        t.due = self.due;
        t.time = self.time;
        t.deadline = self.deadline;
        t.recurrence = self.recurrence;
        if let Some(p) = self.project {
            t.project = p;
        }
    }
}

pub fn parse_quick(input: &str, today: NaiveDate) -> Quick {
    let toks: Vec<&str> = input.split_whitespace().collect();
    let mut q = Quick::default();
    let mut text: Vec<&str> = vec![];
    let mut i = 0;
    while i < toks.len() {
        if let Some(lit) = toks[i].strip_prefix('\\').filter(|s| !s.is_empty()) {
            text.push(lit);
            i += 1;
            continue;
        }
        match try_special(&toks, i, today) {
            Some((n, e)) => {
                match e {
                    Effect::Label(l) => {
                        if !q.labels.contains(&l) {
                            q.labels.push(l)
                        }
                    }
                    Effect::Project(p) => q.project = Some(p),
                    Effect::Priority(p) => q.priority = Some(p),
                    Effect::Due(d) => q.due = Some(d),
                    Effect::Time(t) => q.time = Some(t),
                    Effect::Deadline(d) => q.deadline = Some(d),
                    Effect::Rec(r) => q.recurrence = Some(r),
                }
                i += n;
            }
            None => {
                text.push(toks[i]);
                i += 1;
            }
        }
    }
    if let (Some(r), None) = (&q.recurrence, q.due) {
        q.due = Some(r.first(today));
    }
    if q.time.is_some() && q.due.is_none() {
        q.due = Some(today);
    }
    q.text = text.join(" ");
    q
}

/// Representación editable de una tarea (lo inverso de `parse_quick`).
pub fn to_quick(t: &Task, today: NaiveDate) -> String {
    let words: Vec<&str> = t.text.split_whitespace().collect();
    let mut out: Vec<String> = vec![];
    for i in 0..words.len() {
        if words[i].starts_with('\\') || try_special(&words, i, today).is_some() {
            out.push(format!("\\{}", words[i]));
        } else {
            out.push(words[i].to_string());
        }
    }
    for l in &t.labels {
        out.push(format!("#{l}"));
    }
    if t.priority < 4 {
        out.push(format!("p{}", t.priority));
    }
    if let Some(d) = t.due {
        out.push(d.to_string());
    }
    if let Some(h) = t.time {
        out.push(h.format("%H:%M").to_string());
    }
    if let Some(d) = t.deadline {
        out.push(format!("limit {d}"));
    }
    if let Some(r) = &t.recurrence {
        out.push(r.to_es());
    }
    if !t.project.is_empty() {
        out.push(format!("+{}", t.project.replace(' ', "_")));
    }
    out.join(" ")
}

#[cfg(test)]
mod tests {
    use super::*;

    fn d(s: &str) -> NaiveDate {
        NaiveDate::parse_from_str(s, "%Y-%m-%d").unwrap()
    }
    // 2026-11-05 es jueves
    const HOY: &str = "2026-11-05";

    #[test]
    fn captura_completa() {
        let q = parse_quick("Pagar luz mañana p1 #casa @finanzas +Mis_Cuentas", d(HOY));
        assert_eq!(q.text, "Pagar luz");
        assert_eq!(q.due, Some(d("2026-11-06")));
        assert_eq!(q.priority, Some(1));
        assert_eq!(q.labels, vec!["casa", "finanzas"]);
        assert_eq!(q.project.as_deref(), Some("Mis Cuentas"));
    }

    #[test]
    fn fechas() {
        let h = d(HOY);
        assert_eq!(parse_quick("x viernes", h).due, Some(d("2026-11-06")));
        assert_eq!(parse_quick("x jueves", h).due, Some(d("2026-11-12")));
        assert_eq!(parse_quick("x pasado mañana", h).due, Some(d("2026-11-07")));
        assert_eq!(parse_quick("x en 2 semanas", h).due, Some(d("2026-11-19")));
        assert_eq!(parse_quick("x 1/3", h).due, Some(d("2027-03-01")));
        assert_eq!(parse_quick("x 15/11/26", h).due, Some(d("2026-11-15")));
        let q = parse_quick("Ir en bici", h);
        assert_eq!((q.text.as_str(), q.due), ("Ir en bici", None));
    }

    #[test]
    fn hora_y_limite() {
        let h = d(HOY);
        let t = |s: &str| NaiveTime::parse_from_str(s, "%H:%M").ok();
        let q = parse_quick("Llamar mañana 18:00", h);
        assert_eq!((q.text.as_str(), q.due, q.time), ("Llamar", Some(d("2026-11-06")), t("18:00")));
        assert_eq!(parse_quick("x 9am", h).time, t("09:00"));
        assert_eq!(parse_quick("x 6:30pm", h).time, t("18:30"));
        assert_eq!(parse_quick("x 12am", h).time, t("00:00"));
        assert_eq!(parse_quick("x 12pm", h).time, t("12:00"));
        // Hora sin fecha: hoy.
        assert_eq!(parse_quick("x 9:30", h).due, Some(h));
        // No son horas.
        assert_eq!(parse_quick("Versión 2:3", h).time, None);
        assert_eq!(parse_quick("x 25:00", h).text, "x 25:00");

        let q = parse_quick("Entregar reporte viernes limit 20/11 p1", h);
        assert_eq!((q.text.as_str(), q.due, q.deadline), ("Entregar reporte", Some(d("2026-11-06")), Some(d("2026-11-20"))));
        assert_eq!(parse_quick("x limit viernes", h).deadline, Some(d("2026-11-06")));
        assert_eq!(parse_quick("x {20/11}", h).deadline, Some(d("2026-11-20")));
        let q = parse_quick("x {en 2 semanas} hoy", h);
        assert_eq!((q.deadline, q.due), (Some(d("2026-11-19")), Some(h)));
        let q = parse_quick("Sin limit claro", h);
        assert_eq!((q.text.as_str(), q.deadline), ("Sin limit claro", None));
    }

    #[test]
    fn repeticion() {
        let q = parse_quick("Gym cada lunes", d(HOY));
        assert_eq!(q.recurrence.unwrap().to_obsidian(), "every week on Monday");
        assert_eq!(q.due, Some(d("2026-11-09")));
        let q = parse_quick("Regar cada 3 días", d(HOY));
        assert_eq!(q.recurrence.unwrap().to_es(), "cada 3 días");
        assert_eq!(q.due, Some(d(HOY)));
    }

    #[test]
    fn ida_y_vuelta_con_escape() {
        let h = d(HOY);
        let mut t = Task::new("Revisar lo de hoy en 3 días");
        t.labels = vec!["work".into()];
        t.priority = 2;
        t.due = Some(d("2026-12-01"));
        t.time = NaiveTime::from_hms_opt(18, 0, 0);
        t.deadline = Some(d("2026-12-05"));
        t.project = "Mi Trabajo".into();
        let s = to_quick(&t, h);
        assert_eq!(s, "Revisar lo de \\hoy \\en 3 días #work p2 2026-12-01 18:00 limit 2026-12-05 +Mi_Trabajo");
        let q = parse_quick(&s, h);
        assert_eq!(q.text, t.text);
        assert_eq!((q.due, q.time, q.deadline), (t.due, t.time, t.deadline));
        assert_eq!(q.project.as_deref(), Some("Mi Trabajo"));
    }
}
