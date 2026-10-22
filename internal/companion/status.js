
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
