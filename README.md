# FlowForge

**Describe it. Review it. Run it anywhere.**

FlowForge is a downloadable, self-hostable enterprise workflow platform. You describe a business process in plain language, AI drafts the workflow, a human reviews and approves it in a visual editor — and the result runs centrally via API, or downloads as a single portable file that executes anywhere, even air-gapped.

> **Prototype — AI authoring and execution are functional; SSO/RBAC are deferred.**

---

## Quick start

### Option A — Go single binary (recommended)

```bash
# Build the UI into the Go embed
npm --prefix app install && npm --prefix app run build
cp -r app/dist/* server-go/ui/dist/

# Build and run the control plane
cd server-go
go build -o flowforge ./cmd/flowforge
./flowforge serve          # http://localhost:8080
```

On first launch the server is in **setup mode** — create an admin account via the UI setup screen (or `POST /api/v1/auth/setup`), then log in. The embedded Studio UI is served by the same binary.

### Option B — Node reference server + Vite dev UI

```bash
# Terminal 1: control plane
cd server && npm install && npm run dev     # API on :8080

# Terminal 2: Studio UI (hot reload)
cd app && npm install && npm run dev        # UI on :3000
```

### Enable real AI authoring (optional)

From the **Admin → AI authoring model** card in the UI, pick a provider (OpenAI, OpenRouter, Groq, Together, **Ollama** local, **LM Studio** local, or custom), paste a key, **Test connection**, **Save**. Without a key, authoring uses a deterministic local generator.

---

## Key features

| Feature | What it does |
|---|---|
| **Conversational authoring** | Natural-language prompt → typed workflow draft with per-step confidence scores and highlighted assumptions |
| **Human approval gate** | Nothing executes until a named human approves the AI draft — and that approval is on the audit trail |
| **Durable execution** | Step-by-step engine persisted to SQLite; survives restarts; human-task wait/resume; retry-from-failed-step; cancel |
| **Condition evaluation** | Run-time input (e.g. `total: 24000`) drives `condition` steps; below-threshold auto-approves the next human step |
| **Tracking dashboard** | Fleet KPIs, 14-day execution trends, outcome mix, per-workflow tracker with drill-down and inline actions |
| **Step-level observability** | Live step timeline (pending → running → succeeded/failed/waiting) with outputs and durations |
| **Master data management** | Golden-record entities (vendors, customers, products, employees); new records enter as *pending stewardship* |
| **Step-controls registry** | Built-in controls + custom step types (add/disable/remove from the Admin console) |
| **Sandboxed execution** | `script` steps run in a Starlark sandbox (no host fs/net); `integration` HTTP steps are gated by an egress allow-list + safe-mode |
| **Built-in auth** | bcrypt + HMAC session tokens; first-run admin setup; setup-mode gating |
| **Opt-in TLS** | Self-signed HTTPS auto-generated on first run (`FLOWFORGE_TLS=on`) |
| **Portable artifact** | Export any workflow as a signed `flowforge/v1` YAML file |

---

## Architecture

```
EnterpriseWorkflow/
├── app/            React + Vite Studio UI (8 sections + AuthGate)
├── server/         Node/TS control plane — reference implementation (Fastify + SQLite)
├── dsl/            @flowforge/dsl — frozen flowforge/v1 contract (JSON Schema + parser + serializer)
├── server-go/      Go distributable — single-binary control plane
│   ├── cmd/flowforge/     CLI: serve | validate <file> | run <file> | version
│   └── internal/
│       ├── spec/          flowforge/v1 parse/validate/serialize + conformance tests
│       ├── models/        domain types (JSON tags match the UI contract)
│       ├── store/         SQLite (pure-Go modernc driver) schema + CRUD + seed
│       ├── seed/          deterministic, relationship-correct demo dataset
│       ├── engine/        durable execution (TickAll, approve/retry/cancel, conditions)
│       ├── executor/      sandboxed script (Starlark) + egress-gated HTTP
│       ├── ai/            deterministic generator + OpenAI-compatible LLM caller
│       ├── settings/      runtime AI provider config (key masked)
│       ├── policy/        safe-mode + egress allow-list
│       ├── metrics/       dashboard aggregates
│       ├── auth/          bcrypt + HMAC tokens + middleware
│       └── api/           HTTP control plane mirroring /api/v1/*
├── docs/            product-design, architecture, build-plan, progress, test-strategy, traceability, demo-runbook
└── .gitignore
```

The Node `server/` is the **reference implementation** that defined the contract and UX. The Go `server-go/` is the **distributable** that targets the same contract — both pass the same conformance scenarios. The `dsl/` package is the **frozen `flowforge/v1` spec** shared by all three consumers (editor, API, runner).

---

## Configuration (Go server)

| Env var | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP port |
| `DB_PATH` | `flowforge.db` | SQLite database file |
| `FLOWFORGE_AUTH` | `auto` | `auto` (first-run setup then token-required) or `off` |
| `FLOWFORGE_TLS` | `off` | `on` generates a self-signed cert and serves HTTPS |
| `FLOWFORGE_SAFE_MODE` | `off` | `on` disables script + arbitrary-HTTP steps |
| `FLOWFORGE_EGRESS_ALLOW` | *(empty)* | Comma-separated host suffixes; when set, egress defaults to deny |
| `OPENAI_API_KEY` | *(empty)* | For real LLM authoring (any OpenAI-compatible endpoint) |
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` | Override for OpenRouter, Groq, Ollama, etc. |
| `OPENAI_MODEL` | `gpt-4o-mini` | Model name |

---

## Running tests

```bash
# Go control plane (engine, API, auth, sandbox, policy, store, spec, seed consistency)
cd server-go && go test ./...

# Node reference server
cd server && npm test

# flowforge/v1 DSL contract
cd dsl && npm test
```

All suites are green: **Go** (~30 tests), **Node** 16/16, **DSL** 14/14. See [`docs/test-strategy.md`](docs/test-strategy.md) for the scenario catalog (ENG, API, DSL, AI, MET, SEC, DATA).

---

## API surface (`/api/v1/*`)

| Method | Path | Purpose |
|---|---|---|
| GET | `/health` | Server health + model name |
| GET | `/bootstrap` | One-shot load of all collections |
| GET | `/metrics` | Fleet KPIs + 14-day series + per-workflow stats |
| POST | `/ai/draft` | Author a draft from a prompt |
| GET/POST | `/workflows` | List / create |
| GET/PATCH | `/workflows/{id}` | Read / update |
| POST | `/workflows/{id}/approve` | Human approval (required before deploy) |
| GET/POST | `/workflows/{id}/executions` | Per-workflow executions / start a run |
| GET | `/executions` | List all instances |
| GET | `/executions/{id}` / `/steps` | Instance summary / step-level state |
| POST | `/executions/{id}/approve` | Resolve a waiting human task |
| POST | `/executions/{id}/retry` | Resume from the failed step |
| POST | `/executions/{id}/cancel` | Cancel instance |
| GET/POST | `/mdm` / `/mdm/{entity}` | Golden records / add record |
| GET/POST/PATCH/DELETE | `/controls` / `/controls/{key}/toggle` | Step-control registry |
| GET/PUT/POST | `/settings/ai` / `/settings/ai/test` | AI provider config + connection test |
| GET/POST | `/auth/status` / `/auth/setup` / `/auth/login` / `/auth/me` | Auth + first-run setup |
| GET/POST | `/audit` | Audit trail |

---

## Documentation

| Document | For |
|---|---|
| [`docs/demo-runbook.md`](docs/demo-runbook.md) | Running and demoing today |
| [`docs/product-design.md`](docs/product-design.md) | Licensing, packaging, safety, edition split |
| [`docs/architecture.md`](docs/architecture.md) | System architecture + deployment topologies |
| [`docs/build-plan.md`](docs/build-plan.md) | Phased roadmap (P0–P6) |
| [`docs/progress.md`](docs/progress.md) | Current status + changelog |
| [`docs/test-strategy.md`](docs/test-strategy.md) | Test layers + scenario catalog |
| [`docs/traceability.md`](docs/traceability.md) | Feature → code → tests matrix |

---

## Tech stack

| Layer | Technology |
|---|---|
| Distributable binary | **Go 1.26** (stdlib net/http, embed.FS, modernc.org/sqlite) |
| Reference server | **Node.js + TypeScript** (Fastify, better-sqlite3, Vitest) |
| UI | **React 19 + Vite 7** (Tailwind, shadcn/ui, React Flow, Recharts) |
| Contract | **@flowforge/dsl** (JSON Schema, Ajv, yaml) |
| Script sandbox | **Starlark** (go.starlark.net — pure Go, no host I/O) |
| Auth | bcrypt + HMAC-signed session tokens |
| Database | SQLite (embedded, restart-safe) |

---

## Roadmap

| Phase | Status | Focus |
|---|---|---|
| Reference (Node) | ✅ | Contract + UX |
| P0 Foundations | ✅ | DSL frozen, Go spec verified |
| P1 Single binary | ✅ | Engine + SQLite + API + embedded UI + `serve` |
| P2 Safety | ✅ | Auth + first-run + TLS + sandboxing (script + egress) |
| P3 Distribution | ⬜ | Cross-compile, Docker, Helm, signed releases |
| P4 Extensibility | ⬜ | Connector SDK, WASM plugins, templates |
| P5 Enterprise | ⬜ | SSO/SAML, RBAC, Postgres, HA/Temporal |

---

## License

Apache-2.0 — free for everyone.
