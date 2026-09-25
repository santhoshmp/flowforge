# Leave App — FlowForge integration

A standalone employee-facing leave application whose approval process runs
entirely on FlowForge.

- **`server.mjs`** — the app: serves the React page (`web/dist`), owns
  balances + org map + request history, submits via the FlowForge webhook,
  polls live status, and exposes the two workflow-facing routes
  (`/api/balance/:emp?days=`, `/api/flowforge/callback`).
- **`web/`** — the tiny React page (Vite, zero UI deps): identity picker,
  Apply form (auto day count), My Requests with live pills + notifications,
  and an Approvals inbox routed by the org map (manager gate) and role (HR).

## Design (as built)

- **Only the employee number changes hands** — routing stays in the workflow
  design + the app's org map (employee № → manager sees the card; HR role
  sees final-approval cards).
- **Approvals happen in the leave app**: ✓ → `POST /executions/{id}/approve`,
  ✕ → `.../cancel`.
- **Balance check is a live validation before final approval**: a connector
  step calls the app's balance API mid-flight. 200 → proceed to HR; 409 →
  the run fails. The app also pre-checks at submit (no pointless requests).
- **Notifications live in the leave app**: the final connector posts the
  verdict; the app deducts (idempotently) and notifies.

### Verdict semantics

| Engine state | Meaning | Leave app action |
|---|---|---|
| `completed` | Approved | callback → deduct balance, notify |
| `failed` at balance step | Rejected — insufficient balance | show rejection (no callback) |
| `cancelled` | Rejected by approver | show rejection (no callback) |
| `waiting` | In flight | pill shows the gate (manager / HR review) |

## Run it

```bash
# 1. build the page once
npm --prefix leave-app/web install && npm --prefix leave-app/web run build

# 2. FlowForge with the drop-in connectors (scripts/run-local.ps1 sets this)
FLOWFORGE_CONNECTOR_DIR=./connectors server-go/flowforge.exe serve

# 3. the leave app (:9090) — page at http://localhost:9090
node leave-app/server.mjs
#    env: FF_URL/FF_USER/FF_PASS (FlowForge service account),
#         LISTEN, LEAVE_TOKEN (must match the CALLBACK_TOKEN secret)

# 4. one-time secrets on FlowForge:
#    LEAVE_APP_URL=http://localhost:9090   CALLBACK_TOKEN=dev-leave-token
# 5. one-time workflow deploy:
#    flowforge import leave-app/workflow/leave-request.flow.yaml  (+ approve)
```

Seeded people (pick one in the identity bar): E-4003 Sofia (18d),
E-4007 Elena (12d, manager of Sofia & Jonas), E-4416 Aisha / E-4006 Priya
(HR), E-4417 Jonas (**2d** — insufficient-balance story). Balances and
requests persist in `leave-app/data.json` (gitignored).

`stub/server.mjs` (Phase 1) remains as a minimal reference service.

