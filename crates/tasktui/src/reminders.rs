//! Espejo hacia la app Recordatorios de Apple (solo macOS).
//!
//! Los `.md` son la fuente de verdad. `tasktui sync`:
//! 1. Si completaste en Recordatorios un recordatorio de tasktui, completa la
//!    tarea en el `.md` (si se repite, avanza a la siguiente fecha).
//! 2. Deja en la lista «tasktui» un recordatorio por cada tarea pendiente con
//!    fecha (título, fecha de todo el día, prioridad, proyecto y notas).
//! 3. Lo que crees tú en la lista «tasktui» (a mano o con Siri) se importa
//!    como tarea nueva. El título acepta la captura rápida (`+Proyecto`, `#tag`,
//!    `p1`, `mañana`...); la fecha y prioridad del recordatorio mandan.
//! 4. Borra de esa lista los recordatorios de tasktui cuya tarea ya no está
//!    pendiente o ya no tiene fecha.
//!
//! La relación tarea ↔ recordatorio se guarda en
//! `~/.config/tasktui/recordatorios.json` (los identificadores de EventKit son
//! locales a cada Mac, por eso no van en el vault).

use std::collections::{BTreeMap, HashSet};
use std::fs;
use std::path::PathBuf;

use anyhow::{Context, Result};
use chrono::{NaiveDate, NaiveTime};

use crate::config;
use crate::model::Task;
use crate::today;
use crate::quick;
use crate::store::INBOX;
use crate::store::Store;

/// Nombre de la lista en Recordatorios.
pub const LIST: &str = "tasktui";

/// Cómo debería verse el recordatorio de una tarea.
#[derive(Debug, Clone, PartialEq)]
pub struct Wanted {
    pub task_id: String,
    pub title: String,
    pub due: NaiveDate,
    /// Con hora, el recordatorio avisa a esa hora exacta.
    pub time: Option<NaiveTime>,
    /// Prioridad de EventKit: 0 ninguna, 1 alta, 5 media, 9 baja.
    pub priority: usize,
    pub notes: String,
}

/// Un recordatorio que ya está en la lista.
#[derive(Debug, Clone, PartialEq)]
pub struct Existing {
    pub ek_id: String,
    pub title: String,
    pub due: Option<NaiveDate>,
    pub time: Option<NaiveTime>,
    pub priority: usize,
    pub notes: String,
    pub completed: bool,
}

impl Existing {
    fn matches(&self, w: &Wanted) -> bool {
        !self.completed
            && self.title == w.title
            && self.due == Some(w.due)
            && self.time == w.time
            && self.priority == w.priority
            && self.notes == w.notes
    }
}

pub fn ek_priority(p: u8) -> usize {
    match p {
        1 => 1,
        2 => 5,
        3 => 9,
        _ => 0,
    }
}

/// Prioridad de EventKit → tasktui (RFC 5545: 1-4 alta, 5 media, 6-9 baja).
pub fn task_priority(ek: usize) -> u8 {
    match ek {
        1..=4 => 1,
        5 => 2,
        6..=9 => 3,
        _ => 4,
    }
}

/// Tarea nueva a partir de un recordatorio creado fuera de tasktui.
/// Devuelve la tarea y su proyecto.
pub fn import_task(e: &Existing) -> (Task, String) {
    let q = quick::parse_quick(&e.title, today());
    let text = if q.text.is_empty() { e.title.trim().to_string() } else { q.text };
    let mut t = Task::new(&text);
    t.labels = q.labels;
    t.priority = match task_priority(e.priority) {
        4 => q.priority.unwrap_or(4),
        p => p,
    };
    t.due = e.due.or(q.due);
    t.time = e.time.or(q.time);
    t.deadline = q.deadline;
    t.recurrence = q.recurrence;
    if t.recurrence.is_some() && t.due.is_none() {
        t.due = t.recurrence.as_ref().map(|r| r.first(today()));
    }
    // Las notas del recordatorio son la descripción de la tarea.
    t.description = e
        .notes
        .lines()
        .map(str::trim)
        .filter(|l| !l.is_empty())
        .collect::<Vec<_>>()
        .join("\n");
    (t, q.project.unwrap_or_else(|| INBOX.into()))
}

pub fn wanted(t: &Task) -> Option<Wanted> {
    if t.done {
        return None;
    }
    // Notas del recordatorio: proyecto, límite, descripción y comentarios.
    let mut notes = vec![format!("📁 {}", t.project)];
    if let Some(d) = t.deadline {
        notes.push(format!("🎯 Límite: {}", crate::dates::long(d, today())));
    }
    if !t.description.trim().is_empty() {
        notes.push(t.description.trim().to_string());
    }
    for n in &t.notes {
        notes.push(match n.date {
            Some(d) => format!("• {}: {}", crate::dates::rel(d, today()), n.text),
            None => format!("• {}", n.text),
        });
    }
    Some(Wanted {
        task_id: t.id.clone()?,
        title: t.text.clone(),
        due: t.due?,
        time: t.time,
        priority: ek_priority(t.priority),
        notes: notes.join("\n"),
    })
}

/// Tarea id → identificador del recordatorio.
pub type Mapping = BTreeMap<String, String>;

#[derive(Debug, Default, PartialEq)]
pub struct Plan {
    /// Recordatorio nuevo para estas tareas.
    pub create: Vec<Wanted>,
    /// (ek_id, cómo debe quedar).
    pub update: Vec<(String, Wanted)>,
    /// Recordatorios de tasktui que sobran.
    pub remove: Vec<String>,
    /// Recordatorios creados fuera de tasktui: se vuelven tareas.
    pub import: Vec<Existing>,
}

/// Tareas pendientes cuyo recordatorio marcaste como completado.
pub fn completed_in_reminders(
    map: &Mapping,
    existing: &[Existing],
    pending: &HashSet<String>,
) -> Vec<String> {
    map.iter()
        .filter(|(tid, ek)| {
            pending.contains(*tid) && existing.iter().any(|e| &e.ek_id == *ek && e.completed)
        })
        .map(|(tid, _)| tid.clone())
        .collect()
}

/// Decide qué crear, actualizar, borrar e importar. Actualiza `map`.
///
/// - Un recordatorio que estaba en `map` es de tasktui: si su tarea ya no se
///   quiere, se borra.
/// - Uno pendiente que nunca estuvo en `map` lo creaste tú: se importa (o, si
///   se perdió la relación y coincide el título, se adopta para no duplicar).
/// - Uno completado que nunca estuvo en `map` se deja en paz.
pub fn plan(want: &[Wanted], existing: &[Existing], map: &mut Mapping) -> Plan {
    let by_id = |id: &str| existing.iter().find(|e| e.ek_id == id);
    let ours: HashSet<String> = map.values().cloned().collect();
    map.retain(|tid, ek| by_id(ek).is_some() && want.iter().any(|w| &w.task_id == tid));

    let mut used: HashSet<String> = map.values().cloned().collect();
    let mut p = Plan::default();
    for w in want {
        if !map.contains_key(&w.task_id) {
            let adopt = existing
                .iter()
                .find(|e| !e.completed && e.title == w.title && !used.contains(&e.ek_id));
            if let Some(e) = adopt {
                used.insert(e.ek_id.clone());
                map.insert(w.task_id.clone(), e.ek_id.clone());
            }
        }
        match map.get(&w.task_id).and_then(|ek| by_id(ek)) {
            Some(e) if e.matches(w) => {}
            Some(e) => p.update.push((e.ek_id.clone(), w.clone())),
            None => p.create.push(w.clone()),
        }
    }
    for e in existing.iter().filter(|e| !used.contains(&e.ek_id)) {
        if ours.contains(&e.ek_id) {
            p.remove.push(e.ek_id.clone());
        } else if !e.completed {
            p.import.push(e.clone());
        }
    }
    p
}

fn state_path() -> PathBuf {
    config::config_path().with_file_name("recordatorios.json")
}

fn load_map() -> Mapping {
    fs::read_to_string(state_path())
        .ok()
        .and_then(|s| serde_json::from_str(&s).ok())
        .unwrap_or_default()
}

fn save_map(map: &Mapping) -> Result<()> {
    let p = state_path();
    fs::create_dir_all(p.parent().unwrap())?;
    fs::write(&p, serde_json::to_string_pretty(map)?)?;
    Ok(())
}

#[derive(Debug, Default)]
pub struct Report {
    pub completed: Vec<String>,
    pub imported: Vec<String>,
    pub created: usize,
    pub updated: usize,
    pub removed: usize,
}

/// ¿Se está usando una configuración temporal (`TASKTUI_DIR` o
/// `TASKTUI_SERVER`, p. ej. en pruebas)? Entonces no se toca Recordatorios:
/// la lista y la relación tarea ↔ recordatorio son de tu configuración normal.
fn env_override() -> bool {
    std::env::var_os("TASKTUI_DIR").is_some() || std::env::var_os("TASKTUI_SERVER").is_some()
}

/// Sincroniza. Se serializa con un candado para que dos `sync` simultáneos
/// (TUI + CLI) no creen duplicados.
pub fn sync(store: &mut Store) -> Result<Report> {
    if env_override() {
        anyhow::bail!(
            "Con TASKTUI_DIR o TASKTUI_SERVER definidos no se sincroniza Recordatorios \
             (es solo para tu configuración normal)"
        );
    }
    let lock_path = state_path().with_extension("lock");
    fs::create_dir_all(lock_path.parent().unwrap())?;
    let lock = fs::File::create(&lock_path)?;
    lock.lock().context("No pude obtener el candado de sincronización")?;

    let ek = backend::Reminders::open()?;
    let mut map = load_map();
    let mut report = Report::default();

    // 1. Lo completado en Recordatorios se completa en el .md.
    store.reload()?;
    let pending: HashSet<String> = store
        .iter()
        .filter(|(_, t)| !t.done)
        .filter_map(|(_, t)| t.id.clone())
        .collect();
    let existing = ek.fetch()?;
    for tid in completed_in_reminders(&map, &existing, &pending) {
        store.toggle(&tid)?;
        report.completed.push(tid);
    }

    // 2. Lo creado en Recordatorios entra como tarea.
    store.reload()?;
    let want: Vec<Wanted> = store.iter().filter_map(|(_, t)| wanted(t)).collect();
    let first = plan(&want, &existing, &mut map.clone());
    for e in &first.import {
        let (task, project) = import_task(e);
        let id = store.add(task, &project)?;
        map.insert(id.clone(), e.ek_id.clone());
        report.imported.push(id);
    }
    if !report.imported.is_empty() {
        save_map(&map)?;
    }

    // 3 y 4. Dejar la lista igual a las tareas pendientes con fecha. Los
    // importados ya están en `map`: se ajustan (o se quitan si no tienen fecha).
    store.reload()?;
    let want: Vec<Wanted> = store.iter().filter_map(|(_, t)| wanted(t)).collect();
    let p = plan(&want, &existing, &mut map);
    for (ek_id, w) in &p.update {
        ek.update(ek_id, w)?;
    }
    for w in &p.create {
        let id = ek.create(w)?;
        map.insert(w.task_id.clone(), id);
    }
    for ek_id in &p.remove {
        ek.remove(ek_id)?;
    }
    ek.commit()?;
    save_map(&map)?;
    report.created = p.create.len();
    report.updated = p.update.len();
    report.removed = p.remove.len();
    Ok(report)
}

const AGENT: &str = "dev.madaradev.tasktui.sync";

fn agent_path() -> PathBuf {
    dirs::home_dir()
        .unwrap_or_default()
        .join(format!("Library/LaunchAgents/{AGENT}.plist"))
}

fn launchctl(args: &[&str]) -> bool {
    std::process::Command::new("launchctl")
        .args(args)
        .stdout(std::process::Stdio::null())
        .stderr(std::process::Stdio::null())
        .status()
        .is_ok_and(|s| s.success())
}

/// Instala (o quita) un agente de launchd que ejecuta `tasktui sync` cada
/// `minutes` minutos y al iniciar sesión. Devuelve la ruta del plist.
pub fn set_agent(on: bool, minutes: u32) -> Result<PathBuf> {
    let path = agent_path();
    let uid = unsafe { libc_getuid() };
    let domain = format!("gui/{uid}");
    let _ = launchctl(&["bootout", &format!("{domain}/{AGENT}")]);
    if !on {
        if path.exists() {
            fs::remove_file(&path)?;
        }
        return Ok(path);
    }
    let exe = std::env::current_exe()?.canonicalize()?;
    let log = dirs::home_dir().unwrap_or_default().join("Library/Logs/tasktui-sync.log");
    let plist = format!(
        r#"<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>{AGENT}</string>
	<key>ProgramArguments</key>
	<array>
		<string>{exe}</string>
		<string>sync</string>
		<string>--quiet</string>
	</array>
	<key>StartInterval</key>
	<integer>{secs}</integer>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Background</string>
	<key>StandardErrorPath</key>
	<string>{log}</string>
</dict>
</plist>
"#,
        exe = exe.display(),
        secs = minutes.max(1) * 60,
        log = log.display(),
    );
    fs::create_dir_all(path.parent().unwrap())?;
    fs::write(&path, plist)?;
    if !launchctl(&["bootstrap", &domain, &path.to_string_lossy()]) {
        anyhow::bail!("launchctl no pudo cargar {}", path.display());
    }
    Ok(path)
}

unsafe extern "C" {
    #[link_name = "getuid"]
    fn libc_getuid() -> u32;
}

/// Lanza `tasktui sync` en segundo plano si el espejo está activado.
/// No bloquea ni falla: si algo sale mal, el siguiente sync lo arregla.
pub fn sync_in_background() {
    if cfg!(test) || env_override() || !config::reminders_enabled() {
        return;
    }
    if let Ok(exe) = std::env::current_exe() {
        let _ = std::process::Command::new(exe)
            .args(["sync", "--quiet"])
            .stdin(std::process::Stdio::null())
            .stdout(std::process::Stdio::null())
            .stderr(std::process::Stdio::null())
            .spawn();
    }
}

#[cfg(target_os = "macos")]
mod backend {
    use std::ptr::NonNull;
    use std::sync::mpsc;
    use std::time::Duration;

    use anyhow::{anyhow, bail, Result};
    use block2::RcBlock;
    use chrono::{NaiveDate, NaiveTime};
    use objc2::rc::Retained;
    use objc2::runtime::Bool;
    use objc2_event_kit::{
        EKAlarm, EKAuthorizationStatus, EKCalendar, EKEntityType, EKEventStore, EKReminder,
    };
    use objc2_foundation::{NSArray, NSCalendar, NSDateComponentUndefined, NSDateComponents, NSError, NSString};

    use super::{Existing, Wanted, LIST};

    /// Envoltorio para pasar objetos de Objective-C entre hilos. EventKit
    /// llama a los bloques en otra cola; aquí solo los movemos, no los usamos
    /// en paralelo.
    struct SendBox<T>(T);
    unsafe impl<T> Send for SendBox<T> {}

    const TIMEOUT: Duration = Duration::from_secs(30);

    pub struct Reminders {
        store: Retained<EKEventStore>,
        list: Retained<EKCalendar>,
    }

    fn err(e: Retained<NSError>) -> anyhow::Error {
        anyhow!("{}", e.localizedDescription())
    }

    fn date_of(c: &NSDateComponents) -> Option<NaiveDate> {
        NaiveDate::from_ymd_opt(c.year() as i32, c.month() as u32, c.day() as u32)
    }

    fn time_of(c: &NSDateComponents) -> Option<NaiveTime> {
        if c.hour() == NSDateComponentUndefined {
            return None;
        }
        let m = if c.minute() == NSDateComponentUndefined { 0 } else { c.minute() };
        NaiveTime::from_hms_opt(c.hour() as u32, m as u32, 0)
    }

    /// Fecha de todo el día, o con hora si la hay.
    fn components(d: NaiveDate, t: Option<NaiveTime>) -> Retained<NSDateComponents> {
        use chrono::{Datelike, Timelike};
        let c = NSDateComponents::new();
        c.setYear(d.year() as isize);
        c.setMonth(d.month() as isize);
        c.setDay(d.day() as isize);
        if let Some(t) = t {
            c.setHour(t.hour() as isize);
            c.setMinute(t.minute() as isize);
        }
        c
    }

    impl Reminders {
        pub fn open() -> Result<Self> {
            let store = unsafe { EKEventStore::new() };
            let status = unsafe { EKEventStore::authorizationStatusForEntityType(EKEntityType::Reminder) };
            if status != EKAuthorizationStatus::FullAccess {
                if status == EKAuthorizationStatus::Denied || status == EKAuthorizationStatus::Restricted {
                    bail!(
                        "Sin acceso a Recordatorios. Actívalo en Ajustes del Sistema › Privacidad y seguridad › \
                         Recordatorios (para tu app de terminal)."
                    );
                }
                let (tx, rx) = mpsc::channel();
                let block = RcBlock::new(move |granted: Bool, _e: *mut NSError| {
                    let _ = tx.send(granted.as_bool());
                });
                unsafe {
                    store.requestFullAccessToRemindersWithCompletion(RcBlock::as_ptr(&block));
                }
                let granted = rx.recv_timeout(Duration::from_secs(120)).unwrap_or(false);
                if !granted {
                    bail!("No se concedió acceso a Recordatorios.");
                }
            }
            let list = Self::find_or_create_list(&store)?;
            Ok(Reminders { store, list })
        }

        fn find_or_create_list(store: &EKEventStore) -> Result<Retained<EKCalendar>> {
            let cals = unsafe { store.calendarsForEntityType(EKEntityType::Reminder) };
            if let Some(c) = cals.iter().find(|c| unsafe { c.title() }.to_string() == LIST) {
                return Ok(c);
            }
            let default = unsafe { store.defaultCalendarForNewReminders() }
                .ok_or_else(|| anyhow!("Recordatorios no tiene una lista por defecto"))?;
            let cal = unsafe { EKCalendar::calendarForEntityType_eventStore(EKEntityType::Reminder, store) };
            unsafe {
                cal.setTitle(&NSString::from_str(LIST));
                cal.setSource(default.source().as_deref());
                store.saveCalendar_commit_error(&cal, true).map_err(err)?;
            }
            Ok(cal)
        }

        fn fetch_raw(&self) -> Result<Vec<Retained<EKReminder>>> {
            let cals = NSArray::from_retained_slice(&[self.list.clone()]);
            let pred = unsafe { self.store.predicateForRemindersInCalendars(Some(&cals)) };
            let (tx, rx) = mpsc::channel();
            let block = RcBlock::new(move |arr: *mut NSArray<EKReminder>| {
                let items: Vec<Retained<EKReminder>> = NonNull::new(arr)
                    .map(|a| unsafe { a.as_ref() }.to_vec())
                    .unwrap_or_default();
                let _ = tx.send(SendBox(items));
            });
            unsafe {
                self.store.fetchRemindersMatchingPredicate_completion(&pred, &block);
            }
            let items = rx
                .recv_timeout(TIMEOUT)
                .map_err(|_| anyhow!("Recordatorios no respondió a tiempo"))?;
            Ok(items.0)
        }

        pub fn fetch(&self) -> Result<Vec<Existing>> {
            Ok(self
                .fetch_raw()?
                .iter()
                .map(|r| unsafe {
                    Existing {
                        ek_id: r.calendarItemIdentifier().to_string(),
                        title: r.title().to_string(),
                        due: r.dueDateComponents().as_deref().and_then(date_of),
                        time: r.dueDateComponents().as_deref().and_then(time_of),
                        priority: r.priority(),
                        notes: r.notes().map(|n| n.to_string()).unwrap_or_default(),
                        completed: r.isCompleted(),
                    }
                })
                .collect())
        }

        fn item(&self, ek_id: &str) -> Option<Retained<EKReminder>> {
            let item = unsafe { self.store.calendarItemWithIdentifier(&NSString::from_str(ek_id)) }?;
            item.downcast::<EKReminder>().ok()
        }

        fn fill(r: &EKReminder, w: &Wanted) {
            unsafe {
                r.setTitle(Some(&NSString::from_str(&w.title)));
                let comps = components(w.due, w.time);
                r.setDueDateComponents(Some(&comps));
                // Con hora, una alarma a esa hora; sin hora, Recordatorios avisa
                // a la hora general de «todo el día».
                r.setAlarms(None);
                if w.time.is_some() {
                    if let Some(at) = NSCalendar::currentCalendar().dateFromComponents(&comps) {
                        r.addAlarm(&EKAlarm::alarmWithAbsoluteDate(&at));
                    }
                }
                r.setPriority(w.priority);
                r.setNotes(Some(&NSString::from_str(&w.notes)));
                if r.isCompleted() {
                    r.setCompleted(false);
                }
            }
        }

        pub fn create(&self, w: &Wanted) -> Result<String> {
            let r = unsafe { EKReminder::reminderWithEventStore(&self.store) };
            unsafe { r.setCalendar(Some(&self.list)) };
            Self::fill(&r, w);
            unsafe { self.store.saveReminder_commit_error(&r, false) }.map_err(err)?;
            Ok(unsafe { r.calendarItemIdentifier() }.to_string())
        }

        pub fn update(&self, ek_id: &str, w: &Wanted) -> Result<()> {
            let Some(r) = self.item(ek_id) else { return Ok(()) };
            Self::fill(&r, w);
            unsafe { self.store.saveReminder_commit_error(&r, false) }.map_err(err)
        }

        pub fn remove(&self, ek_id: &str) -> Result<()> {
            let Some(r) = self.item(ek_id) else { return Ok(()) };
            unsafe { self.store.removeReminder_commit_error(&r, false) }.map_err(err)
        }

        pub fn commit(&self) -> Result<()> {
            unsafe { self.store.commit() }.map_err(err)
        }
    }
}

#[cfg(not(target_os = "macos"))]
mod backend {
    use anyhow::{bail, Result};

    use super::{Existing, Wanted};

    pub struct Reminders;

    impl Reminders {
        pub fn open() -> Result<Self> {
            bail!("La sincronización con Recordatorios solo funciona en macOS")
        }
        pub fn fetch(&self) -> Result<Vec<Existing>> {
            Ok(vec![])
        }
        pub fn create(&self, _w: &Wanted) -> Result<String> {
            Ok(String::new())
        }
        pub fn update(&self, _id: &str, _w: &Wanted) -> Result<()> {
            Ok(())
        }
        pub fn remove(&self, _id: &str) -> Result<()> {
            Ok(())
        }
        pub fn commit(&self) -> Result<()> {
            Ok(())
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn d(s: &str) -> NaiveDate {
        NaiveDate::parse_from_str(s, "%Y-%m-%d").unwrap()
    }

    fn w(id: &str, title: &str) -> Wanted {
        Wanted { task_id: id.into(), title: title.into(), due: d("2026-11-06"), time: None, priority: 0, notes: "📁 Inbox".into() }
    }

    fn e(ek: &str, w: &Wanted) -> Existing {
        Existing {
            ek_id: ek.into(),
            title: w.title.clone(),
            due: Some(w.due),
            time: w.time,
            priority: w.priority,
            notes: w.notes.clone(),
            completed: false,
        }
    }

    #[test]
    fn crea_actualiza_borra() {
        let a = w("a", "Uno");
        let b = w("b", "Dos");
        let mut b_vieja = e("EK2", &b);
        b_vieja.due = Some(d("2026-11-01"));
        let sobra = e("EK3", &w("zz", "Ya no existe"));
        let mut map: Mapping = [("b".into(), "EK2".into()), ("zz".into(), "EK3".into())].into();
        let p = plan(&[a.clone(), b.clone()], &[b_vieja, sobra], &mut map);
        assert_eq!(p.create, vec![a]);
        assert_eq!(p.update, vec![("EK2".to_string(), b)]);
        assert_eq!(p.remove, vec!["EK3".to_string()]);
        assert!(!map.contains_key("zz"));
    }

    #[test]
    fn sin_cambios_no_hace_nada() {
        let a = w("a", "Uno");
        let mut map: Mapping = [("a".into(), "EK1".into())].into();
        assert_eq!(plan(&[a.clone()], &[e("EK1", &a)], &mut map), Plan::default());
    }

    #[test]
    fn adopta_si_se_perdio_la_relacion() {
        let a = w("a", "Uno");
        let mut map = Mapping::new();
        let p = plan(&[a.clone()], &[e("EK1", &a)], &mut map);
        assert_eq!(p, Plan::default());
        assert_eq!(map.get("a").map(String::as_str), Some("EK1"));
    }

    #[test]
    fn importa_lo_creado_fuera_y_borra_lo_propio() {
        let mut mio_huerfano = e("EK1", &w("borrada", "Ya la borré"));
        mio_huerfano.completed = false;
        let tuyo = e("EK2", &w("?", "Comprar leche"));
        let mut tuyo_hecho = e("EK3", &w("?", "Algo viejo"));
        tuyo_hecho.completed = true;
        let mut map: Mapping = [("borrada".into(), "EK1".into())].into();
        let p = plan(&[], &[mio_huerfano, tuyo.clone(), tuyo_hecho], &mut map);
        assert_eq!(p.remove, vec!["EK1".to_string()]);
        assert_eq!(p.import, vec![tuyo]);
        assert!(map.is_empty());
    }

    #[test]
    fn importar_usa_captura_rapida() {
        let mut r = e("EK9", &w("?", "Comprar leche +Casa #super p1"));
        r.due = None;
        r.priority = 0;
        r.notes = "deslactosada\n\n2 litros".into();
        let (t, proj) = import_task(&r);
        assert_eq!(t.text, "Comprar leche");
        assert_eq!(proj, "Casa");
        assert_eq!(t.labels, vec!["super"]);
        assert_eq!(t.priority, 1);
        assert_eq!(t.due, None);
        assert_eq!(t.description, "deslactosada\n2 litros");
        // La fecha y prioridad del recordatorio mandan.
        r.title = "Pagar luz mañana".into();
        r.due = Some(d("2026-12-01"));
        r.priority = 5;
        let (t, proj) = import_task(&r);
        assert_eq!((t.text.as_str(), proj.as_str(), t.due, t.priority), ("Pagar luz", "Inbox", Some(d("2026-12-01")), 2));
    }

    #[test]
    fn completado_en_recordatorios() {
        let a = w("a", "Uno");
        let mut hecho = e("EK1", &a);
        hecho.completed = true;
        let map: Mapping = [("a".into(), "EK1".into())].into();
        let pending: HashSet<String> = ["a".to_string()].into();
        assert_eq!(completed_in_reminders(&map, &[hecho.clone()], &pending), vec!["a"]);
        assert!(completed_in_reminders(&map, &[hecho], &HashSet::new()).is_empty());
    }
}
