# Leave App — FlowForge integration

A standalone employee-facing leave application whose approval process runs
entirely on FlowForge. Phase 1 (this) proves the loop with a stub service;
Phase 2 adds the tiny React page.

## Design (decided)

- **Only the employee number changes hands** — routing stays in the workflow
  design + the app's org map (employee № → manager shows the approval card).
- **Approvals happen in the leave app** (Phase 2 UI; Phase 1 used the API):
  approve → `POST /executions/{id}/approve`, reject → `.../cancel`.
- **Balance check is a live validation before final approval**: a connector
  step calls the leave app's balance API mid-flight (authoritative, not
  stale submit-time data). 200 → proceed to HR; 409 → the run fails.
- **Notifications live in the leave app**: the final connector step POSTs
  the verdict back; the app deducts and notifies.

### Verdict semantics

| Engine state | Meaning | Leave app action |
|---|---|---|
| `completed` | Approved | callback → deduct balance, notify |
| `failed` at balance step | Rejected — insufficient balance | show rejection (no callback) |
| `cancelled` | Rejected by approver | show rejection (no callback) |

## Pieces

| Piece | Where |
|---|---|
| Workflow artifact (5 steps) | `workflow/leave-request.flow.yaml` |
| `balance-check` connector | `../connectors/balance-check/` |
| `leave-callback` connector | `../connectors/leave-callback/` |
| Stub service (balance + callback + introspection) | `stub/server.mjs` |
| Secrets (server vault) | `LEAVE_APP_URL`, `CALLBACK_TOKEN` |

## Run it

```bash
# 1. leave-app stub (:9090)
node leave-app/stub/server.mjs

# 2. FlowForge with the drop-in connectors (scripts/run-local.ps1 sets this)
FLOWFORGE_CONNECTOR_DIR=./connectors server-go/flowforge.exe serve

# 3. secrets (once): PUT /api/v1/secrets
#    LEAVE_APP_URL=http://localhost:9090   CALLBACK_TOKEN=dev-leave-token

# 4. deploy the workflow
server-go/flowforge.exe import leave-app/workflow/leave-request.flow.yaml
#    approve in the UI (or POST /workflows/{id}/approve)

# 5. submit leave (external-system style)
TOK=$(curl -s -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/workflows/<id>/hook | jq -r .token)
curl -X POST http://localhost:8080/api/v1/hooks/<id> \
  -H "X-FlowForge-Token: $TOK" -H "Content-Type: application/json" \
  -d '{"employee_no":"E-4003","leave_type":"annual","from":"2026-10-05","to":"2026-10-09","days":5}'
```

Stub balances: E-4003 (18), E-4007 (12), E-4417 (**2** — drives the
insufficient-balance path). Introspection: `GET :9090/api/stub/requests`,
`GET :9090/api/stub/balances`.

## Phase 2 (next)

The tiny React page + thin Node service around these four routes:
`POST /api/leave` (submit → webhook), `GET /api/leave/mine/:emp`,
`GET /api/balance/:emp` (workflow-facing), `POST /api/flowforge/callback`
(workflow-facing) — plus approve/reject cards and the notification badge.
