/* Axion Optimizer — interface (vanilla JS, no external resources). */
'use strict';
(function () {

  // ---------------------------------------------------------------------------
  // DOM helpers (all machine data is inserted as text, never as HTML)
  // ---------------------------------------------------------------------------

  const $ = (sel, root) => (root || document).querySelector(sel);
  const sleep = ms => new Promise(r => setTimeout(r, ms));

  function h(tag, attrs, ...kids) {
    const el = document.createElement(tag);
    if (attrs) {
      for (const [k, v] of Object.entries(attrs)) {
        if (v == null || v === false) continue;
        if (k === 'class') el.className = v;
        else if (k === 'text') el.textContent = v;
        else if (k.startsWith('on') && typeof v === 'function') el.addEventListener(k.slice(2), v);
        else if (k === 'style' && typeof v === 'object') Object.assign(el.style, v);
        else if (k === 'checked' || k === 'disabled' || k === 'value') el[k] = v;
        else if (v === true) el.setAttribute(k, '');
        else el.setAttribute(k, String(v));
      }
    }
    return add(el, kids);
  }

  function add(el, kids) {
    for (const k of kids.flat(Infinity)) {
      if (k == null || k === false) continue;
      el.appendChild(k instanceof Node ? k : document.createTextNode(String(k)));
    }
    return el;
  }

  const ICONS = {
    grid: '<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>',
    search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
    pulse: '<path d="M3 12h4l3-8 4 16 3-8h4"/>',
    sliders: '<path d="M4 6h10M18 6h2M4 12h4M12 12h8M4 18h12"/><circle cx="16" cy="6" r="2"/><circle cx="10" cy="12" r="2"/><circle cx="18" cy="18" r="2"/>',
    crosshair: '<circle cx="12" cy="12" r="8"/><path d="M12 2v4M12 18v4M2 12h4M18 12h4"/><circle cx="12" cy="12" r="1.2" fill="currentColor"/>',
    gamepad: '<path d="M6 8h12a4 4 0 0 1 4 4v1.5A3.5 3.5 0 0 1 18.5 17c-1.3 0-2.2-.7-2.9-1.6L15 14.5H9l-.6.9C7.7 16.3 6.8 17 5.5 17A3.5 3.5 0 0 1 2 13.5V12a4 4 0 0 1 4-4z"/><path d="M7 10.5v3M5.5 12h3"/><circle cx="16" cy="11" r=".8" fill="currentColor"/><circle cx="18" cy="13" r=".8" fill="currentColor"/>',
    chip: '<rect x="6" y="6" width="12" height="12" rx="2"/><rect x="9.5" y="9.5" width="5" height="5" rx="1"/><path d="M9 2v3M15 2v3M9 19v3M15 19v3M2 9h3M2 15h3M19 9h3M19 15h3"/>',
    clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
    ban: '<circle cx="12" cy="12" r="9"/><path d="M5.6 5.6l12.8 12.8"/>',
    info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 7.5v.5"/>',
    check: '<path d="M5 12.5l4.5 4.5L19 7.5"/>',
    x: '<path d="M6 6l12 12M18 6L6 18"/>',
    alert: '<path d="M12 3.5l9.5 16.5h-19z"/><path d="M12 10v4M12 17v.5"/>',
    alertc: '<circle cx="12" cy="12" r="9"/><path d="M12 7.5v5.5M12 16v.5"/>',
    refresh: '<path d="M20 11a8 8 0 0 0-14.5-4.5L4 8M4 4v4h4M4 13a8 8 0 0 0 14.5 4.5L20 16M20 20v-4h-4"/>',
    zap: '<path d="M13 2L4 14h7l-1 8 9-12h-7z"/>',
    undo: '<path d="M9 14L4 9l5-5"/><path d="M4 9h10a6 6 0 0 1 0 12h-3"/>',
    ext: '<path d="M14 4h6v6M20 4l-9 9M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5"/>',
    chevron: '<path d="M9 6l6 6-6 6"/>',
    shield: '<path d="M12 3l8 3v6c0 4.5-3.2 8.2-8 9-4.8-.8-8-4.5-8-9V6z"/><path d="M8.5 12l2.5 2.5 4.5-5"/>',
    monitor: '<rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8M12 16v4"/>',
    wifi: '<path d="M2 8.5a15 15 0 0 1 20 0M5.5 12a10 10 0 0 1 13 0M9 15.5a5 5 0 0 1 6 0"/><circle cx="12" cy="19" r="1" fill="currentColor"/>',
    mouse: '<rect x="6" y="3" width="12" height="18" rx="6"/><path d="M12 7v4"/>',
    layers: '<path d="M12 3l9 5-9 5-9-5z"/><path d="M3 13l9 5 9-5"/>',
    layout: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 9h18M9 9v11"/>',
    wrench: '<path d="M14.5 5.5a4 4 0 0 0-5.3 5.3L4 16l4 4 5.2-5.2a4 4 0 0 0 5.3-5.3l-2.6 2.6-2.8-.6-.6-2.8z"/>',
    hdd: '<rect x="3" y="6" width="18" height="12" rx="2"/><path d="M7 14h.01M11 14h6"/>',
    ram: '<rect x="3" y="7" width="18" height="9" rx="1.5"/><path d="M7 10v3M11 10v3M15 10v3M6 16v3M18 16v3"/>',
    net: '<rect x="5" y="4" width="14" height="11" rx="2"/><path d="M9 15v5h6v-5M9 8v3M12 8v3M15 8v3"/>',
    windows: '<path d="M3 5.5l8-1.2V11H3zM13 4l8-1.2V11h-8zM3 13h8v6.7l-8-1.2zM13 13h8v8.2L13 20z"/>',
    download: '<path d="M12 4v11M7 10l5 5 5-5M5 20h14"/>',
    folder: '<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    trash: '<path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3"/>',
    rocket: '<path d="M5 15c-1.5 1.5-2 5-2 5s3.5-.5 5-2M14 4c3-1 6 0 6 0s1 3 0 6l-7 7-6-6z"/><circle cx="15.5" cy="8.5" r="1.5"/>',
    restore: '<path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5"/><path d="M12 8v4l3 2"/>',
    book: '<path d="M4 5a2 2 0 0 1 2-2h13v16H6a2 2 0 0 0-2 2z"/><path d="M4 19V5"/>',
    lock: '<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/>',
    gauge: '<path d="M4 17a8 8 0 1 1 16 0"/><path d="M12 17l4-6"/>',
    play: '<path d="M7 4l13 8-13 8z"/>',
    keyboard: '<rect x="2.5" y="6" width="19" height="12" rx="2"/><path d="M6 10h.01M9.5 10h.01M13 10h.01M16.5 10h.01M6 13.5h.01M18 13.5h.01M9 14h6"/>',
    usb: '<path d="M12 3v14"/><path d="M9 6l3-3 3 3"/><path d="M12 13l-4-2V8M12 11l4-2V7"/><circle cx="12" cy="19" r="2"/><rect x="14.5" y="5.5" width="3" height="2" rx=".5"/><circle cx="8" cy="7" r="1"/>',
  };
  const CAT_ICON = { gamepad: 'gamepad', cpu: 'chip', mouse: 'mouse', wifi: 'wifi', monitor: 'monitor', shield: 'lock', refresh: 'refresh', layout: 'layout', layers: 'layers', alert: 'alert', wrench: 'wrench' };

  function icon(name) {
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('viewBox', '0 0 24 24');
    svg.setAttribute('fill', 'none');
    svg.setAttribute('stroke', 'currentColor');
    svg.setAttribute('stroke-width', '1.9');
    svg.setAttribute('stroke-linecap', 'round');
    svg.setAttribute('stroke-linejoin', 'round');
    svg.setAttribute('aria-hidden', 'true');
    svg.innerHTML = ICONS[name] || ''; // static strings defined above
    return svg;
  }

  function logo() {
    return h('img', { class: 'brand-mark', src: 'logo-48.png', alt: '', width: '32', height: '32', decoding: 'async' });
  }

  // ---------------------------------------------------------------------------
  // Formatting
  // ---------------------------------------------------------------------------

  function validDate(iso) {
    const d = new Date(iso);
    return !isNaN(d) && d.getFullYear() > 2000 ? d : null;
  }
  const fmtDT = iso => { const d = validDate(iso); return d ? d.toLocaleString('pt-BR', { dateStyle: 'short', timeStyle: 'short' }) : '—'; };
  const fmtDate = iso => { const d = validDate(iso); return d ? d.toLocaleDateString('pt-BR') : '—'; };
  // Date-only values (driver, BIOS and release dates) are midnight UTC: show that day, not the local one.
  const fmtDay = iso => { const d = validDate(iso); return d ? d.toLocaleDateString('pt-BR', { timeZone: 'UTC' }) : '—'; };
  const fmtTime = iso => { const d = validDate(iso); return d ? d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' }) : ''; };
  const GB = b => { const g = b / 1073741824; return (g >= 10 ? g.toFixed(0) : g.toFixed(1)).replace('.', ',') + ' GB'; };
  const plural = (n, s, p) => Number(n).toLocaleString('pt-BR') + ' ' + (n === 1 ? s : p);
  // Decimal terabytes, the unit SSD makers use for endurance (TBW).
  const TBw = b => b >= 1e12 ? (b / 1e12).toFixed(1).replace('.', ',') + ' TB' : Math.round(b / 1e9) + ' GB';
  const NVME_WARN = [[1, 'reserva abaixo do limite'], [2, 'temperatura'], [4, 'confiabilidade degradada'], [8, 'somente leitura'], [16, 'backup de memória'], [32, 'PMR somente leitura']];
  function diskAlerts(d) {
    const n = d.nvme;
    if (!n) return '—';
    const out = NVME_WARN.filter(([bit]) => n.criticalWarning & bit).map(([, t]) => t);
    if (n.mediaErrors) out.push(plural(n.mediaErrors, 'erro de mídia', 'erros de mídia'));
    if (n.unsafeShutdowns) out.push(n.unsafeShutdowns.toLocaleString('pt-BR') + ' desligamentos sem aviso');
    return out.join(' · ') || 'nenhum';
  }
  const bcdVal = v => !v ? '(padrão)' : /^(no|não|nao)$/i.test(v) ? v + ' (equivale ao padrão)' : v;
  function ago(iso) {
    const d = validDate(iso);
    if (!d) return 'nunca';
    const s = (Date.now() - d.getTime()) / 1000;
    if (s < 60) return 'agora';
    if (s < 3600) return 'há ' + Math.floor(s / 60) + ' min';
    if (s < 86400) return 'há ' + Math.floor(s / 3600) + ' h';
    return 'há ' + plural(Math.floor(s / 86400), 'dia', 'dias');
  }
  // ---------------------------------------------------------------------------
  // API
  // ---------------------------------------------------------------------------
  const sizeLabel = b => !b ? '' : b >= 1e9 ? (b / 1e9).toFixed(1).replace('.', ',') + ' GB' : b >= 1e6 ? Math.round(b / 1e6) + ' MB' : Math.max(1, Math.round(b / 1e3)) + ' KB';

  // The desktop window injects the token. In the browser fallback the page
  // opens with a one-time code (#b=) that is traded for the token: the
  // address left in the browser history holds nothing reusable. The token is
  // kept for this tab only, so reloading works.
  let TOKEN = (window.__AXION__ && window.__AXION__.token) || '';
  if (!TOKEN) { try { TOKEN = sessionStorage.getItem('axion-token') || ''; } catch (e) { /* storage unavailable */ } }
  async function redeemBootCode() {
    const m = location.hash.match(/^#b=([0-9a-f]{32})$/);
    if (!m) return;
    history.replaceState(null, '', location.pathname + '#/painel');
    try {
      const r = await fetch('/api/boot', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ code: m[1] }) });
      const j = await r.json().catch(() => null);
      if (r.ok && j && j.token) {
        TOKEN = j.token;
        try { sessionStorage.setItem('axion-token', TOKEN); } catch (e) { /* storage unavailable */ }
      }
    } catch (e) { /* reported by boot() */ }
  }

  async function api(method, path, body) {
    const opt = { method, headers: { 'X-Axion-Token': TOKEN } };
    if (method !== 'GET') {
      opt.headers['Content-Type'] = 'application/json';
      opt.body = JSON.stringify(body || {});
    }
    let r;
    try {
      r = await fetch(path, opt);
    } catch (e) {
      throw new Error('O Axion não está respondendo. Feche e abra o programa novamente.');
    }
    let j = null;
    try { j = await r.json(); } catch (e) { /* empty body */ }
    if (!r.ok) {
      let msg = r.status === 401 ? 'Acesso expirado: abra o Axion de novo pelo atalho do programa.' : (j && j.error) || ('Erro ' + r.status);
      if (!/^(bcdedit|netsh|fsutil|powercfg|dism|wmi|http)/i.test(msg)) msg = msg.charAt(0).toUpperCase() + msg.slice(1); // server messages start in lower case
      const err = new Error(msg);
      err.status = r.status;
      throw err;
    }
    return j;
  }

  const S = { state: null, cache: {}, busy: null };
  async function get(key, path, force) {
    if (force || !S.cache[key]) S.cache[key] = await api('GET', path);
    return S.cache[key];
  }
  function invalidate() { S.cache = {}; }
  // The server rebuilds the diagnostic after quick changes (startup items,
  // closed programs): drop the cached copy and the counters.
  function staleDiag() { delete S.cache.report; delete S.cache.facts; refreshState(); }
  // Match reports already announced by a toast (the banner and the counter stay until the report is opened).
  const sessToasted = new Set();
  let sessPoll = null;
  async function refreshState() {
    const wasLive = !!(S.state && S.state.session);
    try { S.state = await api('GET', '/api/state'); } catch (e) { /* keep last */ }
    renderChrome();
    const st = S.state || {};
    const fresh = (st.unseen || []).find(n => !sessToasted.has(n.id));
    (st.unseen || []).forEach(n => sessToasted.add(n.id));
    if (fresh) toast('Relatório da partida pronto: ' + fresh.name + ', ' + fmtN(fresh.avgFps) + ' FPS médio, 1% low ' + fmtN(fresh.low1Fps) + '. Veja em Jogos › Relatório da partida.', 'ok', 9000);
    if (wasLive !== !!st.session && currentPage() === 'partidas') { delete S.cache.sessions; rerender(); }
    // While a match is recorded, look again soon (the state is read every 15 s otherwise).
    if (st.session && !sessPoll) sessPoll = setTimeout(() => { sessPoll = null; refreshState(); }, 5000);
  }

  async function openURL(url) {
    try {
      await api('POST', '/api/open', { url });
      if (S.state && S.state.mock) toast('Demonstração: abriria ' + url, 'info');
    } catch (e) { toast(e.message, 'bad'); }
  }

  // External links always open in the user's browser, never inside the app
  // (a middle click too: it would open a new window of the elevated app).
  const externalLink = e => {
    const a = e.target.closest && e.target.closest('a[href]');
    if (!a) return;
    const href = a.getAttribute('href') || '';
    if (/^https?:\/\//.test(href)) {
      e.preventDefault();
      if (e.type === 'click' || e.button === 1) openURL(href);
    }
  };
  document.addEventListener('click', externalLink);
  document.addEventListener('auxclick', externalLink);

  // ---------------------------------------------------------------------------
  // Components
  // ---------------------------------------------------------------------------

  function btn(label, onclick, cls, ic, attrs) {
    return h('button', Object.assign({ class: 'btn ' + (cls || ''), type: 'button', onclick }, attrs || {}), ic ? icon(ic) : null, label ? h('span', { text: label }) : null);
  }
  function badge(text, kind, ic) { return h('span', { class: 'badge ' + (kind || '') }, ic ? icon(ic) : null, text); }
  function extLink(title, url) { return h('a', { href: url, class: 'small', rel: 'noreferrer' }, title); }
  function go(page) { location.hash = '#/' + page; }

  function toast(msg, kind, ms) {
    kind = kind || 'info';
    const el = h('div', { class: 'toast ' + kind, role: 'status' }, icon(kind === 'ok' ? 'check' : kind === 'bad' ? 'alertc' : kind === 'warn' ? 'alert' : 'info'), h('div', { text: msg }));
    $('#toast-root').appendChild(el);
    setTimeout(() => el.remove(), ms || 4500);
  }

  // Modals stack: keys go to the topmost one only. Focus stays inside it
  // (Tab cycles) and returns to where it was when it closes. The first focus
  // never lands on the button that applies something: a second Enter from
  // the click that opened the dialog must not confirm it.
  const MODALS = [];
  const FOCUSABLE = 'button, [href], input, select, textarea, summary, [tabindex]:not([tabindex="-1"])';
  function initialFocus(box) {
    const auto = box.querySelector('[data-autofocus]');
    if (auto) return auto;
    const foot = [...box.querySelectorAll('.mf button')].filter(b => !b.disabled);
    if (foot.length === 1) return foot[0];
    return foot.find(b => !b.classList.contains('primary') && !b.classList.contains('danger')) || box.querySelector('.mh .x') || box.querySelector(FOCUSABLE);
  }
  let modalSeq = 0;
  // After closing, focus goes back to the opener; when a re-render replaced
  // it, to the page title, so keyboard users do not restart at the sidebar.
  function modal(o) {
    const box = h('div', { class: 'modal' + (o.wide ? ' wide' : ''), role: 'dialog', 'aria-modal': 'true' });
    const back = h('div', { class: 'modal-back' }, box);
    const prevFocus = document.activeElement;
    let closed = false;
    let dismiss = o.dismiss !== false;
    const me = {};
    const onKey = e => {
      if (MODALS[MODALS.length - 1] !== me) return;
      if (e.key === 'Escape' && dismiss) { e.preventDefault(); close(); return; }
      if (e.key !== 'Tab') return;
      const f = [...box.querySelectorAll(FOCUSABLE)].filter(el => !el.disabled && el.getClientRects().length);
      if (!f.length) { e.preventDefault(); return; }
      const first = f[0], last = f[f.length - 1], cur = document.activeElement;
      if (e.shiftKey && (cur === first || !box.contains(cur))) { e.preventDefault(); last.focus(); }
      else if (!e.shiftKey && (cur === last || !box.contains(cur))) { e.preventDefault(); first.focus(); }
    };
    function close() {
      if (closed) return;
      closed = true;
      back.remove();
      document.removeEventListener('keydown', onKey);
      const i = MODALS.indexOf(me);
      if (i >= 0) MODALS.splice(i, 1);
      const again = prevFocus && !prevFocus.isConnected && prevFocus.dataset && prevFocus.dataset.focus ? document.querySelector('[data-focus="' + prevFocus.dataset.focus + '"]') : null;
      if (prevFocus && prevFocus.isConnected && typeof prevFocus.focus === 'function') prevFocus.focus();
      else if (again) again.focus();
      else focusPageTitle();
      if (o.onClose) o.onClose();
    }
    const xBtn = () => h('button', { class: 'btn ghost sm x', type: 'button', 'aria-label': 'Fechar', onclick: close }, icon('x'));
    const titleId = 'dlg-' + (++modalSeq);
    box.setAttribute('aria-labelledby', titleId);
    const mh = h('div', { class: 'mh' }, h('h3', { id: titleId, text: o.title }), dismiss ? xBtn() : null);
    const mb = h('div', { class: 'mb' }, o.body);
    const mf = h('div', { class: 'mf' });
    box.append(mh, mb);
    if (o.actions && o.actions.length) { add(mf, o.actions); box.append(mf); }
    back.addEventListener('mousedown', e => { if (dismiss && e.target === back) close(); });
    document.addEventListener('keydown', onKey);
    MODALS.push(me);
    $('#modal-root').appendChild(back);
    setTimeout(() => { const f = initialFocus(box); if (f && !closed) f.focus(); }, 30);
    return Object.assign(me, {
      close, box,
      setTitle(t) { $('.mh h3', box).textContent = t; },
      setBody(n) { mb.replaceChildren(n); },
      setActions(list, focusSel) {
        mf.replaceChildren(); add(mf, list); if (!mf.isConnected) box.append(mf);
        const f = focusSel ? mf.querySelector(focusSel) : null;
        if (f) setTimeout(() => f.focus(), 30);
      },
      // allowDismiss: once a running task ends, Esc, the X and a click outside close it.
      allowDismiss() { if (dismiss) return; dismiss = true; mh.append(xBtn()); },
    });
  }

  // confirmDlg resolves true/false. With o.input, that field gets the focus
  // and Enter in it confirms.
  function confirmDlg(o) {
    return new Promise(resolve => {
      let done = false;
      const finish = v => { if (!done) { done = true; m.close(); resolve(v); } };
      if (o.input) {
        o.input.setAttribute('data-autofocus', '');
        o.input.addEventListener('keydown', e => { if (e.key === 'Enter' && !e.isComposing) { e.preventDefault(); finish(true); } });
      }
      const m = modal({
        title: o.title, body: o.body || h('p', { text: o.text }), wide: o.wide,
        onClose: () => { if (!done) { done = true; resolve(false); } },
        actions: [btn(o.cancel || 'Cancelar', () => finish(false), 'ghost'), btn(o.ok || 'Confirmar', () => finish(true), o.danger ? 'danger fill' : 'primary', o.icon)],
      });
    });
  }

  function toggle(checked, onchange, disabled, title, label) {
    const input = h('input', { type: 'checkbox', checked: !!checked, disabled: !!disabled, 'aria-label': label || title || 'Ativar' });
    const wrap = h('label', { class: 'toggle', title: title || null }, input, h('span', { class: 'track' }), h('span', { class: 'knob' }));
    input.addEventListener('change', () => onchange(input.checked, input));
    return wrap;
  }

  function scoreClass(n) { return n < 0 ? 'none' : n >= 90 ? 'ok' : n >= 75 ? 'info' : n >= 50 ? 'warn' : 'bad'; }
  function ring(n) {
    const r = 62, c = 2 * Math.PI * r;
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('viewBox', '0 0 150 150');
    const off = c * (1 - Math.max(0, Math.min(100, n)) / 100);
    svg.innerHTML = '<circle class="ring-track" cx="75" cy="75" r="62" fill="none" stroke-width="12"/>' +
      '<circle class="ring-arc s-' + scoreClass(n) + '" cx="75" cy="75" r="62" fill="none" stroke-width="12" stroke-linecap="round" stroke-dasharray="' + c.toFixed(1) + '" stroke-dashoffset="' + off.toFixed(1) + '"/>';
    // n < 0: nothing to grade yet (the measurement reading before any measurement).
    return h('div', { class: 'ring' }, svg, h('div', { class: 'val' }, h('div', { class: 'num', text: n < 0 ? '—' : String(n) }), h('div', { class: 'of', text: n < 0 ? 'sem medição' : 'de 100' })));
  }

  const SEV = {
    critical: { ic: 'alert', label: 'Importante', badge: 'bad' },
    warning: { ic: 'alertc', label: 'Atenção', badge: 'warn' },
    info: { ic: 'info', label: 'Sugestão', badge: 'info' },
    good: { ic: 'check', label: 'Tudo certo', badge: 'ok' },
  };
  const REC = { 0: ['opcional', 'outline'], 1: ['recomendado', 'accent'], 2: ['não recomendado', 'warn'], 3: ['bloqueado neste PC', 'bad'] };
  function recBadge(level) {
    if (typeof level === 'string') {
      const map = { 'recomendado': 1, 'opcional': 0, 'não recomendado': 2, 'bloqueado': 3 };
      level = map[level] != null ? map[level] : 0;
    }
    const r = REC[level] || REC[0];
    return badge(r[0], r[1]);
  }
  function evBadge(e) { return badge('evidência ' + e, e === 'alta' ? 'ok' : e === 'média' ? 'info' : 'outline'); }
  function riskBadge(r) { return badge('risco ' + r, r === 'seguro' ? 'outline' : r === 'moderado' ? 'warn' : 'bad'); }
  function rebootBadge(r) { return r && r !== 'imediato' ? badge(r, r === 'reiniciar o PC' ? 'warn' : 'outline', 'refresh') : null; }

  function skeleton() {
    return h('div', { class: 'stack' }, h('div', { class: 'skel', style: { width: '28%', height: '26px' } }), h('div', { class: 'skel', style: { width: '52%' } }),
      h('div', { class: 'grid g2', style: { marginTop: '22px' } }, h('div', { class: 'card' }, h('div', { class: 'skel', style: { height: '150px' } })), h('div', { class: 'card' }, h('div', { class: 'skel', style: { height: '150px' } }))));
  }

  function focusPageTitle() {
    const t = $('#content .page-head h1');
    if (t) t.focus({ preventScroll: true });
  }

  // withMark puts the small official mark (48 px file, shown at 40 px) beside the title.
  function pageHead(title, desc, actions, withMark) {
    const h1 = h('h1', { text: title, tabindex: '-1' });
    const heading = withMark ? h('div', { class: 'page-title' }, h('img', { class: 'page-mark', src: 'logo-48.png', alt: '', width: '40', height: '40', decoding: 'async' }), h1) : h1;
    return h('div', { class: 'page-head' }, h('div', null, heading, desc ? h('p', { text: desc }) : null), actions && actions.length ? h('div', { class: 'actions' }, actions) : null);
  }

  function empty(ic, title, text, action) {
    return h('div', { class: 'empty' }, icon(ic), h('h3', { text: title }), h('p', { text }), action || null);
  }

  // ---------------------------------------------------------------------------
  // Jobs
  // ---------------------------------------------------------------------------

  const OUT = {
    applied: ['aplicado', 'ok'], already: ['já estava aplicado', 'outline'], na: ['não se aplica', 'outline'],
    blocked: ['bloqueado', 'bad'], failed: ['falhou', 'bad'], reverted: ['desfeito', 'ok'], conflict: ['conflito', 'warn'], nothing: ['nada a desfazer', 'outline'],
  };

  async function runJob(start, opts) {
    opts = opts || {};
    let job;
    try { job = await start; } catch (e) { toast(e.message, 'bad'); return null; }
    const fill = h('i');
    const bar = h('div', { class: 'progress' }, fill);
    const step = h('div', { class: 'small', text: 'Iniciando…' });
    const log = h('div', { class: 'log hidden' });
    const cancel = btn('Cancelar', async () => {
      cancel.disabled = true;
      try { await api('POST', '/api/jobs/' + job.id + '/cancel', {}); } catch (e) { /* ignore */ }
    }, 'ghost');
    const m = modal({
      title: opts.title || job.title, dismiss: false,
      body: h('div', null, step, h('div', { style: { marginTop: '12px' } }, bar),
        h('div', { style: { marginTop: '10px' } }, h('button', { class: 'linkbtn small', type: 'button', text: 'Ver detalhes técnicos', 'data-autofocus': true, onclick: () => log.classList.toggle('hidden') })), log),
      actions: [cancel],
    });
    S.busy = job; renderTopbar();
    for (;;) {
      await sleep(350);
      try { job = await api('GET', '/api/jobs/' + job.id); } catch (e) { continue; }
      fill.style.width = Math.round((job.progress || 0) * 100) + '%';
      step.textContent = job.step || '…';
      cancel.style.display = job.noCancel ? 'none' : '';
      cancel.title = job.noCancel || '';
      if (job.noCancel && step.textContent.indexOf(job.noCancel) < 0) step.textContent += ' ' + job.noCancel;
      if (job.log && job.log.length) { log.textContent = job.log.join('\n'); log.scrollTop = log.scrollHeight; }
      S.busy = job; renderTopbar();
      if (job.status !== 'running') break;
    }
    S.busy = null;
    invalidate();
    await refreshState();
    if (job.status === 'cancelled' && !(job.kind === 'apps-install' && job.result) && !(job.kind === 'trial' && job.result)) { m.close(); toast('Tarefa cancelada.', 'info'); rerender(); return job; }
    if (job.status === 'failed') {
      m.setTitle('Não foi possível concluir');
      m.setBody(h('div', null, h('div', { class: 'callout warn' }, icon('alert'), h('div', { text: job.error || 'Erro desconhecido.' })), (job.log || []).length ? h('div', { class: 'log' }, job.log.join('\n')) : null));
      m.allowDismiss();
      m.setActions([btn('Fechar', m.close, 'primary')], '.btn.primary');
      rerender();
      return job;
    }
    if (job.kind === 'apps-install' || job.kind === 'apps-upgrade') {
      const r = job.result || {};
      const cancelled = job.status === 'cancelled';
      const one = job.kind === 'apps-upgrade' ? (r.results || [])[0] : null; // upgrade --all is one winget run
      m.setTitle(job.kind === 'apps-install' ? (cancelled ? 'Instalação interrompida' : 'Instalação concluída') : (one && one.partial ? 'Atualização concluída com falhas' : 'Atualização concluída'));
      m.setBody(h('div', null,
        cancelled ? h('div', { class: 'callout', style: { marginBottom: '10px' } }, icon('info'), h('div', { text: 'Você cancelou: o instalador que estava rodando terminou e os seguintes não foram iniciados. Abaixo, o que foi feito até aqui.' })) : null,
        one ? h('div', { class: 'callout ' + (one.ok ? '' : 'warn') }, icon(one.ok ? 'check' : (one.partial ? 'alert' : 'alertc')), h('div', { text: one.outcome })) :
          h('div', { class: 'stats' }, [[r.ok || 0, 'concluído', 'concluídos'], [r.skipped || 0, 'já estava', 'já estavam'], [r.failed || 0, 'falhou', 'falharam']].map(([n, one, many]) => h('div', { class: 'stat' }, h('div', { class: 'n', text: String(n) }), h('div', { class: 'l', text: n === 1 ? one : many })))),
        r.reboot ? h('div', { class: 'callout warn', style: { marginTop: '10px' } }, icon('refresh'), h('div', { text: 'Algum instalador pediu para reiniciar o PC para concluir.' })) : null,
        one ? null : h('div', { class: 'findings', style: { marginTop: '12px' } }, (r.results || []).map(x => h('div', { class: 'finding compact' }, h('div', { class: 'sev ' + (x.ok ? (x.action === 'skipped' ? 'info' : 'good') : 'critical') }, icon(x.ok ? (x.action === 'skipped' ? 'info' : 'check') : 'alertc')),
          h('div', null, h('div', { class: 'ft', text: x.name }), h('div', { class: 'fx', text: ({ install: 'Instalação: ', upgrade: 'Atualização: ', skipped: '', 'upgrade-all': '' }[x.action] || '') + x.outcome }))))),
        (job.log || []).length ? h('details', { style: { marginTop: '12px' }, open: !!(one && !one.ok) }, h('summary', { class: 'small linkbtn', text: 'Ver o que o winget respondeu' }), h('div', { class: 'log', style: { marginTop: '8px' } }, job.log.join('\n'))) : null));
      m.allowDismiss();
      m.setActions([btn('Concluir', m.close, 'primary')], '.btn.primary');
    } else if (job.kind === 'repair') {
      const r = job.result || {};
      m.setTitle(({ clean: 'Nenhum arquivo danificado', fixed: 'Arquivos de sistema repostos', restart: 'Reinício necessário', unfixed: 'Ainda há arquivos danificados', failed: 'O reparo não terminou', unknown: 'Reparo terminado' })[r.verdict] || 'Reparo terminado');
      m.setBody(h('div', null, repairResult(r),
        (job.log || []).length ? h('details', { style: { marginTop: '12px' } }, h('summary', { class: 'small linkbtn', text: 'Ver detalhes técnicos' }), h('div', { class: 'log', style: { marginTop: '8px' } }, job.log.join('\n'))) : null));
      m.allowDismiss();
      m.setActions([btn('Concluir', m.close, 'primary')], '.btn.primary');
    } else if (job.kind === 'trial') {
      const r = job.result || {};
      const o = r.outcome || {};
      trialLast = r;
      m.setTitle(job.status === 'cancelled' ? 'Prova cancelada' : o.verdict === 'melhor' ? 'Melhorou no seu PC' : 'Sem melhora comprovada');
      m.setBody(trialResultView(r));
      m.allowDismiss();
      const acts = o.verdict === 'melhor' && o.action === 'mantido'
        ? [btn('Desfazer', () => { m.close(); undoTrial(o).then(() => rerender()); }, 'ghost', 'undo'), btn('Manter', m.close, 'primary', 'check')]
        : [btn('Ver histórico', () => { m.close(); go('historico'); }, 'ghost'), btn('Concluir', m.close, 'primary')];
      m.setActions(acts, '.btn.primary');
    } else if (job.kind === 'apply' || job.kind === 'revert') {
      m.setTitle(job.kind === 'apply' ? 'Otimizações aplicadas' : 'Alterações desfeitas');
      m.setBody(resultView(job));
      const conflicts = ((job.result && job.result.results) || []).filter(r => r.outcome === 'conflict').map(r => r.tweakId);
      const acts = [];
      if (conflicts.length) acts.push(btn('Forçar desfazer (' + conflicts.length + ')', () => { m.close(); runJob(api('POST', '/api/revert', { ids: conflicts, force: true }), { title: 'Desfazendo (forçado)' }); }, 'danger', 'undo'));
      acts.push(btn('Ver histórico', () => { m.close(); go('historico'); }, 'ghost'));
      if (job.kind === 'apply' && job.result && job.result.counts && job.result.counts.applied) acts.push(btn('Medir agora', () => { m.close(); go('desempenho'); }, 'ghost', 'gauge'));
      acts.push(btn('Concluir', m.close, 'primary'));
      m.allowDismiss();
      m.setActions(acts, '.btn.primary');
    } else {
      m.close();
      if (job.kind === 'restorepoint') toast('Ponto de restauração criado.', 'ok');
      const r = job.result || {};
      if (job.kind === 'drivers-scan') toast(r.searchError ? 'Busca concluída com aviso: ' + r.searchError : (r.offers ? plural(r.offers, 'atualização encontrada', 'atualizações encontradas') + ' no Windows Update.' : 'Nenhum driver novo no Windows Update.'), r.searchError ? 'info' : 'ok', 6000);
      if (job.kind === 'drivers-download') toast(r.failed ? plural(r.ok || 0, 'baixada', 'baixadas') + ', ' + plural(r.failed, 'falhou', 'falharam') + '. Veja o motivo na lista.' : plural(r.ok || 0, 'driver baixado', 'drivers baixados') + '. Agora escolha o que instalar.', r.failed ? 'info' : 'ok', 6000);
      if (job.kind === 'drivers-old-delete') {
        const gone = (r.removed || []).length, bad = r.failed || [];
        toast(plural(gone, 'pacote excluído', 'pacotes excluídos') + (bad.length ? '; não foi possível excluir ' + bad.join(', ') + '.' : '.'), bad.length ? 'bad' : 'ok', 6000);
      }
      if (job.kind === 'drivers-install') {
        const rpFail = r.restorePoint && r.restorePoint.startsWith('failed') ? ' O ponto de restauração não foi criado (' + r.restorePoint.replace(/^failed: /, '') + '): se um driver der problema, use "Reverter driver" no Gerenciador de Dispositivos.' : '';
        toast((r.failed ? plural(r.ok || 0, 'instalado', 'instalados') + ', ' + plural(r.failed, 'falhou', 'falharam') + '.' : plural(r.ok || 0, 'driver instalado', 'drivers instalados') + '.') + (r.reboot ? ' Reinicie o PC para concluir.' : '') + rpFail, r.failed || rpFail ? 'info' : 'ok', rpFail ? 12000 : 7000);
      }
      if (job.kind === 'retrim') toast(r.failed && r.failed.length ? 'Retrim feito em ' + (r.done || []).join(', ') + '; falhou em ' + r.failed.join('; ') + '.' : 'Retrim concluído em ' + (r.done || []).join(', ') + '.', r.failed && r.failed.length ? 'info' : 'ok', 7000);
      if (job.kind === 'perf' && r.summary) toast('Medição salva: ' + fmtN(r.summary.avgFps) + ' FPS médio, 1% low ' + fmtN(r.summary.low1Fps) + '.', 'ok', 6000);
      if (job.kind === 'cleanup') toast(BYTES(r.freed) + ' liberados' + (r.skipped ? '; ' + plural(r.skipped, 'arquivo em uso ficou', 'arquivos em uso ficaram') : '') + '.', 'ok', 6000);
      if (job.kind === 'gpu-download') toast('Driver baixado e com assinatura da NVIDIA verificada.', 'ok');
      if (job.kind === 'measure') toast(job.result && job.result.devices && job.result.devices.length ? 'Medição concluída.' : 'Nenhum movimento registrado. Mova o mouse durante o teste.', job.result && job.result.devices && job.result.devices.length ? 'ok' : 'info');
      if (job.kind === 'apps-refresh') toast(r.installed != null ? plural(r.installed, 'programa do catálogo instalado', 'programas do catálogo instalados') + (r.upgrades ? '; o winget tem ' + plural(r.upgrades, 'atualização disponível', 'atualizações disponíveis') + ' (do catálogo ou não)' : '') + '.' : 'Lista lida.', 'ok', 7000);
      if (job.kind === 'evaluate') {
        const sc = r.scores || {};
        const failed = (r.steps || []).filter(s => s.status === 'failed' || s.status === 'skipped');
        const scoreOf = r => r && r.overall >= 0 ? r.overall : '—';
        toast('Avaliação concluída: configuração ' + scoreOf(sc.config) + ', capacidade ' + scoreOf(sc.capacity) + ', medição ' + scoreOf(sc.measurement) + (sc.measurement && sc.measurement.overall < 0 ? ' (nada medido)' : '') + '.' + (failed.length ? ' ' + failed.map(s => s.title + ' ' + (s.status === 'failed' ? 'falhou' : 'pulado') + (s.note ? ' (' + s.note + ')' : '')).join('; ') + '.' : ''), failed.length ? 'info' : 'ok', 9000);
        if (currentPage() !== 'avaliacao' && !isSimple()) go('avaliacao');
      }
    }
    rerender();
    return job;
  }

  function resultView(job) {
    const r = job.result || {};
    const c = r.counts || {};
    const res = r.results || [];
    const isApply = job.kind === 'apply';
    // [count, singular label, plural label]
    const stats = isApply
      ? [[c.applied || 0, 'aplicada', 'aplicadas'], [c.already || 0, 'já estava', 'já estavam'], [(c.failed || 0) + (c.blocked || 0), 'falhou', 'falharam'], [c.na || 0, 'não se aplica', 'não se aplicam']]
      : [[c.reverted || 0, 'desfeita', 'desfeitas'], [c.conflict || 0, 'conflito', 'conflitos'], [c.failed || 0, 'falhou', 'falharam'], [c.nothing || 0, 'sem alteração', 'sem alteração']];
    const problems = res.filter(x => ['failed', 'blocked', 'conflict'].includes(x.outcome));
    const skipped = isApply ? (r.skipped || []) : [];
    const bad = (c.failed || 0) + (c.blocked || 0) + (c.conflict || 0);
    const head = isApply
      ? (bad ? ['warn', 'alert', 'Algumas otimizações não foram aplicadas'] : (c.applied || 0) ? ['ok', 'check', 'Otimizações aplicadas'] : ['ok', 'info', 'Nada novo para aplicar'])
      : (bad ? ['warn', 'alert', 'Algumas alterações não foram desfeitas'] : ['ok', 'check', 'Alterações desfeitas']);
    return h('div', null,
      h('div', { class: 'res-head ' + head[0] }, icon(head[1]), h('div', null, h('h4', { text: head[2] }),
        isApply ? h('div', { class: 'small muted', text: 'Pode ser desfeito a qualquer momento em Otimizar › Histórico.' }) : null)),
      h('div', { class: 'stats' }, stats.map(([n, one, many]) => h('div', { class: 'stat' + (n && (one === 'aplicada' || one === 'desfeita') ? ' ok' : n && (one === 'falhou' || one === 'conflito') ? ' bad' : ' neutral') }, h('div', { class: 'n', text: String(n) }), h('div', { class: 'l', text: n === 1 ? one : many })))),
      r.scoreBefore && r.scoreAfter && r.scoreBefore !== r.scoreAfter
        ? h('div', { class: 'score-delta' + (r.scoreAfter > r.scoreBefore ? ' up' : ' down') }, icon(r.scoreAfter > r.scoreBefore ? 'check' : 'info'),
          h('div', null, h('b', { text: 'Nota do diagnóstico: ' + r.scoreBefore + ' → ' + r.scoreAfter }),
            h('div', { class: 'small muted', text: (r.reread === false ? 'Recalculada com a análise anterior: a releitura do PC não terminou; use Reanalisar para confirmar.' : 'Recalculada com o PC lido de novo depois das mudanças.') + (r.reboot ? ' Itens que dependem de reiniciar contam como aplicados já agora.' : '') })))
        : null,
      r.reboot ? h('div', { class: 'callout warn' }, icon('refresh'), h('div', { text: 'Algumas mudanças só passam a valer depois de reiniciar o PC.' })) : null,
      isApply && r.restorePoint && r.restorePoint !== 'skipped'
        ? h('p', { class: 'small ' + (r.restorePoint === 'created' ? 'muted' : ''), text: r.restorePoint === 'created' ? 'Ponto de restauração do Windows criado antes das mudanças.' : 'Ponto de restauração não criado: ' + r.restorePoint.replace(/^failed: /, '') + '. O histórico do Axion continua permitindo desfazer cada mudança.' })
        : null,
      problems.length ? h('div', { style: { marginTop: '12px' } }, h('div', { class: 'small', style: { fontWeight: 600, marginBottom: '6px' }, text: 'Itens com problema' }),
        problems.map(p => h('div', { class: 'finding compact' }, h('div', { class: 'sev ' + (p.outcome === 'conflict' ? 'warning' : 'critical') }, icon('alertc')),
          h('div', null, h('div', { class: 'ft', text: p.title || p.tweakId }), h('div', { class: 'fx', text: (OUT[p.outcome] || [p.outcome])[0] + (p.message ? ': ' + p.message : '') }))))) : null,
      skipped.length ? h('div', { style: { marginTop: '12px' } }, h('div', { class: 'small', style: { fontWeight: 600, marginBottom: '6px' }, text: 'Ficaram de fora (desmarcados por você)' }),
        skipped.map(p => h('div', { class: 'finding compact' }, h('div', { class: 'sev info' }, icon('info')),
          h('div', null, h('div', { class: 'ft', text: p.title || p.id }), p.reason ? h('div', { class: 'fx', text: 'Por que foi sugerido: ' + p.reason }) : null))),
        h('p', { class: 'small muted', text: 'Não foram aplicados. Se mudar de ideia, estão em Otimizações (ou no mesmo botão de otimização).' })) : null,
      res.length ? h('details', { style: { marginTop: '12px' } }, h('summary', { class: 'small linkbtn', text: 'Ver todos os itens (' + res.length + ')' }),
        h('table', { class: 'table', style: { marginTop: '8px' } }, h('tbody', null, res.map(x => h('tr', null, h('td', { text: x.title || x.tweakId }), h('td', null, badge((OUT[x.outcome] || [x.outcome, ''])[0], (OUT[x.outcome] || ['', ''])[1]))))))) : null);
  }

  // ---------------------------------------------------------------------------
  // One-click plans and direct actions
  // ---------------------------------------------------------------------------

  let planOpening = false;
  async function openPlan(tier, category) {
    if (planOpening || MODALS.length) return; // a double click opens one plan only
    planOpening = true;
    let plan;
    try {
      plan = await api('GET', '/api/plan?tier=' + encodeURIComponent(tier) + (category ? '&category=' + encodeURIComponent(category) : ''));
    } catch (e) { toast(e.message, 'bad'); return; } finally { planOpening = false; }
    if (!plan.items || !plan.items.length) {
      const m = modal({ title: plan.title, body: h('div', { class: 'callout' }, icon('check'), h('div', { text: 'Nada pendente: tudo o que este modo aplicaria já está aplicado neste PC.' })), actions: [btn('OK', () => m.close(), 'primary')] });
      return;
    }
    const checks = new Map();
    const groups = [];
    for (const it of plan.items) {
      let g = groups.find(x => x.key === it.categoryTitle);
      if (!g) { g = { key: it.categoryTitle, items: [] }; groups.push(g); }
      g.items.push(it);
    }
    // Recommended first in each group, then the optional ones (unchecked).
    groups.forEach(g => g.items.sort((a, b) => (a.optional ? 1 : 0) - (b.optional ? 1 : 0)));
    const nOpt = plan.items.filter(it => it.optional).length;
    const label = h('span');
    const applyB = h('button', { class: 'btn primary', type: 'button' }, icon('zap'), label);
    const upd = () => {
      const n = [...checks.values()].filter(c => c.checked).length;
      label.textContent = 'Aplicar ' + plural(n, 'item', 'itens');
      applyB.disabled = n === 0;
    };
    const rp = h('input', { type: 'checkbox', checked: !!(S.state && S.state.settings && S.state.settings.restorePoint) });
    const body = h('div', null,
      h('p', { class: 'small', style: { marginTop: 0 }, text: plan.note }),
      h('div', { class: 'row small' }, h('button', { class: 'linkbtn', type: 'button', text: 'Marcar todos', onclick: () => { checks.forEach(c => { c.checked = true; }); upd(); } }), h('span', { class: 'faint', text: '·' }),
        h('button', { class: 'linkbtn', type: 'button', text: 'Desmarcar todos', onclick: () => { checks.forEach(c => { c.checked = false; }); upd(); } })),
      nOpt && plan.tier !== 'completo' ? h('p', { class: 'small muted', text: plural(nOpt, 'item opcional vem desmarcado', 'itens opcionais vêm desmarcados') + ': o benefício depende do seu uso. Marque só se fizer sentido para você.' }) : null,
      groups.map(g => h('div', { class: 'plan-group' }, h('h4', { text: g.key + ' (' + g.items.length + ')' }),
        g.items.map(it => {
          const cb = h('input', { type: 'checkbox', checked: it.checked !== false, 'aria-label': it.title });
          cb.addEventListener('change', upd);
          checks.set(it.id, cb);
          return h('label', { class: 'plan-item' + (it.optional ? ' optional' : '') }, cb, h('div', null,
            h('div', { class: 'pt' }, h('span', { text: it.title }), it.optional ? recBadge(it.rec) : evBadge(it.evidence), it.risk && it.risk !== 'seguro' ? riskBadge(it.risk) : rebootBadge(it.reboot)),
            it.summary ? h('div', { class: 'pr', text: it.summary }) : null,
            it.reason ? h('div', { class: 'pr muted', text: 'Para este PC: ' + it.reason }) : null));
        }))),
      plan.reboot ? h('div', { class: 'callout warn', style: { marginTop: '14px' } }, icon('refresh'), h('div', { text: 'Alguns itens só valem após reiniciar o PC.' })) : null);
    upd();
    const m = modal({
      title: plan.title, wide: true, body,
      actions: [h('label', { class: 'check small left' }, rp, 'Criar ponto de restauração antes'), btn('Cancelar', () => m.close(), 'ghost'), applyB],
    });
    applyB.addEventListener('click', async () => {
      const ids = [...checks.entries()].filter(([, c]) => c.checked).map(([id]) => id);
      // Recommended items left unchecked are reported back (optional ones
      // start unchecked: leaving them out is the default, not a choice).
      const skipped = plan.items.filter(it => !it.optional && checks.get(it.id) && !checks.get(it.id).checked).map(it => it.id);
      m.close();
      if (S.state && S.state.settings && rp.checked !== S.state.settings.restorePoint) {
        api('POST', '/api/settings', { restorePoint: rp.checked }).then(refreshState).catch(() => {});
      }
      await runJob(api('POST', '/api/apply', { ids, skipped, restorePoint: rp.checked, label: plan.title }), { title: plan.title });
    });
  }

  async function tweaksById() {
    const tw = await get('tweaks', '/api/tweaks');
    return new Map(tw.tweaks.map(t => [t.id, t]));
  }

  async function applyIds(ids, label) {
    const map = await tweaksById();
    const items = ids.map(id => map.get(id)).filter(Boolean);
    const pending = items.filter(t => t.state.status !== 'on');
    if (!pending.length) { toast('Já está aplicado.', 'ok'); return; }
    const blocked = pending.filter(t => t.state.rec.level === 3);
    const ok = await confirmDlg({
      title: label || 'Aplicar correção', ok: 'Aplicar', icon: 'zap',
      body: h('div', null, h('p', { style: { marginTop: 0 }, text: 'O Axion vai aplicar:' }),
        h('ul', { class: 'plan-mini' }, pending.map(t => h('li', null, icon('check'), h('div', null, h('b', { text: t.title }), ' — ', h('span', { class: 'muted', text: t.summary }))))),
        blocked.length ? h('div', { class: 'callout warn', style: { marginTop: '12px' } }, icon('alert'), h('div', { text: 'Bloqueado neste PC: ' + blocked.map(t => t.state.rec.reason).join('; ') })) : null,
        h('p', { class: 'small muted', text: 'Cada mudança guarda o valor anterior e pode ser desfeita no Histórico.' })),
    });
    if (!ok) return;
    await runJob(api('POST', '/api/apply', { ids: pending.map(t => t.id), allowBlocked: blocked.length > 0, label }), { title: label });
  }

  async function onToggle(t, want, input) {
    const s = t.state;
    if (want) {
      let allowBlocked = false;
      if (s.rec.level === 3 || s.rec.level === 2 || t.riskLevel === 3) {
        const ok = await confirmDlg({
          title: s.rec.level === 3 ? 'Bloqueado neste PC' : 'Tem certeza?', danger: s.rec.level === 3, ok: 'Aplicar mesmo assim',
          body: h('div', null, h('p', { style: { marginTop: 0 }, text: t.title }), h('div', { class: 'callout warn' }, icon('alert'), h('div', { text: s.rec.reason || t.riskNote || 'Este ajuste tem risco ou contraindicação.' }))),
        });
        if (!ok) { input.checked = false; return; }
        allowBlocked = s.rec.level === 3;
      }
      const job = await runJob(api('POST', '/api/apply', { ids: [t.id], allowBlocked }), { title: 'Aplicando: ' + t.title });
      if (!job) input.checked = false; // refused: the PC did not change
    } else {
      if (!s.byAxion) {
        input.checked = true;
        toast('Este item já estava assim no seu Windows: não há alteração do Axion para desfazer.', 'info');
        return;
      }
      const job = await runJob(api('POST', '/api/revert', { ids: [t.id] }), { title: 'Desfazendo: ' + t.title });
      if (!job) input.checked = true; // refused: the PC did not change
    }
  }

  // exportReport asks the format and whether to keep what identifies the
  // PC; the file is anonymous by default (0.7).
  async function exportReport() {
    const fmt = h('select', null, h('option', { value: 'html', text: 'Página (HTML) — legível, para mandar a alguém' }), h('option', { value: 'json', text: 'Dados (JSON) — completo, para análise técnica' }));
    const full = h('input', { type: 'checkbox' });
    const ok = await confirmDlg({
      title: 'Exportar relatório', ok: 'Exportar', icon: 'download',
      body: h('div', null,
        h('label', { class: 'small', style: { display: 'block', marginBottom: '6px' }, text: 'Formato' }), fmt,
        h('label', { class: 'check small', style: { marginTop: '14px' } }, full, 'Relatório completo (com o nome do PC, a conta do Windows, a conta da Steam, endereços de rede e identificadores de dispositivos USB e Bluetooth)'),
        h('p', { class: 'small muted', text: 'Sem a opção marcada, esses dados são substituídos por marcadores como <pc> e <ip>; o nome da cidade do relay da Valve mais próximo continua. O que o diagnóstico leu do hardware e do Windows fica inteiro.' })),
    });
    if (!ok) return;
    const json = fmt.value === 'json';
    try {
      const r = await fetch((json ? '/api/export' : '/api/export/html') + (full.checked ? '?full=1' : ''), { headers: { 'X-Axion-Token': TOKEN } });
      if (!r.ok) throw new Error('Erro ' + r.status);
      const blob = await r.blob();
      const url = URL.createObjectURL(blob);
      const d = new Date(), p2 = n => String(n).padStart(2, '0'); // local time, like the server's name
      const stamp = d.getFullYear() + p2(d.getMonth() + 1) + p2(d.getDate()) + '-' + p2(d.getHours()) + p2(d.getMinutes());
      const a = h('a', { href: url, download: 'axion-relatorio-' + stamp + (json ? '.json' : '.html') });
      document.body.appendChild(a);
      a.click();
      a.remove();
      setTimeout(() => URL.revokeObjectURL(url), 5000);
      toast(full.checked ? 'Relatório completo exportado.' : 'Relatório anônimo exportado.', 'ok');
    } catch (e) { toast('Não foi possível exportar: ' + e.message, 'bad'); }
  }

  async function rescan() {
    await runJob(api('POST', '/api/scan', {}), { title: 'Reanalisando o PC' });
  }

  // ---------------------------------------------------------------------------
  // Use profile and simple mode
  // ---------------------------------------------------------------------------

  const profiles = () => get('profiles', '/api/profiles');
  function profileOf(id, list) {
    return (list || []).find(p => p.id === id) || (list || [])[0] || { id, title: 'Jogo competitivo', scoreTitle: 'Saúde do PC para jogos' };
  }
  const isSimple = () => !!(S.state && S.state.settings && S.state.settings.simple);

  // pickProfile asks what the PC is used for. first: the one-time question
  // on the first run (no closing without an answer). Resolves true when a
  // profile was saved.
  async function pickProfile(first) {
    let data;
    try { data = await get('profiles', '/api/profiles', true); } catch (e) { toast(e.message, 'bad'); return false; }
    let chosen = data.current;
    return new Promise(resolve => {
      let done = false;
      const cards = new Map();
      const paint = () => cards.forEach((el, id) => { el.classList.toggle('active', id === chosen); el.setAttribute('aria-checked', id === chosen ? 'true' : 'false'); });
      const row = (k, v) => h('div', { class: 'pl' }, h('span', { class: 'k', text: k }), h('span', { text: v }));
      const list = h('div', { class: 'profiles', role: 'radiogroup', 'aria-label': 'Perfil de uso' }, data.profiles.map(p => {
        const el = h('button', { class: 'profile', type: 'button', role: 'radio', onclick: () => { chosen = p.id; paint(); } },
          h('div', { class: 'ph' }, h('span', { class: 'dot' }), h('div', null, h('div', { class: 'pt', text: p.title }), h('div', { class: 'ps', text: p.short }))),
          h('div', { class: 'pd' }, row('Na nota: ', p.scored), row('No 1 clique: ', p.plan), row('A avaliação mede: ', p.measures)));
        cards.set(p.id, el);
        return el;
      }));
      paint();
      const finish = v => { if (!done) { done = true; m.close(); resolve(v); } };
      const save = btn(first ? 'Continuar' : 'Salvar', async () => {
        save.disabled = true;
        try { await api('POST', '/api/settings', { profile: chosen }); } catch (e) { save.disabled = false; toast(e.message, 'bad'); return; }
        invalidate(); await refreshState();
        finish(true);
      }, 'primary hero', 'check');
      const m = modal({
        title: first ? 'Para que você usa este PC?' : 'Perfil de uso', wide: true, dismiss: !first,
        body: h('div', null,
          h('p', { class: 'small', style: { marginTop: 0 }, text: 'A nota, o que o 1 clique aplica e o que a avaliação completa mede seguem a resposta. O perfil pode ser trocado a qualquer momento; o histórico não muda e nenhuma otimização é aplicada só por trocar.' }),
          list,
          h('p', { class: 'small muted', style: { marginBottom: 0 }, text: 'Em notebook, em qualquer perfil, o Axion só recomenda ajustes que valem na tomada e nada que gaste bateria.' })),
        onClose: () => { if (!done) { done = true; resolve(false); } },
        actions: [first ? null : btn('Cancelar', () => finish(false), 'ghost'), save],
      });
    });
  }

  async function setSimple(on) {
    try { await api('POST', '/api/settings', { simple: on }); } catch (e) { toast(e.message, 'bad'); return; }
    await refreshState();
    rerender(true);
  }

  // solveSafe applies the recommended plan from the simple mode after one
  // confirmation listing the items (the full plan dialog is the complete mode's).
  async function solveSafe() {
    let plan;
    try { plan = await api('GET', '/api/plan?tier=recomendado'); } catch (e) { toast(e.message, 'bad'); return; }
    const items = (plan.items || []).filter(it => !it.optional);
    if (!items.length) { toast('Nada pendente: o que é seguro e recomendado já está aplicado.', 'ok'); return; }
    const rp = h('input', { type: 'checkbox', checked: !!(S.state && S.state.settings && S.state.settings.restorePoint) });
    const ok = await confirmDlg({
      title: 'Resolver o que é seguro', ok: 'Aplicar ' + plural(items.length, 'item', 'itens'), icon: 'zap', wide: true,
      body: h('div', null,
        h('p', { class: 'small', style: { marginTop: 0 }, text: plan.note }),
        h('ul', { class: 'plan-mini' }, items.map(it => h('li', null, icon('check'), h('div', null, h('b', { text: it.title }), ' — ', h('span', { class: 'muted', text: it.summary }))))),
        plan.reboot ? h('div', { class: 'callout warn', style: { marginTop: '12px' } }, icon('refresh'), h('div', { text: 'Alguns itens só valem após reiniciar o PC.' })) : null,
        h('label', { class: 'check small', style: { marginTop: '12px' } }, rp, 'Criar ponto de restauração do Windows antes'),
        h('p', { class: 'small muted', text: 'Cada mudança guarda o valor anterior e pode ser desfeita no Histórico (modo completo).' })),
    });
    if (!ok) return;
    await runJob(api('POST', '/api/apply', { ids: items.map(it => it.id), restorePoint: rp.checked, label: 'Resolver o que é seguro' }), { title: 'Resolver o que é seguro' });
  }

  const SEV_RANK = { critical: 0, warning: 1, info: 2, good: 3 };
  // activeFindings: the ones that count in the score, heaviest first.
  function activeFindings(rep) {
    return rep.findings.filter(x => !x.dismissed && !x.offProfile).slice().sort((a, b) => (SEV_RANK[a.severity] || 0) - (SEV_RANK[b.severity] || 0));
  }

  async function pageSimple() {
    const [rep, pr] = await Promise.all([get('report', '/api/report'), profiles()]);
    const st = S.state;
    const prof = profileOf(rep.profile || st.settings.profile, pr.profiles);
    const top = activeFindings(rep).filter(x => x.severity !== 'good').slice(0, 3);
    const n = st.pendingRecommended || 0;
    return h('div', { class: 'page simple' },
      pageHead(prof.scoreTitle, 'Perfil ' + prof.title + ' · analisado ' + ago(st.lastScan) + '. Modo simples: a nota, o que mais pesa nela e um botão para o que é seguro.',
        [btn('Trocar perfil', () => pickProfile(false), 'ghost', 'sliders'), btn('Modo completo', () => setSimple(false), 'ghost', 'layout', { title: 'Todas as páginas: diagnóstico, otimizações, CS2, medições, histórico' })]),
      h('div', { class: 'grid g-hero' },
        h('div', { class: 'card' }, h('h2', null, icon('gauge'), prof.scoreTitle),
          h('div', { class: 'score' }, ring(rep.overall), h('div', null,
            h('div', { class: 'grade ' + scoreClass(rep.overall), text: rep.grade }),
            h('div', { class: 'muted', style: { margin: '4px 0 12px' }, text: rep.summary }))),
          h('div', { class: 'faint tiny', style: { marginTop: '14px' }, text: rep.scoreNote })),
        h('div', { class: 'card' }, h('h2', null, icon('zap'), 'Resolver o que é seguro'),
          h('div', { class: 'sub', text: n ? plural(n, 'ajuste recomendado para o seu PC: sem risco, com evidência e reversível.', 'ajustes recomendados para o seu PC: sem risco, com evidência e reversíveis.') : 'Tudo o que é seguro e recomendado para o seu PC já está aplicado.' }),
          h('div', { class: 'row wrap' }, btn('Resolver o que é seguro', solveSafe, 'primary hero', 'zap', { disabled: !n }), btn('Reanalisar', rescan, '', 'refresh'), btn('Exportar relatório', exportReport, '', 'download')),
          st.rebootPending ? h('div', { class: 'callout warn', style: { marginTop: '14px' } }, icon('refresh'), h('div', { text: 'Há alterações que só passam a valer depois de reiniciar o PC.' })) : null)),
      h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('alert'), 'O que mais pesa na nota'),
        top.length ? h('div', { class: 'findings' }, top.map(x => findingCard(x, false))) : h('div', { class: 'callout' }, icon('check'), h('div', { text: 'Nenhum problema encontrado para o seu perfil.' })),
        h('div', { class: 'small muted', style: { marginTop: '12px' }, text: 'O modo completo mostra todos os pontos, as medições (FPS, rede, periféricos) e o histórico de cada mudança.' })));
  }

  // ---------------------------------------------------------------------------
  // Chrome: sidebar, top bar, banners
  // ---------------------------------------------------------------------------

  // Seven entries instead of twenty-five: pages that belong together share one
  // entry and a tab bar. The routes (and so every link, button and action that
  // goes to a page) are unchanged. `sec` starts a labelled section of the menu.
  const NAV = [
    { id: 'painel', title: 'Início', ic: 'grid', pages: ['painel'], sec: 'Principal' },
    { id: 'otimizacoes', title: 'Otimizar', ic: 'sliders', pages: ['otimizacoes', 'prova', 'historico'] },
    { id: 'diagnostico', title: 'Diagnóstico', ic: 'pulse', pages: ['diagnostico', 'avaliacao', 'saude'] },
    { id: 'jogos', title: 'Jogos', ic: 'gamepad', pages: ['jogos', 'cs2', 'partidas', 'obs'], sec: 'Jogos e medição' },
    { id: 'medir', title: 'Medir', ic: 'gauge', pages: ['desempenho', 'latencia', 'dpc', 'rede', 'perifericos'] },
    { id: 'manutencao', title: 'Manutenção', ic: 'wrench', pages: ['limpeza', 'drivers', 'inicializacao', 'processos', 'programas', 'apps', 'bios'], sec: 'Sistema' },
    { id: 'sobre', title: 'Sobre', ic: 'info', pages: ['sobre', 'excluidos'] },
  ];
  const TITLES = {
    painel: 'Início', otimizacoes: 'Otimizações', prova: 'Prova no seu PC', historico: 'Histórico', diagnostico: 'Diagnóstico', avaliacao: 'Avaliação completa',
    cs2: 'Counter-Strike 2', jogos: 'Biblioteca de jogos', desempenho: 'Desempenho (FPS)', partidas: 'Relatório da partida', latencia: 'Atraso de entrada', dpc: 'Latência do sistema', obs: 'Live (OBS)', rede: 'Rede e ping',
    saude: 'Saúde do Windows', limpeza: 'Limpeza', drivers: 'Drivers', inicializacao: 'Inicialização', processos: 'Processos', programas: 'Programas', apps: 'Instalar programas',
    perifericos: 'Periféricos', bios: 'Guia de BIOS', sobre: 'Sobre e metodologia', excluidos: 'O que não fazemos',
  };
  const groupOf = page => NAV.find(g => g.pages.includes(page)) || NAV[0];
  // The tab last open in each entry: clicking the entry again returns to it.
  const lastSub = {};

  function currentPage() {
    const m = location.hash.match(/^#\/([a-z0-9]+)/);
    return m && ROUTES[m[1]] ? m[1] : 'painel';
  }

  function renderSidebar() {
    const st = S.state || {};
    const page = currentPage();
    const cur = groupOf(page);
    lastSub[cur.id] = page;
    const counts = {
      diagnostico: (st.critical || 0) + (st.warning || 0),
      otimizacoes: st.pendingRecommended || 0,
    };
    const sb = $('#sidebar');
    sb.replaceChildren();
    const items = [];
    NAV.forEach(g => {
      if (g.sec) items.push(h('div', { class: 'nav-sep', 'aria-hidden': 'true', text: g.sec }));
      items.push(h('button', { class: 'nav-item' + (g === cur ? ' active' : ''), type: 'button', onclick: () => go(lastSub[g.id] || g.pages[0]), 'aria-current': g === cur ? 'page' : null, 'data-focus': 'nav-' + g.id, title: g.title, 'aria-label': g.title + (counts[g.id] ? ' (' + counts[g.id] + ')' : '') },
        icon(g.ic), h('span', { text: g.title }), counts[g.id] ? h('span', { class: 'nav-count' + (g.id === 'diagnostico' && st.critical ? ' warn' : g.id === 'diagnostico' ? ' mid' : ''), text: String(counts[g.id]) }) : null));
    });
    const adm = st.mock ? h('div', null, badge('Demonstração', 'info'))
      : h('div', { class: 'adm ' + (st.elevated ? 'ok' : 'no'), title: st.elevated ? 'Executando como administrador' : 'Sem privilégios de administrador' },
        icon('shield'), h('span', { text: st.elevated ? 'Administrador' : 'Sem administrador' }));
    add(sb, [
      h('div', { class: 'brand' }, logo(), h('div', { class: 'brand-text' }, h('div', { class: 'brand-name', text: 'AXION' }), h('div', { class: 'brand-sub', text: 'Game Optimizer' }))),
      h('nav', { class: 'nav-group', 'aria-label': 'Seções' }, items),
      h('div', { class: 'sidebar-foot' }, h('b', { text: /^\d/.test(st.version || '') ? 'v' + st.version : (st.version || '') }), adm)]);
  }

  function renderTopbar() {
    const st = S.state || {};
    const busy = S.busy || st.job;
    const live = st.session;
    const pill = busy
      ? h('span', { class: 'pill busy' }, h('span', { class: 'dot' }), (busy.title || 'Trabalhando') + (busy.progress ? ' · ' + Math.round(busy.progress * 100) + '%' : ''))
      : live
        ? h('span', { class: 'pill live', title: 'Gravando a partida para o relatório automático. Pode minimizar o Axion; não feche.' }, h('span', { class: 'dot' }), 'Gravando a partida: ' + live.name)
        : h('span', { class: 'pill' }, h('span', { class: 'dot' }), st.scanned ? 'Analisado ' + ago(st.lastScan) : 'Pronto');
    const tb = $('#topbar');
    tb.replaceChildren();
    const page = currentPage();
    const g = groupOf(page);
    add(tb, [
      isSimple() && st.scanned
        ? h('div', { class: 'crumb' }, h('b', { text: 'Axion · modo simples' }))
        : h('div', { class: 'crumb' }, g.pages.length > 1 ? [h('span', { text: g.title }), icon('chevron')] : null, h('b', { text: TITLES[page] || g.title })),
      st.mock ? badge('Demonstração', 'info') : null,
      h('div', { class: 'spacer' }),
      st.scanned && !isSimple() ? h('button', { class: 'btn ghost cmdk-btn', type: 'button', onclick: openPalette, title: 'Buscar uma página ou ação (Ctrl+K)', 'aria-label': 'Buscar ou ir para (Ctrl+K)', 'data-focus': 'top-cmdk' }, icon('search'), h('span', { class: 'cmdk-txt', text: 'Buscar ou ir para…' }), h('span', { class: 'kbd', text: 'Ctrl K' })) : null,
      pill,
      st.scanned ? btn('Reanalisar', rescan, 'sm', 'refresh', { disabled: !!busy, 'data-focus': 'top-rescan' }) : null]);
  }

  function renderBanners() {
    const st = S.state || {};
    const out = [];
    if (st.scanned && !st.elevated && !st.mock) out.push(h('div', { class: 'banner' }, icon('alert'), h('div', { text: 'O Axion não está como administrador: a análise funciona, mas as otimizações não podem ser aplicadas. Abra o programa novamente e aceite o pedido de permissão.' })));
    if (st.rebootPending) out.push(h('div', { class: 'banner' }, icon('refresh'), h('div', { text: 'Há alterações que só passam a valer depois de reiniciar o PC.' })));
    if (st.drifted) out.push(h('div', { class: 'banner' }, icon('alert'), h('div', { text: plural(st.drifted, 'otimização foi desfeita', 'otimizações foram desfeitas') + ' pelo Windows ou por outro programa.' }), h('div', { class: 'spacer' }), btn('Ver', () => go('diagnostico'), 'sm', null, { 'data-focus': 'banner-drift' })));
    // The simple mode ignores the route, so the button would do nothing there.
    const un = st.unseen || [];
    if (un.length && currentPage() !== 'partidas' && !isSimple()) out.push(h('div', { class: 'banner game' }, icon('gauge'),
      h('div', { text: un.length === 1 ? 'Relatório da partida de ' + un[0].name + ' pronto.' : un.length + ' relatórios de partida novos.' }),
      h('div', { class: 'spacer' }), btn('Ver', () => { sessOpen = un[0].id; go('partidas'); }, 'sm', null, { 'data-focus': 'banner-sess' })));
    const bn = $('#banners');
    bn.replaceChildren();
    add(bn, out);
  }

  // The chrome is rebuilt on every state refresh: keep the keyboard focus on
  // the same control (nav item, Reanalisar) instead of dropping it.
  function renderChrome() {
    const act = document.activeElement;
    const key = act && act.dataset && act.closest && act.closest('#sidebar, #topbar, #banners') ? act.dataset.focus : null;
    renderSidebar(); renderTopbar(); renderBanners();
    if (key) { const n = document.querySelector('#sidebar [data-focus="' + key + '"], #topbar [data-focus="' + key + '"], #banners [data-focus="' + key + '"]'); if (n) n.focus(); }
  }

  // ---------------------------------------------------------------------------
  // Shared renderers
  // ---------------------------------------------------------------------------

  function actionButton(a) {
    if (a.kind === 'tweaks') return btn(a.label, () => applyIds(a.tweaks || [], a.label), 'sm', 'zap');
    // An execution ID (the DNS of the adapter, offered by the network test) is
    // not in /api/tweaks, so applyIds would say "Já está aplicado": it has its
    // own confirmation.
    if (a.kind === 'apply') return btn(a.label, async () => {
      const ok = await confirmDlg({ title: a.label, ok: 'Aplicar', icon: 'zap',
        body: h('div', null, h('p', { style: { marginTop: 0 }, text: a.confirm || '' }),
          h('p', { class: 'small muted', text: 'A mudança fica no Histórico e pode ser desfeita.' })) });
      if (!ok) return;
      await runJob(api('POST', '/api/apply', { ids: a.tweaks || [], label: a.label }), { title: a.label });
    }, 'sm', 'zap');
    if (a.kind === 'page') return ROUTES[a.page] ? btn(a.label, () => { if (a.category && a.page === 'otimizacoes') { optCat = a.category; optQuery = ''; optFilter = 'all'; } go(a.page); }, 'sm', 'chevron') : null;
    if (a.kind === 'url') return btn(a.label, () => openURL(a.url), 'sm', 'ext');
    return null;
  }

  async function dismissFinding(x, dismiss) {
    try {
      await api('POST', '/api/findings/dismiss', { id: x.id, dismiss });
      invalidate();
      await refreshState();
      rerender();
      toast(dismiss ? 'Aviso dispensado: ele fica em "Dispensados" e não conta mais na nota.' : 'Aviso restaurado.', 'ok');
    } catch (e) { toast(e.message, 'bad'); }
  }

  function findingCard(x, compact, opts) {
    const sev = SEV[x.severity] || SEV.info;
    const acts = (x.actions || []).map(actionButton).filter(Boolean);
    // Only a critical finding gets a filled first action.
    if (x.severity === 'critical' && acts[0] && acts[0].classList) acts[0].classList.add('primary');
    if (opts && opts.dismiss && (x.severity === 'warning' || x.severity === 'info')) {
      acts.push(x.dismissed
        ? btn('Restaurar aviso', () => dismissFinding(x, false), 'ghost sm')
        : btn('Dispensar', () => dismissFinding(x, true), 'ghost sm', null, { title: 'Parar de avisar sobre isto (ex.: já tentou e não dá no seu PC). Pode restaurar depois.' }));
    }
    const src = compact ? [] : (x.sources || []).map(s => extLink(s.title, s.url));
    const why = x.explanation
      ? h('details', { class: 'why', open: !compact && x.severity === 'critical' }, h('summary', null, icon('chevron'), 'Por que importa'), h('div', { class: 'fx', text: x.explanation }))
      : null;
    return h('div', { class: 'finding ' + x.severity + (compact ? ' compact' : '') },
      h('div', { class: 'sev ' + x.severity, title: sev.label }, icon(sev.ic)),
      h('div', null, h('div', { class: 'ft' }, compact ? null : badge(sev.label, sev.badge), x.title),
        x.evidence ? h('div', { class: 'fe', text: x.evidence }) : null,
        why,
        acts.length || src.length ? h('div', { class: 'fa' }, acts, src) : null));
  }

  // Where an item stands on this PC, in one plain word (replaces the stack of
  // recommendation / evidence / risk / reboot badges on every row).
  function tweakBucket(t) {
    const s = t.state || { applicable: false, rec: { level: 0 } };
    if (!s.applicable) return 'na';
    if (s.status === 'on') return 'on';
    const lvl = (s.rec && s.rec.level) || 0;
    if (lvl === 3) return 'blocked';
    if (lvl === 2) return 'no';
    // The server says what the recommended plan applies for the use profile;
    // the menu counter and the plan follow the same rule.
    if (t.recommended === true || (t.recommended === undefined && lvl === 1 && t.oneClick)) return 'rec';
    return 'opt';
  }
  const BUCKET = {
    on: { label: 'Ativo', kind: 'ok', ic: 'check' },
    rec: { label: 'Recomendado', kind: 'accent', ic: 'zap' },
    opt: { label: 'Opcional', kind: 'outline', ic: null },
    no: { label: 'Não indicado', kind: 'warn', ic: null },
    blocked: { label: 'Bloqueado neste PC', kind: 'bad', ic: 'alertc' },
    na: { label: 'Não se aplica', kind: 'outline', ic: null },
  };

  function tweakRow(t, opts) {
    opts = opts || {};
    const s = t.state || { rec: { level: 0 } };
    const on = s.status === 'on';
    const bucket = tweakBucket(t);
    const canToggle = s.applicable && !(on && !s.byAxion);
    const tip = !s.applicable ? (s.naReason || 'Não se aplica a este PC') : (on && !s.byAxion ? 'Já está assim no seu Windows' : (on ? 'Desfazer' : 'Aplicar'));
    const tg = toggle(on, (want, input) => onToggle(t, want, input), !canToggle, tip, canToggle ? (on ? 'Desfazer: ' : 'Aplicar: ') + t.title : t.title + ' — ' + tip);
    const b = BUCKET[bucket];
    const status = badge(bucket === 'on' && s.applicable && !s.byAxion ? 'Já ativo no Windows' : b.label, b.kind, b.ic);
    // Only the most urgent note shows on the row (in this order); the rest is in the details.
    const notes = [];
    if (s.pendingUndo) notes.push(badge('desfazer pendente', 'warn', 'alert'));
    if (s.drifted) notes.push(badge('desfeito depois', 'warn', 'alert'));
    if (s.status === 'partial') notes.push(badge('parcial', 'warn'));
    if (on && s.effect === 'ignored') notes.push(badge('não está valendo', 'warn', 'alert'));
    if (on && s.effect === 'reboot') notes.push(badge('vale após reiniciar', 'warn', 'refresh'));
    if (!on && s.applicable) {
      if (t.reboot && t.reboot !== 'imediato') notes.push(badge('Pede ' + t.reboot, t.reboot === 'reiniciar o PC' ? 'warn' : 'outline', 'refresh'));
      if (t.risk !== 'seguro') notes.push(riskBadge(t.risk));
    }
    // The reason this item is (or is not) for this PC, visible without opening anything.
    const why = !s.applicable ? s.naReason : bucket === 'opt' && t.profileNote ? t.profileNote : (bucket !== 'on' && s.rec && s.rec.reason ? s.rec.reason : '');
    const ops = (s.ops || []).map(o => h('li', { class: o.inTarget ? 'in' : 'out' }, o.describe, h('div', { class: 'cur', text: 'atual: ' + (o.current || '—') + (o.error ? ' · erro: ' + o.error : '') })));
    const rows = [
      ['Como funciona', h('div', { text: t.detail })],
      ['Confiança', h('div', { class: 'row wrap' }, evBadge(t.evidence), riskBadge(t.risk), rebootBadge(t.reboot) || badge('vale na hora', 'outline'))],
    ];
    if (notes.length > 1) rows.push(['Também', h('div', { class: 'row wrap' }, notes.slice(1))]);
    if (t.trialOk) rows.push(['Prova no seu PC', h('div', null,
      h('div', { class: 'small muted', text: 'Meça este ajuste no seu jogo: o Axion grava antes e depois e desfaz sozinho se não houver melhora comprovada.' }),
      h('div', { class: 'mt-2' }, btn('Provar no meu PC', () => { trialForm.tweak = t.id; go('prova'); }, 'sm', 'gauge')))]);
    if (on && s.effectNote) rows.push([s.effect === 'reboot' ? 'Falta reiniciar' : 'Por que não está valendo', h('div', { text: s.effectNote })]);
    if (t.riskNote) rows.push(['Risco', h('div', { text: t.riskNote })]);
    if (ops.length) rows.push(['O que é alterado', h('ul', { class: 'ops' }, ops)]);
    if (t.sources && t.sources.length) rows.push(['Fontes', h('div', { class: 'src' }, t.sources.map(x => extLink(x.title, x.url)))]);
    const detail = h('details', null, h('summary', null, icon('chevron'), 'Detalhes e fontes'),
      h('div', { class: 'detail-box' }, h('dl', { class: 'kv' }, rows.map(([k, v]) => [h('dt', { text: k }), h('dd', null, v)]))));
    return h('div', { class: 'tweak st-' + bucket + (on ? ' is-on' : '') },
      h('div', { class: 'tcheck' }, tg),
      h('div', { class: 'tmain' }, h('div', { class: 'tt', text: t.title }), h('div', { class: 'ts', text: t.summary }),
        why ? h('div', { class: 'tw one', title: why }, h('b', { text: s.applicable ? 'Para o seu PC: ' : 'Não se aplica: ' }), why) : null,
        t.trial ? h('div', { class: 'tw' }, h('b', { text: 'Medido no seu PC (' + fmtDT(t.trial.at) + '): ' }), trialBadge(t.trial), ' ', trialLine(t.trial)) : null,
        opts.noDetail ? null : detail),
      h('div', { class: 'tstat' }, status, notes[0] || null));
  }

  function checkItem(c, extra) {
    const stIcon = { ok: 'check', action: 'alert', info: 'info', unknown: 'info' }[c.status] || 'info';
    const stText = { ok: 'ok', action: 'ajustar', info: 'info', unknown: 'não detectado' }[c.status] || c.status;
    const stKind = { ok: 'ok', action: 'warn', info: 'info', unknown: 'outline' }[c.status] || 'outline';
    return h('div', { class: 'check-item' },
      h('div', { class: 'st ' + c.status }, icon(stIcon)),
      h('div', null,
        h('div', { class: 'ct' }, c.title, badge(stText, stKind), c.evidence && c.evidence !== '—' ? evBadge(c.evidence) : null),
        c.current ? h('div', { class: 'cc' }, h('span', { class: 'muted', text: 'Atual: ' }), c.current) : null,
        c.advice || c.why ? h('div', { class: 'cc', text: c.advice || c.why }) : null,
        c.where || c.path ? h('div', { class: 'cw', text: c.where || c.path }) : null,
        c.basis ? h('div', { class: 'cb', text: 'Base: ' + c.basis }) : null,
        c.caution ? h('div', { class: 'ccaut', text: '⚠ ' + c.caution }) : null,
        c.open ? h('div', { style: { marginTop: '8px' } }, btn('Abrir Configurações do Windows', () => openURL(c.open), 'sm', 'ext')) : null,
        c.sources && c.sources.length ? h('div', { class: 'row wrap', style: { marginTop: '6px' } }, c.sources.map(s => extLink(s.title, s.url))) : null,
        extra || null));
  }

  function hwTile(ic, k, v, s) {
    return h('div', { class: 'card tight' }, h('div', { class: 'hw' }, h('div', { class: 'ic' }, icon(ic)),
      h('div', { style: { minWidth: 0 } }, h('div', { class: 'k', text: k }), h('div', { class: 'v clamp2', text: v || '—', title: v || '' }), s ? h('div', { class: 's', text: s }) : null)));
  }

  // ---------------------------------------------------------------------------
  // Pages
  // ---------------------------------------------------------------------------

  async function pagePainel() {
    const [rep, f, pr, tw, gd, pf] = await Promise.all([get('report', '/api/report'), get('facts', '/api/facts'), profiles(), get('tweaks', '/api/tweaks'),
      get('games', '/api/games').catch(() => null), get('perf', '/api/perf').catch(() => null)]);
    const st = S.state;
    const prof = profileOf(rep.profile || st.settings.profile, pr.profiles);
    const attention = activeFindings(rep).filter(x => x.severity === 'critical' || x.severity === 'warning').slice(0, 4);
    const gpu = (f.gpus || []).find(g => !g.integrated) || (f.gpus || [])[0] || {};
    const sysDisk = (f.disks || []).find(d => d.isSystem) || (f.disks || [])[0] || {};
    const vol = (f.volumes || []).find(v => (v.letter || '').toUpperCase().startsWith((f.os.systemDrive || 'C:').toUpperCase())) || {};
    const disp = (f.displays || [])[0] || {};
    const act = (f.adapters || []).find(a => a.up && a.hasGateway) || {};
    const sec = f.security || {};
    // Where each item stands on this PC (the same buckets as the Otimizações page).
    const B = { on: [], rec: [], opt: [], no: [], blocked: [], na: [] };
    tw.tweaks.forEach(t => B[tweakBucket(t)].push(t));
    const nRec = B.rec.length;
    const rp = h('input', { type: 'checkbox', checked: !!st.settings.restorePoint });
    rp.addEventListener('change', async () => {
      try { await api('POST', '/api/settings', { restorePoint: rp.checked }); await refreshState(); } catch (e) { rp.checked = !rp.checked; toast('Não foi possível salvar: ' + e.message, 'bad'); }
    });
    const tier = (t, d, ic, fn, kind) => h('button', { class: 'tier' + (kind ? ' ' + kind : ''), type: 'button', onclick: fn },
      h('div', { class: 'ic' }, icon(ic)), h('div', null, h('div', { class: 't', text: t }), h('div', { class: 'd', text: d })), h('div', { class: 'go' }, icon('chevron')));
    const stat = (n, label, sub, kind, filter) => h('button', { class: 'stat click ' + kind, type: 'button', onclick: () => openOptimizations(filter), title: 'Ver: ' + label.toLowerCase() },
      h('div', { class: 'n', text: String(n) }), h('div', null, h('div', { class: 'l', text: label }), h('div', { class: 's', text: sub })));
    const offN = rep.findings.filter(x => x.offProfile).length;
    const nReboot = B.rec.filter(t => t.reboot === 'reiniciar o PC').length;
    const okIcon = icon('check');
    okIcon.classList.add('ok-ic');
    const next = nRec
      ? h('div', { class: 'card hero-next signature' },
        h('div', { class: 'eyebrow', text: 'Próximo passo' }),
        h('div', { class: 'big', text: plural(nRec, 'melhoria recomendada', 'melhorias recomendadas') + ' para o seu PC' }),
        h('div', { class: 'small muted', style: { marginBottom: '8px' }, text: (nReboot ? plural(nReboot, 'pede reiniciar o PC', 'pedem reiniciar o PC') + ' · ' : '') + 'tudo pode ser desfeito' }),
        h('p', { class: 'muted', text: 'Só itens com evidência para o seu hardware, sem risco e que podem ser desfeitos. Você revisa a lista antes de aplicar.' }),
        h('div', { class: 'row wrap' }, btn(nRec === 1 ? 'Aplicar a recomendada' : 'Aplicar as ' + nRec + ' recomendadas', () => openPlan('recomendado'), 'primary hero', 'zap'), btn('Ver cada uma', () => openOptimizations('rec'))),
        h('label', { class: 'check small muted mt-4' }, rp, 'Criar ponto de restauração do Windows antes de aplicar'))
      : h('div', { class: 'card hero-next done signature' },
        h('div', { class: 'eyebrow', text: 'Próximo passo' }),
        okIcon,
        h('div', { class: 'big', text: 'Tudo certo por aqui' }),
        h('p', { class: 'muted', text: B.on.length ? 'Tudo o que o Axion recomenda para o seu PC já está ativo. Veja os opcionais ou refaça a análise depois de mudar o hardware ou o Windows.' : 'O Axion não encontrou nada a recomendar agora.' }),
        h('div', { class: 'row wrap' }, btn('Reanalisar o PC', rescan, 'primary hero', 'refresh'), B.opt.length ? btn('Ver opcionais (' + B.opt.length + ')', () => openOptimizations('opt')) : null));
    const listTitle = nRec ? 'Recomendadas para o seu PC' : 'Ativas no seu PC';
    const listItems = (nRec ? B.rec : B.on).slice(0, 6);
    const listAll = nRec ? B.rec.length : B.on.length;

    // The cycle: analyse, apply, measure.
    const nRuns = pf && pf.runs ? pf.runs.length : 0;
    const steps = [
      { done: !!st.scanned, title: 'Analisar', sub: st.scanned ? 'feito ' + ago(st.lastScan) : 'ainda não feito', act: btn('Reanalisar', rescan, 'sm ghost') },
      { done: nRec === 0, title: 'Aplicar recomendadas', sub: nRec ? plural(nRec, 'pendente', 'pendentes') : 'tudo aplicado', act: nRec ? btn('Aplicar', () => openPlan('recomendado'), 'sm') : null },
      { done: nRuns > 0, title: 'Medir no seu jogo', sub: nRuns ? plural(nRuns, 'medição salva', 'medições salvas') : 'compare antes e depois', act: btn(nRuns ? 'Medir de novo' : 'Medir FPS', () => go('desempenho'), 'sm ghost') },
    ];
    const firstOpen = steps.findIndex(x => !x.done);
    const cycle = h('div', { class: 'cycle', role: 'list', 'aria-label': 'Seu ciclo: analisar, aplicar, medir' }, steps.map((x, i) =>
      h('div', { class: 'cycle-step' + (x.done ? ' done' : i === firstOpen ? ' now' : ''), role: 'listitem' },
        h('div', { class: 'cn' }, x.done ? icon('check') : String(i + 1)),
        h('div', null, h('div', { class: 'ct', text: x.title }), h('div', { class: 'cs', text: x.sub })), x.act)));

    // The machine, in one line of chips; a click opens "Seu PC em detalhes".
    const openHw = () => { const d = document.getElementById('hw-details'); if (d) { d.open = true; d.scrollIntoView({ behavior: 'smooth', block: 'start' }); } };
    const chip = (ic, text) => text ? h('button', { class: 'hw-chip', type: 'button', title: text, onclick: openHw }, icon(ic), h('span', { text })) : null;
    const hwStrip = h('div', { class: 'hw-strip' },
      chip('chip', f.cpu.name), chip('monitor', gpu.name),
      chip('ram', f.ram.totalBytes ? GB(f.ram.totalBytes) + ' ' + (f.ram.type || '') + (f.ram.configuredMTs ? ' ' + f.ram.configuredMTs + ' MT/s' : '') : ''),
      chip('layout', disp.width ? (disp.monitor ? disp.monitor + ' · ' : '') + disp.currentHz + ' Hz' : ''),
      chip('windows', f.os.caption ? f.os.caption + (f.os.displayVersion ? ' ' + f.os.displayVersion : '') : ''),
      chip('lock', (f.gaming.antiCheats || []).length ? 'Anti-cheat: ' + f.gaming.antiCheats.join(', ') : ''));

    // Your games: up to four, competitive first.
    const isCS2 = gm => /(^|[\\/])cs2\.exe$/i.test(gm.exe || '');
    const initialsOf = n => (n || '?').split(/\s+/).filter(Boolean).slice(0, 2).map(w => w[0]).join('').toUpperCase();
    const gl = ((gd && gd.games) || []).slice().sort((a, b) => (isCS2(b) ? 2 : b.competitive ? 1 : 0) - (isCS2(a) ? 2 : a.competitive ? 1 : 0)).slice(0, 4);
    const gamesCard = gl.length ? h('div', { class: 'card mt-4' },
      h('div', { class: 'card-head' }, icon('gamepad'), h('h3', { text: 'Seus jogos' }), h('button', { class: 'linkbtn small', type: 'button', text: 'Ver a biblioteca →', onclick: () => go('jogos') })),
      h('div', { class: 'game-tiles' }, gl.map(gm => h('div', { class: 'game-tile' },
        h('div', { class: 'gi', text: initialsOf(gm.name) }),
        h('div', { class: 'gt' }, h('div', { class: 'gn', text: gm.name, title: gm.name }), h('div', { class: 'row wrap', style: { gap: '6px', marginTop: '2px' } }, badge(gm.launcher, 'outline'), gm.competitive ? badge('competitivo', 'game') : null)),
        isCS2(gm) ? btn('Perfil CS2', () => go('cs2'), 'game sm') : btn('Ver', () => go('jogos'), 'ghost sm'))))) : null;

    return h('div', { class: 'page home' },
      pageHead('Início', 'Última análise ' + ago(st.lastScan) + ' · perfil ' + prof.title + (offN ? ' · ' + plural(offN, 'ponto fora da nota', 'pontos fora da nota') : ''),
        [btn('Trocar perfil', () => pickProfile(false), 'ghost sm', 'sliders'),
          btn('Modo simples', () => setSimple(true), 'ghost sm', 'layout', { title: 'Uma tela: a nota, o que mais pesa e um botão para o que é seguro' }),
          btn('Exportar relatório', exportReport, 'ghost sm', 'download', { title: 'Página (HTML) ou dados (JSON); anônimo por padrão' })]),
      hwStrip,
      h('div', { class: 'grid g-hero' },
        h('div', { class: 'card hero-score' },
          h('div', { class: 'eyebrow', text: prof.scoreTitle }),
          h('div', { class: 'score' }, ring(rep.overall), h('div', null,
            h('div', { class: 'grade ' + scoreClass(rep.overall), text: rep.grade }),
            h('div', { class: 'muted', style: { margin: '4px 0 12px' }, text: rep.summary }),
            btn('Ver o que pesa na nota', () => go('diagnostico'), 'sm', 'chevron'))),
          h('details', { class: 'note' }, h('summary', { text: 'Como a nota é calculada' }), h('div', { class: 'faint tiny', text: rep.scoreNote }))),
        next),
      cycle,
      h('div', { class: 'stats' },
        stat(B.on.length, 'Ativas', 'já funcionando no seu PC', 'ok', 'on'),
        stat(nRec, 'Recomendadas', 'para você ativar', 'accent', 'rec'),
        stat(B.opt.length, 'Opcionais', 'você decide', 'neutral', 'opt'),
        stat(B.no.length + B.blocked.length, 'Não indicadas', 'melhor deixar como está', 'warn', 'no')),
      h('div', { class: 'grid g-main' },
        h('div', { class: 'card' }, h('div', { class: 'row between wrap' }, h('h2', { style: { margin: 0 } }, icon(nRec ? 'zap' : 'check'), listTitle),
          listAll > listItems.length ? h('button', { class: 'linkbtn small', type: 'button', text: 'Ver todas (' + listAll + ') →', onclick: () => openOptimizations(nRec ? 'rec' : 'on') }) : null),
          listItems.length ? h('div', { class: 'tweak-list' }, listItems.map(t => tweakRow(t, { noDetail: true }))) : h('div', { class: 'callout' }, icon('check'), h('div', { text: 'Nada para mostrar aqui.' }))),
        h('div', { class: 'stack' },
          h('div', { class: 'card' }, h('h2', null, icon('alert'), 'Precisa de atenção'),
            attention.length ? h('div', { class: 'findings' }, attention.map(x => findingCard(x, true))) : h('div', { class: 'callout' }, icon('check'), h('div', { text: 'Nenhum problema importante no seu PC.' })),
            rep.findings.length > attention.length ? h('div', { class: 'mt-3' }, h('button', { class: 'linkbtn small', type: 'button', text: (rep.findings.length === 1 ? 'Ver o ponto do diagnóstico' : 'Ver os ' + rep.findings.length + ' pontos do diagnóstico') + ' →', onclick: () => go('diagnostico') })) : null),
          h('div', { class: 'card' }, h('h2', null, icon('pulse'), 'Notas por área'),
            h('div', { class: 'bars' }, rep.scores.map(s => h('div', { class: 'bar-row' }, h('span', { class: 'ellipsis', text: s.title }),
              h('div', { class: 'bar ' + (s.score >= 90 ? 'ok' : s.score >= 60 ? 'warn' : 'bad') }, h('i', { style: { width: s.score + '%' } })), h('span', { class: 'n', text: String(s.score) }))))))),
      gamesCard,
      h('div', { class: 'eyebrow mt-6', text: 'Outras formas de otimizar' }),
      h('div', { class: 'tiers', style: { marginTop: '0' } },
        tier('Completa', 'Recomendadas + opcionais para você escolher', 'rocket', () => openPlan('completo')),
        tier('Perfil Counter-Strike 2', 'Recomendadas + perfil do cs2.exe no Windows', 'crosshair', () => openPlan('cs2'), 'game'),
        tier('Avaliação completa', 'Análise + rede + FPS + mouse, com três notas', 'gauge', () => go('avaliacao'))),
      h('details', { class: 'card more', id: 'hw-details' }, h('summary', null, icon('chip'), 'Seu PC em detalhes'),
        h('div', { class: 'grid g4 mt-3' },
          hwTile('chip', 'Processador', f.cpu.name, f.cpu.cores + ' núcleos / ' + f.cpu.threads + ' threads' + (f.cpu.isX3D ? ' · 3D V-Cache' : '')),
          hwTile('monitor', 'Placa de vídeo', gpu.name, [gpu.vramMiB ? Math.round(gpu.vramMiB / 1024) + ' GB' : null, gpu.vendorVersion ? 'driver ' + gpu.vendorVersion : gpu.driverVersion].filter(Boolean).join(' · ')),
          hwTile('ram', 'Memória', f.ram.totalBytes ? GB(f.ram.totalBytes) + ' ' + (f.ram.type || '') : '', (f.ram.configuredMTs ? f.ram.configuredMTs + ' MT/s · ' : '') + plural((f.ram.modules || []).length, 'pente', 'pentes')),
          hwTile('hdd', 'Disco do sistema', sysDisk.model, [sysDisk.busType, sysDisk.wearPct >= 0 ? sysDisk.wearPct + '% de desgaste' : null, sysDisk.nvme ? TBw(sysDisk.nvme.dataWrittenBytes) + ' gravados' : null, vol.freeBytes ? GB(vol.freeBytes) + ' livres' : null].filter(Boolean).join(' · ')),
          hwTile('monitor', 'Monitor', disp.monitor, disp.width ? disp.width + '×' + disp.height + ' · ' + disp.currentHz + ' Hz (máx. ' + disp.maxHzAtRes + ' Hz)' : ''),
          hwTile(act.kind === 'Wi-Fi' ? 'wifi' : 'net', 'Rede', act.description, act.kind ? act.kind + (act.linkSpeedMbps ? ' · ' + act.linkSpeedMbps.toLocaleString('pt-BR') + ' Mb/s' : '') : 'sem conexão ativa'),
          hwTile('windows', 'Windows', f.os.caption, (f.os.displayVersion ? f.os.displayVersion + ' · ' : '') + 'build ' + f.os.build + (f.os.ubr ? '.' + f.os.ubr : '')),
          hwTile('lock', 'Segurança', [sec.tpmReady ? 'TPM ' + (sec.tpmVersion || '') : 'sem TPM', sec.secureBoot === 'on' ? 'Secure Boot' : 'sem Secure Boot', sec.vbsRunning ? 'VBS' : null].filter(Boolean).join(' · '),
            (f.gaming.antiCheats || []).length ? 'Anti-cheat: ' + f.gaming.antiCheats.join(', ') : 'Nenhum anti-cheat de kernel encontrado'))));
  }

  // ---------------------------------------------------------------------------
  // Full evaluation: three scores
  // ---------------------------------------------------------------------------

  async function startEvaluation() {
    let ev;
    try { ev = await get('evaluation', '/api/evaluation', true); } catch (e) { toast(e.message, 'bad'); return; }
    const opt = (label, checked, disabled, note) => {
      const cb = h('input', { type: 'checkbox', checked: !!checked && !disabled, disabled: !!disabled });
      return { cb, el: h('label', { class: 'check' + (disabled ? ' muted' : '') }, cb, h('div', null, h('div', { text: label }), note ? h('div', { class: 'small muted', text: note }) : null)) };
    };
    const net = opt('Teste de rede com carga', true, !!ev.netWhy, ev.netWhy || 'Ping, jitter, perda e bufferbloat: cerca de 40 s; usa a conexão por alguns segundos em cada sentido.');
    const perf = opt('FPS no Counter-Strike 2 (60 s)', ev.canPerf, !ev.canPerf, ev.canPerf ? 'Volte para o jogo quando a contagem de 15 s começar e jogue normalmente por 60 s.' : (ev.perfWhy || 'Abra o CS2 numa partida antes de avaliar.'));
    const mouse = opt('Taxa do mouse (6 s)', true, false, 'Uma janela pede para mover o mouse em círculos até a contagem terminar.');
    const ok = await confirmDlg({
      title: 'Avaliação completa', ok: 'Avaliar', icon: 'pulse', wide: true,
      body: h('div', null,
        h('p', { class: 'small', style: { marginTop: 0 }, text: 'A análise do PC roda sempre; marque o que mais medir. Nada é alterado: a avaliação só lê e mede.' }),
        h('div', { class: 'stack' }, net.el, perf.el, mouse.el)),
    });
    if (!ok) return;
    await runJob(api('POST', '/api/evaluate', { net: net.cb.checked, perf: perf.cb.checked, perfSeconds: 60, periph: mouse.cb.checked }), { title: 'Avaliação completa' });
  }

  function pillarCard(title, ic, rep, extra, link, max) {
    const items = (rep.findings || []).filter(x => !x.dismissed && !x.offProfile);
    let bad = items.filter(x => x.severity !== 'good');
    const good = items.filter(x => x.severity === 'good');
    const more = max && bad.length > max ? bad.length - max : 0;
    if (more) bad = bad.slice(0, max);
    return h('div', { class: 'card pillar' }, h('h2', null, icon(ic), title),
      h('div', { class: 'score' }, ring(rep.overall), h('div', null,
        h('div', { class: 'grade ' + scoreClass(rep.overall), text: rep.grade }),
        h('div', { class: 'muted small', style: { margin: '4px 0 10px' }, text: rep.summary }),
        link ? btn(link[0], () => go(link[1]), 'sm', 'chevron') : null)),
      extra || null,
      bad.length ? h('div', { class: 'findings', style: { marginTop: '14px' } }, bad.map(x => findingCard(x, true))) : null,
      more ? h('div', { class: 'small muted', style: { marginTop: '10px' } }, h('button', { class: 'linkbtn small', type: 'button', text: '+ ' + plural(more, 'ponto no diagnóstico →', 'pontos no diagnóstico →'), onclick: () => go(link ? link[1] : 'diagnostico') })) : null,
      good.length ? h('details', { style: { marginTop: '12px' } }, h('summary', { class: 'small linkbtn', text: 'Tudo certo (' + good.length + ')' }),
        h('div', { class: 'findings', style: { marginTop: '10px' } }, good.map(x => findingCard(x, true)))) : null);
  }

  async function pageAvaliacao() {
    const ev = await get('evaluation', '/api/evaluation');
    const st = S.state;
    const last = ev.lastRun;
    const missing = (ev.missing || []).length ? h('div', { class: 'callout', style: { marginTop: '12px' } }, icon('info'),
      h('div', null, h('b', { text: 'Ainda não medido: ' }), ev.missing.join(', ') + '. O que não foi medido não entra na nota; o botão Avaliar tudo mede de uma vez.',
        h('div', { class: 'mt-2' }, btn('Avaliar tudo', startEvaluation, 'sm', 'pulse', { disabled: !!(S.busy || st.job) })))) : null;
    return h('div', { class: 'page' },
      pageHead('Avaliação completa', 'Três notas para o perfil ' + ev.profile.title + ': o que o Windows faz com o hardware, o que o hardware consegue para o seu uso e o que foi medido neste PC.' + (last ? ' Última avaliação completa: ' + ago(last.at) + '.' : ''),
        [btn('Avaliar tudo', startEvaluation, 'primary', 'pulse', { disabled: !!(S.busy || st.job) }), btn('Trocar perfil', () => pickProfile(false), 'ghost', 'sliders')]),
      last && last.steps ? h('div', { class: 'row wrap small', style: { margin: '-6px 0 14px' } }, last.steps.map(s => badge(s.title + ': ' + ({ done: 'ok', skipped: 'pulado', failed: 'falhou', cancelled: 'cancelado' }[s.status] || s.status) + (s.note ? ' — ' + s.note : ''), s.status === 'done' ? 'ok' : s.status === 'skipped' ? 'outline' : 'warn'))) : null,
      h('div', { class: 'grid g3 pillars' },
        pillarCard('Configuração', 'sliders', ev.config, null, ['Ver diagnóstico', 'diagnostico'], 5),
        pillarCard('Capacidade', 'chip', ev.capacity, null, null),
        pillarCard('Medição', 'gauge', ev.measurement, missing, null)),
      h('div', { class: 'faint tiny', style: { marginTop: '14px' }, text: ev.note }));
  }

  let diagFilter = 'all';
  async function pageDiag() {
    const [rep, f, pr] = await Promise.all([get('report', '/api/report'), get('facts', '/api/facts'), profiles()]);
    const prof = profileOf(rep.profile || S.state.settings.profile, pr.profiles);
    const active = rep.findings.filter(x => !x.dismissed && !x.offProfile);
    const dismissed = rep.findings.filter(x => x.dismissed);
    const off = rep.findings.filter(x => x.offProfile);
    const count = s => active.filter(x => x.severity === s).length;
    const dc = (id, label, n, kind, ic) => h('button', { class: 'dcount ' + kind + (diagFilter === id ? ' active' : ''), type: 'button', 'aria-pressed': diagFilter === id ? 'true' : 'false', title: diagFilter === id ? 'Mostrar todos' : 'Mostrar só: ' + label.toLowerCase(), onclick: () => { diagFilter = diagFilter === id ? 'all' : id; rerender(); } },
      icon(ic), h('span', { class: 'n', text: String(n) }), h('span', { class: 'l', text: label }));
    const pass = x => diagFilter === 'all' || x.severity === diagFilter;
    const byArea = rep.scores.map(s => ({ s, items: active.filter(x => x.area === s.area && pass(x)) })).filter(g => g.items.length);
    return h('div', { class: 'page' },
      pageHead('Diagnóstico', 'Cada ponto mostra a evidência lida do seu PC, o porquê e a correção. Última análise: ' + fmtDT(S.state.lastScan) + ' (' + (f.scanSeconds || 0).toFixed(1).replace('.', ',') + ' s).',
        []),
      h('div', { class: 'diag-sum', role: 'group', 'aria-label': 'Filtrar por gravidade' }, dc('critical', 'Importantes', count('critical'), 'bad', 'alert'), dc('warning', 'Atenção', count('warning'), 'warn', 'alertc'), dc('info', 'Sugestões', count('info'), 'info', 'info'), dc('good', 'Tudo certo', count('good'), 'ok', 'check')),
      byArea.length ? byArea.map(g => h('div', { style: { marginBottom: '22px' } },
        h('div', { class: 'row', style: { margin: '0 2px 10px' } }, h('h3', { style: { margin: 0, fontSize: '15px' }, text: g.s.title }), badge('nota ' + g.s.score, g.s.score >= 90 ? 'ok' : g.s.score >= 60 ? 'warn' : 'bad'),
          h('div', { class: 'bar ' + (g.s.score >= 90 ? 'ok' : g.s.score >= 60 ? 'warn' : 'bad'), style: { width: '120px' }, 'aria-hidden': 'true' }, h('i', { style: { width: g.s.score + '%' } }))),
        h('div', { class: 'findings' }, g.items.map(x => findingCard(x, false, { dismiss: true })))))
        : empty('check', 'Nada neste filtro', 'Escolha outro filtro para ver os demais pontos.'),
      off.length ? h('details', { class: 'card', style: { marginBottom: '22px' } },
        h('summary', { class: 'small', text: 'Fora do perfil ' + prof.title + ' (' + off.length + ') — não contam na nota neste perfil' }),
        h('p', { class: 'small muted', style: { margin: '10px 0 0' }, text: 'Pontos que só fazem diferença em jogo ou no desempenho bruto (e, no perfil jogo + live, o HAGS, que é decisão sua). Continuam aqui para consulta; se este PC também for usado para jogar, troque o perfil no Início e eles voltam a contar.' }),
        h('div', { class: 'findings', style: { marginTop: '12px' } }, off.map(x => findingCard(x, false)))) : null,
      dismissed.length ? h('details', { class: 'card', style: { marginBottom: '22px' } },
        h('summary', { class: 'small', text: 'Dispensados por você (' + dismissed.length + ') — não contam na nota' }),
        h('div', { class: 'findings', style: { marginTop: '12px' } }, dismissed.map(x => findingCard(x, false, { dismiss: true })))) : null,
      collectedData(f));
  }

  // Raw values read from Windows, shown in Portuguese.
  const PT = {
    on: 'ligado', off: 'desligado', unsupported: 'não suportado', unknown: 'desconhecido', enabled: 'ligado', disabled: 'desligado',
    Healthy: 'saudável', Warning: 'atenção', Unhealthy: 'com falha', Unspecified: 'não informado', Other: 'outro',
    normal: 'normal (padrão)', restricted: 'restrito', highlyrestricted: 'muito restrito', experimental: 'experimental',
  };
  const pt = v => (v == null || v === '' ? v : PT[v] || v);
  // Win32_DeviceGuard SecurityServicesRunning codes.
  const VBS_SVC = { 1: 'Credential Guard', 2: 'Integridade de Memória', 3: 'System Guard', 4: 'medição SMM', 5: 'proteção de pilha (kernel)', 6: 'proteção de pilha (auditoria)', 7: 'HVPT' };
  const vbsServices = list => (list || []).length ? list.map(n => VBS_SVC[n] || ('código ' + n)).join(', ') : 'nenhum listado';
  const VBS_SCENARIO = { hypervisorenforcedcodeintegrity: 'Integridade de Memória (HVCI)', credentialguard: 'Credential Guard', systemguard: 'System Guard', kernelshadowstacks: 'proteção de pilha no modo kernel', securebiometrics: 'Segurança de entrada aprimorada do Windows Hello', hypervisorenforcedpagingtranslation: 'HVPT' };
  const vbsScenario = k => VBS_SCENARIO[String(k).toLowerCase()] || k;
  // What Windows exposes about heat without a driver: the ACPI thermal zones
  // and the processors' thermal frequency limit (never the processor's own
  // temperature, which only a driver reads).
  function thermalBlock(f, kv) {
    const th = f.thermal || {};
    const zones = th.zones || [];
    const rows = zones.map(z => [z.name || 'Zona térmica', (z.tempC >= 0 ? fmtN(z.tempC, 0) + ' °C' : 'sem leitura de temperatura') + ' · ' +
      (z.passiveLimitPct < 100 ? 'limitando a ' + z.passiveLimitPct + '%' + ((z.reasons & 1) ? ' (temperatura)' : (z.reasons & 2) ? ' (corrente elétrica)' : '') : 'sem limite')]);
    if (!th.zonesRead) rows.push(['Zonas térmicas', 'não lidas']);
    else if (!zones.length) rows.push(['Zonas térmicas', 'nenhuma exposta pelo firmware (comum em desktop e em muitos notebooks)']);
    rows.push(['Limite de frequência por temperatura', !th.limitRead ? 'não lido' : th.mhzLimit < th.maxMhz ? th.mhzLimit + ' de ' + th.maxMhz + ' MHz (limitando agora)' : 'sem limite agora (' + th.maxMhz + ' MHz)']);
    return h('div', { class: 'mt-4' }, h('h3', { text: 'Temperatura (o que o Windows informa)' }), kv(rows),
      h('p', { class: 'small muted', text: 'Uma zona térmica é uma área da placa ou do gabinete escolhida pelo fabricante, não o núcleo do processador. A temperatura do próprio processador só um driver lê, e o Axion não instala driver.' }));
  }

  function collectedData(f) {
    const kv = pairs => h('dl', { class: 'kv' }, pairs.filter(p => p[1] !== undefined && p[1] !== null && p[1] !== '').map(([k, v]) => [h('dt', { text: k }), h('dd', { text: String(v) })]));
    const yes = b => (b ? 'sim' : 'não');
    const table = (cols, rows) => h('div', { class: 'tbl-scroll' }, h('table', { class: 'table' }, h('thead', null, h('tr', null, cols.map(c => h('th', { text: c })))), h('tbody', null, rows.map(r => h('tr', null, r.map(c => h('td', { text: c == null ? '—' : String(c) })))))));
    return h('details', { class: 'card', style: { marginTop: '8px' } },
      h('summary', { style: { cursor: 'pointer', fontWeight: 600 } }, 'Dados coletados deste PC'),
      h('div', { class: 'stack', style: { marginTop: '16px' } },
        h('div', { class: 'grid g2' },
          h('div', null, h('h3', { text: 'Sistema' }), kv([['Windows', f.os.caption + ' ' + (f.os.displayVersion || '')], ['Build', f.os.build + '.' + f.os.ubr], ['Firmware', f.os.firmware || 'desconhecido'], ['Último boot', fmtDT(f.os.lastBoot)], ['Placa-mãe', (f.board.manufacturer || '') + ' ' + (f.board.product || '')], ['BIOS', (f.board.biosVersion || '') + ' (' + fmtDay(f.board.biosDate) + ')'], ['Plano de energia', f.power.activeName], ['Notebook', yes(f.power.isLaptop)]])),
          h('div', null, h('h3', { text: 'Processador e jogo' }), kv([['CPU', f.cpu.name], ['Núcleos/threads', f.cpu.cores + '/' + f.cpu.threads], ['Microcode', f.cpu.microcode ? '0x' + f.cpu.microcode.toString(16).toUpperCase() + (f.cpu.family ? ' (CPUID ' + [f.cpu.family, f.cpu.model, f.cpu.stepping].map(n => (n || 0).toString(16).toUpperCase().padStart(2, '0')).join('-') + ')' : '') : 'desconhecido'], ['Modo de Jogo', yes(f.gaming.gameModeOn)], ['HAGS', f.gaming.hags === 2 ? 'ligado' : f.gaming.hags === 1 ? 'desligado' : 'não suportado'], ['Game Bar', yes(f.gaming.gameBarInstalled)], ['Gravação contínua', yes(f.gaming.historicalRecord)], ['Resolução do timer', f.timer && f.timer.current ? (f.timer.current / 10000).toFixed(2).replace('.', ',') + ' ms' : '—']]))),
        h('div', null, h('h3', { text: 'Memória' }), table(['Slot', 'Fabricante', 'Código', 'Capacidade', 'Velocidade'], (f.ram.modules || []).map(m => [(m.bank + ' ' + m.locator).trim(), m.manufacturer, m.partNumber, GB(m.capacityBytes), (m.configuredMTs || m.speedMTs) + ' MT/s']))),
        h('div', null, h('h3', { text: 'Armazenamento' }),
          table(['Disco', 'Tipo', 'Tamanho', 'Saúde', 'Desgaste', 'Gravado', 'Temp.', 'Horas ligado', 'Alertas do SSD'], (f.disks || []).map(d => [d.model + (d.isSystem ? ' (sistema)' : ''), [d.busType, d.mediaType === 'Unspecified' ? '' : d.mediaType].filter(Boolean).join(' '), GB(d.sizeBytes), pt(d.health) || '—',
            d.wearPct >= 0 ? d.wearPct + '%' : '—', d.nvme ? TBw(d.nvme.dataWrittenBytes) : '—', d.tempC >= 0 ? d.tempC + ' °C' : '—', d.powerOnHours ? d.powerOnHours.toLocaleString('pt-BR') : '—', diskAlerts(d)])),
          h('p', { class: 'small muted', style: { margin: '6px 2px 0' }, text: 'Desgaste, gravado e alertas vêm do log SMART/Saúde do próprio SSD (NVMe) ou dos contadores de confiabilidade do Windows. "—" = o disco ou o adaptador USB não informa.' })),
        h('div', { class: 'grid g2' },
          h('div', null, h('h3', { text: 'Monitores' }), table(['Monitor', 'Modo', 'Máximo'], (f.displays || []).map(d => [d.monitor, d.width + '×' + d.height + ' @ ' + d.currentHz + ' Hz', d.maxHzAtRes + ' Hz']))),
          h('div', null, h('h3', { text: 'Rede' }), table(['Adaptador', 'Tipo', 'Link', 'Ativo'], (f.adapters || []).map(a => [a.description, pt(a.kind), a.linkSpeedMbps ? a.linkSpeedMbps.toLocaleString('pt-BR') + ' Mb/s' : '—', yes(a.up)])))),
        h('div', { class: 'grid g2' },
          h('div', null, h('h3', { text: 'Segurança' }), kv([['VBS em execução', yes(f.security.vbsRunning)], ['Integridade de Memória', yes(f.security.hvciRunning)],
            ['Recursos usando o VBS', vbsServices(f.security.servicesRunning)],
            ['IOMMU (proteção de DMA)', f.security.iommu === 'on' ? 'ligado' : f.security.iommu === 'off' ? 'desligado ou ausente' : 'desconhecido'],
            f.security.vbsPolicy || f.security.hvciPolicy || f.security.vbsLocked ? ['Política / bloqueio UEFI', [f.security.vbsPolicy ? 'política VBS ' + (f.security.vbsPolicy === 'on' ? 'ligada' : 'desligada') : null, f.security.hvciPolicy ? 'política Integridade de Memória ' + ({ on: 'ligada', off: 'desligada', lock: 'ligada com bloqueio UEFI' })[f.security.hvciPolicy] : null, f.security.vbsLocked ? 'bloqueio UEFI' : null].filter(Boolean).join('; ')] : null,
            (f.security.vbsScenarios || []).length ? ['Cenários do Device Guard ligados', f.security.vbsScenarios.map(vbsScenario).join(', ')] : null,
            (f.security.hypervisorFeatures || []).length ? ['Também iniciam o hipervisor', f.security.hypervisorFeatures.join(', ')] : null,
            ['TPM', f.security.tpmReady ? 'pronto (' + f.security.tpmVersion + ')' : f.security.tpmPresent ? 'presente, não pronto' : 'não encontrado'], ['Secure Boot', pt(f.security.secureBoot)], ['Virtualização no firmware', yes(f.security.virtualizationFirmware)], ['Defender em tempo real', pt(f.security.defenderRealtime)], ['Anti-cheats', (f.gaming.antiCheats || []).join(', ') || 'nenhum']].filter(Boolean))),
          h('div', null, h('h3', { text: 'Inicialização (BCD)' }), kv([['useplatformclock', bcdVal(f.boot.usePlatformClock)], ['disabledynamictick', bcdVal(f.boot.disableDynamicTick)], ['useplatformtick', bcdVal(f.boot.usePlatformTick)], ['hypervisorlaunchtype', f.boot.hypervisorLaunch || '(padrão)'], ['testsigning', bcdVal(f.boot.testSigning)], ['nx', f.boot.nx || '—'], ['Ajuste automático do TCP', pt(f.net.tcpAutoTuning) || '—'], ['RSS (várias CPUs na rede)', pt(f.net.rss) || '—']]))),
        thermalBlock(f, kv),
        (f.errors || []).length ? h('div', null, h('h3', { text: 'Avisos da coleta' }), h('ul', { class: 'list-clean small muted' }, f.errors.map(e => h('li', { text: e })))) : null));
  }

  // '*' = every category together. optFilter null = decided on first open
  // (the recommended ones when there are any, otherwise everything).
  let optCat = '*';
  let optQuery = '';
  let optFilter = null;
  const OPT_FILTERS = [['rec', 'Recomendadas'], ['on', 'Ativas'], ['opt', 'Opcionais'], ['no', 'Não indicadas'], ['na', 'Não se aplicam'], ['all', 'Todas']];
  const optMatch = (t, fl) => { const b = tweakBucket(t); return fl === 'all' ? b !== 'na' : fl === 'no' ? (b === 'no' || b === 'blocked') : b === fl; };
  function openOptimizations(filter, cat) {
    const already = currentPage() === 'otimizacoes';
    optFilter = filter || 'all'; optCat = cat || '*'; optQuery = '';
    go('otimizacoes');
    if (already) rerender(true); // same address: no hashchange to redraw it
  }
  // activeHoursCard sets the active hours of Windows Update (0.7): the
  // window in which Windows does not restart the PC. The item is built on
  // demand from the hours chosen, applied through the same engine (it goes
  // to the history and can be undone).
  function activeHoursCard(f) {
    const os = f.os || {};
    const set = os.activeHoursStart >= 0 && os.activeHoursEnd >= 0;
    const curS = set ? os.activeHoursStart : 8, curE = set ? os.activeHoursEnd : 17;
    const auto = os.activeHoursAuto === 1;
    const policy = !!os.activeHoursPolicy;
    const hours = [];
    for (let i = 0; i < 24; i++) hours.push(h('option', { value: String(i), text: String(i).padStart(2, '0') + ':00' }));
    const selS = h('select', { 'aria-label': 'Início do horário ativo' }, hours.map(o => o.cloneNode(true)));
    const selE = h('select', { 'aria-label': 'Fim do horário ativo' }, hours.map(o => o.cloneNode(true)));
    selS.value = String(set ? curS : 18); selE.value = String(set ? curE : 2);
    const note = h('div', { class: 'small muted', style: { marginTop: '8px' } });
    const applyB = btn('Aplicar horário', async () => {
      const a = Number(selS.value), b = Number(selE.value), span = (b - a + 24) % 24;
      if (a === b || span > 18) { toast('A janela precisa ter de 1 a 18 horas.', 'bad'); return; }
      const id = 'activehours:' + a + '-' + b;
      const label = 'Horário ativo das ' + String(a).padStart(2, '0') + ':00 às ' + String(b).padStart(2, '0') + ':00';
      const ok = await confirmDlg({ title: 'Horário ativo do Windows Update', ok: 'Aplicar', icon: 'zap',
        body: h('div', null, h('p', { style: { marginTop: 0 }, text: 'O Windows não vai reiniciar para instalar atualizações ' + label.replace('Horário ativo ', '') + ' (' + span + ' h). Fora da janela as atualizações seguem normalmente.' }),
          auto ? h('div', { class: 'callout warn' }, icon('alert'), h('div', { text: 'O ajuste automático do horário ativo está ligado: o Windows pode mover a janela. Depois de aplicar, escolha "Manualmente" em Configurações › Windows Update › Opções avançadas › Horário ativo.' })) : null,
          h('p', { class: 'small muted', text: 'A mudança fica no histórico e pode ser desfeita.' })) });
      if (!ok) return;
      await runJob(api('POST', '/api/apply', { ids: [id], label }), { title: label });
    }, 'primary sm', 'zap');
    const upd = () => { const a = Number(selS.value), b = Number(selE.value), span = (b - a + 24) % 24; note.textContent = a === b || span > 18 ? 'A janela precisa ter de 1 a 18 horas.' : 'Janela de ' + span + ' h: o Windows não reinicia nesse período.'; applyB.disabled = a === b || span > 18; };
    selS.addEventListener('change', upd); selE.addEventListener('change', upd); upd();
    return h('div', { class: 'card', style: { marginBottom: '14px' } },
      h('h3', null, icon('clock'), 'Horário ativo do Windows Update'),
      h('div', { class: 'small', style: { marginBottom: '10px' } }, h('span', { class: 'muted', text: 'Atual: ' }), policy ? 'fixado por política de grupo (das ' + String(os.activeHoursPolicyStart).padStart(2, '0') + ':00 às ' + String(os.activeHoursPolicyEnd).padStart(2, '0') + ':00); o Axion não altera' : (set ? 'das ' + String(curS).padStart(2, '0') + ':00 às ' + String(curE).padStart(2, '0') + ':00' : 'não definido no registro (o Windows usa 08:00 às 17:00, ou o ajuste automático)') + (auto ? ' · ajuste automático ligado' : os.activeHoursAuto === -1 ? ' · ajuste automático: não gravado (o Windows 11 vem com "Automaticamente")' : ' · ajuste automático desligado')),
      policy ? null : h('div', { class: 'row wrap', style: { gap: '10px', alignItems: 'center' } }, h('span', { class: 'small', text: 'Das' }), selS, h('span', { class: 'small', text: 'às' }), selE, applyB,
        btn('Abrir Configurações', () => openURL('ms-settings:windowsupdate-options'), 'ghost sm', 'ext')),
      policy ? null : note,
      h('div', { class: 'small muted', style: { marginTop: '8px' }, text: 'Dentro da janela (até 18 h) o Windows não reinicia o PC para atualizar: coloque nela as horas em que você joga, transmite ou trabalha. Grava ActiveHoursStart/End, os mesmos valores das Configurações.' }));
  }

  async function pageOpt() {
    const [tw, f] = await Promise.all([get('tweaks', '/api/tweaks'), get('facts', '/api/facts')]);
    const cats = tw.categories;
    const byCat = {};
    cats.forEach(c => { byCat[c.id] = []; });
    tw.tweaks.forEach(t => { (byCat[t.category] = byCat[t.category] || []).push(t); });
    const count = fl => tw.tweaks.filter(t => optMatch(t, fl)).length;
    if (!optFilter) optFilter = count('rec') ? 'rec' : 'all';
    if (optCat !== '*' && !cats.find(c => c.id === optCat)) optCat = '*';
    const search = h('input', { class: 'search', type: 'search', placeholder: 'Buscar otimização…', 'aria-label': 'Buscar otimização', value: optQuery, 'data-focus': 'opt-search' });
    let timer = null;
    search.addEventListener('input', () => { clearTimeout(timer); timer = setTimeout(() => { optQuery = search.value; rerender(); }, 220); });
    const q = optQuery.trim().toLowerCase();
    const filterLabel = (OPT_FILTERS.find(x => x[0] === optFilter) || [])[1] || '';
    const EMPTY = {
      rec: ['check', 'Nada a recomendar agora', 'Tudo o que o Axion recomenda para o seu PC já está ativo.'],
      on: ['sliders', 'Nenhuma otimização ativa ainda', 'Quando você aplicar algo, aparece aqui e no Histórico.'],
      opt: ['sliders', 'Nenhuma opção extra', 'Não há itens opcionais pendentes neste filtro.'],
      no: ['check', 'Nada nesta lista', 'Nenhum item aqui para o seu PC.'],
      na: ['check', 'Nada nesta lista', 'Todos os itens se aplicam ao seu PC.'],
      all: ['check', 'Nada a fazer aqui', 'Nenhum item desta categoria se aplica a este PC.'],
    };
    const emptyFor = () => empty(...(EMPTY[optFilter] || EMPTY.all));
    let right;
    if (q) {
      const hits = tw.tweaks.filter(t => (t.title + ' ' + t.summary + ' ' + t.detail + ' ' + (t.tags || []).join(' ')).toLowerCase().includes(q));
      right = h('div', null, h('div', { class: 'row', style: { marginBottom: '12px' } }, h('span', { class: 'muted', text: plural(hits.length, 'resultado', 'resultados') + ' para "' + optQuery + '"' })),
        hits.length ? hits.map(t => tweakRow(t)) : empty('sliders', 'Nada encontrado', 'Tente outra palavra.'));
    } else {
      const cat = optCat === '*' ? null : cats.find(c => c.id === optCat);
      const scope = (cat ? byCat[cat.id] : tw.tweaks) || [];
      const shown = scope.filter(t => optMatch(t, optFilter));
      const pend = scope.filter(t => tweakBucket(t) === 'rec').length;
      const onN = scope.filter(t => tweakBucket(t) === 'on').length;
      const applN = scope.filter(t => tweakBucket(t) !== 'na').length;
      const headTitle = cat ? cat.title : 'Todas as categorias';
      const headDesc = cat ? cat.description : 'Tudo o que o Axion avaliou para o seu PC, do mais útil ao opcional.';
      // With every category together, items stay grouped under their category's name.
      const body = !shown.length ? emptyFor()
        : cat ? shown.map(t => tweakRow(t))
          : cats.map(c => { const l = shown.filter(t => t.category === c.id); return l.length ? h('div', { class: 'opt-group' }, h('div', { class: 'group-title' }, icon(CAT_ICON[c.icon] || 'sliders'), c.title, h('span', { class: 'n', text: String(l.length) })), l.map(t => tweakRow(t))) : null; });
      right = h('div', null,
        h('div', { class: 'card opt-head' },
          h('div', { class: 'row between wrap' }, h('div', { class: 'oh' }, icon(cat ? (CAT_ICON[cat.icon] || 'sliders') : 'layers'), h('div', null, h('h2', { style: { margin: '0 0 4px' } }, headTitle), h('div', { class: 'muted small', text: headDesc }))),
            // With every category together the page header already holds this button.
            cat && pend ? btn('Aplicar recomendadas desta categoria (' + pend + ')', () => openPlan('categoria', cat.id), 'primary', 'zap')
              : cat && scope.some(t => t.oneClick && t.state.applicable && t.state.rec && t.state.rec.level === 1) ? badge('recomendadas já aplicadas', 'ok', 'check') : null),
          h('div', { class: 'small faint', style: { marginTop: '10px' }, text: onN + ' de ' + applN + ' ativas neste PC' + (optFilter !== 'all' ? ' · mostrando: ' + filterLabel.toLowerCase() : '') })),
        optCat === 'update' ? activeHoursCard(f) : null,
        body);
    }
    const chips = h('div', { class: 'chips opt-filters', role: 'group', 'aria-label': 'Filtrar por situação' }, OPT_FILTERS.map(([id, label]) =>
      h('button', { class: 'chip' + (optFilter === id && !q ? ' active' : ''), type: 'button', onclick: () => { optFilter = id; optQuery = ''; rerender(true); } }, label, h('b', { text: String(count(id)) }))));
    return h('div', { class: 'page' },
      pageHead('Otimizações', 'Cada item diz se está ativo, se é recomendado para o seu PC ou se é só uma opção. Ligue o que quiser; tudo pode ser desfeito no Histórico.',
        count('rec') ? [btn('Aplicar recomendadas (' + count('rec') + ')', () => openPlan('recomendado'), 'primary', 'zap')] : []),
      chips,
      h('div', { class: 'opt-layout' },
        h('div', { class: 'card', style: { padding: '12px' } }, search, h('div', { class: 'cat-list', style: { padding: '8px 0 0' } },
          [{ id: '*', title: 'Todas as categorias', icon: 'layers' }].concat(cats).map(c => {
            const list = c.id === '*' ? tw.tweaks : (byCat[c.id] || []);
            const n = list.filter(t => optMatch(t, optFilter)).length;
            return h('button', { class: 'cat-item' + (c.id === optCat && !q ? ' active' : ''), type: 'button', style: !n && c.id !== '*' ? { opacity: '.5' } : null, onclick: () => {
              optCat = c.id; optQuery = ''; rerender(true);
              if (window.innerWidth <= 1100) requestAnimationFrame(() => { const r = $('.opt-layout > div:nth-child(2)'); if (r) r.scrollIntoView({ block: 'start' }); });
            } },
              icon(c.id === '*' ? 'layers' : (CAT_ICON[c.icon] || 'sliders')), h('span', { text: c.title }), n ? h('span', { class: 'nav-count', text: String(n) }) : null);
          }))),
        h('div', null, right)));
  }

  // Preset competitivo do CS2 (cs2_video.txt): lista antes → depois, marcação
  // editável, confirmação com a lista exata e "Desfazer preset" (o desfazer é
  // o do histórico, valor por valor).
  const PRESET_KIND = { medido: ['custo publicado pela Intel, fora do CS2', 'info'], latencia: ['menos atraso, não FPS', 'info'], hipotese: ['hipótese a medir', 'outline'] };
  function presetCard(p) {
    const items = p.items || [];
    const canPick = i => !p.running && i.applicable && i.status !== 'on' && (i.changes || []).length > 0;
    const sel = new Set(items.filter(i => i.selected && canPick(i)).map(i => i.id));
    let applyBtn = null;
    const sync = () => { if (applyBtn) applyBtn.disabled = p.running || sel.size === 0; };
    const row = i => {
      const kind = PRESET_KIND[i.kind] || PRESET_KIND.hipotese;
      const changes = (i.changes || []).map(c => c.title + ': ' + c.fromLabel + ' → ' + c.toLabel).join(' · ');
      return h('div', { class: 'set-row', style: { alignItems: 'flex-start' } },
        h('label', { class: 'row' },
          h('input', { type: 'checkbox', checked: sel.has(i.id), disabled: !canPick(i), 'aria-label': i.title,
            onchange: e => { if (e.target.checked) sel.add(i.id); else sel.delete(i.id); sync(); } }),
          h('b', { text: i.title })),
        h('div', null,
          h('div', { class: 'row wrap' }, badge(kind[0], kind[1]), evBadge(i.evidence), i.byAxion ? badge('aplicado pelo Axion', 'ok', 'check') : null, i.drifted ? badge('alterado depois do Axion', 'warn') : null),
          h('div', { class: 'small', text: i.summary }),
          h('div', { class: 'small', text: changes || (i.status === 'on' ? 'Já no valor do preset.' : (i.naReason || '')) }),
          i.why ? h('div', { class: 'small muted', text: i.why }) : null,
          (i.sources || []).length ? h('div', { class: 'row wrap' }, i.sources.map(s => extLink(s.title, s.url))) : null));
    };
    async function apply() {
      const chosen = items.filter(i => sel.has(i.id));
      if (!chosen.length) { toast('Marque pelo menos uma opção.', 'warn'); return; }
      const body = h('div', null,
        h('p', { text: 'O Axion vai mudar só estes valores em ' + p.path + ':' }),
        h('ul', null, chosen.flatMap(i => i.changes.map(c => h('li', { text: c.title + ': ' + c.fromLabel + ' → ' + c.toLabel })))),
        h('p', { class: 'small', text: 'Antes, uma cópia do arquivo vai para a pasta de dados do Axion. Para voltar: "Desfazer preset" aqui ou "Desfazer lote" no Histórico. Vale ao abrir o jogo.' }));
      if (!await confirmDlg({ title: 'Aplicar preset competitivo do CS2', body, ok: 'Aplicar', icon: 'zap', wide: true })) return;
      await runJob(api('POST', '/api/cs2/preset', { ids: chosen.map(i => i.id) }), { title: 'Preset competitivo do CS2' });
      invalidate(); rerender();
    }
    async function undo() {
      if (!await confirmDlg({ title: 'Desfazer o preset do CS2', text: 'Restaurar no cs2_video.txt os valores de antes de ' + plural(p.active.length, 'opção', 'opções') + '?', ok: 'Desfazer', danger: true, icon: 'undo' })) return;
      await runJob(api('POST', '/api/revert', { ids: p.active }), { title: 'Desfazendo o preset do CS2' });
      invalidate(); rerender();
    }
    applyBtn = btn('Aplicar preset', apply, 'primary', 'zap');
    sync();
    return h('div', { class: 'card', style: { marginTop: '16px' } },
      h('h2', null, icon('zap'), 'Preset competitivo (cs2_video.txt)'),
      h('div', { class: 'callout' }, icon('info'), h('div', { text: p.honest })),
      p.running ? h('div', { class: 'callout warn', style: { marginTop: '10px' } }, icon('alert'), h('div', null, 'O Counter-Strike 2 está aberto. Feche o jogo para aplicar ou desfazer.', ' ', btn('Verificar de novo', () => { invalidate(); rerender(); }, 'sm ghost', 'refresh'))) : null,
      h('div', { class: 'small muted', style: { marginTop: '8px' }, text: p.cloudNote }),
      p.basis ? h('div', { class: 'small muted', text: 'Medição: ' + p.basis + '.' }) : null,
      h('div', { class: 'set-list', style: { marginTop: '12px' } }, items.map(row)),
      h('div', { class: 'row', style: { marginTop: '12px' } },
        applyBtn,
        (p.active || []).length ? btn('Desfazer preset', undo, 'ghost', 'undo', { disabled: p.running }) : null),
      h('details', { style: { marginTop: '12px' } }, h('summary', { text: 'O que o preset não muda (preferência sua)' }), h('div', { class: 'checks' }, (p.untouched || []).map(c => checkItem(c)))));
  }

  async function pageCS2() {
    const [g, games] = await Promise.all([get('cs2', '/api/cs2'), get('games', '/api/games')]);
    const info = g.info || {};
    const game = (games.games || []).find(x => (x.exeName || '').toLowerCase() === 'cs2.exe');
    const v = info.video;
    const head = pageHead('Counter-Strike 2', 'Diagnóstico específico do CS2: configurações lidas do próprio jogo, opções de inicialização da Steam e o perfil do cs2.exe.',
      [btn('Medir FPS', () => { perfForm.process = 'cs2.exe'; go('desempenho'); }, '', 'gauge'), btn('Aplicar perfil CS2', () => openPlan('cs2'), 'game', 'zap')]);
    head.classList.add('game-head');
    head.firstChild.insertBefore(h('div', { class: 'eyebrow', text: 'Jogo competitivo' }), head.firstChild.firstChild);
    return h('div', { class: 'page' },
      head,
      h('div', { class: 'callout' }, icon('info'), h('div', { text: g.honest })),
      h('div', { class: 'grid g2', style: { marginTop: '16px' } },
        h('div', { class: 'card' }, h('h2', null, icon('crosshair'), 'Instalação'),
          info.installed && info.game
            ? h('dl', { class: 'kv' }, h('dt', { text: 'Status' }), h('dd', null, badge('encontrado', 'ok', 'check')), h('dt', { text: 'Executável' }), h('dd', { class: 'mono', text: info.game.exe || '—' }),
              h('dt', { text: 'Anti-cheat' }), h('dd', { text: info.game.antiCheat || 'VAC' }), h('dt', { text: 'Último jogo' }), h('dd', { text: fmtDT(info.game.lastPlayed) }),
              v ? [h('dt', { text: 'Configuração de vídeo' }), h('dd', { class: 'mono small', text: v.path + ' (' + fmtDate(v.modified) + ')' })] : null)
            : h('div', { class: 'callout warn' }, icon('alert'), h('div', { text: 'O CS2 não foi encontrado nas bibliotecas da Steam deste PC. Se ele estiver instalado em outro lugar, adicione o cs2.exe na Biblioteca de jogos.' }))),
        h('div', { class: 'card' }, h('h2', null, icon('play'), 'Opções de inicialização (Steam)'),
          h('div', { class: 'path', text: info.launchOptions || '(nenhuma)' }),
          (info.launchNotes || []).length ? h('div', { class: 'stack', style: { marginTop: '12px' } }, info.launchNotes.map(n => h('div', { class: 'row', style: { alignItems: 'flex-start' } }, badge(n.option, n.level === 'warning' ? 'warn' : 'info'), h('div', { class: 'small', text: n.text }))))
            : h('div', { class: 'small muted', style: { marginTop: '10px' }, text: 'Nenhuma opção problemática encontrada.' }),
          h('div', { class: 'small faint', style: { marginTop: '12px' }, text: 'Para mudar: Steam → Biblioteca → Counter-Strike 2 → Propriedades → Geral → Opções de inicialização.' }))),
      h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('monitor'), 'Suas configurações de vídeo no CS2'),
        v ? h('div', { class: 'checks' }, (g.settings || []).map(c => checkItem(c)))
          : h('div', { class: 'muted', text: 'Arquivo de vídeo do CS2 não encontrado (cs2_video.txt). Abra o jogo uma vez e reanalise.' })),
      g.preset ? presetCard(g.preset) : null,
      h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('sliders'), 'Perfil do cs2.exe no Windows'),
        game && game.options && game.options.length ? game.options.map(t => tweakRow(t)) : h('div', { class: 'muted', text: 'Disponível quando o CS2 for encontrado.' })),
      h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('check'), 'Checklist com evidência'),
        h('div', { class: 'checks' }, (g.tips || []).map(c => checkItem(c)))));
  }

  async function pageGames() {
    const d = await get('games', '/api/games');
    const list = d.games || [];
    // The "GPU dedicada" switch only works with two GPUs: say why it is off
    // once, instead of only in a tooltip.
    const gpuOpts = list.map(gm => (gm.options || []).find(t => t.id.split(':').pop() === 'gpu')).filter(Boolean);
    const gpuNA = gpuOpts.length && gpuOpts.every(t => !t.state.applicable) ? (gpuOpts.map(t => t.state.naReason || '').find(r => /GPU/.test(r)) || '') : '';
    const gpuNote = gpuNA ? ' A opção "GPU dedicada" fica indisponível porque ' + gpuNA.replace(/\.$/, '') + ': ela só serve para PCs com vídeo integrado e dedicado.' : '';
    const initials = n => (n || '?').split(/\s+/).filter(Boolean).slice(0, 2).map(w => w[0]).join('').toUpperCase();
    async function addGame() {
      try {
        const r = await api('POST', '/api/games/add', { pick: true });
        if (r && r.cancelled) return;
        toast('Jogo adicionado.', 'ok');
        invalidate(); rerender();
      } catch (e) { toast(e.message, 'bad'); }
    }
    return h('div', { class: 'page' },
      pageHead('Biblioteca de jogos', 'Jogos encontrados na Steam, Epic, Riot, Ubisoft, Battle.net, EA app e Xbox. Cada jogo pode ter um perfil próprio no Windows.',
        [btn('Adicionar jogo', addGame, '', 'plus'), btn('Procurar novamente', async () => {
          try { await api('POST', '/api/games/rescan', {}); toast('Bibliotecas verificadas.', 'ok'); } catch (e) { toast(e.message, 'bad'); }
          invalidate(); rerender();
        }, 'ghost', 'refresh')]),
      h('div', { class: 'callout', style: { marginBottom: '16px' } }, icon('info'),
        h('div', { text: 'Os perfis alteram só como o Windows trata aquele executável (prioridade de CPU, GPU usada e modo de tela cheia). Não modificam arquivos do jogo. Prioridade alta só ajuda quando outro programa disputa a CPU.' + gpuNote })),
      d.unused && d.unused.length ? h('div', { class: 'card', style: { marginBottom: '16px' } },
        h('div', { class: 'row wrap' }, h('h2', { style: { margin: 0 } }, icon('trash'), 'Jogos sem uso'), badge(d.unused.length + ' ' + (d.unused.length === 1 ? 'encontrado' : 'encontrados'), 'warn')),
        h('div', { class: 'sub', text: 'Jogos da Steam instalados, mas que ninguém abre há mais de ' + (d.unusedDays || 180) + ' dias. Considere desinstalar para liberar espaço.' }),
        h('div', { class: 'set-list', style: { marginTop: '12px' } },
          d.unused.map(u => h('div', { class: 'set-row' },
            h('span', null, h('b', null, u.name), h('span', { class: 'small muted', text: ' — ' + (u.never ? 'sem registro de uso na Steam; atualizado há ' + plural(u.days, 'dia', 'dias') : 'jogado pela última vez há ' + plural(u.days, 'dia', 'dias')) })),
            h('span', { class: 'row' },
              h('span', { text: sizeLabel(u.sizeBytes) }),
              u.launcher === 'Steam' && u.appId ? btn('Desinstalar', () => openURL('steam://uninstall/' + u.appId), 'sm ghost', 'trash') : null
            ))))) : null,
      list.length ? h('div', { class: 'game-grid' }, list.map(gm => {
        const opts = (gm.options || []).map(t => {
          const s = t.state;
          const short = { priority: 'Prioridade alta', gpu: 'GPU dedicada', fso: 'Tela cheia clássica' }[t.id.split(':').pop()] || t.title;
          return h('div', { class: 'gopt' + (s.applicable ? '' : ' na'), title: s.applicable ? t.summary : (s.naReason || '') },
            toggle(s.status === 'on', (want, input) => onToggle(t, want, input), !s.applicable || (s.status === 'on' && !s.byAxion), s.applicable ? t.title : s.naReason, short + ' — ' + gm.name), h('span', { text: short }));
        });
        return h('div', { class: 'gcard' + (gm.competitive ? ' comp' : '') },
          h('div', { class: 'gi', text: initials(gm.name) }),
          h('div', { style: { minWidth: 0 } }, h('div', { class: 'row wrap' }, h('span', { class: 'gn', text: gm.name }), badge(gm.launcher, 'outline'), gm.competitive ? badge('competitivo', 'game') : null, gm.antiCheat ? badge(gm.antiCheat, 'game', 'shield') : null,
            gm.launcher === 'Manual' ? h('button', { class: 'linkbtn small', type: 'button', text: 'remover', 'aria-label': 'Remover ' + gm.name + ' da biblioteca', onclick: async () => {
              try { await api('POST', '/api/games/remove', { exe: gm.exe }); } catch (e) { toast(e.message, 'bad'); }
              invalidate(); rerender();
            } }) : null),
            h('div', { class: 'gp', text: gm.exe || 'executável não identificado — use "Adicionar jogo"' }),
            (gm.exeName || '').toLowerCase() === 'cs2.exe' ? h('div', { class: 'mt-2' }, btn('Abrir perfil do CS2', () => go('cs2'), 'game sm', 'crosshair')) : null),
          h('div', { class: 'gopts' }, opts));
      })) : empty('gamepad', 'Nenhum jogo encontrado', 'Use "Adicionar jogo" para escolher o executável manualmente.'));
  }

  // Peripherals: inventory, polling test and input settings.
  const VERDICT = { ok: ['estável', 'ok'], low: ['abaixo do máximo', 'warn'], irregular: ['irregular', 'warn'], little: ['poucos dados', 'outline'] };
  const fmtUs = v => v >= 1000 ? (v / 1000).toFixed(2).replace('.', ',') + ' ms' : Math.round(v) + ' µs';
  const fmtHz = v => v >= 100 ? Math.round(v) + ' Hz' : (Math.round(v * 10) / 10).toString().replace('.', ',') + ' Hz';

  function statsBlock(st, label) {
    if (!st) return null;
    const bins = (st.hist || []).filter(b => b.n > 0);
    return h('div', { class: 'mstat' },
      h('div', { class: 'mstat-h', text: label }),
      h('dl', { class: 'kv tight' },
        h('dt', { text: 'Taxa média' }), h('dd', { text: fmtHz(st.rateHz) }),
        h('dt', { text: 'Intervalo mediano' }), h('dd', { text: fmtUs(st.medianUs) }),
        h('dt', { text: '1% mais lentos' }), h('dd', { text: 'acima de ' + fmtUs(st.p99Us) }),
        h('dt', { text: 'Variação (desvio)' }), h('dd', { text: fmtUs(st.stdDevUs) }),
        h('dt', { text: 'No ritmo esperado' }), h('dd', { text: Math.round(st.onTimePct) + '% de ' + st.samples.toLocaleString('pt-BR') }),
        st.pauses ? [h('dt', { text: 'Pausas' }), h('dd', { text: st.pauses + ' (' + Math.round(st.pauseMs) + ' ms)' })] : null),
      bins.length ? h('div', { class: 'hbars' }, bins.map(b => h('div', { class: 'hbar' },
        h('span', { class: 'hl', text: b.label }), h('span', { class: 'ht' }, h('i', { style: { width: Math.max(1.5, b.pct) + '%' } })), h('span', { class: 'hp', text: (b.pct < 1 ? b.pct.toFixed(1).replace('.', ',') : Math.round(b.pct)) + '%' })))) : null);
  }

  function measuredBlock(r) {
    const v = VERDICT[r.verdict] || [r.verdict, 'outline'];
    const hz = r.snapped || Math.round(r.measuredHz || 0);
    return h('div', { class: 'measured' },
      h('div', { class: 'mhead' },
        h('div', { class: 'rate-big' }, hz ? String(hz) : '—', h('span', { text: ' Hz' })),
        h('div', null, badge(v[0], v[1]), h('div', { class: 'small muted', text: r.source === 'usb' ? 'medido no controlador USB' : 'medido na entrega ao aplicativo' }))),
      h('div', { class: 'small', text: r.summary }),
      h('div', { class: 'grid g2', style: { marginTop: '10px', gap: '10px' } },
        statsBlock(r.usb, 'Controlador USB (ETW)'), statsBlock(r.raw, 'Entrega ao aplicativo (raw input)')),
      (r.notes || []).length ? h('ul', { class: 'notes' }, r.notes.map(n => h('li', { text: n }))) : null);
  }

  // Finding inside a device card: evidence visible, explanation on demand,
  // and no link back to this same page.
  function deviceFinding(x) {
    const sev = SEV[x.severity] || SEV.info;
    const acts = (x.actions || []).filter(a => !(a.kind === 'page' && a.page === 'perifericos')).map(actionButton).filter(Boolean);
    return h('div', { class: 'finding' },
      h('div', { class: 'sev ' + x.severity, title: sev.label }, icon(sev.ic)),
      h('div', null, h('div', { class: 'ft', text: x.title }),
        x.evidence ? h('div', { class: 'fe', text: x.evidence }) : null,
        x.explanation ? h('details', { class: 'why' }, h('summary', null, icon('chevron'), 'Por que isso importa'),
          h('div', { class: 'fx', text: x.explanation }),
          (x.sources || []).length ? h('div', { class: 'fa' }, x.sources.map(s => extLink(s.title, s.url))) : null) : null,
        acts.length ? h('div', { class: 'fa' }, acts) : null));
  }

  function deviceCard(d) {
    const vp = d.vid ? (d.vid.toString(16).padStart(4, '0') + ':' + d.pid.toString(16).padStart(4, '0')).toUpperCase() : '';
    const rows = [['Conexão', d.connection], ['Polling máximo da porta', d.rateLabel], ['Porta', d.portLabel], ['Energia', d.powerLabel]].filter(r => r[1]);
    return h('div', { class: 'card device' },
      h('div', { class: 'dev-head' },
        h('div', { class: 'dev-ic' + (d.primary === 'mouse' ? '' : ' kb') }, icon(d.primary === 'mouse' ? 'mouse' : 'keyboard')),
        h('div', { style: { minWidth: 0 } }, h('div', { class: 'dev-name ellipsis', text: d.name, title: d.name }), h('div', { class: 'small muted', text: d.role + (vp ? ' · ' + vp : '') }))),
      h('dl', { class: 'kv' }, rows.map(([k, v]) => [h('dt', { text: k }), h('dd', { text: v })])),
      d.measured ? measuredBlock(d.measured) : null,
      (d.findings || []).length ? h('div', { class: 'dev-findings' }, d.findings.map(deviceFinding)) : null);
  }

  async function pagePerif() {
    const [v, tws] = await Promise.all([get('peripherals', '/api/peripherals'), tweaksById()]);
    const m = v.measurement;
    const secs = h('select', { class: 'select', 'aria-label': 'Duração do teste' },
      [4, 6, 10].map(n => h('option', { value: String(n), selected: n === 6 ? 'selected' : null, text: n + ' segundos' })));
    const usb = h('input', { type: 'checkbox', checked: v.traceAvailable ? 'checked' : null, disabled: v.traceAvailable ? null : 'disabled' });
    const measure = async () => {
      await runJob(api('POST', '/api/peripherals/measure', { seconds: Number(secs.value), usbTrace: usb.checked }), { title: 'Medindo o polling do mouse' });
    };
    const settings = v.settings || {};
    const setRow = (label, value, good) => h('div', { class: 'set-row' }, h('span', { text: label }), badge(value, good === true ? 'ok' : good === false ? 'warn' : 'outline'));
    const q = n => n < 0 ? 'padrão (100)' : String(n);
    // Corrections only appear when the problem exists (or Axion applied them).
    const tweakList = (v.tweaks || []).map(id => tws.get(id)).filter(t => {
      if (!t) return false;
      const st = t.state || {};
      return !t.id.startsWith('fix.') || st.applicable || st.byAxion;
    });
    return h('div', { class: 'page' },
      pageHead('Periféricos', 'Mouse e teclado: como cada um está ligado ao PC, qual polling a porta permite, medição real e os ajustes do Windows que afetam a entrada.',
        [btn('Medir polling do mouse', measure, 'primary', 'gauge')]),
      h('div', { class: 'card measure-card' },
        h('h2', null, icon('gauge'), 'Teste de polling'),
        h('div', { class: 'sub', text: 'Abre uma janela de teste em primeiro plano. Mova o mouse sem parar, em círculos rápidos, até a contagem terminar. Esc cancela.' }),
        h('div', { class: 'row wrap' }, secs,
          h('label', { class: 'check-label', title: v.traceAvailable ? '' : 'Disponível ao executar como administrador' }, usb, ' Medir também no controlador USB (ETW, mais preciso)'),
          h('div', { class: 'spacer' }), btn('Medir agora', measure, 'primary', 'play')),
        m ? h('div', { class: 'small muted', style: { marginTop: '10px' }, text: 'Última medição: ' + fmtDT(m.at) + ' · ' + Math.round(m.seconds) + ' s' + (m.usbTraced ? ' · com rastreamento USB' : ' · só raw input') }) : null,
        m && (m.notes || []).length ? h('ul', { class: 'notes' }, m.notes.map(n => h('li', { text: n }))) : null,
        m && !(m.devices || []).length ? h('div', { class: 'callout warn', style: { marginTop: '10px' } }, icon('alert'), h('div', { text: 'A última medição não registrou movimento.' })) : null),
      (v.devices || []).length
        ? h('div', { class: 'grid g2', style: { marginTop: '16px' } }, v.devices.map(deviceCard))
        : h('div', { class: 'card', style: { marginTop: '16px' } }, empty('mouse', 'Nenhum mouse ou teclado encontrado', 'Conecte os periféricos e reanalise o PC.')),
      (v.findings || []).length ? h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('alert'), 'Configurações de entrada com problema'), h('div', { class: 'stack' }, v.findings.map(x => findingCard(x)))) : null,
      h('div', { class: 'grid g2', style: { marginTop: '16px' } },
        h('div', { class: 'card' }, h('h2', null, icon('sliders'), 'Configurações de entrada do Windows'),
          v.scanned ? h('div', { class: 'set-list' },
            setRow('Aprimorar precisão do ponteiro', settings.mouseAccelOn ? 'ligado' : 'desligado', !settings.mouseAccelOn),
            setRow('Atalho das Teclas de Aderência (Shift 5x)', settings.stickyHotkey ? 'ativo' : 'desligado', !settings.stickyHotkey),
            setRow('Teclas de Filtro', settings.filterKeysOn ? 'ligadas' : 'desligadas', !settings.filterKeysOn),
            setRow('Fila do driver do mouse', q(settings.mouseQueue), settings.mouseQueue < 0 || settings.mouseQueue >= 100),
            setRow('Fila do driver do teclado', q(settings.keyboardQueue), settings.keyboardQueue < 0 || settings.keyboardQueue >= 100),
            setRow('DPCs em thread', settings.threadDpc === 0 ? 'desligados' : 'padrão', settings.threadDpc !== 0),
            setRow('Suspensão seletiva USB (plano, na tomada)', settings.usbSuspendPlan === 0 ? 'desligada' : settings.usbSuspendPlan === 1 ? 'ligada' : 'desconhecida', settings.usbSuspendPlan === 1 ? null : settings.usbSuspendPlan === 0 ? true : null))
            : h('div', { class: 'muted', text: 'Analise o PC para ver as configurações.' })),
        h('div', { class: 'card' }, h('h2', null, icon('info'), 'O que dá e o que não dá para ajustar'), h('div', { class: 'small', text: v.honest }),
          h('div', { class: 'row wrap', style: { marginTop: '10px' } }, (v.sources || []).map(x => extLink(x.title, x.url))))),
      tweakList.length ? h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('zap'), 'Ajustes de entrada'), tweakList.map(t => tweakRow(t))) : null,
      h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('check'), 'Checklist com evidência'), h('div', { class: 'checks' }, (v.tips || []).map(c => checkItem(c)))));
  }

  // ---------------------------------------------------------------------------
  // Drivers
  // ---------------------------------------------------------------------------

  const drvSel = new Set();
  let drvSelStamp = null;
  const DVERDICT = { recomendado: ['recomendado', 'accent'], avaliar: ['avaliar', 'warn'], nao: ['não recomendado', 'bad'], instalado: ['já instalado', 'ok'] };
  const DSTATUS = { baixado: ['baixado', 'info', 'download'], instalado: ['instalado', 'ok', 'check'], falhou: ['falhou', 'bad', 'alertc'] };
  const hasDate = d => d && !String(d).startsWith('0001');

  async function pageDrivers() {
    const [v, oldRes] = await Promise.all([get('drivers', '/api/drivers'), get('drivers_old', '/api/drivers/old')]);
    const searched = hasDate(v.searchedAt);
    if (drvSelStamp !== (v.searchedAt || '')) {
      drvSel.clear();
      (v.offers || []).filter(o => o.select && o.status !== 'instalado').forEach(o => drvSel.add(o.updateId));
      drvSelStamp = v.searchedAt || '';
    }
    const scan = () => runJob(api('POST', '/api/drivers/scan', {}), { title: 'Procurando drivers' });
    return h('div', { class: 'page' },
      pageHead('Drivers', 'O que está instalado em cada componente e o que há de mais novo. Nada é baixado nem instalado sem você escolher.',
        [btn(searched ? 'Procurar de novo' : 'Procurar atualizações', scan, 'primary', 'refresh')]),
      v.reboot ? h('div', { class: 'callout warn' }, icon('refresh'), h('div', { text: 'Reinicie o PC para concluir a instalação dos drivers.' })) : null,
      !v.elevated ? h('div', { class: 'callout warn' }, icon('lock'), h('div', { text: 'Sem privilégios de administrador: dá para procurar, mas baixar, instalar e ocultar exigem executar o Axion como administrador.' })) : null,
      v.gpu ? gpuDriverCard(v.gpu) : null,
      driverOffersCard(v, searched, scan),
      driverComponentsCard(v),
      oldRes && oldRes.packages && oldRes.packages.length ? oldDriverPackagesCard(oldRes) : null,
      h('div', { class: 'grid g2', style: { marginTop: '16px' } }, boardCard(v.board),
        h('div', { class: 'card' }, h('h2', null, icon('info'), 'Como funciona'), h('ul', { class: 'notes' }, (v.notes || []).map(n => h('li', { text: n }))),
          h('div', { class: 'src', style: { marginTop: '8px' } }, (v.sources || []).map(x => extLink(x.title, x.url))))));
  }

  function gpuDriverCard(g) {
    const lat = g.latest ? Object.assign({}, g.latest, { sizeLabel: (g.latest.sizeLabel || '').replace('.', ',') }) : null;
    const nvidia = g.vendor === 'NVIDIA';
    let status = null;
    if (nvidia && g.error) status = h('div', { class: 'callout warn', style: { marginTop: '10px' } }, icon('alert'), h('div', { text: 'Não foi possível consultar a NVIDIA: ' + g.error }));
    else if (nvidia && !lat) status = h('p', { class: 'small muted', text: 'Clique em "Procurar atualizações" para comparar com o driver mais recente da NVIDIA.' });
    const acts = [];
    if (nvidia && lat && !g.upToDate) {
      if (g.file) {
        acts.push(btn('Instalar ' + lat.version, async () => {
          const ok = await confirmDlg({ title: 'Instalar o driver NVIDIA ' + lat.version, ok: 'Abrir o instalador', icon: 'play',
            body: h('div', null, h('p', { text: 'O instalador oficial da NVIDIA será aberto (assinatura verificada: ' + g.signer + '). Siga as telas dele; a tela pode piscar durante a instalação.' }),
              h('p', { class: 'small muted', text: 'A opção "instalação limpa" do instalador volta as configurações do painel da NVIDIA ao padrão; use se tiver problemas com o driver atual. Feche jogos antes de começar.' })) });
          if (!ok) return;
          try { await api('POST', '/api/drivers/gpu/install', {}); toast('Instalador da NVIDIA aberto.', 'ok'); } catch (e) { toast(e.message, 'bad'); }
          invalidate(); rerender();
        }, 'primary', 'play'));
        acts.push(btn('Apagar instalador', async () => { try { await api('POST', '/api/drivers/gpu/delete', {}); } catch (e) { toast(e.message, 'bad'); } invalidate(); rerender(); }, 'ghost', 'trash'));
      } else {
        acts.push(btn('Baixar ' + lat.version + (lat.sizeLabel ? ' (' + lat.sizeLabel + ')' : ''), () => runJob(api('POST', '/api/drivers/gpu/download', {}), { title: 'Baixando o driver NVIDIA ' + lat.version }), 'primary', 'download'));
      }
    }
    return h('div', { class: 'card', style: { marginTop: '4px' } },
      h('div', { class: 'row wrap' }, h('h2', { style: { margin: 0 } }, icon('monitor'), 'Placa de vídeo'), h('div', { class: 'spacer' }),
        nvidia && lat ? badge(g.upToDate ? 'driver em dia' : 'driver novo disponível', g.upToDate ? 'ok' : 'warn', g.upToDate ? 'check' : 'download') : null),
      h('div', { class: 'sub', text: g.name }),
      h('div', { class: 'set-list', style: { marginTop: '8px' } },
        h('div', { class: 'set-row' }, h('span', { text: 'Instalado' }), h('b', { text: (g.installedVersion || '—') + (hasDate(g.installedDate) ? ' · ' + fmtDay(g.installedDate) : '') })),
        lat ? h('div', { class: 'set-row' }, h('span', { text: 'Mais recente da NVIDIA' }),
          h('b', { text: lat.version + ' · ' + (lat.name || 'Game Ready') + (hasDate(lat.released) ? ' · ' + fmtDay(lat.released) : '') + (lat.sizeLabel ? ' · ' + lat.sizeLabel : '') })) : null,
        g.file ? h('div', { class: 'set-row' }, h('span', { text: 'Instalador baixado' }), badge('assinatura verificada: ' + g.signer, 'ok', 'shield')) : null),
      status,
      acts.length ? h('div', { class: 'row wrap', style: { marginTop: '12px' } }, acts) : null,
      h('div', { class: 'row wrap', style: { marginTop: '10px' } },
        lat && lat.detailsUrl ? extLink('Notas da versão ' + lat.version, lat.detailsUrl) : null,
        (g.links || []).map(l => extLink(l.label, l.url))));
  }

  function driverOffersCard(v, searched, scan) {
    const offers = v.offers || [];
    const hidden = v.hidden || [];
    const sel = offers.filter(o => drvSel.has(o.updateId) && o.status !== 'instalado');
    const toDownload = sel.filter(o => !o.downloaded);
    const toInstall = sel.filter(o => o.downloaded);
    const dlSize = toDownload.reduce((n, o) => n + (o.sizeBytes || 0), 0);
    const hide = async (o, hid) => {
      try { await api('POST', '/api/drivers/hide', { id: o.updateId, hidden: hid }); toast(hid ? 'Atualização ocultada: nem o Windows nem o Axion vão oferecê-la de novo.' : 'Atualização visível de novo.', 'ok'); } catch (e) { toast(e.message, 'bad'); }
      drvSel.delete(o.updateId); invalidate(); rerender();
    };
    const row = o => {
      const st = DSTATUS[o.status];
      const input = h('input', { type: 'checkbox', checked: drvSel.has(o.updateId) && o.status !== 'instalado' ? 'checked' : null, disabled: o.status === 'instalado' ? 'disabled' : null, 'aria-label': 'Selecionar ' + o.title });
      input.addEventListener('change', () => { if (input.checked) drvSel.add(o.updateId); else drvSel.delete(o.updateId); rerender(); });
      const vd = DVERDICT[o.verdict] || [o.verdict, 'outline'];
      const from = o.installedVersion ? o.installedVersion + (hasDate(o.installedDate) ? ' (' + fmtDay(o.installedDate) + ')' : '') : 'sem driver do fabricante';
      const to = (o.version || '?') + (hasDate(o.verDate) ? ' (' + fmtDay(o.verDate) + ')' : '');
      return h('div', { class: 'drv-offer' + (o.status === 'instalado' ? ' done' : '') },
        h('label', { class: 'drv-check' }, input),
        h('div', { class: 'drv-main' },
          h('div', { class: 'row wrap', style: { gap: '6px' } }, h('b', { text: o.device || o.title }), badge(o.componentLabel || o.classLabel, 'outline'),
            st ? badge(st[0], st[1], st[2]) : badge(vd[0], vd[1]), o.browseOnly || o.verdict === 'instalado' ? null : badge('automática', 'outline')),
          h('div', { class: 'small', text: o.title }),
          h('div', { class: 'small muted', text: o.verdict === 'instalado' ? 'Em uso: ' + from : 'Instalado: ' + from + ' → oferecido: ' + to + (o.sizeBytes ? ' · ' + sizeLabel(o.sizeBytes) : '') }),
          h('ul', { class: 'notes small' }, (o.reasons || []).map(r => h('li', { text: r }))),
          o.statusDetail ? h('div', { class: 'small ' + (o.status === 'falhou' ? 'bad-text' : 'muted'), text: o.statusDetail }) : null),
        h('div', { class: 'drv-side' }, o.status !== 'instalado' ? h('button', { class: 'linkbtn small', type: 'button', text: 'Ocultar', title: 'Não oferecer mais esta atualização', onclick: () => hide(o, true) }) : null));
    };
    let body;
    if (!searched) body = empty('download', 'Ainda não procurou', 'A busca consulta o Windows Update (drivers certificados para o seu hardware) e, se a placa de vídeo for NVIDIA, o site da NVIDIA.', btn('Procurar atualizações', scan, 'primary', 'refresh'));
    else if (v.searchError && !offers.length) body = h('div', { class: 'callout warn' }, icon('alert'), h('div', { text: 'A busca no Windows Update falhou: ' + v.searchError }));
    else if (!offers.length) body = h('div', { class: 'callout' }, icon('check'), h('div', { text: 'O Windows Update não tem drivers novos para este PC.' }));
    else body = h('div', null, offers.map(row),
      h('div', { class: 'row wrap', style: { marginTop: '12px' } },
        btn(toDownload.length ? 'Baixar ' + plural(toDownload.length, 'selecionada', 'selecionadas') + (dlSize ? ' (' + sizeLabel(dlSize) + ')' : '') : 'Baixar selecionadas',
          () => runJob(api('POST', '/api/drivers/download', { ids: toDownload.map(o => o.updateId) }), { title: 'Baixando drivers' }), 'primary', 'download', { disabled: toDownload.length ? null : 'disabled' }),
        btn(toInstall.length ? 'Instalar ' + plural(toInstall.length, 'baixada', 'baixadas') : 'Instalar baixadas', async () => {
          const ok = await confirmDlg({ title: 'Instalar ' + plural(toInstall.length, 'driver', 'drivers'), ok: 'Instalar', icon: 'check',
            body: h('div', null, h('ul', { class: 'notes' }, toInstall.map(o => h('li', { text: (o.device || o.title) + ' → ' + (o.version || o.title) }))),
              h('p', { class: 'small muted', text: 'O Axion cria um ponto de restauração antes. A tela pode piscar e a rede pode cair por alguns segundos enquanto o driver troca.' })) });
          if (ok) runJob(api('POST', '/api/drivers/install', { ids: toInstall.map(o => o.updateId) }), { title: 'Instalando drivers' });
        }, '', 'check', { disabled: toInstall.length ? null : 'disabled' }),
        h('div', { class: 'spacer' }), h('span', { class: 'small muted', text: 'Primeiro baixe; depois escolha o que instalar.' })));
    return h('div', { class: 'card', style: { marginTop: '16px' } },
      h('div', { class: 'row wrap' }, h('h2', { style: { margin: 0 } }, icon('download'), 'Atualizações do Windows Update'), h('div', { class: 'spacer' }),
        searched ? h('span', { class: 'small muted', text: 'Última busca: ' + fmtDT(v.searchedAt) }) : null),
      h('div', { class: 'sub', text: 'Drivers que a Microsoft distribui para o seu hardware (os mesmos de Configurações > Windows Update > Atualizações opcionais), com a avaliação do Axion para cada um.' }),
      body,
      hidden.length ? h('details', { style: { marginTop: '12px' } }, h('summary', { class: 'small linkbtn', text: 'Ocultas (' + hidden.length + ')' }),
        hidden.map(o => h('div', { class: 'set-row' }, h('span', { text: (o.device || o.title) + ' · ' + (o.version || '') }),
          h('button', { class: 'linkbtn small', type: 'button', text: 'Mostrar de novo', onclick: () => hide(o, false) })))) : null);
  }

  function oldDriverPackagesCard(oldRes) {
    const n = oldRes.packages.length;
    return h('div', { class: 'card', style: { marginTop: '16px' } },
      h('div', { class: 'row wrap' }, h('h2', { style: { margin: 0 } }, icon('trash'), 'Pacotes de vídeo antigos'), badge(plural(n, 'encontrado', 'encontrados'), 'warn')),
      h('div', { class: 'sub', text: 'Versões de driver de vídeo que uma atualização deixou no Repositório de Drivers do Windows e que não estão instaladas em nenhum dispositivo. Excluir pode liberar espaço (o Axion não mede quanto).' }),
      h('div', { class: 'set-list', style: { marginTop: '12px' } },
        oldRes.packages.map(p => h('div', { class: 'set-row' },
          h('span', null, h('b', null, p.provider), h('span', { class: 'small muted', text: ' — versão ' + p.version + ' substituída pela ' + p.newest })),
          h('span', null, p.published)
        ))
      ),
      h('div', { style: { marginTop: '12px' } },
        btn('Excluir ' + plural(n, 'pacote', 'pacotes'), async () => {
          // Confirm on today's list, not on the copy kept since the page was drawn.
          let cur;
          try { cur = await get('drivers_old', '/api/drivers/old', true); } catch (e) { toast(e.message, 'bad'); return; }
          const list = (cur && cur.packages) || [];
          if (!list.length) { toast('Não há mais pacotes antigos para excluir.', 'info'); rerender(); return; }
          const ok = await confirmDlg({ title: 'Excluir pacotes antigos', ok: 'Excluir', icon: 'trash', danger: true,
            body: h('div', null,
              h('p', { text: plural(list.length, 'pacote será excluído', 'pacotes serão excluídos') + ' do Repositório de Drivers do Windows:' }),
              h('ul', null, list.map(p => h('li', { text: p.published + ' — ' + p.provider + ', versão ' + p.version }))),
              h('p', { class: 'small muted', text: 'O Axion não consegue desfazer isto, e o botão "Reverter driver" do Gerenciador de Dispositivos deixa de voltar a essas versões. O Windows recusa excluir um pacote que algum dispositivo esteja usando.' })) });
          if (!ok) return;
          runJob(api('POST', '/api/drivers/old/delete', { published: list.map(p => p.published) }), { title: 'Removendo pacotes de vídeo antigos' });
        }, '', 'trash')
      )
    );
  }

  function driverComponentsCard(v) {
    const comps = v.components || [];
    return h('div', { class: 'card', style: { marginTop: '16px' } },
      h('h2', null, icon('chip'), 'Drivers instalados por componente'),
      v.inventoryError ? h('div', { class: 'callout warn' }, icon('alert'), h('div', { text: v.inventoryError })) : null,
      comps.length ? comps.map(c => h('div', { class: 'drv-comp' }, h('h3', { text: c.label }),
        h('div', { class: 'tbl-scroll' }, h('table', { class: 'table' }, h('thead', null, h('tr', null, ['Dispositivo', 'Driver de', 'Versão', 'Data'].map(x => h('th', { text: x })))),
          h('tbody', null, c.devices.map(d => h('tr', null, h('td', null, d.name, d.generic ? h('span', { class: 'small muted', text: ' (genérico do Windows)' }) : null),
            h('td', { text: d.provider || '—' }), h('td', { text: d.version || '—' }), h('td', { text: hasDate(d.date) ? fmtDay(d.date) : '—' })))))))) :
        h('p', { class: 'small muted', text: 'Nenhum driver listado.' }));
  }

  function boardCard(b) {
    b = b || {};
    return h('div', { class: 'card' }, h('h2', null, icon('chip'), 'Placa-mãe, chipset e BIOS'),
      h('div', { class: 'set-list' },
        h('div', { class: 'set-row' }, h('span', { text: 'Placa-mãe' }), h('b', { text: ((b.maker || '') + ' ' + (b.model || '')).trim() || '—' })),
        h('div', { class: 'set-row' }, h('span', { text: 'BIOS' }), h('b', { text: (b.biosVersion || '—') + (hasDate(b.biosDate) ? ' · ' + fmtDay(b.biosDate) : '') }))),
      h('p', { class: 'small muted', text: 'O Windows Update não atualiza a BIOS de placas-mãe de desktop. Atualize quando a nova versão trouxer correções que te afetam (memória EXPO/XMP, segurança, estabilidade), seguindo o manual e sem desligar o PC durante o processo.' }),
      h('div', { class: 'row wrap' }, b.support ? extLink(b.support.label, b.support.url) : null, b.chipset ? extLink(b.chipset.label, b.chipset.url) : null));
  }

  // ---------------------------------------------------------------------------
  // Limpeza
  // ---------------------------------------------------------------------------

  // ---------------------------------------------------------------------------
  // Desempenho (PresentMon)
  // ---------------------------------------------------------------------------

  const LIMITER = {
    gpu: ['placa de vídeo (GPU)', 'info'], cpu: ['CPU ou limitador do jogo', 'warn'],
    limite: ['limite de FPS ou sincronização', 'outline'], misto: ['misto', 'outline'],
  };
  const MODE_NOTE = {
    'Hardware: Independent Flip': ['ok', 'Direto para a tela, com a latência da tela cheia exclusiva.'],
    'Hardware Composed: Independent Flip': ['ok', 'Direto para a tela por um plano de sobreposição da GPU.'],
    'Hardware: Legacy Flip': ['ok', 'Tela cheia exclusiva.'],
    'Hardware: Legacy Copy to front buffer': ['info', 'Tela cheia antiga, copiando cada quadro.'],
    'Composed: Flip': ['action', 'O Windows compõe a imagem (DWM) antes de mostrar: mais latência. Use tela cheia e feche sobreposições.'],
    'Composed: Copy with GPU GDI': ['action', 'Janela composta por cópia: o modo com mais latência.'],
    'Composed: Copy with CPU GDI': ['action', 'Janela composta por cópia: o modo com mais latência.'],
  };
  const perfSel = { a: new Set(), b: new Set() };
  const perfForm = { process: '', other: '', seconds: 60, mode: 'hotkey', hotkey: 'SCROLLLOCK', delay: 10, label: 'antes' };
  let perfOpen = null;
  let perfCmp = null;
  // Fixed decimals in pt-BR (1.234,5), cached per number of decimals.
  const NF = {};
  const numPT = (v, d) => (NF[d] || (NF[d] = new Intl.NumberFormat('pt-BR', { minimumFractionDigits: d, maximumFractionDigits: d }))).format(v);
  const fmtN = (v, d) => v == null || v < 0 ? '—' : numPT(Number(v), d == null ? 1 : d);
  const fmtSigned = (v, d) => v == null ? '—' : (v > 0 ? '+' : v < 0 ? '−' : '') + numPT(Math.abs(v), d == null ? 1 : d);
  const hotkeyLabel = k => k === 'SCROLLLOCK' ? 'Scroll Lock' : k === 'PAUSE' ? 'Pause' : k.split('+').map(p => ({ ALT: 'Alt', SHIFT: 'Shift', CTRL: 'Ctrl' })[p] || p).join('+');

  // "Prova no seu PC": estado do formulário e leitura do resultado.
  const trialForm = { tweak: '', process: '', seconds: 60, runs: 3, mode: 'hotkey', hotkey: 'SCROLLLOCK', delay: 10 };
  let trialLast = null; // resultado da última prova desta sessão
  const TRIAL_VERDICT = { melhor: ['melhorou', 'ok'], igual: ['sem diferença detectável', 'outline'], pior: ['piorou', 'bad'], misto: ['resultado misto', 'warn'], inconclusivo: ['inconclusivo', 'outline'] };
  const TRIAL_ACTION = { mantido: 'mantido', 'desfeito-auto': 'desfeito automaticamente', desfeito: 'desfeito por você', 'falha-desfazer': 'não foi possível desfazer' };
  const pLabel = p => p == null || p < 0 ? 'p ajustado —' : p < 0.001 ? 'p ajustado < 0,001' : 'p ajustado = ' + fmtN(p, 3);
  function trialLine(o) {
    const ms = o.metrics || [];
    return [['low1Fps', '1% low'], ['avgFps', 'FPS médio'], ['ftP99', 'P99 (menor é melhor)']]
      .map(([k, l]) => { const m = ms.find(x => x.key === k); return m ? l + ' ' + fmtSigned(m.deltaPct, 1) + '% (' + pLabel(m.pAdj) + ')' : null; })
      .filter(Boolean).join(' · ');
  }
  function trialBadge(o) { const v = TRIAL_VERDICT[o.verdict] || [o.verdict || '—', 'outline']; return badge(v[0], v[1], o.verdict === 'melhor' ? 'check' : null); }
  async function undoTrial(o) { return runJob(api('POST', '/api/trial/undo', { trial: o.id }), { title: 'Desfazendo: ' + o.tweakTitle }); }
  function trialResultView(r, withHead) {
    const o = r.outcome || {};
    const cancelled = !o.verdict;
    const failed = o.action === 'falha-desfazer';
    const text = cancelled
      ? (!r.applied ? 'A prova parou antes de aplicar o ajuste: nada foi alterado no PC.' : 'A prova parou e o ajuste foi desfeito pelo histórico: o PC voltou ao estado de antes.')
      : o.verdict === 'melhor' ? 'Houve melhora com significância: o ajuste ficou aplicado. Você pode desfazer agora ou depois, no Histórico.'
        : failed ? 'A prova não terminou bem e o Axion não conseguiu desfazer o ajuste: abra o Histórico e use Desfazer (ou Forçar desfazer).'
          : 'Sem melhora comprovada: o Axion desfez o ajuste pelo histórico e o PC voltou ao estado de antes.';
    return h('div', null,
      withHead ? h('div', { class: 'res-head ' + (o.verdict === 'melhor' ? 'ok' : failed ? 'warn' : 'neutral') }, icon(o.verdict === 'melhor' ? 'check' : failed ? 'alert' : 'info'),
        h('h4', { text: cancelled ? 'Prova cancelada' : o.verdict === 'melhor' ? 'Melhorou no seu PC' : 'Sem melhora comprovada' })) : null,
      h('div', { class: 'callout' + (failed ? ' warn' : '') }, icon(o.verdict === 'melhor' ? 'check' : failed ? 'alert' : 'info'),
        h('div', null, h('b', { text: o.tweakTitle || '' }), cancelled ? null : [' · ', trialBadge(o)], h('div', { class: 'small', style: { marginTop: '4px' }, text }))),
      (o.metrics || []).length ? h('div', { class: 'stats', style: { marginTop: '12px' } }, o.metrics.map(x => h('div', { class: 'stat' }, h('div', { class: 'n', text: fmtSigned(x.deltaPct, 1) + '%' }), h('div', { class: 'l', text: x.label + ' · ' + pLabel(x.pAdj) })))) : null,
      (o.warnings || []).length && !r.comparison ? (o.warnings || []).map(w => h('div', { class: 'callout warn', style: { marginTop: '10px' } }, icon('alert'), h('div', { text: w }))) : null,
      r.comparison ? h('details', { style: { marginTop: '12px' } }, h('summary', { class: 'small linkbtn', text: 'Todas as métricas e avisos' }), perfCompareCard(r.comparison)) : null);
  }

  function frameChart(series, s) {
    const W = 1000, H = 230, L = 46, R = 8, T = 10, B = 26;
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('viewBox', '0 0 ' + W + ' ' + H);
    svg.setAttribute('class', 'ftchart');
    svg.setAttribute('role', 'img');
    svg.setAttribute('aria-label', 'Tempo de quadro ao longo do teste');
    if (!series || !series.length) return svg;
    const avg = s.ftAvg > 0 ? s.ftAvg : 1;
    const top = Math.max(s.ftP99 * 2, avg * StutterFactorJS * 1.25, 1);
    const step = [0.5, 1, 2, 5, 10, 20, 50, 100].find(x => top / x <= 5) || 200;
    const yMax = Math.ceil(top / step) * step;
    const X = i => L + (W - L - R) * (series.length === 1 ? 0 : i / (series.length - 1));
    const Y = v => T + (H - T - B) * (1 - Math.min(v, yMax) / yMax);
    let out = '';
    for (let v = 0; v <= yMax + 1e-9; v += step) {
      out += '<line x1="' + L + '" x2="' + (W - R) + '" y1="' + Y(v).toFixed(1) + '" y2="' + Y(v).toFixed(1) + '" class="c-grid" stroke-width="1"/>' +
        '<text x="' + (L - 6) + '" y="' + (Y(v) + 4).toFixed(1) + '" text-anchor="end" font-size="11" class="c-axis">' + String(v).replace('.', ',') + '</text>';
    }
    const thr = avg * StutterFactorJS;
    if (thr < yMax) out += '<line x1="' + L + '" x2="' + (W - R) + '" y1="' + Y(thr).toFixed(1) + '" y2="' + Y(thr).toFixed(1) + '" class="c-thr" stroke-dasharray="5 4" stroke-width="1.2"/>' +
      '<text x="' + (W - R - 4) + '" y="' + (Y(thr) - 5).toFixed(1) + '" text-anchor="end" font-size="11" class="c-thr-t">engasgo (2,5 × média)</text>';
    out += '<line x1="' + L + '" x2="' + (W - R) + '" y1="' + Y(avg).toFixed(1) + '" y2="' + Y(avg).toFixed(1) + '" class="c-avg" stroke-dasharray="3 4" stroke-width="1.2"/>';
    let pts = '';
    series.forEach((v, i) => { pts += X(i).toFixed(1) + ',' + Y(v).toFixed(1) + ' '; });
    out += '<polyline fill="none" class="c-line" stroke-width="1.3" stroke-linejoin="round" points="' + pts + '"/>';
    series.forEach((v, i) => { if (v > yMax) out += '<circle cx="' + X(i).toFixed(1) + '" cy="' + (T + 3) + '" r="3" class="c-out"/>'; });
    out += '<text x="' + L + '" y="' + (H - 6) + '" font-size="11" class="c-axis">0 s</text>' +
      '<text x="' + (W - R) + '" y="' + (H - 6) + '" text-anchor="end" font-size="11" class="c-axis">' + fmtN(s.seconds, 0) + ' s</text>' +
      '<text x="' + ((L + W) / 2) + '" y="' + (H - 6) + '" text-anchor="middle" font-size="11" class="c-axis">tempo de quadro (ms) · linha tracejada: média</text>';
    svg.innerHTML = out;
    return svg;
  }
  const StutterFactorJS = 2.5;

  function limiterBar(s) {
    if (s.gpuBoundPct < 0) return h('div', { class: 'small muted', text: 'O PresentMon não trouxe os tempos de CPU e GPU desta captura.' });
    const seg = (pct, cls, label) => pct > 0 ? h('i', { class: cls, style: { width: pct + '%' }, title: label + ': ' + fmtN(pct) + '%' }) : null;
    return h('div', null,
      h('div', { class: 'limbar' }, seg(s.gpuBoundPct, 'g', 'GPU'), seg(s.cpuBoundPct, 'c', 'CPU'), seg(s.waitPct, 'w', 'Limite/sincronização')),
      h('div', { class: 'row wrap small', style: { gap: '14px', marginTop: '6px' } },
        h('span', null, h('b', { class: 'dot g' }), ' GPU ' + fmtN(s.gpuBoundPct) + '%'),
        h('span', null, h('b', { class: 'dot c' }), ' CPU ' + fmtN(s.cpuBoundPct) + '%'),
        h('span', null, h('b', { class: 'dot w' }), ' limite/sincronização ' + fmtN(s.waitPct) + '%')));
  }

  function perfAdvice(r) {
    const s = r.summary, out = [];
    if (s.limiter === 'gpu') out.push('A placa de vídeo limitou ' + fmtN(s.gpuBoundPct, 0) + '% dos quadros. Mais FPS aqui vem de configurações gráficas (resolução, sombras, MSAA) ou de upscaling; ajustes do Windows mudam pouco.');
    if (s.limiter === 'cpu' && s.capLikely) out.push('O lado da CPU segurou ' + fmtN(s.cpuBoundPct, 0) + '% dos quadros com um tempo de quadro quase constante: é o sinal de um limite de FPS do próprio jogo (fps_max ou limitador do Reflex), que o PresentMon conta como tempo de CPU. Se você limitou o FPS, é o esperado: compare 1% low e engasgos, não o FPS médio.');
    else if (s.limiter === 'cpu') out.push('O lado da CPU limitou ' + fmtN(s.cpuBoundPct, 0) + '% dos quadros: a placa de vídeo ficou esperando. Se você não usa limite de FPS no jogo (fps_max), mais FPS aqui vem de CPU e memória (perfil EXPO/XMP da RAM, menos programas em segundo plano), não de gráficos mais baixos. Um limite de FPS do jogo também aparece assim, porque o PresentMon conta a espera do limitador como tempo de CPU.');
    if (s.limiter === 'limite') out.push('A maior parte dos quadros esperou um limite (fps_max, V-Sync, G-SYNC/FreeSync ou o limitador do Reflex). É o esperado se você limitou o FPS; nesse caso compare 1% low e engasgos, não o FPS médio.');
    const mode = MODE_NOTE[s.presentMode];
    if (mode && mode[0] === 'action') out.push(mode[1]);
    if (s.syncInterval >= 1) out.push('O jogo pediu sincronização vertical (SyncInterval ' + s.syncInterval + '): elimina o tearing, mas soma latência. Em jogo competitivo o usual é V-Sync desligado.');
    if (s.stutterPct >= 1) out.push('Engasgos ocuparam ' + fmtN(s.stutterPct) + '% do tempo. Nas primeiras partidas depois de atualizar driver ou jogo é normal (compilação de shaders); se persistir, veja programas em segundo plano e temperatura.');
    if (r.swapchains > 1) out.push('Uma sobreposição (Steam, Discord, gravador) desenhou sua própria camada durante o teste; o Axion mediu só a do jogo.');
    return out;
  }

  // The conditions the PC was in and what the sensors read during the
  // recording: a "before" and an "after" in different conditions say nothing
  // about the setting that changed.
  function perfConditions(r) {
    const c = r.conditions, rem = r.remarks || [];
    if (!c && !rem.length) return null;
    const chips = [];
    if (c) {
      chips.push(c.onBattery ? badge('Na bateria' + (c.batteryPct >= 0 ? ' (' + c.batteryPct + '%)' : ''), 'warn', 'alert') : badge(c.hasBattery ? 'Na tomada' : 'Computador de mesa', 'outline'));
      if (c.powerScheme) chips.push(badge('Plano: ' + c.powerScheme, c.powerSaver ? 'warn' : 'outline'));
      if (c.powerMode) chips.push(badge('Modo: ' + c.powerMode, c.efficiency ? 'warn' : 'outline'));
      if (c.displayHz > 0) chips.push(badge('Monitor a ' + c.displayHz + ' Hz', 'outline'));
    }
    return h('div', { class: 'cond-box' },
      h('h3', { text: 'Condições durante a medição' }),
      chips.length ? h('div', { class: 'row wrap', style: { gap: '6px' } }, chips) : null,
      rem.map(x => h('div', { class: 'callout' + (x.level === 'warn' ? ' warn' : ''), style: { marginTop: '8px' } }, icon(x.level === 'warn' ? 'alert' : 'info'),
        h('div', null, h('b', { text: x.title }), x.text ? h('div', { class: 'small muted', style: { marginTop: '2px' }, text: x.text }) : null))),
      !rem.length ? h('p', { class: 'small muted', style: { marginTop: '8px' }, text: 'Nada fora do normal nas condições nem nos sensores durante esta medição.' }) : null);
  }

  function perfRunCard(r) {
    const s = r.summary || {};
    const mode = MODE_NOTE[s.presentMode];
    const lat = [['Present → tela', s.untilDisplayedAvg], ['Entrada → tela (estimada)', s.inputLatencyAvg], ['Clique → tela (estimada)', s.clickLatencyAvg], ['Latência do PC (Reflex)', s.pcLatencyAvg]].filter(x => x[1] > 0);
    const adv = perfAdvice(r);
    return h('div', { class: 'card', id: 'perf-run', style: { marginTop: '16px' } },
      h('div', { class: 'row wrap' }, h('h2', { style: { margin: 0 } }, icon('gauge'), (r.label ? r.label + ' · ' : '') + r.process), h('div', { class: 'spacer' }),
        h('span', { class: 'small muted', text: fmtDT(r.started) + ' · ' + plural(s.frames || 0, 'quadro', 'quadros') + ' em ' + fmtN(s.seconds, 1) + ' s' })),
      h('div', { class: 'stats', style: { marginTop: '12px' } },
        h('div', { class: 'stat' }, h('div', { class: 'n', text: fmtN(s.avgFps) }), h('div', { class: 'l', text: 'FPS médio' })),
        h('div', { class: 'stat' }, h('div', { class: 'n', text: fmtN(s.low1Fps) }), h('div', { class: 'l', text: '1% low' })),
        h('div', { class: 'stat' }, h('div', { class: 'n', text: fmtN(s.low01Fps) }), h('div', { class: 'l', text: '0,1% low' })),
        h('div', { class: 'stat' }, h('div', { class: 'n', text: String(s.stutters || 0) }), h('div', { class: 'l', text: 'engasgos (' + fmtN(s.stutterPct, 2) + '% do tempo)' }))),
      frameChart(r.series, s),
      h('div', { class: 'grid g2', style: { marginTop: '12px' } },
        h('div', null, h('h3', { text: 'Tempo de quadro (ms)' }),
          h('dl', { class: 'kv' }, [['Médio', s.ftAvg], ['Mediana', s.ftMedian], ['P99', s.ftP99], ['P99,9', s.ftP999], ['Máximo', s.ftMax], ['Desvio-padrão', s.ftStdDev]].map(([k, x]) => [h('dt', { text: k }), h('dd', { text: fmtN(x, 2) })]),
            h('dt', { text: 'P1 / P0,1 (percentil)' }), h('dd', { text: fmtN(s.p1Fps) + ' / ' + fmtN(s.p01Fps) + ' FPS' }))),
        h('div', null, h('h3', { text: 'O que limitou os quadros' }), limiterBar(s),
          s.limiter ? h('p', { class: 'small', style: { marginTop: '8px' } }, 'Principal: ', badge((LIMITER[s.limiter] || [s.limiter])[0], (LIMITER[s.limiter] || ['', 'outline'])[1]),
            ' · CPU ocupada ' + fmtN(s.cpuBusyAvg, 2) + ' ms e GPU ocupada ' + fmtN(s.gpuBusyAvg, 2) + ' ms por quadro, em média') : null,
          h('h3', { text: 'Tela', style: { marginTop: '14px' } }),
          h('dl', { class: 'kv' },
            h('dt', { text: 'Modo de apresentação' }), h('dd', null, s.presentMode || '—', mode ? h('div', { class: 'small muted', text: mode[1] }) : null),
            h('dt', { text: 'Tearing permitido' }), h('dd', { text: fmtN(s.tearingPct, 0) + '% dos quadros' + (s.syncInterval === 0 ? ' (V-Sync desligado no jogo)' : '') }),
            h('dt', { text: 'Quadros mostrados' }), h('dd', { text: s.displayedFps > 0 ? fmtN(s.displayedFps) + ' por segundo' + (s.droppedPct > 0 ? ' · ' + fmtN(s.droppedPct, 2) + '% não chegaram à tela' : '') : '—' })),
          lat.length ? [h('h3', { text: 'Latência média (ms)', style: { marginTop: '14px' } }), h('dl', { class: 'kv' }, lat.map(([k, x]) => [h('dt', { text: k }), h('dd', { text: fmtN(x, 2) })]))] : null)),
      perfConditions(r),
      adv.length ? h('div', { class: 'callout', style: { marginTop: '12px' } }, icon('info'), h('ul', { class: 'notes', style: { margin: 0 } }, adv.map(t => h('li', { text: t })))) : null,
      lat.length ? h('p', { class: 'small muted', text: 'As latências vêm dos eventos do Windows e do Reflex, não de um sensor na tela: servem para comparar configurações, não como valor absoluto de ponta a ponta.' }) : null,
      r.note ? h('p', { class: 'small', style: { color: 'var(--warn)' }, text: 'Obs.: ' + r.note + '.' }) : null,
      r.csv ? h('p', { class: 'small muted', text: 'Arquivo CSV desta medição: ' + (S.state.dataDir ? S.state.dataDir.replace(/[\\/]+$/, '') + '\\desempenho\\' : '') + r.csv + ' (abre no Excel, no CapFrameX ou no próprio PresentMon).' }) : null);
  }

  function perfCompareCard(c, opts) {
    const sess = !!(opts && opts.session);
    const anchor = !!(opts && opts.anchor);
    const VERD = { melhor: ['melhor', 'ok'], pior: ['pior', 'bad'], igual: ['sem diferença detectável', 'outline'], inconclusivo: ['inconclusivo', 'outline'], indicativo: ['só indicação', 'outline'] };
    const cell = g => (g.n ? fmtN(g.mean, 2) + (g.sd >= 0 ? ' ± ' + fmtN(g.sd, 2) : '') + ' (n=' + g.n + ')' : '—');
    return h('div', { class: 'card', id: anchor ? 'perf-cmp' : null, style: { marginTop: '16px' } },
      h('h2', null, icon('gauge'), 'Comparação: ' + c.labelA + ' × ' + c.labelB),
      h('table', { class: 'table' },
        h('thead', null, h('tr', null, ['Métrica', c.labelA, c.labelB, 'Diferença', sess ? null : 'p (ajustado)', 'Resultado'].filter(Boolean).map(x => h('th', { text: x })))),
        h('tbody', null, (c.rows || []).map(r => h('tr', null,
          h('td', { text: r.label + ' (' + r.unit + ')' }), h('td', { text: cell(r.a) }), h('td', { text: cell(r.b) }),
          h('td', { text: fmtSigned(r.delta, 2) + ' (' + fmtSigned(r.deltaPct, 1) + '%)' }),
          sess ? null : h('td', { text: r.pAdj >= 0 ? (r.pAdj < 0.001 ? '< 0,001' : fmtN(r.pAdj, 3)) : '—', title: r.p >= 0 ? 'p sem ajuste: ' + fmtN(r.p, 4) : '' }),
          h('td', null, badge((VERD[r.verdict] || [r.verdict])[0], (VERD[r.verdict] || ['', 'outline'])[1])))))),
      (c.warnings || []).map(w => h('div', { class: 'callout warn', style: { marginTop: '10px' } }, icon('alert'), h('div', { text: w }))),
      sess ? h('p', { class: 'small muted', text: 'Média ± desvio-padrão entre as janelas de cada partida. Partidas diferentes não são um teste controlado: não há cálculo de significância aqui.' }) :
      h('p', { class: 'small muted', text: 'Média ± desvio-padrão entre as medições de cada grupo. "p" vem do teste t de Welch (bicaudal): se não houvesse diferença real, um resultado assim apareceria em menos de 5% das vezes quando p < 0,05. Como várias métricas são testadas juntas, os valores de p já vêm ajustados (método de Holm), e diferenças menores que 1% contam como "sem diferença". Confie mais quando FPS médio, 1% low e P99 concordam. Com uma medição só em algum grupo, ou sem nenhuma variação entre as medições, não há como calcular.' }));
  }

  async function pageProva() {
    const v = await get('trial', '/api/trial');
    const ok = (v.candidates || []).filter(x => x.ok), no = (v.candidates || []).filter(x => !x.ok);
    if (!ok.some(x => x.id === trialForm.tweak)) trialForm.tweak = ok.length ? ok[0].id : '';
    const procs = v.processes || [];
    if (!procs.some(p => p.exe === trialForm.process)) trialForm.process = (procs.find(p => p.running) || procs[0] || { exe: 'cs2.exe' }).exe;
    const sel = (label, value, opts, onchange) => {
      const s = h('select', { class: 'select', 'aria-label': label }, opts.map(([val, text]) => h('option', { value: val, selected: String(value) === String(val) ? 'selected' : null, text })));
      s.addEventListener('change', () => onchange(s.value));
      return s;
    };
    const fld = (l, el) => h('label', { class: 'fld' }, h('span', { text: l }), el);
    const cur = ok.find(x => x.id === trialForm.tweak);
    const minutes = Math.ceil(2 * trialForm.runs * (trialForm.seconds + (trialForm.mode === 'delay' ? trialForm.delay : 15)) / 60);
    const start = async () => {
      const yes = await confirmDlg({
        title: 'Provar: ' + cur.title, ok: 'Começar a prova', icon: 'play', wide: true,
        body: h('div', null,
          cur.rec && cur.rec.level === 2 ? h('div', { class: 'callout warn' }, icon('alert'), h('div', { text: 'Não indicado para o seu PC: ' + cur.rec.reason })) : null,
          h('ol', { class: 'notes' },
            h('li', { text: 'Deixe o jogo aberto, no mesmo mapa e no mesmo ponto de partida, e feche o navegador e outros programas pesados.' }),
            h('li', { text: plural(trialForm.runs, 'medição', 'medições') + ' de ' + trialForm.seconds + ' s sem o ajuste. ' + (trialForm.mode === 'hotkey' ? 'Em cada uma, volte ao ponto de partida e aperte ' + hotkeyLabel(trialForm.hotkey) + ' para começar. Não aperte a tecla de novo antes do fim.' : 'Cada uma começa ' + trialForm.delay + ' s depois da anterior: volte ao ponto de partida nesse tempo.') }),
            h('li', { text: 'O Axion aplica "' + cur.title + '" pelo histórico, guardando o valor de antes.' }),
            h('li', { text: 'Mais ' + plural(trialForm.runs, 'medição igual', 'medições iguais') + ', agora com o ajuste.' }),
            h('li', { text: 'Se houver melhora comprovada, o ajuste fica, e você pode desfazer com um clique. Se não houver, o Axion desfaz sozinho.' })),
          h('p', { class: 'small muted', text: 'Tempo estimado: cerca de ' + minutes + ' min. Cancelar a qualquer momento desfaz o ajuste, se ele já tiver sido aplicado.' })),
      });
      if (!yes) return;
      await runJob(api('POST', '/api/trial/start', { tweakId: trialForm.tweak, process: trialForm.process, seconds: trialForm.seconds, runs: trialForm.runs, mode: trialForm.mode, hotkey: trialForm.hotkey, delay: trialForm.delay }), { title: 'Prova: ' + cur.title });
      rerender();
    };
    const tw = sel('Ajuste', trialForm.tweak, ok.map(x => [x.id, x.title]), x => { trialForm.tweak = x; rerender(); });
    const pr = sel('Jogo', trialForm.process, procs.map(p => [p.exe, p.name + ' (' + p.exe + ')' + (p.running ? ' · aberto agora' : '')]), x => { trialForm.process = x; rerender(); });
    const secs = sel('Duração de cada medição', trialForm.seconds, [30, 60, 90, 120].map(n => [n, n + ' s']), x => { trialForm.seconds = +x; rerender(); });
    const runs = sel('Medições de cada lado', trialForm.runs, [2, 3, 4, 5].map(n => [n, n + ' antes e ' + n + ' depois']), x => { trialForm.runs = +x; rerender(); });
    const mode = sel('Como começar', trialForm.mode, [['hotkey', 'Por tecla de atalho'], ['delay', 'Após contagem regressiva']], x => { trialForm.mode = x; rerender(); });
    const hk = trialForm.mode === 'hotkey'
      ? sel('Tecla', trialForm.hotkey, (v.hotkeys || []).map(k => [k, hotkeyLabel(k)]), x => { trialForm.hotkey = x; })
      : sel('Contagem', trialForm.delay, [5, 10, 15, 30].map(n => [n, n + ' s para voltar ao jogo']), x => { trialForm.delay = +x; rerender(); });
    const disabled = !v.ready || !v.elevated || !cur;
    const cell = (o, k) => { const x = (o.metrics || []).find(m => m.key === k); return x ? fmtSigned(x.deltaPct, 1) + '% (' + pLabel(x.pAdj) + ')' : '—'; };
    return h('div', { class: 'page' },
      pageHead('Prova no seu PC', 'Mede um ajuste opcional no seu jogo, antes e depois, e só deixa aplicado o que melhorar de forma comprovada. O que não melhorar é desfeito pelo histórico.'),
      !v.ready ? h('div', { class: 'callout warn' }, icon('alert'), h('div', { text: 'Medição indisponível: ' + (v.why || 'PresentMon não encontrado') })) : null,
      v.ready && !v.elevated ? h('div', { class: 'callout warn' }, icon('alert'), h('div', { text: 'Execute o Axion como administrador para fazer a prova: o PresentMon e as mudanças no Windows exigem essa permissão.' })) : null,
      (v.interrupted || []).map(x => h('div', { class: 'callout warn', style: { marginBottom: '12px' } }, icon('alert'), h('div', null,
        h('b', { text: 'Prova interrompida: ' + x.title }),
        h('div', { class: 'small', text: 'A prova não terminou (o Axion foi fechado ou houve um erro, ' + fmtDT(x.at) + ') e o ajuste ficou aplicado, sem veredito.' }),
        h('div', { class: 'row wrap', style: { marginTop: '8px' } },
          btn('Desfazer agora', async () => { await runJob(api('POST', '/api/revert', { entries: [x.entryId] }), { title: 'Desfazendo: ' + x.title }); rerender(); }, 'sm', 'undo'),
          btn('Deixar aplicado', async () => { try { await api('POST', '/api/trial/keep', { trial: x.trial }); invalidate(); rerender(); } catch (e) { toast(e.message, 'bad'); } }, 'sm ghost'))))),
      h('div', { class: 'card' }, h('h2', null, icon('play'), 'Nova prova'),
        ok.length ? h('div', { class: 'fields grid3' }, fld('Ajuste', tw), fld('Jogo', pr), fld('Duração', secs), fld('Medições', runs), fld('Início', mode), fld(trialForm.mode === 'hotkey' ? 'Tecla' : 'Contagem', hk))
          : h('div', { class: 'callout' }, icon('info'), h('div', { text: 'Nenhum ajuste opcional pode ser provado neste PC agora. Veja abaixo o motivo de cada um.' })),
        cur ? h('div', { class: 'tw', style: { marginTop: '10px' } }, h('b', { text: cur.title + ': ' }), cur.summary, ' ', evBadge(cur.evidence), ' ', riskBadge(cur.risk)) : null,
        (v.preflight || []).map(x => h('div', { class: 'callout warn', style: { marginTop: '12px' } }, icon('alert'), h('div', null, h('b', { text: 'Antes de medir: ' + x.title }), x.text ? h('div', { class: 'small muted', text: x.text }) : null))),
        h('div', { class: 'row wrap', style: { marginTop: '12px' } }, btn('Começar a prova', start, 'primary', 'play', { disabled: disabled ? 'disabled' : null }), h('span', { class: 'small muted', text: 'Cerca de ' + minutes + ' min com o jogo aberto.' }))),
      trialLast ? h('div', { class: 'card signature mt-4' }, h('h2', null, icon('gauge'), 'Resultado da última prova'), trialResultView(trialLast, true)) : null,
      h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('clock'), 'Provas anteriores'),
        (v.recent || []).length ? h('div', { class: 'tbl-scroll' }, h('table', { class: 'table' },
          h('thead', null, h('tr', null, ['Data', 'Ajuste', 'Jogo', 'Resultado', '1% low', 'FPS médio', 'P99', 'Situação', ''].map(x => h('th', { text: x })))),
          h('tbody', null, v.recent.map(o => h('tr', null, h('td', { text: fmtDT(o.at) }), h('td', { text: o.tweakTitle }), h('td', { text: o.process }), h('td', null, trialBadge(o)),
            h('td', { text: cell(o, 'low1Fps') }), h('td', { text: cell(o, 'avgFps') }), h('td', { text: cell(o, 'ftP99') }), h('td', { text: TRIAL_ACTION[o.action] || o.action }),
            h('td', null, o.action === 'mantido' || o.action === 'falha-desfazer' ? btn('Desfazer', async () => { await undoTrial(o); rerender(); }, 'sm ghost', 'undo') : null))))))
          : empty('gauge', 'Nenhuma prova ainda', 'Escolha um ajuste acima. O resultado aparece aqui e no próprio item, em Otimizações.')),
      h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('book'), 'Como o Axion decide'),
        h('p', { class: 'small', text: v.rule }), h('p', { class: 'small muted', text: v.honest }),
        h('div', { class: 'row wrap' }, (v.sources || []).map(x => extLink(x.title, x.url)))),
      no.length ? h('details', { class: 'card more', style: { marginTop: '16px' } }, h('summary', null, icon('info'), 'Por que os outros ajustes não entram (' + no.length + ')'),
        h('ul', { class: 'notes small', style: { marginTop: '10px' } }, no.map(x => h('li', null, h('b', { text: x.title }), ' — ', x.why)))) : null);
  }

  async function pagePerf() {
    const v = await get('perf', '/api/perf');
    const runs = v.runs || [];
    const procs = v.processes || [];
    if (!perfForm.process) perfForm.process = (procs.find(p => p.running) || procs[0] || { exe: 'cs2.exe' }).exe;
    const byId = new Map(runs.map(r => [r.id, r]));
    [perfSel.a, perfSel.b].forEach(set => [...set].forEach(id => { if (!byId.has(id)) { set.delete(id); perfCmp = null; } }));
    if (perfOpen && !byId.has(perfOpen)) perfOpen = null;

    // Capture form
    const procSel = h('select', { class: 'select', 'aria-label': 'Jogo' },
      procs.map(p => h('option', { value: p.exe, selected: perfForm.process === p.exe ? 'selected' : null, text: p.name + ' (' + p.exe + ')' + (p.running ? ' · aberto agora' : '') })),
      h('option', { value: '__other', selected: perfForm.process === '__other' ? 'selected' : null, text: 'Outro executável…' }));
    procSel.addEventListener('change', () => { perfForm.process = procSel.value; rerender(); });
    const other = perfForm.process === '__other' ? h('input', { class: 'select', placeholder: 'ex.: r5apex.exe', value: perfForm.other, 'aria-label': 'Executável do jogo', 'data-focus': 'perf-other' }) : null;
    if (other) other.addEventListener('input', () => { perfForm.other = other.value; });
    const secs = h('select', { class: 'select', 'aria-label': 'Duração' }, [30, 60, 90, 120].map(n => h('option', { value: n, selected: perfForm.seconds === n ? 'selected' : null, text: n + ' s' })));
    secs.addEventListener('change', () => { perfForm.seconds = +secs.value; });
    const modeSel = h('select', { class: 'select', 'aria-label': 'Como começar' },
      h('option', { value: 'hotkey', selected: perfForm.mode === 'hotkey' ? 'selected' : null, text: 'Começar por tecla de atalho' }),
      h('option', { value: 'delay', selected: perfForm.mode === 'delay' ? 'selected' : null, text: 'Começar após contagem regressiva' }));
    modeSel.addEventListener('change', () => { perfForm.mode = modeSel.value; rerender(); });
    const hk = perfForm.mode === 'hotkey'
      ? h('select', { class: 'select', 'aria-label': 'Tecla de atalho' }, (v.hotkeys || []).map(k => h('option', { value: k, selected: perfForm.hotkey === k ? 'selected' : null, text: hotkeyLabel(k) })))
      : h('select', { class: 'select', 'aria-label': 'Contagem regressiva' }, [5, 10, 15, 30].map(n => h('option', { value: n, selected: perfForm.delay === n ? 'selected' : null, text: n + ' s para voltar ao jogo' })));
    hk.addEventListener('change', () => { if (perfForm.mode === 'hotkey') perfForm.hotkey = hk.value; else perfForm.delay = +hk.value; });
    const labels = Array.from(new Set(['antes', 'depois'].concat(v.labels || [])));
    const lab = h('input', { class: 'select', value: perfForm.label, list: 'perf-labels', placeholder: 'rótulo (ex.: antes)', 'aria-label': 'Rótulo da medição', 'data-focus': 'perf-label', style: { width: '170px' } });
    lab.addEventListener('input', () => { perfForm.label = lab.value; });
    const start = async () => {
      const process = perfForm.process === '__other' ? perfForm.other.trim() : perfForm.process;
      if (!process) { toast('Informe o executável do jogo.', 'info'); return; }
      const job = await runJob(api('POST', '/api/perf/capture', { process, seconds: perfForm.seconds, mode: perfForm.mode, hotkey: perfForm.hotkey, delay: perfForm.delay, label: perfForm.label }), { title: 'Medindo ' + process });
      if (job && job.status === 'done' && job.result) { perfOpen = job.result.id; rerender(); }
    };
    const disabled = !v.ready || !v.elevated;

    // Runs table
    // A comparison on screen always matches the groups selected.
    const toggle = (set, other2, id, cb) => { cb.addEventListener('change', () => { if (cb.checked) { set.add(id); other2.delete(id); } else set.delete(id); perfCmp = null; rerender(); }); return cb; };
    const selBox = (set, other2, id, label) => toggle(set, other2, id, h('input', { type: 'checkbox', checked: set.has(id) ? 'checked' : null, 'aria-label': label }));
    const row = r => {
      const s = r.summary || {};
      const lim = LIMITER[s.limiter];
      const acts = h('div', { class: 'tiny', style: { marginTop: '2px' } },
        h('button', { class: 'linkbtn tiny', type: 'button', text: perfOpen === r.id ? 'fechar' : 'ver', onclick: () => { perfOpen = perfOpen === r.id ? null : r.id; rerender(); } }), ' · ',
        h('button', { class: 'linkbtn tiny', type: 'button', text: 'rótulo', onclick: async () => {
          const inp = h('input', { class: 'select', value: r.label || '', style: { width: '100%' }, 'aria-label': 'Novo rótulo', maxlength: '40' });
          const ok = await confirmDlg({ title: 'Rótulo da medição', ok: 'Salvar', icon: 'check', body: h('div', null, inp), input: inp });
          if (!ok) return;
          try { await api('POST', '/api/perf/label', { id: r.id, label: inp.value }); perfCmp = null; invalidate(); rerender(); } catch (e) { toast(e.message, 'bad'); }
        } }), ' · ',
        h('button', { class: 'linkbtn tiny', type: 'button', text: 'apagar', onclick: async () => {
          const ok = await confirmDlg({ title: 'Apagar medição', ok: 'Apagar', danger: true, icon: 'trash', body: h('p', { text: 'A medição de ' + fmtDT(r.started) + ' e o CSV dela serão apagados.' }) });
          if (!ok) return;
          try { await api('POST', '/api/perf/delete', { id: r.id }); perfSel.a.delete(r.id); perfSel.b.delete(r.id); perfCmp = null; invalidate(); rerender(); } catch (e) { toast(e.message, 'bad'); }
        } }));
      return h('tr', { class: perfOpen === r.id ? 'sel' : null },
        h('td', null, selBox(perfSel.a, perfSel.b, r.id, 'Grupo A')), h('td', null, selBox(perfSel.b, perfSel.a, r.id, 'Grupo B')),
        h('td', null, h('div', { text: fmtDT(r.started) }), acts),
        h('td', null, h('div', { text: r.label || '—' }), h('div', { class: 'tiny muted', text: r.process }),
          // A capture taken in conditions that change the FPS (battery, an economy plan, a hot GPU...) is marked.
          (r.remarks || []).some(x => x.level === 'warn') ? h('div', { style: { marginTop: '3px' } }, h('span', { title: (r.remarks || []).filter(x => x.level === 'warn').map(x => x.title).join('; ') }, badge('condições', 'warn', 'alert'))) : null),
        h('td', { text: fmtN(s.avgFps) }), h('td', { text: fmtN(s.low1Fps) }), h('td', { text: fmtN(s.low01Fps) }), h('td', { text: fmtN(s.ftP99, 2) }),
        h('td', { text: String(s.stutters || 0) }), h('td', null, lim ? badge(lim[0].split(' (')[0], lim[1]) : '—'));
    };
    const byLabel = (lbl, set) => { set.clear(); runs.filter(r => r.label === lbl).forEach(r => set.add(r.id)); };
    const la = h('select', { class: 'select', 'aria-label': 'Grupo A por rótulo' }, h('option', { value: '', text: 'Grupo A por rótulo…' }), (v.labels || []).map(l => h('option', { value: l, text: l })));
    la.addEventListener('change', () => { if (la.value) { byLabel(la.value, perfSel.a); [...perfSel.a].forEach(id => perfSel.b.delete(id)); perfCmp = null; rerender(); } });
    const lb = h('select', { class: 'select', 'aria-label': 'Grupo B por rótulo' }, h('option', { value: '', text: 'Grupo B por rótulo…' }), (v.labels || []).map(l => h('option', { value: l, text: l })));
    lb.addEventListener('change', () => { if (lb.value) { byLabel(lb.value, perfSel.b); [...perfSel.b].forEach(id => perfSel.a.delete(id)); perfCmp = null; rerender(); } });
    const groupLabel = set => { const ls = Array.from(new Set([...set].map(id => (byId.get(id) || {}).label || '').filter(Boolean))); return ls.length === 1 ? ls[0] : ''; };
    const compare = async () => {
      try {
        perfCmp = await api('POST', '/api/perf/compare', { a: [...perfSel.a], b: [...perfSel.b], labelA: groupLabel(perfSel.a) || 'A', labelB: groupLabel(perfSel.b) || 'B' });
        rerender();
        setTimeout(() => { const el = document.getElementById('perf-cmp'); if (el) el.scrollIntoView({ behavior: 'smooth' }); }, 60);
      } catch (e) { toast(e.message, 'bad'); }
    };
    let openRun = null;
    if (perfOpen) { try { openRun = await get('perfrun:' + perfOpen, '/api/perf/run/' + perfOpen); } catch (e) { perfOpen = null; } }

    return h('div', { class: 'page' },
      pageHead('Desempenho (FPS)', 'Mede FPS, 1% e 0,1% low, engasgos e o que limitou cada quadro (CPU, GPU ou limite de FPS) com o PresentMon da Intel, e compara antes × depois levando em conta a variação entre medições.'),
      !v.ready ? h('div', { class: 'callout warn' }, icon('alert'), h('div', { text: 'Medição indisponível: ' + (v.why || 'PresentMon não encontrado') })) : null,
      v.ready && !v.elevated ? h('div', { class: 'callout warn' }, icon('alert'), h('div', { text: 'Execute o Axion como administrador para medir (o PresentMon lê eventos do Windows que exigem essa permissão).' })) : null,
      h('div', { class: 'card' },
        h('h2', null, icon('play'), 'Nova medição'),
        h('div', { class: 'fields' },
          h('label', { class: 'fld' }, h('span', { text: 'Jogo' }), procSel, other),
          h('label', { class: 'fld' }, h('span', { text: 'Duração' }), secs),
          h('label', { class: 'fld' }, h('span', { text: 'Início' }), modeSel),
          h('label', { class: 'fld' }, h('span', { text: perfForm.mode === 'hotkey' ? 'Tecla' : 'Contagem' }), hk),
          h('label', { class: 'fld' }, h('span', { text: 'Rótulo' }), lab),
          h('datalist', { id: 'perf-labels' }, labels.map(l => h('option', { value: l })))),
        // What is wrong with the PC right now for a measurement, said before it is taken.
        (v.preflight || []).map(x => h('div', { class: 'callout warn', style: { marginTop: '12px' } }, icon('alert'),
          h('div', null, h('b', { text: 'Antes de medir: ' + x.title }), x.text ? h('div', { class: 'small muted', style: { marginTop: '2px' }, text: x.text }) : null))),
        h('div', { class: 'row wrap', style: { marginTop: '12px' } },
          btn('Iniciar medição', start, 'primary', 'play', { disabled: disabled ? 'disabled' : null }),
          h('span', { class: 'small muted', text: perfForm.mode === 'hotkey' ? 'Depois de iniciar, entre no jogo e aperte ' + hotkeyLabel(perfForm.hotkey) + ' quando o teste começar (aperte de novo para parar antes do tempo).' : 'Depois de iniciar, volte para o jogo antes do fim da contagem.' })),
        h('ul', { class: 'notes small', style: { marginTop: '12px' } },
          h('li', { text: 'Use sempre o mesmo cenário: o mesmo mapa de benchmark da Oficina ou o mesmo trajeto em um mapa offline, do início ao fim.' }),
          h('li', { text: 'Espere o jogo carregar e rodar um pouco antes de começar: a primeira passada compila shaders e engasga.' }),
          h('li', { text: 'Grave 2 ou 3 vezes antes e depois de cada mudança, uma mudança por vez, com o navegador e outros programas pesados fechados.' }))),
      openRun ? perfRunCard(openRun) : null,
      h('div', { class: 'card', style: { marginTop: '16px' } },
        h('div', { class: 'row wrap' }, h('h2', { style: { margin: 0 } }, icon('clock'), 'Medições'), h('div', { class: 'spacer' }),
          runs.length ? [la, lb, btn('Comparar A × B', compare, 'primary', 'gauge', { disabled: perfSel.a.size && perfSel.b.size ? null : 'disabled' })] : null),
        runs.length ? h('div', { class: 'tbl-scroll', style: { marginTop: '10px' } }, h('table', { class: 'table perf-table' },
          h('thead', null, h('tr', null, ['A', 'B', 'Data', 'Rótulo', 'FPS médio', '1% low', '0,1% low', 'P99 (ms)', 'Engasgos', 'Limitado por'].map(x => h('th', { text: x })))),
          h('tbody', null, runs.map(row))))
          : empty('gauge', 'Nenhuma medição ainda', 'Faça a primeira acima. Para comparar, grave algumas com o rótulo "antes", aplique a mudança e grave outras com "depois".')),
      perfCmp ? perfCompareCard(perfCmp, { anchor: true }) : null,
      h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('book'), 'Como os números são calculados'),
        h('p', { class: 'small', text: v.honest }),
        h('ul', { class: 'notes small' },
          h('li', { text: 'FPS médio = quadros ÷ tempo total. 1% low e 0,1% low = média do 1% (0,1%) de quadros mais lentos, convertida em FPS (definição do NVIDIA FrameView). P1 e P0,1 são os percentis.' }),
          h('li', { text: 'Engasgo = quadro mais longo que 2,5 × o tempo médio (padrão do CapFrameX).' }),
          h('li', { text: 'Limitado pela GPU: a GPU ficou ocupada em 90% ou mais do tempo do quadro. CPU ou limitador do jogo: a GPU sobrou e o quadro não esperou na apresentação — inclui o tempo que um limite de FPS do próprio jogo (fps_max, Reflex) passa esperando. Limite/sincronização: o quadro esperou na apresentação (V-Sync, fila cheia, limitadores do driver).' })),
        h('div', { class: 'row wrap' }, (v.sources || []).map(x => extLink(x.title, x.url)))));
  }

  const cleanSel = new Set();
  let cleanSelStamp = null;
  const BYTES = b => { b = b || 0; return b >= 1073741824 ? (b / 1073741824).toFixed(1).replace('.', ',') + ' GB' : b >= 1048576 ? Math.round(b / 1048576) + ' MB' : b > 0 ? Math.max(1, Math.round(b / 1024)) + ' KB' : '0 KB'; };

  function ssdCard(v) {
    const t = v.trim || {};
    const hex = n => '0x' + (n >>> 0).toString(16).toUpperCase();
    const trimItem = {
      title: 'TRIM (aviso de blocos apagados)',
      status: t.ntfs === 0 ? 'ok' : t.ntfs === 1 ? 'action' : 'unknown',
      current: t.ntfs === 0 ? 'ligado (DisableDeleteNotify = 0, padrão do Windows)' : t.ntfs === 1 ? 'desligado (DisableDeleteNotify = 1)' : (v.trimErr || 'não foi possível ler'),
      advice: t.ntfs === 1 ? 'Religue: vale na hora, sem reiniciar. Depois faça um retrim para o SSD saber de todo o espaço liberado enquanto esteve desligado.' : null,
    };
    const taskItem = {
      title: 'Otimização automática de unidades (retrim mensal)',
      status: !v.taskRead ? 'unknown' : v.taskExists && v.taskOn ? 'ok' : 'action',
      current: !v.taskRead ? (v.taskErr || 'não foi possível ler') : !v.taskExists ? 'tarefa removida deste Windows'
        : (v.taskOn ? 'ativa' : 'desativada') + (v.taskLast ? ' · última execução ' + fmtDT(v.taskLast) + (v.taskResult ? ' (código ' + hex(v.taskResult) + ')' : '') : ''),
      advice: v.taskRead && !v.taskExists ? 'Sem essa tarefa o Windows não faz o retrim sozinho: faça um por aqui uma vez por mês.'
        : v.taskRead && !v.taskOn ? 'Reative: em SSD ela faz o retrim uma vez por mês; não desfragmenta o SSD toda semana.' : null,
    };
    const vols = v.volumes || [];
    const ssdVols = vols.filter(x => x.ssd && /^(NTFS|REFS)$/i.test(x.fs || ''));
    const others = vols.filter(x => !ssdVols.includes(x));
    const retrim = async () => {
      const ok = await confirmDlg({ title: 'Fazer retrim agora', ok: 'Fazer retrim', icon: 'zap',
        body: h('div', null, h('p', { style: { marginTop: 0 }, text: 'O Windows vai avisar o SSD de todo o espaço livre em ' + ssdVols.map(x => x.letter).join(', ') + ' (defrag /L).' }),
          h('p', { class: 'small muted', text: 'Não apaga nem move arquivos. Leva de segundos a poucos minutos; o PC pode ficar um pouco mais lento enquanto isso.' })) });
      if (ok) runJob(api('POST', '/api/ssd/retrim', {}), { title: 'Retrim do SSD' });
    };
    const acts = [];
    if (v.fixTrim) acts.push(btn('Religar TRIM', () => applyIds(['fix.trim.on'], 'Religar o TRIM do SSD'), 'primary', 'zap'));
    if (v.fixTask) acts.push(btn('Reativar otimização automática', () => applyIds(['fix.defrag.task'], 'Reativar a otimização automática de unidades'), v.fixTrim ? '' : 'primary', 'zap'));
    if (ssdVols.length) acts.push(btn('Fazer retrim agora', retrim, acts.length ? 'ghost' : 'primary', 'refresh', { disabled: v.elevated && t.ntfs !== 1 ? null : 'disabled',
      title: !v.elevated ? 'Exige executar o Axion como administrador' : t.ntfs === 1 ? 'Religue o TRIM primeiro' : '' }));
    return h('div', { class: 'card', id: 'ssd', style: { marginTop: '16px' } },
      h('h2', null, icon('hdd'), 'Manutenção do SSD'),
      h('p', { class: 'small muted', style: { marginTop: 0 }, text: v.honest }),
      h('div', { class: 'checks' }, checkItem(trimItem), checkItem(taskItem),
        ssdVols.map(x => {
          const days = x.lastRetrim ? (Date.now() - new Date(x.lastRetrim)) / 86400000 : null;
          const late = days !== null && days > 45;
          return checkItem({ title: 'Último retrim de ' + x.letter + (x.label ? ' (' + x.label + ')' : ''), status: days === null ? 'unknown' : late ? 'action' : 'ok',
            current: (x.lastRetrim ? fmtDT(x.lastRetrim) + ' · ' + ago(x.lastRetrim) : 'sem registro no log de eventos do Windows') + (x.disk ? ' · ' + x.disk : ''),
            advice: late ? 'Atrasado: o Windows faz o retrim uma vez por mês na manutenção automática, que só roda com o PC ocioso. Um retrim manual resolve.' : null });
        })),
      !v.scanned ? h('p', { class: 'small muted', text: 'Faça a análise do PC para listar os volumes em SSD.' })
        : !v.hasSSD ? h('p', { class: 'small muted', text: 'Nenhum SSD encontrado neste PC: TRIM e retrim não se aplicam a HD.' })
        : others.length ? h('p', { class: 'small muted', text: 'Fora do retrim: ' + others.map(x => x.letter + (x.disk ? ' (' + x.disk + ')' : '')).join(', ') + ' — o Windows não identificou o disco como SSD ou o sistema de arquivos não é NTFS.' }) : null,
      acts.length ? h('div', { class: 'row wrap', style: { marginTop: '12px' } }, acts) : null,
      h('div', { class: 'row wrap', style: { marginTop: '10px' } }, (v.sources || []).map(x => extLink(x.title, x.url))));
  }

  async function pageLimpeza() {
    const [v, ssd] = await Promise.all([get('cleanup', '/api/cleanup'), get('ssd', '/api/ssd').catch(() => null)]);
    const items = v.items || [];
    if (cleanSelStamp !== v.measuredAt) {
      cleanSel.clear();
      items.filter(it => it.default && it.size && it.size.bytes > 0).forEach(it => cleanSel.add(it.id));
      cleanSelStamp = v.measuredAt;
    }
    const selectable = it => it.kind !== 'link' && it.size && it.size.present && (it.kind === 'dism' || it.size.bytes > 0);
    const chosen = items.filter(it => cleanSel.has(it.id) && selectable(it));
    const total = chosen.reduce((n, it) => n + (it.kind === 'dism' ? 0 : it.size.bytes || 0), 0);
    const run = async () => {
      // Only the cautions of what is selected (the recycle bin is final, the
      // WinSxS cleanup cannot be undone...).
      const cautions = chosen.filter(it => it.caution).map(it => it.title + ': ' + it.caution);
      const ok = await confirmDlg({ title: 'Limpar ' + plural(chosen.length, 'item', 'itens'), ok: 'Limpar', icon: 'trash',
        body: h('div', null, h('ul', { class: 'notes' }, chosen.map(it => h('li', { text: it.title + (it.kind === 'dism' ? '' : ' — ' + BYTES(it.size.bytes)) }))),
          cautions.length ? h('div', { class: 'callout warn', style: { marginTop: '10px' } }, icon('alert'), h('ul', { class: 'notes', style: { margin: 0 } }, cautions.map(c => h('li', { text: c })))) : null,
          h('p', { class: 'small muted', text: 'Arquivos em uso ou recentes ficam.' })) });
      if (ok) runJob(api('POST', '/api/cleanup/run', { ids: chosen.map(it => it.id) }), { title: 'Limpando o disco' });
    };
    const row = it => {
      const m = it.size || {};
      const can = selectable(it);
      const dism = it.kind === 'dism';
      let sizeText = dism ? (m.present ? m.note || 'o tamanho só é conhecido depois da limpeza' : 'exige administrador')
        : !m.present ? 'não encontrado' : m.bytes > 0 ? BYTES(m.bytes) + (m.files ? ' · ' + plural(m.files, 'arquivo', 'arquivos') : '') : 'vazio';
      if (m.partial) sizeText += ' · medição parcial';
      if (m.kept) sizeText += ' · ' + plural(m.kept, 'recente fica', 'recentes ficam');
      if (m.note && !dism) sizeText += ' · ' + m.note;
      const input = h('input', { type: 'checkbox', checked: cleanSel.has(it.id) && can ? 'checked' : null, disabled: can ? null : 'disabled', 'aria-label': 'Selecionar ' + it.title });
      input.addEventListener('change', () => { if (input.checked) cleanSel.add(it.id); else cleanSel.delete(it.id); rerender(); });
      return h('div', { class: 'drv-offer' },
        h('label', { class: 'drv-check' }, it.kind === 'link' ? icon('ext') : input),
        h('div', { class: 'drv-main' },
          h('div', { class: 'row wrap', style: { gap: '6px' } }, h('b', { text: it.title }), it.default ? badge('recomendado', 'accent') : null),
          h('div', { class: 'small muted', text: sizeText }),
          h('div', { class: 'small', text: it.detail }),
          it.caution ? h('div', { class: 'small warn-text', text: '⚠ ' + it.caution }) : null,
          h('div', { class: 'row wrap' }, (it.sources || []).map(x => extLink(x.title, x.url)))),
        h('div', { class: 'drv-side' }, h('b', { text: it.kind === 'dism' || !m.present ? '' : BYTES(m.bytes) }),
          it.kind === 'link' && m.present ? h('div', null, h('button', { class: 'linkbtn small', type: 'button', text: 'Abrir Configurações', onclick: () => openURL(it.link) })) : null));
    };
    const last = v.last;
    const pct = v.total ? Math.round((v.total - v.free) / v.total * 100) : 0;
    return h('div', { class: 'page' },
      pageHead('Limpeza', 'Mede antes de apagar e só mexe no que é descartável. Arquivos em uso ou recentes ficam; caches que deixam jogos e o Windows mais rápidos não entram.',
        [btn('Medir de novo', async () => {
          try { await Promise.all([get('cleanup', '/api/cleanup?fresh=1', true), get('ssd', '/api/ssd', true)]); } catch (e) { toast(e.message, 'bad'); }
          rerender();
        }, 'ghost', 'refresh')]),
      v.total ? h('div', { class: 'card' }, h('div', { class: 'row wrap' }, h('h2', { style: { margin: 0 } }, icon('hdd'), 'Disco do sistema'), h('div', { class: 'spacer' }),
        h('b', { text: BYTES(v.free) + ' livres de ' + BYTES(v.total) })),
        h('div', { class: 'bar ' + (pct > 90 ? 'bad' : pct > 80 ? 'warn' : 'ok'), style: { marginTop: '10px' } }, h('i', { style: { width: pct + '%' } }))) : null,
      last ? h('div', { class: 'callout', style: { marginTop: '12px' } }, icon('check'),
        h('div', { text: 'Última limpeza (' + fmtDT(last.at) + '): ' + BYTES(last.freed) + ' liberados' + (last.skipped ? '; ' + plural(last.skipped, 'arquivo em uso ficou', 'arquivos em uso ficaram') : '') + '.' })) : null,
      ssd ? ssdCard(ssd) : null,
      h('div', { class: 'card', style: { marginTop: '16px' } },
        h('h2', null, icon('trash'), 'O que dá para apagar'),
        items.map(row),
        h('div', { class: 'row wrap sticky-foot' },
          btn(chosen.length ? 'Limpar ' + plural(chosen.length, 'item', 'itens') + (total ? ' · ' + BYTES(total) : '') : 'Limpar selecionados', run, 'primary', 'trash', { disabled: chosen.length ? null : 'disabled' }),
          h('div', { class: 'spacer' }), h('span', { class: 'small muted', text: 'Medido em ' + fmtDT(v.measuredAt) }))),
      h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('ban'), 'O que a Limpeza não apaga (e por quê)'),
        h('div', { class: 'checks' }, (v.notDone || []).map(c => checkItem(c)))));
  }

  // ---------------------------------------------------------------------------
  // Inicialização
  // ---------------------------------------------------------------------------

  const SVERDICT = { desativar: ['pode desativar', 'accent'], manter: ['mantenha', 'ok'], avaliar: ['avaliar', 'outline'] };

  async function pageStartup() {
    const v = await get('startup', '/api/startup');
    const items = v.items || [];
    const set = async (ids, enabled) => {
      try {
        S.cache.startup = await api('POST', '/api/startup/set', { ids, enabled });
        toast(enabled ? 'Ligado de novo na inicialização.' : plural(ids.length, 'item desativado', 'itens desativados') + ' na inicialização. Vale a partir do próximo login; dá para desfazer no Histórico.', 'ok');
      } catch (e) { toast(e.message, 'bad'); delete S.cache.startup; }
      staleDiag();
      rerender();
    };
    const suggested = items.filter(it => it.enabled && it.verdict === 'desativar');
    const row = it => {
      const vd = SVERDICT[it.verdict] || [it.verdict, 'outline'];
      return h('div', { class: 'drv-offer switch-row' + (it.enabled ? '' : ' done') },
        h('div', { class: 'drv-check' }, toggle(it.enabled, on => set([it.id], on), false, it.enabled ? 'Desativar' : 'Ativar')),
        h('div', { class: 'drv-main' },
          h('div', { class: 'row wrap', style: { gap: '6px' } }, h('b', { text: it.name }), badge(it.category, 'outline'),
            it.enabled || it.verdict !== 'desativar' ? badge(it.enabled ? vd[0] : it.verdict === 'manter' ? 'recomendado ligar' : vd[0], vd[1]) : null,
            it.new ? h('span', { class: 'badge info', title: 'Não estava na lista na sua última visita a esta página. É só um aviso: nada foi bloqueado nem desativado.' }, 'novo desde a última visita') : null,
            it.byAxion ? badge('alterado pelo Axion', 'outline') : null,
            it.running ? badge('aberto agora', 'info') : null, it.missing ? badge('programa não existe mais', 'warn') : null),
          h('div', { class: 'small', text: it.advice }),
          h('div', { class: 'small muted mono ellipsis', title: it.command, text: it.sourceLabel + ' · ' + it.command })));
    };
    const on = items.filter(it => it.enabled), off = items.filter(it => !it.enabled);
    return h('div', { class: 'page' },
      pageHead('Inicialização', 'O que abre junto com o Windows e o que dá para desligar sem perder nada. Funciona como o Gerenciador de Tarefas: nada é apagado e dá para ligar de novo.',
        suggested.length ? [btn('Desativar os ' + suggested.length + ' sugeridos', async () => {
          const ok = await confirmDlg({ title: 'Desativar ' + plural(suggested.length, 'item', 'itens'), ok: 'Desativar', icon: 'check',
            body: h('div', null, h('ul', { class: 'notes' }, suggested.map(it => h('li', { text: it.name + ' — ' + it.category }))),
              h('p', { class: 'small muted', text: 'Eles continuam instalados e abrem normalmente quando você quiser usar. Vale a partir do próximo login.' })) });
          if (ok) set(suggested.map(it => it.id), false);
        }, 'primary', 'zap')] : []),
      v.newCount ? h('p', { class: 'small', style: { margin: '4px 0 0' }, text: plural(v.newCount, 'entrada nova', 'entradas novas') + ' desde a sua última visita a esta página. É só um aviso: nada foi desativado.' }) : null,
      h('div', { class: 'stats', style: { marginTop: '4px', gridTemplateColumns: 'repeat(3, 1fr)' } },
        h('div', { class: 'stat' }, h('div', { class: 'n', text: String(v.enabled) }), h('div', { class: 'l', text: 'ligados' })),
        h('div', { class: 'stat' }, h('div', { class: 'n', text: String(v.suggest) }), h('div', { class: 'l', text: 'sugeridos para desativar' })),
        h('div', { class: 'stat' }, h('div', { class: 'n', text: String(off.length) }), h('div', { class: 'l', text: 'desligados' }))),
      h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('play'), 'Ligados'),
        on.length ? on.map(row) : h('p', { class: 'small muted', text: 'Nada liga junto com o Windows.' })),
      off.length ? h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('ban'), 'Desligados'), off.map(row)) : null,
      h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('info'), 'O que isso muda'), h('p', { class: 'small', style: { margin: 0 }, text: v.honest }),
        h('div', { class: 'row wrap', style: { marginTop: '8px' } }, (v.sources || []).map(x => extLink(x.title, x.url)))));
  }

  // ---------------------------------------------------------------------------
  // Processos
  // ---------------------------------------------------------------------------

  const PCAT = { rgb: 'RGB/periférico', browser: 'Navegador', overlay: 'Overlay', capture: 'Captura', cloud: 'Nuvem', launcher: 'Launcher', other: 'App' };
  let procShowAll = false;

  async function pageProcesses() {
    const v = await get('processes', '/api/processes');
    const groups = v.groups || [];
    const user = groups.filter(g => g.user);
    const sys = groups.filter(g => !g.user);
    const close = async g => {
      const ok = await confirmDlg({ title: 'Fechar ' + g.label, ok: 'Fechar', danger: true, icon: 'x',
        text: 'Fecha ' + plural(g.count, 'processo', 'processos') + ' de ' + g.label + ' como "Finalizar tarefa": o que não foi salvo nele se perde.' });
      if (!ok) return;
      try {
        const r = await api('POST', '/api/processes/close', { name: g.name, pids: g.pids });
        const failed = (r.failed || []).length;
        if (failed) toast(plural(r.closed || 0, 'processo fechado', 'processos fechados') + '; ' + plural(failed, 'não pôde ser fechado', 'não puderam ser fechados') + ': ' + r.failed.join('; ') + '.', 'info', 9000);
        else toast(plural(r.closed || 0, 'processo fechado', 'processos fechados') + '.', 'ok');
      } catch (e) { toast(e.message, 'bad'); }
      delete S.cache.processes; staleDiag(); rerender();
    };
    const table = (list, closable) => h('div', { class: 'tbl-scroll' }, h('table', { class: 'table proc-table' },
      h('thead', null, h('tr', null, ['Programa', 'Memória', 'CPU', ''].map(x => h('th', { text: x })))),
      h('tbody', null, list.map(g => h('tr', null,
        h('td', null, h('div', null, h('b', { text: g.label }), g.count > 1 ? h('span', { class: 'small muted', text: ' ×' + g.count }) : null,
          g.category ? h('span', { style: { marginLeft: '6px' } }, badge(PCAT[g.category] || g.category, 'outline')) : null),
          g.path ? h('div', { class: 'small muted mono ellipsis', style: { maxWidth: '520px' }, title: g.path, text: g.path }) : null),
        h('td', { text: BYTES(g.ram) }),
        h('td', { text: (g.cpu || 0).toFixed(1).replace('.', ',') + '%' }),
        h('td', null, closable && !g.protected ? h('button', { class: 'linkbtn small', type: 'button', text: 'Fechar', 'aria-label': 'Fechar ' + g.label, onclick: () => close(g) }) : h('span', { class: 'small muted', title: g.why || '', text: g.protected ? 'protegido' : '' })))))));
    return h('div', { class: 'page' },
      pageHead('Processos', 'O que está aberto e quanto usa de memória e CPU' + (v.at ? ' (lido às ' + new Date(v.at).toLocaleTimeString('pt-BR') + ')' : '') + '. Feche o que não precisa antes de jogar.',
        [btn('Atualizar', () => { delete S.cache.processes; rerender(); }, 'ghost', 'refresh')]),
      h('div', { class: 'stats', style: { gridTemplateColumns: 'repeat(3, 1fr)' } },
        h('div', { class: 'stat' }, h('div', { class: 'n', text: (v.cpuTotal || 0).toFixed(0) + '%' }), h('div', { class: 'l', text: 'CPU em uso' })),
        h('div', { class: 'stat' }, h('div', { class: 'n', text: BYTES(v.ramUsed) }), h('div', { class: 'l', text: 'memória em uso de ' + BYTES(v.ramTotal) })),
        h('div', { class: 'stat' }, h('div', { class: 'n', text: String(user.length) }), h('div', { class: 'l', text: 'programas seus abertos' }))),
      (v.heavy || []).length ? h('div', { class: 'callout warn', style: { marginTop: '12px' } }, icon('alert'),
        h('div', { text: 'Pesados em segundo plano: ' + v.heavy.join(', ') + '. Softwares de RGB fazem polling constante do hardware e navegadores disputam CPU e memória com o jogo.' })) : null,
      h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('layers'), 'Seus programas'), table(user, true)),
      h('div', { class: 'card', style: { marginTop: '16px' } },
        h('div', { class: 'row wrap' }, h('h2', { style: { margin: 0 } }, icon('windows'), 'Windows e serviços'), h('div', { class: 'spacer' }),
          sys.length > 10 ? h('button', { class: 'linkbtn small', type: 'button', text: procShowAll ? 'Mostrar só os 10 maiores' : 'Mostrar todos (' + sys.length + ')', onclick: () => { procShowAll = !procShowAll; rerender(); } }) : null),
        table(procShowAll ? sys : sys.slice(0, 10), false)),
      h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('info'), 'O que isso muda'), h('p', { class: 'small', style: { margin: 0 }, text: v.honest })));
  }

  // ---------------------------------------------------------------------------
  // Programas (desinstalador)
  // ---------------------------------------------------------------------------

  let progQuery = '';
  let progSort = 'name';

  function leftoversDialog(res) {
    const items = res.leftovers || [];
    if (!res.removed) {
      const m = modal({ title: res.name, body: h('div', { class: 'callout warn' }, icon('alert'), h('div', { text: res.note || 'O programa continua instalado.' })), actions: [btn('OK', () => m.close(), 'primary')] });
      return;
    }
    if (!items.length) {
      toast(res.name + ' desinstalado. Nenhuma sobra encontrada.', 'ok', 6000);
      return;
    }
    const sel = new Set(items.filter(l => l.default).map(l => l.id));
    const KIND = { dir: 'Pasta', file: 'Arquivo', regkey: 'Registro', runvalue: 'Inicialização', task: 'Tarefa' };
    const err = h('div', { class: 'callout warn hidden', role: 'alert', style: { marginTop: '10px' } });
    const label = h('span');
    const moveBtn = h('button', { class: 'btn primary', type: 'button' }, icon('trash'), label);
    const upd = () => { label.textContent = sel.size ? 'Mover ' + plural(sel.size, 'item', 'itens') + ' para a quarentena' : 'Marque o que mover'; moveBtn.disabled = !sel.size; };
    const list = h('div', null, items.map(l => {
      const input = h('input', { type: 'checkbox', checked: sel.has(l.id) ? 'checked' : null, 'aria-label': 'Selecionar ' + l.label });
      input.addEventListener('change', () => { if (input.checked) sel.add(l.id); else sel.delete(l.id); upd(); });
      return h('div', { class: 'drv-offer' }, h('label', { class: 'drv-check' }, input),
        h('div', { class: 'drv-main' }, h('div', { class: 'row wrap', style: { gap: '6px' } }, badge(KIND[l.kind] || l.kind, 'outline'), l.bytes ? h('b', { text: BYTES(l.bytes) }) : null),
          h('div', { class: 'small mono', style: { overflowWrap: 'anywhere' }, text: l.label }), h('div', { class: 'small muted', text: l.why })));
    }));
    let busy = false;
    moveBtn.addEventListener('click', async () => {
      if (busy) return;
      if (!sel.size) { err.replaceChildren(icon('alert'), h('div', { text: 'Marque pelo menos um item para mover para a quarentena.' })); err.classList.remove('hidden'); return; }
      busy = true; moveBtn.disabled = true; label.textContent = 'Movendo…';
      try {
        const b = await api('POST', '/api/programs/quarantine', { id: res.id, items: [...sel] });
        m.close(); invalidate(); rerender();
        toast(plural((b.items || []).length, 'item movido', 'itens movidos') + ' para a quarentena' + ((b.errors || []).length ? '; ' + plural(b.errors.length, 'falhou', 'falharam') + ': ' + b.errors.join('; ') : '') + '.', (b.errors || []).length ? 'info' : 'ok', (b.errors || []).length ? 12000 : 6000);
      } catch (e) {
        // Keep the dialog open: the choice is not lost and the user can retry.
        err.replaceChildren(icon('alert'), h('div', { text: 'Não foi possível mover: ' + e.message }));
        err.classList.remove('hidden');
        busy = false; upd();
      }
    });
    upd();
    const m = modal({ title: res.name + ' desinstalado: ' + plural(items.length, 'sobra encontrada', 'sobras encontradas'), wide: true,
      body: h('div', null, h('p', { class: 'small', text: 'Os itens marcados vão para a quarentena por 30 dias (dá para restaurar). Tarefas são só desativadas.' }), list, err),
      actions: [btn('Manter tudo', () => m.close(), 'ghost'), moveBtn] });
  }

  async function rescanLeftovers(r) {
    try {
      leftoversDialog(await api('POST', '/api/programs/leftovers', { id: r.id }));
    } catch (e) { toast(e.message, 'bad'); }
  }

  // ---------------------------------------------------------------------------
  // Instalar programas (catálogo winget)
  // ---------------------------------------------------------------------------

  const appsChecks = new Map(); // item id -> checked, kept across re-renders
  let appsChecksFrom = '';      // which preset filled the map ('' = not yet)
  let appsChecksProfile = '';   // the profile the map was filled for

  async function appsRefresh() {
    await runJob(api('POST', '/api/apps/refresh', {}), { title: 'Lendo os programas instalados' });
  }

  function appsPresetFill(v, mode) {
    appsChecks.clear();
    for (const it of v.items) {
      const canInstall = (it.winget || it.store) && (!it.installed || it.available);
      if (!canInstall) continue;
      let on = false;
      if (mode === 'perfil') on = it.preset === 'marcado';
      else if (mode === 'base') on = it.category === 'base';
      else if (mode === 'salva') on = !!it.selected;
      else if (mode === 'atualizar') on = !!it.available;
      appsChecks.set(it.id, on);
    }
    appsChecksFrom = mode;
    appsChecksProfile = v.profile;
  }

  async function appsInstall(v) {
    const ids = [...appsChecks.entries()].filter(([, on]) => on).map(([id]) => id);
    const items = ids.map(id => v.items.find(x => x.id === id)).filter(Boolean);
    if (!items.length) { toast('Marque pelo menos um programa.', 'info'); return; }
    const paid = items.filter(it => it.paid);
    const ok = await confirmDlg({
      title: 'Instalar programas', ok: items.length === 1 ? 'Instalar 1 programa' : 'Instalar ' + items.length + ' programas', icon: 'download', wide: true,
      body: h('div', null,
        h('p', { class: 'small', style: { marginTop: 0 }, text: 'O winget vai baixar o instalador oficial de cada um, conferir o hash com o catálogo da Microsoft (itens da Store passam pela própria Store) e instalar em silêncio, um por vez, aceitando por você os termos da fonte e a licença de cada pacote. Cancelar não interrompe o instalador que estiver rodando: ele termina e a tarefa para antes do próximo.' }),
        h('ul', { class: 'list-clean' }, items.map(it => h('li', null, h('b', { text: it.name }), ' — ', h('span', { class: 'muted', text: it.available && it.installed ? 'atualizar para ' + it.available : (it.store ? 'Microsoft Store' : it.winget) })))),
        paid.length ? h('div', { class: 'callout', style: { marginTop: '12px' } }, icon('info'), h('div', { text: 'Pago: ' + paid.map(it => it.name).join(', ') + '. O Axion instala o aplicativo; a licença é a sua conta.' })) : null,
        h('p', { class: 'small muted', text: 'A seleção fica salva neste PC (C:\\ProgramData\\Axion) e entra no relatório exportado em Sobre, para você ter a lista em mãos depois de formatar; aqui, o perfil remarca o essencial.' })),
    });
    if (!ok) return;
    try { await api('POST', '/api/apps/select', { ids }); } catch (e) { /* the selection is a convenience */ }
    await runJob(api('POST', '/api/apps/install', { ids }), { title: 'Instalando programas' });
  }

  async function appsUpgradeAll(v) {
    const list = v.upgradeList || [];
    const ok = await confirmDlg({ title: 'Atualizar tudo', ok: 'Atualizar', icon: 'refresh', wide: list.length > 6,
      body: h('div', null, h('p', { style: { marginTop: 0 }, text: 'winget upgrade --all: cada programa que o winget conhece neste PC vai para a versão mais nova do catálogo, esteja ou não na lista do Axion (' + plural(v.upgrades, 'atualização disponível', 'atualizações disponíveis') + '). Programas abertos podem pedir para fechar; o que o winget não reconhece fica como está.' }),
        list.length ? h('ul', { class: 'list-clean small' }, list.map(x => h('li', null, h('b', { text: x.id }), x.version ? ' ' + x.version : '', ' → ' + x.available))) : null,
        h('p', { class: 'small muted', text: 'Enquanto os instaladores rodam a tarefa não pode ser cancelada.' })) });
    if (!ok) return;
    await runJob(api('POST', '/api/apps/upgrade', {}), { title: 'Atualizando todos os programas' });
  }

  async function pageApps() {
    const [v, pr] = await Promise.all([get('apps', '/api/apps'), profiles()]);
    const st = S.state;
    const busy = !!(S.busy || st.job);
    if (!appsChecksFrom) appsPresetFill(v, v.hasSelection ? 'salva' : 'perfil');
    else if (appsChecksProfile !== v.profile) appsPresetFill(v, 'perfil'); // the profile changed: its presets again
    // Items that became installed (no update) leave the map; new rows get their preset.
    for (const it of v.items) {
      const canInstall = (it.winget || it.store) && (!it.installed || it.available);
      if (!canInstall) appsChecks.delete(it.id);
      else if (!appsChecks.has(it.id)) appsChecks.set(it.id, false);
    }
    const nChecked = [...appsChecks.values()].filter(Boolean).length;
    const label = h('span', { text: 'Instalar marcados (' + nChecked + ')' });
    const blocked = !v.winget.available || busy || !!v.warning;
    const installB = h('button', { class: 'btn primary', type: 'button', disabled: blocked || !nChecked, onclick: () => appsInstall(v) }, icon('download'), label);
    const upd = () => { const n = [...appsChecks.values()].filter(Boolean).length; label.textContent = 'Instalar marcados (' + n + ')'; installB.disabled = !v.winget.available || busy || !n || !!v.warning; };
    const profName = profileOf(v.profile, pr.profiles).title;
    const itemRow = it => {
      const canInstall = (it.winget || it.store) && (!it.installed || it.available);
      // Most important first; only two show, the rest is in the tooltip.
      const all = [];
      if (it.installed) all.push([it.available ? 'instalado ' + (it.version || '') + ' → ' + it.available + ' disponível' : 'instalado' + (it.version ? ' ' + it.version : ''), it.available ? 'info' : 'ok', it.available ? 'refresh' : 'check']);
      if (v.detected && v.detected[it.id]) all.push([v.detected[it.id], 'ok']);
      if (it.paid) all.push(['pago', 'warn']);
      if (it.winget && !it.verified) all.push(['ID não conferido', 'warn']);
      if (it.heavy) all.push(['pesado em segundo plano', 'warn']);
      if (it.recommended) all.push(['recomendado', 'accent']);
      else if (it.preset === 'sugerido') all.push(['sugestão do perfil', 'outline']);
      if (it.store) all.push(['Microsoft Store', 'outline']);
      if (it.manual) all.push(['link oficial', 'outline']);
      const badges = all.slice(0, 2).map(b => badge(b[0], b[1], b[2]));
      const more = all.slice(2).map(b => b[0]).join(' · ');
      const body = h('div', null,
        h('div', { class: 'pt', title: more || null }, h('span', { text: it.name }), badges),
        h('div', { class: 'pr', text: it.for + (it.winget ? ' · ' + it.winget : '') }),
        it.note ? h('div', { class: 'pr muted', text: it.note }) : null);
      if (it.manual) return h('div', { class: 'plan-item link optional' }, body, btn('Abrir', () => openURL(it.manual), 'sm', 'ext'));
      let left;
      if (!canInstall) left = h('input', { type: 'checkbox', checked: true, disabled: true, 'aria-label': it.name + ' (instalado)' });
      else {
        const cb = h('input', { type: 'checkbox', checked: !!appsChecks.get(it.id), 'aria-label': it.name });
        cb.addEventListener('change', () => { appsChecks.set(it.id, cb.checked); upd(); });
        left = cb;
      }
      return h(canInstall ? 'label' : 'div', { class: 'plan-item' + (canInstall ? '' : ' optional') }, left, body);
    };
    const cats = v.categories.map(c => {
      const items = v.items.filter(it => it.category === c.id);
      const n = items.filter(it => appsChecks.get(it.id)).length;
      return h('div', { class: 'card', style: { marginBottom: '14px' } },
        h('h3', { style: { marginBottom: c.note ? '4px' : '10px' } }, c.title, n ? badge(plural(n, 'marcado', 'marcados'), 'accent') : null),
        c.note ? h('div', { class: 'small muted', style: { marginBottom: '10px' }, text: c.note }) : null,
        h('div', { class: 'apps-grid' }, items.map(itemRow)));
    });
    const preset = (label, mode, cls) => btn(label, () => { appsPresetFill(v, mode); rerender(); }, cls || 'sm', null);
    return h('div', { class: 'page' },
      pageHead('Instalar programas', 'Marque o que quer e o Axion instala a versão mais atual de cada um pelo winget, o gerenciador de pacotes que já vem no Windows. Depois de formatar, é por aqui que o PC volta a ter tudo.' + (v.scanned ? ' Lista de instalados lida ' + ago(v.scannedAt) + '.' : ''),
        [installB, v.upgrades ? btn('Atualizar tudo (' + v.upgrades + ')', () => appsUpgradeAll(v), '', 'refresh', { disabled: blocked }) : null, v.scanned ? btn('Reler instalados', appsRefresh, 'ghost', 'refresh', { disabled: !v.winget.available || busy }) : null]),
      !v.scanned && v.winget.available && !v.scanError ? h('div', { class: 'callout', style: { marginBottom: '14px' } }, icon('info'), h('div', { class: 'apps-read' },
        h('div', null, h('b', { text: 'O Axion ainda não leu o que está instalado. ' }), 'Com a leitura, o que já existe aparece marcado em cinza e o que tem versão nova pode ser atualizado. Ela roda o winget, que usa a internet para atualizar o índice do catálogo (leva de alguns segundos a um minuto).'),
        btn('Ler o que já está instalado', appsRefresh, 'sm', 'refresh', { disabled: busy }))) : null,
      !v.winget.available ? h('div', { class: 'callout warn', style: { marginBottom: '14px' } }, icon('alert'), h('div', null, h('div', { text: v.winget.why }), h('div', { style: { marginTop: '8px' } }, btn('Instalar o Instalador de Aplicativo (Microsoft)', () => openURL('https://aka.ms/getwinget'), 'sm', 'ext')))) : null,
      v.warning ? h('div', { class: 'callout warn', style: { marginBottom: '14px' } }, icon('alert'), h('div', { text: v.warning })) : null,
      v.scanError ? h('div', { class: 'callout warn', style: { marginBottom: '14px' } }, icon('alert'), h('div', { text: 'A leitura dos instalados falhou: ' + v.scanError })) : null,
      h('div', { class: 'card', style: { marginBottom: '14px' } },
        h('div', { class: 'row wrap' }, h('span', { class: 'small' }, 'Perfil ', h('b', { text: profName }), ' · marcar: '),
          preset('os do meu perfil', 'perfil'), preset('só a base do sistema', 'base'), v.hasSelection ? preset('a seleção salva', 'salva') : null, v.upgrades ? preset('só as atualizações', 'atualizar') : null,
          h('button', { class: 'linkbtn small', type: 'button', text: 'Desmarcar todos', onclick: () => { appsChecks.forEach((_, k) => appsChecks.set(k, false)); rerender(); } })),
        h('div', { class: 'small muted', style: { marginTop: '8px' }, text: 'A primeira linha de cada categoria é a opção recomendada; as demais são alternativas. O que já está instalado aparece marcado em cinza; com atualização disponível, dá para marcar para atualizar. Itens com "link oficial" não estão no winget: o botão abre a página do fabricante no seu navegador.' })),
      cats,
      h('div', { class: 'card' }, h('h3', null, icon('info'), 'Como funciona e limites'), h('div', { class: 'small', text: v.honest }),
        h('div', { class: 'row wrap', style: { marginTop: '10px' } }, (v.sources || []).map(x => extLink(x.title, x.url)))),
      (v.log || []).length ? h('details', { class: 'card', style: { marginTop: '14px' } }, h('summary', { class: 'small', text: 'Instalações feitas pelo Axion (' + v.log.length + ')' }),
        h('table', { class: 'table', style: { marginTop: '10px' } }, h('tbody', null, v.log.map(e => h('tr', null, h('td', { text: fmtDT(e.at) }), h('td', { text: e.name }), h('td', { text: { install: 'instalação', upgrade: 'atualização', 'upgrade-all': 'atualizar tudo', skipped: 'já instalado' }[e.action] || e.action }), h('td', null, badge(e.outcome, e.ok ? 'ok' : 'bad'))))))) : null);
  }

  async function pagePrograms() {
    const v = await get('programs', '/api/programs');
    const q = progQuery.trim().toLowerCase();
    let list = (v.programs || []).filter(p => !q || (p.name + ' ' + (p.publisher || '')).toLowerCase().includes(q));
    const sorters = { name: (a, b) => a.name.localeCompare(b.name, 'pt-BR'), size: (a, b) => (b.sizeBytes || 0) - (a.sizeBytes || 0), date: (a, b) => String(b.installDate).localeCompare(String(a.installDate)) };
    list = list.slice().sort(sorters[progSort] || sorters.name);
    const uninstall = async p => {
      const ok = await confirmDlg({ title: 'Desinstalar ' + p.name, ok: 'Abrir o desinstalador', icon: 'trash',
        body: h('div', null, h('p', { text: 'O desinstalador do próprio programa vai abrir: siga as telas dele. Quando terminar, o Axion procura o que ficou para trás e mostra antes de mover qualquer coisa.' }),
          p.scope === 'user' ? h('p', { class: 'small muted', text: 'Programa instalado só para a sua conta: o desinstalador roda com os seus direitos normais, sem administrador.' }) : null) });
      if (!ok) return;
      const job = await runJob(api('POST', '/api/programs/uninstall', { id: p.id }), { title: 'Desinstalando ' + p.name });
      if (job && job.status === 'done' && job.result) leftoversDialog(job.result);
    };
    const search = h('input', { class: 'search', type: 'search', placeholder: 'Buscar programa ou fabricante', value: progQuery, 'aria-label': 'Buscar programa', 'data-focus': 'prog-search' });
    search.addEventListener('input', () => { progQuery = search.value; clearTimeout(pagePrograms.t); pagePrograms.t = setTimeout(() => rerender(), 180); });
    const sortSel = h('select', { class: 'select', 'aria-label': 'Ordenar' }, [['name', 'Nome'], ['size', 'Tamanho'], ['date', 'Data de instalação']].map(([k, l]) => h('option', { value: k, selected: progSort === k ? 'selected' : null, text: 'Ordenar: ' + l })));
    sortSel.addEventListener('change', () => { progSort = sortSel.value; rerender(); });
    const quarantine = v.quarantine || [];
    const recent = v.recent || [];
    return h('div', { class: 'page' },
      pageHead('Programas', 'Desinstale com o desinstalador do próprio programa e remova o que ele deixa para trás, com quarentena de 30 dias.', []),
      recent.length ? h('div', { class: 'card', style: { marginBottom: '16px' } }, h('h2', null, icon('trash'), 'Desinstalados agora há pouco'),
        h('p', { class: 'small muted', style: { marginTop: 0 }, text: 'Se o desinstalador terminou depois de você cancelar a espera, ou se preferiu decidir depois, procure as sobras de novo.' }),
        recent.map(r => h('div', { class: 'set-row' }, h('div', null, h('b', { text: r.name }), h('div', { class: 'small muted', text: fmtDT(r.at) + (r.found ? ' · ' + plural(r.found, 'sobra encontrada', 'sobras encontradas') + ' na última busca' : '') })),
          btn('Procurar sobras de novo', () => rescanLeftovers(r), 'sm', 'refresh')))) : null,
      h('div', { class: 'row wrap', style: { marginBottom: '12px' } }, search, sortSel, h('div', { class: 'spacer' }), h('span', { class: 'small muted', text: plural(list.length, 'programa', 'programas') })),
      !list.length ? h('div', { class: 'card' }, empty('folder', q ? 'Nenhum programa encontrado' : 'Nenhum programa listado', q ? 'Nada com "' + progQuery.trim() + '" no nome ou no fabricante.' : 'O Windows não informou programas instalados.',
        q ? btn('Limpar a busca', () => { progQuery = ''; rerender(); }, 'sm') : null)) :
      h('div', { class: 'card' }, h('div', { class: 'tbl-scroll' }, h('table', { class: 'table' },
        h('thead', null, h('tr', null, ['Programa', 'Versão', 'Tamanho', 'Instalado em', ''].map(x => h('th', { text: x })))),
        h('tbody', null, list.map(p => h('tr', null,
          h('td', null, h('div', null, h('b', { text: p.name }), p.scope === 'user' ? h('span', { style: { marginLeft: '6px' } }, badge('só sua conta', 'outline')) : null),
            p.publisher ? h('div', { class: 'small muted', text: p.publisher }) : null),
          h('td', { class: 'small', text: p.version || '—' }),
          h('td', { class: 'small', text: p.sizeBytes ? BYTES(p.sizeBytes) : '—' }),
          h('td', { class: 'small', text: hasDate(p.installDate) ? fmtDay(p.installDate) : '—' }),
          h('td', null, p.noRemove ? h('span', { class: 'small muted', text: 'não removível' }) : h('button', { class: 'linkbtn small', type: 'button', text: 'Desinstalar', 'aria-label': 'Desinstalar ' + p.name, onclick: () => uninstall(p) })))))))),
      h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('restore'), 'Quarentena'),
        quarantine.length ? quarantine.map(b => h('div', { class: 'set-row' },
          h('div', null, h('div', { class: 'row wrap', style: { gap: '6px' } }, h('b', { text: b.program }), b.verified ? null : badge('não criado por esta instalação', 'warn')),
            h('div', { class: 'small muted', text: fmtDT(b.at) + ' · ' + plural((b.items || []).length, 'item', 'itens') + (b.bytes ? ' · ' + BYTES(b.bytes) : '') + ' · apagado sozinho após 30 dias' })),
          h('div', { class: 'row' },
            b.verified ? btn('Restaurar', async () => { try { const r = await api('POST', '/api/programs/restore', { batch: b.id }); toast((r.errors || []).length ? 'Restaurado com avisos: ' + r.errors.join('; ') : 'Restaurado.', (r.errors || []).length ? 'info' : 'ok', 7000); } catch (e) { toast(e.message, 'bad'); } invalidate(); rerender(); }, 'sm', 'undo') : null,
            btn('Apagar agora', async () => {
              if (!(await confirmDlg({ title: 'Apagar de vez?', text: 'Os itens deste lote serão apagados definitivamente.', ok: 'Apagar', danger: true, icon: 'trash' }))) return;
              try { await api('POST', '/api/programs/purge', { batch: b.id }); } catch (e) { toast(e.message, 'bad'); } invalidate(); rerender();
            }, 'sm ghost', 'trash')))) : h('p', { class: 'small muted', text: 'Vazia. Sobras removidas pelo Axion ficam aqui por 30 dias.' })),
      h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('info'), 'Como funciona'), h('p', { class: 'small', style: { margin: 0 }, text: v.honest }),
        h('div', { class: 'row wrap', style: { marginTop: '8px' } }, (v.sources || []).map(x => extLink(x.title, x.url)))));
  }

  async function pageBios() {
    const b = await get('bios', '/api/bios');
    return h('div', { class: 'page' },
      pageHead('Guia de BIOS', b.board + ' · BIOS ' + b.bios + (b.firmware ? ' · boot ' + b.firmware : ''),
        b.support ? [btn('Suporte da ' + b.vendor, () => openURL(b.support), '', 'ext')] : []),
      h('div', { class: 'callout warn' }, icon('alert'), h('div', { text: 'O Axion não altera a BIOS. Estes ajustes só podem ser feitos nela; o status abaixo vem do que o Windows consegue ler.' })),
      h('div', { class: 'grid g2', style: { marginTop: '16px' } },
        h('div', { class: 'card' }, h('h2', null, icon('chip'), 'Como entrar na BIOS'), h('p', { class: 'small', style: { margin: 0 }, text: b.enter })),
        h('div', { class: 'card' }, h('h2', null, icon('info'), 'Antes de mexer'), h('p', { class: 'small', style: { margin: 0 }, text: b.note }))),
      h('div', { class: 'checks', style: { marginTop: '16px' } }, (b.items || []).map(c => checkItem(c))));
  }

  async function pageHist() {
    const d = await get('history', '/api/history');
    const entries = d.entries || [];
    const isActive = e => e.active === undefined ? ((e.status === 'applied' || e.status === 'interrupted') && e.changed) : e.active;
    const active = entries.filter(isActive);
    const ST = { applied: ['ativa', 'ok'], reverted: ['desfeita', 'outline'], failed: ['falhou (revertida)', 'bad'], superseded: ['substituída', 'outline'], 'partial-revert': ['desfeita parcialmente', 'warn'], interrupted: ['interrompida', 'warn'], applying: ['aplicando', 'info'] };
    let day = '';
    let batch = '';
    const rows = [];
    const batchSize = {};
    entries.forEach(e => { if (e.batch) batchSize[e.batch] = (batchSize[e.batch] || 0) + 1; });
    for (const e of entries) {
      const dd = fmtDate(e.time);
      if (dd !== day) { day = dd; batch = ''; rows.push(h('div', { class: 'day', text: dd })); }
      if (e.batch && e.batch !== batch && e.batchLabel && batchSize[e.batch] > 1) {
        batch = e.batch;
        const act = entries.filter(x => x.batch === e.batch && isActive(x));
        rows.push(h('div', { class: 'row between', style: { margin: '10px 2px 2px' } },
          h('div', { class: 'row' }, icon('layers'), h('b', { text: e.batchLabel }), h('span', { class: 'small muted', text: fmtTime(e.time) + ' · ' + plural(batchSize[e.batch], 'item', 'itens') })),
          act.length ? btn('Desfazer este lote (' + act.length + ')', async () => {
            const ok = await confirmDlg({ title: 'Desfazer lote', text: 'Restaurar os valores anteriores de ' + plural(act.length, 'alteração', 'alterações') + ' de "' + e.batchLabel + '"?', ok: 'Desfazer', danger: true, icon: 'undo' });
            if (ok) runJob(api('POST', '/api/revert', { entries: act.map(x => x.id) }), { title: 'Desfazendo: ' + e.batchLabel });
          }, 'sm ghost', 'undo') : null));
      } else if (!e.batchLabel || batchSize[e.batch] <= 1) {
        batch = '';
      }
      const s = e.pending ? (e.status === 'failed' ? ['falhou — desfazer pendente', 'bad'] : ['desfeita em parte — falta desfazer', 'warn']) : (ST[e.status] || [e.status, 'outline']);
      const dot = e.pending || e.status === 'partial-revert' || e.status === 'interrupted' ? ' partial' : e.status === 'failed' ? ' failed' : e.status === 'reverted' || e.status === 'superseded' ? ' undone' : '';
      rows.push(h('div', { class: 'hentry' + dot },
        h('div', { class: 'hh' }, h('span', { class: 'time', text: fmtTime(e.time) }), h('span', { class: 'ttl', text: e.title }),
          badge(s[0], s[1]), e.drifted ? badge('alterada depois', 'warn', 'alert') : null, rebootBadge(e.reboot),
          e.legacy && isActive(e) ? h('span', { title: 'Registro de uma versão anterior à 0.3 (ou alterado fora do Axion): Desfazer só restaura o que o catálogo atual descreve; correções não são desfeitas a partir dele.' }, badge('versão anterior', 'outline')) : null,
          h('div', { class: 'right' }, isActive(e) ? btn(e.pending ? 'Tentar desfazer de novo' : 'Desfazer', () => runJob(api('POST', '/api/revert', { entries: [e.id] }), { title: 'Desfazendo: ' + e.title }), 'sm', 'undo') : null)),
        e.error || e.revertNote ? h('div', { class: 'small muted', style: { marginTop: '6px' }, text: e.error || e.revertNote }) : null,
        e.revertedAt ? h('div', { class: 'tiny faint', style: { marginTop: '4px' }, text: 'Desfeita em ' + fmtDT(e.revertedAt) }) : null,
        h('details', { style: { marginTop: '8px' } }, h('summary', { class: 'small linkbtn', text: 'Valores anteriores (' + (e.ops || []).filter(o => !o.skipped).length + ')' }),
          h('ul', { class: 'ops', style: { marginTop: '8px' } }, (e.ops || []).filter(o => !o.skipped).map(o => h('li', { class: o.verified ? 'in' : 'out' }, o.describe, h('div', { class: 'cur', text: 'antes: ' + (o.before || '—') + (o.error ? ' · ' + o.error : '') })))))));
    }
    return h('div', { class: 'page' },
      pageHead('Histórico', 'Cada alteração guarda o valor anterior exato. Desfazer restaura esse valor e confirma pela releitura; se outro programa mexeu depois, o Axion avisa antes de sobrescrever.',
        [btn('Criar ponto de restauração', () => runJob(api('POST', '/api/restorepoint', {}), { title: 'Ponto de restauração' }), '', 'shield'),
          btn('Restauração do Sistema', () => openURL('tool:rstrui'), 'ghost', 'restore'),
          btn('Desfazer tudo (' + active.length + ')', async () => {
            const ok = await confirmDlg({ title: 'Desfazer tudo', text: 'Restaurar os valores anteriores de ' + plural(active.length, 'alteração ativa', 'alterações ativas') + ' feitas pelo Axion?', ok: 'Desfazer tudo', danger: true, icon: 'undo' });
            if (ok) runJob(api('POST', '/api/revert', { all: true }), { title: 'Desfazendo tudo' });
          }, 'danger', 'undo', { disabled: !active.length })]),
      entries.length ? h('div', { class: 'hist' }, rows) : empty('clock', 'Nenhuma alteração ainda', 'Quando você aplicar otimizações, cada mudança aparece aqui com o valor anterior.'));
  }

  async function pageExcl() {
    const list = await get('excluded', '/api/excluded');
    // Badge color by what the verdict says: harmful/risky, myth/unproven,
    // no effect, or out of reach.
    const vkind = v => /^(prejudicial|risco|não executar|incompatível)/.test(v) ? 'bad'
      : /^(global demais|pode piorar)/.test(v) ? 'warn'
      : /^(impossível|não suportado|amplo)/.test(v) ? 'info' : 'outline';
    return h('div', { class: 'page' },
      pageHead('O que não fazemos', 'O Axion recusa estes ajustes de propósito. Cada um tem o mecanismo, o motivo e a fonte.'),
      h('div', { class: 'grid g2' }, list.map(x => h('div', { class: 'card' },
        h('div', { class: 'excl-head' }, h('h3', { style: { margin: 0 }, text: x.name }), badge(x.verdict, vkind(x.verdict))),
        x.mechanism ? h('div', { class: 'small', style: { marginTop: '10px' } }, h('b', { text: 'Mecanismo: ' }), x.mechanism) : null,
        h('div', { class: 'small', style: { marginTop: '8px', color: 'var(--text-2)' }, text: x.reason }),
        x.sources && x.sources.length ? h('div', { class: 'src', style: { marginTop: '10px' } }, x.sources.map(s => extLink(s.title, s.url))) : null))));
  }

  async function pageSobre() {
    const st = S.state;
    const lvl = (t, d) => h('div', { class: 'row', style: { alignItems: 'flex-start' } }, t, h('div', { class: 'small', text: d }));
    return h('div', { class: 'page' },
      pageHead('Sobre e metodologia', 'Como o Axion decide o que fazer no seu PC.'),
      h('div', { class: 'card about-brand signature' },
        h('img', { src: 'logo-256.png', alt: '', width: '96', height: '96', decoding: 'async' }),
        h('div', null, h('div', { class: 'brand-name', text: 'AXION' }), h('div', { class: 'eyebrow', style: { margin: '2px 0 0' }, text: 'Game Optimizer' }),
          h('div', { class: 'mono small muted', style: { marginTop: '4px' }, text: /^\d/.test(st.version || '') ? 'v' + st.version : (st.version || '') }),
          h('p', { text: 'Otimização com evidência, desfazer exato e sem risco de anti-cheat.' }))),
      h('div', { class: 'grid g2' },
        h('div', { class: 'card' }, h('h2', null, icon('book'), 'Como funciona'),
          h('ol', { class: 'list-clean small' },
            h('li', { text: 'Análise: lê hardware, Windows, drivers, BIOS/boot, rede, segurança, anti-cheats e jogos. Nada é alterado.' }),
            h('li', { text: 'Diagnóstico: cada ponto traz a evidência lida do PC, a explicação e a correção.' }),
            h('li', { text: 'Perfil de uso: a nota, o 1 clique e a avaliação seguem o que você usa o PC para fazer; pontos que só fazem diferença em jogo não contam num PC de trabalho, e continuam visíveis à parte.' }),
            h('li', { text: 'Avaliação completa: três notas com a mesma régua — configuração (o diagnóstico), capacidade (o hardware contra os requisitos publicados dos programas do seu uso) e medição (rede, FPS e mouse medidos neste PC). O que não foi medido não entra na nota.' }),
            h('li', { text: 'Recomendação por máquina: o mesmo ajuste pode ser recomendado num PC e bloqueado em outro (ex.: VBS com FACEIT, núcleos estacionados em X3D de dois CCDs).' }),
            h('li', { text: 'Aplicação: antes de cada mudança o valor atual é guardado; depois, o valor é relido para confirmar. Se algo falha, o lote do item é desfeito.' }),
            h('li', { text: 'Reversão: o Histórico restaura o valor anterior exato e avisa se outro programa mudou o item depois.' }),
            h('li', { text: 'Periféricos: a porta USB de cada mouse e teclado é lida do driver do hub (velocidade e bInterval), e a medição de polling usa raw input e, como administrador, os eventos ETW do próprio driver USB do Windows.' }),
            h('li', { text: 'Desempenho: o PresentMon, da Intel, registra cada quadro do jogo pelos eventos ETW do Windows; o Axion calcula FPS, 1% e 0,1% low, engasgos e o que limitou cada quadro, e compara grupos de medições com o teste t de Welch.' }),
            h('li', { text: 'Atraso de entrada: soma o polling medido, a latência do PC medida pelo PresentMon e a varredura do monitor, e mostra quanto cada mudança ainda tiraria.' }),
            h('li', { text: 'Rede: mede latência, oscilação e perda em cada trecho até os relays da Valve, o bufferbloat com a conexão cheia e o DNS, e compara antes × depois.' }),
            h('li', { text: 'Relatório: anônimo por padrão (nome do PC, conta, endereços e números de série substituídos por marcadores); o completo só por escolha. A versão em página (HTML) traz as mesmas evidências e fontes.' }),
            h('li', { text: 'Instalar programas: o Axion não traz instalador nenhum; ele roda o winget da Microsoft, que baixa cada programa do próprio fabricante e confere o hash com o catálogo. Nada é instalado sem você marcar e confirmar, e o que foi instalado fica registrado.' }))),
        h('div', { class: 'card' }, h('h2', null, icon('shield'), 'Níveis de evidência e risco'),
          h('div', { class: 'stack' },
            lvl(evBadge('alta'), 'Documentado pelo fabricante (Microsoft, NVIDIA, AMD, Intel) ou medido de forma consistente por testes independentes confiáveis.'),
            lvl(evBadge('média'), 'Mecanismo real e documentado; o benefício depende do cenário.'),
            lvl(evBadge('baixa'), 'Mecanismo plausível, benefício pequeno ou inconsistente. Nunca entra no modo recomendado.'),
            lvl(riskBadge('moderado'), 'Pode desligar algo que você usa (ex.: WSL2, telemetria do Programa Insider) ou tem efeito que depende do PC. Fica fora do 1 clique: só é aplicado na página do item, depois do aviso.'),
            lvl(riskBadge('avançado'), 'Troca desempenho por segurança ou estabilidade. Nunca entra no 1 clique e pede confirmação.'),
            h('p', { class: 'small muted', style: { margin: '4px 0 0' }, text: 'Os modos de 1 clique só aplicam itens sem risco.' })))),
      h('div', { class: 'grid g2', style: { marginTop: '16px' } },
        h('div', { class: 'card' }, h('h2', null, icon('folder'), 'Dados'),
          h('dl', { class: 'kv' }, h('dt', { text: 'Versão' }), h('dd', { text: st.version }), h('dt', { text: 'Pasta de dados' }), h('dd', { class: 'mono small', text: st.dataDir })),
          h('div', { class: 'row wrap', style: { marginTop: '14px' } }, btn('Abrir pasta', () => openURL('folder:' + st.dataDir), 'sm', 'folder'), btn('Exportar relatório', exportReport, 'sm', 'download'))),
        h('div', { class: 'card' }, h('h2', null, icon('rocket'), 'Próximos módulos'),
          h('ul', { class: 'list-clean small' },
            h('li', { text: 'Drivers pelo Windows Update e pelo fabricante; inventário de drivers; kit de programas exportável para um pendrive' }),
            h('li', { text: 'Wi-Fi: canal, banda e força do sinal no teste de rede; modo jogo no Wi-Fi' }),
            h('li', { text: 'Teste A/B com estatística; GPUs AMD e Intel; assinatura digital' })))));
  }



  // ---------------------------------------------------------------------------
  // Relatório da partida (sessão inteira, automático, opt-in por jogo)
  // ---------------------------------------------------------------------------

  let sessOpen = null;
  const SESS_REASON = { fim: 'o jogo fechou', limite: 'limite de tempo do Axion', fechado: 'o Axion foi fechado durante o jogo', desligado: 'relatório desligado durante o jogo', falha: 'o PresentMon falhou' };
  const fmtDur = s => !(s > 0) ? '—' : s >= 3600 ? Math.floor(s / 3600) + ' h ' + String(Math.floor(s % 3600 / 60)).padStart(2, '0') + ' min' : s < 60 ? Math.round(s) + ' s' : Math.round(s / 60) + ' min';

  function sessionFacts(r) {
    const si = r.session || {}, s = r.summary || {}, g = (r.sensors || {}).gpu;
    const cover = si.wallSecs > 0 ? Math.min(100, 100 * s.seconds / si.wallSecs) : -1;
    const rows = [
      ['Jogo', si.name + ' (' + r.process + ')'],
      ['Gravado', fmtDur(s.seconds) + (cover >= 0 ? ' de ' + fmtDur(si.wallSecs) + ' (' + fmtN(cover, 0) + '% do tempo, desde o início da gravação até a última janela)' : '')],
      ['Janelas', (si.segments || []).length + ' de ' + fmtDur(si.segmentSecs) + (si.empty ? ' · ' + si.empty + ' sem quadros' : '') + (si.paused ? ' · gravação pausada por uma medição manual' : '')],
      ['Como terminou', SESS_REASON[si.reason] || si.reason || '—'],
      ['Latência do PC', s.pcLatencyAvg > 0 ? fmtN(s.pcLatencyAvg, 2) + ' ms em média' : 'não veio nesta partida: o PresentMon só calcula quando o jogo envia os marcadores de PC Latency (como os do NVIDIA Reflex)'],
      ['Placa de vídeo', g ? fmtN(g.avgTempC, 0) + ' °C de média, ' + g.maxTempC + ' °C no máximo · limite por temperatura em ' + fmtN(g.thermalPct, 0) + '% das leituras · freio de hardware em ' + fmtN(g.hwSlowdownPct, 0) + '% · no limite de potência em ' + fmtN(g.powerCapPct, 0) + '%' : 'sem leitura (só placas NVIDIA, pelo nvidia-smi)'],
    ];
    if (si.setting) rows.push(['Limite de FPS salvo no jogo', si.setting + ' (lido na última análise)']);
    return h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('gamepad'), 'Sobre esta partida'),
      h('dl', { class: 'kv' }, rows.map(([k, x]) => [h('dt', { text: k }), h('dd', { text: x })])));
  }

  async function pagePartidas() {
    const v = await get('sessions', '/api/sessions', true);
    const reps = v.reports || [];
    if (sessOpen && !reps.some(r => r.id === sessOpen)) sessOpen = null;
    let open = null;
    if (sessOpen) {
      try {
        open = await get('sess:' + sessOpen, '/api/sessions/run/' + sessOpen);
        if (open.run.session && !open.run.session.seen) api('POST', '/api/sessions/seen', { id: sessOpen }).then(() => refreshState()).catch(() => {});
      } catch (e) { sessOpen = null; }
    }
    const gameRow = gm => h('div', { class: 'row wrap', style: { gap: '10px', padding: '8px 0' } },
      toggle(gm.auto, async on => {
        // The first version was tested on CS2 (VAC): the other anti-cheats get a warning before it is turned on.
        if (on && gm.antiCheat && gm.antiCheat !== 'VAC') {
          const ok = await confirmDlg({ title: 'Relatório automático em jogo com ' + gm.antiCheat, ok: 'Ligar',
            body: h('p', { text: 'O Axion só usa o PresentMon (eventos do Windows) e a lista de processos, como na medição manual. Esta versão foi testada no CS2 (VAC), não com ' + gm.antiCheat + '.' }) });
          if (!ok) { rerender(); return; }
        }
        try { await api('POST', '/api/sessions/auto', { exe: gm.exe, on }); toast(on ? 'Relatório automático ligado para ' + gm.name + '.' : 'Relatório automático desligado para ' + gm.name + '.', 'ok'); }
        catch (e) { toast(e.message, 'bad'); }
        delete S.cache.sessions; refreshState(); rerender();
      }, !v.ready && !gm.auto, null, 'Relatório automático de ' + gm.name),
      h('div', null, h('b', { text: gm.name }), h('div', { class: 'tiny muted', text: gm.exe })),
      gm.competitive ? badge('competitivo', 'outline') : null,
      gm.antiCheat ? badge('anti-cheat: ' + gm.antiCheat, 'outline') : null,
      gm.running ? badge('aberto agora', 'game') : null);
    const row = r => {
      const s = r.summary || {}, si = r.session || {}, lim = LIMITER[s.limiter], g = (r.sensors || {}).gpu;
      const acts = h('div', { class: 'tiny', style: { marginTop: '2px' } },
        h('button', { class: 'linkbtn tiny', type: 'button', text: sessOpen === r.id ? 'fechar' : 'ver', onclick: () => { sessOpen = sessOpen === r.id ? null : r.id; rerender(); } }), ' · ',
        h('button', { class: 'linkbtn tiny', type: 'button', text: 'apagar', onclick: async () => {
          const ok = await confirmDlg({ title: 'Apagar relatório', ok: 'Apagar', danger: true, icon: 'trash', body: h('p', { text: 'O relatório da partida de ' + fmtDT(r.started) + ' será apagado.' }) });
          if (!ok) return;
          try { await api('POST', '/api/sessions/delete', { id: r.id }); if (sessOpen === r.id) sessOpen = null; invalidate(); refreshState(); rerender(); } catch (e) { toast(e.message, 'bad'); }
        } }));
      return h('tr', { class: sessOpen === r.id ? 'sel' : null },
        h('td', null, h('div', { text: fmtDT(r.started) }), acts),
        h('td', null, h('div', { text: si.name || r.process }), !si.seen ? badge('novo', 'game') : null,
          (r.remarks || []).some(x => x.level === 'warn') ? h('span', { title: (r.remarks || []).filter(x => x.level === 'warn').map(x => x.title).join('; ') }, badge('condições', 'warn', 'alert')) : null),
        h('td', { text: fmtDur(s.seconds) }), h('td', { text: fmtN(s.avgFps) }), h('td', { text: fmtN(s.low1Fps) }), h('td', { text: fmtN(s.low01Fps) }),
        h('td', { text: String(s.stutters || 0) }), h('td', null, lim ? badge(lim[0].split(' (')[0], lim[1]) : '—'), h('td', { text: g ? g.maxTempC + ' °C' : '—' }));
    };
    return h('div', { class: 'page' },
      pageHead('Relatório da partida', 'Com o relatório automático ligado para um jogo, o Axion grava a partida inteira com o PresentMon enquanto estiver aberto. Quando o jogo fecha, mostra FPS médio, 1% e 0,1% low, engasgos, o que limitou os quadros e a temperatura da placa de vídeo, e compara com a partida anterior do mesmo jogo. Vem desligado para todos os jogos.'),
      !v.ready ? h('div', { class: 'callout warn' }, icon('alert'), h('div', { text: 'Relatório automático indisponível: ' + (v.why || 'PresentMon não encontrado') })) : null,
      v.live ? h('div', { class: 'callout game' }, icon('play'), h('div', null, h('b', { text: 'Gravando a partida de ' + v.live.name + ' desde ' + fmtDT(v.live.started) }),
        h('div', { class: 'small muted', text: (v.live.waiting ? 'Em pausa: ' + v.live.waiting + '. ' : '') + fmtDur(v.live.seconds) + ' gravados até agora. Pode minimizar o Axion. Se fechar, o relatório vai só até ali.' }))) : null,
      h('div', { class: 'card' }, h('h2', null, icon('gamepad'), 'Relatório automático por jogo'),
        (v.games || []).length ? (v.games || []).map(gameRow) : empty('gamepad', 'Nenhum jogo na biblioteca', 'Adicione o jogo em Jogos › Biblioteca de jogos.'),
        h('ul', { class: 'notes small', style: { marginTop: '12px' } },
          h('li', { text: 'O Axion só acompanha enquanto está aberto (pode ficar minimizado). Não há modo em segundo plano nem ícone perto do relógio.' }),
          h('li', { text: 'A cada ' + v.pollEvery + ' s ele procura, na lista de processos do Windows, o executável dos jogos ligados aqui.' }),
          h('li', { text: 'A partida é gravada em janelas de ' + fmtDur(v.segment) + '. Entre uma janela e outra o PresentMon é reiniciado e os quadros desse intervalo ficam de fora (o relatório mostra quanto do tempo foi gravado).' }),
          h('li', { text: 'Partidas com menos de 1 minuto gravado não geram relatório. Se você iniciar uma medição manual durante a partida, a gravação do relatório pausa até a medição terminar.' }))),
      open ? [perfRunCard(open.run), sessionFacts(open.run),
        open.compare ? perfCompareCard(open.compare, { session: true }) : h('div', { class: 'callout', style: { marginTop: '16px' } }, icon('info'), h('div', { text: 'Primeira partida deste jogo com relatório: a próxima será comparada com esta.' }))] : null,
      h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('clock'), 'Partidas'),
        reps.length ? h('div', { class: 'tbl-scroll', style: { marginTop: '10px' } }, h('table', { class: 'table perf-table' },
          h('thead', null, h('tr', null, ['Data', 'Jogo', 'Gravado', 'FPS médio', '1% low', '0,1% low', 'Engasgos', 'Limitado por', 'GPU máx.'].map(x => h('th', { text: x })))),
          h('tbody', null, reps.map(row))))
          : empty('gamepad', 'Nenhuma partida ainda', 'Ligue o relatório automático de um jogo acima, deixe o Axion aberto e jogue. Quando o jogo fechar, o relatório aparece aqui.')),
      h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('book'), 'Como funciona'),
        h('p', { class: 'small', text: v.honest }),
        v.polls ? h('p', { class: 'small muted', text: 'Custo medido da procura neste PC: ' + fmtN(v.pollAvgMs, 3) + ' ms por consulta, a cada ' + v.pollEvery + ' s (média de ' + v.polls + ' consultas desde que o Axion abriu).' }) : null,
        h('div', { class: 'row wrap', style: { marginTop: '8px' } }, (v.sources || []).map(x => extLink(x.title, x.url)))));
  }


  // ---------------------------------------------------------------------------
  // Atraso de entrada (orçamento de latência de ponta a ponta)
  // ---------------------------------------------------------------------------

  const LATK = { medido: ['medido', 'ok'], calculado: ['calculado', 'info'], 'não medido': ['não medido', 'outline'] };
  const PAGE_LABEL = { perifericos: 'Periféricos', desempenho: 'Desempenho', rede: 'Rede e ping', cs2: 'Counter-Strike 2' };

  function latRow(p) {
    const k = LATK[p.kind] || [p.kind, 'outline'];
    return h('div', { class: 'lat-row' },
      h('div', { class: 'lat-ms' + (p.ms < 0 ? ' unknown' : '') }, h('b', { text: p.ms >= 0 ? fmtN(p.ms, p.ms >= 10 ? 1 : 2) : '?' }), h('small', { text: p.ms >= 0 ? 'ms em média' : '' })),
      h('div', null, h('div', { class: 'row wrap', style: { gap: '6px' } }, h('b', { text: p.label }), badge(k[0], k[1]),
        p.worst > 0 ? h('span', { class: 'small muted', text: 'até ' + fmtN(p.worst, 2) + ' ms' }) : null),
        h('div', { class: 'small muted', text: p.detail }),
        p.page && ROUTES[p.page] ? h('button', { class: 'linkbtn small', type: 'button', text: (p.key === 'keyboard' ? 'Ver em ' : p.kind === 'medido' ? 'Medir de novo em ' : 'Medir em ') + (PAGE_LABEL[p.page] || p.page) + ' →', onclick: () => go(p.page) }) : null));
  }

  async function pageLatencia() {
    const v = await get('latency', '/api/latency');
    const parts = v.mouse || [];
    const known = parts.filter(p => p.ms > 0);
    const total = known.reduce((n, p) => n + p.ms, 0) || 1;
    const COL = { mouse: 'n1', pc: 'n2', scanout: 'n3' };
    return h('div', { class: 'page' },
      pageHead('Atraso de entrada', 'De onde vem cada milissegundo entre a sua mão e a tela, com as medições deste PC — e onde ainda dá para ganhar.'),
      h('div', { class: 'card' },
        h('div', { class: 'row wrap' }, h('h2', { style: { margin: 0 } }, icon('zap'), 'Do movimento do mouse à mira na tela'), h('div', { class: 'spacer' }),
          h('div', { class: 'lat-total' }, h('b', { text: fmtN(v.totalMs, 1) + ' ms' }), h('small', { text: v.complete ? 'em média, sem o painel' : 'parcial: faltam medições' }))),
        known.length ? h('div', { style: { marginTop: '12px' } },
          h('div', { class: 'limbar tall' }, known.map(p => h('i', { class: COL[p.key] || 'w', style: { width: Math.max(p.ms / total * 100, 2) + '%' }, title: p.label + ': ' + fmtN(p.ms, 2) + ' ms' }))),
          h('div', { class: 'row wrap small', style: { gap: '14px', marginTop: '6px' } }, known.map(p => h('span', null, h('b', { class: 'dot ' + (COL[p.key] || 'w') }), ' ' + p.label.split(':')[0].split(' (')[0] + ': ' + fmtN(p.ms, 2) + ' ms')))) : null,
        h('div', { class: 'lat-list', style: { marginTop: '14px' } }, parts.map(latRow))),
      (v.levers || []).length ? h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('rocket'), 'Onde ainda dá para ganhar'),
        h('div', { class: 'stack' }, v.levers.map(l => h('div', { class: 'lever' + (l.done ? ' done' : '') },
          h('div', { class: 'lever-gain' }, l.done ? icon('check') : h('b', { text: l.gainMs >= 0 ? '−' + fmtN(l.gainMs, l.gainMs >= 1 ? 1 : 2) + ' ms' : '?' })),
          h('div', null, h('div', { class: 'row wrap', style: { gap: '6px' } }, h('b', { text: l.title }), l.done ? badge('já está assim', 'ok') : l.gainMs < 0 ? badge('depende do cenário', 'outline') : null),
            h('div', { class: 'small', text: l.text }),
            l.page && ROUTES[l.page] && !l.done ? h('button', { class: 'linkbtn small', type: 'button', text: 'Ir para ' + (PAGE_LABEL[l.page] || l.page) + ' →', onclick: () => go(l.page) }) : null))))) : null,
      (v.missing || []).length ? h('div', { class: 'callout', style: { marginTop: '16px' } }, icon('info'), h('div', null, h('b', { text: 'Para completar o orçamento: ' }),
        h('ul', { class: 'notes', style: { margin: '6px 0 0' } }, v.missing.map(m => h('li', null, m.title + ' — ' + m.text + ' ', ROUTES[m.page] ? h('button', { class: 'linkbtn small', type: 'button', text: 'Ir →', onclick: () => go(m.page) }) : null))))) : null,
      h('div', { class: 'grid g2', style: { marginTop: '16px' } },
        h('div', { class: 'card' }, h('h2', null, icon('keyboard'), 'Teclado'), v.keyboard ? latRow(v.keyboard) : h('p', { class: 'small muted', text: 'Nenhum teclado USB identificado.' })),
        h('div', { class: 'card' }, h('h2', null, icon('usb'), 'Pilha de entrada do Windows'), v.stack ? latRow(v.stack) : h('p', { class: 'small muted', text: 'Medida pelo teste de polling com o rastreamento USB (Periféricos, como administrador).' }))),
      v.network ? h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('wifi'), 'Rede (não entra na soma acima)'), latRow(v.network)) : null,
      h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('book'), 'Como a conta é feita'),
        h('p', { class: 'small', text: v.honest }),
        h('ul', { class: 'notes small' },
          h('li', { text: 'Polling: um movimento espera em média metade do intervalo entre leituras (1000 Hz = 1 ms entre leituras = 0,5 ms em média).' }),
          h('li', { text: 'PC: medido pelo PresentMon da entrada até o quadro ser enviado ao monitor (com os eventos do Reflex quando o jogo os fornece).' }),
          h('li', { text: 'Varredura: o monitor redesenha a tela de cima para baixo a cada ciclo; a mira, no centro, recebe o quadro novo em média na metade do ciclo.' }),
          h('li', { text: 'Painel: o tempo de processamento e de resposta dos pixels varia por modelo e só é medido com sensor de luz.' })),
        h('div', { class: 'row wrap' }, (v.sources || []).map(x => extLink(x.title, x.url)))));
  }

  // ---------------------------------------------------------------------------
  // Rede (latência, bufferbloat, relays da Valve, DNS)
  // ---------------------------------------------------------------------------

  const netForm = { label: 'antes', load: true, dns: true };
  const netSel = { a: null, b: null };
  let netOpen = null;
  let netCmp = null;
  const ms = (v, d) => v == null || v < 0 ? '—' : fmtN(v, d == null ? (v >= 100 ? 0 : v >= 10 ? 1 : 2) : d) + ' ms';
  const lossTxt = v => v == null || v < 0 ? '—' : fmtN(v, v >= 10 || v === 0 ? 0 : 1) + '%';
  const AIMK = { 'ótima': 'ok', boa: 'ok', 'média': 'info', fraca: 'warn', ruim: 'bad' };
  const GRADEK = { 'A+': 'ok', A: 'ok', B: 'info', C: 'warn', D: 'bad', F: 'bad' };
  const ROLE = { lan: 'Seu PC → roteador', internet: 'Até a internet', valve: 'Até a Valve' };

  // Sparkline of one series (ms); lost probes as red ticks at the top.
  function spark(series, top) {
    const W = 220, H = 36;
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('viewBox', '0 0 ' + W + ' ' + H);
    svg.setAttribute('class', 'spark');
    svg.setAttribute('aria-hidden', 'true');
    svg.setAttribute('preserveAspectRatio', 'none');
    if (!series || series.length < 2) return svg;
    const ok = series.filter(v => v >= 0);
    const max = Math.max(top || 0, ...ok, 0.5) * 1.1;
    const X = i => (W - 2) * i / (series.length - 1) + 1;
    const Y = v => H - 2 - (H - 6) * Math.min(v, max) / max;
    let d = '', pen = false, lost = '';
    series.forEach((v, i) => {
      if (v < 0) { pen = false; lost += '<rect x="' + (X(i) - 1).toFixed(1) + '" y="0" width="2" height="5" class="c-lost"/>'; return; }
      d += (pen ? 'L' : 'M') + X(i).toFixed(1) + ' ' + Y(v).toFixed(1) + ' ';
      pen = true;
    });
    svg.innerHTML = '<path d="' + d + '" fill="none" class="c-line" stroke-width="1.4" stroke-linejoin="round"/>' + lost;
    return svg;
  }

  // Where the round trip goes: LAN, provider/internet, route to Valve.
  function latencySplit(r) {
    const lan = (r.targets || []).find(t => t.role === 'lan');
    const net = (r.targets || []).find(t => t.role === 'internet');
    const valve = (r.targets || []).filter(t => t.role === 'valve' && t.idle.recv).sort((a, b) => a.idle.median - b.idle.median)[0];
    if (!lan || !net || !lan.idle.recv || !net.idle.recv) return null;
    const segs = [['Casa (PC → roteador)', lan.idle.median, 'n1'], ['Operadora até a internet', Math.max(net.idle.median - lan.idle.median, 0), 'n2']];
    if (valve) segs.push(['Rota até a Valve (' + valve.name + ')', Math.max(valve.idle.median - net.idle.median, 0), 'n3']);
    const total = segs.reduce((n, s) => n + s[1], 0) || 1;
    return h('div', null,
      h('div', { class: 'limbar tall' }, segs.map(([l, v, c]) => v > 0 ? h('i', { class: c, style: { width: Math.max(v / total * 100, 1.5) + '%' }, title: l + ': ' + ms(v) }) : null)),
      h('div', { class: 'row wrap small', style: { gap: '14px', marginTop: '6px' } }, segs.map(([l, v, c]) => h('span', null, h('b', { class: 'dot ' + c }), ' ' + l + ': ' + ms(v)))),
      h('p', { class: 'small muted', style: { margin: '8px 0 0' }, text: 'Total até ' + (valve ? 'o relay da Valve: ' + ms(valve.idle.median) : 'a internet: ' + ms(net.idle.median)) + ' (ida e volta). No CS2 o placar soma a isso o trecho do relay até o servidor da partida.' }));
  }

  function netFindings(list) {
    return h('div', { class: 'findings' }, (list || []).map(f => findingCard({ severity: f.severity, title: f.title, explanation: f.text, actions: f.actions || [] }, false)));
  }

  function netTargetsCard(r) {
    const rows = (r.targets || []).map(t => {
      const s = t.idle || {};
      // No reply at all: there is no latency to show, only the silence.
      const ok = s.recv > 0;
      return h('tr', null,
        h('td', null, h('div', null, h('b', { text: t.name })), h('div', { class: 'tiny muted', text: (ROLE[t.role] || t.role) + ' · ' + t.addr })),
        h('td', { text: ok ? ms(s.median) : '—' }), h('td', { text: ok ? ms(s.p95) : '—' }), h('td', { text: ok ? ms(s.jitter) : '—' }),
        h('td', null, !ok ? badge('sem resposta', 'outline') : s.lossPct > 0 ? badge(lossTxt(s.lossPct), s.lossPct >= 1 ? 'bad' : 'warn') : h('span', { text: '0%' })),
        h('td', { class: 'spark-cell' }, ok ? spark(t.series) : null));
    });
    return h('div', { class: 'card', style: { marginTop: '16px' } },
      h('h2', null, icon('pulse'), 'Trechos com a conexão parada'),
      h('div', { class: 'tbl-scroll' }, h('table', { class: 'table net-table' },
        h('thead', null, h('tr', null, ['Destino', 'Mediana', 'P95', 'Jitter', 'Perda', 'Ao longo do teste'].map(x => h('th', { text: x })))),
        h('tbody', null, rows))),
      h('p', { class: 'small muted', text: 'P95: 95% dos pings ficaram abaixo disso. Jitter: variação média entre um ping e o seguinte (definição da Cloudflare). Em vermelho no gráfico, pings sem resposta.' }));
  }

  function netBloatCard(r) {
    const b = r.bloat;
    if (!b) return h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('gauge'), 'Bufferbloat e velocidade'),
      h('p', { class: 'small muted', style: { margin: 0 }, text: 'Este teste foi feito sem a parte de carga. Marque "Incluir teste de carga" para medir quanto o ping sobe com a internet em uso.' }));
    const idle = ((r.targets || []).find(t => t.role === 'internet') || {}).idle || {};
    const phase = p => {
      const inet = p.internet || {}, lan = p.lan || {};
      // Median under load with the pings that did not come back in 2 s
      // counted as 2 s (older tests: median of the replies).
      const loaded = p.loadedMs > 0 ? p.loadedMs : inet.median;
      const lost = inet.sent > 0 && inet.lossPct > 0 && !p.noReply ? ' · ' + lossTxt(inet.lossPct) + ' sem resposta em 2 s (contados como 2 s)' : '';
      return h('div', { class: 'bloat-phase' },
        h('div', { class: 'row between' }, h('b', { text: p.dir === 'upload' ? 'Enviando (upload)' : 'Baixando (download)' }), p.error ? badge('falhou', 'bad') : h('span', { class: 'mono', text: fmtN(p.mbps, 0) + ' Mb/s' })),
        p.error ? h('div', { class: 'small', style: { color: 'var(--warn)' }, text: p.error }) : [
          h('div', { class: 'small', text: 'Ping até a internet: ' + ms(idle.median) + ' parado → ' + (p.noReply ? 'nenhum voltou em 2 s com a conexão cheia' : ms(loaded) + ' com a conexão cheia (P95 ' + ms(inet.p95) + ')' + lost) }),
          h('div', { class: 'small muted', text: 'Ao roteador: ' + (lan.recv > 0 ? ms(lan.median) : 'sem resposta') + ' · atraso a mais: ' + (p.noReply ? 'mais de ' + ms(p.addedMs) : '+' + ms(Math.max(p.addedMs || 0, 0))) }),
          p.noReply ? null : spark(p.series, idle.median ? idle.median * 2 : 0)]);
    };
    return h('div', { class: 'card', style: { marginTop: '16px' } },
      h('div', { class: 'row wrap' }, h('h2', { style: { margin: 0 } }, icon('gauge'), 'Bufferbloat e velocidade'), h('div', { class: 'spacer' }),
        b.grade ? h('div', { class: 'grade-tile ' + (GRADEK[b.grade] || 'outline') }, h('span', { text: b.grade }), h('small', { text: 'nota' }))
          : b.partial ? badge('incompleto: sem nota', 'warn') : null),
      b.partial && !b.grade ? h('div', { class: 'callout warn', style: { marginTop: '10px' } }, icon('alert'),
        h('div', { text: 'Uma das direções falhou (motivo abaixo). A nota só vale com as duas medidas: a que funcionou não diz nada sobre a outra. Repita o teste.' })) : null,
      h('div', { class: 'grid g2', style: { marginTop: '10px' } }, (b.phases || []).map(phase)),
      h('p', { class: 'small muted', text: 'Nota pela latência que a conexão cheia acrescenta (escala do teste Waveform, recomendado pelo bufferbloat.net): A+ abaixo de 5 ms, A até 30, B até 60, C até 200, D até 400. ' +
        'Velocidade medida com ' + ((b.phases || [])[0] || {}).streams + ' conexões paralelas ao servidor de teste da Cloudflare, sem contar o início (aceleração do TCP). Dados usados: ' + fmtN(b.dataMB, 0) + ' MB.' }));
  }

  function netRelaysCard(r) {
    const list = r.relays || [];
    if (!list.length) return null;
    const near = new Set((r.targets || []).filter(t => t.role === 'valve').map(t => t.pop));
    return h('div', { class: 'card', style: { marginTop: '16px' } },
      h('h2', null, icon('crosshair'), 'Relays da Valve (Steam Datagram Relay)'),
      h('p', { class: 'small muted', style: { marginTop: 0 }, text: 'No CS2 o seu tráfego entra na rede da Valve pelo relay mais próximo e segue pela rede dela até o servidor. Primeiro o Axion mede todos os locais rapidamente; os 3 mais próximos (em destaque) são medidos em detalhe acima.' }),
      h('div', { class: 'relay-grid' }, list.map(p => h('div', { class: 'relay' + (near.has(p.key) ? ' near' : '') },
        h('span', { text: p.city }), h('span', { class: 'mono', text: p.bestMs >= 0 ? fmtN(p.bestMs, p.bestMs >= 100 ? 0 : 1) + ' ms' : 'sem resposta' })))));
  }

  function netPathCard(r) {
    const hops = (r.path && r.path.hops) || [];
    if (!hops.length) return null;
    const KIND = { privado: ['rede privada', 'outline'], cgnat: ['CGNAT da operadora', 'warn'], 'público': ['internet', 'info'], 'sem resposta': ['sem resposta', 'outline'], local: ['local', 'outline'] };
    return h('details', { class: 'card', style: { marginTop: '16px' } },
      h('summary', { style: { cursor: 'pointer', fontWeight: 600 } }, 'Caminho até a internet (' + plural(hops.length, 'salto', 'saltos') + ')'),
      h('div', { class: 'tbl-scroll', style: { marginTop: '10px' } }, h('table', { class: 'table' },
        h('thead', null, h('tr', null, ['#', 'Endereço', 'Tipo', 'Resposta'].map(x => h('th', { text: x })))),
        h('tbody', null, hops.map(x => h('tr', null, h('td', { text: String(x.ttl) }), h('td', { class: 'mono', text: x.addr || '—' }),
          h('td', null, badge((KIND[x.kind] || [x.kind, 'outline'])[0], (KIND[x.kind] || ['', 'outline'])[1])), h('td', { text: x.addr ? ms(x.rtt) : '—' })))))),
      h('p', { class: 'small muted', text: 'Os roteadores do caminho respondem com o processador deles, que dá prioridade baixa a isso: um salto lento ou sem resposta só indica problema se o destino final também piorar.' }));
  }

  function netDNSCard(r) {
    const list = r.dns || [];
    if (!list.length) return null;
    return h('div', { class: 'card', style: { marginTop: '16px' } },
      h('h2', null, icon('layers'), 'DNS'),
      h('div', { class: 'tbl-scroll' }, h('table', { class: 'table' },
        h('thead', null, h('tr', null, ['Servidor', 'Endereço', 'Mediana', '90%', 'Falhas'].map(x => h('th', { text: x })))),
        h('tbody', null, list.map(d => h('tr', { class: d.system ? 'sel' : null }, h('td', null, d.name, d.system ? h('span', { style: { marginLeft: '6px' } }, badge('em uso', 'accent')) : null),
          h('td', { class: 'mono', text: d.addr }), h('td', { text: ms(d.median) }), h('td', { text: ms(d.p90) }), h('td', { text: d.failures + ' de ' + d.queries })))))),
      h('p', { class: 'small muted', text: 'Cada servidor resolveu os mesmos ' + ((list[0] || {}).queries || 0) + ' nomes (Steam, FACEIT, Discord, Twitch, YouTube…). O DNS só traduz o nome em endereço antes de conectar: não muda o ping durante a partida.' }));
  }

  function netCS2Card(r) {
    const c = r.cs2 || {};
    if (!c.found) return null;
    const buf = c.bufferTicks < 0 ? 0 : c.bufferTicks;
    return h('div', { class: 'card', style: { marginTop: '16px' } },
      h('h2', null, icon('crosshair'), 'Configurações de rede do CS2'),
      h('div', { class: 'set-list' },
        h('div', { class: 'set-row' }, h('span', { text: 'Buffering to smooth over packet loss' }), h('b', { text: buf === 0 ? 'None (nenhum)' : buf + (buf === 1 ? ' packet' : ' packets') + ' · +' + fmtN(buf * 15.625, 1) + ' ms' })),
        h('div', { class: 'set-row' }, h('span', { text: 'Max Acceptable Game Traffic Bandwidth (rate)' }), h('b', { text: c.rate < 0 ? 'padrão (sem restrição)' : c.rate >= 786432 ? 'sem restrição (' + c.rate + ')' : String(c.rate) }))),
      h('ul', { class: 'notes small', style: { marginTop: '10px' } },
        h('li', { text: 'Ligue a telemetria do jogo (Configurações > Jogo > Telemetria): ela mostra ping, perda e oscilação durante a partida e confirma em jogo o que este teste mediu.' }),
        h('li', { text: 'No console, cl_ticktiming print mostra de onde vem o atraso dentro do jogo (comando criado pela Valve em fevereiro de 2024).' }),
        c.file ? h('li', { class: 'muted', text: 'Lido de ' + c.file }) : null));
  }

  function netRunView(r) {
    const aim = r.aim;
    const b = r.bloat;
    const valve = (r.targets || []).filter(t => t.role === 'valve' && t.idle.recv).sort((a, z) => a.idle.median - z.idle.median)[0];
    const net = (r.targets || []).find(t => t.role === 'internet');
    const tiles = [
      ['Para jogos (AIM)', aim ? aim.class : '—', aim ? AIMK[aim.class] : 'outline'],
      ['Bufferbloat', b && b.grade ? b.grade + ((b.phases || []).some(p => p.noReply) ? ' (ping acima de 2 s)' : ' (+' + fmtN(Math.max(b.addedMs, 0), 0) + ' ms)') : b && b.partial ? 'incompleto' : '—',
        b && b.grade ? GRADEK[b.grade] : b && b.partial ? 'warn' : 'outline'],
      ['Valve mais próxima', valve ? valve.name + ' · ' + ms(valve.idle.median) : '—', valve ? (valve.idle.lossPct >= 1 ? 'bad' : 'ok') : 'outline'],
      ['Internet', net && net.idle.recv ? ms(net.idle.median) + ' · jitter ' + ms(net.idle.jitter) : '—', net && net.idle.lossPct >= 1 ? 'bad' : 'ok'],
    ];
    const split = latencySplit(r);
    return h('div', null,
      h('div', { class: 'card', id: 'net-run', style: { marginTop: '16px' } },
        h('div', { class: 'row wrap' }, h('h2', { style: { margin: 0 } }, icon('wifi'), (r.label ? r.label + ' · ' : '') + fmtDT(r.at)), h('div', { class: 'spacer' }),
          h('span', { class: 'small muted', text: (r.adapter.kind || '') + ' · ' + (r.adapter.description || '') + (r.adapter.linkMbps ? ' · link ' + Number(r.adapter.linkMbps).toLocaleString('pt-BR') + ' Mb/s' : '') + (r.adapter.via ? ' · internet via ' + r.adapter.via : '') })),
        h('div', { class: 'net-tiles' }, tiles.map(([k, v, c]) => h('div', { class: 'net-tile ' + (c || '') }, h('div', { class: 'k', text: k }), h('div', { class: 'v', text: v })))),
        split ? h('div', { style: { marginTop: '14px' } }, h('h3', { text: 'Onde está o atraso' }), split) : null,
        (r.errors || []).length ? h('div', { class: 'callout warn', style: { marginTop: '12px' } }, icon('alert'), h('ul', { class: 'notes', style: { margin: 0 } }, r.errors.map(e => h('li', { text: e })))) : null),
      h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('check'), 'O que o teste encontrou'), netFindings(r.findings),
        h('p', { class: 'small muted', text: aim ? 'Classificação para jogos da Cloudflare (AIM): soma pontos pela latência parada, pela perda e pelo atraso a mais sob carga. Resultado: ' + aim.points + ' pontos (' + aim.class + ').'
          : 'A classificação para jogos da Cloudflare (AIM) depende do atraso a mais com a internet em uso: ela só aparece quando o teste de carga roda até o fim.' })),
      netTargetsCard(r), netBloatCard(r), netRelaysCard(r), netCS2Card(r), netDNSCard(r), netPathCard(r));
  }

  function netCompareCard(c) {
    const VERD = { melhor: ['melhor', 'ok'], pior: ['pior', 'bad'], igual: ['sem diferença', 'outline'], 'sem dados': ['sem dados', 'outline'] };
    const val = (v, u) => v < 0 ? '—' : fmtN(v, u === 'Mb/s' ? 0 : u === '%' ? 1 : 2) + ' ' + u;
    return h('div', { class: 'card', id: 'net-cmp', style: { marginTop: '16px' } },
      h('h2', null, icon('gauge'), 'Comparação: ' + (c.a.label || 'A') + ' × ' + (c.b.label || 'B')),
      h('div', { class: 'tbl-scroll' }, h('table', { class: 'table' },
        h('thead', null, h('tr', null, ['Métrica', (c.a.label || 'A') + ' (' + fmtDT(c.a.at) + ')', (c.b.label || 'B') + ' (' + fmtDT(c.b.at) + ')', 'Diferença', 'Resultado'].map(x => h('th', { text: x })))),
        h('tbody', null, c.rows.map(r => h('tr', null, h('td', { text: r.label }), h('td', { text: val(r.a, r.unit) }), h('td', { text: val(r.b, r.unit) }),
          h('td', { text: r.verdict === 'sem dados' ? '—' : fmtSigned(r.delta, r.unit === 'Mb/s' ? 0 : r.unit === '%' ? 1 : 2) + ' ' + r.unit }),
          h('td', null, badge((VERD[r.verdict] || [r.verdict])[0], (VERD[r.verdict] || ['', 'outline'])[1]))))))),
      h('p', { class: 'small muted', text: c.note }));
  }

  async function pageRede() {
    const v = await get('net', '/api/net');
    const runs = v.runs || [];
    const byId = new Map(runs.map(r => [r.id, r]));
    if (netSel.a && !byId.has(netSel.a)) { netSel.a = null; netCmp = null; }
    if (netSel.b && !byId.has(netSel.b)) { netSel.b = null; netCmp = null; }
    if (netOpen && !byId.has(netOpen)) netOpen = null;
    let run = v.last;
    if (netOpen && (!run || run.id !== netOpen)) { try { run = await get('netrun:' + netOpen, '/api/net/run/' + netOpen); } catch (e) { netOpen = null; } }
    const ad = v.adapter;
    const lab = h('input', { class: 'select', value: netForm.label, list: 'net-labels', placeholder: 'rótulo (ex.: antes)', 'aria-label': 'Rótulo do teste', 'data-focus': 'net-label', maxlength: '40', style: { width: '170px' } });
    lab.addEventListener('input', () => { netForm.label = lab.value; });
    // The load server asked to wait (HTTP 429 with Retry-After): the load
    // phase is off until then; the rest of the test still runs.
    const waitUntil = v.retryAt && new Date(v.retryAt) > new Date() ? new Date(v.retryAt) : null;
    const loadCb = h('input', { type: 'checkbox', checked: netForm.load && !waitUntil, disabled: waitUntil ? 'disabled' : null });
    loadCb.addEventListener('change', () => { netForm.load = loadCb.checked; rerender(); });
    const withLoad = netForm.load && !waitUntil;
    const dnsCb = h('input', { type: 'checkbox', checked: netForm.dns });
    dnsCb.addEventListener('change', () => { netForm.dns = dnsCb.checked; });
    const lastData = runs.length && v.last && v.last.bloat ? v.last.bloat.dataMB : 0;
    const secs = 6 + (v.duration.idle || 10) + (withLoad ? 2 * (v.duration.load || 8) : 0) + (netForm.dns ? 2 : 0);
    const start = async () => {
      const job = await runJob(api('POST', '/api/net/test', { label: netForm.label, load: withLoad, dns: netForm.dns }), { title: 'Testando a rede' });
      if (job && job.status === 'done' && job.result) { netOpen = null; netCmp = null; invalidate(); rerender(); setTimeout(() => { const el = document.getElementById('net-run'); if (el) el.scrollIntoView({ behavior: 'smooth' }); }, 80); }
    };
    const pick = (slot, id) => { netSel[slot] = netSel[slot] === id ? null : id; netCmp = null; rerender(); };
    const compare = async () => {
      try {
        netCmp = await api('POST', '/api/net/compare', { a: netSel.a, b: netSel.b });
        rerender();
        setTimeout(() => { const el = document.getElementById('net-cmp'); if (el) el.scrollIntoView({ behavior: 'smooth' }); }, 60);
      } catch (e) { toast(e.message, 'bad'); }
    };
    const row = r => h('tr', { class: run && run.id === r.id ? 'sel' : null },
      h('td', null, h('input', { type: 'radio', name: 'net-a', checked: netSel.a === r.id, 'aria-label': 'Usar como A (antes)', onclick: () => pick('a', r.id) })),
      h('td', null, h('input', { type: 'radio', name: 'net-b', checked: netSel.b === r.id, 'aria-label': 'Usar como B (depois)', onclick: () => pick('b', r.id) })),
      h('td', null, h('div', { text: fmtDT(r.at) }), h('div', { class: 'tiny' },
        h('button', { class: 'linkbtn tiny', type: 'button', text: run && run.id === r.id ? 'aberto' : 'ver', onclick: () => { netOpen = r.id; rerender(); setTimeout(() => { const el = document.getElementById('net-run'); if (el) el.scrollIntoView({ behavior: 'smooth' }); }, 80); } }), ' · ',
        h('button', { class: 'linkbtn tiny', type: 'button', text: 'rótulo', onclick: async () => {
          const inp = h('input', { class: 'select', value: r.label || '', style: { width: '100%' }, 'aria-label': 'Novo rótulo', maxlength: '40' });
          if (!(await confirmDlg({ title: 'Rótulo do teste', ok: 'Salvar', icon: 'check', body: h('div', null, inp), input: inp }))) return;
          try { await api('POST', '/api/net/label', { id: r.id, label: inp.value }); netCmp = null; invalidate(); rerender(); } catch (e) { toast(e.message, 'bad'); }
        } }), ' · ',
        h('button', { class: 'linkbtn tiny', type: 'button', text: 'apagar', onclick: async () => {
          if (!(await confirmDlg({ title: 'Apagar teste', ok: 'Apagar', danger: true, icon: 'trash', body: h('p', { text: 'O teste de ' + fmtDT(r.at) + ' será apagado.' }) }))) return;
          try { await api('POST', '/api/net/delete', { id: r.id }); if (netOpen === r.id) netOpen = null; netCmp = null; invalidate(); rerender(); } catch (e) { toast(e.message, 'bad'); }
        } }))),
      h('td', { text: r.label || '—' }), h('td', { text: r.lanMs >= 0 ? ms(r.lanMs) : '—' }),
      h('td', { text: r.netMs >= 0 ? ms(r.netMs) + ' · ' + lossTxt(r.netLoss) : '—' }),
      h('td', { text: r.valveMs >= 0 ? r.valvePop + ' ' + ms(r.valveMs) + ' · ' + lossTxt(r.valveLoss) : '—' }),
      h('td', null, r.grade ? badge(r.grade + ' (+' + fmtN(Math.max(r.addedMs, 0), 0) + ' ms)', GRADEK[r.grade])
        : r.loadError ? h('span', { title: r.loadError }, badge('incompleto', 'warn')) : '—'),
      h('td', { text: r.downMbps >= 0 || r.upMbps >= 0 ? (r.downMbps >= 0 ? fmtN(r.downMbps, 0) : '—') + ' / ' + (r.upMbps >= 0 ? fmtN(r.upMbps, 0) : '—') : '—' }),
      h('td', null, r.problems ? badge(plural(r.problems, 'problema', 'problemas'), 'warn') : badge('ok', 'ok')));
    return h('div', { class: 'page' },
      pageHead('Rede e ping', 'Mede cada trecho até os relays da Valve que o CS2 usa, o bufferbloat e o DNS, e diz o que fazer em cada caso. Nada é alterado no PC ou no roteador.'),
      h('div', { class: 'card' },
        h('h2', null, icon('play'), 'Novo teste'),
        ad ? h('div', { class: 'small', style: { marginBottom: '10px' } }, badge(ad.kind, ad.kind === 'Wi-Fi' ? 'warn' : 'outline'), ' ' + ad.description + (ad.linkMbps ? ' · link ' + Number(ad.linkMbps).toLocaleString('pt-BR') + ' Mb/s' : '') + ' · roteador ' + ad.gateway)
          : h('div', { class: 'callout warn' }, icon('alert'), h('div', { text: 'Nenhuma conexão com roteador encontrada.' })),
        ad && ad.kind === 'Wi-Fi' ? h('div', { class: 'callout', style: { marginBottom: '10px' } }, icon('info'), h('div', { text: 'Você está no Wi-Fi: para jogar, o cabo tira a maior fonte de oscilação. O teste mostra quanto isso pesa no seu caso.' })) : null,
        h('div', { class: 'fields' },
          h('label', { class: 'fld' }, h('span', { text: 'Rótulo' }), lab),
          h('datalist', { id: 'net-labels' }, (v.labels || []).map(l => h('option', { value: l })))),
        h('div', { class: 'row wrap', style: { marginTop: '10px', gap: '18px' } },
          h('label', { class: 'check small' }, loadCb, 'Incluir teste de carga (bufferbloat e velocidade)'),
          h('label', { class: 'check small' }, dnsCb, 'Testar DNS')),
        waitUntil ? h('div', { class: 'callout warn', style: { marginTop: '10px' } }, icon('clock'), h('div', { text: 'O servidor do teste de carga (Cloudflare) limitou testes seguidos e pediu para esperar até ' + waitUntil.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' }) + '. Até lá, o teste roda sem a etapa de carga.' })) : null,
        h('div', { class: 'row wrap', style: { marginTop: '12px' } },
          btn('Testar a rede', start, 'primary', 'play', { disabled: ad ? null : 'disabled' }),
          h('span', { class: 'small muted', text: 'Leva cerca de ' + secs + ' s. Pause downloads e streaming durante o teste.' + (withLoad ? ' O teste de carga transfere ' + (lastData ? 'cerca de ' + fmtN(lastData, 0) + ' MB (medido no último teste)' : 'algumas centenas de MB (até ~1 GB em conexões de 1 Gb/s)') + '.' : '') }))),
      run ? netRunView(run) : h('div', { class: 'card', style: { marginTop: '16px' } }, empty('wifi', 'Nenhum teste ainda', 'Faça o primeiro acima. Para comparar, rotule como "antes", mude uma coisa (QoS do roteador, cabo, buffer do CS2) e teste de novo como "depois".')),
      runs.length ? h('div', { class: 'card', style: { marginTop: '16px' } },
        h('div', { class: 'row wrap' }, h('h2', { style: { margin: 0 } }, icon('clock'), 'Testes'), h('div', { class: 'spacer' }),
          btn('Comparar A × B', compare, 'primary', 'gauge', { disabled: netSel.a && netSel.b && netSel.a !== netSel.b ? null : 'disabled' })),
        h('div', { class: 'tbl-scroll', style: { marginTop: '10px' } }, h('table', { class: 'table net-hist' },
          h('thead', null, h('tr', null, ['A', 'B', 'Data', 'Rótulo', 'Roteador', 'Internet', 'Valve', 'Bufferbloat', 'Down / Up (Mb/s)', ''].map(x => h('th', { text: x })))),
          h('tbody', null, runs.map(row))))) : null,
      netCmp ? netCompareCard(netCmp) : null,
      h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('ban'), 'O que não muda o seu ping'),
        h('ul', { class: 'notes small' },
          h('li', { text: 'Trocar de DNS: ele só traduz nomes antes de conectar; a partida usa o endereço direto.' }),
          h('li', { text: 'TCPNoDelay/Nagle, NetworkThrottlingIndex e "liberar os 20% do QoS": o CS2 usa UDP e o Windows não reserva banda.' }),
          h('li', { text: 'Abrir portas, UPnP ou "NAT aberto" para o CS2: a Valve conecta todos pelos relays dela.' }),
          h('li', { text: 'Mudar o cl_interp: a Valve travou o comando em fevereiro de 2024; o ajuste agora é o buffer contra perda de pacotes.' }),
          h('li', { text: 'Programas de "redução de ping" (GPN): só ajudam quando a rota da operadora até a Valve é ruim — o teste acima mostra se é o seu caso.' })),
        h('div', { class: 'row wrap' }, btn('Ver todos em "O que não fazemos"', () => go('excluidos'), 'sm', 'chevron'))),
      h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('book'), 'Como o teste funciona'),
        h('p', { class: 'small', text: v.honest }), h('p', { class: 'small muted', text: v.privacy }),
        h('div', { class: 'row wrap' }, (v.sources || []).map(x => extLink(x.title, x.url)))));
  }

  // ---------------------------------------------------------------------------
  // Latência do sistema: DPCs e interrupções dos drivers
  // ---------------------------------------------------------------------------

  const dpcForm = { label: 'antes', seconds: 30 };
  const dpcSel = { a: null, b: null };
  let dpcOpen = null;
  let dpcCmp = null;
  const DPC_VERDICT = { ok: ['sem atrasos relevantes', 'ok'], atencao: ['atenção', 'warn'], importante: ['importante', 'bad'], inconclusivo: ['inconclusivo', 'outline'] };
  const usTxt = v => v == null || v <= 0 ? '—' : v >= 1000 ? fmtN(v / 1000, v >= 10000 ? 0 : 1) + ' ms' : v >= 10 ? fmtN(v, 0) + ' µs' : fmtN(v, 1) + ' µs';
  const pctTxt = v => v == null ? '—' : fmtN(v, v >= 10 ? 0 : v >= 1 ? 1 : 2) + '%';
  const edgeTxt = v => v >= 1000 ? fmtN(v / 1000, 0) + ' ms' : fmtN(v, 0) + ' µs';

  // The distribution of how long the routines lasted (Microsoft's own report
  // shows the same table); buckets entirely above the limit are marked.
  function dpcHist(title, stats, kind, bounds, limit) {
    const hist = new Array(bounds.length + 1).fill(0);
    (stats || []).filter(s => s.kind === kind).forEach(s => (s.hist || []).forEach((n, i) => { if (i < hist.length) hist[i] += n; }));
    const total = hist.reduce((a, b) => a + b, 0);
    if (!total) return null;
    const max = Math.max(...hist);
    const label = i => i === 0 ? 'até ' + edgeTxt(bounds[0]) : i < bounds.length ? 'de ' + edgeTxt(bounds[i - 1]) + ' a ' + edgeTxt(bounds[i]) : 'mais de ' + edgeTxt(bounds[bounds.length - 1]);
    return h('div', { class: 'card' }, h('h2', null, icon('gauge'), title),
      h('div', { class: 'bars dpc-hist' }, hist.map((n, i) => {
        const over = i > 0 && bounds[i - 1] >= limit;
        return h('div', { class: 'bar-row' }, h('span', { class: 'ellipsis', text: label(i) }),
          h('div', { class: 'bar ' + (over ? (n ? 'warn' : 'ok') : 'ok') }, h('i', { style: { width: (max ? Math.max(n ? 2 : 0, n / max * 100) : 0) + '%' } })),
          h('span', { class: 'n', text: n ? fmtN(n, 0) + ' · ' + pctTxt(n / total * 100) : '—' }));
      })),
      h('div', { class: 'small muted', style: { marginTop: '10px' }, text: 'Em laranja, as faixas acima do limite de ' + edgeTxt(limit) + ' que a Microsoft recomenda para ' + (kind === 'dpc' ? 'um DPC' : 'uma ISR') + '.' }));
  }

  function dpcRunView(r, hints, bounds) {
    const s = r.summary;
    const verd = DPC_VERDICT[s.verdict] || DPC_VERDICT.inconclusivo;
    const tile = (k, v, sub, kind) => h('div', { class: 'net-tile ' + (kind || '') }, h('div', { class: 'k', text: k }), h('div', { class: 'v', text: v }), sub ? h('div', { class: 'tiny faint', text: sub }) : null);
    const sev = us => us >= 4000 ? 'bad' : us >= 1000 ? 'warn' : '';
    const drivers = (s.drivers || []).map(d => {
      const hint = (hints || {})[d.module] || {};
      const dv = DPC_VERDICT[d.verdict] || DPC_VERDICT.ok;
      const name = d.module || '(driver não identificado)';
      return h('tr', { class: d.verdict !== 'ok' ? 'sel' : null },
        h('td', null, h('div', { class: 'ellipsis', style: { fontWeight: '600' }, text: name }), hint.area ? h('div', { class: 'tiny faint', text: hint.area }) : null),
        h('td', { text: pctTxt(d.sharePct) }),
        h('td', null, d.dpc.count ? [h('div', { text: fmtN(d.dpc.count, 0) + ' · maior ' + usTxt(d.dpc.maxUs) }), h('div', { class: 'tiny faint', text: '99% até ' + usTxt(d.dpc.p99Us) + (d.dpc.over ? ' · ' + fmtN(d.dpc.over, 0) + ' acima de 100 µs' : '') })] : '—'),
        h('td', null, d.isr.count ? [h('div', { text: fmtN(d.isr.count, 0) + ' · maior ' + usTxt(d.isr.maxUs) }), h('div', { class: 'tiny faint', text: d.isr.over ? fmtN(d.isr.over, 0) + ' acima de 25 µs' : 'nenhuma acima de 25 µs' })] : '—'),
        h('td', null, badge(dv[0], dv[1]), d.why ? h('div', { class: 'tiny', style: { marginTop: '4px' }, text: d.why }) : null,
          d.verdict !== 'ok' && hint.text ? h('div', { class: 'tiny muted', style: { marginTop: '4px' }, text: hint.text }) : null,
          d.verdict !== 'ok' && hint.page && ROUTES[hint.page] ? h('div', { style: { marginTop: '6px' } }, btn(hint.pageLabel || 'Abrir', () => go(hint.page), 'sm', 'chevron')) : null));
    });
    return h('div', { id: 'dpc-run' },
      h('div', { class: 'card', style: { marginTop: '16px' } },
        h('div', { class: 'row wrap' }, h('h2', { style: { margin: 0 } }, icon('pulse'), 'Resultado'), badge(verd[0], verd[1]), h('div', { class: 'spacer' }),
          h('span', { class: 'small muted', text: (r.label ? r.label + ' · ' : '') + fmtDT(r.at) + ' · ' + fmtN(s.seconds, 0) + ' s · ' + s.cpus + ' processadores lógicos' })),
        h('p', { style: { margin: '10px 0 4px', fontWeight: '600' }, text: s.headline }),
        (s.notes || []).map(n => h('div', { class: 'callout' + (s.reliable ? '' : ' warn'), style: { marginTop: '8px' } }, icon(s.reliable ? 'info' : 'alert'), h('div', { text: n }))),
        s.reliable ? h('div', { class: 'net-tiles' },
          tile('Processador em DPCs e interrupções', pctTxt(s.totalPct), 'do tempo de todos os processadores', s.totalPct >= 15 ? 'bad' : s.totalPct >= 5 ? 'warn' : ''),
          tile('Maior DPC', usTxt(s.dpc.maxUs), s.dpc.over ? plural(s.dpc.over, 'DPC acima de 100 µs', 'DPCs acima de 100 µs') : 'nenhum acima de 100 µs', sev(s.dpc.maxUs)),
          tile('Maior interrupção (ISR)', usTxt(s.isr.maxUs), s.isr.over ? plural(s.isr.over, 'ISR acima de 25 µs', 'ISRs acima de 25 µs') : 'nenhuma acima de 25 µs', sev(s.isr.maxUs)),
          tile('Eventos lidos', fmtN(s.events, 0), s.lostEvents ? 'o Windows perdeu ' + fmtN(s.lostEvents, 0) : 'nenhum perdido')) : null),
      s.reliable && drivers.length ? h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('chip'), 'Quem gasta mais tempo'),
        h('div', { class: 'small muted', style: { marginBottom: '8px' }, text: 'Cada linha é um driver do kernel, do que mais ocupa o processador ao que menos ocupa. "Maior" é a rotina mais demorada da medição.' }),
        h('div', { class: 'tbl-scroll' }, h('table', { class: 'table' },
          h('thead', null, h('tr', null, ['Driver', 'Processador', 'DPCs', 'Interrupções (ISR)', 'Situação'].map(x => h('th', { text: x })))),
          h('tbody', null, drivers)))) : null,
      s.reliable ? h('div', { class: 'grid g2', style: { marginTop: '16px' } },
        dpcHist('Duração dos DPCs', r.capture.stats, 'dpc', bounds, 100) || h('div'),
        dpcHist('Duração das interrupções (ISR)', r.capture.stats, 'isr', bounds, 25) || h('div')) : null);
  }

  function dpcCompareCard(c) {
    const VERD = { melhor: ['melhor', 'ok'], pior: ['pior', 'bad'], igual: ['sem diferença', 'outline'], 'sem dados': ['sem dados', 'outline'] };
    const val = (v, u) => u === '%' ? pctTxt(v) : u === 'µs' ? usTxt(v) : fmtN(v, 1) + ' ' + u;
    return h('div', { class: 'card', id: 'dpc-cmp', style: { marginTop: '16px' } },
      h('h2', null, icon('gauge'), 'Comparação: ' + (c.a.label || 'A') + ' × ' + (c.b.label || 'B')),
      h('div', { class: 'tbl-scroll' }, h('table', { class: 'table' },
        h('thead', null, h('tr', null, ['Métrica', (c.a.label || 'A') + ' (' + fmtDT(c.a.at) + ')', (c.b.label || 'B') + ' (' + fmtDT(c.b.at) + ')', 'Resultado'].map(x => h('th', { text: x })))),
        h('tbody', null, c.rows.map(r => h('tr', null, h('td', { text: r.label }), h('td', { text: val(r.a, r.unit) }), h('td', { text: val(r.b, r.unit) }),
          h('td', null, badge((VERD[r.verdict] || [r.verdict])[0], (VERD[r.verdict] || ['', 'outline'])[1]))))))),
      h('p', { class: 'small muted', text: c.note }));
  }

  async function pageDpc() {
    const v = await get('dpc', '/api/dpc');
    const runs = v.runs || [];
    const byId = new Map(runs.map(r => [r.id, r]));
    if (dpcSel.a && !byId.has(dpcSel.a)) { dpcSel.a = null; dpcCmp = null; }
    if (dpcSel.b && !byId.has(dpcSel.b)) { dpcSel.b = null; dpcCmp = null; }
    if (dpcOpen && !byId.has(dpcOpen)) dpcOpen = null;
    let run = v.last, hints = v.hints;
    if (dpcOpen && (!run || run.id !== dpcOpen)) {
      try { const o = await get('dpcrun:' + dpcOpen, '/api/dpc/run/' + dpcOpen); run = o; hints = o.hints; } catch (e) { dpcOpen = null; }
    }
    const lab = h('input', { class: 'select', value: dpcForm.label, list: 'dpc-labels', placeholder: 'rótulo (ex.: antes)', 'aria-label': 'Rótulo da medição', 'data-focus': 'dpc-label', maxlength: '40', style: { width: '170px' } });
    lab.addEventListener('input', () => { dpcForm.label = lab.value; });
    const dur = h('select', { 'aria-label': 'Duração da medição', onchange: e => { dpcForm.seconds = Number(e.target.value); } },
      [10, 30, 60, 120].map(n => h('option', { value: String(n), text: n + ' segundos', selected: dpcForm.seconds === n ? 'selected' : null })));
    const start = async () => {
      const job = await runJob(api('POST', '/api/dpc/test', { label: dpcForm.label, seconds: dpcForm.seconds }), { title: 'Medindo DPCs e interrupções' });
      if (job && job.status === 'done' && job.result) { dpcOpen = null; dpcCmp = null; invalidate(); rerender(); setTimeout(() => { const el = document.getElementById('dpc-run'); if (el) el.scrollIntoView({ behavior: 'smooth' }); }, 80); }
    };
    const pick = (slot, id) => { dpcSel[slot] = dpcSel[slot] === id ? null : id; dpcCmp = null; rerender(); };
    const compare = async () => {
      try {
        dpcCmp = await api('POST', '/api/dpc/compare', { a: dpcSel.a, b: dpcSel.b });
        rerender();
        setTimeout(() => { const el = document.getElementById('dpc-cmp'); if (el) el.scrollIntoView({ behavior: 'smooth' }); }, 60);
      } catch (e) { toast(e.message, 'bad'); }
    };
    const row = r => {
      const vd = DPC_VERDICT[r.verdict] || DPC_VERDICT.inconclusivo;
      return h('tr', { class: run && run.id === r.id ? 'sel' : null },
        h('td', null, h('input', { type: 'radio', name: 'dpc-a', checked: dpcSel.a === r.id, 'aria-label': 'Usar como A (antes)', onclick: () => pick('a', r.id) })),
        h('td', null, h('input', { type: 'radio', name: 'dpc-b', checked: dpcSel.b === r.id, 'aria-label': 'Usar como B (depois)', onclick: () => pick('b', r.id) })),
        h('td', null, h('div', { text: fmtDT(r.at) }), h('div', { class: 'tiny' },
          h('button', { class: 'linkbtn tiny', type: 'button', text: run && run.id === r.id ? 'aberto' : 'ver', onclick: () => { dpcOpen = r.id; rerender(); setTimeout(() => { const el = document.getElementById('dpc-run'); if (el) el.scrollIntoView({ behavior: 'smooth' }); }, 80); } }), ' · ',
          h('button', { class: 'linkbtn tiny', type: 'button', text: 'rótulo', onclick: async () => {
            const inp = h('input', { class: 'select', value: r.label || '', style: { width: '100%' }, 'aria-label': 'Novo rótulo', maxlength: '40' });
            if (!(await confirmDlg({ title: 'Rótulo da medição', ok: 'Salvar', icon: 'check', body: h('div', null, inp), input: inp }))) return;
            try { await api('POST', '/api/dpc/label', { id: r.id, label: inp.value }); dpcCmp = null; invalidate(); rerender(); } catch (e) { toast(e.message, 'bad'); }
          } }), ' · ',
          h('button', { class: 'linkbtn tiny', type: 'button', text: 'apagar', onclick: async () => {
            if (!(await confirmDlg({ title: 'Apagar medição', ok: 'Apagar', danger: true, icon: 'trash', body: h('p', { text: 'A medição de ' + fmtDT(r.at) + ' será apagada.' }) }))) return;
            try { await api('POST', '/api/dpc/delete', { id: r.id }); if (dpcOpen === r.id) dpcOpen = null; dpcCmp = null; invalidate(); rerender(); } catch (e) { toast(e.message, 'bad'); }
          } }))),
        h('td', { text: r.label || '—' }),
        h('td', null, badge(vd[0], vd[1])),
        h('td', { text: r.reliable ? pctTxt(r.totalPct) : '—' }),
        h('td', { text: r.reliable ? usTxt(r.worstDpcUs) + (r.worstDriver ? ' · ' + r.worstDriver : '') : '—' }),
        h('td', { text: r.reliable ? fmtN(r.overPerSec, 1) + ' /s' : '—' }),
        h('td', { text: fmtN(r.seconds, 0) + ' s' }));
    };
    return h('div', { class: 'page' },
      pageHead('Latência do sistema', 'Mostra qual driver do Windows atrasa o processador (DPCs e interrupções), a causa de engasgos que o FPS médio não revela. A medição só lê: nada é alterado.'),
      h('div', { class: 'card' },
        h('h2', null, icon('play'), 'Nova medição'),
        !v.elevated ? h('div', { class: 'callout warn', style: { marginBottom: '10px' } }, icon('alert'), h('div', { text: 'A medição liga o registro de eventos do kernel e precisa do Axion como administrador. Abra o programa de novo e aceite o pedido de permissão.' })) : null,
        h('div', { class: 'fields' },
          h('label', { class: 'fld' }, h('span', { text: 'Duração' }), dur),
          h('label', { class: 'fld' }, h('span', { text: 'Rótulo' }), lab),
          h('datalist', { id: 'dpc-labels' }, (v.labels || []).map(l => h('option', { value: l })))),
        h('div', { class: 'row wrap', style: { marginTop: '12px' } },
          btn('Medir agora', start, 'primary', 'play', { disabled: v.elevated ? null : 'disabled' }),
          h('span', { class: 'small muted', text: 'Para ver o que acontece no jogo, abra o jogo e comece a medir; durante a medição, jogue como de costume. Em repouso, deixe o PC parado.' }))),
      run ? dpcRunView(run, hints, v.bounds || []) : h('div', { class: 'card', style: { marginTop: '16px' } }, empty('pulse', 'Nenhuma medição ainda', 'Faça a primeira acima. Para comparar, rotule como "antes", mude uma coisa (driver, porta USB, opção de energia) e meça de novo como "depois".')),
      runs.length ? h('div', { class: 'card', style: { marginTop: '16px' } },
        h('div', { class: 'row wrap' }, h('h2', { style: { margin: 0 } }, icon('clock'), 'Medições'), h('div', { class: 'spacer' }),
          btn('Comparar A × B', compare, 'primary', 'gauge', { disabled: dpcSel.a && dpcSel.b && dpcSel.a !== dpcSel.b ? null : 'disabled' })),
        h('div', { class: 'tbl-scroll', style: { marginTop: '10px' } }, h('table', { class: 'table' },
          h('thead', null, h('tr', null, ['A', 'B', 'Data', 'Rótulo', 'Situação', 'Processador', 'Maior DPC', 'DPCs > 100 µs', 'Duração'].map(x => h('th', { text: x })))),
          h('tbody', null, runs.map(row))))) : null,
      dpcCmp ? dpcCompareCard(dpcCmp) : null,
      h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('book'), 'Como a medição funciona'),
        h('p', { class: 'small', text: v.honest }), h('p', { class: 'small', text: v.limits }), h('p', { class: 'small muted', text: v.privacy }),
        h('div', { class: 'row wrap' }, (v.sources || []).map(x => extLink(x.title, x.url)))));
  }

  // ---------------------------------------------------------------------------
  // Live (OBS): the numbers OBS prints in its log when a stream or recording ends
  // ---------------------------------------------------------------------------

  const OBS_LEVEL = { ok: ['sem perdas relevantes', 'ok'], atencao: ['atenção', 'warn'], importante: ['importante', 'bad'] };
  const OBS_KIND = { stream: 'Transmissão', recording: 'Gravação', replay: 'Replay', other: 'Outra saída' };
  const obsPct = v => v == null || v < 0 ? '—' : fmtN(v, 1) + '%';
  const obsDur = s => !(s > 0) ? '—' : s >= 3600 ? Math.floor(s / 3600) + ' h ' + String(Math.round((s % 3600) / 60)).padStart(2, '0') + ' min' : s >= 60 ? Math.round(s / 60) + ' min' : Math.round(s) + ' s';
  const obsStat = row => (row.stats || []).find(x => x.kind === 'stream') || (row.stats || []).find(x => x.kind === 'recording') || (row.stats || [])[0] || {};
  let obsOpen = null;

  function obsRowKey(r) { return r.log + '|' + r.at; }

  async function pageObs() {
    const v = await get('obs', '/api/obs');
    const rows = v.rows || [];
    let row = rows[v.latest >= 0 ? v.latest : 0];
    if (obsOpen) { const o = rows.find(r => obsRowKey(r) === obsOpen); if (o) row = o; else obsOpen = null; }
    const reread = btn('Reler os logs', () => { invalidate(); rerender(); }, 'ghost', 'refresh');
    const head = pageHead('Live (OBS)', 'Lê os logs do OBS Studio e diz, com os números que o próprio OBS registra, por que uma transmissão ou gravação engasgou. A leitura só lê: nada é alterado.', [reread]);
    const info = h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('book'), 'Como a análise funciona'),
      h('p', { class: 'small', text: v.honest }), h('p', { class: 'small', text: v.criteria }), h('p', { class: 'small muted', text: v.privacy }),
      h('div', { class: 'row wrap' }, (v.sources || []).map(x => extLink(x.title, x.url))));
    if (!v.found) {
      return h('div', { class: 'page' }, head, h('div', { class: 'card' }, empty('gamepad', 'OBS Studio não encontrado', 'O Axion procurou a pasta ' + v.dir + ' e ela não existe: o OBS ainda não foi usado neste PC, ou usa outra pasta (o Streamlabs Desktop ainda não é lido). Depois de transmitir ou gravar com o OBS, volte aqui.')), info);
    }
    if (!row) {
      return h('div', { class: 'page' }, head, h('div', { class: 'card' }, empty('gamepad', 'Nenhuma transmissão ou gravação terminada', 'Há logs do OBS, mas nenhum com uma transmissão ou gravação que tenha terminado nos últimos 60 dias. O OBS escreve os números ao parar.')), info);
    }
    const st = obsStat(row), an = row.analysis || { findings: [] }, c = row.context || {};
    const lv = id => { const f = (an.findings || []).find(x => x.id === id); return f ? (f.level === 'importante' ? 'bad' : f.level === 'atencao' ? 'warn' : '') : ''; };
    const tile = (k, val, sub, kind) => h('div', { class: 'net-tile ' + (kind || '') }, h('div', { class: 'k', text: k }), h('div', { class: 'v', text: val }), sub ? h('div', { class: 'tiny faint', text: sub }) : null);
    const verd = OBS_LEVEL[an.level] || OBS_LEVEL.ok;
    const kindName = (OBS_KIND[st.kind] || 'Saída').toLowerCase();
    const chips = [];
    if (c.encoder) chips.push(badge('Codificador: ' + c.encoder + (c.rateControl ? ' ' + c.rateControl : ''), 'outline'));
    if (c.bitrateKbps > 0) chips.push(badge(fmtN(c.bitrateKbps / 1000, 1) + ' Mb/s', 'outline'));
    if (c.outputRes) chips.push(badge(c.outputRes + (c.fps > 0 ? ' a ' + fmtN(c.fps, 0) + ' fps' : '') + (c.baseRes && c.baseRes !== c.outputRes ? ' (tela ' + c.baseRes + ')' : ''), 'outline'));
    if (c.gpu) chips.push(badge(c.gpu, 'outline'));
    const finds = (an.findings || []).map(f => {
      const lvl = f.level === 'importante' ? 'bad' : f.level === 'atencao' ? '' : 'info';
      return h('div', { class: 'callout' + (f.level === 'ok' ? '' : ' warn'), style: { marginTop: '10px' } }, icon(f.level === 'ok' ? 'check' : 'alert'),
        h('div', null, h('b', { text: f.title }), h('div', { class: 'small', style: { marginTop: '3px' }, text: f.text }),
          h('div', { class: 'row wrap', style: { marginTop: '8px', gap: '8px' } },
            f.page && ROUTES[f.page] ? btn(f.pageLabel || 'Abrir', () => go(f.page), 'sm', 'chevron') : null,
            (f.sources || []).map(x => extLink(x.title, x.url)))));
    });
    const hist = r => {
      const s = obsStat(r), a = r.analysis || {}, vd = OBS_LEVEL[a.level] || OBS_LEVEL.ok, rc = r.context || {};
      return h('tr', { class: obsRowKey(r) === obsRowKey(row) ? 'sel' : null },
        h('td', null, h('div', { text: fmtDT(r.at) }), h('div', { class: 'tiny' }, h('button', { class: 'linkbtn tiny', type: 'button', text: obsRowKey(r) === obsRowKey(row) ? 'aberta' : 'ver', onclick: () => { obsOpen = obsRowKey(r); rerender(); } }))),
        h('td', { text: OBS_KIND[s.kind] || 'Saída' }), h('td', { text: obsDur(r.seconds) }),
        h('td', { text: (rc.encoder || '—') + (rc.bitrateKbps > 0 ? ' · ' + fmtN(rc.bitrateKbps / 1000, 1) + ' Mb/s' : '') }),
        h('td', { text: obsPct(s.skippedPct) }), h('td', { text: obsPct(s.laggedPct) }), h('td', { text: s.kind === 'stream' ? obsPct(s.droppedPct) : '—' }),
        h('td', null, badge(vd[0], vd[1])));
    };
    return h('div', { class: 'page' }, head,
      h('div', { class: 'card' },
        h('div', { class: 'row wrap' }, h('h2', { style: { margin: 0 } }, icon('gamepad'), (st.kind === 'stream' ? 'Última transmissão' : 'Última ' + kindName)), badge(verd[0], verd[1]), h('div', { class: 'spacer' }),
          h('span', { class: 'small muted', text: fmtDT(row.at) + (row.seconds > 0 ? ' · ' + obsDur(row.seconds) : '') + ' · ' + fmtN(st.frames >= 0 ? st.frames : 0, 0) + ' quadros' })),
        chips.length ? h('div', { class: 'row wrap', style: { gap: '6px', marginTop: '10px' } }, chips) : null,
        h('div', { class: 'net-tiles' },
          tile('Codificador pulou', obsPct(st.skippedPct), st.skipped >= 0 ? fmtN(st.skipped, 0) + ' de ' + fmtN(st.skippedOf, 0) + ' quadros' : 'não informado', lv('encoding')),
          tile('Placa de vídeo atrasou', obsPct(st.laggedPct), st.lagged >= 0 ? fmtN(st.lagged, 0) + ' quadros não compostos a tempo' : 'não informado', lv('rendering')),
          st.kind === 'stream' ? tile('Rede perdeu', obsPct(st.droppedPct), st.dropped >= 0 ? fmtN(st.dropped, 0) + ' quadros' : 'não informado', lv('network')) : null,
          v.uploadMbps > 0 ? tile('Upload medido', fmtN(v.uploadMbps, 0) + ' Mb/s', 'no teste de rede mais recente') : tile('Upload medido', '—', 'nunca medido: use Medir › Rede e ping')),
        finds),
      rows.length ? h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('clock'), 'Transmissões e gravações lidas'),
        h('div', { class: 'tbl-scroll' }, h('table', { class: 'table' },
          h('thead', null, h('tr', null, ['Data', 'Tipo', 'Duração', 'Codificador', 'Pulados', 'Atrasados', 'Perdidos na rede', 'Situação'].map(x => h('th', { text: x })))),
          h('tbody', null, rows.map(hist))))) : null,
      info);
  }

  // ---------------------------------------------------------------------------
  // Saúde do Windows
  // ---------------------------------------------------------------------------

  const HEALTH_STATUS = { ok: ['em ordem', 'ok'], attention: ['atenção', 'warn'], unread: ['não lido', 'outline'] };

  function healthCheck(c) {
    const st = HEALTH_STATUS[c.status] || HEALTH_STATUS.unread;
    if (c.status === 'attention') {
      const strong = c.severity === 'warning';
      const acts = (c.links || []).map(l => actionButton({ kind: l.page ? 'page' : 'url', label: l.label, page: l.page, url: l.url })).filter(Boolean);
      return h('div', { class: 'callout hl-item' + (strong ? ' warn' : ''), style: { marginTop: '10px' } }, icon(strong ? 'alert' : 'info'),
        h('div', null,
          h('div', { class: 'row wrap', style: { gap: '8px' } }, h('b', { text: c.headline || c.title }), badge(strong ? 'atenção' : 'sugestão', strong ? 'warn' : 'outline')),
          h('div', { class: 'small', style: { marginTop: '3px' }, text: 'Lido: ' + c.value }),
          c.detail ? h('div', { class: 'small', style: { marginTop: '6px' }, text: c.detail }) : null,
          c.criterion ? h('div', { class: 'tiny faint', style: { marginTop: '6px' }, text: c.criterion }) : null,
          acts.length || (c.sources || []).length ? h('div', { class: 'row wrap', style: { marginTop: '8px', gap: '8px' } }, acts, (c.sources || []).map(x => extLink(x.title, x.url))) : null,
          c.covered ? h('div', { class: 'tiny faint', style: { marginTop: '6px' }, text: 'Também aparece no Diagnóstico.' }) : null));
    }
    const ok = c.status === 'ok';
    return h('div', { class: 'hl-row' + (ok ? '' : ' unread') }, icon(ok ? 'check' : 'info'),
      h('div', { class: 'hl-main' },
        h('div', { class: 'hl-title', text: c.title }),
        h('div', { class: 'small ' + (ok ? 'muted' : 'faint'), text: ok ? c.value : (c.detail || 'não foi possível ler') })),
      badge(st[0], st[1]));
  }

  // What DISM and SFC reported, tool by tool.
  function repairResult(r) {
    const kind = { clean: ['check', ''], fixed: ['check', ''], restart: ['refresh', 'warn'] }[r.verdict] || ['alert', 'warn'];
    const label = { ok: 'sem problemas', repaired: 'arquivos repostos', reboot: 'pede reinício', unrepaired: 'não conseguiu corrigir', failed: 'não terminou', unknown: 'mensagem não reconhecida', cancelled: 'cancelado', skipped: 'não executado' };
    return h('div', null,
      h('div', { class: 'callout ' + kind[1] }, icon(kind[0]), h('div', { text: r.summary || '' })),
      h('div', { class: 'findings', style: { marginTop: '10px' } }, (r.steps || []).map(s => h('div', { class: 'finding compact' },
        h('div', { class: 'sev ' + (s.outcome === 'ok' || s.outcome === 'repaired' ? 'good' : s.outcome === 'reboot' || s.outcome === 'skipped' || s.outcome === 'cancelled' ? 'info' : 'critical') }, icon(s.outcome === 'ok' || s.outcome === 'repaired' ? 'check' : s.outcome === 'reboot' ? 'refresh' : s.outcome === 'skipped' || s.outcome === 'cancelled' ? 'info' : 'alertc')),
        h('div', null, h('div', { class: 'ft', text: s.title + ' — ' + (label[s.outcome] || s.outcome) }), h('div', { class: 'fx', text: s.summary }))))));
  }

  function repairCard(rp) {
    const run = async () => {
      const ok = await confirmDlg({ title: 'Reparar os arquivos do Windows?', ok: 'Reparar agora', icon: 'wrench', wide: true,
        body: h('div', null,
          h('p', { text: 'O Axion vai rodar, uma depois da outra, duas ferramentas do próprio Windows: o DISM (/RestoreHealth) e o SFC (/scannow).' }),
          h('ul', { class: 'notes' },
            h('li', { text: 'Pode levar vários minutos, às vezes bem mais, e o PC pode ficar mais lento enquanto roda. Não desligue o PC.' }),
            h('li', { text: 'Não há desfazer: as ferramentas só repõem arquivos de sistema danificados pelas versões originais do Windows. Se nada estiver danificado, nada muda.' }),
            h('li', { text: 'Se faltar um arquivo original, o DISM pode buscá-lo no Windows Update; é preciso estar com internet.' }),
            h('li', { text: 'Dá para cancelar, mas parar no meio deixa o reparo incompleto; depois é só rodar de novo.' }),
            h('li', { text: 'Não verifica o disco (CHKDSK) e não aumenta o FPS: repõe arquivos de sistema danificados.' })),
          h('div', { class: 'callout warn', style: { marginTop: '10px' } }, icon('alert'), h('div', { text: 'Feche os outros programas antes. Se o Windows estiver instalando uma atualização, espere terminar.' }))) });
      if (ok) runJob(api('POST', '/api/repair/run', { confirm: true }), { title: 'Reparando arquivos do Windows' });
    };
    const rel = rp.related || [];
    return h('div', { class: 'card', style: { marginTop: '16px' } },
      h('div', { class: 'row wrap' }, h('h2', { style: { margin: 0 } }, icon('wrench'), 'Reparar arquivos do Windows'), badge('sem desfazer', 'warn')),
      h('p', { class: 'small', text: 'Repõe arquivos de sistema danificados; não aumenta o FPS. Pode ajudar quando o Windows dá erros estranhos ou telas azuis, ou depois de programas que removem componentes do sistema. O Axion roda o DISM e depois o SFC, nesta ordem, que é a que a Microsoft indica.' }),
      rel.length ? h('div', { class: 'callout', style: { marginTop: '10px' } }, icon('info'), h('div', null,
        h('div', { text: 'Itens da sua análise que podem ter relação (nem sempre a causa é arquivo danificado):' }),
        h('ul', { class: 'notes', style: { margin: '6px 0 0' } }, rel.map(x => h('li', { text: x }))))) : null,
      rp.elevated ? null : h('div', { class: 'callout warn', style: { marginTop: '10px' } }, icon('alert'), h('div', { text: 'Exige o Axion como administrador. Abra o programa de novo e aceite o pedido de permissão.' })),
      rp.last ? h('div', { style: { marginTop: '12px' } }, h('div', { class: 'small muted', style: { marginBottom: '6px' }, text: 'Último reparo (' + fmtDT(rp.last.at) + ')' }), repairResult(rp.last)) : null,
      h('div', { class: 'row wrap', style: { marginTop: '12px', gap: '8px' } },
        btn('Reparar arquivos do Windows', run, 'primary', 'wrench', rp.elevated ? {} : { disabled: 'disabled' }),
        (rp.sources || []).map(x => extLink(x.title, x.url))));
  }

  async function pageSaude() {
    const [v, rp] = await Promise.all([get('health', '/api/health'), get('repair', '/api/repair').catch(() => null)]);
    const head = pageHead('Saúde do Windows', 'O que o Windows registra sobre a própria saúde: antivírus, atualizações, discos, bateria e inicialização. O Axion lê e aponta; a única ação aqui é o reparo de arquivos do Windows, que só roda se você confirmar.',
      []);
    const info = h('div', { class: 'card', style: { marginTop: '16px' } }, h('h2', null, icon('book'), 'Como a verificação funciona'),
      h('p', { class: 'small', text: v.honest }), h('p', { class: 'small muted', text: v.privacy }));
    if (!v.scanned) {
      return h('div', { class: 'page' }, head, h('div', { class: 'card' }, empty('shield', 'Ainda não há uma análise', 'A saúde do Windows é lida junto com a análise do PC. Faça a análise para ver cada verificação.', btn('Analisar meu PC', () => rescan(), 'primary', 'pulse'))), info);
    }
    const s = v.summary || {};
    const tile = (k, val, sub, kind) => h('div', { class: 'net-tile ' + (kind || '') }, h('div', { class: 'k', text: k }), h('div', { class: 'v', text: val }), sub ? h('div', { class: 'tiny faint', text: sub }) : null);
    const checks = v.checks || [];
    const groups = (v.groups || []).map(g => {
      const list = checks.filter(c => c.group === g);
      if (!list.length) return null;
      const worst = list.some(c => c.status === 'attention') ? ['pede atenção', 'warn'] : list.every(c => c.status === 'ok') ? ['em ordem', 'ok'] : ['parte não lida', 'outline'];
      // What needs attention first, then what is fine, then what was not read.
      const order = { attention: 0, ok: 1, unread: 2 };
      list.sort((a, b) => order[a.status] - order[b.status]);
      return h('div', { class: 'card', style: { marginTop: '16px' } },
        h('div', { class: 'row wrap' }, h('h2', { style: { margin: 0 } }, icon(g === 'Proteção' ? 'shield' : g === 'Energia' ? 'zap' : g === 'Inicialização' ? 'rocket' : 'hdd'), g), badge(worst[0], worst[1])),
        h('div', { class: 'hl-list' }, list.map(healthCheck)));
    });
    return h('div', { class: 'page' }, head,
      h('div', { class: 'net-tiles', style: { gridTemplateColumns: 'repeat(4, minmax(0, 1fr))' } },
        tile('Em ordem', fmtN(s.ok || 0, 0), 'verificações', s.ok ? 'ok' : ''),
        tile('Pedem atenção', fmtN(s.attention || 0, 0), s.attention ? 'veja abaixo' : 'nenhuma', s.attention ? 'warn' : 'ok'),
        tile('Não lidas', fmtN(s.unread || 0, 0), s.unread ? 'o Windows não deixou ler' : 'nenhuma'),
        tile('Última leitura', fmtDT(v.collected), 'na última análise')),
      groups, rp ? repairCard(rp) : null, info);
  }

  // ---------------------------------------------------------------------------
  // Command palette (Ctrl+K): any page or action in a few keys
  // ---------------------------------------------------------------------------

  const plain = t => String(t == null ? '' : t).normalize('NFD').replace(/[\u0300-\u036f]/g, '').toLowerCase();
  let paletteOpen = false;
  // Words people type that are not in the page title.
  const PAGE_KW = {
    cs2: 'cs2 csgo counter strike', desempenho: 'fps quadros frametime presentmon', rede: 'ping dns bufferbloat wifi internet', dpc: 'dpc isr driver latencia', latencia: 'input lag mouse atraso',
    drivers: 'driver nvidia placa de video atualizar', limpeza: 'temp disco arquivos liberar espaco ssd', inicializacao: 'startup boot programas ao ligar', processos: 'tarefas memoria cpu',
    programas: 'desinstalar remover', apps: 'winget instalar', bios: 'xmp resizable bar', perifericos: 'mouse teclado polling hz', obs: 'live stream transmissao', partidas: 'sessao relatorio',
    otimizacoes: 'tweaks ajustes recomendadas', historico: 'desfazer reverter', excluidos: 'placebo recusa', prova: 'teste antes depois ab', saude: 'sfc dism reparar windows', jogos: 'biblioteca steam epic',
  };

  async function openPalette() {
    const st = S.state || {};
    if (paletteOpen || planOpening || MODALS.length || !st.scanned || isSimple()) return;
    paletteOpen = true;
    let tweaks = [];
    const pages = [];
    NAV.forEach(g => g.pages.forEach(id => pages.push({ ic: g.ic, label: TITLES[id], group: g.title, kw: plain(PAGE_KW[id] || ''), run: () => go(id) })));
    const act = (ic, label, run) => ({ ic, label, group: 'Ação', run });
    const actions = [
      act('refresh', 'Reanalisar o PC', rescan),
      act('zap', 'Aplicar as recomendadas', () => openPlan('recomendado')),
      act('crosshair', 'Aplicar perfil CS2', () => openPlan('cs2')),
      act('download', 'Exportar relatório', exportReport),
      act('shield', 'Criar ponto de restauração', () => runJob(api('POST', '/api/restorepoint', {}), { title: 'Ponto de restauração' })),
      act('sliders', 'Trocar perfil', () => pickProfile(false)),
      act('layout', 'Modo simples', () => setSimple(true)),
      act('gauge', 'Medir FPS', () => go('desempenho')),
      act('wifi', 'Testar a rede', () => go('rede')),
      act('pulse', 'Medir latência do sistema', () => go('dpc')),
      act('mouse', 'Medir polling do mouse', () => go('perifericos')),
      act('check', 'Avaliação completa', () => go('avaliacao')),
    ];
    const input = h('input', { class: 'search', type: 'text', placeholder: 'Digite uma página, ação ou otimização…', 'aria-label': 'Buscar página ou ação', role: 'combobox', 'aria-expanded': 'true', 'aria-controls': 'cmdk-list', 'aria-autocomplete': 'list', autocomplete: 'off', spellcheck: 'false', 'data-autofocus': '' });
    const list = h('ul', { class: 'cmdk-list', id: 'cmdk-list', role: 'listbox', 'aria-label': 'Resultados' });
    let shown = [];
    let cur = 0;
    const m = modal({ title: 'Ir para…', body: h('div', { class: 'cmdk' }, input, list), actions: [], onClose: () => { paletteOpen = false; } });
    m.box.classList.add('cmdk');
    if (m.box.parentElement) m.box.parentElement.classList.add('cmdk-back');

    const run = it => { m.close(); setTimeout(() => it.run(), 0); };
    function paint() {
      list.replaceChildren();
      if (!shown.length) { list.appendChild(h('li', { class: 'cmdk-empty', role: 'presentation', text: 'Nada encontrado. Tente outra palavra.' })); input.removeAttribute('aria-activedescendant'); return; }
      shown.forEach((it, i) => list.appendChild(h('li', { class: 'cmdk-item' + (i === cur ? ' active' : ''), id: 'cmdk-i' + i, role: 'option', 'aria-selected': i === cur ? 'true' : 'false', onmousedown: e => { e.preventDefault(); run(it); }, onmousemove: () => { if (cur !== i) { cur = i; paint(); } } },
        icon(it.ic), h('span', { text: it.label }), h('span', { class: 'cl', text: it.group }))));
      input.setAttribute('aria-activedescendant', 'cmdk-i' + cur);
      const el = document.getElementById('cmdk-i' + cur);
      if (el && el.scrollIntoView) el.scrollIntoView({ block: 'nearest' });
    }
    function filter() {
      const q = plain(input.value).trim();
      const tw = tweaks.map(t => ({ ic: 'sliders', label: t.title, group: 'Otimização', run: () => { optFilter = 'all'; optCat = '*'; optQuery = t.title; if (currentPage() === 'otimizacoes') rerender(); else go('otimizacoes'); } }));
      const all = pages.concat(actions, tw);
      if (!q) { shown = pages.concat(actions).slice(0, 30); }
      else {
        const rank = it => { const l = plain(it.label); return l.startsWith(q) ? 0 : l.split(/[\s(]+/).some(w => w.startsWith(q)) ? 1 : l.includes(q) || (it.kw && it.kw.includes(q)) ? 2 : plain(it.group).includes(q) ? 3 : 9; };
        shown = all.map(it => [rank(it), it]).filter(x => x[0] < 9).sort((a, b) => a[0] - b[0]).map(x => x[1]).slice(0, 30);
      }
      cur = 0;
      paint();
    }
    input.addEventListener('input', filter);
    input.addEventListener('keydown', e => {
      if (e.isComposing) return;
      if (e.key === 'ArrowDown') { e.preventDefault(); if (shown.length) { cur = (cur + 1) % shown.length; paint(); } }
      else if (e.key === 'ArrowUp') { e.preventDefault(); if (shown.length) { cur = (cur - 1 + shown.length) % shown.length; paint(); } }
      else if (e.key === 'Enter') { e.preventDefault(); if (shown[cur]) run(shown[cur]); }
    });
    filter();
    get('tweaks', '/api/tweaks').then(d => { tweaks = d.tweaks || []; if (paletteOpen) filter(); }).catch(() => {});
  }
  document.addEventListener('keydown', e => {
    if ((e.ctrlKey || e.metaKey) && !e.altKey && !e.shiftKey && (e.key === 'k' || e.key === 'K')) { e.preventDefault(); openPalette(); }
  });

  // ---------------------------------------------------------------------------
  // Router and boot
  // ---------------------------------------------------------------------------

  const ROUTES = { saude: pageSaude, painel: pagePainel, avaliacao: pageAvaliacao, apps: pageApps, diagnostico: pageDiag, otimizacoes: pageOpt, prova: pageProva, cs2: pageCS2, jogos: pageGames, desempenho: pagePerf, partidas: pagePartidas, latencia: pageLatencia, dpc: pageDpc, obs: pageObs, rede: pageRede, perifericos: pagePerif, drivers: pageDrivers, limpeza: pageLimpeza, inicializacao: pageStartup, processos: pageProcesses, programas: pagePrograms, bios: pageBios, historico: pageHist, excluidos: pageExcl, sobre: pageSobre };
  let renderSeq = 0;
  let lastPage = '';

  // The tab bar of an entry that holds more than one page (Jogos, Manutenção...).
  function subnav(page) {
    const g = groupOf(page);
    if (g.pages.length < 2) return null;
    const st = S.state || {};
    const cnt = { historico: st.applied || 0, partidas: (st.unseen || []).length };
    return h('div', { class: 'subnav-wrap' }, h('nav', { class: 'subnav', 'aria-label': g.title },
      g.pages.map(id => h('button', { class: 'subtab' + (id === page ? ' active' : ''), type: 'button', 'aria-current': id === page ? 'page' : null, onclick: () => go(id), 'data-focus': 'sub-' + id },
        TITLES[id], cnt[id] ? h('span', { class: 'nav-count', text: String(cnt[id]) }) : null))));
  }
  const withSub = (page, el) => [subnav(page), el].filter(Boolean);

  async function render(keepScroll) {
    renderChrome();
    const content = $('#content');
    // The simple mode is one screen whatever the address says; the sidebar
    // goes away with it (CSS on the app element).
    const simple = isSimple() && S.state && S.state.scanned;
    $('#app').classList.toggle('simple', !!simple);
    const page = simple ? 'simples' : currentPage();
    const same = page === lastPage;
    lastPage = page;
    const y = content.scrollTop;
    const seq = ++renderSeq;
    if (!S.state || !S.state.scanned) { content.replaceChildren(scanView()); return; }
    // Inputs marked with data-focus keep focus and caret across re-renders
    // (read before the skeleton replaces the page, or a tab would lose it).
    const act = document.activeElement;
    const focusKey = act && act.dataset ? act.dataset.focus : null;
    const caret = focusKey && typeof act.selectionStart === 'number' ? act.selectionStart : null;
    if (!same) content.replaceChildren(...withSub(page, h('div', { class: 'page' }, skeleton())));
    // Live lists are read again each time their page is opened.
    if (!same && page === 'processos') delete S.cache.processes;
    try {
      const el = await (simple ? pageSimple() : ROUTES[page]());
      if (seq !== renderSeq) return;
      content.replaceChildren(...withSub(page, el));
      content.scrollTop = same || keepScroll ? y : 0;
      let kept = false;
      if (focusKey) {
        const n = content.querySelector('[data-focus="' + focusKey + '"]');
        if (n) { kept = true; n.focus(); if (caret != null) { try { n.setSelectionRange(caret, caret); } catch (e) { /* not a text input */ } } }
      }
      // A new page, or a re-render that removed the focused control: move the
      // focus to the page title instead of letting it fall to the document.
      const cur = document.activeElement;
      const lost = !cur || cur === document.body || !cur.isConnected;
      if (!kept && !MODALS.length && (!same || lost)) focusPageTitle();
    } catch (e) {
      if (seq !== renderSeq) return;
      if (e.status === 409 && /análise/.test(e.message)) { S.state.scanned = false; content.replaceChildren(scanView()); return; }
      content.replaceChildren(h('div', { class: 'page' }, empty('alertc', 'Não foi possível carregar esta página', e.message, btn('Tentar novamente', () => { invalidate(); render(); }, 'primary', 'refresh'))));
    }
  }
  function rerender(resetScroll) { if (resetScroll) { $('#content').scrollTop = 0; } render(!resetScroll); }

  // One scan follower at a time: every view built while the first analysis
  // runs (the user may click the sidebar) shows the same progress instead of
  // starting another analysis.
  let scanUI = null;
  let scanFollowing = false;

  function scanView() {
    const fill = h('i');
    const bar = h('div', { class: 'progress hidden' }, fill);
    const step = h('div', { class: 'small muted', style: { marginTop: '10px' } });
    const start = btn('Analisar meu PC', () => startScan(null), 'primary hero', 'pulse');
    const view = h('div', { class: 'scan-hero' },
      h('div', { class: 'big' }, h('img', { src: 'logo-256.png', alt: '', width: '120', height: '120', decoding: 'async' })),
      h('h1', { text: 'Vamos analisar o seu PC' }),
      h('p', { text: 'O Axion lê o hardware, o Windows, os drivers, a BIOS/boot e os jogos para montar um diagnóstico e sugerir só o que tem evidência de benefício nesta máquina. A análise não altera nada.' }),
      start, h('div', { style: { marginTop: '22px' } }, bar), step);
    scanUI = { fill, bar, step, start };
    const ui = () => scanUI || { fill, bar, step, start };
    // existing: a scan already running (page reloaded mid-scan, or another
    // view of this window started it) is followed instead of starting another.
    async function startScan(existing) {
      if (scanFollowing) { ui().start.disabled = true; ui().bar.classList.remove('hidden'); return; }
      scanFollowing = true;
      try {
        ui().start.disabled = true;
        ui().bar.classList.remove('hidden');
        let job = existing && existing.id ? existing : null;
        if (!job) {
          try { job = await api('POST', '/api/scan', {}); } catch (e) {
            let running = null;
            if (e.status === 409) {
              try { const st = await api('GET', '/api/state'); running = st && st.job; } catch (e2) { /* ignore */ }
            }
            if (running) job = running;
            else { toast(e.message, 'bad'); ui().start.disabled = false; return; }
          }
        }
        S.busy = job; renderTopbar();
        for (;;) {
          await sleep(300);
          try { job = await api('GET', '/api/jobs/' + job.id); } catch (e) { continue; }
          const u = ui();
          u.bar.classList.remove('hidden'); u.start.disabled = true;
          u.fill.style.width = Math.round((job.progress || 0) * 100) + '%';
          u.step.textContent = job.step || '';
          S.busy = job; renderTopbar();
          if (job.status !== 'running') break;
        }
        S.busy = null;
        if (job.status !== 'done') { toast(job.error || 'A análise não terminou.', 'bad'); ui().start.disabled = false; return; }
        invalidate();
        await refreshState();
        // Keep the page the user chose meanwhile; the Painel only by default.
        if (!location.hash || location.hash === '#' || location.hash === '#/') location.hash = '#/painel';
        render();
      } finally {
        scanFollowing = false;
      }
    }
    if (scanFollowing) { start.disabled = true; bar.classList.remove('hidden'); }
    else if (S.state && S.state.job && S.state.job.kind === 'scan') setTimeout(() => startScan(S.state.job), 50);
    else if (S.state && S.state.settings && S.state.settings.accepted) setTimeout(startScan, 250);
    return view;
  }

  function welcome() {
    return new Promise(resolve => {
      const m = modal({
        title: 'Bem-vindo ao Axion', dismiss: false,
        body: h('div', null,
          h('img', { class: 'welcome-mark', src: 'logo-256.png', alt: '', width: '96', height: '96', decoding: 'async' }),
          h('ul', { class: 'plan-mini welcome-list' }, [
            ['shield', 'A análise só lê o sistema. Nada muda sem você confirmar.'],
            ['book', 'Cada otimização mostra o que faz, a evidência, o risco e as fontes.'],
            ['undo', 'Toda alteração guarda o valor anterior e pode ser desfeita no Histórico. Nos lotes, o Axion também cria um ponto de restauração do Windows.'],
            ['check', 'Ajustes sem evidência de ganho (placebo) não são aplicados — veja "O que não fazemos".']].map(([ic, t]) => h('li', null, icon(ic), h('div', { text: t })))),
          h('p', { class: 'small muted', text: 'O Axion não altera a BIOS nem arquivos de jogos e não interage com anti-cheats.' })),
        actions: [btn('Entendi, continuar', async () => {
          try { await api('POST', '/api/settings', { accepted: true }); } catch (e) { /* continue */ }
          m.close(); resolve();
        }, 'primary hero', 'pulse')],
      });
    });
  }

  async function boot() {
    // Skip link: focus the page (no navigation; inline handlers are blocked
    // by the page's content security policy).
    const skip = $('#skip');
    if (skip) skip.addEventListener('click', e => { e.preventDefault(); const t = $('#content .page-head h1'); (t || $('#content')).focus(); });
    await redeemBootCode();
    if (!TOKEN) {
      $('#content').replaceChildren(h('div', { class: 'page' }, empty('lock', 'Acesso não autorizado',
        'Este link de acesso já foi usado ou expirou. Abra o Axion pelo atalho do programa: se ele já estiver aberto no navegador, abre uma aba nova.')));
      return;
    }
    await refreshState();
    if (!S.state) {
      $('#content').replaceChildren(h('div', { class: 'page' }, empty('alertc', 'O Axion não respondeu', 'Feche e abra o programa novamente.')));
      return;
    }
    window.addEventListener('hashchange', () => render());
    if (!S.state.settings.accepted && !S.state.scanned) {
      render();
      await welcome();
      await refreshState();
    }
    // The use profile is asked once (also on an installation older than 0.7,
    // which never chose one); until then the reading is the competitive one.
    if (S.state && !S.state.settings.profile) {
      render();
      await pickProfile(true);
    }
    render();
    setInterval(() => { if (!S.busy) refreshState(); }, 15000);
  }

  boot();
})();
