# FlowForge — Decision Record

Lightweight ADR log. Each entry: context → decision → consequences. Status: adopted unless marked superseded.

| | |
|---|---|
| **Related** | [build-plan.md](./build-plan.md) · [architecture.md](./architecture.md) |

---

## D1 — Custom step types vs the frozen DSL *(adopted 2026-08-20)*

**Context.** Connectors/plugins need custom step behavior, but `flowforge/v1` is a frozen contract; opening a `connector.*` type namespace in the schema enum would churn the contract on every connector.

**Decision.** Add exactly one new step type to the enum: `type: "connector"`. The target connector is named by `params.connector` (registry key); connector-specific parameters ride in the existing open `params` map and are validated **by the connector's own JSON Schema** (declared in the connector manifest), not by the DSL schema. The DSL stays frozen otherwise; this is the only additive change, made once.

**Consequences.**
- Editors/runner only need to know one new type; per-connector params are late-bound and validated at deploy/approve time against the manifest.
- `@flowforge/dsl` schema + Go spec gain `connector` in the enum (additive, version stays `flowforge/v1`).
- Unknown `params.connector` keys fail approval with an actionable error listing installed connectors.

## D2 — Plugin runtime *(adopted 2026-08-20)*

**Context.** `script` steps already run sandboxed via Starlark (P2). Packaged plugins (custom step executors distributed as files) need a stronger, language-agnostic isolation story.

**Decision.** WASM via `wazero` (pure-Go, no cgo, air-gap safe) with a minimal JSON-in/JSON-out ABI:
- Module exports `execute() -> i32` (0 = success) plus memory helpers; host passes input via imported host module `ff` (`input_len()`, `write(ptr,len)`, `result(ptr,len)`, `log(ptr,len)`, `http(ptr,len) -> i32`).
- `http` is the **only** network-capable host function and routes through the existing policy module (safe-mode + egress allow-list) — same gate as `integration` steps.
- Limits: memory pages cap (default 32 MiB), execution timeout per step.

**Consequences.**
- Starlark stays for inline `script` steps (unchanged P2 behavior).
- Plugins are testable offline: `flowforge plugin test <dir>` loads the manifest + module and runs a dry input.
- Adding the `wazero` dependency keeps the air-gap promise (pure Go, no network at build or runtime).

## D3 — Connector distribution format *(adopted 2026-08-20)*

**Context.** Connectors must be addable by external contributors without touching core.

**Decision.** A connector is a **directory** containing `connector.yaml` (manifest: id, version, display name, description, param JSON Schema, auth modes, executor kind) and optional assets (`schema.json`, mock, wasm module). Registry sources, in order: embedded built-ins (`connectors/builtin/…` via `embed.FS`), then `FLOWFORGE_CONNECTOR_DIR` user dirs (later wins on id collision = override). Signing/OCI packaging rides P3 tooling; the manifest carries a `signing` block reserved for it.

**Consequences.**
- Zero-install experience: built-ins ship in the binary; drop-in extensibility via a directory.
- The contract-test harness validates any directory against the manifest spec, so external connectors get the same gate as built-ins.

## D4 — Compatibility policy *(adopted 2026-08-20)*

**Context.** P4 stabilizes the public surface; adopters need change predictability.

**Decision.**
- `/api/v1` and `flowforge/v1` follow **SemVer**: major = breaking (new path/version marker), minor = additive, patch = fixes. Additive releases must not break N-1 clients.
- The runner supports **N-1**: an artifact produced by the previous minor version must still run.
- Deprecations are announced in-release notes + `Deprecated:` API annotations and removed only at the next major.

**Consequences.**
- CI gains a contract check (schema + API surface diff) in P4.5 tooling.
- `connector` enum addition (D1) is an additive minor under this policy.
