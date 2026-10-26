'use strict';
(() => {
const $ = id => document.getElementById(id);
// Messages come from Go (internal/i18n/messages.json); %s are filled in order.
const t = (key, ...args) => { let index = 0; return (window.I18N.messages[key] ?? key).replace(/%[sd]/g, () => String(args[index++])); };
// Desktop window: no browser zoom, reload, context menu or file drop.
const editable = target => target.closest && target.closest('input,textarea');
document.addEventListener('contextmenu', event => { if (!editable(event.target)) event.preventDefault(); });
document.addEventListener('wheel', event => { if (event.ctrlKey) event.preventDefault(); }, {passive: false});
document.addEventListener('keydown', event => {
  const key = event.key.toLowerCase();
  if (key === 'f5' || (event.ctrlKey && ['r', 'p', 'f', 'g', 'u', 's', '+', '-', '=', '0'].includes(key))) event.preventDefault();
});
// Go answers totp_call asynchronously through window.totpDone.
const pending = new Map(); let sequence = 0;
window.totpDone = ({id, error, result}) => { const call = pending.get(id); if (!call) return; pending.delete(id); error ? call.reject(new Error(error)) : call.resolve(result); };
const call = (method, ...args) => new Promise((resolve, reject) => { const id = ++sequence; pending.set(id, {resolve, reject}); window.totp_call(id, method, args); });
// Notifications float over the window (top layer, above dialogs) and never move
// the layout. At most `visible` at once; the rest wait their turn.
const toasts = {visible: 4, queue: []};
function notice(message, error = false, action = null) { toasts.queue.push({message, error, action}); flushToasts(); }
function flushToasts() {
  const box = $('toasts');
  while (box.childElementCount < toasts.visible && toasts.queue.length) box.append(toast(toasts.queue.shift()));
  if (!box.childElementCount) { if (box.matches(':popover-open')) box.hidePopover(); return; }
  // Re-open so the stack stays above a dialog opened since.
  if (box.matches(':popover-open')) box.hidePopover();
  box.showPopover();
}
// A toast with an action stays until it is used or closed.
function toast({message, error, action}) {
  const item = document.createElement('div'); item.className = error ? 'toast error' : 'toast';
  const text = document.createElement('p'); text.textContent = message; text.title = message;
  const close = document.createElement('button'); close.type = 'button'; close.className = 'tool'; close.title = t('close'); close.setAttribute('aria-label', t('close')); close.textContent = '×';
  let timer = 0;
  const remove = () => { clearTimeout(timer); item.remove(); flushToasts(); };
  const arm = () => { clearTimeout(timer); if (!action) timer = setTimeout(remove, error ? 9000 : 4500); };
  close.addEventListener('click', remove);
  item.addEventListener('mouseenter', () => clearTimeout(timer));
  item.addEventListener('mouseleave', arm);
  item.append(text);
  if (action) {
    const button = document.createElement('button'); button.type = 'button'; button.className = 'small'; button.textContent = action.label;
    button.addEventListener('click', () => run(button, async () => { await action.run(); remove(); }));
    item.append(button);
  }
  item.append(close); arm();
  return item;
}
async function run(button, action) {
  button.disabled = true;
  try { await action(); } catch (error) { notice(error.message, true); } finally { button.disabled = false; }
}
function confirmed(message) {
  $('confirm-message').textContent = message; $('confirm').returnValue = '';
  return new Promise(resolve => { $('confirm').addEventListener('close', () => resolve($('confirm').returnValue === 'ok'), {once: true}); $('confirm').showModal(); $('confirm-ok').focus(); });
}
// Go copies outside the WebView, out of the clipboard history (Win+V).
async function copy(text) {
  try { await call('copy', text); return; } catch {}
  try { await navigator.clipboard.writeText(text); return; } catch {}
  throw new Error(t('copy.failed'));
}
const icons = {
  copy: '<rect x="5" y="5" width="9" height="9" rx="1.5"/><path d="M11 5V3.5A1.5 1.5 0 0 0 9.5 2h-6A1.5 1.5 0 0 0 2 3.5v6A1.5 1.5 0 0 0 3.5 11H5"/>',
  wrench: '<path d="M10.6 1.8a3.6 3.6 0 0 0-3.4 4.7l-5 5a1.5 1.5 0 0 0 2.1 2.1l5-5a3.6 3.6 0 0 0 4.7-3.4l-2.1 2.1-2-.5-.5-2z"/>',
  trash: '<path d="M2.5 4h11M6 4V2.5h4V4M4 4l.7 9.5h6.6L12 4M6.5 6.5v4.5M9.5 6.5v4.5"/>',
  plus: '<path d="M8 2.5v11M2.5 8h11"/>',
  pencil: '<path d="M11 2.5l2.5 2.5L6 12.5 2.5 13.5 3.5 10z"/><path d="M9.5 4l2.5 2.5"/>',
  tag: '<path d="M2 2.5h5.5L14 9l-5 5-6.5-6.5z"/><circle cx="5.2" cy="5.7" r="1"/>',
  eye: '<path d="M1 8s2.5-5 7-5 7 5 7 5-2.5 5-7 5-7-5-7-5z"/><circle cx="8" cy="8" r="2"/>',
  'eye-off': '<path d="M1 8s2.5-5 7-5 7 5 7 5-2.5 5-7 5-7-5-7-5z"/><circle cx="8" cy="8" r="2"/><path d="M2 2l12 12"/>',
};
function icon(name) {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('viewBox', '0 0 16 16'); svg.setAttribute('aria-hidden', 'true'); svg.innerHTML = icons[name];
  return svg;
}
function tool(name, label, action, extra = '') {
  const button = document.createElement('button'); button.type = 'button'; button.className = ('tool ' + extra).trim(); button.title = label; button.setAttribute('aria-label', label);
  button.append(icon(name)); button.addEventListener('click', event => { event.stopPropagation(); run(button, () => action(button)); });
  return button;
}
// Borderless window: the header drags it, the outer pixels resize it.
// Windows 11 rounds the corners: the corner zones reach further along the edges.
const border = 5, corner = 14;
function edge(event) {
  const x = event.clientX, y = event.clientY, right = innerWidth - x, bottom = innerHeight - y;
  if (Math.min(x, right, y, bottom) >= border) return '';
  const vertical = y < corner ? 'top' : bottom <= corner ? 'bottom' : '';
  const horizontal = x < corner ? 'left' : right <= corner ? 'right' : '';
  return [vertical, horizontal].filter(Boolean).join('-');
}
const cursors = {top: 'ns-resize', bottom: 'ns-resize', left: 'ew-resize', right: 'ew-resize', 'top-left': 'nwse-resize', 'bottom-right': 'nwse-resize', 'top-right': 'nesw-resize', 'bottom-left': 'nesw-resize'};
document.addEventListener('mousemove', event => { document.documentElement.style.cursor = cursors[edge(event)] || ''; });
document.addEventListener('mousedown', event => {
  if (event.button !== 0 || document.querySelector('dialog[open]')) return;
  const side = edge(event);
  if (side) { event.preventDefault(); event.stopPropagation(); call('window', side).catch(() => {}); return; }
  if (event.target.closest('#titlebar') && !event.target.closest('button')) { event.preventDefault(); call('window', 'drag').catch(() => {}); }
}, true);
$('minimize-window').addEventListener('click', () => call('window', 'minimize').catch(() => {}));
$('close-window').addEventListener('click', () => call('window', 'close').catch(() => {}));
// Tokens: the list refreshes when the first code expires, the rings in between.
let state = {path: '', key: '', codes: []}, loading = null;
const group = code => code.length < 6 ? code : code.slice(0, Math.ceil(code.length / 2)) + ' ' + code.slice(Math.ceil(code.length / 2));
const label = code => code.account ? `${code.issuer} (${code.account})` : code.issuer || code.account;
function lock(message) { $('lock-text').textContent = message; $('lock').hidden = false; $('tokens').hidden = true; }
async function unlock() {
  const button = $('retry'); button.disabled = true;
  try { state = await call('unlock'); $('lock').hidden = true; $('tokens').hidden = false; render(); }
  catch (error) { lock(error.message); }
  finally { button.disabled = false; }
}
async function refresh() {
  if ($('tokens').hidden || loading) return loading;
  loading = call('codes').then(codes => { state.codes = codes; render(); }).catch(error => lock(error.message)).finally(() => { loading = null; });
  return loading;
}
function ring() {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('viewBox', '0 0 22 22'); svg.setAttribute('class', 'ring'); svg.setAttribute('aria-hidden', 'true');
  svg.innerHTML = '<circle class="track" cx="11" cy="11" r="8"/><circle class="left" cx="11" cy="11" r="8" pathLength="100" stroke-dasharray="100 100"/>';
  return svg;
}
// Folders are the "/" separated parts of the issuer but the last one:
// "Azure/fleurent(perso)" is fleurent(perso) in the Azure folder. An issuer
// with at least 2 tokens with an account is a folder of its accounts
// ("Microsoft:a" is a in the Microsoft folder, like "Microsoft/a"). A token
// named like a folder goes inside it. A folder is a card that opens its own
// list (a sub-menu), with a way back on top. See specs/TOKENS.md.
const parts = code => (code.issuer || '').split('/').map(part => part.trim()).filter(Boolean);
function placement(codes) {
  const accounts = new Map();
  for (const code of codes) if (code.account) { const key = parts(code).join('/'); accounts.set(key, (accounts.get(key) || 0) + 1); }
  const place = new Map(), paths = new Set();
  for (const code of codes) {
    const base = parts(code), grouped = base.length && code.account && accounts.get(base.join('/')) >= 2;
    const spot = grouped ? {path: base, name: code.account, account: ''} : {path: base.slice(0, -1), name: base.at(-1) || '', account: code.account};
    place.set(code.id, spot);
    spot.path.forEach((_, depth) => paths.add(spot.path.slice(0, depth + 1).join('/')));
  }
  for (const code of codes) {
    const base = parts(code), spot = place.get(code.id);
    if (spot.path.length < base.length && paths.has(base.join('/'))) spot.path = base;
  }
  return place;
}
let here = [], place = new Map();
const folders = code => place.get(code.id)?.path || [];
const inside = (code, path) => path.every((name, depth) => folders(code)[depth] === name);
function go(path, direction) {
  here = path; $('list').scrollTop = 0; render();
  $('list').classList.remove('slide-in', 'slide-back'); void $('list').offsetWidth;
  $('list').classList.add(direction > 0 ? 'slide-in' : 'slide-back');
}
const up = () => go(here.slice(0, -1), -1);
function folderCard(name, count) {
  const card = document.createElement('article'); card.className = 'card folder'; card.title = [...here, name].join('/');
  const title = document.createElement('div'); title.className = 'label';
  const strong = document.createElement('strong'); strong.textContent = name; title.append(strong);
  const size = document.createElement('div'); size.className = 'count'; size.textContent = t('folder.count', count);
  const side = document.createElement('div'); side.className = 'side';
  const tools = document.createElement('div'); tools.className = 'edit-tools';
  tools.append(tool('tag', t('folder.rename'), () => openRenameFolder([...here, name].join('/'), count)));
  const chevron = document.createElement('span'); chevron.className = 'chevron'; chevron.textContent = '›';
  side.append(tools, chevron);
  card.append(title, size, side);
  card.setAttribute('role', 'button'); card.tabIndex = 0; card.setAttribute('aria-label', name);
  card.addEventListener('click', () => go([...here, name], 1));
  card.addEventListener('keydown', event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); card.click(); } });
  return card;
}
function backRow() {
  const row = document.createElement('button'); row.type = 'button'; row.className = 'back'; row.title = t('folder.back');
  const chevron = document.createElement('span'); chevron.className = 'chevron'; chevron.textContent = '‹';
  const title = document.createElement('strong'); title.textContent = here.join(' / ');
  row.append(chevron, title); row.addEventListener('click', up);
  return row;
}
function render() {
  $('path').textContent = state.path; $('path').title = state.path; $('file').textContent = state.path; $('file').title = state.path;
  const search = $('search').value.trim().toLowerCase();
  place = placement(state.codes);
  // A search lists every match, whatever its folder, under its full name.
  while (here.length && !search && !state.codes.some(code => inside(code, here))) here = here.slice(0, -1);
  const codes = search ? state.codes.filter(code => label(code).toLowerCase().includes(search)) : state.codes.filter(code => inside(code, here));
  const list = $('list'), scroll = list.scrollTop;
  list.replaceChildren();
  if (here.length && !search) list.append(backRow());
  if (!codes.length) {
    const empty = document.createElement('div'); empty.className = 'empty';
    const text = document.createElement('p'); text.textContent = state.codes.length ? t('empty.search') : t('empty.tokens'); empty.append(text);
    if (!state.codes.length) { const button = document.createElement('button'); button.type = 'button'; button.textContent = t('import'); button.addEventListener('click', () => openImport('batch')); empty.append(button); }
    list.append(empty); return;
  }
  // Alphabetical on the name shown, case-insensitive; a folder card counts
  // every token below it.
  const counts = new Map(), entries = [];
  if (!search) for (const code of codes) { const name = folders(code)[here.length]; if (name !== undefined) counts.set(name, (counts.get(name) || 0) + 1); }
  for (const [name, count] of counts) entries.push({key: [name.toLowerCase(), ''], card: folderCard(name, count)});
  for (const code of codes) {
    if (!search && folders(code)[here.length] !== undefined) continue;
    const spot = place.get(code.id);
    const shown = search ? code.issuer : spot.name, sub = search ? code.account : spot.account;
    const card = document.createElement('article'); card.className = code.error ? 'card failed' : 'card'; card.dataset.id = code.id; card.dataset.copied = t('copied');
    const name = document.createElement('div'); name.className = 'label';
    const issuer = document.createElement('strong'); issuer.textContent = shown || sub;
    const account = document.createElement('span'); account.textContent = shown ? sub : '';
    name.title = label(code); name.append(issuer, account);
    const value = document.createElement('div'); value.className = 'code'; value.textContent = code.error || group(code.code);
    if (code.error) value.title = code.error;
    const tools = document.createElement('div'); tools.className = 'edit-tools';
    tools.append(tool('tag', t('rename'), () => openRename(code)), tool('trash', t('delete'), () => remove(code), 'danger'));
    const side = document.createElement('div'); side.style.display = 'flex'; side.style.alignItems = 'center';
    if (!code.error) side.append(ring());
    side.append(tools);
    card.append(name, value, side);
    if (!code.error) card.addEventListener('click', () => { if (!document.body.classList.contains('editing')) run(card, () => copyCode(code, card)); });
    entries.push({key: [(shown || sub).toLowerCase(), shown ? (sub || '').toLowerCase() : ''], card});
  }
  const compare = (a, b) => a < b ? -1 : a > b ? 1 : 0;
  entries.sort((a, b) => compare(a.key[0], b.key[0]) || compare(a.key[1], b.key[1]));
  list.append(...entries.map(entry => entry.card));
  list.scrollTop = scroll; tick();
}
async function copyCode(code, card) {
  // The code shown may have just expired: ask Go for the current one.
  const current = (await call('codes')).find(item => item.id === code.id) || code;
  await copy(current.code);
  card.classList.add('copied'); setTimeout(() => card.classList.remove('copied'), 1200);
  notice(t('copied.code', label(current)));
}
function tick() {
  const now = Date.now();
  let expired = false;
  for (const code of state.codes) {
    if (code.error) continue;
    const left = Math.max(0, code.expires - now) / 1000;
    if (left <= 0) expired = true;
    const card = $('list').querySelector(`.card[data-id="${code.id}"]`);
    if (!card) continue;
    card.classList.toggle('ending', left <= 5);
    card.querySelector('.ring .left')?.setAttribute('stroke-dasharray', `${(left / code.period) * 100} 100`);
    card.title = t('expires', Math.ceil(left));
  }
  if (expired) refresh();
}
setInterval(tick, 250);
window.addEventListener('focus', refresh);
async function remove(code) { if (!await confirmed(t('delete.confirm', label(code)))) return; await call('delete', code.id); await refresh(); notice(t('deleted', label(code))); }
// The rename dialog serves a token (issuer, account) or a folder (its path,
// applied to every token inside).
let renaming = null;
function openRenameDialog(target, title, issuerLabel, help) {
  renaming = target; $('rename-title').textContent = title; $('rename-issuer-label').textContent = issuerLabel; $('rename-help').textContent = help;
  $('rename-account-label').hidden = Boolean(target.folder);
  $('rename').showModal(); $('rename-issuer').focus(); $('rename-issuer').select();
}
function openRename(code) { $('rename-issuer').value = code.issuer; $('rename-account').value = code.account; openRenameDialog(code, t('rename'), t('rename.issuer'), t('rename.help')); }
function openRenameFolder(path, count) { $('rename-issuer').value = path; openRenameDialog({folder: path}, t('folder.rename'), t('folder.name'), t('folder.rename.help', count)); }
$('rename-form').addEventListener('submit', event => { event.preventDefault(); run(event.submitter, async () => {
  if (renaming.folder) {
    const to = $('rename-issuer').value.split('/').map(part => part.trim()).filter(Boolean);
    const moved = await call('rename_folder', renaming.folder, to.join('/'));
    if (here.join('/') === renaming.folder || here.join('/').startsWith(renaming.folder + '/')) here = to;
    $('rename').close(); await refresh(); notice(t('folder.renamed', moved, to.join('/') || '/'));
    return;
  }
  await call('rename', renaming.id, $('rename-issuer').value, $('rename-account').value); $('rename').close(); await refresh(); notice(t('renamed'));
}); });
for (const id of ['close-rename', 'cancel-rename']) $(id).addEventListener('click', () => $('rename').close());
// Add dialog (+): one entry, the batch import box, the export (masked until
// shown). The export is fetched when its tab opens and dropped on close.
let tab = 'add';
function showTab(name) {
  tab = name;
  for (const button of document.querySelectorAll('#import [data-tab]')) { button.classList.toggle('on', button.dataset.tab === name); button.setAttribute('aria-pressed', button.dataset.tab === name); }
  for (const pane of document.querySelectorAll('#import [data-pane]')) pane.hidden = pane.dataset.pane !== name;
  if (name !== 'export') { $('export-text').value = ''; masked(true); }
  if (name === 'export') run($('export-text'), async () => { $('export-text').value = await call('export'); });
  ({add: $('add-issuer'), batch: $('import-text'), export: $('export-text')})[name].focus();
}
function openImport(name = 'add') { if (!$('import').open) $('import').showModal(); showTab(name); }
for (const button of document.querySelectorAll('#import [data-tab]')) button.addEventListener('click', () => showTab(button.dataset.tab));
$('import-button').append(icon('plus'));
$('import-button').addEventListener('click', () => openImport('add'));
$('close-import').addEventListener('click', () => $('import').close());
for (const button of document.querySelectorAll('.cancel-import')) button.addEventListener('click', () => $('import').close());
$('import').addEventListener('close', () => { $('export-text').value = ''; masked(true); });
$('add-form').addEventListener('submit', event => { event.preventDefault(); run(event.submitter, async () => {
  const issuer = $('add-issuer').value, account = $('add-account').value;
  await call('add', issuer, account, $('add-secret').value, $('add-algorithm').value, Number($('add-digits').value), Number($('add-period').value));
  $('add-form').reset(); $('import').close(); await refresh();
  notice(t('added', account ? `${issuer.trim()} (${account.trim()})` : issuer.trim()));
}); });
let reveal;
function masked(on) {
  $('export-text').classList.toggle('masked', on);
  if (!reveal) return;
  reveal.replaceChildren(icon(on ? 'eye' : 'eye-off')); reveal.title = on ? t('show') : t('hide'); reveal.setAttribute('aria-label', reveal.title); reveal.classList.toggle('on', !on);
}
const copyExport = async () => { await copy($('export-text').value); notice(t('copied')); };
$('export-tools').append(reveal = tool('eye', t('show'), () => masked(!$('export-text').classList.contains('masked'))), tool('copy', t('copy'), copyExport));
// A click on the masked text copies it, like a masked field.
$('export-text').addEventListener('click', () => { if ($('export-text').classList.contains('masked') && $('export-text').value) run($('export-text'), copyExport); });
masked(true);
$('import-form').addEventListener('submit', event => { event.preventDefault(); run(event.submitter, async () => {
  const result = await call('import', $('import-text').value);
  await refresh();
  notice(t('import.result', result.added, result.duplicates), result.added === 0 && result.skipped.length > 0);
  for (const skipped of result.skipped) notice(t('import.skipped', skipped.line.length > 40 ? skipped.line.slice(0, 40) + '…' : skipped.line, skipped.reason), true);
  $('import-text').value = result.skipped.map(skipped => skipped.line).join('\n');
  if (!result.skipped.length) $('import').close();
}); });
$('edit-button').append(icon('pencil'));
$('edit-button').addEventListener('click', () => { const on = document.body.classList.toggle('editing'); $('edit-button').classList.toggle('on', on); });
// "/" searches. Escape clears the search, then goes up a folder, then leaves
// the edit mode, then closes the window to the tray; Backspace and Alt+Left go
// up a folder.
document.addEventListener('keydown', event => {
  if (document.querySelector('dialog[open]')) return;
  if ((event.key === '/' && !editable(event.target)) || (event.ctrlKey && event.key.toLowerCase() === 'f')) { event.preventDefault(); $('search').focus(); $('search').select(); }
  if (here.length && !$('search').value && ((event.key === 'Backspace' && !editable(event.target)) || (event.altKey && event.key === 'ArrowLeft'))) { event.preventDefault(); up(); }
  if (event.key === 'Escape') {
    if ($('search').value) { $('search').value = ''; render(); }
    else if (here.length) up();
    else if (document.body.classList.contains('editing')) $('edit-button').click();
    else call('window', 'close').catch(() => {});
  }
});
$('search').addEventListener('input', render);
$('retry').addEventListener('click', unlock);
// Settings: the agent key sealing the secrets, the updates.
async function loadKeys() {
  const keys = await call('agent_keys').catch(() => []);
  const options = [new Option(t('tokens.key.first'), '')];
  for (const key of keys) options.push(new Option(key.comment ? `${key.comment} (${key.fingerprint.slice(7, 15)}…)` : key.fingerprint, key.fingerprint));
  if (state.key && !keys.some(key => key.fingerprint === state.key)) options.push(new Option(state.key, state.key));
  for (const option of options) option.selected = option.value === state.key;
  $('key').replaceChildren(...options); $('key').disabled = !keys.length;
}
$('key').addEventListener('change', () => run($('key'), async () => {
  const count = await call('set_key', $('key').value);
  state.key = $('key').value; notice(t('tokens.resealed', count));
}));
call('version').then(version => { $('version').textContent = version; $('current-version').textContent = version; }).catch(() => {});
// Settings dialog (wrench): language, update channel and manual check. The
// channel list comes from manifest.json; the automatic check stays silent.
$('settings-button').append(icon('wrench'));
$('settings-button').addEventListener('click', () => { $('settings').showModal(); loadKeys(); });
$('close-settings').addEventListener('click', () => $('settings').close());
// A click on the grayed backdrop closes the settings, like Escape.
$('settings').addEventListener('click', event => { if (event.target === $('settings')) $('settings').close(); });
let offered = '';
function updates(release) {
  $('update-enable').checked = release.enabled;
  $('update-disabled').hidden = release.enabled; $('update-fields').hidden = !release.enabled;
  $('update-error').hidden = !release.error; $('update-error').textContent = release.error || '';
  const channels = release.channels || [];
  $('channel').replaceChildren(...channels.map(name => new Option(name, name, false, name === release.channel)));
  $('channel').disabled = !channels.length;
  $('current-version').textContent = release.current;
  $('latest-version').textContent = release.latest || '—';
  $('upstream').textContent = release.upstream.replace(/^https:\/\//, ''); $('upstream').title = release.upstream;
  $('update-dev').hidden = !release.development;
  $('open-release').hidden = !(release.development && release.url);
  if (release.latest) $('open-release').textContent = t('update.download', release.latest);
  if (!release.available || offered === release.latest) return;
  offered = release.latest;
  notice(t('update.available', release.latest, release.current), false, {label: t('update.install'), run: async () => {
    notice(t('update.downloading', release.latest));
    await call('update_install', release.latest);
  }});
}
call('update_check').then(updates).catch(() => {}); // silent: errors stay in the panel
$('check-update').addEventListener('click', () => run($('check-update'), async () => {
  offered = '';
  const release = await call('update_check', true);
  updates(release);
  if (release.error) throw new Error(release.error);
  if (release.development && release.latest) notice(t('update.dev.notice', release.latest));
  else if (release.enabled && !release.available) notice(t('update.uptodate', release.current, release.channel, release.latest || t('update.none')));
}));
$('open-release').addEventListener('click', () => run($('open-release'), () => call('update_open')));
$('update-enable').addEventListener('change', () => run($('update-enable'), async () => {
  offered = '';
  const release = await call('update_enable', $('update-enable').checked);
  updates(release);
  if (release.error) throw new Error(release.error);
}));
$('channel').addEventListener('change', () => run($('channel'), async () => {
  const release = await call('update_channel', $('channel').value);
  updates(release);
  if (!release.available) notice(t('update.channel.selected', release.channel, release.latest || t('update.none')));
}));
// The language applies at once: Go renders the page again.
const languageNames = {'fr-fr': 'Français', 'en-us': 'English'};
$('language').replaceChildren(...window.I18N.languages.map(name => new Option(languageNames[name] || name, name, false, name === window.I18N.language)));
$('language').addEventListener('change', () => run($('language'), () => call('set_language', $('language').value)));
unlock();
})();
