// tasktui web: cliente de la API del Worker, sin dependencias ni compilación.
// Todo el texto de las tareas se pinta con textContent (nunca innerHTML).
'use strict';

const TOKEN_KEY = 'tasktui.token';
const DONE_KEY = 'tasktui.showDone';

const state = {
  token: load(TOKEN_KEY),
  view: 'today',
  query: '',
  showDone: load(DONE_KEY) === '1',
  projects: [],
  labels: [],
  sugg: [],
};

// ---------- utilidades ----------

function load(k) {
  try { return localStorage.getItem(k); } catch { return null; }
}
function save(k, v) {
  try { v == null ? localStorage.removeItem(k) : localStorage.setItem(k, v); } catch { /* sin almacenamiento */ }
}

const $ = (id) => document.getElementById(id);

/** Crea un elemento. `attrs.class`, `attrs.on*` y el resto como atributos. */
function el(tag, attrs = {}, ...children) {
  const n = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v == null || v === false) continue;
    if (k === 'class') n.className = v;
    else if (k.startsWith('on')) n.addEventListener(k.slice(2), v);
    else n.setAttribute(k, v === true ? '' : v);
  }
  for (const c of children.flat()) {
    if (c == null || c === false) continue;
    n.append(c instanceof Node ? c : document.createTextNode(String(c)));
  }
  return n;
}

/** Fecha local de hoy como AAAA-MM-DD (el servidor la usa para «hoy»). */
function todayStr() {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

function parseDate(s) {
  const [y, m, d] = s.split('-').map(Number);
  return new Date(y, m - 1, d);
}

const WEEKDAYS = ['domingo', 'lunes', 'martes', 'miércoles', 'jueves', 'viernes', 'sábado'];
const MONTHS = ['ene', 'feb', 'mar', 'abr', 'may', 'jun', 'jul', 'ago', 'sep', 'oct', 'nov', 'dic'];

/** «Hoy», «Mañana», «viernes», «12 nov». */
function relative(s) {
  const d = parseDate(s);
  const t = parseDate(todayStr());
  const diff = Math.round((d - t) / 86400000);
  if (diff === 0) return 'Hoy';
  if (diff === 1) return 'Mañana';
  if (diff === -1) return 'Ayer';
  if (diff > 1 && diff < 7) return WEEKDAYS[d.getDay()];
  const base = `${d.getDate()} ${MONTHS[d.getMonth()]}`;
  return d.getFullYear() === t.getFullYear() ? base : `${base} ${d.getFullYear()}`;
}

/** «hoy», «ayer» o «el 12 nov». */
function doneWhen(s) {
  const r = relative(s);
  return ['Hoy', 'Ayer'].includes(r) ? r.toLowerCase() : `el ${r}`;
}

/** -1 vencida, 0 hoy, 1 futura. */
function when(s) {
  const a = s, b = todayStr();
  return a < b ? -1 : a === b ? 0 : 1;
}

/** Quita acentos y mayúsculas para comparar. */
function norm(s) {
  return s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();
}

let toastTimer;
function toast(msg, action) {
  const t = $('toast');
  t.replaceChildren(el('span', {}, msg));
  if (action) {
    t.append(el('button', {
      onclick: () => { t.hidden = true; action.run(); },
    }, action.label));
  }
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { t.hidden = true; }, action ? 6000 : 3000);
}

// ---------- API ----------

class ApiError extends Error {
  constructor(msg, status) { super(msg); this.status = status; }
}

async function api(path, { method = 'GET', body } = {}) {
  const sep = path.includes('?') ? '&' : '?';
  let res;
  try {
    res = await fetch(`/api/${path}${sep}today=${todayStr()}`, {
      method,
      headers: {
        Authorization: `Bearer ${state.token}`,
        ...(body ? { 'Content-Type': 'application/json' } : {}),
      },
      body: body ? JSON.stringify(body) : undefined,
    });
  } catch {
    throw new ApiError('Sin conexión', 0);
  }
  const data = await res.json().catch(() => null);
  if (res.status === 401) {
    logout('El servidor rechazó el token. Vuelve a escribirlo.');
    throw new ApiError('Token inválido', 401);
  }
  if (!res.ok) throw new ApiError(data?.error || `Error ${res.status}`, res.status);
  return data;
}

/** Ejecuta un cambio, avisa si falla y recarga la vista. */
async function mutate(fn, okMsg, undo) {
  try {
    const r = await fn();
    if (okMsg) toast(okMsg, undo && { label: 'Deshacer', run: () => mutate(() => undo(r), 'Deshecho') });
    await refresh();
    return r;
  } catch (e) {
    if (e.status !== 401) toast(e.status === 0 ? 'Sin conexión: el cambio no se guardó' : e.message);
    throw e;
  }
}

// ---------- vistas ----------

const FIXED = { today: 'Hoy', upcoming: 'Próximos', inbox: 'Inbox', projects: 'Proyectos', search: 'Buscar' };

function viewTitle(v) {
  if (FIXED[v]) return FIXED[v];
  if (v.startsWith('+')) return v.slice(1);
  return v;
}

/** Proyecto para las tareas nuevas: el de la vista abierta o Inbox. */
function defaultProject() {
  return state.view.startsWith('+') ? state.view.slice(1) : null;
}

function go(view) {
  location.hash = '#/' + encodeURIComponent(view);
}

function readHash() {
  const v = decodeURIComponent(location.hash.replace(/^#\/?/, '')) || 'today';
  state.view = v;
  if (v !== 'search') {
    $('search-row').hidden = true;
  }
  render();
}

async function render() {
  $('title').textContent = viewTitle(state.view);
  document.title = `${viewTitle(state.view)} · tasktui`;
  for (const b of document.querySelectorAll('.tab')) {
    const active = b.dataset.view === state.view
      || (b.dataset.view === 'projects' && /^[+#]/.test(state.view));
    if (active) b.setAttribute('aria-current', 'page');
    else b.removeAttribute('aria-current');
  }
  const p = defaultProject();
  $('add-hint').textContent = `→ ${p || 'Inbox'} · ej. mañana 18:00 p1 #casa +Proyecto limit viernes`;
  await refresh();
}

let loadSeq = 0;

async function refresh() {
  const seq = ++loadSeq;
  const v = state.view;
  const list = $('list');
  try {
    loadMeta();
    if (v === 'projects') {
      await loadMeta(true);
      if (seq !== loadSeq) return;
      showNotice(null);
      list.replaceChildren(...projectsView());
      return;
    }
    if (v === 'search' && !state.query.trim()) {
      list.replaceChildren(el('p', { class: 'empty' }, 'Escribe para buscar en todas tus tareas.'));
      return;
    }
    const params = new URLSearchParams({ view: v === 'search' ? 'all' : v });
    if (v === 'search') params.set('q', state.query.trim());
    if (state.showDone || v === 'search') params.set('all', '1');
    const data = await api(`tasks?${params}`);
    if (seq !== loadSeq) return;
    save(`tasktui.cache.${v}`, JSON.stringify(data));
    showNotice(null);
    list.replaceChildren(...tasksView(data));
  } catch (e) {
    if (seq !== loadSeq || e.status === 401) return;
    const cached = load(`tasktui.cache.${v}`);
    if (e.status === 0 && cached) {
      showNotice('Sin conexión: mostrando la última copia. Los cambios no se guardarán.');
      list.replaceChildren(...tasksView(JSON.parse(cached)));
    } else {
      showNotice(e.message);
    }
  }
}

function showNotice(msg) {
  const n = $('notice');
  n.hidden = !msg;
  n.textContent = msg || '';
}

let metaAt = 0;
/** Proyectos y etiquetas para el autocompletado (como mucho cada 30 s). */
async function loadMeta(force = false) {
  if (!force && Date.now() - metaAt < 30000) return;
  metaAt = Date.now();
  try {
    const [p, l] = await Promise.all([api('projects'), api('labels')]);
    state.projects = p;
    state.labels = l;
  } catch { /* se reintenta en la siguiente carga */ }
}

function tasksView(data) {
  const groups = data.groups.filter((g) => g.tasks.length);
  if (!groups.length) {
    return [el('p', { class: 'empty' }, state.view === 'today' ? 'Nada pendiente para hoy 🎉' : 'Sin tareas aquí.')];
  }
  const out = [];
  for (const g of groups) {
    if (g.title) {
      out.push(el('h2', { class: `group-title${/vencid/i.test(g.title) ? ' overdue' : ''}` }, g.title));
    }
    for (const t of g.tasks) out.push(taskRow(t));
  }
  return out;
}

function taskRow(t) {
  const meta = [];
  if (t.due) {
    const w = when(t.due);
    meta.push(el('span', { class: w < 0 && !t.done ? 'overdue' : w === 0 ? 'today' : null },
      `📅 ${relative(t.due)}${t.time ? ' ' + t.time : ''}`));
  }
  if (t.deadline) {
    meta.push(el('span', { class: when(t.deadline) < 0 && !t.done ? 'overdue' : null }, `🎯 ${relative(t.deadline)}`));
  }
  if (t.recurrence) meta.push(el('span', {}, '🔁'));
  if (!state.view.startsWith('+')) meta.push(el('span', {}, t.project));
  for (const l of t.labels) meta.push(el('span', { class: 'label' }, `#${l}`));
  if (t.description) meta.push(el('span', { title: 'Tiene descripción' }, '📝'));
  if (t.notes?.length) meta.push(el('span', { title: 'Notas' }, `💬 ${t.notes.length}`));

  const row = el('div', { class: `task${t.done ? ' done' : ''}` });
  row.append(
    el('button', {
      class: `check${t.priority < 4 ? ' p' + t.priority : ''}`,
      'aria-label': t.done ? `Reabrir «${t.text}»` : `Completar «${t.text}»`,
      onclick: () => { row.classList.add('leaving'); toggle(t); },
    }, '✓'),
    el('button', { class: 'task-main', onclick: () => openDetail(t.id) },
      el('div', { class: 'task-text' }, t.text),
      meta.length ? el('div', { class: 'meta' }, meta) : null),
  );
  return row;
}

function projectsView() {
  const out = [];
  for (const p of state.projects) {
    out.push(el('button', { class: 'project-row', onclick: () => go('+' + p.name) },
      el('span', {}, p.name), el('span', { class: 'count' }, p.open || '')));
  }
  if (state.labels.length) {
    out.push(el('h2', { class: 'group-title' }, 'Etiquetas'));
    for (const l of state.labels) {
      out.push(el('button', { class: 'project-row', onclick: () => go('#' + l.name) },
        el('span', { class: 'label' }, '#' + l.name), el('span', { class: 'count' }, l.open || '')));
    }
  }
  return out.length ? out : [el('p', { class: 'empty' }, 'Aún no hay proyectos.')];
}

// ---------- acciones ----------

/** Completa o reabre. Deshacer devuelve la tarea a como estaba. */
async function toggle(t) {
  const before = structuredClone(t);
  await mutate(
    () => api(`tasks/${t.id}/done`, { method: 'POST' }),
    t.done ? 'Reabierta' : (t.recurrence ? 'Completada · se repite' : 'Completada'),
    async (r) => {
      // Recurrente: se borra la ocurrencia completada y se repone la original.
      if (r.occurrence) await api(`tasks/${r.occurrence.id}`, { method: 'DELETE' });
      await api(`tasks/${before.id}`, { method: 'PUT', body: before });
    },
  ).catch(() => refresh());
}

async function addTask(text) {
  const body = { text };
  const p = defaultProject();
  if (p) body.project = p;
  const t = await mutate(() => api('tasks', { method: 'POST', body }), null);
  const where = t.due ? `${relative(t.due)}${t.time ? ' ' + t.time : ''}` : t.project;
  toast(`Añadida: ${t.text} · ${where}`, {
    label: 'Deshacer',
    run: () => mutate(() => api(`tasks/${t.id}`, { method: 'DELETE' }), 'Eliminada'),
  });
  metaAt = 0;
}

// ---------- detalle ----------

const PRIORITY = { 1: 'Alta (p1)', 2: 'Media (p2)', 3: 'Baja (p3)', 4: 'Sin prioridad' };

async function openDetail(id) {
  const dlg = $('detail');
  const body = $('detail-body');
  body.replaceChildren(el('p', { class: 'muted' }, 'Cargando…'));
  if (!dlg.open) dlg.showModal();
  try {
    const t = await api(`tasks/${id}`);
    body.replaceChildren(...detailView(t));
  } catch (e) {
    body.replaceChildren(el('p', { class: 'error' }, e.message));
  }
}

function detailView(t) {
  const fields = [];
  const add = (k, v) => fields.push(el('dt', {}, k), el('dd', {}, v));
  add('Estado', t.done ? `Completada${t.done_date ? ' ' + doneWhen(t.done_date) : ''}` : 'Pendiente');
  if (t.due) add('Fecha', `${relative(t.due)} (${t.due})${t.time ? ' a las ' + t.time : ''}`);
  if (t.deadline) add('Límite', `${relative(t.deadline)} (${t.deadline})`);
  if (t.recurrence) add('Repite', t.recurrence);
  add('Prioridad', PRIORITY[t.priority] || t.priority);
  add('Proyecto', t.project + (t.section ? ` / ${t.section}` : ''));
  if (t.labels.length) add('Etiquetas', t.labels.map((l) => '#' + l).join(' '));
  add('Id', t.id);

  const reopen = () => openDetail(t.id);
  const patch = (body, msg) => mutate(() => api(`tasks/${t.id}`, { method: 'PATCH', body }), msg).then(reopen, () => {});

  // Acciones rápidas.
  const actions = el('div', { class: 'actions' },
    el('button', { class: 'primary', onclick: async () => { await toggle(t); reopen(); } },
      t.done ? 'Reabrir' : 'Completar'),
    el('button', { class: 'secondary', onclick: () => patch({ text: 'hoy' }, 'Para hoy') }, 'Hoy'),
    el('button', { class: 'secondary', onclick: () => patch({ text: 'mañana' }, 'Para mañana') }, 'Mañana'),
    t.due && el('button', { class: 'secondary', onclick: () => patch({ no_date: true }, 'Sin fecha') }, 'Sin fecha'),
    t.deadline && el('button', { class: 'secondary', onclick: () => patch({ no_deadline: true }, 'Sin límite') }, 'Quitar límite'),
  );

  // Cambio con captura rápida: solo cambia lo que escribas.
  const quick = el('input', { placeholder: 'viernes 10:00 p2 +Casa limit lunes', enterkeyhint: 'done' });
  const quickForm = el('form', {
    class: 'stack',
    onsubmit: (e) => {
      e.preventDefault();
      if (quick.value.trim()) patch({ text: quick.value.trim() }, 'Actualizada');
    },
  }, quick, el('p', { class: 'muted', style: 'margin:0;font-size:13px' },
    'Solo cambia lo que escribas: fecha, hora, prioridad, etiquetas, proyecto o límite. Si escribes texto, también cambia el título.'));

  // Descripción.
  const desc = el('textarea', { rows: 4, placeholder: 'Detalles, enlaces, pasos…' });
  desc.value = t.description || '';
  const descForm = el('form', {
    class: 'stack',
    onsubmit: (e) => { e.preventDefault(); patch({ description: desc.value }, 'Descripción guardada'); },
  }, desc, el('div', {}, el('button', { class: 'secondary', type: 'submit' }, 'Guardar descripción')));

  // Notas con fecha.
  const notes = (t.notes || []).map((n) => el('div', { class: 'note' },
    n.date ? el('time', { datetime: n.date }, relative(n.date)) : null, n.text));
  const note = el('input', { placeholder: 'Añadir nota de seguimiento', enterkeyhint: 'send' });
  const noteForm = el('form', {
    class: 'add-row',
    onsubmit: (e) => {
      e.preventDefault();
      const text = note.value.trim();
      if (!text) return;
      mutate(() => api(`tasks/${t.id}/notes`, { method: 'POST', body: { text } }), 'Nota añadida').then(reopen, () => {});
    },
  }, note, el('button', { class: 'primary add-btn', type: 'submit', 'aria-label': 'Añadir nota' }, '＋'));

  // Borrar en dos toques.
  const del = el('button', { class: 'danger' }, 'Eliminar tarea');
  del.addEventListener('click', () => {
    if (!del.dataset.armed) {
      del.dataset.armed = '1';
      del.textContent = 'Toca otra vez para eliminar';
      setTimeout(() => { delete del.dataset.armed; del.textContent = 'Eliminar tarea'; }, 4000);
      return;
    }
    $('detail').close();
    mutate(() => api(`tasks/${t.id}`, { method: 'DELETE' }), 'Eliminada', () =>
      api(`tasks/${t.id}`, { method: 'PUT', body: t })).catch(() => {});
  });

  return [
    el('h2', {}, t.text),
    el('dl', { class: 'fields' }, fields),
    actions,
    el('h3', { class: 'section-title' }, 'Cambiar'),
    quickForm,
    el('h3', { class: 'section-title' }, 'Descripción'),
    descForm,
    el('h3', { class: 'section-title' }, `Notas${notes.length ? ` (${notes.length})` : ''}`),
    ...notes,
    noteForm,
    el('div', { class: 'actions', style: 'margin-top:24px' }, del),
  ];
}

// ---------- autocompletado de +Proyecto y #etiqueta ----------

/** Palabra que se está escribiendo si empieza con + o #. */
function currentToken(input) {
  const upto = input.value.slice(0, input.selectionStart ?? input.value.length);
  const m = upto.match(/(?:^|\s)([+#@])([^\s]*)$/);
  return m && { sigil: m[1] === '@' ? '#' : m[1], word: m[2], start: upto.length - m[2].length - 1 };
}

function updateSuggestions() {
  const input = $('add-input');
  const box = $('sugg');
  const tok = currentToken(input);
  if (!tok) { box.hidden = true; return; }
  const pool = tok.sigil === '+' ? state.projects : state.labels;
  const w = norm(tok.word.replace(/_/g, ' '));
  const opts = pool
    .filter((x) => norm(x.name).includes(w))
    .sort((a, b) => norm(a.name).startsWith(w) === norm(b.name).startsWith(w)
      ? b.open - a.open : norm(a.name).startsWith(w) ? -1 : 1)
    .slice(0, 8);
  if (!opts.length) { box.hidden = true; return; }
  box.replaceChildren(...opts.map((o) => el('button', {
    type: 'button',
    class: 'chip',
    // mousedown evita que el campo pierda el foco (y se cierre el teclado).
    onmousedown: (e) => e.preventDefault(),
    onclick: () => accept(tok, o.name),
  }, tok.sigil + o.name, o.open ? el('span', { class: 'count' }, o.open) : null)));
  box.hidden = false;
}

function accept(tok, name) {
  const input = $('add-input');
  const value = tok.sigil + (tok.sigil === '+' ? name.replace(/ /g, '_') : name) + ' ';
  const end = input.selectionStart ?? input.value.length;
  input.value = input.value.slice(0, tok.start) + value + input.value.slice(end);
  const pos = tok.start + value.length;
  input.setSelectionRange(pos, pos);
  input.focus();
  updateSuggestions();
}

// ---------- sesión ----------

function logout(msg) {
  state.token = null;
  save(TOKEN_KEY, null);
  for (const k of Object.keys(localStorage)) if (k.startsWith('tasktui.cache.')) save(k, null);
  $('app').hidden = true;
  $('login').hidden = false;
  $('login-error').textContent = msg || '';
  $('menu').open && $('menu').close();
}

async function login(token) {
  state.token = token;
  try {
    await api('projects');
  } catch (e) {
    if (e.status !== 401) $('login-error').textContent = e.message;
    state.token = null;
    return;
  }
  save(TOKEN_KEY, token);
  start();
}

function start() {
  $('login').hidden = true;
  $('app').hidden = false;
  $('show-done').checked = state.showDone;
  $('server-info').textContent = `Servidor: ${location.host}`;
  readHash();
}

// ---------- eventos ----------

function bind() {
  $('login-form').addEventListener('submit', (e) => {
    e.preventDefault();
    $('login-error').textContent = '';
    login($('token').value.trim());
  });

  for (const b of document.querySelectorAll('.tab')) b.addEventListener('click', () => go(b.dataset.view));
  window.addEventListener('hashchange', readHash);

  $('btn-refresh').addEventListener('click', () => { metaAt = 0; refresh(); });
  $('btn-search').addEventListener('click', () => {
    const row = $('search-row');
    row.hidden = !row.hidden;
    if (!row.hidden) { go('search'); $('search').focus(); } else if (state.view === 'search') go('today');
  });
  let searchTimer;
  $('search').addEventListener('input', (e) => {
    state.query = e.target.value;
    clearTimeout(searchTimer);
    searchTimer = setTimeout(refresh, 250);
  });

  $('btn-menu').addEventListener('click', () => $('menu').showModal());
  $('show-done').addEventListener('change', (e) => {
    state.showDone = e.target.checked;
    save(DONE_KEY, state.showDone ? '1' : null);
    refresh();
  });
  $('btn-logout').addEventListener('click', () => logout());

  const input = $('add-input');
  input.addEventListener('input', updateSuggestions);
  input.addEventListener('click', updateSuggestions);
  input.addEventListener('keydown', (e) => {
    const first = $('sugg').querySelector('.chip');
    if (e.key === 'Tab' && first && !$('sugg').hidden) {
      e.preventDefault();
      first.click();
    }
  });
  input.addEventListener('blur', () => setTimeout(() => { $('sugg').hidden = true; }, 150));
  $('add-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    const text = input.value.trim();
    if (!text) return;
    try {
      await addTask(text);
      input.value = '';
      $('sugg').hidden = true;
    } catch { /* el aviso ya se mostró y el texto se conserva */ }
  });

  // Al cerrar el detalle se recarga la lista; al volver a la app también.
  $('detail').addEventListener('close', () => refresh());
  // Tocar fuera de la hoja la cierra.
  for (const d of document.querySelectorAll('dialog')) {
    d.addEventListener('click', (e) => { if (e.target === d) d.close(); });
  }
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible' && state.token) refresh();
  });
}

bind();
if (state.token) start();
else $('login').hidden = false;

if ('serviceWorker' in navigator) {
  navigator.serviceWorker.register('/sw.js').catch(() => { /* la app funciona sin él */ });
}
