# FlowForge — Versioning & Compatibility Policy

| | |
|---|---|
| **Status** | Adopted (decision D4, 2026-08-20) |
| **Applies to** | `/api/v1` REST surface · `flowforge/v1` DSL · connector manifests · plugin ABI |
| **Related** | [decisions.md](./decisions.md) · [build-plan.md](./build-plan.md) |

## 1. Versioned surfaces

| Surface | Current | Where it lives |
|---|---|---|
| REST API | `/api/v1` (v1) | `server-go/internal/api`, `server/src/routes.ts`, [openapi.yaml](./openapi.yaml) |
| DSL artifact | `flowforge/v1` | `dsl/src/schema.json`, `server-go/internal/spec` |
| Connector manifest | `connector.yaml` (unversioned fields today) | `server-go/internal/connectors` |
| WASM plugin ABI | `ff` host module (see `internal/wasm`) | `server-go/internal/wasm` |

## 2. SemVer mapping

- **Major** — a breaking change: new API path/version marker (e.g. `/api/v2`)
  or a new `apiVersion` (e.g. `flowforge/v2`). Old versions are supported for
  at least one major cycle after the new one ships.
- **Minor** — additive only: new optional endpoints, new optional fields, new
  step types (e.g. `connector` added 2026-08-20). N-1 clients must keep working.
- **Patch** — fixes and clarifications with no surface change.

## 3. Guarantees

1. **Runner N-1**: an artifact produced by the previous minor version of the
   DSL must run on the current runner. Conformance suites (`dsl/tests`,
   `server-go/internal/spec`) enforce this on every change.
2. **Additive enums**: new step types never remove or re-mean old ones.
3. **Deprecation**: marked with `Deprecated:` annotations + release notes;
   removal only at the next major.

## 4. Connector / plugin compatibility

- Connector manifests gain fields additively; unknown fields are ignored
  (forward compatible), missing required fields fail with actionable errors
  (`flowforge connector validate`).
- The plugin ABI (`ff` host module) is versioned with the WASM executor; a
  module importing functions that do not exist fails at instantiation with the
  missing import named — never silently.

## 5. CI enforcement (P4.5 tooling)

`docs/openapi.yaml` is the published contract; contract drift checks
(schema + endpoint diff against the live server) land with the P3 release
tooling. Until then, the Go and Node suites plus the DSL conformance tests are
the gate.
