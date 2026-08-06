# FlowForge

**Describe it. Review it. Run it anywhere.**

FlowForge is an open, lightweight enterprise workflow platform. You describe a business process in plain language, AI drafts the workflow, a human reviews and approves it in a visual editor — and the result runs centrally via API, or downloads as a single portable file that executes anywhere, even air-gapped.

> **Status:** early prototype. The AI authoring loop, review editor, live step tracking, admin console, MDM module, and YAML export are demonstrated end-to-end in the prototype UI.

---

## Why FlowForge?

IBM BPM, jBPM, and Drools are powerful — and heavy. They demand BPMN/XML specialists, a big server footprint, and weeks to first workflow. Business users end up filing tickets instead of building automations.

FlowForge is the deliberate opposite:

| | The heavyweight way | The FlowForge way |
|---|---|---|
| Authoring | Weeks of BPMN/XML by specialists | One sentence → reviewed draft in under a minute |
| Artifact | Proprietary, server-bound | One human-readable `.flow.yaml` you own |
| Execution | Central only | Central API **or** standalone runner, anywhere |
| AI | Absent or ungoverned | AI drafts, **a named human must approve** — audited |
| Debugging | Black box until it breaks | Every step live on a timeline; retry from the failure |
| Data | Free-text references | Golden records via built-in master data module |

## The six pillars

1. **Conversational authoring** — natural language → typed workflow draft with **per-step confidence scores** and **explicit AI assumptions** highlighted for confirmation.
2. **Human approval as a trust primitive** — nothing executes without an approver. Every draft, edit, and approval lands on an immutable audit trail (who, when, which model).
3. **API-first orchestration** — the UI is just a client. Everything is an API call: define, validate, deploy, execute, query, cancel, retry.
4. **Portable execution** — every workflow exports as `flowforge/v1` YAML and runs offline with the standalone runner (`flowforge run invoice.flow.yaml`). MDM lookups degrade to a bundled snapshot; state can phone home when connectivity returns.
5. **Step-level observability** — live instance timelines with per-step state, inputs, outputs, and duration. Retry resumes from the failed step only.
6. **Master data management** — canonical entities (vendors, customers, products, employees) with golden records and stewardship. Workflows reference entities by ID, never free text — which is what makes AI generation reliable and executions traceable.

## Quickstart (5 minutes)

```bash
# 1. Start the platform
docker run -p 8080:8080 flowforge/server

# 2. Open the Studio at http://localhost:8080 and type:
#    "When a vendor invoice over $10K arrives, extract line items,
#     validate against the vendor master, route to the cost-center manager
#     for approval, escalate to Finance VP after 48 hours, then post to the ERP."

# 3. Review the AI draft on canvas, confirm the highlighted assumptions,
#    edit anything, then click "Approve & deploy".

# 4. Execute it centrally:
curl -X POST http://localhost:8080/api/v1/workflows/vendor-invoice-approval/executions \
  -H "Content-Type: application/json" \
  -d '{"entity": "vendors/V-10293", "input": {"total": 24310.00}}'

# 5. …or take it with you and run it with zero dependencies:
flowforge run vendor-invoice-approval.flow.yaml
```

## The workflow is a file

`flowforge/v1` is an open, versioned spec — the single artifact consumed by the AI author, the editor, the API, and the portable runner:

```yaml
apiVersion: flowforge/v1
kind: Workflow
metadata:
  name: vendor-invoice-approval
  version: 3
  createdBy: priya
  approvedBy: ravi            # required — no approval, no execution
  authoredWith: flowforge-author (local llm)
spec:
  trigger:
    event: vendor_invoice.created
  steps:
    - id: extract_line_items
      type: ai.extract
      params: { fields: "line_items, vendor, total, currency" }
    - id: validate_vendor
      type: mdm.validate
      params: { entity: vendors, match_on: "vendor_id, tax_id", on_mismatch: route_to_steward }
    - id: amount_check
      type: condition
      params: { expression: "total > 10000", on_false: auto_approve }
    - id: manager_approval
      type: human.approval
      params: { approver: Cost-Center Manager, resolve_via: hr_hierarchy, sla_hours: "48" }
      on_sla_breach: escalate
    - id: post_to_erp
      type: integration.post
      params: { system: ERP, endpoint: erp.inbound.invoices }
```

Spec changes go through public RFCs — the community builds on this file, so it is treated as sacred.

## Architecture

```
┌──────────────────────────── control plane ────────────────────────────┐
│  Studio (NL → AI draft → review → approve)   Admin console   MDM       │
│  ──────────────────────────────────────────────────────────────────   │
│  REST/GraphQL API  ·  durable orchestrator  ·  audit log  ·  vault    │
└────────────────────────────────────────────────────────────────────────┘
        ▲ central workers                       ▲ standalone runner
        │ (webhooks, polling, retries)          │ (offline · air-gapped ·
        │                                       │  phone-home optional)
        └──────────── same flowforge/v1 artifact, same execution engine ──┘
```

- **Model-agnostic AI layer** — point it at OpenAI, Anthropic, Azure, or a fully local model (Ollama). Local model + standalone runner = authoring and execution with zero external calls.
- **Human-in-the-loop twice** — at authoring time (approve the draft) and at execution time (`human.approval` steps with SLAs and escalation).
- **Durable execution** — instance state survives restarts; replay and inspect any run.

## API sketch

| Endpoint | Purpose |
|---|---|
| `POST /api/v1/workflows` | Create from prompt (AI draft) or YAML |
| `POST /api/v1/workflows/{id}/approve` | Human approval — required before deploy |
| `POST /api/v1/workflows/{id}/executions` | Start an execution (idempotency-key supported) |
| `GET  /api/v1/executions/{id}/steps` | Step-level state, outputs, durations |
| `POST /api/v1/executions/{id}/retry` | Resume from the failed step |
| `GET  /api/v1/mdm/{entity}` | Query golden records |
| `GET  /api/v1/audit` | Full audit trail |

## Roadmap

- **v0.1 (MVP)** — NL → draft → approve → run → track loop; linear + branching flows; HTTP/email/Slack connectors; MDM registry; YAML export + container runner
- **v0.2** — parallel branches, loops, sub-workflows, compensation; environments & versioning; RBAC/SSO; OpenAPI-import step generation
- **v0.3** — MDM sync connectors + stewardship UI; runner fleet telemetry; template gallery
- **v0.4** — process mining on execution history; AI copilot that learns from your edits

## Contributing

FlowForge is free for everyone, forever — Apache-2.0. The best first contribution is a **connector**: small, well-scoped, high-value. See `CONTRIBUTING.md` (coming soon), and grab anything tagged `good first issue`.

## License

[Apache-2.0](LICENSE) — use it, fork it, run it anywhere. That's the point.
