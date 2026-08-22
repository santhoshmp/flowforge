# FlowForge — Progress Tracker

> **Living document.** Update on every meaningful change: move items between statuses, log a changelog entry, and add/advance the corresponding test scenarios in [test-strategy.md](./test-strategy.md). This is the single source of truth for *what's done, what's in progress, and what's next*.

| | |
|---|---|
| **Last updated** | 2026-08-22 |
| **Current phase** | **P3 distribution — core shipped** (matrix, Docker, Helm, signing, SBOM pipeline, artifact signing). First tagged release pending |
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
| P3 — Distribution | 🔨 core | Matrix + Docker + Helm + signing + SBOM shipped (2026-08-22); first tagged release pending |
| P4 — Extensibility | ✅ core | Registry, Connector SDK + secrets, WASM runtime, templates, CI, OpenAPI done (2026-08-20). Docs-site polish + app lint cleanup remain |
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

### Extensibility (F-EXT) — P4
- ✅ Step-executor registry (interface + built-in self-registration, later-wins override, dispatch tests EXT-01..03) — `server-go/internal/executor` — P4.1
- ✅ Connector SDK: manifests, embedded built-ins (http-json, slack-webhook, smtp) + drop-in dirs (`FLOWFORGE_CONNECTOR_DIR`), params validation, `${params/input/secret/env}` templating, http/smtp/wasm executors, approve-gate validation, redacted dry-run (`/api/v1/connectors`, CLI `connector validate|test`, Admin UI card) — `server-go/internal/connectors`, `connectors/README.md` — P4.2 (CONN-01..05)
- ✅ Secrets store (AES-256-GCM at rest, env-key support, names-only API) — `server-go/internal/secrets` — P4.2 (SEC-04)
- ✅ WASM plugin runtime — `wazero`, 32 MiB memory cap + 5s timeout, egress-gated `ff.http_request`, `flowforge plugin test` — `server-go/internal/wasm` — P4.3 (PLG-01..05)
- ✅ Templates gallery — six embedded `flowforge/v1` artifacts, `/api/v1/templates` + instantiate, Home "start from a template" — `server-go/internal/templates` — P4.4 (TPL-01/02)
- ✅ OpenAPI (`docs/openapi.yaml`) + versioning policy (D4, `docs/versioning.md`) + quickstart (`docs/quickstart.md`) — P4.5
- ✅ CI (dsl → server/app; go vet/test/build + template conformance) — `.github/workflows/ci.yml` — P4.0
- ⬜ Docs site + 5-min quickstart integration; app lint cleanup (~33 scaffold findings) — P4.5 remainder

### AI configuration (F-AI)
- ✅ Provider selector (incl. Ollama/LM Studio local), key masking, test connection, runtime apply — `server/src/settings.ts`, `app/src/components/AIConfigCard.tsx`
- ⬜ Guardrails, AI governance log, PII redaction — P1/P5

### API (F-API)
- ✅ Full REST surface (`/api/v1/*`) incl. `/metrics`, per-workflow executions, run-with-input — `server/src/routes.ts`

### DSL (F-DSL)
- ✅ **Frozen `flowforge/v1` contract** — shared `dsl/` package (`@flowforge/dsl`): JSON Schema + parser + canonical serializer, 14 tests green (DSL-01/02/03)
- ✅ YAML serialization (reference) — `server/src/yaml.ts`, `app/src/lib/dsl.ts`
- ✅ **Node `server` + `app` consume `@flowforge/dsl`** (single contract source; dsl v0.2.0) — P4.0
- ✅ `connector` step type added additively (decision D1); Go spec updated — P4
- ✅ **Artifact signing** (F-DSL-03): Ed25519 detached `.sig`, `flowforge keygen/sign/verify` — `server-go/internal/signing` — P3 (SIGN-01..03)

### Distributable, Go (F-DIST)
- ✅ **Go control plane (P1 core)**: durable engine (`TickAll`) + SQLite (`modernc`) + REST `/api/v1/*` + **embedded React UI** + `serve` — `server-go/` (verified, runs on :8080)
- ✅ AI authoring (OpenAI-compatible + deterministic fallback), metrics, MDM, controls, settings, run-with-input, condition evaluation
- ✅ `flowforge/v1` spec (parse/validate/serialize) + CLI (`version|validate|run|serve|connectors|connector|plugin|keygen|sign|verify`) + conformance tests
- ✅ P2 safety: auth + first-run + TLS + policy + sandboxed execution (Starlark + egress-gated HTTP) + WASM plugin runtime (P4.3)
- ✅ **Cross-compile matrix** (linux/darwin/win × amd64/arm64) + packaging + SHA256SUMS — `scripts/build.{sh,ps1}`, verified locally (DIST-01)
- ✅ **Docker**: multi-stage image (UI build → static CGO-free binary → alpine non-root, /data volume) + compose with healthcheck + safety toggles — `Dockerfile`, `docker-compose.yml` (CI builds + smokes the container)
- ✅ **Helm chart**: single replica + SQLite PVC (Recreate), probes on `/api/v1/health`, ingress, safety/persistence values — `chart/flowforge/` (CI lints + renders)
- 🔨 **Release pipeline**: tag `v*` → matrix + SBOM (syft SPDX) + cosign keyless signatures + multi-arch ghcr image + GitHub Release — `.github/workflows/release.yml`, runbook `docs/release.md` (first tagged release pending)

### Persistence (F-PERSIST)
- ✅ SQLite (durable, restart-safe) — `server/src/db.ts`
- ⬜ Postgres backend, migration framework — P5

### Security (F-SEC)
- ✅ Built-in auth (bcrypt + HMAC session tokens) with first-run admin setup + setup-mode gating — `server-go/internal/{auth,store}`
- ✅ Opt-in self-signed TLS (`FLOWFORGE_TLS`) — `server-go/cmd/flowforge`
- ✅ Request policy module: safe-mode + egress allow-list (`FLOWFORGE_SAFE_MODE`, `FLOWFORGE_EGRESS_ALLOW`) — `server-go/internal/policy`
- ✅ UI auth gate (login + first-run setup screens, token + 401 handling) — `app/src/components/AuthGate.tsx`, `app/src/lib/api.ts`
- ✅ **Sandboxed real execution**: `script` steps run in a Starlark sandbox (no host fs/net; `load` disabled); `integration` HTTP steps make real calls gated by egress allow-list / safe-mode (opt-in via `code`/`url`, else simulated) — `server-go/internal/executor`, wired in `engine`
- ✅ WASM plugin runtime (`wazero`: memory cap + timeout, egress-gated host fns) — `server-go/internal/wasm` — P4.3 (PLG-01..05)

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
| Go executor registry | `server-go/internal/executor/registry_test.go` | EXT-01/02 | ✅ |
| Go connectors SDK | `server-go/internal/connectors/connectors_test.go` | CONN-01..05 | ✅ |
| Go WASM runtime | `server-go/internal/wasm/wasm_test.go` | PLG-01..05 | ✅ |
| Go secrets | `server-go/internal/secrets/secrets_test.go` | SEC-04 | ✅ |
| Go templates | `server-go/internal/templates/templates_test.go` | TPL-01/02 | ✅ |
| Go ext API | `server-go/internal/api/ext_test.go` | CONN-04, TPL-02, EXT-03, SEC-04 | ✅ |
| Go signing | `server-go/internal/signing/signing_test.go` | SIGN-01..03 | ✅ |

Run: `cd server && npm test` (watch: `npm run test:watch`). Scenario IDs map to [test-strategy.md](./test-strategy.md) and [traceability.md](./traceability.md).

## Changelog

- **2026-08-22** — **P3 distribution core shipped**. Artifact signing (F-DSL-03): Ed25519 detached signatures + `flowforge keygen/sign/verify` CLI (`internal/signing`, SIGN-01..03 green; tamper + wrong-key rejection verified live). Cross-compile matrix via `scripts/build.{sh,ps1}` — all 6 targets (linux/darwin/win × amd64/arm64) + SHA256SUMS verified locally. Docker: multi-stage `Dockerfile` (UI → CGO-free static binary → alpine non-root, /data volume) + `docker-compose.yml` (healthcheck, safety toggles). Helm chart `chart/flowforge` (single replica + SQLite PVC, Recreate, probes on the public `/api/v1/health`, ingress, safety values). Release pipeline `release.yml`: tag `v*` → matrix + syft SBOM + cosign keyless signatures (blobs + multi-arch ghcr image) + GitHub Release; CI gained docker (build + container smoke) and helm (lint + render) jobs. CLI version now ldflags-settable. Docs: `docs/release.md` runbook, README distribution section, stale F-DIST/F-SEC rows fixed. Go suite green (~55, incl. signing). Docker Desktop was down locally — image build verified via the CI job.

- **2026-08-20** — **P4 extensibility core shipped**. Decisions D1–D4 adopted (`docs/decisions.md`). P4.0: CI (`.github/workflows/ci.yml` — the never-done P0 exit), Node `server` + `app` migrated onto `@flowforge/dsl` v0.2.0 (single contract source), doc drift fixed. P4.1: executor registry refactor (`internal/executor`). P4.2: Connector SDK (`internal/connectors` — manifests, embedded built-ins http-json/slack-webhook/smtp, drop-in dirs, params validation, templating, approve-gate validation, redacted dry-run, CLI) + encrypted secrets vault (`internal/secrets`, AES-256-GCM) + `/api/v1/{connectors,secrets}` + Admin UI connectors card. P4.3: WASM plugin runtime (`internal/wasm`, wazero — 32 MiB cap, 5s timeout, egress-gated `ff.http_request`; `plugin test` CLI; PLG tests use hand-crafted wasm binaries, no toolchain). P4.4: template gallery (`internal/templates`, 6 artifacts, instantiate → draft, Home UI). P4.5: `docs/openapi.yaml`, `docs/versioning.md`, `docs/quickstart.md`. DSL gained `connector` step type additively (D1); Go suite ~50 green; Node 16/16; DSL 14/14; live smoke verified (setup → connectors/templates/instantiate/test/secrets/approve-gate). UI rebuilt into the Go embed.
- **2026-08-17** — **P4 planning**: chose extensibility as the next feature set (P3 distribution deferred behind it; still gates public beta). Detailed Phase 4 into workstreams 4.0–4.5 + blocking decisions D1–D4 in [build-plan.md](./build-plan.md); added feature group F-EXT; rewrote "Next up". No code changes.

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

1. **First tagged release** (`v0.1.0`): tag → verify the release pipeline end-to-end (matrix, SBOM, cosign, ghcr image, GitHub Release) per `docs/release.md`; publish the SHA256SUMS.
2. **CI dry-run**: confirm the new docker (build + smoke) and helm (lint + render) jobs pass on GitHub before tagging.
3. **P4 remainder (small):** docs site + quickstart integration; app lint cleanup (~33 scaffold findings, react-hooks v7 rules).
4. **Community hardening:** "write your first plugin" guide with a real TinyGo/Rust example module; connector contract-test harness as a reusable `go test` helper.
5. Later: P5 enterprise (SSO/RBAC, Postgres, HA/Temporal) per [build-plan.md](./build-plan.md).
