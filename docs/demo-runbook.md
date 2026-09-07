# FlowForge — Demo Runbook

| | |
|---|---|
| **Demo org** | Meridian Components ([demo-pack/organization.md](../demo-pack/organization.md)) |
| **Prep time** | ~2 minutes |
| **Paths** | 5-minute teaser · 20-minute full demo |

## Step 0 — Install FlowForge (~1 minute)

Pick one. You need the `flowforge` binary (or its container) before anything
below works.

**Option A — release binary** (recommended):

1. Download the archive for your platform from
   [github.com/santhoshmp/flowforge/releases/latest](https://github.com/santhoshmp/flowforge/releases/latest)
   (linux/darwin/windows × amd64/arm64) plus `SHA256SUMS`.
2. Verify and extract:

   ```bash
   sha256sum -c SHA256SUMS --ignore-missing
   tar xzf flowforge-vX.Y.Z-linux-amd64.tar.gz   # or: unzip the windows .zip
   ```

3. Put `flowforge` (or `flowforge.exe`) on your PATH, or just run it from
   the current directory as `./flowforge`.

**Option B — Docker:**

```bash
docker pull ghcr.io/santhoshmp/flowforge
```

The demo commands then become (both persist into the `flowforge-data` volume):

```bash
# one-off: load Meridian Components (no port needed - it exits after loading)
docker run --rm -v flowforge-data:/data ghcr.io/santhoshmp/flowforge demo

# serve
docker run -d -p 8080:8080 -v flowforge-data:/data ghcr.io/santhoshmp/flowforge
```

**Option C — build from source** (needs Go 1.25+ and Node 22):

```bash
git clone https://github.com/santhoshmp/flowforge && cd flowforge
npm --prefix app install && npm --prefix app run build
cp -r app/dist/* server-go/ui/dist/          # Windows: copy app\dist\* server-go\ui\dist\
cd server-go && go build -o flowforge ./cmd/flowforge
./flowforge version                          # smoke check
```

Sanity check either path: `flowforge version` (or
`docker run --rm ghcr.io/santhoshmp/flowforge version`) prints a version.

## Step 1 — Load the demo org and start (once)

```bash
flowforge demo      # loads Meridian Components: 6 workflows, 24 runs, master data
flowforge serve     # http://localhost:8080
```

(Docker users: the two commands in Step 0 Option B — load once with
`docker run --rm -v flowforge-data:/data … demo`, then serve.)

In the browser: create the admin account (present it as *Elena Fischer,
Plant Manager*), log in. Optional: Admin → AI authoring model → point at
Ollama/OpenAI for live AI drafting (Act 2 is stronger with a real model;
the fallback works offline).

Reset any time: stop the server, delete `flowforge.db`
(Docker: `docker volume rm flowforge-data`), then `flowforge demo` and
`flowforge serve` again.

## The 5-minute teaser

1. **Overview** — "Six workflows from one fictional company, two weeks of
   real history." Point at the template gallery.
2. **Dashboard** — 61 runs, 82.9% success, the failed-then-retried dip.
3. **Human tasks** — Admin console: six departments each waiting on a
   *named person*. Approve Aisha Khan's stuck invoice (INV-2026-0887);
   Executions shows it complete seconds later.
4. **Portability** — Workflows → export the invoice flow → show the YAML →
   `flowforge sign` / `flowforge verify` in a terminal.

## The 20-minute demo

### Act 1 — The pain (1 min)
Open `demo-pack/organization.md`: approvals in inboxes, no audit trail,
"where is invoice 0887?" Then the Dashboard: *this is the same company,
two weeks later.*

### Act 2 — Describe it (4 min)
Studio → prompt:
*"When a vendor invoice over 10K arrives, validate the vendor against
master data, route to Aisha Khan for approval, escalate to Priya Raman
if she doesn't act in 48 hours, then post to ERP."*
Show the draft: typed steps, confidence scores, assumptions. Edit a step
live (rename, adjust threshold) — point out every change is visible.

### Act 3 — The approval gate (3 min)
Try to run the draft → refused. Approve → deploy. Show the audit entry:
*AI proposed, Priya Raman disposed.* Message: **nothing runs unaudited.**

### Act 4 — Run it live (4 min)
Run the deployed invoice flow with
[`invoice-above-threshold.json`](../demo-pack/inputs/invoice-above-threshold.json)
(EUR 31,200): Executions → steps stream pending → running → **waiting on
Aisha Khan**. Approve the task → completes with the ERP step.
Then [`invoice-below-threshold.json`](../demo-pack/inputs/invoice-below-threshold.json)
(EUR 4,800): the condition step **auto-approves** — no human in the loop
for small invoices. That's policy-driven routing.

### Act 5 — Failure, observably (3 min)
Show INV-2026-0834 in history: failed on `blocked by egress policy`,
retried, completed. Click the failed run: the step-level error, the
retry that resumed *only* the failed step. Message: **failures are
first-class, and recovery doesn't replay the world.**

### Act 6 — Master data governance (3 min)
Master Data → suppliers: two records *pending stewardship* — a suspected
duplicate (V-1012) and a tax-id mismatch (V-1006). Run the
supplier-master-change flow with
[`supplier-master-change.json`](../demo-pack/inputs/supplier-master-change.json):
Tomas Herrera gets the stewardship task before anything touches the
golden record.

### Act 7 — Portability + provenance (2 min)
Workflows → export `vendor-invoice-approval` → open the YAML (a
`flowforge/v1` artifact — readable, versioned). Terminal:

```bash
flowforge keygen .
flowforge sign vendor-invoice-approval.flow.yaml
flowforge verify vendor-invoice-approval.flow.yaml
```

"It's our workflow, in a file we own, with a signature anyone can verify
offline."

### Close (1 min)
"One binary. SQLite inside. Connectors and WASM plugins when you need
more. Docker or Helm when you're ready." → README install section.

## Cheat sheet

| Moment | Where | What to show |
|---|---|---|
| Living company | Dashboard | 14-day series, 6 waiting tasks, 82.9% success |
| Governance | Admin → Human task queue | Named approvers, resolve Aisha's invoice |
| Conditions | Executions | Above vs below EUR 10K, auto-approve |
| Recovery | Executions → INV-2026-0834 | Failed step → retry → completed |
| MDM | Master Data → Suppliers | Pending-stewardship duplicate + mismatch |
| Portability | Workflows → Export | YAML artifact + sign/verify |
| Extensibility | Admin → Connectors | http-json, slack-webhook, smtp + drop-in dirs |
| Templates | Overview | Start-from-template gallery |
