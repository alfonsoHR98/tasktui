//! tasktui — tareas en la terminal, guardadas como Markdown compatible con Obsidian.

mod config;
mod files;
mod reminders;
mod remote;
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
    /// Añade una tarea: tasktui add Pagar luz mañana 18:00 p1 #casa +Finanzas limit 20/11
    Add {
        #[arg(required = true)]
        text: Vec<String>,
        /// Descripción de la tarea
        #[arg(short, long)]
        description: Option<String>,
    },
    /// Lista tareas: today (por defecto), inbox, upcoming, all, +Proyecto, #etiqueta
    #[command(alias = "ls")]
    List {
        view: Option<String>,
        /// Incluir completadas
        #[arg(short, long)]
        all: bool,
        /// Filtrar por texto
        #[arg(short, long, alias = "buscar")]
        search: Option<String>,
    },
    /// Muestra una tarea con su descripción y notas
    Show { id: String },
    /// Completa (o reabre) tareas por id
    Done {
        #[arg(required = true)]
        ids: Vec<String>,
    },
    /// Cambia solo lo indicado: tasktui edit a1b2c viernes 18:00 p2 limit 20/11
    Edit {
        id: String,
        text: Vec<String>,
        /// Quita la fecha, la hora y la repetición
        #[arg(long, alias = "sin-fecha")]
        no_date: bool,
        /// Quita la fecha límite
        #[arg(long)]
        no_deadline: bool,
    },
    /// Cambia la descripción: tasktui describe a1b2c texto (sin texto abre $EDITOR)
    Describe { id: String, text: Vec<String> },
    /// Añade una nota con la fecha de hoy: tasktui note a1b2c Llamó, sigue pendiente
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
    /// Espejo con la app Recordatorios (macOS): tasktui sync [--auto on|off] [--agent on|off]
    Sync {
        /// Sincronizar en segundo plano tras cada cambio (on) o dejar de hacerlo (off)
        #[arg(long, value_parser = ["on", "off"])]
        auto: Option<String>,
        /// Sincronizar cada N minutos con launchd (on), o quitarlo (off)
        #[arg(long, alias = "agente", value_parser = ["on", "off"])]
        agent: Option<String>,
        /// Minutos entre sincronizaciones del agente
        #[arg(long, alias = "cada", default_value_t = 10)]
        every: u32,
        /// Sin salida (para ejecutarlo en segundo plano)
        #[arg(short, long)]
        quiet: bool,
    },
    /// Lista los proyectos
    Projects,
    /// Lista las etiquetas
    Labels,
    /// Muestra la carpeta de datos (modo local), o la cambia si se da una ruta
    Dir { path: Option<String> },
    /// Servidor: tasktui server [connect <url> | local | remote | push]
    #[command(alias = "servidor")]
    Server {
        #[command(subcommand)]
        action: Option<ServerCmd>,
    },
    /// Escribe todas las tareas como .md (formato Obsidian Tasks) en una carpeta
    #[command(alias = "exportar")]
    Export { dir: String },
}

#[derive(Subcommand)]
enum ServerCmd {
    /// Guarda la URL y el token (lo pide por la entrada estándar) y activa el modo remoto
    #[command(alias = "conectar")]
    Connect { url: String },
    /// Usar los .md locales (el servidor queda configurado)
    Local,
    /// Usar el servidor
    #[command(alias = "remoto")]
    Remote,
    /// Sube las tareas de los .md locales al servidor (conserva ids; no borra nada)
    #[command(alias = "subir")]
    Push,
}

fn task_json(t: &Task) -> Value {
    serde_json::to_value(TaskDto::from(t)).unwrap_or(Value::Null)
}

fn task_plain(t: &Task, show_project: bool) -> String {
    let today = today();
    let check = if t.done { "✓" } else { "○" };
    let prio = if t.priority < 4 { format!("p{}", t.priority) } else { "  ".into() };
    let mut due = t.due.map(|d| dates::rel(d, today)).unwrap_or_default();
    if let Some(h) = t.time {
        due.push_str(&format!(" {}", h.format("%H:%M")));
    }
    let mut s = format!("{:<6} {check} {prio} {due:<8} {}", t.id.as_deref().unwrap_or("-"), t.text);
    if let Some(d) = t.deadline {
        s.push_str(&format!(" 🎯{}", dates::rel(d, today)));
    }
    if !t.description.trim().is_empty() {
        s.push_str(" 📝");
    }
    if !t.notes.is_empty() {
        s.push_str(&format!(" 💬{}", t.notes.len()));
    }
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

    if let Some(Cmd::Server { action }) = &cli.cmd {
        return server(action.as_ref(), cli.json);
    }

    let mut store = Store::open()?;
    if let Some(n) = store.notice() {
        eprintln!("{n}");
    }
    let json = cli.json;
    let Some(cmd) = cli.cmd else {
        return tui::run(store);
    };
    let today = today();

    let mutating = matches!(
        cmd,
        Cmd::Add { .. }
            | Cmd::Done { .. }
            | Cmd::Edit { .. }
            | Cmd::Describe { .. }
            | Cmd::Note { .. }
            | Cmd::Rm { .. }
    );
    match cmd {
        Cmd::Dir { .. } | Cmd::Server { .. } => unreachable!(),
        Cmd::Export { dir } => {
            let dir = std::path::PathBuf::from(&dir);
            let n = files::export(&store, &dir)?;
            println!("Exportados {n} proyectos a {}", dir.display());
        }
        Cmd::Sync { auto, agent, every, quiet } => {
            if let Some(a) = agent {
                let path = reminders::set_agent(a == "on", every)?;
                if !quiet {
                    if a == "on" {
                        println!("Agente instalado: sincroniza cada {every} min ({})", path.display());
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
        Cmd::Add { text, description } => {
            let q = quick::parse_quick(&text.join(" "), today);
            if q.text.is_empty() {
                bail!("La tarea necesita texto");
            }
            let mut t = q.into_task(INBOX);
            t.description = description.unwrap_or_default().trim().to_string();
            let project = t.project.clone();
            let id = store.add(t, &project)?;
            let t = store.task_at(store.find(&id).unwrap()).unwrap();
            if json {
                println!("{}", task_json(t));
            } else {
                println!("Añadida: {}", task_plain(t, true));
            }
        }
        Cmd::List { view, all, search } => {
            let view = match view.as_deref() {
                None => View::Today,
                Some(v) => View::parse(v).ok_or_else(|| {
                    anyhow::anyhow!("Vista desconocida «{v}». Usa: today, inbox, upcoming, all, +Proyecto, #etiqueta")
                })?,
            };
            let groups = store.view(&view, today, all, search.as_deref().unwrap_or(""));
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
        Cmd::Show { id } => {
            let t = store.find(&id).and_then(|p| store.task_at(p)).ok_or_else(|| anyhow::anyhow!("No existe la tarea {id}"))?;
            if json {
                println!("{}", task_json(t));
            } else {
                println!("{}", task_plain(t, true));
                if let Some(d) = t.deadline {
                    println!("  🎯 límite {}", dates::long(d, today));
                }
                for l in t.description.lines() {
                    println!("  {l}");
                }
                for n in &t.notes {
                    match n.date {
                        Some(d) => println!("  › {}: {}", dates::rel(d, today), n.text),
                        None => println!("  › {}", n.text),
                    }
                }
            }
        }
        Cmd::Describe { id, text } => {
            let desc = if text.is_empty() {
                let current = store
                    .find(&id)
                    .and_then(|p| store.task_at(p))
                    .map(|t| t.description.clone())
                    .ok_or_else(|| anyhow::anyhow!("No existe la tarea {id}"))?;
                edit_text(&id, &current)?
            } else {
                text.join(" ")
            };
            store.update(&id, |t| t.description = desc.trim().to_string())?;
            let t = store.task_at(store.find(&id).unwrap()).unwrap();
            if json {
                println!("{}", task_json(t));
            } else {
                println!("Descripción guardada: {}", task_plain(t, true));
            }
        }
        Cmd::Edit { id, text, no_date, no_deadline } => {
            let q = quick::parse_quick(&text.join(" "), today);
            store.update(&id, |t| {
                q.apply_partial(t);
                if no_date {
                    t.due = None;
                    t.time = None;
                    t.recurrence = None;
                }
                if no_deadline {
                    t.deadline = None;
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

/// `tasktui server …`
fn server(action: Option<&ServerCmd>, json: bool) -> Result<()> {
    match action {
        None => {
            let modo = match config::mode()? {
                config::Mode::Local(d) => format!("local ({})", d.display()),
                config::Mode::Remote(s) => format!("remoto ({})", s.url),
            };
            let estado = match config::server() {
                Some(s) => match remote::Client::new(&s).get("projects") {
                    Ok(_) => format!("{} responde y acepta el token", s.url),
                    Err(e) => format!("{}: {e}", s.url),
                },
                None => "sin servidor configurado (tasktui server connect <url>)".into(),
            };
            if json {
                println!("{}", serde_json::json!({ "mode": modo, "server": estado }));
            } else {
                println!("Modo: {modo}\nServidor: {estado}");
            }
        }
        Some(ServerCmd::Connect { url }) => {
            eprint!("Token del servidor: ");
            let mut token = String::new();
            std::io::stdin().read_line(&mut token)?;
            let server = config::Server { url: url.trim_end_matches('/').to_string(), token: token.trim().to_string() };
            if server.token.is_empty() {
                bail!("El token está vacío");
            }
            remote::Client::new(&server).get("projects")?;
            config::save_server(&server)?;
            config::set_remote(true)?;
            println!("Conectado a {} (modo remoto). Token en {}", server.url, config::server_path().display());
        }
        Some(ServerCmd::Local) => {
            config::set_remote(false)?;
            println!("Modo local: {}", config::data_dir()?.display());
        }
        Some(ServerCmd::Remote) => {
            let Some(s) = config::server() else {
                bail!("No hay servidor configurado: tasktui server connect <url>");
            };
            config::set_remote(true)?;
            println!("Modo remoto: {}", s.url);
        }
        Some(ServerCmd::Push) => {
            let Some(s) = config::server() else {
                bail!("No hay servidor configurado: tasktui server connect <url>");
            };
            let local = files::FileStore::open(config::data_dir()?)?;
            let tareas: Vec<TaskDto> = local.iter().map(|(_, t)| TaskDto::from(t)).collect();
            let n = remote::Client::new(&s).import(&local.projects(), &tareas)?;
            println!("Subidas {n} tareas de {} a {}", local.dir.display(), s.url);
        }
    }
    Ok(())
}

/// Edita un texto en `$VISUAL`/`$EDITOR` (o `nano`) y devuelve el resultado.
pub fn edit_text(id: &str, current: &str) -> Result<String> {
    let path = std::env::temp_dir().join(format!("tasktui-{id}.md"));
    std::fs::write(&path, current)?;
    let editor = std::env::var("VISUAL").or_else(|_| std::env::var("EDITOR")).unwrap_or_else(|_| "nano".into());
    let mut parts = editor.split_whitespace();
    let status = std::process::Command::new(parts.next().unwrap_or("nano")).args(parts).arg(&path).status()?;
    let text = std::fs::read_to_string(&path)?;
    let _ = std::fs::remove_file(&path);
    if !status.success() {
        bail!("El editor terminó con error");
    }
    Ok(text.trim_end().to_string())
}
