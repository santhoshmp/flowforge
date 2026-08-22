# FlowForge — Build Plan (Roadmap to Public Release)

| | |
|---|---|
| **Document** | Phased build plan for the downloadable product |
| **Status** | Active — P4 detailed |
| **Version** | 1.1 |
| **Last updated** | 2026-08-17 (P4 pulled ahead of P3; Phase 4 detailed) |
| **Scope** | From the current prototype to a public, downloadable, self-hostable tool |
| **Related** | [product-design.md](./product-design.md) · [architecture.md](./architecture.md) |

---

## 1. Starting point

- **`app/`** — React/Vite Studio UI (author → approve → run → monitor → dashboard), now backend-backed.
- **`server/`** — Node/TS control plane (Fastify + SQLite), durable in-process engine, LLM authoring, MDM, metrics.
- This is a **working reference implementation** that defines the `flowforge/v1` contract and the UX. The build plan hardens it into a downloadable product and ports the distributable to Go (single binary).

## 2. Strategy

- **Contract-first:** freeze `flowforge/v1` (JSON Schema + parser) and the REST surface early; everything targets it.
- **Two tracks in parallel after Phase 1:** (A) safety/distribution, (B) enterprise features.
- **Keep the reference (Node) demoable** while the Go distributable is built; a shared conformance suite proves parity.
- **Release a public beta early** (single binary, safe-by-default, local AI) — then harden to GA.

## 3. Phased roadmap

```mermaid
gantt
    title FlowForge distributable roadmap (planning ranges)
    dateFormat  YYYY-MM-DD
    axisFormat  %b
    section Foundations
    P0 Decisions, repo, CI, DSL freeze          :p0, 2026-08-04, 3w
    section Standalone
    P1 Go single-binary MVP (embedded UI, SQLite, engine, local auth, TLS, run cmd) :p1, after p0, 7w
    section Safety
    P2 Sandboxing, egress, secrets, first-run wizard :p2, after p1, 5w
    section Distribution
    P3 Cross-compile, Docker, Helm, signing+SBOM, auto-update :p3, after p2, 4w
    section Extensibility
    P4 Connector SDK, plugins, templates, API/DSL stabilization, docs :p4, after p3, 6w
    section Enterprise
    P5 SSO, RBAC, Postgres, HA/Temporal, audit export, multi-tenancy :p5, after p3, 10w
    section GA
    P6 Security audit, telemetry opt-out, marketplace, beta to GA :p6, after p4, 8w
```

### Phase 0 — Foundations & decisions *(2–3 weeks)*
- License decision (Apache-2.0 open-core proposed), **trademark registration**, edition split finalized.
- Confirm **Go** distributable; repo restructure (monorepo: `dsl`, `server-go`, `ui`, `runner`, `connectors`, `docs`).
- **Freeze `flowforge/v1`**: JSON Schema + Go parser + TS parser, round-trip tests.
- CI (build, test, lint, SBOM), release pipeline skeleton.
- **Exit:** contract frozen; CI green; signed hello-world binary builds.

### Phase 1 — Single-binary MVP *(6–7 weeks)* — *“a real, downloadable FlowForge”*
- Go control plane: REST `/api/v1`, embedded React UI (`embed.FS`), SQLite.
- Port the **in-process durable engine** (persisted replay, human-task wait/resume, retry-from-failed) — validated against the Node reference via conformance tests.
- Built-in local auth (bcrypt), sessions/API tokens; **self-signed TLS** on first run.
- CLI: `serve`, `run <file.flow.yaml>` (portable runner with bundled MDM snapshot), `migrate`, `version`.
- AI adapter (OpenAI-compatible + local + deterministic fallback).
- MDM registry, audit, dashboard.
- **Exit:** one binary downloaded → `flowforge serve` → full describe→approve→run→track loop; `flowforge run file.flow.yaml` works offline.

### Phase 2 — Safety & first-run *(4–5 weeks)* — *release blocker*
- **Sandboxing:** WASM (`wazero`) for `script` steps; restricted HTTP client for connectors.
- **Egress allow-list** (default-deny option); per-step resource limits (CPU/mem/timeouts).
- **Secrets:** encrypted local file + env; never logged.
- **First-run setup wizard** (admin user, storage, IdP-or-skip, AI provider).
- Safe-mode toggle (disable script/arbitrary-HTTP).
- **Exit:** no user-supplied logic runs in the trusted process; safe-mode demonstrable.

### Phase 3 — Distribution *(3–4 weeks)* — *core shipped 2026-08-22; first tagged release pending*
- Cross-compile matrix (linux/darwin/win × amd64/arm64). ✅ `scripts/build.{sh,ps1}` (all 6 verified locally)
- Docker image + `docker-compose.yml`; **Helm chart**. ✅ `Dockerfile`, `docker-compose.yml`, `chart/flowforge` (CI builds/smokes the image, lints/renders the chart)
- Release artifacts: checksums, **cosign signatures**, **SBOM**. ✅ `.github/workflows/release.yml` (syft SPDX + cosign keyless on blobs + multi-arch ghcr image); runbook in `docs/release.md`
- Opt-in auto-update with signature verification. ⬜ deferred post-beta
- Artifact signing for `.flow.yaml` (F-DSL-03, from the P4 plan). ✅ Ed25519 detached `.sig` via `flowforge keygen/sign/verify`
- **Exit:** one-line install on each platform; reproducible, signed release. 🔨 pending the first `v*` tag

### Phase 4 — Extensibility & ecosystem *(7–10 weeks)* — *detailed 2026-08-17*

> **Sequencing note (2026-08-17):** P4 was pulled ahead of P3 (distribution) by explicit decision. P3 still gates the public beta and follows P4 core (4.0–4.4); workstream 4.5 may interleave with P3.

**Decisions (D1–D4) — ADOPTED 2026-08-20 (rationale in `docs/decisions.md`):**

- **D1 — Custom step types vs the frozen DSL.** Keep `flowforge/v1` frozen; add a single `type: "connector"` step whose `params.connector` names a registry entry, with connector-specific params validated by the connector's own JSON Schema. *(Alternative — open `connector.*` type enum/pattern — rejected: schema churn on a frozen contract.)*
- **D2 — Plugin runtime.** WASM via `wazero` (pure-Go, air-gap safe) with a minimal JSON-in/JSON-out ABI; Starlark remains for inline `script` steps.
- **D3 — Connector distribution format.** A directory + `connector.yaml` manifest for P4; signing/OCI packaging rides the P3 tooling.
- **D4 — Compatibility policy.** SemVer + deprecation policy for `/api/v1` and the DSL; the runner supports N-1.

**Workstreams:**

- **4.0 Foundations *(1 wk)*** — CI (build + test + lint for Go/Node/DSL — closes the never-done P0 exit gap); migrate Node `server`/`app` onto `@flowforge/dsl`; fix doc drift (`docs/README.md` "skeleton" claim, Go version mismatch).
- **4.1 Step-executor registry *(1–2 wk)*** — refactor the hardcoded dispatch in `server-go/internal/executor` into a `StepExecutor` interface + runtime registry; built-ins self-register at startup; align with the existing step-controls registry; unknown types fail validation with actionable errors.
- **4.2 Connector SDK *(2–3 wk)*** — `connectors/` module: typed manifest (param schema; auth modes header/bearer/basic/oauth2-client-credentials; rate limits), dry-run mocks, a contract-test harness every connector must pass, and `flowforge connector validate|test` CLI. Ship 2–3 reference connectors (generic HTTP/JSON, Slack webhook, SMTP) as contributor exemplars. REST `/api/v1/connectors` + Admin UI section. The **secrets store** (P2 leftover: encrypted local file + env refs, never logged) lands here as the auth-material dependency.
- **4.3 WASM plugin runtime *(2–3 wk)*** — `wazero` host with resource limits (memory-page cap + per-step timeout), host functions limited to `log` + egress-gated HTTP; plugin manifest, enable/disable, `flowforge plugin test` CLI, "write your first plugin" guide. Security tests must prove: no filesystem, no network outside the gate, limits enforced. *(Shipped 2026-08-20 — PLG-01..05; ABI in docs/decisions.md D2.)*
- **4.4 Templates gallery *(1 wk)*** — extract the six seed workflows (currently inline in `server-go/internal/seed/seed.go`) into `templates/*.flow.yaml` + manifest (name, category, description); `/api/v1/templates`; "start from template" in Home/Studio; templates validated against the DSL schema in CI.
- **4.5 Stabilization & docs *(1–2 wk)*** — publish OpenAPI for `/api/v1`; write the SemVer + deprecation policy (D4); docs site + 5-minute quickstart; contributor guides (connector, plugin).

**New test scenario families** (to be added to [test-strategy.md](./test-strategy.md) as they land): **EXT** (registry dispatch), **CONN** (SDK contract), **PLG** (WASM sandbox + limits), **TPL** (template validity).

- **Exit:** an external contributor can add and ship a connector end-to-end (manifest → contract tests → registry → canvas → run), and a WASM plugin executes a custom step type inside its limits.

### Phase 5 — Enterprise edition *(parallel, ~10 weeks)*
- OIDC/SAML/LDAP SSO; fine-grained RBAC; teams.
- Postgres backend; **HA** (stateless API replicas + worker replicas).
- **Temporal** engine (same interpreter); object store for artifacts/snapshots.
- Multi-tenancy; audit export/retention; compliance packs.
- **Exit:** HA deployment with SSO and durable cross-process execution.

### Phase 6 — Beta → GA *(6–8 weeks)*
- **Anonymous opt-out telemetry** + first-run prompt.
- **External security audit** + pen test; `SECURITY.md`; responsible-disclosure process.
- Template/connector **marketplace** (community).
- Performance/load testing; runbooks; public beta → fixes → **GA**.
- **Exit (GA):** signed binary + Docker + Helm; safe-by-default; docs; security posture validated.

## 4. Team & effort (planning ranges)

Assumes ~5–6 people: 1 tech lead, 2–3 backend (Go), 1 frontend, part-time SRE + design.

| Phase | Duration | Lead need |
|---|---|---|
| 0 — Foundations | 2–3 wks | Tech lead |
| 1 — Single-binary MVP | 6–7 wks | Full team |
| 2 — Safety & first-run | 4–5 wks | Backend-heavy |
| 3 — Distribution | 3–4 wks | SRE + backend |
| 4 — Extensibility | 5–6 wks | Full team |
| 5 — Enterprise | ~10 wks (parallel) | Backend-heavy |
| 6 — Beta → GA | 6–8 wks | Full team |

**~4–5 months to a public single-binary beta** (end of Phase 3); **~7–9 months to GA** with enterprise edition.

## 5. Milestone definitions

- **Public Beta (after P3):** downloadable signed binary + Docker; safe-by-default; local AI; the full UX loop; 5-minute quickstart.
- **GA (after P6):** Beta + hardened sandboxing audited, enterprise edition (SSO/RBAC/HA), connector SDK + templates, telemetry, security audit passed.

## 6. Risks & mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Sandboxing gap → customer compromise | Critical liability | Default-deny egress, WASM isolation, safe-mode; **audit before GA** |
| Go rewrite drifts from Node reference | Behavioral bugs | Shared DSL + conformance test suite as the gate |
| DSL churn breaks runner/artifacts | Broken portability | Freeze in P0; SemVer + RFC; signed version-pinned artifacts; runner supports N-1 |
| Install friction kills adoption | Low uptake | Single binary, zero-config SQLite, 5-min quickstart |
| Enterprise features creep into core | Unclear free/paid line | Maintain the edition table; gate features behind the enterprise build flag |
| Trademark/license ambiguity | Fork/brand risk | Apache core + registered trademark; clear AUP |
| Heavy deps break air-gap promise | Loses key differentiator | No hard cloud/LLM dependency; bundle local model + snapshot |

## 7. Immediate next steps (first 2–3 weeks) — P4 kick-off *(updated 2026-08-17)*

1. Settle decisions **D1–D4** (Phase 4 section above).
2. Stand up **CI** (Go + Node + DSL suites, lint, build) — closes the P0 exit gap.
3. Migrate Node `server`/`app` onto `@flowforge/dsl`; fix doc drift.
4. Refactor the Go executor into the **step-executor registry** (4.1) with dispatch contract tests.
5. Draft the **connector manifest spec + SDK skeleton**; start the reference HTTP/JSON connector (4.2).

---

*This plan operationalizes [product-design.md](./product-design.md) against [architecture.md](./architecture.md).*
