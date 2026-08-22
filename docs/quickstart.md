# FlowForge — 5-Minute Quickstart

From nothing to a running, approved workflow — with the single Go binary.

## 1. Build (once)

```bash
npm --prefix app install && npm --prefix app run build   # Studio UI → embedded
cp -r app/dist/* server-go/ui/dist/                      # (Windows: copy)
cd server-go && go build -o flowforge ./cmd/flowforge
```

## 2. Run

```bash
./flowforge serve          # http://localhost:8080
```

First launch is in **setup mode**: create the admin account in the browser
(or `POST /api/v1/auth/setup`), then log in.

## 3. Author & approve (2 minutes)

1. **Studio** → describe your process → *Generate draft*.
2. Review the canvas: confidence scores, assumptions; edit anything.
3. **Approve & deploy** — nothing runs before a human approves.

## 4. Run & watch (1 minute)

- **Workflows** → *Run* (optionally pass run input like `{"total": 24000}` to
  drive condition steps).
- **Executions** → step timeline; approve the waiting human task to finish.

## 5. Take it with you

**Workflows** → *Export*: a portable `*.flow.yaml` you can validate and
preview anywhere:

```bash
./flowforge validate my-flow.flow.yaml
./flowforge run my-flow.flow.yaml        # execution plan preview
```

## Beyond the basics

- **Templates** — Overview → *Start from a template* (six proven patterns).
- **Connectors** — `type: connector` steps call typed integrations
  (Admin → Connectors); add your own by dropping a directory with a
  `connector.yaml` into `FLOWFORGE_CONNECTOR_DIR`:
  ```bash
  ./flowforge connectors                    # list installed
  ./flowforge connector validate ./my-conn  # validate a drop-in
  ./flowforge connector test ./my-conn      # dry-run (redacted preview)
  ```
- **Plugins** — WASM executors run sandboxed (memory cap + timeout,
  egress-gated HTTP): `./flowforge plugin test ./my-plugin`.
- **Secrets** — `PUT /api/v1/secrets {name, value}` (encrypted at rest);
  reference in connectors as `${secret.NAME}`.
- **Local AI** — Admin → AI authoring model → Ollama / LM Studio.
