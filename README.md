<div align="center">

# FlowForge

**Describe it. Approve it. Run it anywhere.**

The self-hosted workflow platform where AI drafts, humans approve,
and every workflow is a portable file you own.

[![Release](https://img.shields.io/github/v/release/santhoshmp/flowforge?style=flat-square&color=6d28d9)](https://github.com/santhoshmp/flowforge/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/santhoshmp/flowforge/ci.yml?style=flat-square&label=CI)](https://github.com/santhoshmp/flowforge/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue?style=flat-square)](LICENSE)
[![Platforms](https://img.shields.io/badge/platforms-linux%20%7C%20darwin%20%7C%20windows%20%7C%20amd64%20%7C%20arm64-64748b?style=flat-square)](https://github.com/santhoshmp/flowforge/releases/latest)

```bash
docker run -p 8080:8080 -v flowforge-data:/data ghcr.io/santhoshmp/flowforge
```

[Quick start](#install) · [Features](#features) · [Security](#security--governance) · [Extend](#extensibility) · [API](#api) · [Docs](#documentation)

</div>

---

## Why FlowForge

Enterprise workflow automation is usually a choice between heavyweight BPM
suites and ephemeral SaaS you can't hold onto. FlowForge is a third option:

- **AI proposes, humans dispose.** Describe a process in plain language; the
  AI drafts a typed workflow with per-step confidence and explicit
  assumptions. Nothing executes until a named human approves — and every
  approval lands on an immutable audit trail.
- **Your workflow is a file.** Every workflow exports as a readable,
  versioned, *signable* `flowforge/v1` YAML artifact. Run it centrally via
  API, on a laptop, or in an air-gapped site. No lock-in, no proprietary
  storage format.
- **One binary, batteries included.** Engine, SQLite, REST API, and the full
  Studio UI ship in a single ~6 MB binary. `docker run` and you're live —
  first-run setup takes under a minute.
- **Safe by default.** Sandboxed script steps, egress allow-lists, encrypted
  secrets, and connector approval gates — governance built in, not bolted on.

## The loop

| | Step | What happens |
|---|---|---|
| 1 | **Describe** | *"When an invoice over $10K arrives, validate the vendor, route to the manager, then post to ERP."* |
| 2 | **Review** | The draft appears on a visual canvas — confidence scores, assumptions, inline editing, chat refinement |
| 3 | **Approve** | One click from a named approver. Connector steps are validated against installed manifests at the gate |
| 4 | **Run & track** | Live step timeline with outputs and durations; retry from the failed step — never the whole flow |
| 5 | **Take it with you** | Export the `.flow.yaml`, verify its Ed25519 signature, run it anywhere |

## Install

**Docker** (fastest):

```bash
docker run -d -p 8080:8080 -v flowforge-data:/data ghcr.io/santhoshmp/flowforge
# or: docker compose up -d      (see docker-compose.yml)
```

**Kubernetes** (Helm):

```bash
helm install flowforge chart/flowforge \
  --set ingress.enabled=true --set ingress.host=flowforge.example.com
```

**Binaries** — grab a signed archive for your platform from the
[latest release](https://github.com/santhoshmp/flowforge/releases/latest)
(linux/darwin/windows × amd64/arm64), verify it, run it:

```bash
sha256sum -c SHA256SUMS
cosign verify-blob \
  --certificate flowforge-vX.Y.Z-linux-amd64.tar.gz.pem \
  --signature     flowforge-vX.Y.Z-linux-amd64.tar.gz.sig \
  --certificate-identity-regexp "https://github.com/santhoshmp/" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  flowforge-vX.Y.Z-linux-amd64.tar.gz
tar xzf flowforge-vX.Y.Z-linux-amd64.tar.gz && ./flowforge serve
```

<details>
<summary><b>From source</b> (Node dev loop or Go build)</summary>

```bash
# Go single binary — build the UI into the embed first
npm --prefix app install && npm --prefix app run build
cp -r app/dist/* server-go/ui/dist/
cd server-go && go build -o flowforge ./cmd/flowforge && ./flowforge serve

# Reference implementation (hot reload): server on :8080, UI on :3000
cd server && npm install && npm run dev
cd app && npm install && npm run dev
```
</details>

On first launch the server is in **setup mode**: open `http://localhost:8080`,
create the admin account, log in. Optional: enable real AI authoring in
**Admin → AI authoring model** — OpenAI, OpenRouter, Groq, or local
**Ollama / LM Studio**. Without a key, a deterministic local generator drafts
workflows so the whole loop works offline.

## Features

**Authoring**

| | |
|---|---|
| Conversational authoring | Prompt → typed draft with per-step confidence + assumptions |
| Visual editor | Canvas, JSON view, palette, step panel; custom step types from the Admin console |
| Template gallery | Six proven `flowforge/v1` patterns (finance / HR / operations) — instantiate, edit, approve |
| Master data | Golden-record entities (vendors, customers, products, employees); new records enter *pending stewardship* |

**Execution**

| | |
|---|---|
| Durable engine | Step-by-step execution persisted to SQLite; survives restarts |
| Human tasks | Wait / resume with SLA tracking and escalation |
| Conditions | Run input (e.g. `total: 24000`) drives branching; below-threshold auto-approves |
| Failure handling | Retry from the failed step (never re-runs completed work); cancel with audit |
| Observability | Fleet KPIs, 14-day trends, per-workflow tracker, live step timeline |

**Extensibility**

| | |
|---|---|
| Connector SDK | Typed `type: connector` steps — http / smtp / wasm executors, params schemas, redacted dry-runs, approve-time validation, drop-in directories |
| Built-in connectors | `http-json`, `slack-webhook`, `smtp` ship embedded; override by id with your own |
| WASM plugins | Custom connectors as `.wasm` (wazero, pure Go) — 32 MiB cap, 5s timeout, one egress-gated host call as the only network path |
| Encrypted secrets | AES-256-GCM vault; `${secret.NAME}` refs; values never logged, never returned by the API |
| Sandboxed scripts | `script` steps run in a Starlark sandbox — no host fs, no network |

## Security & governance

- **Human approval gate** — no workflow deploys without a named approver.
- **Built-in auth** — bcrypt + HMAC session tokens, first-run admin setup,
  setup-mode lockout. Opt-in self-signed TLS.
- **Egress control** — default-deny allow-list (`FLOWFORGE_EGRESS_ALLOW`) and
  `FLOWFORGE_SAFE_MODE` that disables all real execution.
- **Audit trail** — every draft, approval, run, export, and secret operation.
- **Signed supply chain** — releases carry SHA256SUMS, cosign keyless
  signatures, and an SPDX SBOM; workflow artifacts sign offline with
  Ed25519 (`flowforge keygen / sign / verify`).

## Extensibility in 30 seconds

A connector is a directory with a manifest and a params schema:

```yaml
# connectors/my-erp/connector.yaml
id: my-erp
name: My ERP
version: 1.0.0
executor: http
http:
  method: POST
  url: "${secret.ERP_URL}/invoices"
  body: '{"total": "${input.total}"}'
```

```bash
export FLOWFORGE_CONNECTOR_DIR=./connectors
flowforge connector validate ./connectors/my-erp
flowforge connector test ./connectors/my-erp input.json
```

Drop it in, reference it as `type: connector` in any workflow — the approval
gate validates params against your schema before deploy. See
[`connectors/README.md`](connectors/README.md) and [`docs/decisions.md`](docs/decisions.md)
(the `ff` WASM ABI, the connector format, and the compatibility policy).

## The portable artifact

Every workflow exports as a `flowforge/v1` YAML file — human-readable,
versioned, and verifiable:

```bash
flowforge validate invoice.flow.yaml
flowforge run invoice.flow.yaml          # execution-plan preview
flowforge sign   invoice.flow.yaml       # Ed25519 detached signature
flowforge verify invoice.flow.yaml       # offline provenance check
```

The DSL is a frozen contract ([`dsl/`](dsl)) — JSON Schema, parser, and
canonical serializer shared by the editor, the API, and the runner, with
round-trip conformance tests on every change.

## Architecture at a glance

```
┌───────────────────────────── one binary ─────────────────────────────┐
│  Studio UI (React, embedded)   │  REST /api/v1   │  CLI (validate ·  │
│  canvas · dashboards · admin   │  OpenAPI spec   │  run · sign · …)  │
├────────────────────────────────┴──────────────────┴──────────────────┤
│  Control plane: auth · policy (safe-mode + egress) · audit · secrets │
│  Durable engine: tick-based, SQLite-backed, human tasks + retries    │
│  Executors: Starlark sandbox · gated HTTP · connectors (http/smtp)   │
│             · WASM plugins (wazero, memory/time capped)              │
├──────────────────────────────────────────────────────────────────────┤
│  flowforge/v1 artifact (dsl/): JSON Schema · parser · serializer     │
└──────────────────────────────────────────────────────────────────────┘
```

<details>
<summary><b>Repository layout</b></summary>

```
app/            Studio UI (React 19 + Vite)
server/         Node/TS reference implementation (Fastify + SQLite)
dsl/            @flowforge/dsl — the frozen flowforge/v1 contract
server-go/      Go distributable: engine, API, embedded UI, connectors,
                wasm runtime, templates, secrets, signing (+ e2e suite)
connectors/     user drop-in connector directory
chart/flowforge Helm chart (single replica + SQLite PVC)
scripts/        release build (cross-compile matrix + checksums)
Dockerfile      multi-stage: UI → static binary → alpine (non-root)
docs/           quickstart · architecture · decisions · openapi · release …
```
</details>

<details>
<summary><b>Configuration</b></summary>

| Env var | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP port |
| `DB_PATH` | `flowforge.db` | SQLite database file |
| `FLOWFORGE_AUTH` | `auto` | `auto` (setup mode → token required) or `off` |
| `FLOWFORGE_TLS` | `off` | `on` serves HTTPS with a generated self-signed cert |
| `FLOWFORGE_SAFE_MODE` | `off` | `on` disables all real execution |
| `FLOWFORGE_EGRESS_ALLOW` | *(empty)* | Comma-separated host suffixes; when set, egress defaults to deny |
| `FLOWFORGE_CONNECTOR_DIR` | *(empty)* | Drop-in connector directory |
| `FLOWFORGE_SECRETS_FILE` | `flowforge.secrets` | Encrypted vault location |
| `FLOWFORGE_SECRETS_KEY` | *(auto key file)* | base64 32-byte key (overrides the key file) |
| `OPENAI_API_KEY` / `OPENAI_BASE_URL` / `OPENAI_MODEL` | — | AI authoring (any OpenAI-compatible endpoint; or configure in the UI) |

</details>

## API

Full OpenAPI contract: [`docs/openapi.yaml`](docs/openapi.yaml).
The core loop over HTTP:

```bash
# 1. draft from a prompt
curl -X POST /api/v1/ai/draft -d '{"prompt":"invoice approval over 10k …"}'

# 2. human approval (gated: connectors validated here)
curl -X POST /api/v1/workflows/{id}/approve

# 3. run with input
curl -X POST /api/v1/workflows/{id}/executions -d '{"input":{"total":24000}}'

# 4. resolve the human task mid-flight, or retry from a failed step
curl -X POST /api/v1/executions/{id}/approve
curl -X POST /api/v1/executions/{id}/retry
```

Also: `/metrics`, `/mdm`, `/controls`, `/audit`, `/settings/ai`,
`/connectors` (+ `/test`), `/templates` (+ `/instantiate`), `/secrets`.

## Quality

The suite that guards all of this — **Go 94 tests across 14 packages**
including a **binary E2E suite** (built binary: CLI, live `serve`, full
lifecycle with the real scheduler, kill-and-restart durability),
**Node 16/16**, **DSL 14/14**, plus CI jobs for Docker (build + container
smoke) and Helm (lint + render). Scenario catalog:
[`docs/test-strategy.md`](docs/test-strategy.md)
(ENG · API · DSL · SEC · STORE · EXT · CONN · PLG · TPL · SIGN · E2E · DIST).

```bash
cd server-go && go test ./...     # includes e2e
cd server && npm test
cd dsl && npm test
```

## Documentation

| | |
|---|---|
| [`docs/quickstart.md`](docs/quickstart.md) | 5-minute quickstart |
| [`docs/release.md`](docs/release.md) | Release runbook — verifying signatures, artifact signing |
| [`docs/openapi.yaml`](docs/openapi.yaml) | The `/api/v1` contract |
| [`docs/decisions.md`](docs/decisions.md) | Architecture decision record (D1–D4) |
| [`docs/versioning.md`](docs/versioning.md) | SemVer + N-1 compatibility policy |
| [`docs/architecture.md`](docs/architecture.md) | System design + deployment topologies |
| [`docs/build-plan.md`](docs/build-plan.md) · [`docs/progress.md`](docs/progress.md) | Roadmap + status |
| [`docs/test-strategy.md`](docs/test-strategy.md) · [`docs/traceability.md`](docs/traceability.md) | Testing |

## Tech stack

Go 1.26 (single static binary, `embed.FS`, pure-Go SQLite, Starlark, wazero) ·
React 19 + Vite 7 + Tailwind + shadcn/ui · Node/TS reference server ·
JSON Schema frozen contract — all Apache-2.0.

## Roadmap

| Phase | Status | |
|---|---|---|
| Single binary · safety · distribution | ✅ | Engine + embedded UI + auth/TLS + signed releases (v0.1.0) |
| Extensibility | ✅ | Connector SDK · WASM plugins · secrets · templates · OpenAPI |
| Community hardening | 🔨 | Plugin authoring guide · docs site · lint cleanup |
| Enterprise | ⬜ | SSO/SAML · RBAC · Postgres · HA |

---

<div align="center">

**Apache-2.0** — free for everyone, forever.

[Report an issue](https://github.com/santhoshmp/flowforge/issues) · [Releases](https://github.com/santhoshmp/flowforge/releases) · [v0.1.0](https://github.com/santhoshmp/flowforge/releases/tag/v0.1.0)

</div>
