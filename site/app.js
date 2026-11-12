// Página de tasktui: captura rápida y una réplica de la TUI, sin dependencias.
// La réplica imita la interfaz real (internal/tui) con tareas de ejemplo; no guarda nada.
(() => {
  'use strict';
  const $ = (s, el = document) => el.querySelector(s);
  const $$ = (s, el = document) => [...el.querySelectorAll(s)];
  const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));

  // ---------- fechas en español ----------
  const DAYS = ['dom', 'lun', 'mar', 'mié', 'jue', 'vie', 'sáb'];
  const MONTHS = ['ene', 'feb', 'mar', 'abr', 'may', 'jun', 'jul', 'ago', 'sep', 'oct', 'nov', 'dic'];
  const WEEK = { domingo: 0, lunes: 1, martes: 2, miercoles: 3, jueves: 4, viernes: 5, sabado: 6,
    sunday: 0, monday: 1, tuesday: 2, wednesday: 3, thursday: 4, friday: 5, saturday: 6 };
  const plain = (s) => s.toLowerCase().normalize('NFD').replace(/[̀-ͯ]/g, '');
  const today = new Date(); today.setHours(0, 0, 0, 0);
  const dayAt = (off) => { const d = new Date(today); d.setDate(d.getDate() + off); return d; };
  const rel = (off) => {
    if (off === 0) return 'hoy';
    if (off === 1) return 'mañana';
    if (off === -1) return 'ayer';
    const d = dayAt(off);
    return off > 1 && off < 7 ? DAYS[d.getDay()] : `${d.getDate()} ${MONTHS[d.getMonth()]}`;
  };
  const long = (off) => { const d = dayAt(off); return `${DAYS[d.getDay()]} ${d.getDate()} ${MONTHS[d.getMonth()]}`; };
  const away = (off) => off === 0 ? 'hoy' : off === 1 ? 'mañana' : off === -1 ? 'venció ayer'
    : off < 0 ? `venció hace ${-off} días` : `en ${off} días`;

  // ---------- captura rápida (subconjunto de internal/core/quick.go) ----------
  function dateAt(w, i) {
    const a = plain(w[i] || ''), b = plain(w[i + 1] || ''), c = plain(w[i + 2] || '');
    if (a === 'hoy' || a === 'today') return { off: 0, n: 1 };
    if (a === 'manana' || a === 'tomorrow') return { off: 1, n: 1 };
    if (a === 'pasado' && b === 'manana') return { off: 2, n: 2 };
    if (a in WEEK) return { off: ((WEEK[a] - today.getDay() + 6) % 7) + 1, n: 1 };
    if ((a === 'en' || a === 'in') && /^\d{1,3}$/.test(b)) {
      if (/^(dia|dias|day|days)$/.test(c)) return { off: +b, n: 3 };
      if (/^(semana|semanas|week|weeks)$/.test(c)) return { off: +b * 7, n: 3 };
    }
    const m = a.match(/^(\d{1,2})\/(\d{1,2})(?:\/(\d{4}))?$/);
    if (m && +m[2] >= 1 && +m[2] <= 12 && +m[1] >= 1 && +m[1] <= 31) {
      let d = new Date(m[3] ? +m[3] : today.getFullYear(), +m[2] - 1, +m[1]);
      if (!m[3] && d < today) d = new Date(today.getFullYear() + 1, +m[2] - 1, +m[1]);
      return { off: Math.round((d - today) / 864e5), n: 1 };
    }
    return null;
  }
  function timeOf(word) {
    const s = word.toLowerCase();
    let m = s.match(/^(\d{1,2}):(\d{2})$/);
    if (m && +m[1] < 24 && +m[2] < 60) return `${m[1].padStart(2, '0')}:${m[2]}`;
    m = s.match(/^(\d{1,2})(?::(\d{2}))?(am|pm)$/);
    if (m && +m[1] >= 1 && +m[1] <= 12) {
      const h = (+m[1] % 12) + (m[3] === 'pm' ? 12 : 0);
      return `${String(h).padStart(2, '0')}:${m[2] || '00'}`;
    }
    return null;
  }
  function recAt(w, i) {
    if (!/^(cada|every)$/.test(plain(w[i] || ''))) return null;
    const b = plain(w[i + 1] || ''), c = plain(w[i + 2] || '');
    const unit = (u) => /^(dia|dias|day|days)$/.test(u) ? ['día', 'días'] : /^(semana|semanas|week|weeks)$/.test(u) ? ['semana', 'semanas']
      : /^(mes|meses|month|months)$/.test(u) ? ['mes', 'meses'] : /^(ano|anos|year|years)$/.test(u) ? ['año', 'años'] : null;
    if (/^\d{1,3}$/.test(b) && unit(c)) return { text: `cada ${+b} ${unit(c)[+b === 1 ? 0 : 1]}`, n: 3 };
    if (unit(b)) return { text: `cada ${unit(b)[0]}`, n: 2 };
    if (b in WEEK) return { text: `cada ${DAYS[WEEK[b]]}`, n: 2 };
    return null;
  }
  function parseQuick(input) {
    const w = input.trim().split(/\s+/).filter(Boolean);
    const q = { text: '', labels: [], project: null, prio: null, due: null, time: null, deadline: null, rec: null, tokens: [] };
    const words = [];
    const tok = (i, n, kind) => q.tokens.push({ t: w.slice(i, i + n).join(' '), kind });
    for (let i = 0; i < w.length;) {
      const x = w[i];
      let d, r, t;
      if (x.startsWith('\\') && x.length > 1) { words.push(x.slice(1)); tok(i, 1, 'text'); i++; }
      else if (/^[#@][^\s#@]+$/.test(x)) { q.labels.push(x.slice(1)); tok(i, 1, 'label'); i++; }
      else if (/^\+\S+$/.test(x)) { q.project = x.slice(1).replace(/_/g, ' '); tok(i, 1, 'project'); i++; }
      else if (/^p[1-4]$/i.test(x)) { q.prio = +x[1]; tok(i, 1, 'prio'); i++; }
      else if (plain(x) === 'limit' && (d = dateAt(w, i + 1))) { q.deadline = d.off; tok(i, d.n + 1, 'deadline'); i += d.n + 1; }
      else if ((r = recAt(w, i))) { q.rec = r.text; tok(i, r.n, 'rec'); i += r.n; }
      else if ((d = dateAt(w, i))) { q.due = d.off; tok(i, d.n, 'due'); i += d.n; }
      else if ((t = timeOf(x))) { q.time = t; tok(i, 1, 'time'); i++; }
      else { words.push(x); tok(i, 1, 'text'); i++; }
    }
    if (q.time && q.due === null) q.due = 0;
    q.text = words.join(' ');
    return q;
  }
  const whenClass = (off) => off < 0 ? 'late' : off === 0 ? 'today' : 'soon';

  // ---------- sección «captura rápida» ----------
  const playIn = $('#play-in');
  if (playIn) {
    const row = (k, v, cls = '') => `<dt>${k}</dt><dd class="${v ? cls : 'none'}">${v || '—'}</dd>`;
    const renderPlay = () => {
      const q = parseQuick(playIn.value);
      // Las palabras de texto seguidas se juntan en una sola pieza.
      const merged = [];
      for (const t of q.tokens) {
        const last = merged[merged.length - 1];
        if (t.kind === 'text' && last && last.kind === 'text') last.t += ' ' + t.t;
        else merged.push({ ...t });
      }
      $('#play-tokens').innerHTML = merged.map((t) => `<span class="tk ${t.kind}">${esc(t.t)}</span>`).join('');
      $('#play-out').innerHTML =
        row('Tarea', esc(q.text)) +
        row('Fecha', q.due === null ? '' : `${long(q.due)}${q.time ? ' · ' + q.time : ''} <span class="cm">· ${away(q.due).replace('venció ', '')}</span>`, whenClass(q.due ?? 0)) +
        row('Límite', q.deadline === null ? '' : long(q.deadline), 'c-red') +
        row('Prioridad', q.prio ? `p${q.prio} · ${['alta', 'media', 'baja', 'ninguna'][q.prio - 1]}` : '', `p${q.prio}`) +
        row('Repite', q.rec || '', 'c-cyan') +
        row('Etiquetas', q.labels.map((l) => '#' + esc(l)).join('  '), 'c-mauve') +
        row('Proyecto', q.project ? '▸ ' + esc(q.project) : '<span class="cm">Inbox</span>', 'c-blue');
    };
    playIn.addEventListener('input', renderPlay);
    let typing = 0;
    $$('.play-ex button').forEach((b) => b.addEventListener('click', () => {
      const text = b.dataset.ex, id = ++typing;
      if (matchMedia('(prefers-reduced-motion: reduce)').matches) { playIn.value = text; renderPlay(); return; }
      playIn.value = '';
      let i = 0;
      (function step() {
        if (id !== typing) return;
        playIn.value = text.slice(0, ++i);
        renderPlay();
        if (i < text.length) setTimeout(step, 22);
      })();
    }));
    renderPlay();
  }

  // ---------- réplica de la TUI ----------
  const root = $('#tui');
  if (root) {
    let seq = 0;
    const mk = (quick, extra = {}) => {
      const q = parseQuick(quick);
      return { id: ++seq, text: q.text, labels: q.labels, project: q.project || 'Inbox', prio: q.prio || 4, due: q.due, time: q.time,
        deadline: q.deadline, rec: q.rec, desc: '', notes: [], done: false, ...extra };
    };
    let tasks = [
      mk('Renovar el dominio hoy p1 +Trabajo #web limit hoy', { due: -1, desc: 'Vence el registro; pagar dos años de una vez.', notes: ['El cobro automático falló, hay que hacerlo a mano'] }),
      mk('Responder el correo del contador hoy +Finanzas', { due: -3 }),
      mk('Llamar al dentista hoy 12:30 p3 #salud'),
      mk('Pagar la luz hoy 18:00 p1 +Finanzas #casa cada mes'),
      mk('Revisar el pull request de la API hoy p2 +Trabajo #código', { desc: 'Faltan pruebas del endpoint de exportación.' }),
      mk('Regar las plantas hoy #casa cada 3 días'),
      mk('Comprar café en grano mañana #compras'),
      mk('Preparar la demo en 3 días 10:00 p2 +Trabajo', { notes: ['Usar los datos de prueba, no producción'] }),
      mk('Respaldar la NAS en 4 días +Casa cada semana'),
      mk('Leer el capítulo 4 +Lectura #libros'),
      mk('Planear las vacaciones de diciembre p3 +Casa'),
      mk('Ordenar el escritorio #casa'),
      mk('Enviar la factura de octubre hoy +Finanzas', { done: true }),
      mk('Sacar la basura hoy #casa', { done: true }),
    ];
    const st = { view: 'hoy', focus: 'list', sel: 0, mode: 'normal', msg: '', query: '', g: false, undo: [], showDone: false };

    const sideItems = () => {
      const open = tasks.filter((t) => !t.done);
      const count = (f) => open.filter(f).length;
      const items = [
        { id: 'inbox', icon: '▣', c: 'c-blue', label: 'Inbox', n: count((t) => t.project === 'Inbox') },
        { id: 'hoy', icon: '◉', c: 'c-green', label: 'Hoy', n: count((t) => t.due !== null && t.due <= 0) },
        { id: 'prox', icon: '◷', c: 'c-cyan', label: 'Próximos', n: count((t) => t.due > 0) },
        { id: 'todas', icon: '≡', c: 'c-sub', label: 'Todas', n: open.length },
        { head: 'PROYECTOS' },
      ];
      [...new Set(tasks.map((t) => t.project))].filter((p) => p !== 'Inbox').sort()
        .forEach((p) => items.push({ id: '+' + p, icon: '▸', c: 'c-yellow', label: p, n: count((t) => t.project === p) }));
      const labels = [...new Set(open.flatMap((t) => t.labels))].sort();
      if (labels.length) items.push({ head: 'ETIQUETAS' });
      labels.forEach((l) => items.push({ id: '#' + l, icon: '#', c: 'c-mauve', label: l, n: count((t) => t.labels.includes(l)) }));
      return items;
    };
    const viewTitle = () => ({ inbox: 'Inbox', hoy: 'Hoy', prox: 'Próximos', todas: 'Todas' }[st.view] || st.view.slice(1));

    // Grupos de la vista actual: [{title, late, items}]
    function groups() {
      let list = tasks.filter((t) => st.showDone || !t.done);
      if (st.query) list = list.filter((t) => plain(t.text + ' ' + t.labels.join(' ') + ' ' + t.project).includes(plain(st.query)));
      const byDue = (a, b) => (a.due ?? 1e9) - (b.due ?? 1e9) || (a.time || '99').localeCompare(b.time || '99') || a.prio - b.prio;
      const v = st.view;
      if (v === 'hoy') {
        const late = list.filter((t) => t.due !== null && t.due < 0).sort(byDue);
        const now = list.filter((t) => t.due === 0).sort(byDue);
        return [late.length && { title: 'Vencidas', late: true, items: late }, now.length && { title: `${long(0)} · hoy`, items: now }].filter(Boolean);
      }
      if (v === 'prox') {
        const out = [];
        list.filter((t) => t.due > 0).sort(byDue).forEach((t) => {
          const title = `${long(t.due)} · ${away(t.due)}`;
          const g = out.find((x) => x.title === title) || out[out.push({ title, items: [] }) - 1];
          g.items.push(t);
        });
        return out;
      }
      if (v === 'todas') {
        return [...new Set(list.map((t) => t.project))].sort((a, b) => (a === 'Inbox' ? -1 : b === 'Inbox' ? 1 : a.localeCompare(b)))
          .map((p) => ({ title: p, items: list.filter((t) => t.project === p) }));
      }
      const f = v === 'inbox' ? (t) => t.project === 'Inbox' : v[0] === '+' ? (t) => t.project === v.slice(1) : (t) => t.labels.includes(v.slice(1));
      const items = list.filter(f);
      return items.length ? [{ items }] : [];
    }
    const flat = () => groups().flatMap((g) => g.items);
    const current = () => flat()[st.sel] || null;

    const HINTS = [['a', 'añadir'], ['x', 'completar'], ['e', 'editar'], ['d', 'borrar'], ['/', 'buscar'], ['u', 'deshacer'], ['?', 'ayuda']];
    const HELP = [
      ['Navegación', [['j / k  ↓ / ↑', 'moverse'], ['h / l', 'barra lateral ↔ lista'], ['gi gt gu ga', 'Inbox · Hoy · Próximos · Todas'], ['/  Esc', 'buscar · limpiar búsqueda'], ['H', 'mostrar u ocultar completadas']]],
      ['Tareas', [['a', 'añadir (captura rápida)'], ['e', 'editar con la misma sintaxis'], ['x  espacio', 'completar / reabrir'], ['t', 'poner para hoy'], ['1 2 3 4', 'prioridad'], ['d', 'eliminar'], ['u', 'deshacer']]],
    ];

    function rowHTML(t, i, showProject) {
      const cls = ['t-row', i === st.sel ? 'sel' : '', t.done ? 'done' : ''].join(' ');
      const check = t.done ? '✓' : t.prio === 1 ? '◉' : '○';
      const when = `<span class="when ${t.done || t.due === null ? '' : whenClass(t.due)}">${t.due === null ? '' : rel(t.due) + (t.time ? ' ' + t.time : '')}</span>`;
      const dl = `<span class="when dl ${t.deadline !== null && t.deadline <= 0 ? 'late' : 'soon'}">${t.deadline === null || t.done ? '' : '⚑ ' + rel(t.deadline)}</span>`;
      const icons = (t.desc ? '¶' : '') + (t.notes.length ? '✎' : '') + (t.rec ? '<span class="c-cyan">↻</span>' : '');
      return `<div class="${cls}" data-i="${i}">
        <button class="t-check p${t.prio}" data-check="${i}" tabindex="-1" aria-label="${t.done ? 'Reabrir' : 'Completar'}: ${esc(t.text)}">${check}</button>
        <span class="grow">${esc(t.text)}${t.labels.map((l) => `<span class="lab">#${esc(l)}</span>`).join('')}</span>
        ${when}${dl}${showProject ? `<span class="proj">${esc(t.project)}</span>` : ''}<span class="icons">${icons}</span></div>`;
    }

    function detailHTML(t) {
      if (!t) return '<div class="t-empty"><span class="cm">Elige una tarea para ver su detalle</span></div>';
      const f = [];
      if (t.due !== null) f.push(['◷ Fecha', `<span class="${t.done ? '' : whenClass(t.due)}">${long(t.due)}${t.time ? ' · ' + t.time : ''}</span> <small>· ${away(t.due)}</small>`]);
      if (t.deadline !== null) f.push(['⚑ Límite', `<span class="late">${long(t.deadline)}</span> <small>· ${away(t.deadline)}</small>`]);
      if (t.prio < 4) f.push(['▲ Prioridad', `<span class="p${t.prio}">p${t.prio} · ${['alta', 'media', 'baja'][t.prio - 1]}</span>`]);
      if (t.rec) f.push(['↻ Repite', t.rec]);
      if (t.labels.length) f.push(['# Etiquetas', `<span class="c-mauve">${t.labels.map((l) => '#' + esc(l)).join('  ')}</span>`]);
      return `<div class="t-detail"><h3>${esc(t.text)}</h3><div class="where">▸ ${esc(t.project)}</div>
        <dl>${f.map(([k, v]) => `<dt>${k}</dt><dd>${v}</dd>`).join('')}</dl>
        ${t.desc ? `<h4>Descripción</h4><p>${esc(t.desc)}</p>` : ''}
        ${t.notes.length ? `<h4>Notas · ${t.notes.length}</h4>${t.notes.map((n) => `<p class="note"><small>hoy</small>  ${esc(n)}</p>`).join('')}` : ''}
        <div class="id">id ${(t.id * 7919 % 46656).toString(36).padStart(3, 'a')}${'xk'}</div></div>`;
    }

    function render() {
      const gs = groups(), all = gs.flatMap((g) => g.items);
      st.sel = Math.max(0, Math.min(st.sel, all.length - 1));
      const showProject = !['inbox', 'todas'].includes(st.view) && st.view[0] !== '+';
      const side = sideItems().map((it) => it.head ? `<div class="t-head">${it.head}</div>`
        : `<div class="t-row ${it.id === st.view ? 'sel' : ''}" data-view="${esc(it.id)}"><span class="ic ${it.c}">${it.icon}</span><span class="grow">${esc(it.label)}</span><span class="n">${it.n || ''}</span></div>`).join('');
      const doneToday = tasks.filter((t) => t.done).length, openToday = tasks.filter((t) => !t.done && t.due !== null && t.due <= 0).length;
      const total = doneToday + openToday;
      let i = 0;
      const list = all.length ? gs.map((g) => (g.title ? `<div class="t-group ${g.late ? 'late' : ''}">${esc(g.title)}</div>` : '') +
        g.items.map((t) => rowHTML(t, i++, showProject)).join('')).join('')
        : `<div class="t-empty">${st.query ? `<b class="c-yellow">∅</b><span>Sin resultados para «${esc(st.query)}»</span><span class="cm">Esc  limpiar búsqueda</span>`
          : '<b>✓</b><span>Nada pendiente por aquí</span><span class="cm">a  añadir tarea</span>'}</div>`;
      const pill = { normal: ['NORMAL', ''], input: [st.kind === 'search' ? 'BUSCAR' : 'ESCRIBIR', 'write'], help: ['AYUDA', 'help'], confirm: ['ELIMINAR', 'del'] }[st.mode];
      const mid = st.mode === 'confirm' ? `<span class="t-msg c-red">¿Eliminar «${esc(current()?.text || '')}»?  <b>y</b> sí · <b>n</b> no</span>`
        : st.msg ? `<span class="t-msg">${esc(st.msg)}</span>`
          : `<span class="hints">${HINTS.map(([k, l]) => `<button type="button" data-key="${k}" tabindex="-1"><b>${k}</b> ${l}</button>`).join('')}</span>`;
      const title = `${viewTitle()} · ${all.length}${st.query ? ' · /' + esc(st.query) : ''}`;
      root.innerHTML = `
        <div class="t-panes">
          <div class="t-pane side ${st.focus === 'side' ? 'focus' : ''}"><span class="pt">tasktui</span><div class="t-body">${side}</div>
            ${total ? `<div class="t-prog"><div><span>hoy</span><b>${doneToday}/${total}</b></div><div class="bar"><span style="width:${(doneToday / total) * 100}%"></span></div></div>` : ''}</div>
          <div class="t-pane list ${st.focus === 'list' ? 'focus' : ''}"><span class="pt">${title}</span><div class="t-body">${list}</div></div>
          <div class="t-pane detail"><span class="pt">Detalle</span><div class="t-body">${detailHTML(all[st.sel])}</div></div>
        </div>
        <div class="t-status"><span class="pill ${pill[1]}">${pill[0]}</span>${mid}<span class="t-right"><span>▤ demo</span>${long(0)}</span></div>
        <div class="t-pop pane" id="t-pop" ${st.mode === 'input' ? '' : 'hidden'}>
          <span class="pt" id="t-pop-title"></span>
          <div class="t-in"><span class="prompt">❯</span><input id="t-input" type="text" autocomplete="off" autocapitalize="off" spellcheck="false" aria-labelledby="t-pop-title"></div>
          <div class="t-prev" id="t-prev"></div>
          <div class="t-tip" id="t-tip"></div>
        </div>
        ${st.mode === 'help' ? `<div class="t-pop t-help pane"><span class="pt">Atajos · cualquier tecla para cerrar</span>${HELP.map(([h, ks]) =>
          `<h4>${h}</h4><dl>${ks.map(([k, d]) => `<dt>${k}</dt><dd>${d}</dd>`).join('')}</dl>`).join('')}</div>` : ''}`;
      // Mantener la fila elegida a la vista sin mover la página (por eso no scrollIntoView).
      const row = $('.list .sel', root), body = row && row.parentElement;
      if (row) {
        if (row.offsetTop < body.scrollTop) body.scrollTop = Math.max(row.offsetTop - 28, 0);
        else if (row.offsetTop + row.offsetHeight > body.scrollTop + body.clientHeight) body.scrollTop = row.offsetTop + row.offsetHeight - body.clientHeight;
      }
      if (st.mode === 'input') openPopup();
    }

    // ---------- ventana de captura ----------
    function openPopup() {
      const input = $('#t-input', root), t = current();
      const titles = { add: `Nueva tarea → ${st.view[0] === '+' ? st.view.slice(1) : 'Inbox'}`, edit: 'Editar tarea', search: 'Buscar' };
      $('#t-pop-title', root).textContent = titles[st.kind];
      $('#t-tip', root).textContent = st.kind === 'search' ? 'Enter fijar · Esc limpiar' : '#etiqueta  +Proyecto  p1-p4  mañana 18:00  limit 20/11  cada lunes';
      if (st.kind === 'edit' && t) input.value = toQuick(t);
      if (st.kind === 'search') input.value = st.query;
      const preview = () => {
        if (st.kind === 'search') { $('#t-prev', root).innerHTML = ''; return; }
        const q = parseQuick(input.value), parts = [];
        const due = q.due ?? (st.kind === 'add' && st.view === 'hoy' ? 0 : null);
        if (due !== null) parts.push(`<span class="today">◷ ${long(due)}${q.time ? ' · ' + q.time : ''}</span>`);
        if (q.deadline !== null) parts.push(`<span class="late">⚑ límite ${long(q.deadline)}</span>`);
        if (q.prio && q.prio < 4) parts.push(`<span class="p${q.prio}">p${q.prio}</span>`);
        if (q.rec) parts.push(`<span class="c-cyan">↻ ${q.rec}</span>`);
        q.labels.forEach((l) => parts.push(`<span class="c-mauve">#${esc(l)}</span>`));
        if (q.project) parts.push(`<span class="c-blue">▸ ${esc(q.project)}</span>`);
        $('#t-prev', root).innerHTML = parts.join('');
      };
      input.addEventListener('input', preview);
      input.addEventListener('keydown', (e) => {
        e.stopPropagation();
        if (e.key === 'Escape') { if (st.kind === 'search') st.query = ''; closeInput(); }
        else if (e.key === 'Enter') { submit(input.value); }
      });
      preview();
      input.focus({ preventScroll: true });
    }
    const toQuick = (t) => [t.text, t.due !== null ? (t.due >= 0 && t.due <= 1 ? rel(t.due) : `${dayAt(t.due).getDate()}/${dayAt(t.due).getMonth() + 1}`) : '', t.time || '',
      t.prio < 4 ? 'p' + t.prio : '', t.project !== 'Inbox' ? '+' + t.project.replace(/ /g, '_') : '', ...t.labels.map((l) => '#' + l), t.rec || '']
      .filter(Boolean).join(' ');
    function closeInput() { st.mode = 'normal'; render(); root.focus({ preventScroll: true }); }
    function submit(value) {
      if (st.kind === 'search') { st.query = value.trim(); st.sel = 0; return closeInput(); }
      const q = parseQuick(value);
      if (!q.text) { st.msg = 'Escribe el texto de la tarea'; return closeInput(); }
      snapshot();
      if (st.kind === 'edit') {
        const t = current();
        Object.assign(t, { text: q.text, labels: q.labels, project: q.project || t.project, prio: q.prio || t.prio, due: q.due, time: q.time, deadline: q.deadline, rec: q.rec });
        st.msg = 'Tarea actualizada';
      } else {
        const t = mk(value);
        if (!q.project && st.view[0] === '+') t.project = st.view.slice(1);
        if (st.view[0] === '#' && !t.labels.includes(st.view.slice(1))) t.labels.push(st.view.slice(1));
        if (t.due === null && st.view === 'hoy') t.due = 0;
        tasks.push(t);
        st.mode = 'normal';
        st.sel = Math.max(0, flat().indexOf(t));
        st.msg = flat().includes(t) ? 'Añadida' : `Añadida a ${t.project}${t.due !== null ? ' · ' + rel(t.due) : ''}`;
      }
      closeInput();
    }

    // ---------- acciones ----------
    const snapshot = () => { st.undo.push(JSON.stringify(tasks)); if (st.undo.length > 50) st.undo.shift(); };
    function toggle(t) {
      if (!t) return;
      snapshot();
      t.done = !t.done;
      st.msg = t.done ? `Completada «${t.text}» · u deshace` : 'Reabierta';
    }
    function setView(v) { st.view = v; st.sel = 0; st.query = ''; }
    function moveSide(d) {
      const ids = sideItems().filter((x) => x.id).map((x) => x.id);
      setView(ids[Math.max(0, Math.min(ids.length - 1, ids.indexOf(st.view) + d))]);
    }
    function key(k) {
      if (st.mode === 'help') { st.mode = 'normal'; return render(); }
      if (st.mode === 'confirm') {
        st.mode = 'normal';
        if (k === 'y' || k === 'Y') { snapshot(); tasks = tasks.filter((t) => t !== current()); st.msg = 'Eliminada · u deshace'; }
        return render();
      }
      st.msg = '';
      const t = current();
      if (st.g) {
        st.g = false;
        const v = { i: 'inbox', t: 'hoy', u: 'prox', a: 'todas' }[k];
        if (v) setView(v); else if (k === 'g') st.sel = 0;
        return render();
      }
      const side = st.focus === 'side';
      switch (k) {
        case 'j': case 'ArrowDown': side ? moveSide(1) : st.sel++; break;
        case 'k': case 'ArrowUp': side ? moveSide(-1) : st.sel--; break;
        case 'h': case 'ArrowLeft': st.focus = 'side'; break;
        case 'l': case 'ArrowRight': case 'Enter': st.focus = 'list'; break;
        case 'g': st.g = true; return;
        case 'G': st.sel = 1e9; break;
        case 'a': st.mode = 'input'; st.kind = 'add'; st.focus = 'list'; break;
        case 'e': if (t) { st.mode = 'input'; st.kind = 'edit'; } break;
        case '/': st.mode = 'input'; st.kind = 'search'; break;
        case 'x': case ' ': toggle(t); break;
        case 'd': if (t) st.mode = 'confirm'; break;
        case 't': if (t) { snapshot(); t.due = 0; st.msg = 'Para hoy'; } break;
        case '1': case '2': case '3': case '4': if (t) { snapshot(); t.prio = +k; st.msg = `Prioridad p${k}`; } break;
        case 'u': if (st.undo.length) { tasks = JSON.parse(st.undo.pop()); st.msg = 'Deshecho'; } else st.msg = 'Nada que deshacer'; break;
        case 'H': st.showDone = !st.showDone; st.msg = st.showDone ? 'Mostrando completadas' : 'Completadas ocultas'; break;
        case '?': st.mode = 'help'; break;
        case 'Escape': if (st.query) st.query = ''; else root.blur(); break;
        default: return false;
      }
      render();
      return true;
    }

    root.addEventListener('keydown', (e) => {
      if (e.metaKey || e.ctrlKey || e.altKey || e.key === 'Tab') return;
      if (key(e.key) !== false) e.preventDefault();
    });
    root.addEventListener('click', (e) => {
      const el = e.target.closest('[data-key],[data-check],[data-i],[data-view]');
      if (!el || st.mode === 'input') return;
      if (el.dataset.key) key(el.dataset.key);
      else if (el.dataset.check) { st.focus = 'list'; st.sel = +el.dataset.check; toggle(current()); render(); }
      else if (el.dataset.i) { st.focus = 'list'; st.sel = +el.dataset.i; st.msg = ''; render(); }
      else { st.focus = 'side'; setView(el.dataset.view); render(); }
      if (st.mode !== 'input') root.focus({ preventScroll: true });
    });
    render();
  }

  // ---------- copiar, pestañas, navegación ----------
  $$('[data-copy]').forEach((b) => b.addEventListener('click', async () => {
    const text = $(b.dataset.copy).innerText.split('\n').map((l) => l.replace(/^❯\s*/, '').replace(/\s+#\s.*$/, '').trimEnd())
      .filter((l) => l && !l.startsWith('#')).join('\n');
    try { await navigator.clipboard.writeText(text); b.textContent = 'copiado ✓'; b.classList.add('ok'); }
    catch { b.textContent = 'no se pudo copiar'; }
    setTimeout(() => { b.textContent = 'copiar'; b.classList.remove('ok'); }, 1600);
  }));

  const tabs = $$('[role="tab"]');
  const pick = (tab, focus) => {
    tabs.forEach((t) => {
      const on = t === tab;
      t.setAttribute('aria-selected', on);
      t.tabIndex = on ? 0 : -1;
      $('#' + t.getAttribute('aria-controls')).hidden = !on;
    });
    if (focus) tab.focus();
  };
  tabs.forEach((t, i) => {
    t.addEventListener('click', () => pick(t));
    t.addEventListener('keydown', (e) => {
      const d = { ArrowRight: 1, ArrowLeft: -1 }[e.key];
      if (d) pick(tabs[(i + d + tabs.length) % tabs.length], true);
    });
  });

  const spies = $$('[data-spy]'), sections = $$('[data-section]');
  const bar = $('#read-bar'), pct = $('#read-pct');
  let ticking = false;
  const onScroll = () => {
    ticking = false;
    const max = document.documentElement.scrollHeight - innerHeight;
    const p = max > 0 ? Math.min(1, scrollY / max) : 0;
    if (bar) { bar.style.width = p * 100 + '%'; pct.textContent = Math.round(p * 100) + '%'; }
    document.documentElement.style.setProperty('--p', p.toFixed(4)); // la aurora se desplaza con la lectura
    let cur = sections[0];
    for (const s of sections) if (s.getBoundingClientRect().top <= innerHeight * 0.35) cur = s;
    spies.forEach((a) => a.dataset.spy === cur.id ? a.setAttribute('aria-current', 'true') : a.removeAttribute('aria-current'));
  };
  addEventListener('scroll', () => { if (!ticking) { ticking = true; requestAnimationFrame(onScroll); } }, { passive: true });
  onScroll();
})();
