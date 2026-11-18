//! Nombres de proyecto: cómo se limpian, se comparan y se reconocen en la captura
//! rápida. Así «+niuko_consultores», «+Niuko Consultores», «+"Niuko Consultores"»
//! y «Ramirez Medellin» llegan al proyecto que ya existe en vez de crear otro.
//!
//! Es la misma lógica que `internal/core/project.go`: los casos de
//! `testdata/captura.json` se prueban en los dos lenguajes.

/// Comillas que pueden rodear un nombre: +"Niuko Consultores", +«Mi proyecto».
const QUOTES: &[char] = &['"', '\'', '«', '»', '“', '”', '‘', '’', '`'];

/// Cuántas palabras se prueban tras «+» para encontrar un proyecto con espacios.
const MAX_PROJECT_WORDS: usize = 6;

fn is_quote(c: char) -> bool {
    QUOTES.contains(&c)
}

fn fold(c: char) -> char {
    match c {
        'á' | 'à' | 'ä' | 'â' => 'a',
        'é' | 'è' | 'ë' | 'ê' => 'e',
        'í' | 'ì' | 'ï' | 'î' => 'i',
        'ó' | 'ò' | 'ö' | 'ô' => 'o',
        'ú' | 'ù' | 'ü' | 'û' => 'u',
        'ñ' => 'n',
        'ç' => 'c',
        _ => c,
    }
}

/// Limpia un nombre escrito por el usuario: quita comillas y espacios sobrantes y
/// cambia «_» por espacio.
pub fn clean_project_name(s: &str) -> String {
    let s = s.replace('_', " ");
    let s = s.trim().trim_matches(is_quote);
    s.split_whitespace().collect::<Vec<_>>().join(" ")
}

/// La forma de comparar nombres: sin mayúsculas, acentos, comillas, «_» ni
/// espacios repetidos.
pub fn project_key(s: &str) -> String {
    clean_project_name(s).to_lowercase().chars().map(fold).collect()
}

/// ¿Son el mismo proyecto?
pub fn same_project(a: &str, b: &str) -> bool {
    project_key(a) == project_key(b)
}

/// El proyecto existente equivalente a `name`.
pub fn match_project<'a>(projects: &'a [String], name: &str) -> Option<&'a String> {
    let k = project_key(name);
    if k.is_empty() {
        return None;
    }
    projects.iter().find(|p| project_key(p) == k)
}

/// Nombre canónico: el existente si lo hay (true) o el nombre limpio (false).
pub fn resolve_project(projects: &[String], name: &str) -> (String, bool) {
    match match_project(projects, name) {
        Some(p) => (p.clone(), true),
        None => (clean_project_name(name), false),
    }
}

/// Proyectos que se parecen a `name` (empiezan igual o lo contienen), para avisar
/// de un posible error al escribirlo.
pub fn suggest_projects(projects: &[String], name: &str) -> Vec<String> {
    let k = project_key(name);
    if k.is_empty() {
        return vec![];
    }
    let mut out: Vec<String> = projects
        .iter()
        .filter(|p| {
            let pk = project_key(p);
            pk != k && (pk.starts_with(&k) || k.starts_with(&pk) || pk.contains(&k))
        })
        .cloned()
        .collect();
    out.sort();
    out
}

/// Lee un +Proyecto que empieza en `toks[i]`. Devuelve el nombre y cuántos tokens
/// consume. Entiende +Finanzas, +Niuko_Consultores, +"Niuko Consultores",
/// +«Niuko Consultores» y +Niuko Consultores (si ese proyecto ya existe).
pub(crate) fn project_token(toks: &[&str], i: usize, projects: &[String]) -> Option<(String, usize)> {
    let p = toks[i].strip_prefix('+')?;
    let first = p.chars().next()?;
    if is_quote(first) {
        // Hasta el token que cierra las comillas; sin cierre, solo esta palabra.
        let mut end = i;
        for (j, tok) in toks.iter().enumerate().skip(i) {
            let w = if j == i { p.trim_start_matches(is_quote) } else { tok };
            if w.chars().last().is_some_and(is_quote) {
                end = j;
                break;
            }
        }
        let mut parts = vec![p];
        parts.extend_from_slice(&toks[i + 1..=end]);
        let mut name = clean_project_name(&parts.join(" "));
        if !name.chars().any(char::is_alphabetic) {
            return None;
        }
        if let Some(m) = match_project(projects, &name) {
            name = m.clone();
        }
        return Some((name, end - i + 1));
    }
    if !p.chars().any(char::is_alphabetic) {
        return None;
    }
    // Proyecto existente con espacios: el más largo que coincida.
    let max = (toks.len() - i).min(MAX_PROJECT_WORDS);
    for n in (2..=max).rev() {
        let mut parts = vec![p];
        parts.extend_from_slice(&toks[i + 1..i + n]);
        if let Some(m) = match_project(projects, &parts.join(" ")) {
            return Some((m.clone(), n));
        }
    }
    let name = clean_project_name(p);
    Some((match_project(projects, &name).cloned().unwrap_or(name), 1))
}
