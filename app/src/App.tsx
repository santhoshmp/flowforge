import { useState } from 'react';
import { Workflow as WorkflowIcon, Home as HomeIcon, Wand2, Layers, Activity, ShieldCheck, Database, Github, FlaskConical, LayoutDashboard, LogOut } from 'lucide-react';
import { Toaster } from '@/components/ui/sonner';
import { StoreProvider, useStore } from '@/lib/store';
import type { Workflow } from '@/lib/types';
import Home from '@/sections/Home';
import Dashboard from '@/sections/Dashboard';
import Studio from '@/sections/Studio';
import Workflows from '@/sections/Workflows';
import TestLab from '@/sections/TestLab';
import Monitor from '@/sections/Monitor';
import Admin from '@/sections/Admin';
import MDM from '@/sections/MDM';
import { cn } from '@/lib/utils';
import { getUser, clearSession } from '@/lib/api';

type Page = 'home' | 'dashboard' | 'studio' | 'workflows' | 'test' | 'monitor' | 'admin' | 'mdm';

const NAV: { key: Page; label: string; icon: typeof HomeIcon; desc: string }[] = [
  { key: 'home', label: 'Overview', icon: HomeIcon, desc: 'What & why' },
  { key: 'dashboard', label: 'Dashboard', icon: LayoutDashboard, desc: 'Track & analyze' },
  { key: 'studio', label: 'Studio', icon: Wand2, desc: 'AI authoring & editor' },
  { key: 'workflows', label: 'Workflows', icon: Layers, desc: 'Deploy & export' },
  { key: 'test', label: 'Test Lab', icon: FlaskConical, desc: 'Validate & sandbox' },
  { key: 'monitor', label: 'Executions', icon: Activity, desc: 'Step tracking' },
  { key: 'admin', label: 'Admin', icon: ShieldCheck, desc: 'Monitor & audit' },
  { key: 'mdm', label: 'Master Data', icon: Database, desc: 'Golden records' },
];

function Shell() {
  const { instances, workflows } = useStore();
  const [page, setPage] = useState<Page>(() => {
    const edit = new URLSearchParams(window.location.search).get('edit');
    return edit ? 'studio' : 'home';
  });
  const [editWf, setEditWf] = useState<Workflow | null>(() => {
    const edit = new URLSearchParams(window.location.search).get('edit');
    return workflows.find((w) => w.id === edit) ?? null;
  });
  const waiting = instances.filter((i) => i.status === 'waiting').length;

  return (
    <div className="flex min-h-screen bg-slate-50/60">
      {/* Sidebar */}
      <aside className="sticky top-0 flex h-screen w-60 shrink-0 flex-col border-r bg-white">
        <div className="flex items-center gap-2.5 px-5 py-5 border-b">
          <span className="inline-flex h-9 w-9 items-center justify-center rounded-lg bg-violet-600 text-white">
            <WorkflowIcon className="h-5 w-5" />
          </span>
          <div>
            <div className="font-bold text-sm leading-tight">FlowForge</div>
            <div className="text-[10px] text-muted-foreground">describe · approve · run anywhere</div>
          </div>
        </div>
        <nav className="flex-1 space-y-1 p-3">
          {NAV.map((n) => (
            <button
              key={n.key}
              onClick={() => setPage(n.key)}
              className={cn(
                'flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left transition-colors',
                page === n.key ? 'bg-violet-50 text-violet-900' : 'text-slate-600 hover:bg-slate-100',
              )}
            >
              <n.icon className={cn('h-4 w-4 shrink-0', page === n.key ? 'text-violet-600' : 'text-slate-400')} />
              <span className="flex-1">
                <span className={cn('block text-sm', page === n.key && 'font-semibold')}>{n.label}</span>
                <span className="block text-[10px] text-muted-foreground">{n.desc}</span>
              </span>
              {n.key === 'admin' && waiting > 0 && (
                <span className="rounded-full bg-amber-100 border border-amber-300 px-1.5 py-0.5 text-[10px] font-bold text-amber-700">{waiting}</span>
              )}
            </button>
          ))}
        </nav>
        <div className="border-t p-4 space-y-2">
          <div className="rounded-lg bg-muted/60 border px-3 py-2 text-[10px] text-muted-foreground leading-relaxed">
            <span className="font-semibold text-foreground">Prototype</span> — AI authoring & API are mocked; the UX loop is real.
          </div>
          <a className="flex items-center gap-1.5 text-[11px] text-muted-foreground hover:text-foreground px-1" href="#" onClick={(e) => e.preventDefault()}>
            <Github className="h-3.5 w-3.5" /> Apache-2.0 · free for everyone
          </a>
          {getUser() && (
            <button onClick={() => { clearSession(); window.location.reload(); }} className="flex w-full items-center gap-1.5 rounded-md border px-2 py-1.5 text-[11px] text-muted-foreground hover:border-rose-200 hover:bg-rose-50 hover:text-rose-600">
              <LogOut className="h-3.5 w-3.5" /> Sign out · {getUser()}
            </button>
          )}
        </div>
      </aside>

      {/* Main */}
      <main className="flex-1 px-8 py-8 max-w-6xl">
        {page === 'home' && (
          <Home
            onGoStudio={() => setPage('studio')}
            onEditWorkflow={(wf) => { setEditWf(wf); setPage('studio'); }}
          />
        )}
        {page === 'dashboard' && <Dashboard onGoMonitor={() => setPage('monitor')} />}
        {page === 'studio' && <Studio onGoMonitor={() => setPage('monitor')} editWorkflow={editWf} onEditDone={() => setEditWf(null)} />}
        {page === 'workflows' && <Workflows onGoMonitor={() => setPage('monitor')} onEdit={(wf) => { setEditWf(wf); setPage('studio'); }} />}
        {page === 'test' && <TestLab />}
        {page === 'monitor' && <Monitor />}
        {page === 'admin' && <Admin />}
        {page === 'mdm' && <MDM />}
      </main>
      <Toaster position="bottom-right" />
    </div>
  );
}

export default function App() {
  return (
    <StoreProvider>
      <Shell />
    </StoreProvider>
  );
}
