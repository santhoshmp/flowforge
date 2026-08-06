import { useState } from 'react';
import { Building2, Users, Package, IdCard, Plus, ShieldCheck, Database, type LucideIcon } from 'lucide-react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { useStore } from '@/lib/store';
import { toast } from 'sonner';
import { cn } from '@/lib/utils';

const ENTITY_ICONS: Record<string, LucideIcon> = { Building2, Users, Package, IdCard };

export default function MDM() {
  const { mdm, addMDMRecord, instances } = useStore();
  const [addingTo, setAddingTo] = useState<string | null>(null);
  const [form, setForm] = useState<Record<string, string>>({});

  const entity = mdm.find((e) => e.key === addingTo);

  const usedIn = () =>
    instances.filter((i) => i.stepRuns.some((s) => s.output?.toLowerCase().includes('matched'))).length;

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Master Data</h1>
          <p className="text-sm text-muted-foreground mt-1">
            The entities your workflows reason about. AI authoring maps “the vendor” to <span className="font-medium text-foreground">your actual vendor master</span> — and every execution is traceable back to a golden record.
          </p>
        </div>
        <Badge variant="outline" className="gap-1.5 text-[11px]"><Database className="h-3 w-3" /> golden records · steward-approved</Badge>
      </div>

      <Tabs defaultValue="vendors">
        <TabsList>
          {mdm.map((e) => {
            const Icon = ENTITY_ICONS[e.icon] ?? Database;
            return (
              <TabsTrigger key={e.key} value={e.key} className="gap-1.5">
                <Icon className="h-3.5 w-3.5" /> {e.label}
                <span className="ml-1 rounded-full bg-muted px-1.5 text-[10px]">{e.records.length}</span>
              </TabsTrigger>
            );
          })}
        </TabsList>

        {mdm.map((e) => (
          <TabsContent key={e.key} value={e.key} className="mt-4">
            <div className="rounded-xl border bg-card shadow-sm overflow-hidden">
              <div className="flex items-center gap-3 border-b px-5 py-3">
                <span className="font-semibold text-sm">{e.label} — golden records</span>
                <span className="text-[11px] text-muted-foreground">referenced by {usedIn()} recent executions</span>
                <Button size="sm" variant="outline" className="ml-auto h-8 gap-1.5" onClick={() => { setForm({}); setAddingTo(e.key); }}>
                  <Plus className="h-3.5 w-3.5" /> Add record
                </Button>
              </div>
              <Table>
                <TableHeader>
                  <TableRow>
                    {e.fields.map((f) => <TableHead key={f} className="text-xs">{f}</TableHead>)}
                    <TableHead className="text-xs">record</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {e.records.map((r) => (
                    <TableRow key={r.id}>
                      {e.fields.map((f) => <TableCell key={f} className="text-xs font-mono">{r[f] ?? '—'}</TableCell>)}
                      <TableCell>
                        <span className={cn('inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[10px] font-medium',
                          r.status === 'golden' ? 'bg-emerald-50 text-emerald-700 border-emerald-200' : 'bg-amber-50 text-amber-700 border-amber-200')}>
                          <ShieldCheck className="h-3 w-3" /> {r.status}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </TabsContent>
        ))}
      </Tabs>

      <div className="rounded-xl border bg-muted/40 p-4 text-xs text-muted-foreground">
        <span className="font-semibold text-foreground">How workflows use this:</span> steps reference entities by MDM ID (<code className="bg-white border rounded px-1">vendors/V-10293</code>), never free text.
        Mismatches route to a data steward instead of failing silently. Sync connectors (ERP, CRM, HRIS) keep the master fresh — a full match/merge engine is on the roadmap.
      </div>

      {/* Add record dialog */}
      <Dialog open={!!addingTo} onOpenChange={(o) => !o && setAddingTo(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader><DialogTitle>Add {entity?.label.slice(0, -1) ?? 'record'}</DialogTitle></DialogHeader>
          {entity && (
            <div className="space-y-3">
              {entity.fields.filter((f) => f !== 'status').map((f) => (
                <div key={f}>
                  <label className="text-xs font-medium text-muted-foreground">{f}</label>
                  <Input value={form[f] ?? ''} onChange={(e) => setForm({ ...form, [f]: e.target.value })} className="h-8 mt-1 text-sm" />
                </div>
              ))}
              <div className="flex justify-end gap-2 pt-2">
                <Button variant="outline" size="sm" onClick={() => setAddingTo(null)}>Cancel</Button>
                <Button size="sm" onClick={() => {
                  const id = form[entity.fields[0]] || `X-${Math.floor(Math.random() * 9000 + 1000)}`;
                  addMDMRecord(entity.key, { id, ...form, status: 'pending stewardship' });
                  setAddingTo(null);
                  toast.success('Record created', { description: 'Marked “pending stewardship” until a data steward approves it as a golden record.' });
                }}>Create</Button>
              </div>
            </div>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
