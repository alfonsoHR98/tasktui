//! Lectura/escritura de una línea de tarea en formato Obsidian Tasks:
//!
//! `- [ ] Pagar luz #casa ⏫ 🔁 every month 📅 2026-11-06 ⏰ 18:00 🎯 2026-11-20 🆔 a1b2c`
//!
//! `⏰` (hora) y `🎯` (fecha límite) son propios de tasktui: Obsidian Tasks no
//! los interpreta, pero los conserva como texto.

use chrono::{NaiveDate, NaiveTime};

use crate::model::{Recurrence, Task};

const DUE: &str = "📅";
const DONE: &str = "✅";
const ID: &str = "🆔";
const TIME: &str = "⏰";
const DEADLINE: &str = "🎯";
const REC: &str = "🔁";
const PRIO: [(&str, u8); 4] = [("🔺", 1), ("⏫", 2), ("🔼", 3), ("🔽", 4)];
/// Otros campos de Obsidian Tasks: se conservan pero no se interpretan.
const OTHER: [&str; 6] = ["⏳", "🛫", "➕", "❌", "🏁", "⛔"];

fn markers() -> impl Iterator<Item = &'static str> {
    [DUE, DONE, ID, REC, TIME, DEADLINE]
        .into_iter()
        .chain(PRIO.iter().map(|(m, _)| *m))
        .chain(OTHER)
}

/// Si el token empieza por un marcador, devuelve (marcador, resto pegado).
fn split_marker(tok: &str) -> Option<(&'static str, &str)> {
    markers().find_map(|m| tok.strip_prefix(m).map(|r| (m, r)))
}

fn parse_date(s: &str) -> Option<NaiveDate> {
    NaiveDate::parse_from_str(s, "%Y-%m-%d").ok()
}

/// ¿Es una etiqueta válida de Obsidian? (`#12` no lo es).
pub fn as_tag(tok: &str) -> Option<&str> {
    let t = tok.strip_prefix('#')?;
    let t = t.trim_end_matches([',', '.', ';', ':', '!', '?', ')']);
    if t.is_empty() || t.starts_with('#') || t.chars().all(|c| c.is_ascii_digit()) {
        return None;
    }
    Some(t)
}

pub fn parse_line(line: &str) -> Option<Task> {
    let trimmed = line.trim_start();
    let indent = &line[..line.len() - trimmed.len()];
    let after = trimmed
        .strip_prefix("- [")
        .or_else(|| trimmed.strip_prefix("* ["))?;
    let mut it = after.chars();
    let status = it.next()?;
    let rest = it.as_str().strip_prefix(']')?;
    if !(rest.is_empty() || rest.starts_with(' ')) {
        return None;
    }

    let toks: Vec<String> = rest
        .split_whitespace()
        .map(|t| t.replace('\u{FE0F}', ""))
        .collect();
    let mut t = Task::new("");
    t.indent = indent.to_string();
    t.status = status;
    t.done = matches!(status, 'x' | 'X' | '-');

    let mut text: Vec<String> = vec![];
    let mut i = 0;
    while i < toks.len() {
        let tok = &toks[i];
        if let Some((marker, glued)) = split_marker(tok) {
            // Valor: lo pegado al emoji o el siguiente token.
            let value = |i: &mut usize| -> String {
                if !glued.is_empty() {
                    glued.to_string()
                } else if *i + 1 < toks.len() {
                    *i += 1;
                    toks[*i].clone()
                } else {
                    String::new()
                }
            };
            match marker {
                DUE => {
                    let v = value(&mut i);
                    match parse_date(&v) {
                        Some(d) => t.due = Some(d),
                        None => t.extra.push(format!("{DUE} {v}")),
                    }
                }
                DONE => {
                    let v = value(&mut i);
                    match parse_date(&v) {
                        Some(d) => t.done_date = Some(d),
                        None => t.extra.push(format!("{DONE} {v}")),
                    }
                }
                TIME => {
                    let v = value(&mut i);
                    match NaiveTime::parse_from_str(&v, "%H:%M") {
                        Ok(h) => t.time = Some(h),
                        Err(_) => t.extra.push(format!("{TIME} {v}")),
                    }
                }
                DEADLINE => {
                    let v = value(&mut i);
                    match parse_date(&v) {
                        Some(d) => t.deadline = Some(d),
                        None => t.extra.push(format!("{DEADLINE} {v}")),
                    }
                }
                ID => {
                    let v = value(&mut i);
                    if !v.is_empty() {
                        t.id = Some(v);
                    }
                }
                REC => {
                    let mut parts: Vec<String> = vec![];
                    if !glued.is_empty() {
                        parts.push(glued.to_string());
                    }
                    while i + 1 < toks.len() && split_marker(&toks[i + 1]).is_none() {
                        i += 1;
                        parts.push(toks[i].clone());
                    }
                    let raw = parts.join(" ");
                    match Recurrence::parse_obsidian(&raw) {
                        Some(r) => t.recurrence = Some(r),
                        None => t.extra.push(format!("{REC} {raw}")),
                    }
                }
                m => {
                    if let Some((_, p)) = PRIO.iter().find(|(pm, _)| *pm == m) {
                        t.priority = *p;
                        if !glued.is_empty() {
                            text.push(glued.to_string());
                        }
                    } else {
                        let v = value(&mut i);
                        t.extra.push(format!("{m} {v}").trim_end().to_string());
                    }
                }
            }
        } else if let Some(tag) = as_tag(tok) {
            t.labels.push(tag.to_string());
        } else {
            text.push(tok.clone());
        }
        i += 1;
    }
    t.text = text.join(" ");
    Some(t)
}

pub fn render_line(t: &Task) -> String {
    let mut s = format!("{}- [{}]", t.indent, t.status);
    let mut push = |part: &str| {
        if !part.is_empty() {
            s.push(' ');
            s.push_str(part);
        }
    };
    push(&t.text);
    for l in &t.labels {
        push(&format!("#{l}"));
    }
    if t.priority < 4 {
        push(PRIO[t.priority as usize - 1].0);
    }
    if let Some(r) = &t.recurrence {
        push(&format!("{REC} {}", r.to_obsidian()));
    }
    for e in &t.extra {
        push(e);
    }
    if let Some(d) = t.due {
        push(&format!("{DUE} {d}"));
    }
    if let Some(h) = t.time {
        push(&format!("{TIME} {}", h.format("%H:%M")));
    }
    if let Some(d) = t.deadline {
        push(&format!("{DEADLINE} {d}"));
    }
    if let Some(id) = &t.id {
        push(&format!("{ID} {id}"));
    }
    if let Some(d) = t.done_date {
        push(&format!("{DONE} {d}"));
    }
    s
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn ida_y_vuelta() {
        let line = "  - [ ] Pagar luz #casa #finanzas ⏫ 🔁 every month 📅 2026-11-06 🆔 a1b2c";
        let t = parse_line(line).unwrap();
        assert_eq!(t.text, "Pagar luz");
        assert_eq!(t.labels, vec!["casa", "finanzas"]);
        assert_eq!(t.priority, 2);
        assert_eq!(t.due.unwrap().to_string(), "2026-11-06");
        assert_eq!(t.id.as_deref(), Some("a1b2c"));
        assert_eq!(t.indent, "  ");
        assert_eq!(render_line(&t), line);
    }

    #[test]
    fn hora_y_limite() {
        let line = "- [ ] Entregar ⏫ 📅 2026-11-06 ⏰ 18:00 🎯 2026-11-20 🆔 a1b2c";
        let t = parse_line(line).unwrap();
        assert_eq!(t.time.unwrap().format("%H:%M").to_string(), "18:00");
        assert_eq!(t.deadline.unwrap().to_string(), "2026-11-20");
        assert_eq!(t.text, "Entregar");
        assert_eq!(render_line(&t), line);
    }

    #[test]
    fn hecha_y_campos_desconocidos() {
        let t = parse_line("- [x] Llamar ⏳ 2026-11-01 ✅ 2026-11-02").unwrap();
        assert!(t.done);
        assert_eq!(t.extra, vec!["⏳ 2026-11-01"]);
        assert_eq!(t.done_date.unwrap().to_string(), "2026-11-02");
        assert_eq!(render_line(&t), "- [x] Llamar ⏳ 2026-11-01 ✅ 2026-11-02");
    }

    #[test]
    fn no_es_tarea() {
        assert!(parse_line("- texto").is_none());
        assert!(parse_line("## Sección").is_none());
        assert!(parse_line("- [ ]sin espacio").is_none());
        let t = parse_line("- [ ] issue #12 y emoji 📅️ 2026-01-01").unwrap();
        assert_eq!(t.text, "issue #12 y emoji");
        assert!(t.due.is_some());
    }
}
