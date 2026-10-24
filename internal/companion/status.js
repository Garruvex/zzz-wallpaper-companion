
// Languages. /ui/api.js starts with window.companionI18n = { lang, table }, the table mapping English
// text to the current language. Pages are written in English and translate themselves: any text,
// placeholder, title or aria-label that is exactly an English string in the table is swapped, now and as
// pages change. Text with values in it goes through t('… {name} …', { name }). Anything inside
// translate="no" (game names, your dates, holiday names) is never touched.
(() => {
  let { lang = 'en', table = {} } = window.companionI18n || {};
  window.companionLang = lang;
  const rendered = new Map();
  const originals = new WeakMap();
  const originalAttributes = new WeakMap();
  window.t = (text, values) => {
    const out = table[text] ?? text;
    const result = values ? out.replace(/\{(\w+)\}/g, (match, key) => key in values ? values[key] : match) : out;
    if (values) rendered.set(result, { text, values });
    return result;
  };
  document.documentElement.lang = lang;

  const ATTRIBUTES = ['placeholder', 'title', 'aria-label'];
  const kept = node => node.parentElement?.closest('[translate="no"], script, style');
  const text = node => {
    const raw = node.nodeValue;
    if (kept(node)) return;
    const prior = originals.get(node);
    const key = prior && prior.last === raw ? prior.key : raw.trim();
    const template = rendered.get(key);
    const next = template ? window.t(template.text, template.values) : (table[key] ?? key);
    const value = raw.replace(raw.trim(), next);
    originals.set(node, { key, last: value });
    // Only a real change is written: writing the same text again would wake the observer forever.
    if (key && value !== raw) node.nodeValue = value;
  };
  const attributes = element => {
    if (element.closest('[translate="no"]')) return;
    let saved = originalAttributes.get(element);
    if (!saved) originalAttributes.set(element, saved = {});
    for (const name of ATTRIBUTES) {
      const value = element.getAttribute(name);
      if (!value) continue;
      const prior = saved[name];
      const key = prior && prior.last === value ? prior.key : value.trim();
      const template = rendered.get(key);
      const next = template ? window.t(template.text, template.values) : (table[key] ?? key);
      saved[name] = { key, last: next };
      if (next !== value) element.setAttribute(name, next);
    }
  };
  const visit = root => {
    if (root.nodeType === Node.TEXT_NODE) { text(root); return; }
    if (root.nodeType !== Node.ELEMENT_NODE) return;
    attributes(root);
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT | NodeFilter.SHOW_ELEMENT);
    for (let node = walker.nextNode(); node; node = walker.nextNode()) node.nodeType === Node.TEXT_NODE ? text(node) : attributes(node);
  };
  let originalTitle = document.title;
  let lastTitle = originalTitle;
  const titled = () => {
    if (document.title !== lastTitle) originalTitle = document.title;
    const next = originalTitle.split(' · ').map(part => table[part] ?? part).join(' · ');
    lastTitle = next;
    if (next !== document.title) document.title = next;
  };
  window.companionApplyLanguage = data => {
    lang = data.current; table = data.table || {};
    window.companionLang = lang;
    window.companionI18n = { lang, table };
    document.documentElement.lang = lang;
    if (document.body) visit(document.body);
    titled();
  };
  new MutationObserver(records => {
    for (const record of records) {
      if (record.type === 'childList') record.addedNodes.forEach(visit);
      else if (record.type === 'characterData') text(record.target);
      else attributes(record.target);
    }
    titled();
  }).observe(document.documentElement, { childList: true, subtree: true, characterData: true, attributes: true, attributeFilter: ATTRIBUTES });
  if (document.body) visit(document.body);
  titled();
})();

// Shows a page's status line as a message bar from the shared UI kit.
//   companionSay(text, kind)   kind: "info" (default), "caution" or "error"; empty text clears it.
// The bar goes inside the page's #status element. Without the kit (the port-conflict recovery page
// serves neither the kit nor the theme) it falls back to plain text.
(() => {
  let latest = 0;
  const kit = import('/ui/zzz/zzz_ui.js').catch(() => null);
  window.companionSay = async (text, kind = 'info') => {
    const mine = ++latest;
    const box = document.getElementById('status');
    if (!box) return;
    if (!text) { box.replaceChildren(); return; }
    const module = await kit;
    if (mine !== latest) return;
    if (module) box.replaceChildren(module.createMessage(document, kind, { text }));
    else box.textContent = text;
  };
})();

// Panels are built like the game's: a title card at the top (big name, grey caps subtitle),
// then item cards, each a black title bar (name, with tools such as tags on the right) over a body.
window.companionTitleCard = (doc, title, subtitle = '') => {
  const card = doc.createElement('div');
  card.className = 'zzz-title-card';
  const name = doc.createElement('b');
  name.textContent = title;
  const sub = doc.createElement('small');
  sub.textContent = subtitle;
  card.append(name, sub);
  return card;
};
window.companionCard = (doc, title, { tag = 'div', tools = [], body = [] } = {}) => {
  const card = doc.createElement(tag);
  card.className = 'zzz-item';
  const bar = doc.createElement('div');
  bar.className = 'zzz-item-bar';
  const name = doc.createElement('span');
  name.className = 'zzz-item-title';
  name.append(title);
  bar.append(name, ...tools.filter(Boolean));
  const inner = doc.createElement('div');
  inner.className = 'zzz-item-body';
  inner.append(...body.filter(Boolean));
  card.append(bar, inner);
  return card;
};

// Asks with the shared UI kit's Confirm / Cancel dialog; resolves true or false. options: title, message,
// confirmLabel, cancelLabel. Without the kit (the recovery page) it falls back to the browser's confirm().
window.companionConfirm = async options => {
  const module = await import('/ui/zzz/zzz_ui.js').catch(() => null);
  return module ? module.confirmDialog(options) : window.confirm([options.title, options.message].filter(Boolean).join('\n\n'));
};

// Saves and applies the language without reloading the page.
window.companionSaveLanguage = async language => {
  const response = await fetch('/ui/language', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ language }) });
  const body = await response.json();
  if (!response.ok) throw Error(body.error || t('Could not save the language.'));
  window.companionApplyLanguage(body);
  return body;
};

// A game's name in the Library and on its tape. An empty name goes back to the game file's name.
window.companionRenameGame = async (id, title) => {
  const response = await fetch('/games/rename', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ id, title }) });
  const body = await response.json();
  if (!response.ok) throw Error(body.error || 'Could not rename the game.');
  return body;
};

// The Storage section: what the cache holds, and emptying it (removed games with their saves, map tiles).
window.companionCacheUse = async () => (await fetch('/games/cache', { cache: 'no-store' })).json();
window.companionClearCache = async () => {
  const response = await fetch('/games/cache/clear', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ confirm: true }) });
  const body = await response.json();
  if (!response.ok) throw Error(body.error || t('Could not clean up storage.'));
  return body;
};

// The pad as the game station sees it, for the Controller section: { connected, buttons, axes }.
window.companionPadState = async signal => (await fetch('/controller/state', { cache: 'no-store', signal })).json();

// The keyboard controls for the Keyboard section: { keys, defaults }, each button name to a Windows
// virtual-key code (0 for none). Saving null restores the defaults.
window.companionKeys = async () => (await fetch('/controller/keys', { cache: 'no-store' })).json();
window.companionSaveKeys = async keys => {
  const response = await fetch('/controller/keys', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ keys }) });
  const body = await response.json();
  if (!response.ok) throw Error(body.error || 'Could not save the keys.');
  return body;
};
