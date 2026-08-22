# FlowForge — Test Strategy

> How we prove FlowForge works, and how the suite grows with the product. Every behavioral change should add or update a scenario here and a row in [traceability.md](./traceability.md).

| | |
|---|---|
| **Last updated** | 2026-08-22 |
| **Runner** | Vitest (server, dsl); Go testing (server-go); CI: `.github/workflows/ci.yml` |
| **Run** | `cd server && npm test` · `cd dsl && npm test` · `cd server-go && go test ./...` |
| **Related** | [progress.md](./progress.md) · [traceability.md](./traceability.md) |

## 1. Principles

1. **Contract-first.** The `flowforge/v1` DSL and the REST surface are the contracts; tests pin them so refactors (incl. the Node→Go port) can't silently change behavior.
2. **Deterministic by default.** Engine and API tests run with the scheduler **off** and advance state via `tickAll` — no timers, no flakiness. Live-LLM and network behavior is isolated in separate, opt-in tests.
3. **Two engines, one suite.** The Go distributable must pass the **same conformance scenarios** as the Node reference (the scenario IDs below are the shared gate).
4. **Every scenario has an ID** (e.g. `ENG-02`) used in the test file, this doc, and the traceability matrix.

## 2. Test layers

| Layer | What | Where | Status |
|---|---|---|---|
| **Unit** | Pure functions (DSL serialize, AI normalization, metrics) | `server/tests/{yaml,ai,metrics}.test.ts`, `dsl/tests/dsl.test.ts` | ✅ |
| **Engine** | Step-by-step execution semantics (in-memory DB, `tickAll`) | `server/tests/engine.test.ts` | ✅ |
| **API contract** | HTTP behavior via Fastify `.inject` (no port) | `server/tests/api.test.ts` | ✅ |
| **Conformance (DSL)** | Round-trip parse/serialize against a corpus | *planned* | ⬜ DSL-02 |
| **Live / integration** | Real LLM + connectors (opt-in, gated by env) | *planned* | ⬜ AI-03 |
| **UI / E2E** | Studio → approve → run → track (Playwright) | *planned* | ⬜ UX-* |
| **Safety** | Sandboxing, egress denial, resource limits | *planned* (P2) | ⬜ SEC-* |
| **Distribution** | Binary build matrix, signed release artifacts | *planned* (P3) | ⬜ DIST-* |

## 3. Testability hooks in the code

- `createServer(d, { schedule, logger, cors })` (`server/src/app.ts`) — builds the app without binding a port; tests use `.inject()`.
- `openDB(':memory:')` — ephemeral DB per test.
- `tickAll(d)` (`server/src/engine.ts`) — one synchronous engine transition; the scheduler calls this on an interval, tests call it directly.
- `tests/helpers.ts` — `memDB`, `sampleWorkflow`, `newRun`, `drive`/`drain`.

## 4. Scenario catalog

### Engine (F-EXEC) — `engine.test.ts`
| ID | Scenario | Automated |
|---|---|---|
| ENG-01 | Runs to a human approval and **waits** when above threshold | ✅ |
| ENG-02 | **Completes** after a waiting task is approved; escalation **skips** | ✅ |
| ENG-03 | **Auto-approves** the manager step when condition is below threshold | ✅ |
| ENG-04 | **Retries from the failed step** without re-running completed steps | ✅ |
| ENG-05 | **Cancels** a running instance and records `endedAt` | ✅ |

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
| PLG-01 | WASM plugin publishes a JSON result through the `ff` ABI | ✅ (`server-go/internal/wasm/wasm_test.go`) |
| PLG-02 | Memory cap enforced at instantiation | ✅ (`wasm_test.go`) |
| PLG-03 | Execution timeout interrupts a spinning plugin | ✅ (`wasm_test.go`) |
| PLG-04 | `ff.http_request` blocked by egress policy (no call); allowed reaches the server | ✅ (`wasm_test.go`) |
| PLG-05 | Safe-mode blocks plugin HTTP | ✅ (`wasm_test.go`) |
| TPL-01 | Gallery templates parse + validate against the frozen DSL; traversal rejected | ✅ (`server-go/internal/templates/templates_test.go`) |
| TPL-02 | Instantiate creates a draft (trigger + steps, string params, uniqued ids) | ✅ (`templates_test.go`, `ext_test.go`) |

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

- Every public REST endpoint has at least one API-* test.
- Every step resolution branch (succeed / wait / skip / auto-approve / fail→retry) has an ENG-* test.
- The DSL conformance corpus covers all step types and param shapes (DSL-02).
- No test depends on the network unless explicitly opt-in (AI-03).

## 6. Adding a test

1. Pick or create a scenario ID (area prefix: `ENG`, `API`, `DSL`, `AI`, `MET`, `SEC`, `DIST`, `UX`, `CONF`).
2. Write it in the matching `server/tests/*.test.ts` (or a new file); use the helpers, keep it deterministic.
3. Add a row to the **scenario catalog** above and to [traceability.md](./traceability.md).
4. `npm test` must stay green before merge.
