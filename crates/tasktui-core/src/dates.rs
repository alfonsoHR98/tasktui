//! Fechas en español: nombres de días y formato relativo.

use chrono::{Datelike, Duration, NaiveDate, Weekday};

const WD_ABBR: [&str; 7] = ["lun", "mar", "mié", "jue", "vie", "sáb", "dom"];
const MON_ABBR: [&str; 12] = [
    "ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic",
];

pub fn weekday_es(s: &str) -> Option<Weekday> {
    Some(match s {
        "lunes" => Weekday::Mon,
        "martes" => Weekday::Tue,
        "miércoles" | "miercoles" => Weekday::Wed,
        "jueves" => Weekday::Thu,
        "viernes" => Weekday::Fri,
        "sábado" | "sabado" => Weekday::Sat,
        "domingo" => Weekday::Sun,
        _ => return None,
    })
}

/// Próximo día de la semana `w` (si hoy es `w`, la semana que viene).
pub fn next_weekday(today: NaiveDate, w: Weekday) -> NaiveDate {
    let mut d = today + Duration::days(1);
    while d.weekday() != w {
        d += Duration::days(1);
    }
    d
}

fn wd(d: NaiveDate) -> &'static str {
    WD_ABBR[d.weekday().num_days_from_monday() as usize]
}

fn day_month(d: NaiveDate, today: NaiveDate) -> String {
    let mut s = format!("{} {}", d.day(), MON_ABBR[d.month0() as usize]);
    if d.year() != today.year() {
        s.push_str(&format!(" {}", d.year()));
    }
    s
}

/// Formato corto relativo: "hoy", "mañana", "vie", "15 nov".
pub fn rel(d: NaiveDate, today: NaiveDate) -> String {
    match (d - today).num_days() {
        0 => "hoy".into(),
        1 => "mañana".into(),
        -1 => "ayer".into(),
        2..=6 => wd(d).into(),
        _ => day_month(d, today),
    }
}

/// Formato largo para encabezados: "jue 6 nov · mañana".
pub fn long(d: NaiveDate, today: NaiveDate) -> String {
    let base = format!("{} {}", wd(d), day_month(d, today));
    match (d - today).num_days() {
        0 => format!("{base} · hoy"),
        1 => format!("{base} · mañana"),
        _ => base,
    }
}
