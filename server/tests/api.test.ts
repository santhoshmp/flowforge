import { describe, it, expect, beforeAll, afterAll } from 'vitest';
import type { FastifyInstance } from 'fastify';
import { createServer } from '../src/app.js';
import { memDB } from './helpers.js';
import { seedIfEmpty } from '../src/seed.js';
import { tickAll } from '../src/engine.js';
import type { DB } from '../src/db.js';

// Scenario IDs: API-01..API-06 (see docs/test-strategy.md). Feature F-API.
// In-process Fastify (.inject) with an in-memory DB and the scheduler OFF —
// the engine is advanced manually via tickAll for determinism.

let app: FastifyInstance;
let d: DB;

beforeAll(async () => {
  d = memDB();
  seedIfEmpty(d);
  app = await createServer(d, { schedule: false, logger: false, cors: false });
});
afterAll(async () => {
  await app.close();
});

function drive(instanceId: string, max = 50): void {
  for (let i = 0; i < max; i++) {
    tickAll(d);
    const c = d.getInstance(instanceId);
    if (c && c.status !== 'running') break;
  }
}

describe('api: control plane contract (API)', () => {
  it('API-01 GET /bootstrap returns seeded collections', async () => {
    const r = await app.inject({ method: 'GET', url: '/api/v1/bootstrap' });
    expect(r.statusCode).toBe(200);
    const b = r.json();
    expect(b.workflows.length).toBeGreaterThan(0);
    expect(b.instances.length).toBeGreaterThan(0);
    expect(b.mdm.length).toBeGreaterThan(0);
    expect(b.controls.length).toBeGreaterThan(0);
  });

  it('API-02 GET /metrics returns fleet, 14-day series, and per-workflow stats', async () => {
    const r = await app.inject({ method: 'GET', url: '/api/v1/metrics' });
    expect(r.statusCode).toBe(200);
    const m = r.json();
    expect(m.fleet.totalRuns).toBeGreaterThan(0);
    expect(m.byDay).toHaveLength(14);
    expect(Array.isArray(m.workflows)).toBe(true);
  });

  it('API-03 POST /ai/draft returns a draft (fallback) without a key', async () => {
    const r = await app.inject({
      method: 'POST', url: '/api/v1/ai/draft',
      payload: { prompt: 'When a support ticket is created, classify it, route to the lead for approval.' },
    });
    expect(r.statusCode).toBe(200);
    const j = r.json();
    expect(j.draft.steps.length).toBeGreaterThan(0);
    expect(j.draft.steps[0].type).toBe('trigger');
  });

  it('API-04 create + approve + run; engine reaches a terminal/waiting state', async () => {
    const list = (await app.inject({ method: 'GET', url: '/api/v1/workflows' })).json();
    const src = list[0];
    const created = (await app.inject({
      method: 'POST', url: '/api/v1/workflows',
      payload: { name: 'API Test', description: 'd', prompt: 'p', steps: src.steps },
    })).json();
    expect(created.status).toBe('draft');
    await app.inject({ method: 'POST', url: `/api/v1/workflows/${created.id}/approve` });
    const run = (await app.inject({
      method: 'POST', url: `/api/v1/workflows/${created.id}/executions`,
      payload: { input: { total: 99999 } },
    })).json();
    expect(run.status).toBe('running');
    drive(run.id);
    const after = d.getInstance(run.id);
    expect(['waiting', 'completed', 'failed']).toContain(after?.status);
  });

  it('API-05 rejects execution of a draft workflow', async () => {
    const created = (await app.inject({
      method: 'POST', url: '/api/v1/workflows',
      payload: { name: 'Draft Only', description: 'd', prompt: 'p', steps: [{ id: 't', type: 'trigger', name: 't', params: {}, confidence: 90, assumptions: [] }] },
    })).json();
    const r = await app.inject({ method: 'POST', url: `/api/v1/workflows/${created.id}/executions` });
    expect(r.statusCode).toBe(400);
  });

  it('API-06 PUT /settings/ai persists and masks the key (never returns the raw key)', async () => {
    // Assembled at runtime so no secret-scanner pattern appears in source.
    const rawKey = ['sk-', 'testkey0123456789abcdef'].join('');
    const r = await app.inject({
      method: 'PUT', url: '/api/v1/settings/ai',
      payload: { provider: 'openai', baseURL: 'https://api.openai.com/v1', model: 'gpt-4o-mini', apiKey: rawKey },
    });
    expect(r.statusCode).toBe(200);
    const j = r.json();
    expect(j.hasKey).toBe(true);
    expect(j.maskedKey).toContain('••••');
    expect(r.body).not.toContain(rawKey);
  });

  it('API-07 GET /workflows/:id/executions lists only that workflow executions', async () => {
    const r = await app.inject({ method: 'GET', url: '/api/v1/workflows/wf-invoice/executions' });
    expect(r.statusCode).toBe(200);
    const arr = r.json();
    expect(Array.isArray(arr)).toBe(true);
    expect(arr.length).toBeGreaterThan(0);
    expect(arr.every((i: { workflowId: string }) => i.workflowId === 'wf-invoice')).toBe(true);
  });
});
