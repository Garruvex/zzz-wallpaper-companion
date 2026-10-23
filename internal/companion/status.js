
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
