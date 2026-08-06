# FlowForge — Product Design (Downloadable, Self-Hosted Enterprise Tool)

| | |
|---|---|
| **Document** | Product Design |
| **Status** | Draft for review |
| **Version** | 1.0 |
| **Last updated** | 2026-08-01 |
| **Scope** | Designing FlowForge as a freeware, downloadable, self-hostable enterprise workflow tool |
| **Related** | [architecture.md](./architecture.md) · [build-plan.md](./build-plan.md) · [design-and-production-plan.md](./design-and-production-plan.md) |

---

## 1. Product vision

FlowForge is a **downloadable, self-hostable enterprise workflow platform**: describe a process in plain language, AI drafts it, a human approves it, and it runs centrally via API **or** as a portable file anywhere — including air-gapped. It is the deliberate opposite of heavyweight BPM suites: minutes to first workflow, one human-readable artifact, runs anywhere.

The product is **freeware** — free to download, own, and operate on the customer's own infrastructure, with no mandatory dependency on any cloud or vendor.

## 2. Guiding principles

1. **Install in minutes, value in five.** A single binary or one container; no database to stand up by default.
2. **Owns nothing of yours.** Runs on the customer's infra; data, secrets, and models stay with them.
3. **Works air-gapped.** No hard dependency on any external service, including LLMs.
4. **The workflow is a file.** `flowforge/v1` YAML is the portable, owned artifact.
5. **Safe by default.** Untrusted execution is sandboxed; egress is opt-in.
6. **Trust is a primitive.** AI proposes; a named human approves; everything is audited.
7. **Extensible by the community.** Connectors and templates are the contribution surface.

## 3. What ships (the product surface)

A single distributable that contains:

- **Control plane** — REST API, durable execution engine, audit log, MDM registry, admin console.
- **Studio UI** — the React authoring/review/monitor/dashboard experience (embedded as static assets).
- **Portable runner** — executes a signed `.flow.yaml` standalone.
- **CLI** — `serve`, `run`, `migrate`, `backup`/`restore`, `version`.
- **Connector runtime** — HTTP/email/Slack/webhook/database + a plugin/SDK loader.
- **Templates** — a starter gallery (invoice, onboarding, leave, ticket, expense, PO).

## 4. Licensing & edition model

"Freeware" must be made precise. For enterprise trust + ecosystem growth, **open-core** is recommended.

| Option | Adoption | Protects cloud biz | Enterprise-friendly | Notes |
|---|---|---|---|---|
| Apache-2.0 (current README) | Highest | No | Yes | Best for connector ecosystem |
| AGPL-3.0 | Medium | Somewhat | Mixed (some ban AGPL) | Forces SaaS forks open |
| BSL / Elastic (source-available) | Lower | Yes | No | Not OSI-OSS; limits community |
| **Open-core (recommended)** | High | Yes | Yes | Apache core + paid enterprise modules |

**Recommended:** Apache-2.0 for the engine, DSL, runner, Studio, connectors, and templates. A small **Enterprise Edition** adds SSO (SAML/OIDC), RBAC, HA/multi-replica, audit export, advanced MDM match-merge, and process mining. The **trademark "FlowForge"** is registered separately so the name is protected even though the code is forkable.

**Community vs Enterprise split (indicative):**

| Capability | Community (free) | Enterprise (paid) |
|---|---|---|
| Authoring, approval, run, monitor, dashboard | ✅ | ✅ |
| Local users / password auth | ✅ | ✅ |
| SQLite, single binary | ✅ | ✅ |
| Sandboxed steps + egress control | ✅ | ✅ |
| OIDC/SAML/LDAP SSO | — | ✅ |
| Fine-grained RBAC, teams | — | ✅ |
| Postgres + HA (multi-replica API + workers) | — | ✅ |
| Multi-tenancy | — | ✅ |
| Audit export / retention, compliance packs | — | ✅ |
| Process mining, advanced match-merge | — | ✅ |

## 5. Packaging & distribution

The #1 adoption factor. Enterprises give a tool ~5 minutes.

- **Single static binary** is the gold standard: embed the built UI and serve it from the same process. Removes today's "two terminals" setup.
- Provide **all three** distribution paths:
  1. **Single binary** (`flowforge`) — laptop / single-server / air-gapped.
  2. **Docker image** + `docker-compose.yml` — small teams.
  3. **Helm chart** — Kubernetes / HA.
- **Cross-compile**: Linux / macOS / Windows × amd64 / arm64.
- **Trust artifacts per release**: SHA256 checksums, **Sigstore/cosign signatures**, and an **SBOM**.
- **Air-gapped install** is a differentiator (and it's in the pitch): bundle a local-LLM path and an MDM snapshot; no telemetry phoning home.

**Stack decision (engine/binary language):**

| Path | Pros | Cons |
|---|---|---|
| **Rewrite distributable in Go** (recommended) | True single static binary; tiny image; best air-gapped story; matches the planned Go runner | Upfront rewrite cost; duplicate logic to manage via shared spec/tests |
| Ship Node as **Single Executable Application (SEA)** | Reuses the existing Node/TS code | Larger binaries; less "native" feel |

Recommendation: **Go for the distributable** (control plane + runner + embedded UI via `embed.FS`); the DSL and types become a language-agnostic, shared spec with a conformance test suite. Keep the Node/TS prototype as a fast iteration/reference implementation.

## 6. Deployment topologies

| Topology | Who | Shape |
|---|---|---|
| **Laptop / single-server** | Individual, small org, air-gap | One process, SQLite, in-process engine |
| **Team (compose)** | 10s of users | One container (+ optional Postgres), worker in-process |
| **Enterprise (Kubernetes)** | 100s–1000s, HA | API replicas, separate worker replicas, Postgres HA, object store, secrets via Vault/K8s, ingress TLS, optional Temporal |
| **Embedded** | Product teams embedding workflow into their app | Library/SDK consuming the engine + DSL |

## 7. First-run experience

- **Precedence chain:** CLI flags > env vars > `flowforge.yaml` config file > built-in defaults. Documented.
- **Setup wizard** on first launch: create admin user → choose storage (SQLite default) → point at an IdP or skip → configure AI provider (reuse the existing Admin AI card as part of onboarding).
- **Zero-config defaults:** SQLite in `~/.flowforge/data`, localhost bind, deterministic fallback AI so it works with no API key.
- **Data portability:** `flowforge export/import` (`.flow.yaml` + JSON dump of MDM/audit) — backup, move, never feel locked in.

## 8. Security model

- **Auth, bring-your-own:** built-in username/password (bcrypt) for tiny installs **plus** OIDC/SAML/LDAP for enterprise. Never build proprietary SSO.
- **TLS by default:** auto-generate a self-signed cert on first run; allow user-supplied certs.
- **Secrets:** env / local encrypted file (`~/.flowforge/secrets`) for single-user; Kubernetes secrets / Vault for enterprise. Never logged or echoed (mask everywhere — already done in the API).
- **Multi-tenancy:** row-level `org_id` from day one, even if shipped single-tenant first.
- **RBAC roles:** `author / approver / operator / admin / steward / viewer`.

## 9. Sandboxing untrusted execution (the biggest risk)

FlowForge runs **user-supplied logic**: `script` steps, connector HTTP calls, and imported workflows. A malicious/buggy workflow could exfiltrate data or damage the host — and you will be blamed.

- **Never** run user steps in the main/trusted process.
- Isolation options (strongest first):
  1. **WASM** runtime for `script` steps (pure-Go `wazero` fits the single-binary story).
  2. **Sidecar containers** per execution / per connector.
  3. **Restricted OS users** with seccomp/AppArmor.
- **Egress allow-lists** per workflow/org — **default-deny** option.
- **Resource limits**: CPU / memory / wall-clock timeouts per step.
- **Safe mode**: a toggle that disables `script` and arbitrary-HTTP steps entirely, for orgs that only want approval routing.

Treat sandboxing as a **release blocker**, not a roadmap item.

## 10. Data & persistence

- **Embedded SQLite by default**; **Postgres as an optional backend** for HA/scale. Same schema, swappable driver.
- **Append-only, tamper-evident audit** (hash-chained / signed) in all editions.
- **Retention policies** configurable (audit, instance history).
- **Migrations** are versioned and reversible; the N→N+1 path is tested in CI.

## 11. Extensibility & ecosystem

- **Connector SDK:** typed contract, config/auth schemas, retry semantics, dry-run mocks. The best first contribution (your README already says this — formalize it).
- **Stable public API** with **SemVer + deprecation policy**; versioned API (`/api/v1`) and DSL (`flowforge/v1`).
- **The DSL is sacred** — the portable contract between authoring, API, and runner. JSON Schema + RFC process.
- **Plugin loading** for custom step types without recompiling (WASM or a plugin protocol).

## 12. Upgrades, backup, observability

- **Reversible migrations** that never destroy user data.
- **Graceful upgrades:** in-flight executions finish on their workflow version.
- **One-command backup/restore;** documented DR.
- **Observability built in:** structured logs (stdout/files), `/health`, Prometheus metrics, and the live dashboard. Enterprises point their own stack at these.
- **Auto-update** (opt-in, checksum-verified) for the binary edition; manual control for air-gapped.

## 13. Telemetry, privacy, trust

- **Anonymous, opt-out usage telemetry** (counts only; never payloads/entities) with a first-run prompt and a documented off-switch. Vital for product decisions; toxic if opaque.
- **No hard dependency on any cloud/LLM** — cloud features are opt-in.
- Publish a **threat model**, **`SECURITY.md`**, and a **responsible-disclosure** channel.

## 14. Documentation & first-value time

- **5-minute quickstart** ending in a real running workflow.
- **Templates gallery** seeded from the six demo workflows (invoice, onboarding, leave, ticket, expense, PO).
- A clear **"free vs paid"** table so there are no surprises.

## 15. Decisions log (open)

1. License: **Apache-2.0 open-core** (proposed).
2. Binary language: **Go** for the distributable (proposed); Node/TS retained as reference.
3. Default storage: **SQLite**, Postgres optional.
4. Enterprise engine: **Temporal**; community engine: the in-process durable engine.
5. Telemetry: **opt-out, anonymous**.
6. Trademark "FlowForge": **registered separately**.

---

*See [architecture.md](./architecture.md) for the system design and [build-plan.md](./build-plan.md) for the execution roadmap.*
