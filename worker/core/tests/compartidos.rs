//! Casos compartidos con Go (internal/core/shared_test.go): los dos parsers deben
//! interpretar igual la captura rápida.

use chrono::NaiveDate;
use serde_json::Value;
use tasktui_core::quick::parse_quick_in;

#[test]
fn casos_compartidos() {
    let raw = include_str!("../../../testdata/captura.json");
    let f: Value = serde_json::from_str(raw).unwrap();
    let today = NaiveDate::parse_from_str(f["today"].as_str().unwrap(), "%Y-%m-%d").unwrap();
    let projects: Vec<String> =
        f["projects"].as_array().unwrap().iter().map(|p| p.as_str().unwrap().to_string()).collect();
    let mut fallos = vec![];
    for c in f["cases"].as_array().unwrap() {
        let input = c["in"].as_str().unwrap();
        let known = c.get("known").and_then(Value::as_bool).unwrap_or(true);
        let ps: &[String] = if known { &projects } else { &[] };
        let q = parse_quick_in(input, today, ps);
        let mut check = |campo: &str, got: Option<String>| {
            if let Some(want) = c.get(campo) {
                let want = want.as_str().map(str::to_string);
                if got != want {
                    fallos.push(format!("{input:?}: {campo} = {got:?}, quería {want:?}"));
                }
            }
        };
        check("text", Some(q.text.clone()));
        check("project", q.project.clone());
        check("due", q.due.map(|d| d.format("%Y-%m-%d").to_string()));
        check("time", q.time.map(|t| t.format("%H:%M").to_string()));
        check("deadline", q.deadline.map(|d| d.format("%Y-%m-%d").to_string()));
        if let Some(p) = c.get("priority").and_then(Value::as_u64) {
            if q.priority != Some(p as u8) {
                fallos.push(format!("{input:?}: prioridad {:?}, quería {p}", q.priority));
            }
        }
        if let Some(ls) = c.get("labels").and_then(Value::as_array) {
            let want: Vec<String> = ls.iter().map(|l| l.as_str().unwrap().to_string()).collect();
            if q.labels != want {
                fallos.push(format!("{input:?}: etiquetas {:?}, quería {want:?}", q.labels));
            }
        }
    }
    assert!(fallos.is_empty(), "\n{}", fallos.join("\n"));
}
