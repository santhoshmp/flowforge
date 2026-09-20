import { useState } from 'react';
import { Building2, Users, Package, IdCard, Plus, ShieldCheck, Database, ShieldQuestion, GitMerge, Check, X, type LucideIcon } from 'lucide-react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useStore } from '@/lib/store';
import { toast } from 'sonner';
import { cn } from '@/lib/utils';

const ENTITY_ICONS: Record<string, LucideIcon> = { Building2, Users, Package, IdCard };

export default function MDM() {
  const { mdm, addMDMRecord, resolveMDMRecord, instances } = useStore();
  const [addingTo, setAddingTo] = useState<string | null>(null);
  const [form, setForm] = useState<Record<string, string>>({});
  const [merging, setMerging] = useState<{ entity: string; id: string } | null>(null);
  const [mergeTarget, setMergeTarget] = useState('');

  const entity = mdm.find((e) => e.key === addingTo);
  const mergeEntity = mdm.find((e) => e.key === merging?.entity);
  const mergeRecord = mergeEntity?.records.find((r) => r.id === merging?.id);

  const usedIn = () =>
    instances.filter((i) => i.stepRuns.some((s) => s.output?.toLowerCase().includes('matched'))).length;

  const resolve = async (entityKey: string, id: string, action: 'promote' | 'reject') => {
    try {
      await resolveMDMRecord(entityKey, { id, action });
      toast.success(action === 'promote' ? 'Record promoted to golden' : 'Record rejected',
        { description: `${id} · resolution recorded on the audit trail.` });
    } catch (e) {
      toast.error('Resolution failed', { description: (e as Error).message });
    }
  };

  const doMerge = async () => {
    if (!merging || !mergeTarget) return;
    try {
      await resolveMDMRecord(merging.entity, { id: merging.id, action: 'merge', mergeInto: mergeTarget });
      toast.success('Records merged', { description: `${merging.id} merged into ${mergeTarget} · audit trail updated.` });
      setMerging(null);
      setMergeTarget('');
    } catch (e) {
      toast.error('Merge failed', { description: (e as Error).message });
    }
  };

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
                    {e.records.some((r) => r.status === 'pending stewardship') && (
                      <TableHead className="text-xs">steward actions</TableHead>
                    )}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {e.records.map((r) => (
                    <TableRow key={r.id} className={r.status === 'pending stewardship' ? 'bg-amber-50/30' : undefined}>
                      {e.fields.map((f) => <TableCell key={f} className="text-xs font-mono">{r[f] ?? '—'}</TableCell>)}
                      <TableCell>
                        <span className={cn('inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[10px] font-medium',
                          r.status === 'golden' ? 'bg-emerald-50 text-emerald-700 border-emerald-200' : 'bg-amber-50 text-amber-700 border-amber-200')}>
                          {r.status === 'golden' ? <ShieldCheck className="h-3 w-3" /> : <ShieldQuestion className="h-3 w-3" />} {r.status}
                        </span>
                      </TableCell>
                      {e.records.some((x) => x.status === 'pending stewardship') && (
                        <TableCell>
                          {r.status === 'pending stewardship' ? (
                            <div className="flex items-center gap-1.5">
                              <Button size="sm" variant="outline" className="h-7 gap-1 px-2 text-[11px] text-emerald-700 hover:text-emerald-800"
                                onClick={() => resolve(e.key, r.id, 'promote')} title="Approve this record as a golden record">
                                <Check className="h-3 w-3" /> Promote
                              </Button>
                              <Button size="sm" variant="outline" className="h-7 gap-1 px-2 text-[11px]"
                                onClick={() => { setMerging({ entity: e.key, id: r.id }); setMergeTarget(''); }} title="Merge this record into an existing golden record">
                                <GitMerge className="h-3 w-3" /> Merge
                              </Button>
                              <Button size="sm" variant="outline" className="h-7 gap-1 px-2 text-[11px] text-rose-700 hover:text-rose-800"
                                onClick={() => resolve(e.key, r.id, 'reject')} title="Discard this record">
                                <X className="h-3 w-3" /> Reject
                              </Button>
                            </div>
                          ) : (
                            <span className="text-[10px] text-muted-foreground">—</span>
                          )}
                        </TableCell>
                      )}
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
        Mismatches route to a data steward instead of failing silently — resolve them here (promote, merge, or reject); every resolution lands on the audit trail.
      </div>

      {/* Merge dialog */}
      <Dialog open={!!merging} onOpenChange={(o) => { if (!o) { setMerging(null); setMergeTarget(''); } }}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader><DialogTitle>Merge into a golden record</DialogTitle></DialogHeader>
          {mergeEntity && (
            <div className="space-y-3">
              <p className="text-xs text-muted-foreground">
                <span className="font-mono font-medium text-foreground">{mergeRecord?.id}</span> ({mergeRecord?.name})
                will be removed and its identity folded into the target golden record. The action is audited.
              </p>
              <div>
                <label className="text-xs font-medium text-muted-foreground">Target golden record</label>
                <Select value={mergeTarget} onValueChange={setMergeTarget}>
                  <SelectTrigger className="mt-1 h-9 text-sm"><SelectValue placeholder="Choose a golden record…" /></SelectTrigger>
                  <SelectContent>
                    {mergeEntity.records.filter((r) => r.status === 'golden').map((r) => (
                      <SelectItem key={r.id} value={r.id} className="text-sm font-mono">{r.id} — {r.name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="flex justify-end gap-2 pt-2">
                <Button variant="outline" size="sm" onClick={() => { setMerging(null); setMergeTarget(''); }}>Cancel</Button>
                <Button size="sm" disabled={!mergeTarget} onClick={doMerge}>
                  <GitMerge className="h-3.5 w-3.5 mr-1.5" /> Merge
                </Button>
              </div>
            </div>
          )}
        </DialogContent>
      </Dialog>

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
