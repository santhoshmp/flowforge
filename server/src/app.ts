import 'dotenv/config';
import Fastify, { type FastifyInstance } from 'fastify';
import cors from '@fastify/cors';
import type { DB } from './db.js';
import { registerRoutes } from './routes.js';
import { startScheduler } from './engine.js';

// ---------------------------------------------------------------------------
// Server factory. Separated from index.ts so tests can build an in-process
// app (Fastify .inject) with an in-memory DB and drive the engine via tickAll,
// without binding a port or starting the scheduler.
// ---------------------------------------------------------------------------

export interface ServerOptions {
  schedule?: boolean; // default true
  logger?: boolean; // default true
  cors?: boolean; // default true
}

export async function createServer(d: DB, opts: ServerOptions = {}): Promise<FastifyInstance> {
  const app = Fastify({ logger: opts.logger ?? true });

  if (opts.cors !== false) {
    await app.register(cors, {
      origin: [/^http:\/\/localhost(:\d+)?$/, /^http:\/\/127\.0\.0\.1(:\d+)?$/],
      methods: ['GET', 'POST', 'PATCH', 'DELETE', 'OPTIONS'],
    });
  }

  // Tolerant body parsing: accept any content-type (or none) as JSON, defaulting
  // to {} for empty bodies, so approve/retry/cancel POSTs work from UI and curl.
  app.addContentTypeParser('*', (_req, payload, done) => {
    const chunks: Buffer[] = [];
    payload.on('data', (c: Buffer) => chunks.push(c));
    payload.on('end', () => {
      const raw = Buffer.concat(chunks).toString('utf8').trim();
      if (!raw) return done(null, {});
      try {
        done(null, JSON.parse(raw));
      } catch {
        done(null, {});
      }
    });
    payload.on('error', done);
  });

  registerRoutes(app, d);

  const stopScheduler = opts.schedule !== false ? startScheduler(d) : undefined;
  app.addHook('onClose', () => {
    stopScheduler?.();
  });

  return app;
}
