//! Vistas: qué tareas se muestran y cómo se agrupan.

use chrono::NaiveDate;

use crate::dates;
use crate::model::Task;
use crate::INBOX;

#[derive(Debug, Clone, PartialEq)]
pub enum View {
    Inbox,
    Today,
    Upcoming,
    All,
    Project(String),
    Label(String),
}

impl View {
    pub fn title(&self) -> String {
        match self {
            View::Inbox => INBOX.into(),
            View::Today => "Hoy".into(),
            View::Upcoming => "Próximos".into(),
            View::All => "Todas".into(),
            View::Project(p) => p.clone(),
            View::Label(l) => format!("#{l}"),
        }
    }

    /// `hoy`, `inbox`, `proximos`, `todas`, `+Proyecto`, `#etiqueta`.
    pub fn parse(s: &str) -> Option<Self> {
        Some(match s.to_lowercase().as_str() {
            "hoy" | "today" => View::Today,
            "inbox" | "entrada" => View::Inbox,
            "proximos" | "próximos" | "upcoming" => View::Upcoming,
            "todas" | "todo" | "all" => View::All,
            _ => {
                if let Some(p) = s.strip_prefix('+') {
                    View::Project(p.replace('_', " "))
                } else if let Some(l) = s.strip_prefix('#').or_else(|| s.strip_prefix('@')) {
                    View::Label(l.to_string())
                } else {
                    return None;
                }
            }
        })
    }
}

/// Un grupo de la vista. `K` es como quien llama identifica cada tarea
/// (posición en el archivo, id de base de datos...).
pub struct Group<K> {
    pub title: Option<String>,
    pub items: Vec<K>,
}

fn matches(t: &Task, q: &str) -> bool {
    q.is_empty()
        || t.text.to_lowercase().contains(q)
        || t.labels.iter().any(|l| l.to_lowercase().contains(q))
        || t.project.to_lowercase().contains(q)
}

fn push_grouped<K>(groups: &mut Vec<Group<K>>, title: Option<String>, pos: K) {
    match groups.last_mut() {
        Some(g) if g.title == title => g.items.push(pos),
        _ => groups.push(Group { title, items: vec![pos] }),
    }
}

type Item<'a, K> = (K, &'a Task);

fn by_priority<K>(v: &mut [Item<K>]) {
    v.sort_by_key(|(_, t)| (t.priority, t.due));
}

/// Proyectos en orden: Inbox primero y luego alfabético.
pub fn projects<'a>(tasks: impl IntoIterator<Item = &'a Task>) -> Vec<String> {
    let mut ps: Vec<String> = tasks.into_iter().map(|t| t.project.clone()).collect();
    ps.sort_by_key(|p| (p != INBOX, p.to_lowercase()));
    ps.dedup();
    ps
}

/// Arma una vista. `tasks` en el orden en que aparecen (archivo y línea);
/// `projects` en el orden de la vista Todas (ver [`projects`]).
pub fn build<K: Copy>(
    tasks: &[(K, &Task)],
    projects: &[String],
    view: &View,
    today: NaiveDate,
    show_done: bool,
    query: &str,
) -> Vec<Group<K>> {
    let q = query.to_lowercase();
    let visible = |t: &Task| (show_done || !t.done) && matches(t, &q);
    let mut groups: Vec<Group<K>> = vec![];
    let iter = || tasks.iter().copied();

    match view {
        View::Inbox | View::Project(_) => {
            let name = match view {
                View::Project(p) => p.to_lowercase(),
                _ => INBOX.to_lowercase(),
            };
            for (pos, t) in iter() {
                if t.project.to_lowercase() == name && visible(t) {
                    push_grouped(&mut groups, t.section.clone(), pos);
                }
            }
        }
        View::Today => {
            let (mut over, mut tod, mut done): (Vec<Item<K>>, Vec<Item<K>>, Vec<Item<K>>) = (vec![], vec![], vec![]);
            for (pos, t) in iter().filter(|(_, t)| matches(t, &q)) {
                if !t.done {
                    match t.due {
                        Some(d) if d < today => over.push((pos, t)),
                        Some(d) if d == today => tod.push((pos, t)),
                        _ => {}
                    }
                } else if show_done && t.done_date == Some(today) {
                    done.push((pos, t));
                }
            }
            by_priority(&mut over);
            by_priority(&mut tod);
            for (title, v) in [
                ("Vencidas".to_string(), over),
                (dates::long(today, today), tod),
                ("Completadas hoy".to_string(), done),
            ] {
                groups.push(Group { title: Some(title), items: v.into_iter().map(|(p, _)| p).collect() });
            }
        }
        View::Upcoming => {
            let mut v: Vec<Item<K>> = iter()
                .filter(|(_, t)| !t.done && t.due.is_some_and(|d| d > today) && matches(t, &q))
                .collect();
            v.sort_by_key(|(_, t)| (t.due, t.priority));
            for (pos, t) in v {
                push_grouped(&mut groups, Some(dates::long(t.due.unwrap(), today)), pos);
            }
        }
        View::All => {
            for p in projects {
                let items = iter()
                    .filter(|(_, t)| &t.project == p && visible(t))
                    .map(|(pos, _)| pos)
                    .collect();
                groups.push(Group { title: Some(p.clone()), items });
            }
        }
        View::Label(l) => {
            let l = l.to_lowercase();
            for (pos, t) in iter() {
                if visible(t) && t.labels.iter().any(|x| x.to_lowercase() == l) {
                    push_grouped(&mut groups, Some(t.project.clone()), pos);
                }
            }
        }
    }
    groups.retain(|g| !g.items.is_empty());
    groups
}
