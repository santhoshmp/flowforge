import { ArrowRight, MessageSquareText, ShieldCheck, Package, Activity, Database, Blocks, Scale } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { useStore } from '@/lib/store';

const USP = [
  { icon: MessageSquareText, title: 'Say it, don’t draw it', body: 'Describe a process in plain language. AI drafts the workflow with per-step confidence scores and explicit assumptions for you to confirm.' },
  { icon: ShieldCheck, title: 'AI proposes, human disposes', body: 'Nothing executes without a named human approving the draft. Every approval lands on an immutable audit trail — governance built in, not bolted on.' },
  { icon: Package, title: 'Download it. Run it anywhere.', body: 'Every workflow is a portable, human-readable YAML file. Execute centrally via API — or on a laptop, a VPC, or an air-gapped site with the standalone runner.' },
  { icon: Activity, title: 'Every step, live', body: 'Instance timelines show each step’s state, inputs, outputs, and duration. Retry from the failed step — never re-run the whole flow.' },
  { icon: Database, title: 'Master data built in', body: 'Workflows reference vendors, customers, and employees by golden record, so AI generation is grounded and executions stay traceable.' },
  { icon: Blocks, title: 'Integrates with anything', body: 'HTTP, webhooks, Slack, email, databases, SAP, Salesforce — any OpenAPI spec becomes a typed step automatically.' },
];

export default function Home({ onGoStudio }: { onGoStudio: () => void }) {
  const { workflows, instances } = useStore();
  return (
    <div className="space-y-10">
      {/* Hero */}
      <div className="rounded-2xl border bg-gradient-to-br from-violet-50 via-white to-indigo-50 px-8 py-10">
        <div className="max-w-2xl">
          <div className="inline-flex items-center gap-2 rounded-full border bg-white px-3 py-1 text-[11px] font-medium text-muted-foreground mb-4">
            <Scale className="h-3 w-3" /> Apache-2.0 · free for everyone · self-hosted
          </div>
          <h1 className="text-3xl font-bold tracking-tight leading-tight">
            The workflow platform you can <span className="text-violet-700">describe in a sentence</span> — and run anywhere.
          </h1>
          <p className="mt-3 text-sm text-muted-foreground leading-relaxed">
            FlowForge makes enterprise workflow automation lightweight: natural-language authoring with human approval,
            a readable YAML spec you own, API-first execution, and step-level observability.
            Your first workflow takes minutes, not months.
          </p>
          <div className="mt-5 flex gap-2">
            <Button onClick={onGoStudio} className="gap-2 bg-violet-600 hover:bg-violet-700">
              Describe your first workflow <ArrowRight className="h-4 w-4" />
            </Button>
            <Button variant="outline" onClick={() => window.scrollTo({ top: document.body.scrollHeight, behavior: 'smooth' })}>
              See how it works
            </Button>
          </div>
          <div className="mt-6 flex gap-6 text-xs text-muted-foreground">
            <span><span className="font-bold text-foreground text-base">{workflows.length}</span> workflows</span>
            <span><span className="font-bold text-foreground text-base">{instances.length}</span> executions tracked</span>
            <span><span className="font-bold text-foreground text-base">1</span> YAML file per workflow</span>
          </div>
        </div>
      </div>

      {/* USP grid */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {USP.map((u) => (
          <div key={u.title} className="rounded-xl border bg-card p-5 shadow-sm hover:shadow-md transition-shadow">
            <u.icon className="h-5 w-5 text-violet-600" />
            <div className="mt-3 font-semibold text-sm">{u.title}</div>
            <p className="mt-1.5 text-xs text-muted-foreground leading-relaxed">{u.body}</p>
          </div>
        ))}
      </div>

      {/* How it works */}
      <div className="rounded-xl border bg-card p-6 shadow-sm">
        <div className="font-semibold text-sm mb-4">The loop — from sentence to production</div>
        <div className="grid gap-3 md:grid-cols-4">
          {[
            ['1 · Describe', '“When an invoice over $10K arrives, validate the vendor, route to the manager, then post to ERP.”'],
            ['2 · Review the AI draft', 'Steps appear on canvas with confidence scores and assumptions highlighted. Edit visually or by chat.'],
            ['3 · Approve & deploy', 'One click from a named approver. Instantly callable: POST /api/v1/workflows/{id}/executions'],
            ['4 · Track or take it with you', 'Watch each step live — or download the .flow.yaml and run it offline with the standalone runner.'],
          ].map(([t, b]) => (
            <div key={t} className="rounded-lg bg-muted/50 border p-4">
              <div className="text-xs font-bold text-violet-700">{t}</div>
              <p className="mt-1.5 text-[11px] text-muted-foreground leading-relaxed">{b}</p>
            </div>
          ))}
        </div>
      </div>

    </div>
  );
}
