// Leave app — Phase 2 service (zero dependencies, Node 18+).
//
// The standalone employee-facing leave application: serves the tiny React
// page (web/dist), owns balances + the org map + request history, and
// delegates the approval process to FlowForge via its webhook + API.
//
//   Employee-facing (page)          Workflow-facing (connectors, unchanged
//   ----------------------           from the Phase-1 stub contract)
//   POST /api/leave                  GET  /api/balance/:emp?days=
//   GET  /api/view/:emp              POST /api/flowforge/callback
//   POST /api/leave/:id/approve
//   POST /api/leave/:id/reject       Ops
//   GET  /api/org                    GET /health
//   GET  /  -> web/dist
//
// Env: LISTEN (9090)  LEAVE_TOKEN (dev-leave-token)
//     FF_URL (http://localhost:8081)  FF_USER/FF_PASS (service account)
//
// Run:  node leave-app/server.mjs      (build the page first:
//                                       npm --prefix leave-app/web install && run build)

import { createServer } from 'node:http';
import { readFile, writeFile } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { randomBytes } from 'node:crypto';

const ROOT = dirname(fileURLToPath(import.meta.url));
const LISTEN = Number(process.env.LISTEN ?? 9090);
const LEAVE_TOKEN = process.env.LEAVE_TOKEN ?? 'dev-leave-token';
const FF_URL = process.env.FF_URL ?? 'http://localhost:8081';
const FF_USER = process.env.FF_USER ?? 'santhosh';
const FF_PASS = process.env.FF_PASS ?? 'flowforge123';
const DATA_FILE = join(ROOT, 'data.json');
const WEB_DIST = join(ROOT, 'web', 'dist');

// ---------------------------------------------------------------- state ----

const seed = () => ({
  seq: 3,
  employees: {
    'E-4003': { name: 'Sofia Lindqvist', manager: 'E-4007', role: 'employee' },
    'E-4007': { name: 'Elena Fischer', manager: 'E-4006', role: 'employee' },
    'E-4416': { name: 'Aisha Khan', manager: 'E-4006', role: 'hr' },
    'E-4006': { name: 'Priya Raman', manager: 'E-4006', role: 'hr' },
    'E-4417': { name: 'Jonas de Vries', manager: 'E-4007', role: 'employee' },
  },
  balances: { 'E-4003': 18, 'E-4007': 12, 'E-4416': 20, 'E-4006': 25, 'E-4417': 2 },
  requests: [],
  notifications: [],
  readAt: {}, // emp -> ISO timestamp of last "mark read"
});

let db;
if (existsSync(DATA_FILE)) {
  // Merge with seed defaults so state from older versions never crashes
  // the service (missing fields gain their defaults).
  db = { ...seed(), ...JSON.parse(await readFile(DATA_FILE, 'utf8')) };
  db.readAt = db.readAt ?? {};
} else {
  db = seed();
}
const save = () => writeFile(DATA_FILE, JSON.stringify(db, null, 2)).catch(() => {});

const notify = (emp, text) => {
  db.notifications.unshift({ id: randomBytes(4).toString('hex'), emp, text, at: new Date().toISOString() });
  db.notifications = db.notifications.slice(0, 100);
  save();
};

// ------------------------------------------------------ FlowForge client ----

let ffToken = null;
async function ff(path, opts = {}) {
  if (!ffToken) {
    const r = await fetch(`${FF_URL}/api/v1/auth/login`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: FF_USER, password: FF_PASS }),
    });
    if (!r.ok) throw new Error(`FlowForge login failed (${r.status})`);
    ffToken = (await r.json()).token;
  }
  const res = await fetch(`${FF_URL}${path}`, {
    ...opts,
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${ffToken}`, ...(opts.headers ?? {}) },
  });
  if (res.status === 401) { ffToken = null; return ff(path, opts); } // token aged out
  return res;
}

async function discoverHook() {
  const wfs = await (await ff('/api/v1/workflows')).json();
  // Latest deployed version of the leave workflow wins.
  const candidates = wfs.filter((w) => w.name === 'Leave Request' && w.status === 'deployed')
    .sort((a, b) => b.version - a.version);
  const wf = candidates[0];
  if (!wf) throw new Error('deploy workflow "Leave Request" first (leave-app/workflow/leave-request.flow.yaml)');
  const hook = await (await ff(`/api/v1/workflows/${wf.id}/hook`)).json();
  return hook; // { url, token }
}
let hook = null;

// Live status poll: refresh open requests from FlowForge.
async function pollOpen() {
  const open = db.requests.filter((r) => !['approved', 'rejected'].includes(rverdict(r)));
  await Promise.all(open.map(async (req) => {
    try {
      const inst = await (await ff(`/api/v1/executions/${req.instanceId}`)).json();
      applyInstanceState(req, inst);
    } catch { /* FlowForge briefly away — keep last state */ }
  }));
  save();
}

function rverdict(req) { return req.verdict ?? 'pending'; }

function applyInstanceState(req, inst) {
  const prev = rverdict(req);
  const prevStage = req.live?.stage ?? '';
  const verdict =
    inst.status === 'completed' ? 'approved' :
    inst.status === 'failed' ? 'rejected' :
    inst.status === 'cancelled' ? 'rejected' : 'pending';
  req.live = {
    status: inst.status,
    stage: inst.waitingOn ?? '',
    error: inst.error ?? '',
  };
  // Escalation: the manager gate breached its SLA — HR now holds the card.
  if (prevStage !== 'HR Escalation' && req.live.stage === 'HR Escalation') {
    req.escalated = true;
    notify(req.employee_no, `Leave ${req.id} ESCALATED — manager did not respond within the SLA; HR now approves it.`);
  }
  if (verdict !== 'pending') {
    req.verdict = verdict;
    if (verdict === 'approved' && !req.deducted) { // callback usually did this already
      const b = db.balances[req.employee_no];
      if (b != null) db.balances[req.employee_no] = Math.max(0, b - Number(req.days));
      req.deducted = true;
    }
    if (prev === 'pending') {
      notify(req.employee_no, verdict === 'approved'
        ? `Leave ${req.id} approved (${req.days}d ${req.leave_type}).`
        : inst.status === 'failed'
          ? `Leave ${req.id} rejected — ${inst.error || 'validation failed'}.`
          : `Leave ${req.id} rejected by the approver.`);
    }
  }
}

setInterval(() => { pollOpen().catch(() => {}); }, 2000);

// ------------------------------------------------------------------ app ----

const json = (res, code, body) => { res.writeHead(code, { 'Content-Type': 'application/json' }); res.end(JSON.stringify(body)); };
const readBody = (req) => new Promise((resolve) => { let b = ''; req.on('data', (c) => (b += c)); req.on('end', () => { try { resolve(JSON.parse(b)); } catch { resolve(null); } }); });
const stageLabel = (req) => req.live?.stage === 'Reporting Manager' ? 'manager'
  : req.live?.stage === 'HR Final Approval' ? 'hr'
    : req.live?.stage === 'HR Escalation' ? 'escalated'
      : req.live?.stage ? 'other' : 'processing';

const server = createServer(async (req, res) => {
  const url = new URL(req.url, `http://localhost:${LISTEN}`);
  const p = url.pathname;

  // ---- workflow-facing (connector contract, shared-secret gated) --------
  if (p.startsWith('/api/balance') || p === '/api/flowforge/callback') {
    if (req.headers['x-leave-token'] !== LEAVE_TOKEN) return json(res, 401, { error: 'invalid leave-app token' });
  }

  if (req.method === 'GET' && p.startsWith('/api/balance/')) {
    const emp = p.split('/').pop();
    const balance = db.balances[emp];
    if (balance == null) return json(res, 404, { error: `unknown employee ${emp}` });
    const days = Number(url.searchParams.get('days') ?? 0);
    if (days > balance) return json(res, 409, { ok: false, error: `insufficient balance: ${days} requested, ${balance} available`, balance });
    return json(res, 200, { ok: true, employee_no: emp, balance });
  }

  if (req.method === 'POST' && p === '/api/flowforge/callback') {
    const payload = await readBody(req);
    if (!payload) return json(res, 400, { error: 'bad json' });
    const req0 = db.requests.find((r) => r.instanceId && r.employee_no === String(payload.employee_no) && rverdict(r) === 'pending');
    if (payload.verdict === 'approved') {
      const emp = String(payload.employee_no);
      const days = Number(payload.days);
      const bal = db.balances[emp];
      if (bal == null) return json(res, 404, { error: `unknown employee ${emp}` });
      if (req0 && !req0.deducted) {
        if (days > bal) return json(res, 409, { error: 'balance changed since approval', balance: bal });
        db.balances[emp] = bal - days; // deduct ONLY on the approved callback
        req0.deducted = true;
        req0.verdict = 'approved';
        notify(emp, `Leave ${req0.id} approved (${days}d ${payload.leave_type}). Balance now ${db.balances[emp]}.`);
        save();
      }
    }
    console.log(`[callback] ${JSON.stringify(payload)}`);
    return json(res, 200, { received: true });
  }

  // ---- employee-facing ---------------------------------------------------
  if (req.method === 'GET' && p === '/api/org') {
    return json(res, 200, Object.entries(db.employees).map(([no, e]) => ({ no, ...e })));
  }

  if (req.method === 'GET' && p.startsWith('/api/view/')) {
    const emp = p.split('/').pop();
    const me = db.employees[emp];
    if (!me) return json(res, 404, { error: 'unknown employee' });
    const mine = db.requests.filter((r) => r.employee_no === emp).sort((a, b) => b.created.localeCompare(a.created));
    const approvals = db.requests.filter((r) => {
      if (rverdict(r) !== 'pending' || r.employee_no === emp) return false;
      const stage = stageLabel(r);
      if (stage === 'manager') return db.employees[r.employee_no]?.manager === emp;
      if (stage === 'hr' || stage === 'escalated') return me.role === 'hr';
      return false;
    });
    const myNotifs = db.notifications.filter((n) => n.emp === emp).slice(0, 20);
    const readAt = db.readAt[emp] ?? '';
    const unread = myNotifs.filter((n) => n.at > readAt).length;
    const pendingDays = mine.filter((r) => rverdict(r) === 'pending').reduce((a, r) => a + Number(r.days), 0);
    return json(res, 200, {
      me: { no: emp, ...me },
      balance: db.balances[emp] ?? 0,
      pendingDays,
      requests: mine,
      approvals,
      notifications: myNotifs,
      unread,
    });
  }

  if (req.method === 'POST' && p === '/api/leave/read') {
    const b = await readBody(req);
    if (b?.emp && db.employees[b.emp]) {
      db.readAt[b.emp] = new Date().toISOString();
      save();
    }
    return json(res, 200, { ok: true });
  }

  const steps = p.match(/^\/api\/leave\/(LV-\d+)\/steps$/);
  if (req.method === 'GET' && steps) {
    const req0 = db.requests.find((r) => r.id === steps[1]);
    if (!req0) return json(res, 404, { error: 'unknown request' });
    const r = await ff(`/api/v1/executions/${req0.instanceId}/steps`);
    if (!r.ok) return json(res, 502, { error: `FlowForge steps failed (${r.status})` });
    // Audit cross-link: the workflow's own step timeline.
    return json(res, 200, { instanceId: req0.instanceId, steps: await r.json() });
  }

  if (req.method === 'POST' && p === '/api/leave') {
    const b = await readBody(req);
    if (!b?.employee_no || !db.employees[b.employee_no]) return json(res, 400, { error: 'unknown employee_no' });
    const days = Number(b.days ?? 0);
    if (!(days >= 1 && b.from && b.to)) return json(res, 400, { error: 'from/to/days required (days >= 1)' });
    const balance = db.balances[b.employee_no] ?? 0;
    if (days > balance) return json(res, 400, { error: `insufficient balance: ${days} requested, ${balance} available` });
    if (!hook) hook = await discoverHook();
    const r = await fetch(hook.url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-FlowForge-Token': hook.token },
      body: JSON.stringify({
        entity: `LV-${String(db.seq).padStart(3, '0')} · ${db.employees[b.employee_no].name} · ${b.leave_type} ${days}d`,
        employee_no: b.employee_no, leave_type: b.leave_type, from: b.from, to: b.to, days,
      }),
    });
    if (!r.ok) return json(res, 502, { error: `FlowForge hook failed (${r.status})` });
    const { instanceId } = await r.json();
    const id = `LV-${String(db.seq++).padStart(3, '0')}`;
    db.requests.push({
      id, instanceId, employee_no: b.employee_no, leave_type: b.leave_type,
      from: b.from, to: b.to, days, reason: b.reason ?? '', verdict: null,
      live: { status: 'running', stage: '', error: '' },
      created: new Date().toISOString(),
    });
    notify(b.employee_no, `Leave ${id} submitted (${days}d ${b.leave_type}) — routing to your manager.`);
    save();
    return json(res, 200, db.requests.at(-1));
  }

  const act = p.match(/^\/api\/leave\/(LV-\d+)\/(approve|reject)$/);
  if (req.method === 'POST' && act) {
    const req0 = db.requests.find((r) => r.id === act[1]);
    if (!req0) return json(res, 404, { error: 'unknown request' });
    if (rverdict(req0) !== 'pending') return json(res, 409, { error: `already ${rverdict(req0)}` });
    const who = (await readBody(req))?.approver ?? 'approver';
    const r = act[2] === 'approve'
      ? await ff(`/api/v1/executions/${req0.instanceId}/approve`, { method: 'POST' })
      : await ff(`/api/v1/executions/${req0.instanceId}/cancel`, { method: 'POST' });
    if (!r.ok) return json(res, 502, { error: `FlowForge ${act[2]} failed (${r.status})` });
    if (act[2] === 'reject') {
      req0.verdict = 'rejected';
      notify(req0.employee_no, `Leave ${req0.id} rejected by ${who}.`);
      save();
    }
    return json(res, 200, { ok: true, action: act[2] });
  }

  if (req.method === 'GET' && (p === '/health' || p === '/api/health')) {
    return json(res, 200, { status: 'ok', ff: ffToken ? 'connected' : 'pending' });
  }

  // ---- static page --------------------------------------------------------
  if (req.method === 'GET' && !p.startsWith('/api/')) {
    const file = p === '/' ? 'index.html' : p.slice(1);
    const path = join(WEB_DIST, file);
    if (!existsSync(path)) return json(res, 404, { error: 'build the page: npm --prefix leave-app/web run build' });
    const types = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml' };
    const ext = path.slice(path.lastIndexOf('.'));
    res.writeHead(200, { 'Content-Type': types[ext] ?? 'application/octet-stream' });
    return res.end(await readFile(path));
  }

  json(res, 404, { error: 'not found' });
});

server.listen(LISTEN, () => {
  console.log(`leave app on http://localhost:${LISTEN}  (FlowForge at ${FF_URL})`);
  discoverHook().then((h) => { hook = h; console.log(`hook wired: ${h.url}`); }).catch((e) => console.log(`hook: ${e.message}`));
});
