import type { Workflow, WorkflowStep } from './types.js';

// Serialize a workflow to the flowforge/v1 DSL (YAML) — the single artifact
// consumed by the editor, the central API, and the portable runner.

const yq = (v: string) => (/[:#\n"']/.test(v) ? `"${v.replace(/"/g, '\\"')}"` : v);

function stepYaml(s: WorkflowStep): string {
  const lines = [
    `  - id: ${s.id.split('_').slice(0, -1).join('_') || s.id}`,
    `    type: ${s.type}`,
    `    name: ${yq(s.name)}`,
  ];
  const entries = Object.entries(s.params);
  if (entries.length) {
    lines.push('    params:');
    for (const [k, v] of entries) lines.push(`      ${k}: ${yq(String(v))}`);
  }
  if (s.type === 'human.approval') {
    lines.push('    on_sla_breach: escalate');
  }
  return lines.join('\n');
}

function triggerYaml(wf: Workflow): string {
  const t = wf.steps.find((s) => s.type === 'trigger');
  if (!t) return '    event: manual';
  return Object.entries(t.params).map(([k, v]) => `    ${k}: ${yq(String(v))}`).join('\n');
}

export function toYAML(wf: Workflow): string {
  return `# flowforge/v1 — portable workflow definition
# Run anywhere: flowforge run ${wf.name.toLowerCase().replace(/[^a-z0-9]+/g, '-')}.flow.yaml
apiVersion: flowforge/v1
kind: Workflow
metadata:
  name: ${wf.name.toLowerCase().replace(/[^a-z0-9]+/g, '-')}
  version: ${wf.version}
  createdBy: ${wf.createdBy}
  approvedBy: ${wf.approvedBy ?? 'pending'}
  authoredWith: ${yq(wf.aiModel)}
spec:
  description: ${yq(wf.description)}
  trigger:
${triggerYaml(wf)}
  steps:
${wf.steps.filter((s) => s.type !== 'trigger').map(stepYaml).join('\n')}
`;
}

export function runnerReadme(wf: Workflow): string {
  return `# ${wf.name} — standalone runner package

This package executes without any connection to the FlowForge control plane.

\`\`\`bash
docker run -v $(pwd):/flows flowforge/runner:1.0 run /flows/${wf.name.toLowerCase().replace(/[^a-z0-9]+/g, '-')}.flow.yaml
\`\`\`

- Signed manifest: version ${wf.version}, approved by ${wf.approvedBy ?? 'pending'}
- MDM lookups resolve against the bundled snapshot (./mdm-snapshot.json)
- When connectivity returns, the runner can phone home execution state (opt-in: --report-to <url>)
`;
}
