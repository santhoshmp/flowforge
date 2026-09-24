// Leave app — Phase 1 stub service (zero dependencies, Node 18+).
//
// Doubles as the seed of the Phase 2 service:
//   GET  /api/balance/:empNo       <- called by the balance-check connector
//                                    200 {ok,balance} | 409 {error,balance}
//   POST /api/flowforge/callback   <- called by the leave-callback connector
//                                    (approved verdict: deduct + record)
//   GET  /api/stub/requests        <- test introspection: what arrived
//   GET  /api/stub/balances        <- test introspection
//
// Run: node leave-app/stub/server.mjs   (LISTEN default 9090)

import { createServer } from 'node:http';
import { randomBytes } from 'node:crypto';

const LISTEN = Number(process.env.LISTEN ?? 9090);
const TOKEN = process.env.LEAVE_TOKEN ?? 'dev-leave-token';

// In-memory balances (source of truth lives in the leave app by design).
const balances = new Map([
  ['E-4003', 18], // Sofia — plenty
  ['E-4007', 12], // Elena
  ['E-4417', 2],  // Jonas — tight (drives the insufficient-balance path)
]);

const requests = []; // callbacks received from FlowForge

const server = createServer((req, res) => {
  const json = (code, body) => {
    res.writeHead(code, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify(body));
  };
  const url = new URL(req.url, `http://localhost:${LISTEN}`);

  // Shared-secret gate (the connectors send X-Leave-Token).
  if (url.pathname.startsWith('/api/balance') || url.pathname === '/api/flowforge/callback') {
    if (req.headers['x-leave-token'] !== TOKEN) {
      return json(401, { error: 'invalid leave-app token' });
    }
  }

  if (req.method === 'GET' && url.pathname.startsWith('/api/balance/')) {
    const emp = url.pathname.split('/').pop();
    if (!balances.has(emp)) return json(404, { error: `unknown employee ${emp}` });
    const balance = balances.get(emp);
    const days = Number(url.searchParams.get('days') ?? 0);
    if (days > balance) {
      return json(409, { ok: false, error: `insufficient balance: ${days} requested, ${balance} available`, balance });
    }
    return json(200, { ok: true, employee_no: emp, balance });
  }

  if (req.method === 'POST' && url.pathname === '/api/flowforge/callback') {
    let body = '';
    req.on('data', (c) => (body += c));
    req.on('end', () => {
      let payload;
      try { payload = JSON.parse(body); } catch { return json(400, { error: 'bad json' }); }
      if (payload.verdict === 'approved') {
        const emp = String(payload.employee_no);
        const days = Number(payload.days);
        const bal = balances.get(emp);
        if (bal == null) return json(404, { error: `unknown employee ${emp}` });
        if (days > bal) return json(409, { error: 'balance changed since approval', balance: bal });
        balances.set(emp, bal - days); // deduct ONLY on the approved callback
      }
      const rec = { at: new Date().toISOString(), payload };
      requests.push(rec);
      console.log(`[callback] ${JSON.stringify(rec)}`);
      return json(200, { received: true });
    });
    return;
  }

  if (req.method === 'GET' && url.pathname === '/api/stub/requests') return json(200, requests);
  if (req.method === 'GET' && url.pathname === '/api/stub/balances') return json(200, Object.fromEntries(balances));
  if (req.method === 'GET' && url.pathname === '/health') return json(200, { status: 'ok', token: TOKEN.slice(0, 4) + '…' });

  json(404, { error: 'not found' });
});

server.listen(LISTEN, () => {
  console.log(`leave-app stub on http://localhost:${LISTEN} (token ${TOKEN.slice(0, 4)}…)`);
  if (process.env.LEAVE_TOKEN === undefined) {
    console.log(`note: using dev token; FlowForge secret CALLBACK_TOKEN must be "${TOKEN}"`);
  }
});
void randomBytes; // reserved for Phase 2 session ids
