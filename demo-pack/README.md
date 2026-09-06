# Meridian Components — FlowForge Demo Pack

A ready-to-run demo organization for FlowForge: a fictional mid-market
electronics manufacturer with named people, golden-record master data,
six departmental workflows, and two weeks of realistic run history —
so the dashboards, human-task queue, and audit trail are alive from
the first click.

## Load it

```bash
flowforge demo          # loads the org into DB_PATH (default flowforge.db)
flowforge serve         # open http://localhost:8080, create the admin, explore
```

`flowforge demo` is idempotent — run it again any time to reset the
Meridian data to its scripted state.

## What's inside

| | |
|---|---|
| [`organization.md`](organization.md) | The company, its people, and the story lines |
| [`inputs/`](inputs/) | Run inputs for driving each scenario live |
| [`../server-go/internal/demopack/workflows/`](../server-go/internal/demopack/workflows/) | The six `flowforge/v1` workflow artifacts (embedded in the binary) |
| [`../docs/demo-runbook.md`](../docs/demo-runbook.md) | The scripted demo: 5-minute and 20-minute paths |

## The six workflows

| Workflow | Department | Live task waiting on |
|---|---|---|
| Vendor invoice approval | Finance | Aisha Khan (Controller) |
| Purchase order approval | Procurement | Marcus Webb (Procurement Director) |
| Employee onboarding | HR | Sofia Lindqvist (HR Business Partner) |
| Support ticket routing | Support | Daniel Osei (Support Lead) |
| Supplier master change | Data governance | Tomas Herrera (Data Steward) |
| Customer contract renewal | Sales/CFO | Priya Raman (CFO) |

Each has 2–8 historical runs across the last 14 days — completed,
failed-then-retried, cancelled, and one live human task — so every
screen has something true to show.
