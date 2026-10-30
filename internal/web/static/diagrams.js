// PlantUML diagrams on the Diagrams page: inline SVG with zoom/pan, a source editor with live preview,
// saves pushed to every open tab over SSE. A classic script on purpose: it must work even if the Mermaid module fails.
(function () {
  // PlantUML diagrams: inline SVG with zoom/pan; click opens the source editor; saves from others arrive over SSE.
  const me = Math.random().toString(36).slice(2, 10);
  function mount(box, svgText) {
    if (svgText !== undefined) box.innerHTML = svgText;
    const svg = box.querySelector("svg");
    if (!svg) return;
    const vb = svg.viewBox.baseVal, w = box.clientWidth;
    // PlantUML emits preserveAspectRatio="none": with our width/height that scales x and y differently (stretched text)
    svg.setAttribute("preserveAspectRatio", "xMidYMid meet");
    svg.removeAttribute("width"); svg.removeAttribute("height");
    svg.style.width = "100%";
    svg.style.height = Math.max(240, Math.min(900, (vb && vb.width ? vb.height * Math.min(1.2, w / vb.width) : 400) + 30)) + "px";
    try { const pz = window.svgPanZoom(svg, { zoomEnabled: true, controlIconsEnabled: true, dblClickZoomEnabled: false, fit: true, center: true, minZoom: 0.3, maxZoom: 8 });
      pz.resize(); pz.fit(); pz.center(); } catch (e) { console.warn("pan-zoom", e); }
  }
  async function refresh(id) {
    const box = document.querySelector(`.pz[data-diagram="${id}"]`);
    const r = await fetch(`/api/diagrams/${id}/svg`);
    if (r.ok) mount(box, await r.text());
  }
  document.querySelectorAll(".pz").forEach(box => { mount(box); refresh(box.dataset.diagram); });

  const ed = document.getElementById("pz-editor"), pv = ed.querySelector(".pz-preview"), msg = ed.querySelector(".pz-msg");

  // PlantUML highlighting for CodeMirror 5 (simple mode).
  CodeMirror.defineSimpleMode("plantuml", {
    start: [
      { regex: /\s*'.*/, token: "comment", sol: true },
      { regex: /@(startuml|enduml)\b/, token: "meta" },
      { regex: /!include\b.*/, token: "meta" },
      { regex: /"(?:[^\\"]|\\.)*"/, token: "string" },
      { regex: /\*\*[^*]+\*\*/, token: "strong" },
      { regex: /#[0-9a-fA-F]{3,6}\b/, token: "number" },
      { regex: /\b(participant|actor|database|boundary|control|entity|queue|collections|as|autonumber|skinparam|hide|show|state)\b/, token: "def" },
      { regex: /\b(group|alt|else|end|loop|opt|par|break|critical|note|over|of|right|left|top|bottom|ref)\b/, token: "keyword" },
      { regex: /\[\*\]/, token: "atom" },
      { regex: /<?-+(?:>>?|x|o)?|<?\.+>?|-->>|->x/, token: "operator" },
      { regex: /:(?=\s)/, token: "operator", next: "label" },
    ],
    label: [{ regex: /.*/, token: "string-2", next: "start" }],
  });
  const cm = CodeMirror(ed.querySelector(".pz-code"), { mode: "plantuml", lineNumbers: true, lineWrapping: true, styleActiveLine: true, tabSize: 2 });
  let skinShift = 0;
  const shift = () => (/^!include skin\.iuml/m.test(cm.getValue()) ? skinShift : 0);

  // Click a diagram element → cursor jumps to its source line (PlantUML marks elements with data-source-line).
  pv.addEventListener("click", e => {
    const el = e.target.closest("[data-source-line]");
    let line = el ? +el.getAttribute("data-source-line") + 1 - shift() : 0; // 1-based editor line
    if (!el) {
      // Implicitly declared elements (state boxes) carry no source line: jump to the first line naming the clicked text.
      // A click on a box's <rect> has no text: take the label drawn inside that box.
      let word = e.target.tagName === "text" ? e.target.textContent.trim() : "";
      if (!word) {
        const b = e.target.getBoundingClientRect();
        const t = [...pv.querySelectorAll("svg text")].find(t => { const r = t.getBoundingClientRect();
          return r.left >= b.left && r.right <= b.right && r.top >= b.top && r.bottom <= b.bottom; });
        word = t ? t.textContent.trim() : "";
      }
      if (!word) return;
      const re = new RegExp(`(^|[^\\w])${word.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}([^\\w]|$)`);
      for (let i = 0; i < cm.lineCount(); i++) if (re.test(cm.getLine(i))) { line = i + 1; break; }
    }
    if (line < 1) return;
    cm.focus(); cm.setCursor({ line: line - 1, ch: 0 });
    cm.scrollIntoView({ line: line - 1, ch: 0 }, 120);
    const h = cm.addLineClass(line - 1, "background", "pz-flash");
    setTimeout(() => cm.removeLineClass(h, "background", "pz-flash"), 1200);
  });
  // Cursor on a line → outline the diagram elements drawn from it.
  cm.on("cursorActivity", () => {
    pv.querySelectorAll(".pz-hit").forEach(x => x.classList.remove("pz-hit"));
    const n = cm.getCursor().line + shift() - 0; // 0-based server line = editor line (0-based) + shift
    pv.querySelectorAll(`[data-source-line="${n}"]`).forEach(x => x.classList.add("pz-hit"));
  });
  let cur = null, timer = null;
  document.querySelectorAll(".pz").forEach(box => box.addEventListener("dblclick", () => openEditor(box.dataset.diagram)));
  document.querySelectorAll(".pz-edit").forEach(b => b.addEventListener("click", () => openEditor(b.dataset.diagram)));
  async function openEditor(id) {
    const d = await (await fetch(`/api/diagrams/${id}`)).json();
    if (!d.editable) { alertMsg("Editing needs the PlantUML server (PLANTUML_URL)."); return; }
    cur = id; skinShift = d.skinShift || 0; cm.setValue(d.source); cm.clearHistory(); setTimeout(() => cm.refresh(), 0); ed.querySelector(".pz-title").textContent = id + (d.default ? " · default" : " · edited, rev " + d.rev);
    ed.classList.add("on"); preview();
  }
  function alertMsg(t) { msg.textContent = t; }
  async function preview() {
    const r = await fetch(`/api/diagrams/${cur}/preview`, { method: "POST", body: cm.getValue() });
    if (r.ok) { pv.innerHTML = await r.text(); const ps = pv.querySelector("svg"); if (ps) { ps.setAttribute("preserveAspectRatio", "xMidYMid meet"); ps.style.width = ""; ps.style.height = ""; } msg.textContent = ""; msg.className = "pz-msg"; }
    else { msg.textContent = (r.status === 422 ? "PlantUML: " : "") + await r.text(); msg.className = "pz-msg err"; }
  }
  cm.on("change", () => { clearTimeout(timer); timer = setTimeout(preview, 500); });
  ed.querySelector(".pz-save").addEventListener("click", async () => {
    const r = await fetch(`/api/diagrams/${cur}`, { method: "PUT", body: cm.getValue(), headers: { "X-Client": me } });
    if (!r.ok) { msg.textContent = await r.text(); msg.className = "pz-msg err"; return; }
    ed.classList.remove("on"); refresh(cur);
  });
  ed.querySelector(".pz-reset").addEventListener("click", async () => {
    await fetch(`/api/diagrams/${cur}`, { method: "DELETE", headers: { "X-Client": me } });
    ed.classList.remove("on"); refresh(cur);
  });
  ed.querySelector(".pz-close").addEventListener("click", () => ed.classList.remove("on"));
  const es = new EventSource("/events");
  es.addEventListener("diagram", ev => { const m = JSON.parse(ev.data); if (m.client !== me) refresh(m.id); });
})();
