//! Configuración: `~/.config/tasktui/config.toml`
//!
//! ```toml
//! data_dir = "~/Library/Mobile Documents/iCloud~md~obsidian/Documents/MiVault/Tareas"
//! recordatorios = true   # espejo automático hacia Recordatorios
//! remoto = true          # usar el servidor de `servidor.toml` (por defecto, si existe)
//! ```
//!
//! El servidor va aparte, en `~/.config/tasktui/servidor.toml` (permisos 600,
//! porque guarda el token):
//!
//! ```toml
//! url = "https://tasks.ejemplo.com"
//! token = "…"
//! ```
//!
//! Variables de entorno (tienen prioridad sobre los archivos):
//! - `TASKTUI_DIR`: modo local con esa carpeta.
//! - `TASKTUI_SERVER` + `TASKTUI_TOKEN`: modo remoto con ese servidor.

use std::fs;
use std::path::PathBuf;

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};

#[derive(Default, Serialize, Deserialize)]
struct Config {
    data_dir: Option<String>,
    /// Espejo automático hacia la app Recordatorios.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    recordatorios: Option<bool>,
    /// `false` fuerza el modo local aunque haya servidor configurado.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    remoto: Option<bool>,
}

/// Servidor de tasktui (ver `crates/tasktui-worker`).
#[derive(Clone, Serialize, Deserialize)]
pub struct Server {
    pub url: String,
    pub token: String,
}

/// Dónde viven las tareas.
pub enum Mode {
    Local(PathBuf),
    Remote(Server),
}

fn home() -> PathBuf {
    dirs::home_dir().unwrap_or_else(|| PathBuf::from("."))
}

pub fn config_path() -> PathBuf {
    home().join(".config/tasktui/config.toml")
}

fn expand(p: &str) -> PathBuf {
    match p.strip_prefix("~/") {
        Some(rest) => home().join(rest),
        None => PathBuf::from(p),
    }
}

fn load() -> Result<Config> {
    let path = config_path();
    if !path.exists() {
        return Ok(Config::default());
    }
    let s = fs::read_to_string(&path)?;
    toml::from_str(&s).with_context(|| format!("Configuración inválida en {}", path.display()))
}

pub fn data_dir() -> Result<PathBuf> {
    if let Ok(d) = std::env::var("TASKTUI_DIR") {
        return Ok(expand(&d));
    }
    Ok(match load()?.data_dir {
        Some(d) => expand(&d),
        None => home().join(".local/share/tasktui"),
    })
}

fn save(cfg: &Config) -> Result<()> {
    let path = config_path();
    fs::create_dir_all(path.parent().unwrap())?;
    fs::write(&path, toml::to_string(cfg)?)?;
    Ok(())
}

pub fn set_data_dir(dir: &str) -> Result<PathBuf> {
    let mut cfg = load()?;
    cfg.data_dir = Some(dir.to_string());
    save(&cfg)?;
    Ok(expand(dir))
}

pub fn reminders_enabled() -> bool {
    load().ok().and_then(|c| c.recordatorios).unwrap_or(false)
}

pub fn set_reminders(on: bool) -> Result<()> {
    let mut cfg = load()?;
    cfg.recordatorios = Some(on);
    save(&cfg)
}

pub fn server_path() -> PathBuf {
    config_path().with_file_name("servidor.toml")
}

pub fn server() -> Option<Server> {
    if let (Ok(url), Ok(token)) = (std::env::var("TASKTUI_SERVER"), std::env::var("TASKTUI_TOKEN")) {
        return Some(Server { url, token });
    }
    let s = fs::read_to_string(server_path()).ok()?;
    toml::from_str(&s).ok()
}

pub fn save_server(server: &Server) -> Result<()> {
    let path = server_path();
    fs::create_dir_all(path.parent().unwrap())?;
    fs::write(&path, toml::to_string(server)?)?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(&path, fs::Permissions::from_mode(0o600))?;
    }
    Ok(())
}

pub fn set_remote(on: bool) -> Result<()> {
    let mut cfg = load()?;
    cfg.remoto = Some(on);
    save(&cfg)
}

/// Modo activo: `TASKTUI_DIR` → local; `remoto = false` → local;
/// si hay `servidor.toml` → remoto; si no → local.
pub fn mode() -> Result<Mode> {
    if std::env::var("TASKTUI_DIR").is_ok() {
        return Ok(Mode::Local(data_dir()?));
    }
    if std::env::var("TASKTUI_SERVER").is_ok() {
        if let Some(s) = server() {
            return Ok(Mode::Remote(s));
        }
    }
    if load()?.remoto != Some(false) {
        if let Some(s) = server() {
            return Ok(Mode::Remote(s));
        }
    }
    Ok(Mode::Local(data_dir()?))
}
