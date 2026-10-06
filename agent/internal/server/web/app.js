'use strict';

// ---------- helpers ----------
const $ = (sel, el = document) => el.querySelector(sel);

function h(tag, props = {}, ...kids) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(props)) {
    if (v == null || v === false) continue;
    if (k === 'class') el.className = v;
    else if (k === 'style') el.style.cssText = v;
    else if (k.startsWith('on')) el.addEventListener(k.slice(2), v);
    else if (k in el && typeof v !== 'string') el[k] = v;
    else el.setAttribute(k, v === true ? '' : v);
  }
  for (const kid of kids.flat()) {
    if (kid == null || kid === false) continue;
    el.append(kid instanceof Node ? kid : document.createTextNode(String(kid)));
  }
  return el;
}

async function req(method, url, body, rawType) {
  const opts = { method, headers: { 'X-Barphone-UI': '1' } };
  if (body !== undefined) {
    if (rawType) { opts.headers['Content-Type'] = rawType; opts.body = body; }
    else { opts.headers['Content-Type'] = 'application/json'; opts.body = JSON.stringify(body); }
  }
  const r = await fetch(url, opts);
  const data = (r.headers.get('content-type') || '').includes('json') ? await r.json() : null;
  if (!r.ok) throw new Error((data && (data.message || data.error)) || `${r.status} ${r.statusText}`);
  return data;
}

function toast(text, isError = false) {
  const el = h('div', { class: 'toast' + (isError ? ' error' : '') }, text);
  $('#toasts').append(el);
  setTimeout(() => el.remove(), isError ? 5000 : 2200);
}

const rtf = new Intl.RelativeTimeFormat('ru', { numeric: 'auto' });
function ago(iso) {
  if (!iso) return '';
  const s = (new Date(iso).getTime() - Date.now()) / 1000;
  const a = Math.abs(s);
  if (a < 45) return 'только что';
  if (a < 3600) return rtf.format(Math.round(s / 60), 'minute');
  if (a < 86400) return rtf.format(Math.round(s / 3600), 'hour');
  if (a < 86400 * 7) return rtf.format(Math.round(s / 86400), 'day');
  return new Date(iso).toLocaleDateString('ru');
}

const iconURL = (hash) => `/api/icon/${hash}.png`;
const KIND_LABEL = {
  app: 'Приложение', path: 'Файл или программа', url: 'Ссылка',
  keys: 'Сочетание клавиш', text: 'Текст', system: 'Системное действие', folder: 'Папка',
  macro: 'Макрос', wait: 'Пауза', timer: 'Таймер', stat: 'Живая плитка', trackpad: 'Трекпад', command: 'Команда',
};
const GLYPH = {
  keys: '⌨️', text: '📝', folder: '📁', macro: '⚡', wait: '⏱️', timer: '⏲️', cpu: '📈', ram: '🧠', trackpad: '🖱️', command: '💻',
  media_play_pause: '⏯️', media_next: '⏭️', media_prev: '⏮️', media_stop: '⏹️',
  volume: '🎚️', volume_up: '🔊', volume_down: '🔉', mute: '🔇',
  brightness: '🔆', brightness_up: '☀️', brightness_down: '🔅',
  desktops: '🗂️', desktop_next: '⏩', desktop_prev: '⏪', task_view: '🪟',
  lock: '🔒', sleep: '🌙', display_off: '🖥️', shutdown: '🔌', restart: '🔄',
};
const glyphOf = (b) => (b.kind === 'system' || b.kind === 'stat' ? GLYPH[b.target] : GLYPH[b.kind]);
const systemAction = (id) => (state.systemActions || []).find((a) => a.id === id);

function tileFace(b) {
  const glyph = glyphOf(b);
  return [
    b.icon
      ? h('img', { src: iconURL(b.icon), alt: '' })
      : h('div', { class: 'glyph' }, glyph || (b.kind === 'url' ? '↗' : (b.title || '?').trim().charAt(0).toUpperCase())),
    h('div', { class: 'label' }, b.title),
    b.kind === 'folder' ? h('span', { class: 'count' }, (b.buttons || []).length) : null,
  ].filter(Boolean);
}

// ---------- state ----------
let state = null;
let localVersion = 0; // bumps on every local deck edit
let savedVersion = 0;
let saveTimer = null;

// Profiles: the page edits one of them at a time ("the edited profile"); every deck
// operation goes through curDeck(). Button lookups search all profiles.
let editingProfileId = 'default';
const curProfile = () => state.profiles.find((p) => p.id === editingProfileId) || state.profiles[0];
const curDeck = () => curProfile().deck;
// A folder of the edited profile can be open: the grid, "+" and drag & drop then work on
// its buttons (curList) instead of the profile's top level.
let viewFolderId = null;
const curFolder = () => (viewFolderId && curDeck().buttons.find((b) => b.id === viewFolderId && b.kind === 'folder')) || null;
const curList = () => { const f = curFolder(); return f ? (f.buttons = f.buttons || []) : curDeck().buttons; };
const flatButtons = (bs) => bs.flatMap((b) => [b, ...(b.buttons || [])]);
const allButtons = () => state.profiles.flatMap((p) => flatButtons(p.deck.buttons));
const findButton = (id) => allButtons().find((b) => b.id === id);
// The agent omits empty lists in some answers; the page always wants arrays.
const normProfiles = (ps) => ps.map((p) => ({ ...p, apps: p.apps || [], deck: { ...p.deck, buttons: (p.deck && p.deck.buttons) || [] } }));

async function refresh() {
  const fresh = await req('GET', '/api/state');
  fresh.profiles = normProfiles(fresh.profiles);
  if (state && localVersion !== savedVersion) fresh.profiles = state.profiles; // keep unsaved edits
  state = fresh;
  if (!state.profiles.some((p) => p.id === editingProfileId)) editingProfileId = state.profiles[0].id;
  render();
}

let refreshTimer = null;
function scheduleRefresh() {
  clearTimeout(refreshTimer);
  refreshTimer = setTimeout(() => refresh().catch((e) => console.error(e)), 60);
}

function connectEvents() {
  const es = new EventSource('/api/events');
  es.addEventListener('change', scheduleRefresh);
  es.onerror = () => { es.close(); setTimeout(connectEvents, 2000); };
}

// Deck edits are applied locally right away and saved (debounced) as a whole.
function editDeck(mutate, delay = 0) {
  mutate(curDeck());
  localVersion++;
  render();
  clearTimeout(saveTimer);
  saveTimer = setTimeout(saveDeck, delay);
}

// Profile-level edits (names, app bindings, adding/removing profiles) save right away.
function editProfiles(mutate) {
  mutate(state.profiles);
  localVersion++;
  render();
  clearTimeout(saveTimer);
  return saveDeck();
}

// Saves all profiles at once. New profiles and buttons get their IDs from the agent;
// the edited profile is kept by position when it was just created.
async function saveDeck() {
  const v = localVersion;
  const index = Math.max(0, state.profiles.indexOf(curProfile()));
  try {
    const saved = normProfiles(await req('PUT', '/api/profiles', state.profiles));
    if (v === localVersion) {
      state.profiles = saved;
      savedVersion = v;
      if (!saved.some((p) => p.id === editingProfileId)) editingProfileId = (saved[index] || saved[0]).id;
      render();
    }
    return true;
  } catch (e) {
    savedVersion = localVersion;
    toast('Не сохранилось: ' + e.message, true);
    refresh();
    return false;
  }
}

// ---------- render ----------
function render() {
  if (!state) return;
  const name = $('#machineName');
  if (document.activeElement !== name) name.value = state.machine.name;
  document.title = `barphone — ${state.machine.name}`;
  renderProfiles();
  if (viewFolderId && !curFolder()) viewFolderId = null; // deleted, or another profile
  renderFolderBar();
  renderDeck();
  renderDevices();
  renderRecent();
  renderNet();
  if ($('#editDialog').open) { renderEditPreview(); renderSteps(); }
  if ($('#pairDialog').open) renderPair();
  if ($('#bindDialog').open) renderBind();
  if ($('#addDialog').open) {
    renderApps();
    if (!$('#addDialog [data-panel=system]').hidden) renderSystemList();
  }
}

let dragIndex = null;
let renderedDeck = '';

// Rebuilding the grid on every SSE event (phones connecting, icons arriving) would eat
// clicks and break drags that straddle a refresh, so only redraw when the deck changed.
function renderDeck(force = false) {
  const folder = curFolder();
  const key = editingProfileId + '/' + (folder ? folder.id : '') + JSON.stringify(curDeck());
  if (!force && (key === renderedDeck || dragIndex !== null)) return;
  renderedDeck = key;
  const { columns } = curDeck();
  const buttons = curList();
  $('#colsValue').value = columns;
  const grid = $('#grid');
  grid.style.setProperty('--cols', columns);
  const clearMarks = (el) => el.classList.remove('drop-before', 'drop-after', 'drop-into');
  // The middle half of a folder takes the dragged button in; the edges reorder.
  const intoZone = (e, el) => e.offsetX > el.clientWidth * 0.25 && e.offsetX < el.clientWidth * 0.75;
  const back = folder && h('div', {
    class: 'tile back', 'data-back': true, title: 'Ко всему профилю. Перетащите сюда кнопку, чтобы вынуть её из папки.',
    ondragover: (e) => { if (dragIndex === null) return; e.preventDefault(); e.currentTarget.classList.add('drop-into'); },
    ondragleave: (e) => clearMarks(e.currentTarget),
    ondrop: (e) => {
      e.preventDefault();
      const from = dragIndex;
      dragIndex = null;
      if (from !== null) moveButton(buttons[from].id, null);
    },
  }, h('div', { class: 'glyph' }, '←'), h('div', { class: 'label' }, 'Назад'));
  grid.replaceChildren(
    ...(back ? [back] : []),
    ...buttons.map((b, i) => h('div', {
      class: 'tile', draggable: 'true', 'data-id': b.id,
      title: b.kind === 'folder' ? `${b.title}\nПапка: кнопок ${(b.buttons || []).length}. Клик — открыть.`
        : b.kind === 'macro' ? `${b.title}\nМакрос: шагов ${(b.steps || []).length}`
        : `${b.title}\n${KIND_LABEL[b.kind]}: ${b.target}`,
      ondragstart: (e) => { dragIndex = i; e.currentTarget.classList.add('dragging'); e.dataTransfer.effectAllowed = 'move'; },
      ondragend: () => { dragIndex = null; renderDeck(true); },
      ondragover: (e) => {
        if (dragIndex === null) return;
        e.preventDefault();
        const el = e.currentTarget;
        clearMarks(el);
        if (b.kind === 'folder' && dragIndex !== i && buttons[dragIndex].kind !== 'folder' && intoZone(e, el)) {
          el.classList.add('drop-into');
          return;
        }
        el.classList.add(e.offsetX > el.clientWidth / 2 ? 'drop-after' : 'drop-before');
      },
      ondragleave: (e) => clearMarks(e.currentTarget),
      ondrop: (e) => {
        e.preventDefault();
        const from = dragIndex;
        if (from === null) return;
        dragIndex = null;
        if (e.currentTarget.classList.contains('drop-into')) return moveButton(buttons[from].id, b.id);
        let to = i + (e.offsetX > e.currentTarget.clientWidth / 2 ? 1 : 0);
        if (from < to) to--;
        if (from === to) return renderDeck(true);
        editDeck(() => { const list = curList(); const [m] = list.splice(from, 1); list.splice(to, 0, m); });
      },
    }, tileFace(b))),
    h('div', { class: 'tile add', 'data-add': true, title: folder ? `Добавить кнопку в «${folder.title}»` : 'Добавить кнопку' }, '+'),
  );
}

// Moves a button of the edited profile into a folder, or to the top level (folderId null).
function moveButton(id, folderId) {
  editDeck((d) => {
    let moved = null;
    const take = (list) => {
      const i = list.findIndex((b) => b.id === id);
      if (i >= 0) moved = list.splice(i, 1)[0];
    };
    take(d.buttons);
    for (const b of d.buttons) if (!moved && b.buttons) take(b.buttons);
    if (!moved) return;
    const folder = folderId && moved.kind !== 'folder' ? d.buttons.find((b) => b.id === folderId) : null;
    if (folder) (folder.buttons = folder.buttons || []).push(moved);
    else d.buttons.push(moved);
  });
}

function renderFolderBar() {
  const f = curFolder();
  $('#folderBar').hidden = !f;
  if (f) $('#folderName').textContent = f.title;
}
$('#folderBack').onclick = () => { viewFolderId = null; render(); };
$('#folderEdit').onclick = () => { const f = curFolder(); if (f) openEdit(f.id); };

// One delegated handler: if the grid was rebuilt between mousedown and mouseup the click
// lands on the grid itself, so resolve the tile from the pointer position.
$('#grid').addEventListener('click', (e) => {
  const tile = e.target.closest('.tile') || document.elementFromPoint(e.clientX, e.clientY)?.closest('#grid .tile');
  if (!tile) return;
  if (tile.dataset.add !== undefined) openAdd();
  else if (tile.dataset.back !== undefined) { viewFolderId = null; render(); }
  else if (tile.dataset.id) {
    const b = findButton(tile.dataset.id);
    if (b && b.kind === 'folder') { viewFolderId = b.id; render(); } else openEdit(tile.dataset.id);
  }
});

function renderDevices() {
  const ul = $('#devices');
  if (!state.devices.length) {
    ul.replaceChildren(h('li', { class: 'empty' }, 'Пока ни одного. Нажмите «Подключить телефон».'));
    return;
  }
  ul.replaceChildren(...state.devices.map((d) => h('li', {},
    h('span', { class: 'dot' + (d.online ? ' on' : '') }),
    h('div', { class: 'grow' },
      h('div', { class: 'title' }, d.name),
      h('div', { class: 'sub' }, d.online ? 'на связи' : (d.lastSeen ? 'был(а) ' + ago(d.lastSeen) : 'ещё не подключался')),
    ),
    h('button', {
      class: 'icon-btn', title: 'Отключить телефон', 'aria-label': 'Отключить телефон',
      onclick: async (e) => {
        const btn = e.currentTarget;
        if (!btn.dataset.armed) { btn.dataset.armed = '1'; btn.textContent = '?'; btn.title = 'Нажмите ещё раз, чтобы отключить'; setTimeout(() => { delete btn.dataset.armed; btn.textContent = '✕'; }, 3000); return; }
        try { await req('DELETE', `/api/devices/${encodeURIComponent(d.id)}`); toast(`«${d.name}» отключён`); } catch (err) { toast(err.message, true); }
      },
    }, '✕'),
  )));
}

function renderRecent() {
  const ul = $('#recent');
  const byId = new Map(allButtons().map((b) => [b.id, b]));
  const items = state.recent.map((r) => [r, byId.get(r.id)]).filter(([, b]) => b);
  if (!items.length) {
    ul.replaceChildren(h('li', { class: 'empty' }, 'Здесь появится то, что вы запускали с телефона.'));
    return;
  }
  ul.replaceChildren(...items.map(([r, b]) => h('li', {},
    b.icon ? h('img', { src: iconURL(b.icon), alt: '' }) : h('span', { class: 'dot' }),
    h('div', { class: 'grow' }, h('div', { class: 'title' }, b.title)),
    h('span', { class: 'sub' }, ago(r.at)),
  )));
}

function renderNet() {
  const box = $('#netinfo');
  const addrs = state.lan.addrs;
  box.replaceChildren(
    ...(addrs.length
      ? addrs.map((a) => h('div', { class: 'row' + (a.virtual ? ' muted' : '') },
          h('code', {}, `${a.ip}:${state.lan.port}`), h('span', { class: 'small muted' }, a.iface)))
      : [h('div', { class: 'muted' }, 'Нет подключения к локальной сети.')]),
    renderFirewall(),
  );
  const row = $('#autostartRow');
  row.hidden = !state.autostart.supported;
  $('#autostart').checked = !!state.autostart.enabled;
}

const CATEGORY = { Public: 'общедоступная', Private: 'частная', DomainAuthenticated: 'доменная' };
let fwPending = false;

function renderFirewall() {
  const fw = state.firewall || {};
  if (!fw.supported) return null;
  if (fwPending) return h('div', { class: 'fw pending' }, 'Подтвердите запрос Windows (контроль учётных записей)…');
  if (!fw.checked) {
    return h('div', { class: 'fw pending' }, fw.error ? 'Не удалось проверить брандмауэр: ' + fw.error : 'Проверяю брандмауэр…');
  }
  const net = fw.network ? `Сеть «${fw.network}»${CATEGORY[fw.category] ? ' — ' + CATEGORY[fw.category] : ''}.` : '';
  if (fw.allowed) return h('div', { class: 'fw ok' }, '✓ Брандмауэр пропускает телефоны. ' + net);
  return h('div', { class: 'fw warn' },
    h('div', {}, h('b', {}, 'Брандмауэр Windows не пустит телефон.'), ' ', net),
    h('div', { class: 'small muted' }, 'Кнопка добавит правило только для barphone (Windows спросит права администратора). Подключиться смогут лишь сопряжённые телефоны.'),
    h('button', { class: 'btn primary small', onclick: allowFirewall }, 'Разрешить подключения'),
    h('div', { class: 'small muted' }, 'Вручную: Панель управления → Брандмауэр Защитника Windows → «Разрешить взаимодействие с приложением» → добавьте barphone-agent.exe и отметьте частную и публичную сеть.'),
  );
}

async function allowFirewall() {
  fwPending = true;
  renderNet();
  try {
    state.firewall = await req('POST', '/api/firewall/allow');
    state.firewall.supported = true;
    toast(state.firewall.allowed ? 'Правило брандмауэра добавлено' : 'Правило добавлено, но сеть всё ещё закрыта', !state.firewall.allowed);
  } catch (e) {
    toast(/cancel/i.test(e.message) ? 'Запрос отменён' : 'Брандмауэр: ' + e.message, true);
  }
  fwPending = false;
  renderNet();
}

// ---------- profiles ----------
const profileOf = (key) => state.profiles.find((p) => p.apps.some((a) => a.keys.includes(key)));

function renderProfiles() {
  $('#profileTabs').replaceChildren(
    ...state.profiles.map((p) => h('button', {
      class: 'pill', role: 'tab', 'aria-selected': String(p.id === editingProfileId),
      title: p.id === state.activeProfile ? 'Сейчас этот профиль на телефоне' : '',
      onclick: () => { editingProfileId = p.id; viewFolderId = null; render(); },
    }, p.id === state.activeProfile ? h('span', { class: 'live' }) : null, p.name)),
    h('button', { class: 'pill add', onclick: addProfile, title: 'Новый профиль' }, '+ Профиль'),
  );

  const fg = state.foreground;
  const active = state.profiles.find((p) => p.id === state.activeProfile);
  $('#fgNow').replaceChildren(...(fg
    ? ['Сейчас на компьютере: ', h('b', {}, fg.name || '—'), ` → на телефоне «${active ? active.name : ''}»`]
    : ['Профиль на телефоне: ', h('b', {}, active ? active.name : '')]));

  const p = curProfile();
  const isDefault = p.id === 'default';
  const nameInput = $('#profileName');
  if (document.activeElement !== nameInput) nameInput.value = p.name;
  $('#profileAppsRow').hidden = isDefault;
  $('#profileDelete').hidden = isDefault;
  $('#profileDefaultHint').hidden = !isDefault;
  $('#profileApps').replaceChildren(...p.apps.map((a, i) => h('span', { class: 'chip app', title: a.keys.join('\n') },
    a.name,
    h('button', {
      title: 'Отвязать', 'aria-label': `Отвязать ${a.name}`,
      onclick: () => editProfiles(() => { curProfile().apps.splice(i, 1); }),
    }, '✕'),
  )));
  if (!p.apps.length) $('#profileApps').append(h('span', { class: 'small muted' }, 'ни одного приложения'));
}

async function addProfile() {
  const n = state.profiles.length;
  await editProfiles((ps) => ps.push({ id: '', name: `Профиль ${n}`, apps: [], deck: { columns: curDeck().columns, buttons: [] } }));
  editingProfileId = state.profiles[state.profiles.length - 1].id;
  viewFolderId = null;
  render();
  $('#profileName').focus();
  $('#profileName').select();
}

$('#profileName').addEventListener('change', (e) => {
  const name = e.target.value.trim();
  if (!name) { e.target.value = curProfile().name; return; }
  editProfiles(() => { curProfile().name = name; });
});
$('#profileName').addEventListener('keydown', (e) => { if (e.key === 'Enter') e.target.blur(); });

$('#profileDelete').onclick = (e) => {
  const btn = e.currentTarget;
  if (!btn.dataset.armed) {
    btn.dataset.armed = '1';
    btn.textContent = 'Точно удалить профиль и его кнопки?';
    setTimeout(() => { delete btn.dataset.armed; btn.textContent = 'Удалить профиль'; }, 3000);
    return;
  }
  delete btn.dataset.armed;
  btn.textContent = 'Удалить профиль';
  const id = editingProfileId;
  editingProfileId = 'default';
  editProfiles((ps) => { ps.splice(ps.findIndex((p) => p.id === id), 1); });
};

// --- binding apps to a profile ---
$('#bindApp').onclick = () => {
  $('#bindSearch').value = '';
  $('#bindDialog').showModal();
  renderBind();
  if (!apps) loadApps(false).then(renderBind);
};
$('#bindSearch').addEventListener('input', () => renderBind());

async function bindApp(name, keys) {
  if (!keys.length) { toast(`Не удалось определить «${name}»`, true); return; }
  const ok = await editProfiles(() => { curProfile().apps.push({ name, keys }); });
  if (ok) toast(`«${curProfile().name}» будет включаться для ${name}`);
}

const appKeysCache = new Map(); // installed app target → keys, once asked

// The profile an app is already bound to: by its keys when known, else by the rule name.
function ownerOf(name, keys) {
  return keys.map(profileOf).find(Boolean) || state.profiles.find((p) => p.apps.some((a) => a.name === name));
}

function bindRow(name, sub, owner, onPick, icon) {
  const mine = owner && owner.id === curProfile().id;
  return h('li', { class: owner ? 'disabled' : '', onclick: owner ? null : onPick },
    icon || null,
    h('span', { class: 'name' }, name),
    sub ? h('span', { class: 'keys' }, sub) : null,
    owner ? h('span', { class: mine ? 'added' : 'hint' }, mine ? '✓ привязано' : `в профиле «${owner.name}»`) : null,
  );
}

function renderBind() {
  const recentApps = state.recentApps || [];
  $('#bindRecent').replaceChildren(...(recentApps.length
    ? recentApps.map((a) => {
        const name = a.name || a.keys[0];
        return bindRow(name, a.keys[0].replace(/^\w+:/, ''), ownerOf(name, a.keys), () => bindApp(name, a.keys));
      })
    : [h('li', { class: 'msg' }, 'Пока пусто: переключитесь на нужное приложение на компьютере, и оно появится здесь.')]));
  const q = $('#bindSearch').value.trim().toLowerCase();
  const list = (apps || []).filter((a) => !q || a.name.toLowerCase().includes(q)).slice(0, 200);
  $('#bindApps').replaceChildren(...(apps
    ? list.map((a) => bindRow(a.name, '', ownerOf(a.name, appKeysCache.get(a.target) || []), async () => {
        try {
          const { keys } = await req('GET', `/api/appkeys?kind=${encodeURIComponent(a.kind)}&target=${encodeURIComponent(a.target)}`);
          appKeysCache.set(a.target, keys);
          await bindApp(a.name, keys);
        } catch (err) { toast(err.message, true); }
      }, h('img', {
        src: `/api/appicon?kind=${encodeURIComponent(a.kind)}&target=${encodeURIComponent(a.target)}`, alt: '', loading: 'lazy',
        onerror: (e) => e.currentTarget.replaceWith(h('span', { class: 'ph' })),
      })))
    : [h('li', { class: 'msg' }, 'Читаю список приложений…')]));
}

// ---------- header controls ----------
$('#machineName').addEventListener('change', async (e) => {
  const name = e.target.value.trim();
  if (!name) { e.target.value = state.machine.name; return; }
  try { await req('PUT', '/api/machine', { name }); toast('Имя сохранено'); } catch (err) { toast(err.message, true); }
});
$('#machineName').addEventListener('keydown', (e) => { if (e.key === 'Enter') e.target.blur(); });

$('#colsMinus').onclick = () => editDeck((d) => { d.columns = Math.max(2, d.columns - 1); });
$('#colsPlus').onclick = () => editDeck((d) => { d.columns = Math.min(8, d.columns + 1); });

$('#autostart').addEventListener('change', async (e) => {
  try { await req('PUT', '/api/autostart', { enabled: e.target.checked }); } catch (err) { toast(err.message, true); refresh(); }
});

for (const dlg of document.querySelectorAll('dialog')) {
  dlg.addEventListener('click', (e) => {
    if (e.target.closest('[data-close]') || e.target === dlg) dlg.close();
  });
}

// ---------- add dialog ----------
let apps = null;
let appsLoading = false;

// The add dialog also picks macro steps: stepFor is then the macro's ID.
let stepFor = null;

function openAdd(macroId = null) {
  const dlg = $('#addDialog');
  const folder = curFolder();
  stepFor = macroId;
  const macro = macroId && findButton(macroId);
  $('#addHeading').textContent = macro ? `Шаг макроса «${macro.title}»`
    : folder ? `Добавить в папку «${folder.title}»` : 'Добавить кнопку';
  // A macro step can be a command (without confirmation), nothing else from «Ещё».
  for (const el of document.querySelectorAll('#addDialog .more-item')) el.hidden = Boolean(macro) && el.id !== 'commandForm';
  $('#commandConfirmRow').hidden = Boolean(macro);
  $('#folderForm').hidden = Boolean(folder) || Boolean(macro);
  $('#folderInFolder').hidden = !folder || Boolean(macro);
  selectTab('apps');
  dlg.showModal();
  $('#appSearch').value = '';
  $('#appSearch').focus();
  loadApps(false);
}

function selectTab(name) {
  for (const t of document.querySelectorAll('#addDialog [role=tab]')) t.setAttribute('aria-selected', String(t.dataset.tab === name));
  for (const p of document.querySelectorAll('#addDialog .tab-panel')) p.hidden = p.dataset.panel !== name;
  if (name === 'system') renderSystemList();
  stopRecording();
}
for (const t of document.querySelectorAll('#addDialog [role=tab]')) t.onclick = () => selectTab(t.dataset.tab);
$('#addDialog').addEventListener('close', () => { stepFor = null; });

async function loadApps(refreshList) {
  if (appsLoading || (apps && !refreshList)) return renderApps();
  appsLoading = true;
  renderApps();
  try { apps = await req('GET', '/api/apps' + (refreshList ? '?refresh=1' : '')); } catch (e) { toast('Список приложений: ' + e.message, true); apps = apps || []; }
  appsLoading = false;
  renderApps();
}
$('#appsRefresh').onclick = () => loadApps(true);
$('#appSearch').addEventListener('input', () => renderApps());

function renderApps() {
  const ul = $('#appList');
  if (appsLoading && !apps) { ul.replaceChildren(h('li', { class: 'msg' }, 'Читаю список приложений…')); return; }
  const q = $('#appSearch').value.trim().toLowerCase();
  const added = new Set(stepFor ? [] : curList().filter((b) => b.kind === 'app').map((b) => b.target));
  const list = (apps || []).filter((a) => !q || a.name.toLowerCase().includes(q));
  if (!list.length) { ul.replaceChildren(h('li', { class: 'msg' }, q ? 'Ничего не нашлось. Попробуйте вкладку «Файл или программа».' : 'Приложения не найдены.')); return; }
  const scroll = ul.scrollTop;
  ul.replaceChildren(...list.map((a) => h('li', { onclick: () => addButton({ kind: a.kind, target: a.target, title: a.name }) },
    h('img', {
      src: `/api/appicon?kind=${encodeURIComponent(a.kind)}&target=${encodeURIComponent(a.target)}`, alt: '', loading: 'lazy',
      onerror: (e) => e.currentTarget.replaceWith(h('span', { class: 'ph' })),
    }),
    h('span', { class: 'name' }, a.name),
    added.has(a.target) ? h('span', { class: 'added' }, '✓ на деке') : null,
  )));
  ul.scrollTop = scroll;
}

// Saves right away and only reports success once the agent has accepted the button.
async function addButton(b) {
  if (stepFor) return addStep(b);
  const folder = curFolder();
  const nb = { id: '', title: b.title || '', kind: b.kind, target: b.target || '', args: b.args || '' };
  if (b.kind === 'command') Object.assign(nb, { dir: b.dir || '', confirm: Boolean(b.confirm), keepOpen: Boolean(b.keepOpen) });
  if (b.kind === 'folder') nb.buttons = [];
  curList().push(nb);
  localVersion++;
  render();
  clearTimeout(saveTimer);
  const ok = await saveDeck();
  if (ok) toast(`Добавлено${folder ? ` в «${folder.title}»` : ''}: ${b.title || nb.target.trim()}`);
  return ok;
}

// --- macros ---
async function addStep(b) {
  const macro = findButton(stepFor);
  $('#addDialog').close();
  if (!macro) return false;
  (macro.steps = macro.steps || []).push({ title: b.title || '', kind: b.kind, target: b.target || '', args: b.args || '', dir: b.dir || '', keepOpen: Boolean(b.keepOpen) });
  localVersion++;
  render();
  clearTimeout(saveTimer);
  return saveDeck();
}

function editSteps(mutate, delay = 0) {
  editDeck(() => {
    const m = editing();
    if (m) mutate((m.steps = m.steps || []));
  }, delay);
}

$('#stepAdd').onclick = () => openAdd(editingId);
$('#stepWait').onclick = () => editSteps((steps) => steps.push({ kind: 'wait', target: '1000', title: '' }));

const stepTyping = (b) => (b.steps || []).some((st) => st.kind === 'keys' || st.kind === 'text');

let renderedSteps = '';
function renderSteps(force = false) {
  const b = editing();
  const row = $('#editStepsRow');
  row.hidden = !b || b.kind !== 'macro';
  if (row.hidden) return;
  // Typing into the browser tab would be the result of «Проверить» for keys and text steps.
  $('#editTest').hidden = stepTyping(b);
  const steps = b.steps || [];
  const key = b.id + JSON.stringify(steps);
  if (!force && key === renderedSteps) return; // keep focus in a pause being edited
  renderedSteps = key;
  if (!steps.length) {
    $('#editSteps').replaceChildren(h('li', { class: 'empty' }, 'Пока ни одного шага. Добавьте действия и паузы между ними.'));
    return;
  }
  $('#editSteps').replaceChildren(...steps.map((st, i) => h('li', {},
    h('span', { class: 'n' }, i + 1),
    st.kind === 'wait'
      ? h('span', { class: 'what' }, h('span', { class: 'ico' }, '⏱️'), 'Пауза',
          h('input', {
            type: 'number', min: '0.05', max: '60', step: '0.05', value: String(Number(st.target) / 1000), 'aria-label': 'Секунд',
            onchange: (e) => {
              const ms = Math.round(Math.min(60, Math.max(0.05, Number(e.target.value) || 1)) * 1000);
              editSteps((list) => { list[i].target = String(ms); list[i].title = ''; });
            },
          }), 'с')
      : h('span', { class: 'what', title: `${KIND_LABEL[st.kind]}: ${st.target}` },
          ['app', 'path'].includes(st.kind)
            ? h('img', {
                src: `/api/appicon?kind=${encodeURIComponent(st.kind)}&target=${encodeURIComponent(st.target)}`, alt: '',
                onerror: (e) => e.currentTarget.replaceWith(h('span', { class: 'ico' }, '•')),
              })
            : h('span', { class: 'ico' }, glyphOf(st) || (st.kind === 'url' ? '↗' : '•')),
          h('span', {}, st.title || st.target)),
    h('button', { type: 'button', class: 'icon-btn', title: 'Выше', 'aria-label': 'Выше', disabled: i === 0,
      onclick: () => editSteps((list) => list.splice(i - 1, 0, ...list.splice(i, 1))) }, '↑'),
    h('button', { type: 'button', class: 'icon-btn', title: 'Ниже', 'aria-label': 'Ниже', disabled: i === steps.length - 1,
      onclick: () => editSteps((list) => list.splice(i + 1, 0, ...list.splice(i, 1))) }, '↓'),
    h('button', { type: 'button', class: 'icon-btn', title: 'Убрать шаг', 'aria-label': 'Убрать шаг',
      onclick: () => editSteps((list) => list.splice(i, 1)) }, '✕'),
  )));
}

$('#commandForm').addEventListener('submit', (e) => {
  e.preventDefault();
  addButton({
    kind: 'command', target: $('#commandTarget').value.trim(), dir: $('#commandDir').value.trim(), title: $('#commandTitle').value.trim(),
    confirm: $('#commandConfirm').checked, keepOpen: $('#commandKeep').checked,
  });
  e.target.reset();
  $('#addDialog').close();
});

$('#addTrackpad').onclick = () => {
  $('#addDialog').close();
  addButton({ kind: 'trackpad', title: 'Трекпад' });
};

for (const [id, target, title] of [['#statCpu', 'cpu', 'Процессор'], ['#statRam', 'ram', 'Память']]) {
  $(id).onclick = () => {
    $('#addDialog').close();
    addButton({ kind: 'stat', target, title });
  };
}

$('#timerForm').addEventListener('submit', (e) => {
  e.preventDefault();
  const seconds = Math.round(Number($('#timerMinutes').value) * 60);
  addButton({ kind: 'timer', target: String(Math.min(86400, Math.max(1, seconds))), title: $('#timerTitle').value.trim() });
  e.target.reset();
  $('#addDialog').close();
});

// Timer durations are stored in seconds and edited in minutes ("1,5" = 90 s).
const timerMinutes = (b) => String(Number(b.target) / 60).replace('.', ',');

$('#macroForm').addEventListener('submit', async (e) => {
  e.preventDefault();
  const title = $('#macroTitle').value.trim();
  e.target.reset();
  $('#addDialog').close();
  if (await addButton({ kind: 'macro', title })) {
    const list = curList();
    const m = list[list.length - 1];
    if (m && m.kind === 'macro') openEdit(m.id);
  }
});

$('#folderForm').addEventListener('submit', (e) => {
  e.preventDefault();
  addButton({ kind: 'folder', title: $('#folderTitle').value.trim() });
  e.target.reset();
  $('#addDialog').close();
});

$('#pickFile').onclick = async () => {
  try {
    const { path } = await req('POST', '/api/pickfile');
    if (path) {
      $('#pathTarget').value = path;
      if (!$('#pathTitle').value) $('#pathTitle').value = path.split(/[\\/]/).pop().replace(/\.(exe|lnk|app|bat|cmd)$/i, '');
    }
  } catch (e) { toast(e.message, true); }
};
$('#pathForm').addEventListener('submit', (e) => {
  e.preventDefault();
  addButton({ kind: 'path', target: $('#pathTarget').value.trim(), args: $('#pathArgs').value.trim(), title: $('#pathTitle').value.trim() });
  e.target.reset();
  $('#addDialog').close();
});
// --- key combinations ---
const KEY_PRESETS = [
  ['Win+D', 'Свернуть всё'], ['Alt+Tab', 'Другое окно'], ['Ctrl+C', 'Копировать'], ['Ctrl+V', 'Вставить'],
  ['Ctrl+Z', 'Отменить'], ['Win+Shift+S', 'Скриншот области'], ['Win+V', 'Буфер обмена'],
  ['Ctrl+Shift+Escape', 'Диспетчер задач'], ['Alt+F4', 'Закрыть окно'], ['F5', 'Обновить'],
];
$('#keysPresets').replaceChildren(...KEY_PRESETS.map(([combo, title]) => h('button', {
  type: 'button', class: 'chip', title: combo,
  onclick: () => { $('#keysTarget').value = combo; $('#keysTitle').value = title; },
}, h('b', {}, combo), title)));

// Browser key codes → names the agent understands (see agent/internal/launch/keys.go).
const CODE_NAMES = {
  ArrowUp: 'Up', ArrowDown: 'Down', ArrowLeft: 'Left', ArrowRight: 'Right', ContextMenu: 'Menu',
  Semicolon: ';', Equal: '=', Comma: ',', Minus: '-', Period: '.', Slash: '/', Backquote: '`',
  BracketLeft: '[', Backslash: '\\', BracketRight: ']', Quote: "'",
};
function keyName(code) {
  if (/^Key[A-Z]$/.test(code)) return code.slice(3);
  if (/^(Digit|Numpad)[0-9]$/.test(code)) return code.slice(-1);
  if (/^F\d{1,2}$/.test(code)) return code;
  if (CODE_NAMES[code]) return CODE_NAMES[code];
  if (['Enter', 'Escape', 'Tab', 'Space', 'Backspace', 'Delete', 'Insert', 'Home', 'End', 'PageUp', 'PageDown',
    'PrintScreen', 'Pause', 'CapsLock', 'NumLock', 'ScrollLock'].includes(code)) return code;
  return null;
}

let recording = false;
function stopRecording() {
  recording = false;
  $('#keysTarget').classList.remove('recording');
  $('#keysRecord').textContent = 'Записать';
}
$('#keysRecord').onclick = () => {
  if (recording) return stopRecording();
  recording = true;
  $('#keysTarget').classList.add('recording');
  $('#keysRecord').textContent = 'Нажмите сочетание…';
  $('#keysTarget').focus();
};
$('#keysTarget').addEventListener('keydown', (e) => {
  if (!recording) return;
  e.preventDefault();
  const key = keyName(e.code);
  if (!key) return; // a modifier on its own: wait for the main key
  const mods = [e.metaKey && 'Win', e.ctrlKey && 'Ctrl', e.altKey && 'Alt', e.shiftKey && 'Shift'].filter(Boolean);
  $('#keysTarget').value = [...mods, key].join('+');
  stopRecording();
});
$('#keysForm').addEventListener('submit', (e) => {
  e.preventDefault();
  addButton({ kind: 'keys', target: $('#keysTarget').value.trim(), title: $('#keysTitle').value.trim() });
  e.target.reset();
  $('#addDialog').close();
});

// --- text ---
$('#textForm').addEventListener('submit', (e) => {
  e.preventDefault();
  let text = $('#textTarget').value;
  if ($('#textEnter').checked && !text.endsWith('\n')) text += '\n';
  addButton({ kind: 'text', target: text, title: $('#textTitle').value.trim() });
  e.target.reset();
  $('#addDialog').close();
});

// --- system actions ---
function renderSystemList() {
  const added = new Set(stepFor ? [] : curList().filter((b) => b.kind === 'system').map((b) => b.target));
  // Shutdown and restart need the phone's confirmation: never a macro step.
  const actions = (state.systemActions || []).filter((a) => !(stepFor && a.confirm));
  $('#systemList').replaceChildren(...actions.map((a) => h('li', {
    onclick: () => addButton({ kind: 'system', target: a.id, title: a.title }),
  },
    h('span', { class: 'glyph-ico' }, GLYPH[a.id] || '⚙️'),
    h('span', { class: 'name' }, a.title),
    a.slider ? h('span', { class: 'hint' }, a.id === 'volume' ? 'тап — без звука, удержание — ползунок' : 'удержание — ползунок') : null,
    a.id.startsWith('brightness') ? h('span', { class: 'hint' }, 'Windows: экран ноутбука и мониторы с DDC/CI') : null,
    a.swipe ? h('span', { class: 'hint' }, 'свайп по плитке — соседний стол, тап — обзор, удержание — список') : null,
    a.confirm ? h('span', { class: 'hint' }, 'спросит подтверждение') : null,
    added.has(a.id) ? h('span', { class: 'added' }, '✓ на деке') : null,
  )));
}

$('#urlForm').addEventListener('submit', (e) => {
  e.preventDefault();
  addButton({ kind: 'url', target: $('#urlTarget').value.trim(), title: $('#urlTitle').value.trim() });
  e.target.reset();
  $('#addDialog').close();
});

// ---------- edit dialog ----------
let editingId = null;
const editing = () => findButton(editingId);

function openEdit(id) {
  editingId = id;
  const b = editing();
  if (!b) return;
  const action = b.kind === 'system' ? systemAction(b.target) : null;
  $('#editTitle').value = b.title;
  $('#editTargetLabel').textContent = { keys: 'Сочетание клавиш', system: 'Действие', timer: 'Длительность, минут', command: 'Команда' }[b.kind] || 'Что запускать';
  for (const id of ['#editDirRow', '#editConfirmRow', '#editKeepRow']) $(id).hidden = b.kind !== 'command';
  $('#editDir').value = b.dir || '';
  $('#editConfirm').checked = Boolean(b.confirm);
  $('#editKeep').checked = Boolean(b.keepOpen);
  $('#editTarget').value = action ? action.title : b.kind === 'timer' ? timerMinutes(b) : b.target;
  $('#editTarget').readOnly = b.kind === 'app' || b.kind === 'system';
  $('#editTargetRow').hidden = ['text', 'folder', 'macro', 'stat', 'trackpad'].includes(b.kind);
  $('#editTextRow').hidden = b.kind !== 'text';
  $('#editText').value = b.kind === 'text' ? b.target : '';
  $('#editArgs').value = b.args || '';
  $('#editArgsRow').hidden = b.kind !== 'path';
  $('#editRunning').value = b.onRunning || '';
  $('#editRunningRow').hidden = !['app', 'path'].includes(b.kind);
  // Typing keys/text would land in this browser tab, and power actions need the phone's confirmation.
  $('#editTest').hidden = ['keys', 'text', 'folder', 'timer', 'trackpad'].includes(b.kind) || Boolean(action && action.confirm);
  renderEditPlace(b);
  const hints = {
    app: ' · чтобы выбрать другое, добавьте новую кнопку',
    keys: ' · уходит в активное окно на компьютере',
    text: ' · вводится в активное окно, перенос строки — Enter',
    folder: ' · на телефоне открывается тапом; кнопки внутри — в самой папке на деке',
    macro: ' · уже открытая программа выводится вперёд; пока макрос идёт, второй не запустится',
    timer: ' · отсчёт идёт на телефоне: тап — старт, ещё тап — остановить',
    stat: ' · на телефоне показывает загрузку и обновляется каждые пару секунд; тап — диспетчер задач',
    trackpad: ' · тап открывает на телефоне трекпад и клавиатуру; удалите кнопку — и телефон больше не сможет управлять мышью',
    command: ' · запускается в окне консоли на этом ПК; когда закончится, телефон получит уведомление',
  };
  $('#editKind').textContent = KIND_LABEL[b.kind] + (hints[b.kind] || '') +
    (action && action.slider ? (action.id === 'volume'
      ? ' · на телефоне: тап — без звука, удержание и ведение пальцем — громкость'
      : ' · на телефоне: удержание и ведение пальцем — уровень') : '') +
    (action && action.confirm ? ' · телефон спросит подтверждение' : '') +
    (action && action.swipe ? ' · на телефоне: свайп по плитке — соседний рабочий стол, тап — обзор, удержание — список столов' : '');
  const del = $('#editDelete');
  delete del.dataset.armed;
  del.textContent = b.kind === 'folder' ? 'Удалить папку' : 'Удалить';
  renderEditPreview();
  renderSteps(true);
  $('#editDialog').showModal();
}

// "Где лежит": the top level of the profile or one of its folders.
function renderEditPlace(b) {
  const folders = curDeck().buttons.filter((x) => x.kind === 'folder');
  const row = $('#editPlaceRow');
  row.hidden = b.kind === 'folder' || !folders.length;
  if (row.hidden) return;
  const owner = folders.find((f) => (f.buttons || []).some((x) => x.id === b.id));
  $('#editPlace').replaceChildren(
    h('option', { value: '' }, `Прямо в профиле «${curProfile().name}»`),
    ...folders.map((f) => h('option', { value: f.id }, `📁 ${f.title}`)),
  );
  $('#editPlace').value = owner ? owner.id : '';
}
$('#editPlace').addEventListener('change', (e) => moveButton(editingId, e.target.value || null));

function renderEditPreview() {
  const b = editing();
  if (!b) { $('#editDialog').close(); return; }
  $('#editPreview').replaceChildren(...tileFace(b));
}

function bindEditField(sel, field) {
  $(sel).addEventListener('input', (e) => {
    let value = e.target.value;
    const b = editing();
    // Don't save a combination that is still being typed ("Ctrl+Shift+").
    if (b && b.kind === 'keys' && field === 'target' && (!value.trim() || /\+\s*$/.test(value))) return;
    let retitle = false;
    if (b && b.kind === 'timer' && field === 'target') {
      const seconds = Math.round(Number(value.replace(',', '.')) * 60);
      if (!(seconds >= 1 && seconds <= 86400)) return;
      value = String(seconds);
      retitle = /^Таймер \d/.test(b.title); // an automatic title follows the duration
    }
    editDeck(() => {
      const cur = editing();
      if (!cur) return;
      cur[field] = value;
      if (retitle) cur.title = '';
    }, 600);
  });
}
bindEditField('#editTitle', 'title');
bindEditField('#editTarget', 'target');
bindEditField('#editArgs', 'args');
bindEditField('#editText', 'target');
bindEditField('#editDir', 'dir');
for (const [sel, field] of [['#editConfirm', 'confirm'], ['#editKeep', 'keepOpen']]) {
  $(sel).addEventListener('change', (e) => {
    const value = e.target.checked;
    editDeck(() => { const b = editing(); if (b) b[field] = value; });
  });
}
$('#editRunning').addEventListener('change', (e) => {
  const value = e.target.value;
  editDeck(() => { const b = editing(); if (b) b.onRunning = value; });
});
$('#editForm').addEventListener('submit', (e) => { e.preventDefault(); $('#editDialog').close(); });
$('#editDialog').addEventListener('close', () => {
  if (localVersion !== savedVersion) { clearTimeout(saveTimer); saveDeck(); }
});

$('#editDelete').onclick = (e) => {
  const btn = e.currentTarget;
  const b = editing();
  const isFolder = Boolean(b && b.kind === 'folder');
  if (!btn.dataset.armed) {
    const inside = isFolder ? (b.buttons || []).length : 0;
    btn.dataset.armed = '1';
    btn.textContent = inside ? `Удалить папку и ${inside} кн. в ней?` : 'Точно удалить?';
    setTimeout(() => { delete btn.dataset.armed; btn.textContent = isFolder ? 'Удалить папку' : 'Удалить'; }, 3000);
    return;
  }
  const id = editingId;
  $('#editDialog').close();
  const drop = (list) => list.filter((x) => x.id !== id).map((x) => (x.buttons ? { ...x, buttons: drop(x.buttons) } : x));
  editDeck(() => { for (const p of state.profiles) p.deck.buttons = drop(p.deck.buttons); });
};

$('#editTest').onclick = async () => {
  if (localVersion !== savedVersion) { clearTimeout(saveTimer); await saveDeck(); }
  try { await req('POST', `/api/buttons/${encodeURIComponent(editingId)}/launch`); toast(editing()?.kind === 'macro' ? 'Макрос запущен' : 'Запущено'); } catch (e) { toast('Не запустилось: ' + e.message, true); }
};

$('#iconFile').addEventListener('change', async (e) => {
  const file = e.target.files[0];
  e.target.value = '';
  if (!file) return;
  try { await req('PUT', `/api/buttons/${encodeURIComponent(editingId)}/icon`, file, file.type || 'application/octet-stream'); } catch (err) { toast('Иконка: ' + err.message, true); }
});
$('#iconReset').onclick = async () => {
  try { await req('DELETE', `/api/buttons/${encodeURIComponent(editingId)}/icon`); } catch (err) { toast(err.message, true); }
};

// ---------- pairing ----------
let pairWatch = null; // { devicesBefore, success }
let pairTick = null;

$('#pairBtn').onclick = startPairing;

async function startPairing() {
  // Never show the previous (now invalid) code while the new one is on its way.
  const body = $('#pairBody');
  pairWatch = null;
  delete body.dataset.code;
  body.replaceChildren(h('div', { class: 'muted' }, 'Создаю код…'));
  const dlg = $('#pairDialog');
  if (!dlg.open) dlg.showModal();
  try {
    state.pairing = await req('POST', '/api/pairing');
  } catch (e) { toast(e.message, true); dlg.close(); return; }
  pairWatch = { devicesBefore: new Set(state.devices.map((d) => d.id + d.pairedAt)), success: null };
  renderPair();
  clearInterval(pairTick);
  pairTick = setInterval(renderPairCountdown, 1000);
}

$('#pairDialog').addEventListener('close', () => {
  clearInterval(pairTick);
  if (state.pairing && state.pairing.active) req('DELETE', '/api/pairing').catch(() => {});
  pairWatch = null;
});

function renderPair() {
  const body = $('#pairBody');
  if (!pairWatch) return;
  const fresh = state.devices.find((d) => !pairWatch.devicesBefore.has(d.id + d.pairedAt));
  if (fresh && !pairWatch.success) {
    pairWatch.success = fresh;
    clearInterval(pairTick);
    setTimeout(() => { if ($('#pairDialog').open && pairWatch && pairWatch.success) $('#pairDialog').close(); }, 2200);
  }
  if (pairWatch.success) {
    body.replaceChildren(
      h('div', { class: 'ok' }, '✓'),
      h('div', {}, h('b', {}, pairWatch.success.name), ' подключён'),
      h('div', { class: 'muted' }, 'Дека уже на экране телефона.'),
    );
    return;
  }
  const p = state.pairing;
  if (!p || !p.active) {
    body.replaceChildren(
      h('div', { class: 'muted' }, 'Код больше не действует (истёк или было слишком много неверных попыток).'),
      h('button', { class: 'btn primary', onclick: startPairing }, 'Новый код'),
    );
    return;
  }
  if (body.dataset.code === p.code && body.firstChild) { renderPairCountdown(); return; }
  body.dataset.code = p.code;
  const ips = state.lan.addrs.map((a) => a.ip);
  body.replaceChildren(
    h('img', { class: 'qr', src: p.qr, alt: 'QR-код для подключения' }),
    h('div', { class: 'code' }, p.code.slice(0, 3) + ' ' + p.code.slice(3)),
    h('div', { class: 'muted', id: 'pairCountdown' }),
    h('ol', {},
      h('li', {}, 'Откройте barphone на телефоне'),
      h('li', {}, 'Смахните вниз → «Добавить компьютер»'),
      h('li', {}, 'Наведите камеру на QR или введите код'),
    ),
    ips.length
      ? h('div', { class: 'small muted' }, `Телефон должен быть в той же сети: ${ips.join(', ')} · порт ${state.lan.port}`)
      : h('div', { class: 'small', style: 'color: var(--danger)' }, 'Компьютер не подключён к локальной сети.'),
  );
  renderPairCountdown();
}

function renderPairCountdown() {
  const el = $('#pairCountdown');
  const p = state && state.pairing;
  if (!el || !p || !p.active) return;
  const left = Math.max(0, Math.round((new Date(p.expiresAt).getTime() - Date.now()) / 1000));
  if (left === 0) { state.pairing = { active: false }; delete $('#pairBody').dataset.code; renderPair(); return; }
  el.textContent = `Код действует ещё ${Math.floor(left / 60)}:${String(left % 60).padStart(2, '0')}`;
}

// ---------- start ----------
refresh()
  .then(() => {
    connectEvents();
    if (location.hash === '#pair') { history.replaceState(null, '', '/'); startPairing(); }
  })
  .catch((e) => toast('Агент недоступен: ' + e.message, true));
setInterval(() => { if (state) { renderDevices(); renderRecent(); } }, 30000);
