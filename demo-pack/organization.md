# Meridian Components — the demo organization

> Fictional mid-market electronics-components manufacturer. Rotterdam HQ,
> assembly plant in Penang, ~850 employees. All data below is synthetic and
> exists only in the demo pack.

## The pain (your opening line)

*"Meridian runs procurement-to-pay, onboarding, and support escalations
across email, spreadsheets, and three ERP screens. Nothing has an audit
trail. Approvals stall in inboxes. Nobody can say where an invoice is."*

FlowForge replaces that with described, approved, observable workflows —
without replacing the ERP.

## The people (named approvers in the workflows)

| Name | Role | Appears in |
|---|---|---|
| Priya Raman | CFO | Invoice escalation · contract renewals · final approver of record |
| Aisha Khan | Controller | Vendor invoice approvals (the live task) |
| Marcus Webb | Procurement Director | Purchase orders above EUR 5K |
| Sofia Lindqvist | HR Business Partner | New-hire onboarding |
| Daniel Osei | Support Lead | Critical ticket reviews (4h SLA) |
| Tomas Herrera | Data Steward | Supplier master-data changes |
| Elena Fischer | Plant Manager, Penang | The person you "log in as" for the demo |
| Jonas de Vries | Operations Planner (new hire) | The onboarding record pending stewardship |

## Master data (golden records + two governance stories)

**Suppliers** — Shenzhen Brightway Electronics, TechSupply Global,
Nordic Copper Works, Penang Polymers (golden); **Brightway Electronics
Shenzhen** (V-1012, suspected duplicate of V-1001) and **Veloce
Logistics** (V-1006, tax-id mismatch) sit *pending stewardship* — these
power the data-governance demo.

**Customers** — Vestergaard Medical, Nordwind Automotiv, Halden Marine.
**Products** — MCU boards, copper heat spreaders, polymer housings,
cable harnesses. **Employees** — the named team above plus the new hire.

## Story lines baked into the history

- **The stuck invoice:** INV-2026-0887 (EUR 24,500, Shenzhen Brightway)
  waits for Aisha Khan — resolve it live in the demo.
- **The recovered failure:** INV-2026-0834 failed posting to the ERP
  (`blocked by egress policy`), then succeeded on retry — shows in the
  run history and on the success-rate chart (82.9%).
- **The duplicate supplier:** V-1012 is one click from being merged or
  rejected by Tomas Herrera — MDM with governance, not a free-for-all.
- **The rush PO:** PO-4515 (EUR 11,200, MCU boards) waits for Marcus Webb.
- **The line-down ticket:** TCK-9068 (Nordwind line 3) pages on-call
  after Daniel Osei approves — the 4h SLA is on the timeline.
- **The big renewal:** C-2002 Nordwind (EUR 126,000) waits for the CFO.

## Run inputs (drive scenarios live)

See [`inputs/`](inputs/). Use them with
`POST /api/v1/workflows/{id}/executions {"input": …}` or the UI Run dialog.
