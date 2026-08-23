# FlowForge — Test Strategy

> How we prove FlowForge works, and how the suite grows with the product. Every behavioral change should add or update a scenario here and a row in [traceability.md](./traceability.md).

| | |
|---|---|
| **Last updated** | 2026-08-22 (product-wide suite: STORE/AUTH/E2E families, deep ENG/CONN/PLG/API/TPL) |
| **Runner** | Vitest (server, dsl); Go testing (server-go, incl. `e2e` binary suite); CI: `.github/workflows/ci.yml` |
| **Run** | `cd server && npm test` · `cd dsl && npm test` · `cd server-go && go test ./...` (e2e included, ~25s) |
| **Related** | [progress.md](./progress.md) · [traceability.md](./traceability.md) |

## 1. Principles

1. **Contract-first.** The `flowforge/v1` DSL and the REST surface are the contracts; tests pin them so refactors (incl. the Node→Go port) can't silently change behavior.
2. **Deterministic by default.** Engine and API tests run with the scheduler **off** and advance state via `tickAll` — no timers, no flakiness. Live-LLM and network behavior is isolated in separate, opt-in tests.
3. **Two engines, one suite.** The Go distributable must pass the **same conformance scenarios** as the Node reference (the scenario IDs below are the shared gate).
4. **Every scenario has an ID** (e.g. `ENG-02`) used in the test file, this doc, and the traceability matrix.

## 2. Test layers

| Layer | What | Where | Status |
|---|---|---|---|
| **Unit** | Pure functions (DSL serialize, AI normalization, metrics; Go spec/policy/signing/manifests) | `server/tests/{yaml,ai,metrics}.test.ts`, `dsl/tests/dsl.test.ts`, `server-go/internal/*` | ✅ |
| **Persistence** | Every collection roundtrip (SQLite, nested JSON columns) | `server-go/internal/store/store_crud_test.go` | ✅ |
| **Engine** | Step-by-step execution semantics (in-memory DB, `TickAll`) | `server/tests/engine.test.ts`, `server-go/internal/engine/` | ✅ |
| **API contract** | HTTP behavior (Node `.inject`; Go `httptest`) — full REST surface + lifecycles | `server/tests/api.test.ts`, `server-go/internal/api/` | ✅ |
| **Conformance (DSL)** | Round-trip parse/serialize against a corpus (TS + Go parity) | `dsl/tests/dsl.test.ts`, `server-go/internal/spec` | ✅ DSL-02/03 |
| **Safety** | Sandboxing, egress denial, resource limits, token integrity, secrets | `server-go/internal/{executor,wasm,auth,secrets,policy}` | ✅ SEC-*/PLG-* |
| **Binary E2E** | The built product: CLI suite + live `serve` full loop + restart durability | `server-go/e2e/e2e_test.go` | ✅ E2E-01..07 |
| **Live / integration** | Real LLM (opt-in, gated by env) | *planned* | ⬜ AI-03 |
| **Browser UI E2E** | Studio → approve → run → track (Playwright) | *planned* | ⬜ UX-01 |
| **Distribution** | Image build + container smoke + chart lint (CI); signed release on tag | `.github/workflows/{ci,release}.yml` | 🟡 DIST-01..04 |

## 3. Testability hooks in the code

- `createServer(d, { schedule, logger, cors })` (`server/src/app.ts`) — builds the app without binding a port; tests use `.inject()`.
- `openDB(':memory:')` / `store.Open(":memory:")` — ephemeral DB per test (Node / Go).
- `tickAll(d)` / `engine.TickAll(s, pol)` — one synchronous engine transition; the scheduler calls this on an interval, tests call it directly.
- `tests/helpers.ts` (Node) — `memDB`, `sampleWorkflow`, `newRun`, `drive`/`drain`.
- Go API tests drive the engine behind the live server (`driveInst` in `lifecycle_test.go`) — same transitions the scheduler performs.
- `server-go/e2e` builds the real binary once in `TestMain`, then exercises CLI + `serve` (free port, temp DB dir) including a kill-and-restart durability check.

## 4. Scenario catalog

### Engine (F-EXEC) — `engine.test.ts`
| ID | Scenario | Automated |
|---|---|---|
| ENG-01 | Runs to a human approval and **waits** when above threshold | ✅ |
| ENG-02 | **Completes** after a waiting task is approved; escalation **skips** | ✅ |
| ENG-03 | **Auto-approves** the manager step when condition is below threshold | ✅ |
| ENG-04 | **Retries from the failed step** without re-running completed steps | ✅ |
| ENG-05 | **Cancels** a running instance and records `endedAt` | ✅ |
| ENG-06 | Configured `script` step runs for real through the engine (output + duration) | ✅ (`engine_exec_test.go`) |
| ENG-07 | Broken script fails the instance with the error surfaced | ✅ |
| ENG-08 | Connector step runs egress-gated; flaky upstream fails → **retry succeeds** (only the failed step re-runs) | ✅ |
| ENG-09 | Safe-mode disables real execution (script + connector) | ✅ |
| ENG-10 | Cancel a **waiting** instance; no further steps run afterward | ✅ |

### Metrics / Dashboard (F-DASH) — `metrics.test.ts`
| ID | Scenario | Automated |
|---|---|---|
| MET-01 | Fleet counts, 14-day series, per-workflow breakdown | ✅ |

### AI authoring (F-AI) — `ai.test.ts`
| ID | Scenario | Automated |
|---|---|---|
| AI-01 | Deterministic, well-formed fallback draft (one trigger, valid types, confidence range) | ✅ |
| AI-02 | Infers condition + human approval from the prompt | ✅ |
| AI-03 | Live LLM produces a schema-valid draft (gated by `OPENAI_API_KEY`/provider) | ⬜ manual/opt-in |

### DSL (F-DSL) — `dsl/tests/dsl.test.ts` (canonical) + `server/tests/yaml.test.ts`
| ID | Scenario | Automated |
|---|---|---|
| DSL-01 | Serializes a workflow to a `flowforge/v1` document | ✅ |
| DSL-02 | Round-trips a corpus of YAML ⇄ object (serialize → parse == original) | ✅ (`dsl/`) |
| DSL-03 | Rejects an invalid spec against the JSON Schema (8 cases) | ✅ (`dsl/`) |

### API contract (F-API) — `api.test.ts`
| ID | Scenario | Automated |
|---|---|---|
| API-01 | `GET /bootstrap` returns seeded collections | ✅ |
| API-02 | `GET /metrics` returns fleet + 14-day series + per-workflow | ✅ |
| API-03 | `POST /ai/draft` returns a draft (fallback) without a key | ✅ |
| API-04 | create + approve + run; engine reaches a terminal/waiting state | ✅ |
| API-05 | Rejects execution of a **draft** workflow | ✅ |
| API-06 | `PUT /settings/ai` persists and **masks** the key (raw key never returned) | ✅ |
| API-07 | `GET /workflows/:id/executions` lists only that workflow's runs | ✅ |
| API-08 | `PATCH /workflows/{id}` updates selectively; 404 on unknown | ✅ (`api_surface_test.go`) |
| API-09 | MDM entity fetch + record add (pending stewardship) + 404s | ✅ |
| API-10 | Controls CRUD surface: create w/ defaults, dup/bad-key 400s, patch, toggle ×2, delete + built-in/in-use guards | ✅ |
| API-11 | Audit add defaults (actor/action/kind) + newest-first list | ✅ |
| API-12 | AI settings get/put masked + `/test` result envelope | ✅ |
| API-13 | Executions: list/get/steps/404s, run-with-input persisted, per-workflow filter, cancel, retry no-op | ✅ |
| API-14 | Logout contract (200 + ok) | ✅ |
| API-15 | **Full lifecycle over REST**: template → edit → draft-run refused → approve → run w/ input → WAIT → task approve → COMPLETE + audit trail | ✅ (`lifecycle_test.go`) |
| API-16 | Script failure surfaces; retry stays failed until fixed; below-threshold auto-approve completes without waiting | ✅ |

### Extensibility (F-EXT) — P4 scenarios
| ID | Scenario | Automated |
|---|---|---|
| EXT-01 | Executor registry dispatch: script/integration real, unhandled types simulate | ✅ (`server-go/internal/executor/registry_test.go`) |
| EXT-02 | Custom executor registration + later-wins override | ✅ (`registry_test.go`) |
| EXT-03 | Engine runs `connector` steps for real via the registry (deny-listed target fails the instance with the policy error) | ✅ (`server-go/internal/api/ext_test.go`) |
| CONN-01 | Built-in manifests validate; bad manifests reject with actionable errors | ✅ (`server-go/internal/connectors/connectors_test.go`) |
| CONN-02 | User drop-in dirs load and override built-ins by id | ✅ (`connectors_test.go`) |
| CONN-03 | Params validation (required, string) + unresolved `${secret.*}` refs error | ✅ (`connectors_test.go`) |
| CONN-04 | Approve gate: unknown connector / missing params → 400; valid → deployed | ✅ (`server-go/internal/api/ext_test.go`) |
| CONN-05 | http connector executes egress-gated; wasm connector runs its module | ✅ (`connectors_test.go`) |
| CONN-06 | Auth modes (bearer/custom-header/basic) reach the request; secrets from the vault; missing secret fails **before egress** | ✅ (`auth_modes_test.go`) |
| CONN-07 | Preview warns on unresolved refs and never exposes resolved secret values | ✅ |
| PLG-01 | WASM plugin publishes a JSON result through the `ff` ABI | ✅ (`server-go/internal/wasm/wasm_test.go`) |
| PLG-02 | Memory cap enforced at instantiation | ✅ (`wasm_test.go`) |
| PLG-03 | Execution timeout interrupts a spinning plugin | ✅ (`wasm_test.go`) |
| PLG-04 | `ff.http_request` blocked by egress policy (no call); allowed reaches the server | ✅ (`wasm_test.go`) |
| PLG-05 | Safe-mode blocks plugin HTTP | ✅ (`wasm_test.go`) |
| PLG-06 | `ff.log` captures lines surfaced with the result | ✅ (`wasm_host_test.go`) |
| PLG-07 | Garbage/nil/truncated modules rejected at instantiation | ✅ |
| PLG-08 | Missing required exports rejected **by name** | ✅ |
| PLG-09 | Hostile `alloc` (out-of-range pointer) refused, no crash | ✅ |
| PLG-10 | Non-zero exit code surfaces as a failure with the code | ✅ |
| TPL-01 | Gallery templates parse + validate against the frozen DSL; traversal rejected | ✅ (`server-go/internal/templates/templates_test.go`) |
| TPL-02 | Instantiate creates a draft (trigger + steps, string params, uniqued ids) | ✅ (`templates_test.go`, `ext_test.go`) |
| TPL-03 | **Every** gallery entry instantiates (loop): draft/trigger/order/unique ids/categories | ✅ (`gallery_loop_test.go`) |

### Persistence (F-PERSIST) — P4/P5 suite
| ID | Scenario | Automated |
|---|---|---|
| STORE-01 | Workflow upsert → get → update (nested steps/params/assumptions JSON roundtrip; no duplicates) | ✅ (`store_crud_test.go`) |
| STORE-02 | Instance roundtrip: step runs, nested input, terminal fields (error/endedAt) | ✅ |
| STORE-03 | Controls CRUD + MDM upsert records + settings kv + audit append-only | ✅ |
| STORE-04 | Users: add/count/username lookup/UNIQUE duplicate rejected | ✅ |

### Binary E2E (product) — `server-go/e2e/e2e_test.go`
| ID | Scenario | Automated |
|---|---|---|
| E2E-01 | `flowforge version` prints the stamp | ✅ |
| E2E-02 | `validate` accepts a gallery artifact / rejects a broken one; `run` prints the plan | ✅ |
| E2E-03 | `connectors` lists built-ins; `connector validate` checks a drop-in dir | ✅ |
| E2E-04 | `keygen`/`sign`/`verify` roundtrip; keygen refuses overwrite; tamper rejected | ✅ |
| E2E-05 | Live `serve`: setup lock (403+setupRequired) → admin setup → 409 dup → 401/200 gating → /auth/me | ✅ |
| E2E-06 | Embedded UI: index served + SPA fallback | ✅ |
| E2E-07 | **Full loop on a live server with the real scheduler**: connectors/templates/secrets over HTTP → template → approve → run → WAIT (scheduler advances) → task approve → COMPLETE → **restart on the same DB survives** | ✅ |

### Artifact signing (F-DSL-03) — P3
| ID | Scenario | Automated |
|---|---|---|
| SIGN-01 | keygen → sign → verify roundtrip via key files + `.sig` sibling | ✅ (`server-go/internal/signing/signing_test.go`) |
| SIGN-02 | Tampered artifact fails verify; missing signature is an explicit error | ✅ (`signing_test.go`) |
| SIGN-03 | Signature from a different key does not verify | ✅ (`signing_test.go`) |

### Planned (placeholders for future phases)
| ID | Scenario | Phase |
|---|---|---|
| SEC-01 | Auth flow: setup-mode lock → setup → 401 without token → 200 with token → /auth/me | ✅ (`server-go/internal/api/auth_test.go`) |
| SEC-01b | Token tamper rejection (forged claims, flipped signature, garbage, cross-server) | ✅ (`server-go/internal/auth/auth_test.go`) |
| SEC-01c | Expired tokens rejected; future-exp control verifies | ✅ |
| SEC-01d | Wrap gating matrix: off/auto, public paths, missing/invalid/valid token | ✅ |
| SEC-01e | Credential rules + bcrypt verify/no-plaintext | ✅ |
| SEC-02 | Egress allow-list (default-deny, suffix match) + safe-mode disables scripts | ✅ (`server-go/internal/policy/policy_test.go`) |
| SEC-03 | Script sandbox (no host fs/net) + input transform; HTTP egress allow/deny + safe-mode | ✅ (`server-go/internal/executor/executor_test.go`) |
| SEC-04 | Secrets vault: encrypted at rest, names-only reads, wrong-key rejection | ✅ (`server-go/internal/secrets/secrets_test.go`) |
| SEC-05 | Fine-grained RBAC roles / SSO | ⬜ P5 |
| DIST-01 | Cross-compile matrix (linux/darwin/win × amd64/arm64) builds + checksums | ✅ local (`scripts/build.ps1` verified 2026-08-22) + release workflow |
| DIST-02 | Release artifacts signed (cosign keyless) with an SBOM; image on ghcr signed | 🟡 pipeline (`release.yml`; runs on tag) |
| DIST-03 | Docker image builds and serves `/api/v1/health` | 🟡 CI docker job (build + container smoke) |
| DIST-04 | Helm chart lints and renders (default + ingress/no-persistence variants) | 🟡 CI helm job |
| UX-01 | Playwright: describe → approve → run → approve-task → completed | P1/P4 |
| CONF-01 | Go `spec` round-trips + rejects identically (mirror of DSL-02/03) | ✅ (`server-go/internal/spec`, Go 1.26.5) |
| CONF-02 | Go engine mirrors ENG-01..05 (wait/approve/retry/cancel/auto-approve) | ✅ (`server-go/internal/engine`) |
| CONF-03 | Go API mirrors API-01..06 (bootstrap/metrics/ai/run/settings masking) | ✅ (`server-go/internal/api`) |

## 5. Coverage goals

- Every public REST endpoint has at least one API-* test. ✅ (API-01..14 + E2E-05..07)
- Every step resolution branch (succeed / wait / skip / auto-approve / fail→retry) has an ENG-* test. ✅ (ENG-01..10)
- The DSL conformance corpus covers all step types and param shapes (DSL-02). ✅
- No test depends on the network unless it spins up its own `httptest` server or is explicitly opt-in (AI-03).
- The shipped binary passes the E2E suite before any release tag (part of the release checklist).

## 6. Adding a test

1. Pick or create a scenario ID (area prefix: `ENG`, `API`, `DSL`, `AI`, `MET`, `SEC`, `STORE`, `EXT`, `CONN`, `PLG`, `TPL`, `SIGN`, `E2E`, `DIST`, `UX`, `CONF`).
2. Write it where the behavior lives — Node reference in `server/tests/*.test.ts`, product behavior in the matching `server-go/internal/*` package, whole-product flows in `server-go/e2e`. Use the helpers, keep it deterministic.
3. Add a row to the **scenario catalog** above and to [traceability.md](./traceability.md).
4. `go test ./...` (server-go), `npm test` (server, dsl) must stay green before merge.
