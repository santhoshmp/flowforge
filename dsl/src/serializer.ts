import type { WorkflowSpec } from './types.js';

// Canonical YAML serializer for flowforge/v1. Deterministic output so that
// serialize -> parse round-trips losslessly (see tests: DSL-02).

const slug = (s: string): string => s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');

// Quote a scalar if it contains YAML-significant characters OR would otherwise
// be coerced to a non-string type (number / bool / null) on parse. Params are
// strings by contract, so "48" must round-trip as a string, not an int.
const NUM = /^[-+]?(?:\d+(?:[.]\d*)?|[.]\d+)(?:[eE][-+]?\d+)?$/;
const BOOLNULL = /^(?:true|false|null|~|yes|no|on|off)$/i;
const yq = (v: string): string => {
  if (v === '') return '""';
  if (NUM.test(v) || BOOLNULL.test(v) || /[:#\n"'\[\]{}&,!*|>%@`]/.test(v) || /^\s|\s$|^\W/.test(v)) {
    return `"${v.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;
  }
  return v;
};

function stepYaml(s: WorkflowSpec['spec']['steps'][number]): string {
  const lines = [`  - id: ${yq(s.id)}`, `    type: ${s.type}`, `    name: ${yq(s.name)}`];
  const entries = Object.entries(s.params ?? {});
  if (entries.length) {
    lines.push('    params:');
    for (const [k, v] of entries) lines.push(`      ${k}: ${yq(String(v))}`);
  }
  if (s.on_sla_breach) lines.push(`    on_sla_breach: ${yq(s.on_sla_breach)}`);
  return lines.join('\n');
}

function triggerYaml(spec: WorkflowSpec): string {
  const t = spec.spec.trigger;
  const entries = Object.entries(t).filter(([k]) => k !== 'event');
  const lines = [`    event: ${yq(t.event)}`];
  for (const [k, v] of entries) lines.push(`    ${k}: ${yq(String(v))}`);
  return lines.join('\n');
}

export function toYAML(spec: WorkflowSpec): string {
  const m = spec.metadata;
  const lines: string[] = [
    '# flowforge/v1 — portable workflow definition',
    `# Run anywhere: flowforge run ${slug(m.name)}.flow.yaml`,
    `apiVersion: ${spec.apiVersion}`,
    `kind: ${spec.kind}`,
    'metadata:',
    `  name: ${slug(m.name)}`,
    `  version: ${m.version}`,
    `  createdBy: ${yq(m.createdBy)}`,
  ];
  if (m.approvedBy) lines.push(`  approvedBy: ${yq(m.approvedBy)}`);
  if (m.authoredWith) lines.push(`  authoredWith: ${yq(m.authoredWith)}`);

  lines.push('spec:');
  if (spec.spec.description) lines.push(`  description: ${yq(spec.spec.description)}`);
  lines.push('  trigger:');
  lines.push(triggerYaml(spec));
  lines.push('  steps:');
  for (const s of spec.spec.steps) lines.push(stepYaml(s));

  return lines.join('\n') + '\n';
}
