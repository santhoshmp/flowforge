# FlowForge — Go Distributable (control plane + runner)

The single-binary, self-hostable FlowForge runtime. This is the **P1 control plane**: a durable execution engine, SQLite persistence, the full `/api/v1/*` REST surface the React UI expects, an OpenAI-compatible AI authoring layer, MDM, metrics, and the **embedded Studio UI** — all in one binary.

> Mirrors the Node reference (`server/`) and the frozen contract (`dsl/`). Same UX, same API, same scenarios.

## Layout

```
server-go/
  cmd/flowforge/        CLI: version | validate <file> | run <file> | serve
  internal/
    spec/               flowforge/v1 contract (parse/validate/serialize) + conformance tests
    models/             domain types (JSON tags match the UI contract)
    store/              SQLite (modernc, pure-Go) schema + CRUD + seed
    seed/               demo dataset (6 workflows, ~37 executions, audit, mdm, controls)
    engine/             durable engine: TickAll + approve/retry/cancel + conditions
    ai/                 deterministic generator + OpenAI-compatible caller
    settings/           runtime AI provider config (key masked)
    metrics/            dashboard aggregates
    api/                HTTP control plane (stdlib router) mirroring /api/v1/*
    util/               shared helpers
  ui/                   embedded React UI (go:embed); copy app/dist → ui/dist before release
  fixtures/             sample .flow.yaml
```

## Prerequisites

- **Go 1.22+** (verified with **Go 1.26.5**, user-local at `%LOCALAPPDATA%\Programs\go`).
- (To refresh the embedded UI) Node + npm in `app/`.

## Build & test

```bash
cd server-go
go test ./...                       # engine, api, store, ai, spec conformance
go build -o flowforge ./cmd/flowforge
```

## Run the control plane

```bash
./flowforge serve                    # http://localhost:8080  (API + UI)
# env: PORT (8080), DB_PATH (flowforge.db),
#      OPENAI_API_KEY / OPENAI_BASE_URL / OPENAI_MODEL (optional; falls back to deterministic),
#      FLOWFORGE_AUTH (auto|off, default auto: first-run setup then token-required),
#      FLOWFORGE_TLS (on|off, default off: self-signed HTTPS),
#      FLOWFORGE_SAFE_MODE (on|off: disables script + arbitrary-HTTP steps),
#      FLOWFORGE_EGRESS_ALLOW (comma-separated host suffixes; default-deny when set)
```

Then open **http://localhost:8080** — the embedded Studio UI is served by the same binary and talks to `/api/v1/*`.

**First run:** with `FLOWFORGE_AUTH=auto` (default) and no users yet, the server is in *setup mode* — the app is locked until you create an admin (UI setup screen, or `POST /api/v1/auth/setup`). After that, login is required and sessions are HMAC-signed tokens.

Refresh the embedded UI after a frontend change:

```bash
npm --prefix app run build
Copy-Item app/dist/* server-go/ui/dist/ -Recurse -Force   # (PowerShell)
go build -o flowforge ./cmd/flowforge
```

## Artifact CLI

```bash
./flowforge version
./flowforge validate fixtures/vendor-invoice.flow.yaml
./flowforge run fixtures/vendor-invoice.flow.yaml        # plan preview
```

## Status

- ✅ `serve`: HTTP API + durable engine (scheduler-driven `TickAll`) + **embedded React UI**.
- ✅ SQLite persistence (restart-safe; instances resume), seed dataset, audit, MDM, controls.
- ✅ AI authoring: OpenAI-compatible (multi-provider, incl. local Ollama/LM Studio) + deterministic fallback; runtime config + key masking + test-connection.
- ✅ Metrics (`/api/v1/metrics`), per-workflow executions, run-with-input, condition evaluation.
- ✅ **P2 safety**: built-in auth (bcrypt + HMAC tokens) with first-run admin setup + setup-mode gating; opt-in self-signed TLS; request policy module (safe-mode + egress allow-list).
- ✅ Tests: engine (ENG-01..05), API (API-01..06), auth flow, store, policy, spec conformance — all green under Go 1.26.5.
- ✅ **P2 sandboxing**: `script` steps run in a Starlark sandbox (no host fs/net, `load` disabled); `integration` HTTP steps make real calls gated by egress/safe-mode (opt-in via `code`/`url`, else simulated). WASM reserved for packaged plugins (P4).
- ⬜ **P3**: cross-compile matrix, Docker, Helm, signed releases.
