//! Interfaz de terminal (ratatui).

use std::time::Duration;

use anyhow::Result;
use chrono::NaiveDate;
use ratatui::crossterm::event::{self, Event, KeyCode, KeyEvent, KeyEventKind, KeyModifiers};
use ratatui::layout::{Constraint, Layout, Position, Rect};
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, Clear, List, ListItem, ListState, Paragraph, Wrap};
use ratatui::{DefaultTerminal, Frame};
use unicode_width::UnicodeWidthStr;

use crate::dates;
use crate::model::Task;
use crate::today;
use crate::quick::{self, Quick};
use crate::store::{Snapshot, Store, INBOX};
use crate::views::View;

const SEL_BG: Color = Color::Indexed(237);
const ACCENT: Color = Color::Red;

fn prio_color(p: u8) -> Color {
    match p {
        1 => Color::Red,
        2 => Color::Yellow,
        3 => Color::Blue,
        _ => Color::Gray,
    }
}

#[derive(Clone, PartialEq)]
enum SideKind {
    Header,
    View(View),
}

struct SideItem {
    label: String,
    kind: SideKind,
    count: usize,
}

enum Row {
    Header(String),
    Task((usize, usize)),
}

#[derive(PartialEq)]
enum Focus {
    Side,
    List,
}

#[derive(Clone)]
enum InputKind {
    Add,
    Edit(String),
    Schedule(String),
    Note(String),
    Search,
}

enum Mode {
    Normal,
    Input(InputKind),
    Confirm(String),
    Help,
}

#[derive(Default)]
struct Input {
    buf: Vec<char>,
    cur: usize,
}

impl Input {
    fn set(&mut self, s: &str) {
        self.buf = s.chars().collect();
        self.cur = self.buf.len();
    }
    fn text(&self) -> String {
        self.buf.iter().collect()
    }
    fn handle(&mut self, k: KeyEvent) {
        let ctrl = k.modifiers.contains(KeyModifiers::CONTROL);
        match k.code {
            KeyCode::Char('a') if ctrl => self.cur = 0,
            KeyCode::Char('e') if ctrl => self.cur = self.buf.len(),
            KeyCode::Char('u') if ctrl => {
                self.buf.drain(..self.cur);
                self.cur = 0;
            }
            KeyCode::Char('w') if ctrl => {
                while self.cur > 0 && self.buf[self.cur - 1] == ' ' {
                    self.cur -= 1;
                    self.buf.remove(self.cur);
                }
                while self.cur > 0 && self.buf[self.cur - 1] != ' ' {
                    self.cur -= 1;
                    self.buf.remove(self.cur);
                }
            }
            KeyCode::Char(_) if ctrl => {}
            KeyCode::Char(c) => {
                self.buf.insert(self.cur, c);
                self.cur += 1;
            }
            KeyCode::Backspace if self.cur > 0 => {
                self.cur -= 1;
                self.buf.remove(self.cur);
            }
            KeyCode::Delete if self.cur < self.buf.len() => {
                self.buf.remove(self.cur);
            }
            KeyCode::Left => self.cur = self.cur.saturating_sub(1),
            KeyCode::Right => self.cur = (self.cur + 1).min(self.buf.len()),
            KeyCode::Home => self.cur = 0,
            KeyCode::End => self.cur = self.buf.len(),
            _ => {}
        }
    }
}

pub struct App {
    store: Store,
    today: NaiveDate,
    side: Vec<SideItem>,
    side_idx: usize,
    view: View,
    rows: Vec<Row>,
    task_rows: Vec<usize>,
    sel: usize,
    sel_id: Option<String>,
    list_state: ListState,
    focus: Focus,
    mode: Mode,
    input: Input,
    query: String,
    show_done: bool,
    msg: String,
    pending_g: bool,
    /// Tarea cuya descripción se editará en `$EDITOR` (fuera de la TUI).
    pending_editor: Option<String>,
    undo: Vec<Snapshot>,
    quit: bool,
}

pub fn run(store: Store) -> Result<()> {
    let mut terminal = ratatui::init();
    let mut app = App::new(store);
    let result = app.event_loop(&mut terminal);
    ratatui::restore();
    result
}

impl App {
    pub fn new(store: Store) -> Self {
        let mut app = App {
            store,
            today: today(),
            side: vec![],
            side_idx: 0,
            view: View::Today,
            rows: vec![],
            task_rows: vec![],
            sel: 0,
            sel_id: None,
            list_state: ListState::default(),
            focus: Focus::List,
            mode: Mode::Normal,
            input: Input::default(),
            query: String::new(),
            show_done: false,
            msg: String::new(),
            pending_g: false,
            pending_editor: None,
            undo: vec![],
            quit: false,
        };
        app.refresh();
        app.msg = app.store.notice().unwrap_or_default();
        app
    }

    fn event_loop(&mut self, term: &mut DefaultTerminal) -> Result<()> {
        while !self.quit {
            term.draw(|f| self.draw(f))?;
            if event::poll(Duration::from_secs(30))? {
                if let Event::Key(k) = event::read()? {
                    if k.kind == KeyEventKind::Press {
                        self.on_key(k);
                    }
                    if let Some(id) = self.pending_editor.take() {
                        // El editor necesita la terminal: se suspende la TUI.
                        ratatui::restore();
                        let edited = self.edit_in_editor(&id);
                        *term = ratatui::init();
                        match edited {
                            Ok(Some(desc)) => self.mutate(|s| {
                                s.update(&id, |t| t.description = desc)?;
                                Ok(("Descripción guardada".into(), None))
                            }),
                            Ok(None) => self.msg = "Descripción sin cambios".into(),
                            Err(e) => self.msg = format!("Error: {e}"),
                        }
                    }
                }
            } else {
                // Sin actividad: recarga por si hubo cambios desde el celular.
                let _ = self.store.reload();
                self.refresh();
            }
        }
        Ok(())
    }

    /// Abre la descripción en `$EDITOR`. Devuelve el texto nuevo, o `None`
    /// si no cambió.
    fn edit_in_editor(&self, id: &str) -> Result<Option<String>> {
        let current = self
            .store
            .find(id)
            .and_then(|p| self.store.task_at(p))
            .map(|t| t.description.clone())
            .unwrap_or_default();
        let new = crate::edit_text(id, &current)?;
        Ok((new != current.trim_end()).then_some(new))
    }

    // ---------- estado ----------

    fn selected_pos(&self) -> Option<(usize, usize)> {
        match self.rows.get(*self.task_rows.get(self.sel)?)? {
            Row::Task(p) => Some(*p),
            Row::Header(_) => None,
        }
    }

    fn selected(&self) -> Option<&Task> {
        self.store.task_at(self.selected_pos()?)
    }

    fn sync_sel(&mut self) {
        self.sel_id = self.selected().and_then(|t| t.id.clone());
        self.list_state.select(self.task_rows.get(self.sel).copied());
    }

    fn refresh(&mut self) {
        self.today = today();
        let today = self.today;
        let s = &self.store;
        let open = |f: &dyn Fn(&Task) -> bool| s.iter().filter(|(_, t)| !t.done && f(t)).count();

        let mut side = vec![
            SideItem { label: "▣ Inbox".into(), kind: SideKind::View(View::Inbox), count: open(&|t| t.project == INBOX) },
            SideItem { label: "◉ Hoy".into(), kind: SideKind::View(View::Today), count: open(&|t| t.due.is_some_and(|d| d <= today)) },
            SideItem { label: "◷ Próximos".into(), kind: SideKind::View(View::Upcoming), count: open(&|t| t.due.is_some_and(|d| d > today)) },
            SideItem { label: "≡ Todas".into(), kind: SideKind::View(View::All), count: open(&|_| true) },
            SideItem { label: "PROYECTOS".into(), kind: SideKind::Header, count: 0 },
        ];
        for p in s.projects().into_iter().filter(|p| p != INBOX) {
            let count = open(&|t| t.project == p);
            side.push(SideItem { label: format!("▸ {p}"), kind: SideKind::View(View::Project(p)), count });
        }
        let labels = s.labels();
        if !labels.is_empty() {
            side.push(SideItem { label: "ETIQUETAS".into(), kind: SideKind::Header, count: 0 });
            for (l, count) in labels {
                side.push(SideItem { label: format!("# {l}"), kind: SideKind::View(View::Label(l)), count });
            }
        }
        self.side = side;
        if let Some(i) = self.side.iter().position(|i| i.kind == SideKind::View(self.view.clone())) {
            self.side_idx = i;
        }

        self.rows.clear();
        self.task_rows.clear();
        for g in self.store.view(&self.view, today, self.show_done, &self.query) {
            if let Some(t) = g.title {
                self.rows.push(Row::Header(t));
            }
            for p in g.items {
                self.task_rows.push(self.rows.len());
                self.rows.push(Row::Task(p));
            }
        }
        // Mantener la selección en la misma tarea si sigue visible.
        if let Some(id) = self.sel_id.clone() {
            if let Some(i) = (0..self.task_rows.len()).find(|&i| {
                self.sel = i;
                self.selected().and_then(|t| t.id.as_deref()) == Some(id.as_str())
            }) {
                self.sel = i;
            } else {
                self.sel = self.sel_idx_clamped();
            }
        }
        self.sel = self.sel.min(self.task_rows.len().saturating_sub(1));
        self.sync_sel();
    }

    fn sel_idx_clamped(&self) -> usize {
        self.sel.min(self.task_rows.len().saturating_sub(1))
    }

    fn set_view(&mut self, v: View) {
        self.view = v;
        self.sel = 0;
        self.sel_id = None;
        self.list_state = ListState::default();
        self.refresh();
    }

    fn move_side(&mut self, delta: i32) {
        let n = self.side.len() as i32;
        let mut i = self.side_idx as i32;
        loop {
            i += delta;
            if i < 0 || i >= n {
                return;
            }
            if let SideKind::View(v) = &self.side[i as usize].kind {
                self.side_idx = i as usize;
                let v = v.clone();
                self.set_view(v);
                return;
            }
        }
    }

    fn move_sel(&mut self, delta: i32) {
        let n = self.task_rows.len() as i32;
        if n == 0 {
            return;
        }
        self.sel = (self.sel as i32 + delta).clamp(0, n - 1) as usize;
        self.sync_sel();
    }

    /// Ejecuta un cambio guardando una copia para poder deshacer.
    fn mutate(&mut self, f: impl FnOnce(&mut Store) -> Result<(String, Option<String>)>) {
        if let Err(e) = self.store.reload() {
            self.msg = format!("Error: {e}");
            return;
        }
        let snap = self.store.snapshot();
        match f(&mut self.store) {
            Ok((msg, select)) => {
                if let Ok(s) = snap {
                    self.undo.push(s);
                    if self.undo.len() > 50 {
                        self.undo.remove(0);
                    }
                }
                self.msg = msg;
                if select.is_some() {
                    self.sel_id = select;
                }
                crate::reminders::sync_in_background();
            }
            Err(e) => self.msg = format!("Error: {e}"),
        }
        self.refresh();
    }

    fn default_project(&self) -> String {
        match &self.view {
            View::Project(p) => p.clone(),
            _ => INBOX.into(),
        }
    }

    // ---------- teclado ----------

    fn on_key(&mut self, k: KeyEvent) {
        if k.modifiers.contains(KeyModifiers::CONTROL) && k.code == KeyCode::Char('c') {
            self.quit = true;
            return;
        }
        match &self.mode {
            Mode::Help => self.mode = Mode::Normal,
            Mode::Confirm(id) => {
                let id = id.clone();
                self.mode = Mode::Normal;
                if matches!(k.code, KeyCode::Char('y' | 's') | KeyCode::Enter) {
                    self.mutate(|s| {
                        s.delete(&id)?;
                        Ok(("Tarea eliminada (u para deshacer)".into(), None))
                    });
                } else {
                    self.msg = "Cancelado".into();
                }
            }
            Mode::Input(kind) => {
                let kind = kind.clone();
                match k.code {
                    KeyCode::Esc => {
                        if matches!(kind, InputKind::Search) {
                            self.query.clear();
                            self.refresh();
                        }
                        self.mode = Mode::Normal;
                    }
                    KeyCode::Enter => {
                        self.mode = Mode::Normal;
                        self.submit(kind);
                    }
                    _ => {
                        self.input.handle(k);
                        if matches!(kind, InputKind::Search) {
                            self.query = self.input.text();
                            self.sel = 0;
                            self.refresh();
                        }
                    }
                }
            }
            Mode::Normal => self.on_normal_key(k),
        }
    }

    fn open_input(&mut self, kind: InputKind, prefill: &str) {
        self.input.set(prefill);
        self.mode = Mode::Input(kind);
    }

    fn on_normal_key(&mut self, k: KeyEvent) {
        self.msg.clear();
        if std::mem::take(&mut self.pending_g) {
            match k.code {
                KeyCode::Char('g') => {
                    self.sel = 0;
                    self.sync_sel();
                }
                KeyCode::Char('i') => self.set_view(View::Inbox),
                KeyCode::Char('t') => self.set_view(View::Today),
                KeyCode::Char('u' | 'p') => self.set_view(View::Upcoming),
                KeyCode::Char('a') => self.set_view(View::All),
                _ => {}
            }
            return;
        }
        let side = self.focus == Focus::Side;
        match k.code {
            KeyCode::Char('q') => self.quit = true,
            KeyCode::Char('?') => self.mode = Mode::Help,
            KeyCode::Char('g') => self.pending_g = true,
            KeyCode::Char('G') => {
                self.sel = self.task_rows.len().saturating_sub(1);
                self.sync_sel();
            }
            KeyCode::Tab | KeyCode::BackTab => {
                self.focus = if side { Focus::List } else { Focus::Side };
            }
            KeyCode::Char('h') | KeyCode::Left => self.focus = Focus::Side,
            KeyCode::Char('l') | KeyCode::Right | KeyCode::Enter => self.focus = Focus::List,
            KeyCode::Char('j') | KeyCode::Down if side => self.move_side(1),
            KeyCode::Char('k') | KeyCode::Up if side => self.move_side(-1),
            KeyCode::Char('j') | KeyCode::Down => self.move_sel(1),
            KeyCode::Char('k') | KeyCode::Up => self.move_sel(-1),
            KeyCode::Char('a') => self.open_input(InputKind::Add, ""),
            KeyCode::Char('/') => {
                let q = self.query.clone();
                self.open_input(InputKind::Search, &q);
            }
            KeyCode::Esc => {
                self.query.clear();
                self.refresh();
            }
            KeyCode::Char('H') => {
                self.show_done = !self.show_done;
                self.msg = if self.show_done { "Mostrando completadas" } else { "Ocultando completadas" }.into();
                self.refresh();
            }
            KeyCode::Char('u') => match self.undo.pop() {
                Some(snap) => {
                    self.msg = match self.store.restore(&snap) {
                        Ok(()) => {
                            crate::reminders::sync_in_background();
                            "Deshecho".into()
                        }
                        Err(e) => format!("Error: {e}"),
                    };
                    self.refresh();
                }
                None => self.msg = "Nada que deshacer".into(),
            },
            KeyCode::Char('r') => {
                self.msg = match self.store.reload() {
                    Ok(()) => "Recargado".into(),
                    Err(e) => format!("Error: {e}"),
                };
                self.refresh();
            }
            _ => self.on_task_key(k),
        }
    }

    /// Teclas que actúan sobre la tarea seleccionada.
    fn on_task_key(&mut self, k: KeyEvent) {
        let Some(t) = self.selected().cloned() else { return };
        let Some(id) = t.id.clone() else {
            self.msg = "Tarea completada sin id: edítala desde Obsidian".into();
            return;
        };
        match k.code {
            KeyCode::Char('x') | KeyCode::Char(' ') => self.mutate(|s| {
                let done = s.toggle(&id)?;
                let msg = match (done, &t.recurrence) {
                    (true, Some(r)) => format!("✓ Hecha · se repite {}", r.to_es()),
                    (true, None) => "✓ Completada".into(),
                    (false, _) => "Reabierta".into(),
                };
                Ok((msg, None))
            }),
            KeyCode::Char('e') => {
                let s = quick::to_quick(&t, self.today);
                self.open_input(InputKind::Edit(id), &s);
            }
            KeyCode::Char('s') => {
                let mut s = t.due.map(|d| d.to_string()).unwrap_or_default();
                if let Some(h) = t.time {
                    s.push_str(&format!(" {}", h.format("%H:%M")));
                }
                if let Some(d) = t.deadline {
                    s.push_str(&format!(" limit {d}"));
                }
                self.open_input(InputKind::Schedule(id), s.trim());
            }
            KeyCode::Char('t') => {
                let today = self.today;
                self.mutate(|s| {
                    s.update(&id, |t| t.due = Some(today))?;
                    Ok(("Para hoy".into(), None))
                });
            }
            KeyCode::Char(c @ '1'..='4') => {
                let p = c.to_digit(10).unwrap() as u8;
                self.mutate(|s| {
                    s.update(&id, |t| t.priority = p)?;
                    Ok((format!("Prioridad p{p}"), None))
                });
            }
            KeyCode::Char('d') => self.mode = Mode::Confirm(id),
            KeyCode::Char('i') => self.pending_editor = Some(id),
            KeyCode::Char('n') => self.open_input(InputKind::Note(id), ""),
            _ => {}
        }
    }

    fn submit(&mut self, kind: InputKind) {
        let text = self.input.text();
        let today = self.today;
        let q = quick::parse_quick(&text, today);
        match kind {
            InputKind::Search => {}
            InputKind::Add => {
                if q.text.is_empty() {
                    self.msg = "La tarea necesita texto".into();
                    return;
                }
                let mut t = Task::new(&q.text);
                t.labels = q.labels;
                t.priority = q.priority.unwrap_or(4);
                t.recurrence = q.recurrence;
                t.due = q.due.or((self.view == View::Today).then_some(today));
                if let View::Label(l) = &self.view {
                    if !t.labels.iter().any(|x| x.eq_ignore_ascii_case(l)) {
                        t.labels.push(l.clone());
                    }
                }
                let project = q.project.unwrap_or_else(|| self.default_project());
                self.mutate(|s| {
                    let id = s.add(t, &project)?;
                    Ok((format!("Añadida en {project}"), Some(id)))
                });
            }
            InputKind::Edit(id) => {
                if q.text.is_empty() {
                    self.msg = "La tarea necesita texto".into();
                    return;
                }
                self.mutate(|s| {
                    s.update(&id, |t| q.apply_full(t))?;
                    Ok(("Guardada".into(), None))
                });
            }
            InputKind::Note(id) => {
                if text.trim().is_empty() {
                    return;
                }
                self.mutate(|s| {
                    s.add_note(&id, &text)?;
                    Ok(("Nota añadida".into(), None))
                });
            }
            InputKind::Schedule(id) => {
                if !text.trim().is_empty() && q.due.is_none() && q.deadline.is_none() {
                    self.msg = format!("No entendí la fecha «{text}»");
                    return;
                }
                self.mutate(|s| {
                    s.update(&id, |t| {
                        t.due = q.due;
                        t.time = q.time;
                        t.deadline = q.deadline;
                        if q.recurrence.is_some() {
                            t.recurrence = q.recurrence;
                        }
                    })?;
                    let msg = match q.due {
                        Some(d) => format!("Fecha: {}", dates::long(d, today)),
                        None => "Sin fecha".into(),
                    };
                    Ok((msg, None))
                });
            }
        }
    }

    // ---------- dibujo ----------

    pub fn draw(&mut self, f: &mut Frame) {
        let [main, status] = Layout::vertical([Constraint::Min(1), Constraint::Length(1)]).areas(f.area());
        let side_w = (main.width / 3).min(26);
        let [side, right] = Layout::horizontal([Constraint::Length(side_w), Constraint::Min(10)]).areas(main);
        let extra = self
            .selected()
            .map(|t| t.notes.len() + t.description.lines().filter(|l| !l.trim().is_empty()).count())
            .unwrap_or(0) as u16;
        let detail_h = 6 + extra.min(8);
        let [list, detail] = Layout::vertical([Constraint::Min(3), Constraint::Length(detail_h)]).areas(right);
        self.draw_side(f, side);
        self.draw_list(f, list);
        self.draw_detail(f, detail);
        self.draw_status(f, status);
        match &self.mode {
            Mode::Input(kind) => {
                let kind = kind.clone();
                self.draw_input(f, kind);
            }
            Mode::Help => draw_help(f),
            _ => {}
        }
    }

    fn block(&self, title: String, focused: bool) -> Block<'static> {
        let color = if focused { ACCENT } else { Color::DarkGray };
        Block::bordered()
            .title(Span::styled(title, Style::new().fg(color).add_modifier(Modifier::BOLD)))
            .border_style(Style::new().fg(color))
    }

    fn draw_side(&self, f: &mut Frame, area: Rect) {
        let inner_w = area.width.saturating_sub(2) as usize;
        let items: Vec<ListItem> = self
            .side
            .iter()
            .map(|it| match it.kind {
                SideKind::Header => ListItem::new(Line::styled(
                    format!(" {}", it.label),
                    Style::new().fg(Color::DarkGray).add_modifier(Modifier::BOLD),
                ))
                .style(Style::new()),
                SideKind::View(_) => {
                    let count = if it.count > 0 { it.count.to_string() } else { String::new() };
                    let label = format!(" {}", it.label);
                    let pad = inner_w.saturating_sub(label.width() + count.width() + 2);
                    ListItem::new(Line::from(vec![
                        Span::raw(label),
                        Span::raw(" ".repeat(pad)),
                        Span::styled(count, Style::new().fg(Color::DarkGray)),
                    ]))
                }
            })
            .collect();
        let focused = self.focus == Focus::Side;
        let hl = if focused {
            Style::new().bg(SEL_BG).fg(ACCENT).add_modifier(Modifier::BOLD)
        } else {
            Style::new().fg(ACCENT)
        };
        let list = List::new(items).block(self.block(
            // ☁ = conectado al servidor; ⚠ = sin conexión (solo lectura).
            match (self.store.dir(), self.store.notice()) {
                (Some(_), _) => " tasktui ".into(),
                (None, None) => " tasktui ☁ ".into(),
                (None, Some(_)) => " tasktui ⚠ ".into(),
            },
            focused,
        )).highlight_style(hl);
        let mut st = ListState::default().with_selected(Some(self.side_idx));
        f.render_stateful_widget(list, area, &mut st);
    }

    fn task_line(&self, t: &Task) -> Line<'static> {
        let today = self.today;
        let mut spans = vec![];
        let depth = t.indent.replace('\t', "  ").len() / 2;
        spans.push(Span::raw(" ".repeat(depth * 2 + 1)));
        let check = if t.done { "✓ " } else { "○ " };
        spans.push(Span::styled(check, Style::new().fg(prio_color(t.priority)).add_modifier(Modifier::BOLD)));
        let text_style = if t.done {
            Style::new().fg(Color::DarkGray).add_modifier(Modifier::CROSSED_OUT)
        } else {
            Style::new()
        };
        spans.push(Span::styled(t.text.clone(), text_style));
        if let Some(d) = t.due {
            let color = if t.is_overdue(today) {
                Color::Red
            } else if d == today {
                Color::Green
            } else {
                Color::Cyan
            };
            let mut when = dates::rel(d, today);
            if let Some(h) = t.time {
                when.push_str(&format!(" {}", h.format("%H:%M")));
            }
            spans.push(Span::styled(format!("  {when}"), Style::new().fg(color)));
        }
        if let Some(d) = t.deadline.filter(|_| !t.done) {
            spans.push(Span::styled(format!("  🎯 {}", dates::rel(d, today)), deadline_style(d, today)));
        }
        if !t.description.trim().is_empty() || !t.notes.is_empty() {
            spans.push(Span::styled(" ≡", Style::new().fg(Color::DarkGray)));
        }
        if t.recurrence.is_some() {
            spans.push(Span::styled(" ↻", Style::new().fg(Color::Cyan)));
        }
        for l in &t.labels {
            spans.push(Span::styled(format!("  #{l}"), Style::new().fg(Color::Magenta)));
        }
        let in_project = matches!(self.view, View::Project(_) | View::Inbox | View::All);
        if !in_project {
            spans.push(Span::styled(format!("  · {}", t.project), Style::new().fg(Color::DarkGray)));
        }
        Line::from(spans)
    }

    fn draw_list(&mut self, f: &mut Frame, area: Rect) {
        let mut title = format!(" {} · {} ", self.view.title(), self.task_rows.len());
        if !self.query.is_empty() {
            title.push_str(&format!("· /{} ", self.query));
        }
        let focused = self.focus == Focus::List;
        let block = self.block(title, focused);
        if self.task_rows.is_empty() {
            let msg = if self.query.is_empty() {
                "  Nada pendiente por aquí ✨   (a = añadir tarea)"
            } else {
                "  Sin resultados   (Esc = limpiar búsqueda)"
            };
            f.render_widget(
                Paragraph::new(vec![Line::raw(""), Line::styled(msg, Style::new().fg(Color::DarkGray))]).block(block),
                area,
            );
            return;
        }
        let items: Vec<ListItem> = self
            .rows
            .iter()
            .map(|r| match r {
                Row::Header(h) => ListItem::new(Line::styled(
                    format!(" {h}"),
                    Style::new().fg(ACCENT).add_modifier(Modifier::BOLD),
                )),
                Row::Task(p) => match self.store.task_at(*p) {
                    Some(t) => ListItem::new(self.task_line(t)),
                    None => ListItem::new(""),
                },
            })
            .collect();
        let hl = if focused { Style::new().bg(SEL_BG) } else { Style::new() };
        let list = List::new(items).block(block).highlight_style(hl);
        f.render_stateful_widget(list, area, &mut self.list_state);
    }

    fn draw_detail(&self, f: &mut Frame, area: Rect) {
        let block = self.block(" Detalle ".into(), false);
        let Some(t) = self.selected() else {
            f.render_widget(block, area);
            return;
        };
        let dim = Style::new().fg(Color::DarkGray);
        let mut where_ = t.project.clone();
        if let Some(s) = &t.section {
            where_.push_str(&format!(" › {s}"));
        }
        let mut meta = vec![];
        if let Some(d) = t.due {
            let mut when = dates::long(d, self.today);
            if let Some(h) = t.time {
                when.push_str(&format!(" · {}", h.format("%H:%M")));
            }
            meta.push(format!("📅 {when}"));
        }
        if let Some(d) = t.deadline {
            meta.push(format!("🎯 límite {}", dates::long(d, self.today)));
        }
        if t.priority < 4 {
            meta.push(format!("p{}", t.priority));
        }
        if let Some(r) = &t.recurrence {
            meta.push(format!("↻ {}", r.to_es()));
        }
        if !t.labels.is_empty() {
            meta.push(t.labels.iter().map(|l| format!("#{l}")).collect::<Vec<_>>().join(" "));
        }
        let mut lines = vec![
            Line::styled(t.text.clone(), Style::new().add_modifier(Modifier::BOLD)),
            Line::styled(where_, dim),
            Line::raw(meta.join("   ")),
            Line::styled(
                format!("id {}   {}", t.id.as_deref().unwrap_or("—"), t.extra.join(" ")),
                dim,
            ),
        ];
        for l in t.description.lines().filter(|l| !l.trim().is_empty()) {
            lines.push(Line::raw(l.to_string()));
        }
        for n in &t.notes {
            let when = n.date.map(|d| format!("{} · ", dates::rel(d, self.today))).unwrap_or_default();
            lines.push(Line::from(vec![
                Span::styled(format!("› {when}"), dim),
                Span::raw(n.text.clone()),
            ]));
        }
        f.render_widget(Paragraph::new(lines).block(block).wrap(Wrap { trim: true }), area);
    }

    fn draw_status(&self, f: &mut Frame, area: Rect) {
        let (text, style) = match &self.mode {
            Mode::Confirm(_) => {
                let name = self.selected().map(|t| t.text.clone()).unwrap_or_default();
                (format!(" ¿Eliminar «{name}»? (y/n)"), Style::new().fg(Color::Black).bg(Color::Red))
            }
            _ if !self.msg.is_empty() => (format!(" {}", self.msg), Style::new().fg(Color::Yellow)),
            _ => (
                " a añadir · x completar · e editar · s fecha · i descripción · n nota · d borrar · / buscar · ? ayuda".into(),
                Style::new().fg(Color::DarkGray),
            ),
        };
        f.render_widget(Paragraph::new(text).style(style), area);
    }

    fn draw_input(&self, f: &mut Frame, kind: InputKind) {
        let area = f.area();
        let w = area.width.saturating_sub(4).min(80);
        let h = 6.min(area.height);
        let popup = Rect {
            x: area.x + (area.width - w) / 2,
            y: (area.height / 3).min(area.height - h),
            width: w,
            height: h,
        };
        f.render_widget(Clear, popup);
        let title = match kind {
            InputKind::Add => format!(" Nueva tarea → {} ", self.default_project()),
            InputKind::Edit(_) => " Editar tarea ".into(),
            InputKind::Schedule(_) => " Fecha (mañana 18:00, viernes, 15/11 limit 20/11; vacío = sin fecha) ".into(),
            InputKind::Note(_) => " Nueva nota (se guarda con la fecha de hoy) ".into(),
            InputKind::Search => " Buscar ".into(),
        };
        let block = Block::bordered()
            .title(Span::styled(title, Style::new().fg(ACCENT).add_modifier(Modifier::BOLD)))
            .border_style(Style::new().fg(ACCENT));
        let inner = block.inner(popup);
        f.render_widget(block, popup);

        // Línea de texto con desplazamiento horizontal para ver el cursor.
        let avail = inner.width.saturating_sub(1) as usize;
        let mut start = 0;
        let width_of = |a: usize, b: usize| self.input.buf[a..b].iter().collect::<String>().width();
        while start < self.input.cur && width_of(start, self.input.cur) > avail {
            start += 1;
        }
        let visible: String = self.input.buf[start..].iter().collect();
        f.render_widget(Paragraph::new(visible), Rect { height: 1, ..inner });
        f.set_cursor_position(Position::new(inner.x + width_of(start, self.input.cur) as u16, inner.y));

        let dim = Style::new().fg(Color::DarkGray);
        let (preview, hint) = match kind {
            InputKind::Add | InputKind::Edit(_) => (
                self.preview(&quick::parse_quick(&self.input.text(), self.today), matches!(kind, InputKind::Add)),
                Line::styled("#etiqueta  +Proyecto  p1-p4  mañana 18:00  limit 20/11  cada lunes  \\literal", dim),
            ),
            InputKind::Schedule(_) => {
                let q = quick::parse_quick(&self.input.text(), self.today);
                let mut desc = vec![];
                if let Some(d) = q.due {
                    let mut s = format!("📅 {}", dates::long(d, self.today));
                    if let Some(h) = q.time {
                        s.push_str(&format!(" · {}", h.format("%H:%M")));
                    }
                    desc.push(s);
                }
                if let Some(d) = q.deadline {
                    desc.push(format!("🎯 límite {}", dates::long(d, self.today)));
                }
                let p = match (desc.is_empty(), self.input.buf.is_empty()) {
                    (_, true) => Line::styled("→ sin fecha", dim),
                    (false, _) => Line::styled(format!("→ {}", desc.join("   ")), Style::new().fg(Color::Green)),
                    (true, _) => Line::styled("→ ?", Style::new().fg(Color::Red)),
                };
                (p, Line::styled("Enter guardar · Esc cancelar", dim))
            }
            InputKind::Note(_) => (Line::raw(""), Line::styled("Enter guardar · Esc cancelar", dim)),
            InputKind::Search => (Line::raw(""), Line::styled("Enter fijar · Esc limpiar", dim)),
        };
        if inner.height >= 4 {
            f.render_widget(Paragraph::new(preview), Rect { y: inner.y + 2, height: 1, ..inner });
            f.render_widget(Paragraph::new(hint), Rect { y: inner.y + 3, height: 1, ..inner });
        }
    }

    /// Vista previa de cómo se interpretará la captura rápida.
    fn preview(&self, q: &Quick, adding: bool) -> Line<'static> {
        let mut spans = vec![Span::styled("→ ", Style::new().fg(Color::DarkGray))];
        let mut push = |s: String, c: Color| {
            spans.push(Span::styled(s, Style::new().fg(c)));
            spans.push(Span::raw("  "));
        };
        let due = q.due.or((adding && self.view == View::Today).then_some(self.today));
        if let Some(d) = due {
            let mut s = format!("📅 {}", dates::long(d, self.today));
            if let Some(h) = q.time {
                s.push_str(&format!(" · {}", h.format("%H:%M")));
            }
            push(s, Color::Green);
        }
        if let Some(d) = q.deadline {
            push(format!("🎯 límite {}", dates::long(d, self.today)), Color::Red);
        }
        if let Some(p) = q.priority.filter(|p| *p < 4) {
            push(format!("p{p}"), prio_color(p));
        }
        if let Some(r) = &q.recurrence {
            push(format!("↻ {}", r.to_es()), Color::Cyan);
        }
        for l in &q.labels {
            push(format!("#{l}"), Color::Magenta);
        }
        if let Some(p) = &q.project {
            push(format!("▸ {p}"), Color::Blue);
        }
        Line::from(spans)
    }
}

/// Fecha límite: rojo si ya pasó o es hoy, amarillo si faltan 2 días o menos.
fn deadline_style(d: NaiveDate, today: NaiveDate) -> Style {
    match (d - today).num_days() {
        ..=0 => Style::new().fg(Color::Red).add_modifier(Modifier::BOLD),
        1..=2 => Style::new().fg(Color::Yellow),
        _ => Style::new().fg(Color::DarkGray),
    }
}

fn draw_help(f: &mut Frame) {
    let keys = [
        ("Navegación", ""),
        ("j / k  ↓ / ↑", "moverse"),
        ("h / l  Tab", "barra lateral ↔ lista"),
        ("gi gt gu ga", "ir a Inbox · Hoy · Próximos · Todas"),
        ("gg / G", "inicio / final"),
        ("/  Esc", "buscar · limpiar búsqueda"),
        ("H", "mostrar/ocultar completadas"),
        ("", ""),
        ("Tareas", ""),
        ("a", "añadir (captura rápida)"),
        ("e", "editar"),
        ("x  espacio", "completar / reabrir"),
        ("s", "fecha, hora y límite (mañana 18:00 limit 20/11)"),
        ("i", "editar la descripción en $EDITOR"),
        ("n", "añadir una nota con fecha"),
        ("t", "poner para hoy"),
        ("1 2 3 4", "prioridad"),
        ("d", "eliminar"),
        ("u", "deshacer"),
        ("r", "recargar desde disco"),
        ("q", "salir"),
    ];
    let lines: Vec<Line> = keys
        .iter()
        .map(|(k, d)| {
            if d.is_empty() {
                Line::styled(format!(" {k}"), Style::new().fg(ACCENT).add_modifier(Modifier::BOLD))
            } else {
                Line::from(vec![
                    Span::styled(format!("   {k:<14}"), Style::new().fg(Color::Yellow)),
                    Span::raw(d.to_string()),
                ])
            }
        })
        .collect();
    let area = f.area();
    let (w, h) = (56.min(area.width), (lines.len() as u16 + 2).min(area.height));
    let popup = Rect { x: (area.width - w) / 2, y: (area.height - h) / 2, width: w, height: h };
    f.render_widget(Clear, popup);
    f.render_widget(
        Paragraph::new(lines).block(
            Block::bordered()
                .title(" Atajos · cualquier tecla para cerrar ")
                .border_style(Style::new().fg(ACCENT)),
        ),
        popup,
    );
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::store::tests::temp_store;
    use ratatui::backend::TestBackend;
    use ratatui::Terminal;

    fn key(c: char) -> KeyEvent {
        KeyEvent::new(KeyCode::Char(c), KeyModifiers::NONE)
    }

    #[test]
    fn dibuja_y_añade() {
        let store = temp_store("tui");
        let dir = store.dir().unwrap().to_path_buf();
        let mut app = App::new(store);
        app.on_key(key('a'));
        for c in "Comprar café mañana p1 #super +Casa".chars() {
            app.on_key(key(c));
        }
        app.on_key(KeyEvent::new(KeyCode::Enter, KeyModifiers::NONE));
        assert!(app.msg.starts_with("Añadida en Casa"), "{}", app.msg);
        app.set_view(View::Upcoming);
        assert_eq!(app.task_rows.len(), 1);

        let mut term = Terminal::new(TestBackend::new(100, 20)).unwrap();
        term.draw(|f| app.draw(f)).unwrap();
        let out = format!("{}", term.backend());
        assert!(out.contains("Comprar café"));
        assert!(out.contains("#super"));
        println!("{out}");
        let _ = std::fs::remove_dir_all(dir);
    }
}
