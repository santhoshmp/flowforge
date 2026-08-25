import { toYAML as specToYAML, type SpecStep, type SpecTrigger, type WorkflowSpec } from '@flowforge/dsl';
import type { Workflow, WorkflowStep } from './types';

// Serialize a workflow to the flowforge/v1 DSL (YAML) via the shared
// @flowforge/dsl contract package — single source of truth for the artifact
// consumed by the editor, the central API, and the portable runner.

const slug = (s: string): string => s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');

// Internal step ids carry a trailing uniqueness suffix (e.g. `mgr_2`); the
// portable artifact uses the stable base id.
const specId = (id: string): string => id.split('_').slice(0, -1).join('_') || id;

function toSpecStep(s: WorkflowStep): SpecStep {
  const step: SpecStep = { id: specId(s.id), type: s.type as SpecStep['type'], name: s.name };
  const params = Object.entries(s.params ?? {}).reduce<Record<string, string>>((acc, [k, v]) => {
    acc[k] = String(v);
    return acc;
  }, {});
  if (Object.keys(params).length) step.params = params;
  if (s.type === 'human.approval') step.on_sla_breach = 'escalate';
  return step;
}

export function toSpec(wf: Workflow): WorkflowSpec {
  const trig = wf.steps.find((s) => s.type === 'trigger');
  const trigger = Object.entries(trig?.params ?? {}).reduce<SpecTrigger>(
    (acc, [k, v]) => {
      acc[k] = String(v);
      return acc;
    },
    { event: 'manual' },
  );
  return {
    apiVersion: 'flowforge/v1',
    kind: 'Workflow',
    metadata: {
      name: slug(wf.name) || 'workflow',
      version: wf.version,
      createdBy: wf.createdBy || 'unknown',
      approvedBy: wf.approvedBy ?? 'pending',
      authoredWith: wf.aiModel,
    },
    spec: {
      ...(wf.description ? { description: wf.description } : {}),
      trigger,
      steps: wf.steps.filter((s) => s.type !== 'trigger').map(toSpecStep),
    },
  };
}

export function toYAML(wf: Workflow): string {
  return specToYAML(toSpec(wf));
}

export function download(filename: string, content: string, mime = 'text/yaml') {
  const blob = new Blob([content], { type: mime });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

export function runnerReadme(wf: Workflow): string {
  return `# ${wf.name} — portable flowforge/v1 artifact

This file is the whole workflow — a human-readable YAML contract you own.

Validate and preview it anywhere with the FlowForge CLI:

\`\`\`bash
flowforge validate ${slug(wf.name)}.flow.yaml
flowforge run ${slug(wf.name)}.flow.yaml        # prints the execution plan
\`\`\`

- Manifest: version ${wf.version}, approved by ${wf.approvedBy ?? 'pending'}
- Runs on any FlowForge control plane: \`flowforge serve\` (single binary, embedded UI)
- Provenance: sign/verify with \`flowforge sign\` / \`flowforge verify\` (Ed25519 detached signatures)
`;
}
