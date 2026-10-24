// Builders for the shared ZZZ-style UI (style/ui/zzz_ui.css).

const SVG_NS = "http://www.w3.org/2000/svg";
// Each glyph is a list of black strokes on a coloured disc. The check's tail
// runs on to the edge of its disc, which clips it. A path with `solid` is
// filled as well.
const GLYPHS = {
  confirm: { size: [32, 28], box: "0 0 22 20", paths: [{ d: "M2.5 10.5L8.5 16.5L22.15 0.35" }] },
  cancel: { size: [30, 30], box: "0 0 20 20", paths: [{ d: "M3.5 3.5L16.5 16.5M16.5 3.5L3.5 16.5" }] },
  // Deck menu: save drops into a tray, load lifts out of it, new game adds, eject lifts the tape.
  save: { size: [30, 30], box: "0 0 20 20", paths: [{ d: "M10 3V12M5.5 8L10 12.5L14.5 8M4 16.5H16" }] },
  load: { size: [30, 30], box: "0 0 20 20", paths: [{ d: "M10 13V4M5.5 8.5L10 4L14.5 8.5M4 16.5H16" }] },
  new: { size: [30, 30], box: "0 0 20 20", paths: [{ d: "M10 3.5V16.5M3.5 10H16.5" }] },
  eject: { size: [30, 30], box: "0 0 20 20", paths: [{ d: "M10 3.5L16.5 12H3.5Z", solid: true, width: 2.5 }, { d: "M3.5 16.5H16.5", width: 3.5 }] },
  // Companion settings: put the defaults back.
  reset: { size: [30, 30], box: "0 0 20 20", paths: [{ d: "M4 9a7 7 0 1 1 1 7M4 3v6h6" }] },
};

let glyphCount = 0;

// The disc is a coloured circle with the icon cut out of it (a mask), so whatever
// is behind the cap shows through. Strokes are black in the mask, which hides them.
function glyph(doc, kind) {
  const { size, box, paths } = GLYPHS[kind];
  const id = `zzz-glyph-${++glyphCount}`;
  const make = (tag, attrs) => {
    const node = doc.createElementNS(SVG_NS, tag);
    for (const [name, value] of Object.entries(attrs)) node.setAttribute(name, String(value));
    return node;
  };
  const icon = make("svg", { x: (38 - size[0]) / 2, y: (38 - size[1]) / 2, width: size[0], height: size[1], viewBox: box, overflow: "visible" });
  for (const { d, solid = false, width = 4.5 } of paths) {
    icon.appendChild(make("path", { d, stroke: "#000", "stroke-width": width, "stroke-linecap": "round", "stroke-linejoin": "round", fill: solid ? "#000" : "none" }));
  }
  const mask = make("mask", { id });
  mask.append(make("rect", { width: 38, height: 38, fill: "#fff" }), icon);
  const defs = make("defs", {});
  defs.appendChild(mask);
  const svg = make("svg", { class: "zzz-pill-disc", width: 38, height: 38, viewBox: "0 0 38 38", "aria-hidden": "true" });
  svg.append(defs, make("circle", { cx: 19, cy: 19, r: 19, fill: "currentColor", mask: `url(#${id})` }));
  return svg;
}

// Press and release for a .zzz-sel element. The CSS gives hover, selected and
// pressed (:active); on release the filled shape blinks on and off, like the game.
// Enter and Space press it too. Activation stays with the caller.
export function makeSelectable(el) {
  el.classList.add("zzz-sel");
  const blink = () => {
    if (el.classList.contains("is-disabled")) return;
    el.classList.remove("is-flash");
    void el.offsetWidth;
    el.classList.add("is-flash");
    const done = event => {
      if (event.animationName !== "zzz-flash") return;
      el.classList.remove("is-flash");
      el.removeEventListener?.("animationend", done);
    };
    el.addEventListener("animationend", done);
  };
  el.addEventListener("pointerup", blink);
  el.addEventListener("keydown", event => { if (!event.repeat && (event.key === "Enter" || event.key === " ")) el.classList.add("is-pressed"); });
  el.addEventListener("keyup", event => {
    if (event.key !== "Enter" && event.key !== " ") return;
    el.classList.remove("is-pressed");
    blink();
  });
  return el;
}

// kind: "confirm", "cancel", "save", "load", "new", "eject" or "reset". Activates on click, Enter or Space.
export function createPill(doc, kind, label, onActivate) {
  const pill = doc.createElement("div");
  pill.className = `zzz-pill is-${kind}`;
  pill.setAttribute("role", "button");
  pill.tabIndex = 0;
  const cap = doc.createElement("span");
  cap.className = "zzz-pill-cap";
  cap.appendChild(glyph(doc, kind));
  const text = doc.createElement("span");
  text.className = "zzz-pill-label";
  text.textContent = label;
  pill.append(cap, text);
  pill.addEventListener("click", onActivate);
  pill.addEventListener("keydown", event => {
    if (event.key === "Enter" || event.key === " ") { event.preventDefault(); onActivate(); }
  });
  return makeSelectable(pill);
}

// Round button drawings, in a 38 x 38 box, measured from the game's help button: a thick
// ring with a tapered bar hanging from its top (merged into the ring) and a leaning
// parallelogram dot below. Both shapes lean the same way. Drawn in the button's text
// colour, so pressed (black on the fill) needs no second drawing.
// The mark swings about the ring's centre (CSS), so the bar stays hung from the ring.
const ROUND = {
  help: {
    ring: { r: 16.5, width: 5 },
    shapes: [
      "16.8,3.5 24.4,3.5 21.6,16.3 16.5,16.3", // the bar: wide at the top, narrowing as it leans
      "16.4,21.4 21.6,21.4 19.8,26.2 14.4,26.2", // the dot
    ],
  },
};

// A round button: the pill's cap on its own (the in-game help button). A white ring
// with a tapered bar hung from its top and a leaning dot, which swing both ways; pressed, the whole button fills and the
// drawing turns black. kind: a key of ROUND, e.g. "help". Activates on click, Enter or Space.
export function createRoundButton(doc, kind, onActivate) {
  const { ring, shapes } = ROUND[kind];
  const make = (tag, attrs) => {
    const node = doc.createElementNS(SVG_NS, tag);
    for (const [name, value] of Object.entries(attrs)) node.setAttribute(name, String(value));
    return node;
  };
  const disc = make("svg", { class: "zzz-round-disc", width: 38, height: 38, viewBox: "0 0 38 38", "aria-hidden": "true" });
  disc.appendChild(make("circle", { cx: 19, cy: 19, r: ring.r, fill: "none", stroke: "currentColor", "stroke-width": ring.width }));
  const mark = make("g", { class: "zzz-round-mark" });
  for (const points of shapes) mark.appendChild(make("polygon", { points, fill: "currentColor" }));
  disc.appendChild(mark);

  const button = doc.createElement("div");
  button.className = "zzz-round";
  button.setAttribute("role", "button");
  button.tabIndex = 0;
  button.appendChild(disc);
  button.addEventListener("click", onActivate);
  button.addEventListener("keydown", event => {
    if (event.key === "Enter" || event.key === " ") { event.preventDefault(); onActivate(); }
  });
  return makeSelectable(button);
}

// Motion (the zzz-in / zzz-out / zzz-swap block in style/ui/zzz_ui.css). Each one finishes at once when no
// animation actually runs (reduced motion turned off in the CSS, a host without animations, a hidden page), so
// callers never wait on an animation that is not there.
const animates = el => typeof el.getAnimations === "function" && Boolean(el.dataset);
function motion(el, kind, done) {
  el.dataset.zzzMotion = kind;
  const name = `zzz-${kind}`;
  if (!el.getAnimations().some(animation => animation.animationName === name)) { done(); return; }
  let finished = false;
  const finish = () => {
    if (finished) return;
    finished = true;
    el.removeEventListener("animationend", end);
    el.removeEventListener("animationcancel", end);
    clearTimeout(timer);
    done();
  };
  const end = event => { if (event.target === el && event.animationName === name) finish(); };
  el.addEventListener("animationend", end);
  el.addEventListener("animationcancel", end);
  // In case the animation never reports back (the page is hidden).
  const timer = setTimeout(finish, 1500);
}

// Shows an element with the in transition. Showing one that is still leaving turns it round.
export function enter(el) {
  if (!animates(el)) { el.hidden = false; return; }
  if (!el.hidden && el.dataset.zzzMotion !== "out") return;
  el.hidden = false;
  motion(el, "in", () => { if (el.dataset.zzzMotion === "in") delete el.dataset.zzzMotion; });
}

// Hides an element after the out transition. done runs once it is hidden, and not at all if it is shown again
// before then.
export function leave(el, done) {
  if (el.hidden || !animates(el)) { el.hidden = true; done?.(); return; }
  motion(el, "out", () => {
    if (el.dataset.zzzMotion !== "out") return;
    el.hidden = true;
    delete el.dataset.zzzMotion;
    done?.();
  });
}

// Replays the switch transition on content that just changed: direction 1 slides in from the right, -1 from
// the left.
export function swap(el, direction = 1) {
  if (!el?.classList) return;
  el.style?.setProperty?.("--zzz-swap-x", `calc(${10 * direction} * var(--zzz-u))`);
  el.classList.remove("zzz-swap-in");
  void el.offsetWidth;
  el.classList.add("zzz-swap-in");
}

// Makes a word an attention mark (.zzz-hop in style/ui/zzz_ui.css): one span per letter with its place in the
// push. The thin "!" widens twice as much.
export function markLetters(el) {
  const doc = el.ownerDocument;
  el.classList.add("zzz-hop");
  el.replaceChildren(...[...el.textContent].map((letter, index) => {
    const span = doc.createElement("span");
    span.textContent = letter;
    span.style.setProperty("--i", index);
    span.style.setProperty("--k", (0.7 * 1.3 ** index).toFixed(3));
    if (letter === "!") span.style.setProperty("--w", 2);
    return span;
  }));
  return el;
}

let open = null;

// Asks the player to confirm something. Resolves true on Confirm, false on
// Cancel, Escape or a click outside the band. Only one is open at a time; a
// second request cancels the first. Focus starts on Cancel, so the keyboard
// never confirms by accident.
export function confirmDialog({ title, message = "", confirmLabel = "Confirm", cancelLabel = "Cancel", doc = document } = {}) {
  open?.close(false);
  return new Promise(resolve => {
    const previous = doc.activeElement;
    const backdrop = doc.createElement("div");
    backdrop.className = "zzz-modal";
    const band = doc.createElement("div");
    band.className = "zzz-modal-band";
    band.setAttribute("role", "alertdialog");
    band.setAttribute("aria-modal", "true");
    band.setAttribute("aria-label", title);
    const heading = doc.createElement("h2");
    heading.className = "zzz-modal-title";
    heading.textContent = title;
    const actions = doc.createElement("div");
    actions.className = "zzz-modal-actions";
    const close = answer => {
      if (open?.backdrop !== backdrop) return;
      open = null;
      doc.removeEventListener("keydown", onKey, true);
      backdrop.inert = true;
      leave(band);
      leave(backdrop, () => backdrop.remove());
      try { previous?.focus?.(); } catch { /* element is gone */ }
      resolve(answer);
    };
    const cancel = createPill(doc, "cancel", cancelLabel, () => close(false));
    const confirm = createPill(doc, "confirm", confirmLabel, () => close(true));
    actions.append(cancel, confirm);
    band.appendChild(heading);
    if (message) {
      const text = doc.createElement("p");
      text.className = "zzz-modal-text";
      text.textContent = message;
      band.appendChild(text);
    }
    band.appendChild(actions);
    backdrop.appendChild(band);
    backdrop.addEventListener("pointerdown", event => { if (event.target === backdrop) close(false); });
    function onKey(event) {
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); close(false); }
      else if (event.key === "ArrowLeft" || event.key === "ArrowRight") { event.preventDefault(); (doc.activeElement === cancel ? confirm : cancel).focus(); }
      else if (event.key === "Tab") { event.preventDefault(); (doc.activeElement === cancel ? confirm : cancel).focus(); }
    }
    doc.addEventListener("keydown", onKey, true);
    open = { backdrop, close };
    backdrop.hidden = band.hidden = true;
    (doc.body || doc.documentElement).appendChild(backdrop);
    enter(backdrop);
    enter(band);
    cancel.focus();
  });
}

// Message bar medals (style/ui/zzz_ui.css, .zzz-message): dark glyphs in a 24 x 24 box.
const MESSAGE_GLYPHS = {
  error: [["path", { d: "M6 6l12 12M18 6L6 18", stroke: "currentColor", "stroke-width": 4, "stroke-linecap": "round" }]],
  caution: [["path", { d: "M12 3.5v10.5", stroke: "currentColor", "stroke-width": 4.2, "stroke-linecap": "round" }], ["circle", { cx: 12, cy: 19.5, r: 2.4, fill: "currentColor" }]],
  info: [["circle", { cx: 12, cy: 4.6, r: 2.4, fill: "currentColor" }], ["path", { d: "M12 10v10.5", stroke: "currentColor", "stroke-width": 4.2, "stroke-linecap": "round" }]],
};

// A message bar. kind: "error", "caution" or "info". headline is optional and shown first in
// the kind's colour; text is the message. Returns the element; place and remove it yourself.
export function createMessage(doc, kind, { headline = "", text = "" } = {}) {
  const shapes = MESSAGE_GLYPHS[kind] || MESSAGE_GLYPHS.info;
  const bar = doc.createElement("div");
  bar.className = `zzz-message is-${MESSAGE_GLYPHS[kind] ? kind : "info"}`;
  bar.setAttribute("role", kind === "error" ? "alert" : "status");
  if (headline) {
    const head = doc.createElement("em");
    head.className = "zzz-message-head";
    head.textContent = headline;
    bar.appendChild(head);
  }
  if (text) {
    const body = doc.createElement("span");
    body.className = "zzz-message-text";
    body.textContent = text;
    bar.appendChild(body);
  }
  const medal = doc.createElement("span");
  medal.className = "zzz-message-medal";
  const svg = doc.createElementNS(SVG_NS, "svg");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("aria-hidden", "true");
  for (const [tag, attrs] of shapes) {
    const shape = doc.createElementNS(SVG_NS, tag);
    for (const [name, value] of Object.entries(attrs)) shape.setAttribute(name, String(value));
    svg.appendChild(shape);
  }
  medal.appendChild(svg);
  bar.appendChild(medal);
  return bar;
}

// A short notification bar. Replaces the previous one in the same host; it slides in and out.
export function showToast(host, text, { ms = 1600, className = "" } = {}) {
  host.__zzzToast?.remove();
  const node = host.ownerDocument.createElement("div");
  node.className = className ? `zzz-toast ${className}` : "zzz-toast";
  node.setAttribute("role", "status");
  node.textContent = text;
  node.hidden = true;
  host.appendChild(node);
  host.__zzzToast = node;
  enter(node);
  const dismiss = () => { clearTimeout(timer); if (host.__zzzToast === node) host.__zzzToast = null; leave(node, () => node.remove()); };
  const timer = setTimeout(dismiss, ms);
  return dismiss;
}
