# FlowForge — Documentation

Start here. FlowForge is a downloadable, self-hostable enterprise workflow platform: describe a process → AI drafts it → a human approves it → it runs centrally via API **or** as a portable file anywhere (even air-gapped).

## Run the prototype now

- **[demo-runbook.md](./demo-runbook.md)** — install, configure AI (incl. local LLMs), and a step-by-step demo script. UI on :3000, API on :8080.

## Product & strategy

- **[product-design.md](./product-design.md)** — freeware/open-core model, packaging, distribution, security, sandboxing, telemetry, edition split.
- **[architecture.md](./architecture.md)** — system architecture, deployment topologies (binary / compose / Kubernetes), engine tiers, security boundaries, stack.
- **[build-plan.md](./build-plan.md)** — phased roadmap from prototype to public beta → GA, with milestones, effort, and risks.

## Engineering reference

- **[design-and-production-plan.md](./design-and-production-plan.md)** — the detailed domain model, data schema, Temporal execution design, API contract, and the original production plan.
- **[openapi.yaml](./openapi.yaml)** — the published `/api/v1` contract.
- **[release.md](./release.md)** — release runbook: tag → matrix + SBOM + cosign + ghcr; verifying releases; artifact signing.

## Development process (living docs — keep updated)

- **[progress.md](./progress.md)** — what's done / in progress / planned, phase status, test status, and a changelog.
- **[test-strategy.md](./test-strategy.md)** — test layers, the scenario catalog (ENG/API/DSL/AI/MET/…), and how to add a test.
- **[traceability.md](./traceability.md)** — feature → code → tests → status matrix.

## At a glance

| Doc | For |
|---|---|
| demo-runbook | running and demoing the prototype today |
| product-design | what we're building and why (licensing, safety, packaging) |
| architecture | how it's structured (diagrams, topologies) |
| build-plan | when and in what order we build it |
| design-and-production-plan | deep engineering detail (model, engine, API) |
| progress | current status + changelog (update on every change) |
| test-strategy | what we test and the scenario IDs |
| traceability | feature → code → tests map |

## Repository layout (current)

```
app/            React + Vite Studio UI (reference frontend)
server/         Node/TS control plane + SQLite + engine (reference backend)
dsl/            @flowforge/dsl — the frozen flowforge/v1 contract (schema + parser + serializer, tested)
server-go/      Go distributable: single binary (engine + SQLite + REST + embedded UI + sandboxed execution)
  └─ internal/  … connectors (SDK + built-ins), wasm (plugin runtime), templates (gallery), secrets (vault)
connectors/     User drop-in connector directory (FLOWFORGE_CONNECTOR_DIR) — see connectors/README.md
docs/           this documentation
```

The reference (Node) implementation and the Go distributable both target the
**frozen `flowforge/v1` contract** in `dsl/`. See [architecture.md](./architecture.md).

## Extensibility docs

- **[decisions.md](./decisions.md)** — decision record (D1 connector step type, D2 WASM ABI, D3 connector format, D4 compatibility policy).
- **[versioning.md](./versioning.md)** — SemVer + N-1 compatibility policy for the API and DSL.
- **[openapi.yaml](./openapi.yaml)** — the published `/api/v1` contract.
- **[quickstart.md](./quickstart.md)** — 5-minute quickstart with the Go binary.
