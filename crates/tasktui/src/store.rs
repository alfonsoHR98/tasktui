//! Almacén de tareas: archivos `.md` locales ([`FileStore`]) o el servidor
//! ([`RemoteStore`]). La TUI, la CLI y Recordatorios usan esta interfaz y no
//! saben cuál de los dos hay detrás.
//!
//! Cada tarea se identifica por su posición `(a, b)`: en archivos es
//! (documento, línea); en remoto es `(0, índice)`.

use std::collections::BTreeMap;
use std::path::Path;

use anyhow::{bail, Result};
use chrono::NaiveDate;

use crate::config::{self, Mode};
use crate::files::{FileSnapshot, FileStore};
use crate::model::Task;
use crate::remote::RemoteStore;
use crate::views::{self, Group, View};

pub use tasktui_core::INBOX;

pub enum Store {
    Files(FileStore),
    Remote(RemoteStore),
}

/// Copia para deshacer.
pub enum Snapshot {
    Files(FileSnapshot),
    Remote(Vec<Task>),
}

impl Store {
    /// Abre el almacén del modo configurado (ver [`config::mode`]).
    pub fn open() -> Result<Self> {
        Ok(match config::mode()? {
            Mode::Local(dir) => Store::Files(FileStore::open(dir)?),
            Mode::Remote(server) => Store::Remote(RemoteStore::open(&server)?),
        })
    }

    /// Aviso para mostrar al abrir (p. ej. sin conexión).
    pub fn notice(&self) -> Option<String> {
        match self {
            Store::Remote(r) if r.offline => {
                Some(format!("Sin conexión con {}: mostrando la última copia (solo lectura)", r.url()))
            }
            _ => None,
        }
    }

    pub fn dir(&self) -> Option<&Path> {
        match self {
            Store::Files(f) => Some(&f.dir),
            Store::Remote(_) => None,
        }
    }

    pub fn reload(&mut self) -> Result<()> {
        match self {
            Store::Files(f) => f.reload(),
            Store::Remote(r) => r.reload(),
        }
    }

    pub fn iter(&self) -> Box<dyn Iterator<Item = ((usize, usize), &Task)> + '_> {
        match self {
            Store::Files(f) => Box::new(f.iter()),
            Store::Remote(r) => Box::new(r.tasks().iter().enumerate().map(|(i, t)| ((0, i), t))),
        }
    }

    /// Arma una vista con los criterios del núcleo.
    pub fn view(&self, view: &View, today: NaiveDate, show_done: bool, query: &str) -> Vec<Group<(usize, usize)>> {
        let tasks: Vec<_> = self.iter().collect();
        views::build(&tasks, &self.projects(), view, today, show_done, query)
    }

    pub fn task_at(&self, pos: (usize, usize)) -> Option<&Task> {
        match self {
            Store::Files(f) => f.task_at(pos),
            Store::Remote(r) => r.tasks().get(pos.1),
        }
    }

    pub fn find(&self, id: &str) -> Option<(usize, usize)> {
        match self {
            Store::Files(f) => f.find(id),
            Store::Remote(r) => r.find(id).map(|i| (0, i)),
        }
    }

    pub fn projects(&self) -> Vec<String> {
        match self {
            Store::Files(f) => f.projects(),
            Store::Remote(r) => r.projects(),
        }
    }

    /// Etiquetas con su número de tareas pendientes.
    pub fn labels(&self) -> Vec<(String, usize)> {
        let mut m: BTreeMap<String, usize> = BTreeMap::new();
        for (_, t) in self.iter() {
            for l in &t.labels {
                let e = m.entry(l.clone()).or_default();
                if !t.done {
                    *e += 1;
                }
            }
        }
        m.into_iter().collect()
    }

    pub fn add(&mut self, task: Task, project: &str) -> Result<String> {
        match self {
            Store::Files(f) => f.add(task, project),
            Store::Remote(r) => r.add(task, project),
        }
    }

    /// Modifica una tarea. Si cambia `project`, se mueve.
    pub fn update(&mut self, id: &str, f: impl FnOnce(&mut Task)) -> Result<()> {
        match self {
            Store::Files(s) => s.update(id, f),
            Store::Remote(r) => r.update(id, f),
        }
    }

    /// Completa o reabre. Devuelve `true` si quedó completada.
    pub fn toggle(&mut self, id: &str) -> Result<bool> {
        match self {
            Store::Files(f) => f.toggle(id),
            Store::Remote(r) => r.toggle(id),
        }
    }

    pub fn delete(&mut self, id: &str) -> Result<()> {
        match self {
            Store::Files(f) => f.delete(id),
            Store::Remote(r) => r.delete(id),
        }
    }

    pub fn add_note(&mut self, id: &str, text: &str) -> Result<()> {
        match self {
            Store::Files(f) => f.add_note(id, text),
            Store::Remote(r) => r.add_note(id, text),
        }
    }

    pub fn snapshot(&self) -> Result<Snapshot> {
        Ok(match self {
            Store::Files(f) => Snapshot::Files(f.snapshot()?),
            Store::Remote(r) => Snapshot::Remote(r.snapshot()),
        })
    }

    pub fn restore(&mut self, snap: &Snapshot) -> Result<()> {
        match (self, snap) {
            (Store::Files(f), Snapshot::Files(s)) => f.restore(s),
            (Store::Remote(r), Snapshot::Remote(s)) => r.restore(s),
            _ => bail!("La copia para deshacer es de otro almacén"),
        }
    }
}

#[cfg(test)]
pub mod tests {
    use super::*;

    pub fn temp_store(name: &str) -> Store {
        Store::Files(crate::files::tests::temp_store(name))
    }
}
