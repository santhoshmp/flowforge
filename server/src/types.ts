// FlowForge core domain types — mirrors the open DSL spec (flowforge/v1).
// Kept in sync with the frontend src/lib/types.ts until extracted to a shared package.

export type StepType =
  | 'trigger'
  | 'ai.extract'
  | 'ai.classify'
  | 'mdm.lookup'
  | 'mdm.validate'
  | 'condition'
  | 'human.approval'
  | 'notify'
  | 'integration.post'
  | 'integration.http'
  | 'script'
  | 'wait';

export type StepStatus = 'pending' | 'running' | 'waiting' | 'succeeded' | 'failed' | 'skipped';
export type InstanceStatus = 'running' | 'waiting' | 'failed' | 'completed' | 'cancelled';
export type WorkflowStatus = 'draft' | 'approved' | 'deployed';

export interface ControlDef {
  key: string;
  label: string;
  color: string;
  icon: string;
  enabled: boolean;
  custom?: boolean;
  description?: string;
}

export interface WorkflowStep {
  id: string;
  type: string;
  name: string;
  params: Record<string, string>;
  confidence: number;
  assumptions: string[];
  edited?: boolean;
  position?: { x: number; y: number };
}

export interface Workflow {
  id: string;
  name: string;
  description: string;
  prompt: string;
  status: WorkflowStatus;
  version: number;
  steps: WorkflowStep[];
  createdBy: string;
  approvedBy?: string;
  aiModel: string;
  createdAt: string;
  runs: number;
}

export interface StepRun {
  stepId: string;
  name: string;
  type: string;
  status: StepStatus;
  startedAt?: string;
  durationMs?: number;
  output?: string;
  note?: string;
}

export interface Instance {
  id: string;
  workflowId: string;
  workflowName: string;
  status: InstanceStatus;
  entity?: string;
  startedAt: string;
  endedAt?: string;
  input?: Record<string, unknown>;
  autoApprove?: boolean;
  stepRuns: StepRun[];
  currentStep: number;
  waitingOn?: string;
  error?: string;
}

export interface AuditEntry {
  id: string;
  at: string;
  actor: string;
  action: string;
  detail: string;
  kind: 'ai' | 'approval' | 'deploy' | 'execution' | 'mdm' | 'export';
}

export interface MDMRecord {
  id: string;
  [key: string]: string;
}

export interface MDMEntity {
  key: string;
  label: string;
  icon: string;
  fields: string[];
  records: MDMRecord[];
}

export const STEP_META: Record<StepType, { label: string; color: string; icon: string }> = {
  trigger: { label: 'Trigger', color: 'emerald', icon: 'Zap' },
  'ai.extract': { label: 'AI Extract', color: 'violet', icon: 'Sparkles' },
  'ai.classify': { label: 'AI Classify', color: 'violet', icon: 'Sparkles' },
  'mdm.lookup': { label: 'MDM Lookup', color: 'amber', icon: 'Database' },
  'mdm.validate': { label: 'MDM Validate', color: 'amber', icon: 'ShieldCheck' },
  condition: { label: 'Condition', color: 'sky', icon: 'GitBranch' },
  'human.approval': { label: 'Human Approval', color: 'rose', icon: 'UserCheck' },
  notify: { label: 'Notify', color: 'indigo', icon: 'Bell' },
  'integration.post': { label: 'Post to System', color: 'cyan', icon: 'ArrowRightLeft' },
  'integration.http': { label: 'HTTP Call', color: 'cyan', icon: 'Globe' },
  script: { label: 'Script', color: 'slate', icon: 'Code' },
  wait: { label: 'Wait / SLA', color: 'orange', icon: 'Timer' },
};

export interface GeneratedDraft {
  name: string;
  description: string;
  steps: WorkflowStep[];
  model: string;
  overallConfidence: number;
}
