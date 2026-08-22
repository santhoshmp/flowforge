// flowforge/v1 — the portable workflow spec types. This is the canonical
// contract; the control plane and runner consume exactly this shape.

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
  | 'wait'
  | 'connector';

export const STEP_TYPES: StepType[] = [
  'trigger', 'ai.extract', 'ai.classify', 'mdm.lookup', 'mdm.validate',
  'condition', 'human.approval', 'notify', 'integration.post',
  'integration.http', 'script', 'wait', 'connector',
];

export interface SpecStep {
  id: string;
  type: StepType;
  name: string;
  params?: Record<string, string>;
  on_sla_breach?: string;
}

export interface SpecTrigger {
  event: string;
  [extra: string]: string;
}

export interface SpecMetadata {
  name: string;
  version: number;
  createdBy: string;
  approvedBy?: string;
  authoredWith?: string;
}

export interface WorkflowSpec {
  apiVersion: 'flowforge/v1';
  kind: 'Workflow';
  metadata: SpecMetadata;
  spec: {
    description?: string;
    trigger: SpecTrigger;
    steps: SpecStep[];
  };
}

export const API_VERSION = 'flowforge/v1' as const;
