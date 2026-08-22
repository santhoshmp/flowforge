# FlowForge Connector Directory

Drop-in location for user connectors. Point the server at this directory (or
any other) with `FLOWFORGE_CONNECTOR_DIR`; each subdirectory is a connector.

Built-in connectors (`http-json`, `slack-webhook`, `smtp`) ship embedded in
the binary — see `server-go/internal/connectors/builtin/` for canonical
examples of every manifest feature.

## Author a connector (P4.2 SDK)

A connector is a directory containing:

```
my-conn/
├── connector.yaml       # manifest: id, executor, auth, templates
└── params.schema.json   # JSON Schema (type object) for step params
```

`connector.yaml`:

```yaml
id: my-conn              # kebab-case; names the connector in step params
name: My Connector
version: 0.1.0
description: What it does.
category: generic
executor: http           # http | smtp | wasm
auth:
  mode: none             # none | header | bearer | basic
paramsSchema: params.schema.json
http:
  method: POST
  url: "${params.url}"   # ${params.*} ${input.*} ${secret.*} ${env.*}
  body: '{"k": "${input.total}"}'
```

Validate and dry-run:

```bash
flowforge connector validate ./connectors/my-conn
flowforge connector test ./connectors/my-conn input.json   # {"params":…,"input":…}
```

## Rules

- Same `id` as a built-in overrides it (user drop-ins win).
- Secrets are referenced as `${secret.NAME}` and resolved from the encrypted
  vault (`PUT /api/v1/secrets`) — never inlined in workflows, never logged.
- Real execution is gated by the policy module: safe-mode blocks connectors;
  outbound HTTP must pass the egress allow-list.
- Approval (`POST /workflows/{id}/approve`) validates every `connector` step
  against the installed manifests before deploying.

## WASM plugins (P4.3)

Set `executor: wasm` and ship a `module.wasm` (see `docs/decisions.md` D2 for
the `ff` host ABI: `result`, `log`, `http_request`, `response`). Plugins run
with a 32 MiB memory cap and a 5s timeout; the only network path is the
egress-gated `ff.http_request`. Test with `flowforge plugin test <dir>`.
