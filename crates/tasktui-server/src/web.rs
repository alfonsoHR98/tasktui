//! La página web (PWA), incrustada en el binario desde `tasktui-worker/web`.

use axum::http::{header, HeaderValue};
use axum::response::{IntoResponse, Response};

macro_rules! file {
    ($path:literal, $mime:literal) => {
        ($path, $mime, include_bytes!(concat!("../../tasktui-worker/web/", $path)) as &[u8])
    };
}

const FILES: &[(&str, &str, &[u8])] = &[
    file!("index.html", "text/html; charset=utf-8"),
    file!("app.js", "text/javascript; charset=utf-8"),
    file!("style.css", "text/css; charset=utf-8"),
    file!("sw.js", "text/javascript; charset=utf-8"),
    file!("manifest.webmanifest", "application/manifest+json"),
    file!("icon.svg", "image/svg+xml"),
    file!("icon-180.png", "image/png"),
    file!("icon-192.png", "image/png"),
    file!("icon-512.png", "image/png"),
];

/// Mismas cabeceras que `web/_headers` (Workers Static Assets).
const CSP: &str = "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'";

pub fn serve(path: &str) -> Option<Response> {
    let name = if path == "/" { "index.html" } else { path.trim_start_matches('/') };
    let (_, mime, bytes) = FILES.iter().find(|(n, _, _)| *n == name)?;
    let mut r = (*bytes).into_response();
    let h = r.headers_mut();
    h.insert(header::CONTENT_TYPE, HeaderValue::from_static(mime));
    h.insert(header::CONTENT_SECURITY_POLICY, HeaderValue::from_static(CSP));
    h.insert(header::X_CONTENT_TYPE_OPTIONS, HeaderValue::from_static("nosniff"));
    h.insert(header::REFERRER_POLICY, HeaderValue::from_static("no-referrer"));
    Some(r)
}
