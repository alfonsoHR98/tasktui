//! tasktui — tareas en la terminal, guardadas como Markdown compatible con Obsidian.

mod config;
mod reminders;
mod store;
mod tui;

// El núcleo compartido se usa como si fueran módulos de este crate.
use tasktui_core::{dates, markdown, model, quick, views};

use anyhow::{bail, Result};
use clap::{Parser, Subcommand};
use serde_json::{json, Value};

use chrono::{Local, NaiveDate};
use model::Task;
use store::{Store, INBOX};
use tasktui_core::dto::TaskDto;
use views::View;

/// Fecha local de hoy (el núcleo no usa reloj: se la pasamos).
pub fn today() -> NaiveDate {
    Local::now().date_naive()
}

#[derive(Parser)]
#[command(name = "tasktui", version, about = "Tareas en la terminal (Markdown + Obsidian). Sin argumentos abre la TUI.")]
struct Cli {
    /// Salida en JSON (para scripts y para Claude)
    #[arg(long, global = true)]
    json: bool,
    #[command(subcommand)]
    cmd: Option<Cmd>,
}

#[derive(Subcommand)]
enum Cmd {
    /// Añade una tarea: tasktui add Pagar luz mañana p1 #casa +Finanzas
    Add {
        #[arg(required = true)]
        text: Vec<String>,
    },
    /// Lista tareas: hoy (por defecto), inbox, proximos, todas, +Proyecto, #etiqueta
    #[command(alias = "ls")]
    List {
        vista: Option<String>,
        /// Incluir completadas
        #[arg(short, long)]
        all: bool,
        /// Filtrar por texto
        #[arg(short, long)]
        buscar: Option<String>,
    },
    /// Completa (o reabre) tareas por id
    Done {
        #[arg(required = true)]
        ids: Vec<String>,
    },
    /// Cambia solo lo indicado: tasktui edit a1b2c viernes p2
    Edit {
        id: String,
        text: Vec<String>,
        /// Quita la fecha y la repetición
        #[arg(long)]
        sin_fecha: bool,
    },
    /// Añade una nota debajo de la tarea: tasktui note a1b2c Llamó, sigue pendiente
    Note {
        id: String,
        #[arg(required = true)]
        text: Vec<String>,
    },
    /// Elimina tareas por id
    Rm {
        #[arg(required = true)]
        ids: Vec<String>,
    },
    /// Espejo hacia la app Recordatorios: tasktui sync [--auto on|off]
    Sync {
        /// Sincronizar solo tras cada cambio (on) o dejar de hacerlo (off)
        #[arg(long, value_parser = ["on", "off"])]
        auto: Option<String>,
        /// Sincronizar cada N minutos con launchd (on), o quitarlo (off)
        #[arg(long, value_parser = ["on", "off"])]
        agente: Option<String>,
        /// Minutos entre sincronizaciones del agente
        #[arg(long, default_value_t = 10)]
        cada: u32,
        /// Sin salida (para ejecutarlo en segundo plano)
        #[arg(short, long)]
        quiet: bool,
    },
    /// Lista los proyectos
    Projects,
    /// Lista las etiquetas
    Labels,
    /// Muestra la carpeta de datos, o la cambia si se da una ruta
    Dir { path: Option<String> },
}

fn task_json(t: &Task) -> Value {
    serde_json::to_value(TaskDto::from(t)).unwrap_or(Value::Null)
}

fn task_plain(t: &Task, show_project: bool) -> String {
    let today = today();
    let check = if t.done { "✓" } else { "○" };
    let prio = if t.priority < 4 { format!("p{}", t.priority) } else { "  ".into() };
    let due = t.due.map(|d| dates::rel(d, today)).unwrap_or_default();
    let mut s = format!("{:<6} {check} {prio} {due:<8} {}", t.id.as_deref().unwrap_or("-"), t.text);
    if t.recurrence.is_some() {
        s.push_str(" ↻");
    }
    for l in &t.labels {
        s.push_str(&format!(" #{l}"));
    }
    if show_project {
        s.push_str(&format!("  · {}", t.project));
    }
    s
}

fn main() -> Result<()> {
    let cli = Cli::parse();
    if let Some(Cmd::Dir { path }) = &cli.cmd {
        let dir = match path {
            Some(p) => {
                let d = config::set_data_dir(p)?;
                println!("Carpeta configurada en {}", config::config_path().display());
                d
            }
            None => config::data_dir()?,
        };
        println!("{}", dir.display());
        return Ok(());
    }

    let mut store = Store::open(config::data_dir()?)?;
    let json = cli.json;
    let Some(cmd) = cli.cmd else {
        return tui::run(store);
    };
    let today = today();

    let mutating = matches!(
        cmd,
        Cmd::Add { .. } | Cmd::Done { .. } | Cmd::Edit { .. } | Cmd::Note { .. } | Cmd::Rm { .. }
    );
    match cmd {
        Cmd::Dir { .. } => unreachable!(),
        Cmd::Sync { auto, agente, cada, quiet } => {
            if let Some(a) = agente {
                let path = reminders::set_agent(a == "on", cada)?;
                if !quiet {
                    if a == "on" {
                        println!("Agente instalado: sincroniza cada {cada} min ({})", path.display());
                    } else {
                        println!("Agente quitado");
                    }
                }
                if a == "off" {
                    return Ok(());
                }
            }
            if let Some(a) = auto {
                config::set_reminders(a == "on")?;
                if !quiet {
                    println!(
                        "Sincronización automática {}",
                        if a == "on" { "activada" } else { "desactivada" }
                    );
                }
            }
            let r = reminders::sync(&mut store)?;
            if json {
                println!(
                    "{}",
                    json!({"completed": r.completed, "imported": r.imported, "created": r.created, "updated": r.updated, "removed": r.removed})
                );
            } else if !quiet {
                for id in &r.completed {
                    println!("Completada desde Recordatorios: {id}");
                }
                for id in &r.imported {
                    if let Some(t) = store.find(id).and_then(|p| store.task_at(p)) {
                        println!("Importada desde Recordatorios: {}", task_plain(t, true));
                    }
                }
                println!(
                    "Recordatorios («{}»): {} nuevos, {} actualizados, {} quitados",
                    reminders::LIST,
                    r.created,
                    r.updated,
                    r.removed
                );
            }
        }
        Cmd::Add { text } => {
            let q = quick::parse_quick(&text.join(" "), today);
            if q.text.is_empty() {
                bail!("La tarea necesita texto");
            }
            let mut t = Task::new(&q.text);
            t.labels = q.labels;
            t.priority = q.priority.unwrap_or(4);
            t.due = q.due;
            t.recurrence = q.recurrence;
            let project = q.project.unwrap_or_else(|| INBOX.into());
            let id = store.add(t, &project)?;
            let t = store.task_at(store.find(&id).unwrap()).unwrap();
            if json {
                println!("{}", task_json(t));
            } else {
                println!("Añadida: {}", task_plain(t, true));
            }
        }
        Cmd::List { vista, all, buscar } => {
            let view = match vista.as_deref() {
                None => View::Today,
                Some(v) => View::parse(v).ok_or_else(|| {
                    anyhow::anyhow!("Vista desconocida «{v}». Usa: hoy, inbox, proximos, todas, +Proyecto, #etiqueta")
                })?,
            };
            let groups = store.view(&view, today, all, buscar.as_deref().unwrap_or(""));
            let show_project = !matches!(view, View::Project(_) | View::Inbox | View::All);
            if json {
                let tasks: Vec<Value> = groups
                    .iter()
                    .flat_map(|g| g.items.iter())
                    .filter_map(|p| store.task_at(*p).map(task_json))
                    .collect();
                println!("{}", serde_json::to_string_pretty(&tasks)?);
            } else {
                if groups.is_empty() {
                    println!("Nada pendiente en {}", view.title());
                }
                for g in groups {
                    if let Some(t) = g.title {
                        println!("\n{t}");
                    }
                    for p in g.items {
                        if let Some(t) = store.task_at(p) {
                            println!("  {}", task_plain(t, show_project));
                        }
                    }
                }
            }
        }
        Cmd::Done { ids } => {
            for id in ids {
                let done = store.toggle(&id)?;
                println!("{} {id}", if done { "✓ completada" } else { "○ reabierta" });
            }
        }
        Cmd::Edit { id, text, sin_fecha } => {
            let q = quick::parse_quick(&text.join(" "), today);
            store.update(&id, |t| {
                if !q.text.is_empty() {
                    t.text = q.text;
                }
                if !q.labels.is_empty() {
                    t.labels = q.labels;
                }
                if let Some(p) = q.priority {
                    t.priority = p;
                }
                if q.due.is_some() {
                    t.due = q.due;
                }
                if q.recurrence.is_some() {
                    t.recurrence = q.recurrence;
                }
                if let Some(p) = q.project {
                    t.project = p;
                }
                if sin_fecha {
                    t.due = None;
                    t.recurrence = None;
                }
            })?;
            let t = store.task_at(store.find(&id).unwrap()).unwrap();
            if json {
                println!("{}", task_json(t));
            } else {
                println!("Guardada: {}", task_plain(t, true));
            }
        }
        Cmd::Note { id, text } => {
            store.add_note(&id, &text.join(" "))?;
            let t = store.task_at(store.find(&id).unwrap()).unwrap();
            if json {
                println!("{}", task_json(t));
            } else {
                println!("Nota añadida a {}", task_plain(t, true));
            }
        }
        Cmd::Rm { ids } => {
            for id in ids {
                store.delete(&id)?;
                println!("Eliminada {id}");
            }
        }
        Cmd::Projects => {
            let ps: Vec<(String, usize)> = store
                .projects()
                .into_iter()
                .map(|p| {
                    let n = store.iter().filter(|(_, t)| !t.done && t.project == p).count();
                    (p, n)
                })
                .collect();
            if json {
                let v: Vec<Value> = ps.iter().map(|(p, n)| json!({"name": p, "open": n})).collect();
                println!("{}", serde_json::to_string_pretty(&v)?);
            } else {
                for (p, n) in ps {
                    println!("{n:>4}  {p}");
                }
            }
        }
        Cmd::Labels => {
            let ls = store.labels();
            if json {
                let v: Vec<Value> = ls.iter().map(|(l, n)| json!({"name": l, "open": n})).collect();
                println!("{}", serde_json::to_string_pretty(&v)?);
            } else {
                for (l, n) in ls {
                    println!("{n:>4}  #{l}");
                }
            }
        }
    }
    if mutating {
        reminders::sync_in_background();
    }
    Ok(())
}
