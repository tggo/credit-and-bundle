// Model boards on React Flow (@xyflow/react): table nodes inside service groups, draggable,
// editable in the side panel, connectable by dragging between handles. Saved on the server.
import React, { useState, useCallback, useEffect, useRef } from "react";
import { createRoot } from "react-dom/client";
import {
  ReactFlow, ReactFlowProvider, Background, Controls, MiniMap, Handle, Position, MarkerType, ConnectionMode,
  applyNodeChanges, applyEdgeChanges, addEdge, useReactFlow, Panel,
  BaseEdge, EdgeLabelRenderer, getSmoothStepPath, ViewportPortal, NodeResizer, NodeResizeControl, useStore,
} from "@xyflow/react";
import htm from "htm";

const html = htm.bind(React.createElement);
// Bump when the generated layout changes: older saves (and stale open tabs) are ignored and the board is regenerated.
const BOARD_VERSION = 21;

// ---------- definitions ----------
// Rows are "type name [key/note]"; a leading "+" marks a new column, "~" a changed one.
// t(id, title, status, x, y, rows): status same | changed | added
const t = (id, title, status, x, y, rows) => ({ id, title, status, x, y, fields: rows });
// e(source, target, label, card, opts): card = [sourceEnd, targetEnd] in crow's-foot terms: one | many | zmany | onlyone
const e = (s, tg, label, card = ["one", "many"], o = {}) => ({ s, tg, label, card, ...o });

const DEFS = {
  current: {
    groups: [
      ["g_app", "APPLICATION SERVICE", "app", ["application"]], ["g_cat", "CATALOG SERVICE", "cat", ["plan", "offering"]],
      ["g_bill", "BILLING SERVICE", "bill", ["subscription"]], ["g_lic", "LICENSE SERVICE", "lic", ["license"]],
      ["g_cust", "CUSTOMER SERVICE", "cust", ["customer"]],
    ],
    tables: [
      t("application", "APPLICATION", "same", 0, 0, ["uuid id PK", "uuid vendor_id FK", "string name", "string status"]),
      t("plan", "PLAN", "same", 380, 0, ["uuid id PK", "uuid application_id FK", "string name", "int slot_count", "string licensing_model", "int trial_period", "int grace_period"]),
      t("offering", "OFFERING", "same", 760, 0, ["uuid id PK", "uuid plan_id FK", "decimal price", "string billing_period"]),
      t("subscription", "SUBSCRIPTION", "same", 1180, 0, ["uuid id PK", "uuid offering_id FK", "uuid customer_id FK", "string status", "datetime next_billing_at", "string payment_ref"]),
      t("license", "LICENSE", "same", 1580, 0, ["uuid id PK", "uuid subscription_id FK", "uuid customer_id FK", "string key UK", "string status", "date due_date"]),
      t("customer", "CUSTOMER", "same", 1200, 330, ["uuid id PK", "string email UK", "string status"]),
    ],
    edges: [
      e("application", "plan", "", ["onlyone", "zmany"]), e("plan", "offering", "", ["onlyone", "many"]),
      e("offering", "subscription", "", ["onlyone", "zmany"]), e("subscription", "license", "", ["onlyone", "onlyone"]),
      e("customer", "subscription", "", ["onlyone", "zmany"], { sh: "right", th: "right" }),
      e("customer", "license", "", ["onlyone", "zmany"], { sh: "right", th: "left" }),
    ],
  },
  changed: {
    groups: [
      ["g_app", "APPLICATION SERVICE", "app", ["application"]],
      ["g_cat", "CATALOG SERVICE", "cat", ["plan", "action_price", "offering", "offering_item", "credit_package"]],
      ["g_bill", "BILLING SERVICE (Billing team)", "bill", ["subscription", "payment"]],
      ["g_lic", "LICENSE SERVICE", "lic", ["license"]], ["g_cust", "CUSTOMER SERVICE", "cust", ["customer"]],
      ["g_cr", "CREDITS SERVICE (new)", "cred", ["credit_account", "credit_lot", "lot_scope_app", "credit_charge", "credit_ledger"]],
    ],
    tables: [
      t("application", "APPLICATION", "same", 0, 0, ["uuid id PK", "uuid vendor_id FK", "string name", "string status"]),
      t("plan", "PLAN", "changed", 380, 0, ["uuid id PK", "uuid application_id FK", "string name", "int slot_count", "string licensing_model", "- int trial_period → OFFERING.trial_days", "- int grace_period → OFFERING.grace_days"]),
      t("action_price", "ACTION_PRICE", "added", 380, 300, ["uuid application_id PK", "string action_code PK", "bigint credits", "timestamptz valid_from PK"]),
      t("offering", "OFFERING", "changed", 760, 0, ["uuid id PK", "string name", "string status", "- uuid plan_id → OFFERING_ITEM.plan_id", "+ string kind single|bundle|credit_pack", "~ bigint price_minor", "+ string currency", "~ string billing_period +one_time", "+ int trial_days", "+ int grace_days"]),
      t("offering_item", "OFFERING_ITEM", "added", 760, 330, ["uuid id PK", "uuid offering_id FK", "string item_type plan|credit_package", "uuid plan_id FK", "uuid credit_package_id FK", "int quantity", "bigint allocation_weight"]),
      t("credit_package", "CREDIT_PACKAGE", "added", 760, 590, ["uuid id PK", "string name", "bigint credits", "int validity_days", "+ uuid application_id FK in-app pack"]),
      t("subscription", "SUBSCRIPTION", "changed", 1180, 0, ["uuid id PK", "uuid offering_id FK", "uuid customer_id FK", "string status", "+ string kind recurring|one_time", "+ string provider_sub_ref UK", "~ timestamptz current_period_start", "~ timestamptz current_period_end", "+ timestamptz created_at", "- timestamptz next_billing_at → PAYMENT.period_end", "- string payment_ref → PAYMENT.payment_ref"]),
      t("payment", "PAYMENT", "added", 1180, 300, ["uuid id PK", "uuid subscription_id FK", "string payment_ref UK", "bigint amount_minor", "string currency", "bigint refunded_minor", "timestamptz period_start", "timestamptz period_end", "string status", "timestamptz created_at"]),
      t("license", "LICENSE", "changed", 1580, 0, ["uuid id PK", "uuid subscription_id FK UK①", "uuid customer_id FK", "+ uuid offering_item_id FK UK①", "+ uuid application_id FK", "string key UK", "string status", "timestamptz due_date"]),
      t("customer", "CUSTOMER", "same", 1580, 380, ["uuid id PK", "string email UK", "string name", "string status"]),
      t("credit_account", "CREDIT_ACCOUNT", "added", 2000, 0, ["uuid id PK", "uuid customer_id UK", "string status active|blocked"]),
      t("credit_lot", "CREDIT_LOT", "added", 2000, 190, ["uuid id PK", "uuid account_id FK", "string source_type bundle|subscription|pack|promo", "uuid payment_id FK UK①", "uuid offering_item_id FK UK①", "uuid subscription_id FK refund/cancel", "string label", "string scope platform|bundle", "string kind paid|bonus", "bigint granted", "bigint remaining ≥0", "timestamptz expires_at", "timestamptz revoked_at payment refunded", "timestamptz created_at"]),
      t("lot_scope_app", "LOT_SCOPE_APP", "added", 2000, 500, ["uuid lot_id PK", "uuid application_id PK"]),
      t("credit_charge", "CREDIT_CHARGE", "added", 2400, 0, ["uuid id PK", "uuid account_id FK UK①", "uuid application_id FK", "string action_code", "bigint amount price at charge time", "string status", "string idempotency_key UK①", "string request_hash same key+other body→422", "jsonb result", "bigint cost_minor provider cost", "timestamptz created_at"]),
      t("credit_ledger", "CREDIT_LEDGER", "added", 2400, 330, ["bigint id PK", "uuid account_id FK", "uuid lot_id FK", "uuid charge_id FK", "string type grant|charge|refund|expire|revoke", "bigint delta", "string note", "timestamptz created_at"]),
    ],
    edges: [
      e("application", "plan", "", ["onlyone", "many"]),
      e("application", "action_price", "", ["onlyone", "zmany"], { sh: "bottom", th: "left" }),
      e("plan", "offering_item", "", ["onlyone", "zmany"], { sh: "bottom", th: "left" }),
      e("offering", "offering_item", "", ["onlyone", "many"], { sh: "bottom", th: "top" }),
      e("credit_package", "offering_item", "", ["onlyone", "zmany"], { sh: "top", th: "bottom" }),
      e("offering", "subscription", "", ["onlyone", "zmany"]),
      e("subscription", "payment", "", ["onlyone", "zmany"], { sh: "bottom", th: "top" }),
      e("subscription", "license", "1 : N now", ["onlyone", "zmany"]),
      e("offering_item", "license", "per plan item", ["onlyone", "zmany"], { sh: "right", th: "left" }),
      e("customer", "subscription", "", ["onlyone", "zmany"], { sh: "left", th: "right" }),
      e("customer", "credit_account", "wallet", ["onlyone", "zone"], { sh: "right", th: "left" }),
      e("credit_account", "credit_lot", "", ["onlyone", "zmany"], { sh: "bottom", th: "top" }),
      e("payment", "credit_lot", "grants per period", ["onlyone", "zmany"], { sh: "right", th: "left", animated: true }),
      e("credit_lot", "lot_scope_app", "", ["onlyone", "zmany"], { sh: "bottom", th: "top" }),
      e("credit_account", "credit_charge", "", ["onlyone", "zmany"]),
      e("credit_lot", "credit_ledger", "", ["onlyone", "zmany"], { sh: "right", th: "left" }),
      e("credit_charge", "credit_ledger", "", ["onlyone", "zmany"], { sh: "bottom", th: "top" }),
      e("action_price", "credit_charge", "price copied at charge time", ["onlyone", "zmany"], { sh: "f-2-R", th: "f-4-L", animated: true }), // credits → amount
    ],
  },
  charge: {
    groups: [["g_flow", "AI ACTION: CHARGE CREDITS", "cred", ["app", "gw", "db", "ai", "ok", "fail"]]],
    tables: [
      t("app", "VENDOR APP", "same", 0, 0, ["POST /api/v1/credits/charges", "Idempotency-Key action input"]),
      t("gw", "AI GATEWAY", "changed", 420, 0, ["stateless N instances", "1. stored key? → replay: 200 / 409 / 422 / 502", "2. license? none → 403", "3. price? none → 404"]),
      t("db", "CREDITS DB (one tx)", "added", 840, 0, ["4. lock credit_account FOR UPDATE", "5. insert charge (account,key) UK", "→ key exists: replay 200 / 409 / 422 / 502", "6. pick lots bundle→bonus→expiry", "7. debit lots, ledger rows", "8. short → ROLLBACK → 402", "→ enough → COMMIT"]),
      t("ai", "AI PROVIDER", "same", 1260, 0, ["max_tokens capped", "to the action price"]),
      t("ok", "SUCCESS / CONTENT BLOCK", "added", 1020, 330, ["charge completed", "result stored → 201"]),
      t("fail", "PROVIDER 5xx / TIMEOUT", "changed", 1440, 330, ["refund into same lots", "charge refunded → 502", "upstream_failed / timed_out"]),
    ],
    edges: [
      e("app", "gw", "request", ["one", "one"]), e("gw", "db", "charge", ["one", "one"]),
      e("db", "ai", "after commit", ["one", "one"], { animated: true }),
      e("ai", "ok", "ok", ["one", "one"], { sh: "bottom", th: "top" }), e("ai", "fail", "error", ["one", "one"], { sh: "bottom", th: "top", animated: true }),
    ],
  },
};

function edge(id, source, target, label, card = ["one", "many"], o = {}) {
  return { id, source, target, label, type: "wp", animated: !!o.animated, zIndex: 5, data: { points: [] },
    sourceHandle: slot(o.sh || "right"), targetHandle: slot(o.th || "left"),
    markerStart: "cf-" + card[0], markerEnd: "cf-" + card[1],
    style: { stroke: "#5c6370", strokeWidth: 1.6 },
    labelStyle: { fill: "#343a40", fontSize: 12, fontWeight: 500 }, labelBgStyle: { fill: "#ffffff" },
    labelBgPadding: [6, 3], labelBgBorderRadius: 4, pathOptions: { borderRadius: 12 } };
}

// Each side has three connection slots (25/50/75 %) so parallel lines do not merge.
const SLOTS = [1, 2, 3];
const slot = (h) => (h && (/-\d$/.test(h) || h.startsWith("f-")) ? h : (h || "right") + "-2");

const W = 300, HEAD = 34, ROW = 26;
// Wide enough for the longest row: type column + name + key/note, monospace ~7.3 px per char.
const tableWidth = (fields) => Math.max(W, ...fields.map(f => 110 + f.replace(/^[+~-]\s/, "").split(/\s+/).slice(1).join(" ").length * 7.3 + 30));
const nodeH = (fields) => HEAD + fields.length * ROW + 2;

// Columns are the distinct x values of DEFS; each column is as wide as its widest table, so wide tables never
// run into the next column or service box.
function packColumns(tables) {
  const xs = [...new Set(tables.map(t => t.x))].sort((a, b) => a - b);
  const nx = {};
  let cur = 0;
  for (const x of xs) {
    nx[x] = cur;
    cur += Math.max(...tables.filter(t => t.x === x).map(t => tableWidth(t.fields))) + 110;
  }
  // Inside a column a table starts below the previous one, however many rows that one has.
  const out = tables.map(t => ({ ...t, x: nx[t.x] }));
  for (const x of xs) {
    let bottom = -Infinity;
    for (const t of out.filter(t => t.x === nx[x]).sort((a, b) => a.y - b.y)) {
      t.y = Math.max(t.y, bottom + 50);
      bottom = t.y + nodeH(t.fields);
    }
  }
  return out;
}

// FK columns that may be NULL: the "one" end of their line is drawn "zero or one" (o|), not "exactly one" (||).
// TestBoardCardinalityMatchesSchema keeps this list equal to the schema.
const NULLABLE_FK = new Set(["credit_lot.payment_id", "credit_lot.offering_item_id", "credit_lot.subscription_id",
  "credit_ledger.charge_id", "offering_item.plan_id", "offering_item.credit_package_id", "credit_package.application_id"]);

function build(def) {
  const pos = Object.fromEntries(packColumns(def.tables).map(x => [x.id, { ...x }]));
  const nodes = [];
  for (const [gid, label, color, members] of def.groups) {
    const ms = members.map(m => pos[m]);
    const x1 = Math.min(...ms.map(m => m.x)) - 22, y1 = Math.min(...ms.map(m => m.y)) - 44;
    const x2 = Math.max(...ms.map(m => m.x + tableWidth(m.fields))) + 22, y2 = Math.max(...ms.map(m => m.y + nodeH(m.fields))) + 22;
    nodes.push({ id: gid, type: "service", position: { x: x1, y: y1 }, data: { label, color }, width: x2 - x1, height: y2 - y1, zIndex: -1 });
    for (const m of members) pos[m].parent = { id: gid, x: x1, y: y1 };
  }
  for (const x of Object.values(pos)) {
    const p = x.parent;
    nodes.push({ id: x.id, type: "table", parentId: p?.id, position: p ? { x: x.x - p.x, y: x.y - p.y } : { x: x.x, y: x.y },
      width: tableWidth(x.fields), data: { title: x.title, status: x.status, fields: x.fields }, zIndex: 10 });
  }
  // Point relations at fields: PK "id" of the source → the FK row of the target (e.g. plan.id → offering.plan_id).
  const abs = Object.fromEntries(def.tables.map(x => [x.id, x]));
  const rowName = (tb, i) => parseRow(tb.fields[i]).name;
  const findRow = (tb, names) => tb.fields.findIndex((_, i) => names.includes(rowName(tb, i)));
  const edges = def.edges.map((x, i) => {
    const a = abs[x.s], b = abs[x.tg];
    if (a && b) {
      const base = x.s.replace(/^credit_/, "");
      const fk = findRow(b, [x.s + "_id", base + "_id"]);
      const pk = findRow(a, ["id"]);
      if (fk >= 0 && pk >= 0) {
        const right = b.x > a.x + W / 2, left = b.x + W / 2 < a.x;
        const sh = right ? "R" : left ? "L" : "R", th = right ? "L" : left ? "R" : "R";
        const card = NULLABLE_FK.has(`${x.tg}.${rowName(b, fk)}`) ? ["zone", x.card[1]] : x.card;
        return edge(`e${i}`, x.s, x.tg, x.label, card, { ...x, sh: `f-${pk}-${sh}`, th: `f-${fk}-${th}` });
      }
    }
    return edge(`e${i}`, x.s, x.tg, x.label, x.card, x);
  });
  // Every other "<table>_id" column that points at a table on this board gets its own field-to-field line,
  // so a new FK never sits on the board without a relation (credit_ledger.account_id → credit_account.id).
  const covered = new Set(def.edges.map(x => `${x.tg}.${x.s}`));
  for (const b of def.tables) {
    b.fields.forEach((f, fi) => {
      const r = parseRow(f);
      if (r.mark === "del" || r.name === "id" || !r.name.endsWith("_id")) return;
      const base = r.name.slice(0, -3);
      const target = [base, "credit_" + base].find(id => abs[id] && id !== b.id);
      if (!target || covered.has(`${b.id}.${target}`)) return;
      covered.add(`${b.id}.${target}`);
      const a = abs[target], pk = findRow(a, ["id"]);
      if (pk < 0) return;
      const right = b.x > a.x + W / 2, left = b.x + W / 2 < a.x;
      const sh = right ? "R" : left ? "L" : "R", th = right ? "L" : left ? "R" : "R";
      edges.push(edge(`fk-${b.id}-${r.name}`, target, b.id, "", [NULLABLE_FK.has(`${b.id}.${r.name}`) ? "zone" : "onlyone", "zmany"], { sh: `f-${pk}-${sh}`, th: `f-${fi}-${th}` }));
    });
  }
  return { nodes, edges };
}

// ---------- node renderers ----------
const sides = [Position.Top, Position.Right, Position.Bottom, Position.Left];

function parseRow(f) {
  let mark = "";
  if (/^[+~-]\s/.test(f)) { mark = { "+": "add", "~": "chg", "-": "del" }[f[0]]; f = f.slice(2); }
  const m = f.match(/^(uuid|string|int|bigint|decimal|datetime|date|timestamptz|jsonb|bool)\s+(\S+)\s*(.*)$/);
  return m ? { mark, type: m[1], name: m[2], key: m[3] } : { mark, type: "", name: f, key: "" };
}

// Tables resize horizontally: drag the right edge (shown on hover).
function TableNode({ data, selected }) {
  return html`<div class=${"tbl tbl-" + data.status + (selected ? " sel" : "")}>
    <${NodeResizeControl} position="right" variant="line" resizeDirection="horizontal" minWidth=${220} className="tbl-resize" />
    ${sides.flatMap(p => SLOTS.map(i => html`<${Handle} key=${p + i} id=${p + "-" + i} type="source" position=${p}
      style=${p === Position.Top || p === Position.Bottom ? { left: i * 25 + "%" } : { top: i * 25 + "%" }} />`))}
    <div class="tbl-h">${data.title}</div>
    <div class="tbl-b">${data.fields.map((f, i) => {
      const r = parseRow(f);
      return html`<div key=${i} class=${"tbl-r" + (r.mark ? " tbl-r-" + r.mark : "")}>
        <${Handle} id=${"f-" + i + "-L"} type="source" position=${Position.Left} className="fh" />
        <${Handle} id=${"f-" + i + "-R"} type="source" position=${Position.Right} className="fh" />
        <span class="c-type">${{ add: "+ ", chg: "~ ", del: "− " }[r.mark] || ""}${r.type}</span><span class="c-name">${r.name}</span><span class="c-key">${r.key}</span></div>`;
    })}</div>
  </div>`;
}

function ServiceNode({ data, selected }) {
  return html`<${React.Fragment}>
    <${NodeResizer} isVisible=${true} minWidth=${200} minHeight=${120} lineStyle=${{ borderColor: "#3b5bdb" }}
      handleStyle=${{ width: 10, height: 10, borderRadius: 3, background: "#fff", border: "2px solid #3b5bdb" }} />
    <div class=${"svc svc-" + (data.color || "app")}><div class="svc-l">${data.label}</div></div>
  <//>`;
}

// Free text block: sticky note on the canvas. Edit in the side panel.
function NoteNode({ data, selected }) {
  return html`<div class=${"note note-" + (data.color || "violet") + (selected ? " sel" : "")}>
    ${sides.flatMap(p => SLOTS.map(i => html`<${Handle} key=${p + i} id=${p + "-" + i} type="source" position=${p}
      style=${p === Position.Top || p === Position.Bottom ? { left: i * 25 + "%" } : { top: i * 25 + "%" }} />`))}
    ${data.title ? html`<div class="note-t">${data.title}</div>` : null}<div class="note-x">${data.text}</div></div>`;
}

const nodeTypes = { table: TableNode, service: ServiceNode, note: NoteNode };

// Edge with bend points. Without points it is a rounded step line; with points it goes through them.
// Selected: drag the white dots to move bends, drag a blue "+" to add one, double-click a dot to remove it.
function roundedPath(pts, r = 10) {
  let d = `M ${pts[0].x} ${pts[0].y}`;
  for (let i = 1; i < pts.length - 1; i++) {
    const a = pts[i - 1], b = pts[i], c = pts[i + 1];
    const l1 = Math.hypot(b.x - a.x, b.y - a.y), l2 = Math.hypot(c.x - b.x, c.y - b.y);
    const k = Math.min(r, l1 / 2, l2 / 2);
    const p1 = { x: b.x - (b.x - a.x) / (l1 || 1) * k, y: b.y - (b.y - a.y) / (l1 || 1) * k };
    const p2 = { x: b.x + (c.x - b.x) / (l2 || 1) * k, y: b.y + (c.y - b.y) / (l2 || 1) * k };
    d += ` L ${p1.x} ${p1.y} Q ${b.x} ${b.y} ${p2.x} ${p2.y}`;
  }
  const z = pts[pts.length - 1];
  return d + ` L ${z.x} ${z.y}`;
}

const OUT = { top: [0, -1], bottom: [0, 1], left: [-1, 0], right: [1, 0] };


// Wire routing: an orthogonal path from handle to handle that goes around every table.
// A* on a 10 px grid; tables (plus a margin) are walls; every turn costs extra so wires stay straight.
const GRID = 10, MARGIN = 14, STUB = 24, TURN = 8;
const DIRS = [[1, 0], [0, 1], [-1, 0], [0, -1]];
function routeWire(a, b, obstacles) {
  const pad = 400;
  const minX = Math.min(a.x, b.x) - pad, minY = Math.min(a.y, b.y) - pad;
  const w = Math.ceil((Math.abs(a.x - b.x) + 2 * pad) / GRID) + 1, h = Math.ceil((Math.abs(a.y - b.y) + 2 * pad) / GRID) + 1;
  const blocked = new Uint8Array(w * h);
  for (const o of obstacles) {
    const x1 = Math.max(0, Math.floor((o.x - MARGIN - minX) / GRID)), x2 = Math.min(w - 1, Math.ceil((o.x + o.w + MARGIN - minX) / GRID));
    const y1 = Math.max(0, Math.floor((o.y - MARGIN - minY) / GRID)), y2 = Math.min(h - 1, Math.ceil((o.y + o.h + MARGIN - minY) / GRID));
    for (let y = y1; y <= y2; y++) for (let x = x1; x <= x2; x++) blocked[y * w + x] = 1;
  }
  const cell = (p) => [Math.round((p.x - minX) / GRID), Math.round((p.y - minY) / GRID)];
  const [sx, sy] = cell(a), [tx, ty] = cell(b);
  if (sx < 0 || sy < 0 || tx < 0 || ty < 0 || sx >= w || tx >= w || sy >= h || ty >= h) return null;
  blocked[sy * w + sx] = 0; blocked[ty * w + tx] = 0;
  const N = w * h * 4, g = new Float64Array(N).fill(Infinity), prev = new Int32Array(N).fill(-1);
  const heap = []; // [f, state]
  const push = (f, st) => { heap.push([f, st]); let i = heap.length - 1; while (i > 0) { const p = (i - 1) >> 1; if (heap[p][0] <= heap[i][0]) break; [heap[p], heap[i]] = [heap[i], heap[p]]; i = p; } };
  const pop = () => { const top = heap[0], last = heap.pop(); if (heap.length) { heap[0] = last; let i = 0; for (;;) { const l = 2 * i + 1, r = l + 1; let m = i; if (l < heap.length && heap[l][0] < heap[m][0]) m = l; if (r < heap.length && heap[r][0] < heap[m][0]) m = r; if (m === i) break; [heap[m], heap[i]] = [heap[i], heap[m]]; i = m; } } return top; };
  const hgt = (x, y) => Math.abs(x - tx) + Math.abs(y - ty);
  for (let d = 0; d < 4; d++) { const st = (sy * w + sx) * 4 + d; g[st] = 0; push(hgt(sx, sy), st); }
  let end = -1, guard = 0;
  while (heap.length && guard++ < 400000) {
    const [, st] = pop(); const c = st >> 2, d = st & 3, x = c % w, y = (c / w) | 0;
    if (x === tx && y === ty) { end = st; break; }
    for (let nd = 0; nd < 4; nd++) {
      if ((nd + 2) % 4 === d) continue;
      const nx = x + DIRS[nd][0], ny = y + DIRS[nd][1];
      if (nx < 0 || ny < 0 || nx >= w || ny >= h || blocked[ny * w + nx]) continue;
      const ns = (ny * w + nx) * 4 + nd, cost = g[st] + 1 + (nd === d ? 0 : TURN);
      if (cost < g[ns]) { g[ns] = cost; prev[ns] = st; push(cost + hgt(nx, ny), ns); }
    }
  }
  if (end < 0) return null;
  const pts = [];
  for (let st = end, d = -1; st >= 0; st = prev[st]) {
    const c = st >> 2, nd = st & 3;
    if (nd !== d || prev[st] < 0) pts.push({ x: minX + (c % w) * GRID, y: minY + ((c / w) | 0) * GRID });
    d = nd;
  }
  return pts.reverse();
}
const midOf = (ps) => {
  let L = 0; for (let i = 1; i < ps.length; i++) L += Math.hypot(ps[i].x - ps[i - 1].x, ps[i].y - ps[i - 1].y);
  let half = L / 2;
  for (let i = 1; i < ps.length; i++) { const d = Math.hypot(ps[i].x - ps[i - 1].x, ps[i].y - ps[i - 1].y); if (half <= d) { const t = half / (d || 1); return { x: ps[i - 1].x + t * (ps[i].x - ps[i - 1].x), y: ps[i - 1].y + t * (ps[i].y - ps[i - 1].y) }; } half -= d; }
  return ps[0];
};

function WpEdge(props) {
  const { id, sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, data, selected, label, markerStart, markerEnd, style, animated } = props;
  const rf = useReactFlow();
  const points = data?.points || [];
  // Table rectangles in flow coordinates; the string key keeps the route memoised until something moves.
  const obsKey = useStore(st => [...st.nodeLookup.values()].filter(n => n.type === "table")
    .map(n => `${Math.round(n.internals.positionAbsolute.x)},${Math.round(n.internals.positionAbsolute.y)},${n.measured?.width || 0},${n.measured?.height || 0}`).join(";"));
  const wire = React.useMemo(() => {
    if (points.length) return null;
    const so = OUT[sourcePosition], to = OUT[targetPosition];
    const a = { x: sourceX + so[0] * STUB, y: sourceY + so[1] * STUB }, b = { x: targetX + to[0] * STUB, y: targetY + to[1] * STUB };
    const obstacles = obsKey ? obsKey.split(";").map(r => { const [x, y, w, h] = r.split(",").map(Number); return { x, y, w, h }; }) : [];
    const route = routeWire(a, b, obstacles);
    return route ? [{ x: sourceX, y: sourceY }, ...route, { x: targetX, y: targetY }] : null;
  }, [points.length, sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, obsKey]);
  let path, lx, ly, all;
  if (!points.length && wire) {
    all = wire; path = roundedPath(all);
    ({ x: lx, y: ly } = midOf(all));
  } else if (!points.length) {
    [path, lx, ly] = getSmoothStepPath({ sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, borderRadius: 12 });
    all = [{ x: sourceX, y: sourceY }, { x: targetX, y: targetY }];
  } else {
    // short stubs out of the handles keep the crow's feet straight
    const so = OUT[sourcePosition], to = OUT[targetPosition];
    all = [{ x: sourceX, y: sourceY }, { x: sourceX + so[0] * 24, y: sourceY + so[1] * 24 }, ...points,
      { x: targetX + to[0] * 24, y: targetY + to[1] * 24 }, { x: targetX, y: targetY }];
    path = roundedPath(all);
    const mid = all[Math.floor(all.length / 2)];
    lx = mid.x; ly = mid.y;
  }

  const setPoints = (fn) => rf.setEdges(es => es.map(e => e.id === id ? { ...e, data: { ...e.data, points: fn(e.data?.points || []) } } : e));
  const drag = (ev, index, insert) => {
    ev.stopPropagation(); ev.preventDefault();
    let i = index;
    if (insert) setPoints(ps => { const n = [...ps]; n.splice(i, 0, rf.screenToFlowPosition({ x: ev.clientX, y: ev.clientY })); return n; });
    const move = (m) => {
      const p = rf.screenToFlowPosition({ x: m.clientX, y: m.clientY });
      const snap = { x: Math.round(p.x / 10) * 10, y: Math.round(p.y / 10) * 10 };
      setPoints(ps => ps.map((q, j) => j === i ? snap : q));
    };
    const up = () => { window.removeEventListener("pointermove", move); window.removeEventListener("pointerup", up); };
    window.addEventListener("pointermove", move); window.addEventListener("pointerup", up);
  };

  // "+" handles on the middle of every segment between the user's bends (or of the whole line)
  const anchors = points.length ? [all[1], ...points, all[all.length - 2]] : null;
  const plus = anchors ? anchors.slice(1).map((b, k) => ({ x: (anchors[k].x + b.x) / 2, y: (anchors[k].y + b.y) / 2, at: k }))
    : [{ x: lx, y: ly, at: 0 }];
  // Pulling the line itself inserts a bend into the nearest segment.
  const grab = (ev) => {
    if (!selected) return;
    const p = rf.screenToFlowPosition({ x: ev.clientX, y: ev.clientY });
    let at = 0;
    if (anchors) {
      let best = Infinity;
      anchors.slice(1).forEach((b, k) => {
        const a = anchors[k], dx = b.x - a.x, dy = b.y - a.y, L = dx * dx + dy * dy || 1;
        const t = Math.max(0, Math.min(1, ((p.x - a.x) * dx + (p.y - a.y) * dy) / L));
        const d = Math.hypot(a.x + t * dx - p.x, a.y + t * dy - p.y);
        if (d < best) { best = d; at = k; }
      });
    }
    drag(ev, at, true);
  };

  return html`<${React.Fragment}>
    <${BaseEdge} id=${id} path=${path} markerStart=${markerStart} markerEnd=${markerEnd}
      style=${{ ...style, stroke: selected ? "#3b5bdb" : style?.stroke, strokeWidth: selected ? 2.4 : style?.strokeWidth,
        strokeDasharray: animated ? "6 4" : undefined }} interactionWidth=${16} />
    ${selected ? html`<path d=${path} fill="none" stroke="transparent" strokeWidth="18" class="nodrag nopan grab-line" onPointerDown=${grab} />` : null}
    <${EdgeLabelRenderer}>
      ${label ? html`<div class="edge-label" style=${{ transform: `translate(-50%,-50%) translate(${lx}px,${ly}px)` }}>${label}</div>` : null}
      ${selected ? points.map((p, i) => html`<div key=${"p" + i} class="bend nodrag nopan" title="drag to move, double-click to remove"
          style=${{ transform: `translate(-50%,-50%) translate(${p.x}px,${p.y}px)` }}
          onPointerDown=${ev => drag(ev, i)} onDoubleClick=${ev => { ev.stopPropagation(); setPoints(ps => ps.filter((_, j) => j !== i)); }}></div>`) : null}
      ${selected ? plus.map(p => html`<div key=${"a" + p.at} class="bend-add nodrag nopan" title="drag to add a bend"
          style=${{ transform: `translate(-50%,-50%) translate(${p.x}px,${p.y}px)` }}
          onPointerDown=${ev => drag(ev, p.at, true)}>+</div>`) : null}
    <//>
  <//>`;
}

const edgeTypes = { wp: WpEdge };

// Crow's-foot markers, referenced by edges as markerStart/markerEnd = "cf-<kind>".
function CrowsFoot() {
  const m = (id, body) => html`<marker id=${id} key=${id} viewBox="0 0 24 24" markerWidth="22" markerHeight="22" refX="22" refY="12" orient="auto-start-reverse" markerUnits="userSpaceOnUse">
    <g fill="none" stroke="#5c6370" strokeWidth="1.6">${body}</g></marker>`;
  return html`<svg style=${{ position: "absolute", width: 0, height: 0 }}><defs>
    ${m("cf-one", html`<path d="M14 5 V19 M0 12 H22" />`)}
    ${m("cf-onlyone", html`<path d="M10 5 V19 M15 5 V19 M0 12 H22" />`)}
    ${m("cf-many", html`<path d="M8 5 V19 M0 12 H22 M12 12 L22 4 M12 12 L22 20" />`)}
    ${m("cf-zone", html`<circle cx="8" cy="12" r="4" fill="#fff" /><path d="M12 12 H22 M18 5 V19" />`)}
    ${m("cf-zmany", html`<circle cx="6" cy="12" r="4" fill="#fff" /><path d="M10 12 H22 M12 12 L22 4 M12 12 L22 20" />`)}
  </defs></svg>`;
}

// ---------- side panel ----------
const CARD_OPTS = [["onlyone", "exactly one ||"], ["one", "one |"], ["many", "one or many |<"], ["zmany", "zero or many o<"], ["zone", "zero or one o|"]]
  .map(([v, t]) => html`<option key=${v} value=${v}>${t}</option>`);
const HANDLE_OPTS = ["top", "right", "bottom", "left"].flatMap(sd => SLOTS.map(i =>
  html`<option key=${sd + i} value=${sd + "-" + i}>table ${sd} ${["", "· first", "· middle", "· last"][i]}</option>`));
function handleOpts(nodes, id) {
  const n = nodes.find(x => x.id === id);
  const rows = n?.type === "table" ? n.data.fields.flatMap((f, i) => ["L", "R"].map(sd =>
    html`<option key=${"f" + i + sd} value=${`f-${i}-${sd}`}>${parseRow(f).name} · ${sd === "L" ? "left" : "right"}</option>`)) : [];
  return [...rows, ...HANDLE_OPTS];
}
// Table and column descriptions from Postgres COMMENT ON (the same text as the Data page).
const COMMENTS = {};
fetch("/api/schema/comments").then(r => r.json()).then(j => Object.assign(COMMENTS, j)).catch(() => {});
const commentsFor = (id) => [id + "s", id + "es", id].map(t => COMMENTS[t] && { table: t, ...COMMENTS[t] }).find(Boolean);
function TableRole({ node }) {
  const c = commentsFor(node.id);
  if (!c) return html`<p class="role muted">No description in the schema for this table (design-only or renamed).</p>`;
  const cols = node.data.fields.map(f => parseRow(f).name).filter(n => c.cols[n]);
  return html`<div class="role">
    <p><b>Role.</b> ${c.comment || "—"}</p>
    ${cols.length ? html`<dl>${cols.map(n => html`<dt key=${"t" + n}><code>${n}</code></dt><dd key=${"d" + n}>${c.cols[n]}</dd>`)}</dl>` : null}
    <p class="muted small">From <code>COMMENT ON</code> on <code>${c.table}</code> — same as the Data page.</p>
  </div>`;
}

function Inspector({ nodes, node, edge, onNode, onEdge, onDelete }) {
  if (node && node.type === "table") {
    const d = node.data;
    return html`<div class="insp">
      <h4>${d.title}</h4>
      <${TableRole} node=${node} />
      <h4>Edit</h4>
      <label>Title<input value=${d.title} onInput=${e => onNode({ ...d, title: e.target.value })} /></label>
      <label>Status<select value=${d.status} onChange=${e => onNode({ ...d, status: e.target.value })}>
        <option value="same">unchanged</option><option value="changed">changed</option><option value="added">new</option></select></label>
      <label>Fields — one per line. Prefix: + new column, ~ changed type or meaning, - removed (write where it moved: → TABLE.column)
        <textarea rows="12" value=${d.fields.join("\n")} onInput=${e => onNode({ ...d, fields: e.target.value.split("\n") })}></textarea></label>
      <button class="danger" onClick=${onDelete}>Delete table</button>
    </div>`;
  }
  if (node && node.type === "note") {
    const d = node.data;
    return html`<div class="insp">
      <h4>Text block</h4>
      <label>Title<input value=${d.title || ""} onInput=${e => onNode({ ...d, title: e.target.value })} /></label>
      <label>Text<textarea rows="12" value=${d.text} onInput=${e => onNode({ ...d, text: e.target.value })}></textarea></label>
      <label>Color<select value=${d.color || "violet"} onChange=${e => onNode({ ...d, color: e.target.value })}>
        <option value="yellow">yellow</option><option value="violet">violet</option><option value="blue">blue</option><option value="green">green</option><option value="red">red</option></select></label>
      <button class="danger" onClick=${onDelete}>Delete block</button>
    </div>`;
  }
  if (node && node.type === "service") {
    return html`<div class="insp"><h4>Service area</h4>
      <label>Name<input value=${node.data.label} onInput=${e => onNode({ label: e.target.value })} /></label>
      <p class="muted small">Drag the area to move all its tables together. Drag the blue corner or edge handles to resize it.</p></div>`;
  }
  if (edge) {
    return html`<div class="insp"><h4>Relation</h4>
      <label>Label<input value=${edge.label || ""} onInput=${e => onEdge({ label: e.target.value })} /></label>
      <label>Cardinality at source<select value=${(edge.markerStart || "").replace("cf-", "")} onChange=${e => onEdge({ markerStart: "cf-" + e.target.value })}>${CARD_OPTS}</select></label>
      <label>Cardinality at target<select value=${(edge.markerEnd || "").replace("cf-", "")} onChange=${e => onEdge({ markerEnd: "cf-" + e.target.value })}>${CARD_OPTS}</select></label>
      <div class="two-sel">
        <label>From<select value=${edge.sourceHandle} onChange=${e => onEdge({ sourceHandle: e.target.value })}>${handleOpts(nodes, edge.source)}</select></label>
        <label>To<select value=${edge.targetHandle} onChange=${e => onEdge({ targetHandle: e.target.value })}>${handleOpts(nodes, edge.target)}</select></label>
      </div>
      <p class="muted small">Selected line (blue): grab the line anywhere and pull — a bend appears. Drag white dots to move bends, double-click a dot to remove it. To re-attach an end, pick a field or side above.</p>
      <button onClick=${() => onEdge({ data: { ...(edge.data || {}), points: [] } })}>Straighten (remove bends)</button>
      <label class="row"><input type="checkbox" checked=${!!edge.animated} onChange=${e => onEdge({ animated: e.target.checked })} /> async / derived (animated)</label>
      <button class="danger" onClick=${onDelete}>Delete relation</button></div>`;
  }
  return html`<div class="insp muted small">
    <h4>How to edit</h4>
    <p>Drag tables and service areas; tables move with their area.</p>
    <p>Drag from a dot on a table edge to another table to add a relation.</p>
    <p>Click a table or a relation to edit it here. Delete / Backspace removes the selection.</p>
    <p>Everything is saved on the server automatically.</p></div>`;
}

// ---------- collaboration ----------
// Every tab has an identity; saves carry it so a tab ignores its own echo.
const NAMES = ["Otter", "Lynx", "Heron", "Fox", "Panda", "Koala", "Falcon", "Badger", "Orca", "Ibex"];
const COLORS = ["#e8590c", "#2f9e44", "#1c7ed6", "#ae3ec9", "#f08c00", "#0c8599", "#d6336c", "#5f3dc4"];
const ME = (() => {
  let saved = null;
  try { saved = JSON.parse(sessionStorage.getItem("board-me") || "null"); } catch {}
  const me = saved || { name: NAMES[Math.random() * NAMES.length | 0], color: COLORS[Math.random() * COLORS.length | 0] };
  me.client = Math.random().toString(36).slice(2, 10);
  try { sessionStorage.setItem("board-me", JSON.stringify({ name: me.name, color: me.color })); } catch {}
  return me;
})();

function Cursors({ cursors }) {
  return html`<${ViewportPortal}>${Object.values(cursors).map(c => html`<div key=${c.client} class="rcursor"
      style=${{ transform: `translate(${c.x}px, ${c.y}px)`, "--c": c.color }}>
      <svg width="18" height="18" viewBox="0 0 18 18"><path d="M1 1 L1 15 L5 11 L8 17 L10 16 L7 10 L13 10 Z" fill=${c.color} stroke="#fff" strokeWidth="1.2" /></svg>
      <span>${c.name}</span></div>`)}<//>`;
}

// ---------- app ----------
function Board({ board, initial }) {
  const [nodes, setNodes] = useState(initial.nodes);
  const [edges, setEdges] = useState(initial.edges);
  const [sel, setSel] = useState({ node: null, edge: null });
  const [saved, setSaved] = useState("");
  const rf = useReactFlow();
  const first = useRef(true);
  const remote = useRef(false);     // next state change came from another tab: do not save it back
  const dragging = useRef(false);   // hold remote scenes while the local user drags
  const pending = useRef(null);
  const rev = useRef(0);
  const lastScene = useRef("");     // last scene saved or received: identical scenes are not saved again
  const [cursors, setCursors] = useState({});

  // Strip view-only flags before saving or sharing.
  const clean = (ns) => ns.map(({ selected, dragging, measured, ...n }) => n);

  useEffect(() => {
    if (first.current) { first.current = false; return; }
    if (remote.current) { remote.current = false; return; }
    const body = JSON.stringify({ v: BOARD_VERSION, nodes: clean(nodes), edges: edges.map(({ selected, ...e }) => e) });
    if (body === lastScene.current) return;
    const id = setTimeout(() => {
      lastScene.current = body;
      fetch("/api/boards/" + board, { method: "PUT", headers: { "Content-Type": "application/json", "X-Client": ME.client }, body })
        .then(r => { rev.current = +r.headers.get("X-Rev") || rev.current; setSaved("saved " + new Date().toLocaleTimeString()); });
    }, 150);
    return () => clearTimeout(id);
  }, [nodes, edges]);

  const applyRemote = useCallback((m) => {
    if (m.rev <= rev.current) return;
    rev.current = m.rev;
    remote.current = true;
    lastScene.current = JSON.stringify({ v: BOARD_VERSION, nodes: clean(m.scene.nodes), edges: m.scene.edges.map(({ selected, ...e }) => e) });
    setNodes(cur => {
      const sel = new Set(cur.filter(n => n.selected).map(n => n.id));
      return m.scene.nodes.map(n => ({ ...n, selected: sel.has(n.id), zIndex: n.zIndex }));
    });
    setEdges(cur => {
      const sel = new Set(cur.filter(e => e.selected).map(e => e.id));
      return m.scene.edges.map(e => ({ ...e, selected: sel.has(e.id) }));
    });
    setSaved("updated by others " + new Date().toLocaleTimeString());
  }, []);

  useEffect(() => {
    const es = new EventSource("/events");
    es.addEventListener("board", ev => {
      let m; try { m = JSON.parse(ev.data); } catch { return; }
      if (m.board !== board || m.client === ME.client) return;
      if (m.type === "reset") { location.reload(); return; }
      if (m.type === "cursor") { setCursors(c => ({ ...c, [m.client]: { ...m, at: Date.now() } })); return; }
      if (m.type === "scene" && m.scene?.v === BOARD_VERSION) {
        if (dragging.current) pending.current = m; else applyRemote(m);
      }
    });
    const gc = setInterval(() => setCursors(c => Object.fromEntries(Object.entries(c).filter(([, v]) => Date.now() - v.at < 8000))), 2000);
    return () => { es.close(); clearInterval(gc); };
  }, [board]);

  // Send own cursor (throttled) and a heartbeat so others see who is here.
  const last = useRef({ t: 0, x: 0, y: 0 });
  const sendPresence = (x, y) => {
    last.current = { t: Date.now(), x, y };
    fetch(`/api/boards/${board}/presence`, { method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ client: ME.client, name: ME.name, color: ME.color, x, y }) });
  };
  const onMove = (ev) => {
    if (Date.now() - last.current.t < 60) return;
    const p = rf.screenToFlowPosition({ x: ev.clientX, y: ev.clientY });
    sendPresence(p.x, p.y);
  };
  useEffect(() => {
    const hb = setInterval(() => { if (Date.now() - last.current.t > 3000) sendPresence(last.current.x, last.current.y); }, 3000);
    return () => clearInterval(hb);
  }, [board]);

  const onNodesChange = useCallback(ch => setNodes(n => applyNodeChanges(ch, n)), []);
  const onEdgesChange = useCallback(ch => setEdges(e => applyEdgeChanges(ch, e)), []);
  const onConnect = useCallback(c => setEdges(es => addEdge(edge("e" + Date.now(), c.source, c.target, "", ["onlyone", "zmany"], { sh: c.sourceHandle, th: c.targetHandle }), es)), []);
  const onSelectionChange = useCallback(({ nodes: ns, edges: es }) => setSel({ node: ns[0]?.id || null, edge: es[0]?.id || null }), []);

  const node = nodes.find(n => n.id === sel.node);
  const ed = edges.find(e => e.id === sel.edge);

  const addTable = () => {
    const c = rf.screenToFlowPosition({ x: window.innerWidth / 2 - 150, y: window.innerHeight / 2 - 80 });
    setNodes(n => [...n, { id: "t" + Date.now(), type: "table", position: c, zIndex: 10, data: { title: "NEW_TABLE", status: "added", fields: ["uuid id PK"] } }]);
  };
  const addNote = () => {
    const c = rf.screenToFlowPosition({ x: window.innerWidth / 2 - 120, y: window.innerHeight / 2 - 40 });
    setNodes(n => [...n, { id: "n" + Date.now(), type: "note", position: c, zIndex: 10, data: { title: "", text: "Double-check: …", color: "yellow" } }]);
  };

  // Line filter: FK relations (solid) and async / derived links (dashed: grants per period, price copy). View only, per viewer.
  const [show, setShow] = useState(() => { try { return JSON.parse(localStorage.getItem("board-show")) || { fk: true, async: true }; } catch { return { fk: true, async: true }; } });
  const toggle = (k) => setShow(v => { const n = { ...v, [k]: !v[k] }; try { localStorage.setItem("board-show", JSON.stringify(n)); } catch {} return n; });
  // Hovered line: drawn on top in blue, and the two tables it joins are outlined, so you see where it goes.
  const [hover, setHover] = useState(null);
  const shown = edges.map(e => ({ ...e, hidden: !show[e.animated ? "async" : "fk"],
    ...(e.id === hover ? { zIndex: 1000, style: { ...e.style, stroke: "#3b5bdb", strokeWidth: 3 }, className: "hovered" } : {}) }));
  const hoverEdge = edges.find(e => e.id === hover);
  const shownNodes = hoverEdge ? nodes.map(n => (n.id === hoverEdge.source || n.id === hoverEdge.target) ? { ...n, className: "hl" } : n) : nodes;

  return html`<div class="board-wrap">
    <div class="board-canvas" onPointerMove=${onMove}>
      <${ReactFlow} nodes=${shownNodes} edges=${shown} onEdgeMouseEnter=${(_, e) => setHover(e.id)} onEdgeMouseLeave=${() => setHover(null)} nodeTypes=${nodeTypes} edgeTypes=${edgeTypes} colorMode="light"
        onNodesChange=${onNodesChange} onEdgesChange=${onEdgesChange} onConnect=${onConnect}
        onSelectionChange=${onSelectionChange} connectionMode=${ConnectionMode.Loose}
        onNodeDragStart=${() => { dragging.current = true; }}
        onNodeDragStop=${() => { dragging.current = false; if (pending.current) { const m = pending.current; pending.current = null; setTimeout(() => applyRemote(m), 300); } }}
        fitView fitViewOptions=${{ padding: 0.1 }} minZoom=${0.15} snapToGrid snapGrid=${[10, 10]}
        proOptions=${{ hideAttribution: true }}>
        <${Background} variant="dots" gap=${20} size=${1.2} color="#d7dce5" />
        <${CrowsFoot} />
        <${Cursors} cursors=${cursors} />
        <${Controls} />
        <${MiniMap} pannable zoomable nodeColor=${n => n.type === "service" ? "#f1f3f5" : n.type === "note" ? "#e5dbff" : ({ same: "#bfe0fb", changed: "#ffe58f", added: "#b2f2bb" })[n.data.status]} />
        <${Panel} position="top-left">
          <div class="board-tools">
            <button onClick=${addTable}>+ Table</button>
            <button onClick=${addNote}>+ Text</button>
            <button onClick=${() => rf.fitView({ padding: 0.1, duration: 300 })}>Fit</button>
            <label class="flt"><input type="checkbox" checked=${show.fk} onChange=${() => toggle("fk")} /> FK links</label>
            <label class="flt"><input type="checkbox" checked=${show.async} onChange=${() => toggle("async")} /> async / derived</label>
            <span class="legend"><i class="lg-same"></i>unchanged <i class="lg-changed"></i>changed <i class="lg-added"></i>new <i class="lg-del"></i>removed / moved</span>
            <span class="people">
              <i class="dot" style=${{ background: ME.color }} title="you"></i>${ME.name} (you)
              ${Object.values(cursors).map(c => html`<span key=${c.client}><i class="dot" style=${{ background: c.color }}></i>${c.name}</span>`)}
            </span>
            <span class="muted small">${saved}</span>
          </div>
        <//>
      <//>
    </div>
    <${Inspector} nodes=${nodes} node=${node} edge=${ed}
      onNode=${data => setNodes(ns => ns.map(n => n.id === node.id ? { ...n, data } : n))}
      onEdge=${patch => setEdges(es => es.map(e => e.id === ed.id ? { ...e, ...patch } : e))}
      onDelete=${() => {
        if (node) { setNodes(ns => ns.filter(n => n.id !== node.id && n.parentId !== node.id)); setEdges(es => es.filter(e => e.source !== node.id && e.target !== node.id)); }
        if (ed) setEdges(es => es.filter(e => e.id !== ed.id));
        setSel({ node: null, edge: null });
      }} />
  </div>`;
}

async function main() {
  const board = new URLSearchParams(location.search).get("b") || "changed";
  let initial = null;
  const r = await fetch("/api/boards/" + board);
  if (r.ok) {
    const s = await r.json();
    // Layering: service areas below edges, edges below tables (also fixes boards saved before this rule).
    const z = { service: -1, table: 10, note: 10 };
    if (s && s.v === BOARD_VERSION && s.nodes?.length) initial = {
      nodes: s.nodes.filter(n => n.type !== "flow").map(n => ({ ...n, selected: false, zIndex: z[n.type] ?? 10 })),
      edges: s.edges.map(e => ({ ...e, zIndex: 5, type: "wp", data: { points: [], ...(e.data || {}) },
        sourceHandle: slot(e.sourceHandle), targetHandle: slot(e.targetHandle) })),
    };
  }
  initial ||= build(DEFS[board]);
  createRoot(document.getElementById("board")).render(html`<${ReactFlowProvider}><${Board} board=${board} initial=${initial} /><//>`);

  document.getElementById("regen").addEventListener("click", async () => {
    await fetch("/api/boards/" + board, { method: "DELETE", headers: { "X-Client": ME.client } });
    location.reload();
  });
}
main();
