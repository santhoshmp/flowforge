import 'dotenv/config';
import { openDB } from './db.js';
import { seedIfEmpty } from './seed.js';
import { createServer } from './app.js';

// ---------------------------------------------------------------------------
// FlowForge control-plane prototype entrypoint. Durable (SQLite), API-first,
// with a real LLM authoring endpoint. No auth in this prototype (single demo
// user). The app is built by createServer() in app.ts (shared with tests).
// ---------------------------------------------------------------------------

const PORT = Number(process.env.PORT || 8080);
const DB_PATH = process.env.DB_PATH || './flowforge.db';

async function main() {
  const d = openDB(DB_PATH);
  seedIfEmpty(d);

  const app = await createServer(d);

  try {
    await app.listen({ port: PORT, host: '0.0.0.0' });
    app.log.info(`FlowForge control plane on http://localhost:${PORT}`);
  } catch (e) {
    app.log.error(e);
    d.close();
    process.exit(1);
  }
}

main();
