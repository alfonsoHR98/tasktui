//! Almacén local: una carpeta de archivos Markdown.
//!
//! - Cada archivo `.md` es un proyecto (`Trabajo/Cliente.md` → proyecto `Trabajo/Cliente`).
//! - Los encabezados `##` dentro del archivo son secciones.
//! - Las líneas que no son tareas se conservan intactas, igual que las tareas
//!   que no se modifican (no se reformatea lo que escribiste en Obsidian).
//! - Las líneas con más sangría justo debajo de una tarea son su bloque: el
//!   texto sin viñeta es la descripción y las viñetas (`- 2026-11-06: texto`)
//!   son notas. El bloque se mueve y se borra junto con la tarea.

use std::collections::HashSet;
use std::fs;
use std::hash::{BuildHasher, Hasher, RandomState};
use std::path::{Path, PathBuf};

use anyhow::{anyhow, bail, Context, Result};
use chrono::NaiveDate;

use crate::markdown;
use crate::model::{Completion, Note, Task};
use crate::today;

pub use tasktui_core::INBOX;

pub enum Line {
    /// `raw` = texto original; `None` si la tarea se modificó y hay que re-generarla.
    Task { task: Task, raw: Option<String> },
    Other(String),
}

pub struct Doc {
    pub path: PathBuf,
    pub project: String,
    pub lines: Vec<Line>,
}

impl Doc {
    fn render(&self) -> String {
        let mut s = self
            .lines
            .iter()
            .map(|l| match l {
                Line::Task { raw: Some(r), .. } => r.clone(),
                Line::Task { task, raw: None } => markdown::render_line(task),
                Line::Other(o) => o.clone(),
            })
            .collect::<Vec<_>>()
            .join("\n");
        s.push('\n');
        s
    }

    fn has_tasks(&self) -> bool {
        self.lines.iter().any(|l| matches!(l, Line::Task { .. }))
    }
}

/// Copia del contenido de los archivos, para deshacer.
pub type FileSnapshot = Vec<(PathBuf, String)>;

pub struct FileStore {
    pub dir: PathBuf,
    pub docs: Vec<Doc>,
}

fn walk(dir: &Path, out: &mut Vec<PathBuf>) -> Result<()> {
    for e in fs::read_dir(dir).with_context(|| format!("No pude leer {}", dir.display()))? {
        let e = e?;
        if e.file_name().to_string_lossy().starts_with('.') {
            continue; // .obsidian, .trash, temporales
        }
        let p = e.path();
        if p.is_dir() {
            walk(&p, out)?;
        } else if p.extension().is_some_and(|x| x == "md") {
            out.push(p);
        }
    }
    Ok(())
}

fn write_atomic(path: &Path, content: &str) -> Result<()> {
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent)?;
    }
    let name = path.file_name().unwrap_or_default().to_string_lossy();
    let tmp = path.with_file_name(format!(".{name}.tmp"));
    fs::write(&tmp, content)?;
    fs::rename(&tmp, path)?;
    Ok(())
}

/// Ancho de la sangría (un tabulador cuenta como 4).
fn indent_width(s: &str) -> usize {
    s.chars()
        .take_while(|c| c.is_whitespace())
        .map(|c| if c == '\t' { 4 } else { 1 })
        .sum()
}

/// Línea del bloque de una tarea: nota (con viñeta) o descripción (sin ella).
enum BlockLine {
    Note(Note),
    Desc(String),
}

fn block_line(s: &str) -> BlockLine {
    let t = s.trim();
    match t.strip_prefix("- ").or_else(|| t.strip_prefix("* ")) {
        Some(rest) => {
            let dated = rest
                .split_once(": ")
                .and_then(|(d, txt)| NaiveDate::parse_from_str(d, "%Y-%m-%d").ok().map(|d| (d, txt)));
            BlockLine::Note(match dated {
                Some((d, txt)) => Note::new(txt, Some(d)),
                None => Note::new(rest, None),
            })
        }
        None => BlockLine::Desc(t.to_string()),
    }
}

/// ¿La línea `s` es nota de una tarea con sangría `task_indent`?
fn is_note_of(s: &str, task_indent: &str) -> bool {
    !s.trim().is_empty() && heading(s).is_none() && indent_width(s) > indent_width(task_indent)
}

/// Línea de nota tal como se escribe en disco, bajo una tarea con `indent`.
pub fn note_line(indent: &str, n: &Note) -> String {
    match n.date {
        Some(d) => format!("{indent}    - {d}: {}", n.text.trim()),
        None => format!("{indent}    - {}", n.text.trim()),
    }
}

/// El bloque completo bajo la tarea: descripción y luego notas.
pub fn block_lines(t: &Task) -> Vec<String> {
    t.description
        .lines()
        .map(str::trim)
        .filter(|l| !l.is_empty())
        .map(|l| format!("{}    {l}", t.indent))
        .chain(t.notes.iter().map(|n| note_line(&t.indent, n)))
        .collect()
}

fn heading(s: &str) -> Option<(usize, String)> {
    let n = s.chars().take_while(|c| *c == '#').count();
    if n == 0 || n > 6 || !s[n..].starts_with(' ') {
        return None;
    }
    Some((n, s[n..].trim().to_string()))
}

pub fn gen_id(existing: &HashSet<String>) -> String {
    const ALPHA: &[u8] = b"0123456789abcdefghijklmnopqrstuvwxyz";
    loop {
        let mut h = RandomState::new().build_hasher();
        h.write_u128(
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .map(|d| d.as_nanos())
                .unwrap_or(0),
        );
        let mut v = h.finish();
        let id: String = (0..5)
            .map(|_| {
                let c = ALPHA[(v % 36) as usize] as char;
                v /= 36;
                c
            })
            .collect();
        if !existing.contains(&id) {
            return id;
        }
    }
}

impl FileStore {
    pub fn open(dir: PathBuf) -> Result<Self> {
        fs::create_dir_all(&dir).with_context(|| format!("No pude crear {}", dir.display()))?;
        let inbox = dir.join(format!("{INBOX}.md"));
        if !inbox.exists() {
            write_atomic(&inbox, &format!("# {INBOX}\n\n"))?;
        }
        let mut s = FileStore { dir, docs: vec![] };
        s.reload()?;
        Ok(s)
    }

    /// Vuelve a leer todo desde disco (por si cambió desde Obsidian/el celular).
    pub fn reload(&mut self) -> Result<()> {
        let mut files = vec![];
        walk(&self.dir, &mut files)?;
        files.sort();
        let mut docs = vec![];
        for path in files {
            let content = fs::read_to_string(&path)
                .with_context(|| format!("No pude leer {}", path.display()))?;
            docs.push(self.parse_doc(path, &content));
        }
        self.docs = docs;
        self.normalize_ids()
    }

    fn parse_doc(&self, path: PathBuf, content: &str) -> Doc {
        let rel = path.strip_prefix(&self.dir).unwrap_or(&path);
        let project = rel.with_extension("").to_string_lossy().replace('\\', "/");
        let mut section = None;
        let mut lines = vec![];
        // Última tarea a la que todavía se le pueden pegar notas.
        let mut owner: Option<usize> = None;
        for raw in content.lines() {
            if let Some(mut task) = markdown::parse_line(raw) {
                task.project = project.clone();
                task.section = section.clone();
                owner = Some(lines.len());
                lines.push(Line::Task { task, raw: Some(raw.to_string()) });
                continue;
            }
            if let Some(oi) = owner {
                if let Line::Task { task, .. } = &mut lines[oi] {
                    if is_note_of(raw, &task.indent) {
                        match block_line(raw) {
                            BlockLine::Note(n) => task.notes.push(n),
                            BlockLine::Desc(d) => {
                                if !task.description.is_empty() {
                                    task.description.push('\n');
                                }
                                task.description.push_str(&d);
                            }
                        }
                        lines.push(Line::Other(raw.to_string()));
                        continue;
                    }
                }
                owner = None;
            }
            if let Some((level, title)) = heading(raw) {
                section = (level >= 2).then_some(title);
            }
            lines.push(Line::Other(raw.to_string()));
        }
        Doc { path, project, lines }
    }

    /// Toda tarea pendiente necesita un 🆔 para poder referirse a ella.
    fn normalize_ids(&mut self) -> Result<()> {
        let mut ids = self.ids();
        for di in 0..self.docs.len() {
            let mut dirty = false;
            for line in &mut self.docs[di].lines {
                if let Line::Task { task, raw } = line {
                    if !task.done && task.id.is_none() {
                        let id = gen_id(&ids);
                        ids.insert(id.clone());
                        task.id = Some(id);
                        *raw = None;
                        dirty = true;
                    }
                }
            }
            if dirty {
                self.write(di)?;
            }
        }
        Ok(())
    }

    fn write(&self, di: usize) -> Result<()> {
        let d = &self.docs[di];
        write_atomic(&d.path, &d.render())
    }

    pub fn iter(&self) -> impl Iterator<Item = ((usize, usize), &Task)> {
        self.docs.iter().enumerate().flat_map(|(di, d)| {
            d.lines.iter().enumerate().filter_map(move |(li, l)| match l {
                Line::Task { task, .. } => Some(((di, li), task)),
                Line::Other(_) => None,
            })
        })
    }

    pub fn task_at(&self, pos: (usize, usize)) -> Option<&Task> {
        match self.docs.get(pos.0)?.lines.get(pos.1)? {
            Line::Task { task, .. } => Some(task),
            Line::Other(_) => None,
        }
    }

    pub fn find(&self, id: &str) -> Option<(usize, usize)> {
        self.iter()
            .find(|(_, t)| t.id.as_deref() == Some(id))
            .map(|(p, _)| p)
    }

    fn find_or_err(&self, id: &str) -> Result<(usize, usize)> {
        self.find(id).ok_or_else(|| anyhow!("No encontré la tarea «{id}»"))
    }

    fn ids(&self) -> HashSet<String> {
        self.iter().filter_map(|(_, t)| t.id.clone()).collect()
    }

    /// Proyectos: Inbox primero y el resto en orden alfabético.
    pub fn projects(&self) -> Vec<String> {
        let mut ps: Vec<String> = self
            .docs
            .iter()
            .map(|d| d.project.clone())
            .filter(|p| p != INBOX)
            .collect();
        ps.sort_by_key(|p| p.to_lowercase());
        ps.insert(0, INBOX.to_string());
        ps
    }


    fn doc_index(&self, project: &str) -> Option<usize> {
        let p = project.to_lowercase();
        self.docs.iter().position(|d| d.project.to_lowercase() == p)
    }

    fn create_doc(&mut self, project: &str) -> Result<usize> {
        let name = project.trim().trim_matches('/');
        if name.is_empty()
            || name
                .split('/')
                .any(|c| c.trim().is_empty() || c == ".." || c.starts_with('.'))
        {
            bail!("Nombre de proyecto inválido: «{project}»");
        }
        let title = name.rsplit('/').next().unwrap_or(name);
        let doc = Doc {
            path: self.dir.join(format!("{name}.md")),
            project: name.to_string(),
            lines: vec![Line::Other(format!("# {title}")), Line::Other(String::new())],
        };
        self.docs.push(doc);
        let di = self.docs.len() - 1;
        self.write(di)?;
        Ok(di)
    }

    fn doc_or_create(&mut self, project: &str) -> Result<usize> {
        match self.doc_index(project) {
            Some(i) => Ok(i),
            None => self.create_doc(project),
        }
    }

    /// Fin (exclusivo) del bloque de la tarea en `pos`: la tarea y sus notas.
    fn block_end(&self, pos: (usize, usize)) -> usize {
        let lines = &self.docs[pos.0].lines;
        let Some(Line::Task { task, .. }) = lines.get(pos.1) else {
            return pos.1 + 1;
        };
        let mut j = pos.1 + 1;
        while let Some(Line::Other(s)) = lines.get(j) {
            if !is_note_of(s, &task.indent) {
                break;
            }
            j += 1;
        }
        j
    }

    /// Inserta tras la última tarea del archivo (y sus notas), o al final.
    /// Las notas de la tarea se escriben debajo.
    fn insert_task(&mut self, di: usize, mut task: Task) {
        task.project = self.docs[di].project.clone();
        let pos = self.docs[di]
            .lines
            .iter()
            .rposition(|l| matches!(l, Line::Task { .. }))
            .map(|p| self.block_end((di, p)))
            .unwrap_or(self.docs[di].lines.len());
        let notes: Vec<Line> = block_lines(&task).into_iter().map(Line::Other).collect();
        let doc = &mut self.docs[di];
        doc.lines.insert(pos, Line::Task { task, raw: None });
        for (k, n) in notes.into_iter().enumerate() {
            doc.lines.insert(pos + 1 + k, n);
        }
    }

    /// Quita la tarea en `pos` junto con sus notas.
    fn remove_block(&mut self, pos: (usize, usize)) {
        let end = self.block_end(pos);
        self.docs[pos.0].lines.drain(pos.1..end);
    }

    /// Añade una nota debajo de la tarea (tras sus notas actuales).
    pub fn add_note(&mut self, id: &str, text: &str) -> Result<()> {
        if text.trim().is_empty() {
            bail!("La nota está vacía");
        }
        self.reload()?;
        let pos = self.find_or_err(id)?;
        let end = self.block_end(pos);
        let indent = self.task_at(pos).unwrap().indent.clone();
        let note = Note::new(text, Some(today()));
        self.docs[pos.0].lines.insert(end, Line::Other(note_line(&indent, &note)));
        self.write(pos.0)
    }

    fn set_task(&mut self, pos: (usize, usize), task: Task) {
        self.docs[pos.0].lines[pos.1] = Line::Task { task, raw: None };
    }

    pub fn add(&mut self, mut task: Task, project: &str) -> Result<String> {
        self.reload()?;
        let id = gen_id(&self.ids());
        task.id = Some(id.clone());
        let di = self.doc_or_create(project)?;
        self.insert_task(di, task);
        self.write(di)?;
        Ok(id)
    }

    /// Modifica una tarea. Si cambia `project`, se mueve a otro archivo.
    pub fn update(&mut self, id: &str, f: impl FnOnce(&mut Task)) -> Result<()> {
        self.reload()?;
        let pos = self.find_or_err(id)?;
        let before = self.task_at(pos).cloned().unwrap();
        let mut t = before.clone();
        f(&mut t);
        let same = t.project.to_lowercase() == self.docs[pos.0].project.to_lowercase();
        if same {
            t.project = self.docs[pos.0].project.clone();
            // Si cambió la descripción o las notas, se reescribe el bloque.
            if t.description != before.description || t.notes != before.notes {
                let end = self.block_end(pos);
                let new: Vec<Line> = block_lines(&t).into_iter().map(Line::Other).collect();
                self.docs[pos.0].lines.splice(pos.1 + 1..end, new);
            }
            self.set_task(pos, t);
            return self.write(pos.0);
        }
        let target = self.doc_or_create(&t.project)?;
        // `doc_or_create` puede añadir un documento al final: `pos` sigue valiendo.
        self.remove_block(pos);
        self.write(pos.0)?;
        t.section = None;
        t.indent.clear();
        self.insert_task(target, t);
        self.write(target)
    }

    /// Alterna completada/pendiente. Devuelve `true` si quedó completada.
    /// Si se repite, la tarea pendiente conserva el id y avanza de fecha
    /// (como Obsidian Tasks, la ocurrencia hecha queda debajo).
    pub fn toggle(&mut self, id: &str) -> Result<bool> {
        self.reload()?;
        let pos = self.find_or_err(id)?;
        let mut t = self.task_at(pos).cloned().unwrap();
        let today = today();
        if t.done {
            t.set_done(false, today);
            self.set_task(pos, t);
            self.write(pos.0)?;
            return Ok(false);
        }
        match t.complete(today, gen_id(&self.ids())) {
            Completion::Simple(done) => self.set_task(pos, done),
            Completion::Recurring { next, done } => {
                let end = self.block_end(pos);
                self.set_task(pos, next);
                self.docs[pos.0].lines.insert(end, Line::Task { task: done, raw: None });
            }
        }
        self.write(pos.0)?;
        Ok(true)
    }

    pub fn delete(&mut self, id: &str) -> Result<()> {
        self.reload()?;
        let pos = self.find_or_err(id)?;
        self.remove_block(pos);
        self.write(pos.0)
    }

    pub fn snapshot(&self) -> Result<FileSnapshot> {
        self.docs
            .iter()
            .map(|d| Ok((d.path.clone(), fs::read_to_string(&d.path)?)))
            .collect()
    }

    /// Restaura una copia. Los archivos creados después solo se borran si no
    /// tienen tareas (para no perder nada escrito desde otro dispositivo).
    pub fn restore(&mut self, snap: &FileSnapshot) -> Result<()> {
        self.reload()?;
        for d in &self.docs {
            if !snap.iter().any(|(p, _)| p == &d.path) && !d.has_tasks() {
                fs::remove_file(&d.path)?;
            }
        }
        for (p, content) in snap {
            if fs::read_to_string(p).ok().as_deref() != Some(content.as_str()) {
                write_atomic(p, content)?;
            }
        }
        self.reload()
    }
}

/// Escribe las tareas de cualquier almacén como `.md` (un archivo por proyecto,
/// `##` por sección, notas con sangría). Devuelve cuántos proyectos escribió.
pub fn export(store: &crate::store::Store, dir: &Path) -> Result<usize> {
    let projects = store.projects();
    for p in &projects {
        let title = p.rsplit('/').next().unwrap_or(p);
        let mut out = format!("# {title}\n");
        let tasks: Vec<&Task> = store.iter().map(|(_, t)| t).filter(|t| &t.project == p).collect();
        // Secciones en orden de aparición, primero las tareas sin sección.
        let mut sections: Vec<Option<String>> = vec![None];
        for t in &tasks {
            if !sections.contains(&t.section) {
                sections.push(t.section.clone());
            }
        }
        for sec in sections {
            let group: Vec<&&Task> = tasks.iter().filter(|t| t.section == sec).collect();
            if group.is_empty() && sec.is_some() {
                continue;
            }
            out.push('\n');
            if let Some(name) = &sec {
                out.push_str(&format!("## {name}\n\n"));
            }
            for t in group {
                out.push_str(&markdown::render_line(t));
                out.push('\n');
                for l in block_lines(t) {
                    out.push_str(&l);
                    out.push('\n');
                }
            }
        }
        let path = dir.join(format!("{p}.md"));
        if let Some(parent) = path.parent() {
            fs::create_dir_all(parent)?;
        }
        write_atomic(&path, &out)?;
    }
    Ok(projects.len())
}

#[cfg(test)]
pub mod tests {
    use super::*;

    pub fn temp_store(name: &str) -> FileStore {
        let dir = std::env::temp_dir().join(format!("tasktui-test-{name}-{}", std::process::id()));
        let _ = fs::remove_dir_all(&dir);
        FileStore::open(dir).unwrap()
    }

    #[test]
    fn flujo_basico() {
        let mut s = temp_store("flujo");
        fs::write(
            s.dir.join("Casa.md"),
            "# Casa\nNotas libres\n\n## Compras\n- [ ]   Leche   #super\n",
        )
        .unwrap();
        s.reload().unwrap();
        // Se asignó id y la sección se detectó
        let (_, leche) = s.iter().find(|(_, t)| t.text == "Leche").unwrap();
        assert_eq!(leche.section.as_deref(), Some("Compras"));
        let id_leche = leche.id.clone().unwrap();

        let id = s.add(Task::new("Pan"), "casa").unwrap();
        let content = fs::read_to_string(s.dir.join("Casa.md")).unwrap();
        assert!(content.starts_with("# Casa\nNotas libres\n"));
        assert!(content.contains(&format!("- [ ] Pan 🆔 {id}")));

        // Mover de proyecto crea el archivo
        s.update(&id_leche, |t| t.project = "Super/Semanal".into()).unwrap();
        assert!(s.dir.join("Super/Semanal.md").exists());
        assert_eq!(s.task_at(s.find(&id_leche).unwrap()).unwrap().project, "Super/Semanal");

        // Deshacer
        let snap = s.snapshot().unwrap();
        s.delete(&id).unwrap();
        assert!(s.find(&id).is_none());
        s.restore(&snap).unwrap();
        assert!(s.find(&id).is_some());
        let _ = fs::remove_dir_all(&s.dir);
    }

    #[test]
    fn completar_recurrente() {
        let mut s = temp_store("rec");
        let mut t = Task::new("Gym");
        t.recurrence = crate::model::Recurrence::parse_obsidian("every day");
        t.due = Some(today());
        let id = s.add(t, INBOX).unwrap();
        assert!(s.toggle(&id).unwrap());
        let pend = s.task_at(s.find(&id).unwrap()).unwrap();
        assert!(!pend.done);
        assert_eq!(pend.due, Some(today() + chrono::Duration::days(1)));
        assert_eq!(s.iter().filter(|(_, t)| t.done).count(), 1);
        let _ = fs::remove_dir_all(&s.dir);
    }

    #[test]
    fn notas_viajan_con_la_tarea() {
        let mut s = temp_store("notas");
        fs::write(
            s.dir.join("Inbox.md"),
            "# Inbox\n\n- [ ] Uno 🆔 aaaaa\n    - nota 1\n  texto suelto\n- [ ] Dos 🆔 bbbbb\n\nFin\n",
        )
        .unwrap();
        s.reload().unwrap();
        let uno = s.task_at(s.find("aaaaa").unwrap()).unwrap();
        assert_eq!(uno.notes, vec![Note::new("nota 1", None)]);
        assert_eq!(uno.description, "texto suelto");
        s.add_note("bbbbb", "nota de dos").unwrap();
        s.update("aaaaa", |t| t.project = "Otro".into()).unwrap();
        let inbox = fs::read_to_string(s.dir.join("Inbox.md")).unwrap();
        assert_eq!(inbox, format!("# Inbox\n\n- [ ] Dos 🆔 bbbbb\n    - {}: nota de dos\n\nFin\n", today()));
        let otro = fs::read_to_string(s.dir.join("Otro.md")).unwrap();
        assert!(otro.contains("- [ ] Uno 🆔 aaaaa\n    texto suelto\n    - nota 1\n"), "{otro}");
        // Cambiar la descripción reescribe el bloque y conserva las notas.
        s.update("aaaaa", |t| t.description = "línea 1\nlínea 2".into()).unwrap();
        let otro = fs::read_to_string(s.dir.join("Otro.md")).unwrap();
        assert!(otro.contains("- [ ] Uno 🆔 aaaaa\n    línea 1\n    línea 2\n    - nota 1\n"), "{otro}");
        let uno = s.task_at(s.find("aaaaa").unwrap()).unwrap();
        assert_eq!(uno.description, "línea 1\nlínea 2");
        s.delete("aaaaa").unwrap();
        assert!(!fs::read_to_string(s.dir.join("Otro.md")).unwrap().contains("nota"));
        let _ = fs::remove_dir_all(&s.dir);
    }
}
