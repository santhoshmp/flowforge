# FlowForge — Traceability Matrix (Feature → Code → Tests)

> Maps each capability to where it lives in the code and which tests cover it, so future enhancements are easy to locate and safe to change. Keep this in sync with [progress.md](./progress.md) and [test-strategy.md](./test-strategy.md).

| | |
|---|---|
| **Last updated** | 2026-08-22 |
| **Legend** | ✅ done · 🟡 partial · ⬜ planned |

## Backend — `server/`

| ID | Capability | Code | Tests | Status |
|---|---|---|---|---|
| F-EXEC-01 | Durable step-by-step engine (persisted transitions) | `src/engine.ts` → `tickInstance`, `tickAll` | ENG-01..05 | ✅ |
| F-EXEC-02 | Human-task wait + resume | `src/engine.ts` → `approveWaiting`; `routes.ts` `/approve` | ENG-01, ENG-02, API-04 | ✅ |
| F-EXEC-03 | Retry from failed step (no re-run of completed) | `src/engine.ts` → `retryFailed`; `routes.ts` `/retry` | ENG-04 | ✅ |
| F-EXEC-04 | Cancel + `endedAt` | `src/engine.ts` → `cancelInstance`; `routes.ts` `/cancel` | ENG-05 | ✅ |
| F-EXEC-05 | Condition eval + auto-approve from input | `src/engine.ts` (condition branch) | ENG-02, ENG-03 | ✅ |
| F-EXEC-06 | SLA-escalation skip | `src/engine.ts` (`previous_step.sla_breached`) | ENG-02 | ✅ |
| F-EXEC-07 | Scheduler (interval → `tickAll`) | `src/engine.ts` → `startScheduler`; `src/index.ts` | — (covered transitively) | ✅ |
| F-EXEC-08 | Temporal interpreter (HA) | *planned* (`server-go`) | CONF-* | ⬜ P5 |
| F-AI-01 | NL → typed draft (confidence + assumptions) | `src/ai.ts` → `generateDraft`, `authorDeterministic`, `normalizeDraft` | AI-01, AI-02, API-03 | ✅ |
| F-AI-02 | OpenAI-compatible LLM (multi-provider) + fallback | `src/ai.ts` → `authorWithLLM`; `src/settings.ts` | AI-01 (fallback path) | 🟡 (live = AI-03 ⬜) |
| F-AI-03 | Provider config + key masking + test connection | `src/settings.ts`; `routes.ts` `/settings/ai*` | API-06 | ✅ |
| F-DSL-01 | `flowforge/v1` YAML serialization — now via the shared `@flowforge/dsl` package (single contract source) | `server/src/yaml.ts` → `toSpec` + `@flowforge/dsl` `toYAML` | DSL-01 | ✅ |
| F-DSL-02 | **Frozen contract**: JSON Schema + parser + canonical serializer (round-trip); `connector` step type added additively (D1) | `dsl/src/{schema.json,parser.ts,serializer.ts}` (`@flowforge/dsl` v0.2.0) | DSL-01..03 | ✅ |
| F-DSL-03 | Artifact signing: Ed25519 detached `.sig` over the exact artifact bytes; `flowforge keygen/sign/verify` (offline verification) | `server-go/internal/signing` (+`signing_test.go`), `cmd/flowforge` | SIGN-01..03 | ✅ |
| F-API-01 | REST surface (`/api/v1/*`) | `src/routes.ts` → `registerRoutes` | API-01..07 | ✅ |
| F-API-02 | Per-workflow executions + run-with-input | `routes.ts` (`/workflows/:id/executions`) | API-04, API-05, API-07 | ✅ |
| F-DASH-01 | Metrics (fleet, 14-day series, per-workflow) | `src/metrics.ts` → `computeMetrics`; `routes.ts` `/metrics` | MET-01, API-02 | ✅ |
| F-APPROVE-01 | Approve & deploy gate + audit | `routes.ts` `/approve`; `db.ts` audit | API-04, API-05 | ✅ |
| F-MDM-01 | Golden records + add (→ pending stewardship) | `src/seed.ts`; `routes.ts` `/mdm*`; `db.ts` | API-01 (seeded) | ✅ |
| F-CTRL-01 | Step-control registry (CRUD + toggle + usage guard) | `routes.ts` `/controls*`; `db.ts` | API-01 (seeded) | ✅ |
| F-PERSIST-01 | SQLite (durable, restart-safe) | `src/db.ts` → `openDB`, mappers, upserts | all (via `:memory:`) | ✅ |
| F-PERSIST-02 | Postgres backend + migration framework | *planned* | — | ⬜ P5 |
| F-SEC-01 | Built-in auth (bcrypt + HMAC tokens) + first-run admin setup + setup-mode gating | `server-go/internal/auth`, `server-go/internal/store/users.go`, `server-go/internal/api/auth_test.go` | SEC-01 | ✅ |
| F-SEC-02 | Opt-in self-signed TLS (`FLOWFORGE_TLS`) | `server-go/cmd/flowforge` (`selfSignedCert`) | — | ✅ |
| F-SEC-03 | Request policy: safe-mode + egress allow-list | `server-go/internal/policy`, `server-go/internal/policy/policy_test.go` | SEC-02 | ✅ |
| F-SEC-04 | UI auth gate (login + setup screens, token + 401) | `app/src/components/AuthGate.tsx`, `app/src/lib/api.ts` | — | ✅ |
| F-SEC-05 | Sandboxed real execution: Starlark script sandbox + egress-gated HTTP (opt-in via `code`/`url`) | `server-go/internal/executor`, `server-go/internal/engine`, `server-go/internal/executor/executor_test.go` | SEC-03 | ✅ |
| F-SEC-06 | WASM plugin runtime: wazero, memory cap + timeout, egress-gated `ff.http_request` | `server-go/internal/wasm`, `internal/wasm/wasm_test.go` | PLG-01..05 | ✅ |
| F-SEC-07 | Fine-grained RBAC roles / SSO (OIDC/SAML) | *planned* | SEC-05 | ⬜ P5 |
| F-DIST-00 | Go `flowforge/v1` spec (parse/validate/serialize) + CLI | `server-go/internal/spec/spec.go`, `server-go/cmd/flowforge` | CONF-01 | ✅ verified (Go 1.26.5) |
| F-DIST-0a | **Go control plane**: durable engine + SQLite + `/api/v1/*` + embedded UI + `serve` | `server-go/internal/{store,engine,api,ai,settings,metrics,seed,models}`, `server-go/ui` | CONF-02, CONF-03 | ✅ |
| F-DIST-01 | Cross-compile matrix + packaging + SHA256SUMS | `scripts/build.{sh,ps1}` | DIST-01 | ✅ |
| F-DIST-02 | Release pipeline: SBOM (syft SPDX), cosign keyless signing, GitHub Release, multi-arch ghcr image | `.github/workflows/release.yml`, `docs/release.md` | DIST-02 | 🟡 (runs on tag) |
| F-DIST-03 | Container image: multi-stage build (UI → static Go binary → alpine, non-root, /data volume) + compose | `Dockerfile`, `docker-compose.yml`, `.dockerignore` | DIST-03 | 🟡 (CI docker job) |
| F-DIST-04 | Helm chart: single replica + SQLite PVC (Recreate), probes on `/api/v1/health`, ingress, safety values | `chart/flowforge/` | DIST-04 | 🟡 (CI helm job) |
| F-EXT-01 | Step-executor registry (interface + self-registering built-ins, later-wins override) | `server-go/internal/executor` (+`registry_test.go`) | EXT-01, EXT-02 | ✅ |
| F-EXT-02 | Connector SDK: manifests, embedded built-ins + drop-in dirs, params validation, `${params/input/secret/env}` templating, http/smtp/wasm executors, approve-gate validation, redacted dry-run test | `server-go/internal/connectors` (+`connectors_test.go`), `internal/api/ext.go`, CLI `connector validate/test` | CONN-01..05, EXT-03 | ✅ |
| F-EXT-03 | Encrypted secrets vault (AES-256-GCM at rest, names-only API, env-key support) | `server-go/internal/secrets` (+`secrets_test.go`), `internal/api/ext.go` | SEC-04 | ✅ |
| F-EXT-04 | WASM plugin ABI + runtime (`ff` host module) | `server-go/internal/wasm` | PLG-01..05 | ✅ |
| F-EXT-05 | Template gallery: six embedded `flowforge/v1` artifacts + instantiate → draft | `server-go/internal/templates` (+`templates_test.go`), `internal/api/ext.go` | TPL-01, TPL-02 | ✅ |
| F-EXT-06 | CI: dsl (typecheck/test/build) → server (typecheck/test), app (build), go (vet/test/build + template conformance) | `.github/workflows/ci.yml` | (gate) | ✅ |
| F-EXT-07 | OpenAPI contract + versioning policy + quickstart | `docs/{openapi.yaml,versioning.md,quickstart.md}` | — | ✅ |
| F-EXT-08 | Contributor docs for connectors/plugins | `connectors/README.md`, `docs/decisions.md` | — | ✅ |
| F-FACT-01 | Testable server factory + engine hook | `src/app.ts` → `createServer`; `engine.ts` → `tickAll` | (enables all API/ENG tests) | ✅ |

## Frontend — `app/`

| ID | Capability | Code | Status |
|---|---|---|---|
| F-UI-STUDIO | Author → review → approve (canvas, JSON, palette, step panel) | `sections/Studio.tsx`, `components/{FlowCanvas,StepPanel,JsonTree,step}.tsx` | ✅ |
| F-UI-DASH | Tracking dashboard (KPIs, charts, tracker, drill-down) | `sections/Dashboard.tsx` | ✅ |
| F-UI-MONITOR | Live step timeline + approve/retry/cancel | `sections/Monitor.tsx` | ✅ |
| F-UI-WORKFLOWS | List / run / edit / export YAML | `sections/Workflows.tsx` | ✅ |
| F-UI-TESTLAB | Pre-flight checks + sandboxed run | `sections/TestLab.tsx` | 🟡 (client-side sim) |
| F-UI-ADMIN | Queue, audit, controls, AI config, connectors card | `sections/Admin.tsx`, `components/AIConfigCard.tsx` | ✅ |
| F-UI-MDM | Golden-record tables + add | `sections/MDM.tsx` | ✅ |
| F-UI-STORE | Backend-backed store + live polling | `lib/store.tsx`, `lib/api.ts` | ✅ |
| F-UI-DSL | YAML export via `@flowforge/dsl` | `lib/dsl.ts` | ✅ |
| F-UI-TEMPLATES | Template gallery + instantiate → studio | `sections/Home.tsx`, `App.tsx` | ✅ |
| F-UI-NAV | Sidebar nav + routing | `App.tsx` | 🟡 (state nav; router decorative) |

## Docs — `docs/`

| ID | Artifact | Status |
|---|---|---|
| F-DOC-01 | `product-design.md`, `architecture.md`, `build-plan.md` | ✅ |
| F-DOC-02 | `demo-runbook.md` | ✅ |
| F-DOC-03 | `progress.md`, `test-strategy.md`, `traceability.md` | ✅ |

## How to use this matrix

- **Adding a feature:** assign an ID under the right area, point to the file(s), list the covering test IDs, set status, and mirror it in [progress.md](./progress.md).
- **Changing code:** find the row, run its tests first (`npm test -- <pattern>`), keep them green.
- **Porting to Go (P1):** the Go control plane now reproduces the ENG/API/DSL behavior behind `CONF-01..03` (engine + API + spec all green). The Node `server` remains the reference; both implement the same contract.
