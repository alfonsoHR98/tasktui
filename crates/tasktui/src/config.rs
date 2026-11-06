//! Configuración: `~/.config/tasktui/config.toml`
//!
//! ```toml
//! data_dir = "~/Library/Mobile Documents/iCloud~md~obsidian/Documents/MiVault/Tareas"
//! recordatorios = true   # espejo automático hacia Recordatorios
//! ```
//! La variable `TASKTUI_DIR` tiene prioridad sobre el archivo.

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
