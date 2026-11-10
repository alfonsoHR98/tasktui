//! Núcleo de tasktui, compartido por la TUI/CLI y el servidor.
//!
//! No usa reloj, archivos ni red: quien llama pasa la fecha de «hoy». Así
//! compila igual a nativo que a wasm (Cloudflare Workers).
//!
//! - [`model`]: `Task` y `Recurrence`.
//! - [`quick`]: captura rápida en español (`Pagar luz mañana p1 #casa +Finanzas`).
//! - [`dates`]: fechas en español («hoy», «vie», «15 nov»).
//! - [`markdown`]: una línea en formato del plugin Obsidian Tasks.
//! - [`views`]: Inbox, Hoy, Próximos, Todas, Proyecto y Etiqueta.
//! - [`dto`]: la tarea en JSON, el contrato de la API y de `--json`.

pub mod dates;
pub mod dto;
pub mod markdown;
pub mod mcp;
pub mod model;
pub mod quick;
pub mod views;

/// Proyecto por defecto.
pub const INBOX: &str = "Inbox";
