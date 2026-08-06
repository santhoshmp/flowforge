# FlowForge — Progress Tracker

> **Living document.** Update on every meaningful change: move items between statuses, log a changelog entry, and add/advance the corresponding test scenarios in [test-strategy.md](./test-strategy.md). This is the single source of truth for *what's done, what's in progress, and what's next*.

| | |
|---|---|
| **Last updated** | 2026-08-01 |
| **Current phase** | Prototype complete → entering **P0/P1** (distributable) |
| **Test status** | 16 automated server tests, all green (`npm test`) |
| **Related** | [test-strategy.md](./test-strategy.md) · [traceability.md](./traceability.md) · [build-plan.md](./build-plan.md) |

## Status legend

- ✅ **Done** — implemented and tested (in the current reference build).
- 🟡 **Partial** — core works; gaps remain.
- 🔨 **In progress** — actively being built.
- ⬜ **Planned** — not started (see build-plan phases).

## Phase status (see [build-plan.md](./build-plan.md))

| Phase | Status | Notes |
|---|---|---|
| Reference implementation (Node/TS server + React UI) | ✅ | Defines the contract & UX |
| P0 — Foundations & decisions | 🔨 | License/trademark, Go vs Node, DSL freeze |
| P1 — Single-binary MVP (Go) | ✅ core | Control plane runs (`serve`): engine + SQLite + API + embedded UI. P2 safety remains |
| P2 — Safety & first-run | ✅ | Auth + first-run + TLS + policy + sandboxed real execution (script sandbox + egress-gated HTTP). WASM plugin format deferred to P4 |
| P3 — Distribution | ⬜ | Cross-compile, Docker, Helm, signing |
| P4 — Extensibility | ⬜ | Connector SDK, plugins, templates |
| P5 — Enterprise edition | ⬜ | SSO, RBAC, Postgres, HA/Temporal |
| P6 — Beta → GA | ⬜ | Audit, telemetry, marketplace |

## Feature checklist

### Authoring (F-AUTH)
- ✅ NL prompt → typed draft (confidence + assumptions) — `server/src/ai.ts`, `app/src/sections/Studio.tsx`
- ✅ Canvas editor, palette, step panel, JSON view — `app/src/components/{FlowCanvas,StepPanel,JsonTree,step}.tsx`

### Approval & trust (F-APPROVE)
- ✅ Approve & deploy gate (nothing runs unapproved) — `server/src/routes.ts` (`/approve`)
- ✅ Append-only audit trail — `server/src/db.ts`, `routes.ts`

### Execution engine (F-EXEC)
- ✅ Durable step-by-step engine (persisted transitions) — `server/src/engine.ts`
- ✅ Human-task wait/resume, retry-from-failed, cancel — `engine.ts`
- ✅ Condition evaluation + auto-approve from run input — `engine.ts`
- ✅ SLA-escalation skip, ISO timestamps, `endedAt` — `engine.ts`
- ⬜ Temporal interpreter (enterprise HA) — P5

### Tracking dashboard (F-DASH)
- ✅ KPIs, 14-day trend, outcome mix, step performance — `server/src/metrics.ts`, `app/src/sections/Dashboard.tsx`
- ✅ Workflow tracker + drill-down + inline actions — `Dashboard.tsx`

### Master data (F-MDM)
- ✅ Golden records, add record (→ pending stewardship) — `server/src/seed.ts`, `routes.ts`, `app/src/sections/MDM.tsx`
- ⬜ Match/merge engine, sync connectors — P2/P5

### Step controls (F-CTRL)
- ✅ Registry, custom CRUD, enable/disable, usage guard — `routes.ts`, `app/src/sections/Admin.tsx`

### AI configuration (F-AI)
- ✅ Provider selector (incl. Ollama/LM Studio local), key masking, test connection, runtime apply — `server/src/settings.ts`, `app/src/components/AIConfigCard.tsx`
- ⬜ Guardrails, AI governance log, PII redaction — P1/P5

### API (F-API)
- ✅ Full REST surface (`/api/v1/*`) incl. `/metrics`, per-workflow executions, run-with-input — `server/src/routes.ts`

### DSL (F-DSL)
- ✅ **Frozen `flowforge/v1` contract** — shared `dsl/` package (`@flowforge/dsl`): JSON Schema + parser + canonical serializer, 14 tests green (DSL-01/02/03)
- ✅ YAML serialization (reference) — `server/src/yaml.ts`, `app/src/lib/dsl.ts`
- ⬜ Migrate server/app to consume `@flowforge/dsl`; artifact signing — P1

### Distributable, Go (F-DIST)
- ✅ **Go control plane (P1 core)**: durable engine (`TickAll`) + SQLite (`modernc`) + REST `/api/v1/*` + **embedded React UI** + `serve` — `server-go/` (verified, runs on :8080)
- ✅ AI authoring (OpenAI-compatible + deterministic fallback), metrics, MDM, controls, settings, run-with-input, condition evaluation
- ✅ `flowforge/v1` spec (parse/validate/serialize) + CLI (`version|validate|run|serve`) + conformance tests
- ⬜ Sandboxing (WASM) + egress control + first-run wizard + auth/TLS — P2
- ⬜ Cross-compile matrix, Docker, Helm, signed releases — P3

### Persistence (F-PERSIST)
- ✅ SQLite (durable, restart-safe) — `server/src/db.ts`
- ⬜ Postgres backend, migration framework — P5

### Security (F-SEC)
- ✅ Built-in auth (bcrypt + HMAC session tokens) with first-run admin setup + setup-mode gating — `server-go/internal/{auth,store}`
- ✅ Opt-in self-signed TLS (`FLOWFORGE_TLS`) — `server-go/cmd/flowforge`
- ✅ Request policy module: safe-mode + egress allow-list (`FLOWFORGE_SAFE_MODE`, `FLOWFORGE_EGRESS_ALLOW`) — `server-go/internal/policy`
- ✅ UI auth gate (login + first-run setup screens, token + 401 handling) — `app/src/components/AuthGate.tsx`, `app/src/lib/api.ts`
- ✅ **Sandboxed real execution**: `script` steps run in a Starlark sandbox (no host fs/net; `load` disabled); `integration` HTTP steps make real calls gated by egress allow-list / safe-mode (opt-in via `code`/`url`, else simulated) — `server-go/internal/executor`, wired in `engine`
- ⬜ WASM as a packaged plugin/connector format (Starlark covers inline scripts today) — P4

### Documentation (F-DOCS)
- ✅ product-design, architecture, build-plan, demo-runbook, progress, test-strategy, traceability — `docs/`

## Test status

| Suite | File | Scenarios | Status |
|---|---|---|---|
| Engine lifecycle | `server/tests/engine.test.ts` | ENG-01..05 | ✅ 5/5 |
| Metrics | `server/tests/metrics.test.ts` | MET-01 | ✅ 1/1 |
| AI authoring | `server/tests/ai.test.ts` | AI-01..02 | ✅ 2/2 |
| DSL serialization | `server/tests/yaml.test.ts` | DSL-01 | ✅ 1/1 |
| API contract | `server/tests/api.test.ts` | API-01..07 | ✅ 7/7 |
| **DSL contract** | `dsl/tests/dsl.test.ts` | DSL-01..03 | ✅ 14/14 |
| Go conformance | `server-go/internal/spec/spec_test.go` | DSL-02/03 (CONF-01) | ✅ verified (Go 1.26.5) |
| Go engine | `server-go/internal/engine/engine_test.go` | ENG-01..05 (CONF-02) | ✅ 5/5 |
| Go API | `server-go/internal/api/api_test.go` | API-01..06 (CONF-03) | ✅ 6/6 |
| Go store | `server-go/internal/store/store_test.go` | seed + round-trip | ✅ |
| Go auth | `server-go/internal/api/auth_test.go` | SEC-01 (setup/login/protected) | ✅ |
| Go policy | `server-go/internal/policy/policy_test.go` | SEC-02 (egress/safe-mode) | ✅ |
| Go executor | `server-go/internal/executor/executor_test.go` | SEC-03 (script sandbox + egress) | ✅ |

Run: `cd server && npm test` (watch: `npm run test:watch`). Scenario IDs map to [test-strategy.md](./test-strategy.md) and [traceability.md](./traceability.md).

## Changelog

- **2026-08-04** — **P2 sandboxing (real execution)**: `script` steps now run in a Starlark sandbox (pure-Go, no host fs/net, `load` disabled) and `integration` HTTP steps make real calls — both opt-in (`code`/`url`) and gated by the policy module (safe-mode + egress allow-list); failures halt the instance. Verified live: a script computed from input while a non-allow-listed HTTP call was **blocked by egress policy** (instance failed). Tests green (executor sandbox + egress + safe-mode; full Go suite). WASM reserved for P4 packaged plugins.
- **2026-08-03** — **P2 safety & first-run (partial)** on the Go control plane: built-in auth (bcrypt + HMAC session tokens) with first-run admin setup + setup-mode gating; opt-in self-signed TLS; request policy module (safe-mode + egress allow-list). UI gained an auth gate (login + setup screens, bearer token, 401 handling). Verified end-to-end (403 setup-lock → setup → 401 without token → 200 with token). Tests green (auth flow, policy, full Go suite; Node 16/16; DSL 14/14). WASM sandboxing of real execution remains (engine simulates today; policy hooks in place).
- **2026-08-02** — Go control plane (P1 core) complete and verified: durable engine (`TickAll`), SQLite (`modernc`), full `/api/v1/*` REST API, OpenAI-compatible AI authoring (+fallback), metrics, MDM, controls, settings, and the **embedded React UI** — all in one binary (`flowforge serve` on :8080). Tests green (engine 5/5, API 6/6, store, spec). End-to-end lifecycle verified (create → approve → run → wait → approve → complete; escalation skips). UI built and copied into the embed.
- **2026-08-02 (earlier)** — Installed Go 1.26.5 (user-local toolchain). `server-go` spec skeleton **verified green** (`go test ./...`); CLI builds and runs (`version|validate|run`). `go.sum` added.
- **2026-08-01 (2)** — Froze `flowforge/v1` as a shared, tested package (`dsl/`, `@flowforge/dsl`): JSON Schema + parser + canonical serializer, 14 tests green (DSL-01/02/03). Started the Go distributable skeleton (`server-go/`): spec parse/validate/serialize + CLI (`version|validate|run`) + conformance tests.
- **2026-08-01** — Added Vitest suite (16 tests) + test factory (`createServer`) + `tickAll` engine hook; added Dashboard (KPIs/charts/tracker), `metrics` + per-workflow executions APIs, condition evaluation, rich seed (6 workflows / ~37 runs / 14-day series), AI provider config, ISO timestamps. Authored product-design / architecture / build-plan docs.
- *(earlier)* — Built Node/TS control plane (Fastify + SQLite), durable engine, LLM authoring (OpenAI-compatible + fallback), MDM, controls, audit; wired React UI to the backend.

## How to update this doc

1. When you merge a change, flip the relevant feature status and add a one-line **changelog** entry (date + summary).
2. If you add/changed behavior, add or update a scenario in [test-strategy.md](./test-strategy.md) and a row in [traceability.md](./traceability.md).
3. Keep "Next up" honest — it drives the next session.

## Next up

1. **P2 safety**: sandbox `script`/connector execution (WASM), default-deny egress, first-run wizard, built-in auth + self-signed TLS.
2. **P3 distribution**: cross-compile matrix (linux/darwin/win × amd64/arm64), Docker image, Helm chart, signed releases + SBOM.
3. Migrate the Node `server`/`app` to consume `@flowforge/dsl` (single contract source of truth).
4. Artifact signing for `.flow.yaml` (F-DSL-03).
