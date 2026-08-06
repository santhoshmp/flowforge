import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
import { parseSpec, validateSpec, toYAML, SpecError, type WorkflowSpec } from '../src/index.js';

const here = dirname(fileURLToPath(import.meta.url));
const fixture = (f: string) => readFileSync(resolve(here, 'fixtures', f), 'utf8');

// Scenario IDs: DSL-01, DSL-02, DSL-03 (see docs/test-strategy.md). Feature F-DSL.

const invoice: WorkflowSpec = {
  apiVersion: 'flowforge/v1',
  kind: 'Workflow',
  metadata: { name: 'vendor-invoice-approval', version: 3, createdBy: 'Priya', approvedBy: 'Ravi', authoredWith: 'flowforge-author' },
  spec: {
    description: 'Extract, validate, approve and post vendor invoices over $10K.',
    trigger: { event: 'vendor_invoice.created', source: 'any' },
    steps: [
      { id: 'extract_line_items', type: 'ai.extract', name: 'Extract line items', params: { fields: 'line_items, vendor, total, currency', model: 'auto' } },
      { id: 'validate_vendor', type: 'mdm.validate', name: 'Validate vendor against master', params: { entity: 'vendors', match_on: 'vendor_id, tax_id' } },
      { id: 'amount_check', type: 'condition', name: 'Amount over 10000', params: { expression: 'total > 10000', on_false: 'auto_approve' } },
      { id: 'manager_approval', type: 'human.approval', name: 'Cost-Center Manager approval', params: { approver: 'Cost-Center Manager', sla_hours: '48' }, on_sla_breach: 'escalate' },
      { id: 'post_to_erp', type: 'integration.post', name: 'Post to ERP', params: { system: 'ERP', endpoint: 'erp.inbound.invoices' } },
    ],
  },
};

// Corpus used for round-trip parity (the same set the Go parser must match).
const corpus: WorkflowSpec[] = [
  invoice,
  {
    apiVersion: 'flowforge/v1', kind: 'Workflow',
    metadata: { name: 'ticket-routing', version: 1, createdBy: 'Carlos' },
    spec: {
      trigger: { event: 'support_ticket.created' },
      steps: [
        { id: 'classify', type: 'ai.classify', name: 'Classify priority' },
        { id: 'lead_approval', type: 'human.approval', name: 'Support Lead approval', params: { approver: 'Support Lead', sla_hours: '4' } },
        { id: 'notify_oncall', type: 'notify', name: 'Notify on-call', params: { channel: 'slack' } },
      ],
    },
  },
];

describe('dsl: serialization (DSL-01)', () => {
  it('serializes a spec to a flowforge/v1 document', () => {
    const y = toYAML(invoice);
    expect(y).toContain('apiVersion: flowforge/v1');
    expect(y).toContain('kind: Workflow');
    expect(y).toContain('name: vendor-invoice-approval');
    expect(y).toContain('version: 3');
    expect(y).toContain('event: vendor_invoice.created');
    expect(y).toContain('type: human.approval');
    expect(y).toContain('on_sla_breach: escalate');
  });
});

describe('dsl: round-trip (DSL-02)', () => {
  for (const spec of corpus) {
    it(`round-trips "${spec.metadata.name}" (serialize -> parse == original)`, () => {
      const parsed = parseSpec(toYAML(spec));
      expect(parsed).toEqual(spec);
    });
  }

  it('parses a hand-written fixture identically to the in-memory spec', () => {
    const parsed = parseSpec(fixture('valid-invoice.flow.yaml'));
    expect(parsed).toEqual(invoice);
  });

  it('serialize(parse(fixture)) is stable', () => {
    const once = toYAML(parseSpec(fixture('valid-invoice.flow.yaml')));
    const twice = toYAML(parseSpec(once));
    expect(twice).toBe(once);
  });
});

describe('dsl: schema rejection (DSL-03)', () => {
  const valid = invoice;

  const expectInvalid = (label: string, mutate: (s: WorkflowSpec) => unknown) => {
    it(`rejects ${label}`, () => {
      const clone = JSON.parse(JSON.stringify(valid)) as WorkflowSpec;
      expect(() => validateSpec(mutate(clone))).toThrow(SpecError);
    });
  };

  expectInvalid('wrong apiVersion', (s) => ((s as WorkflowSpec & { apiVersion: string }).apiVersion = 'flowforge/v0'));
  expectInvalid('wrong kind', (s) => ((s as WorkflowSpec & { kind: string }).kind = 'Job'));
  expectInvalid('unknown step type', (s) => ((s.spec.steps[0] as WorkflowSpec['spec']['steps'][number] & { type: string }).type = 'rocket.launch'));
  expectInvalid('non-slug name', (s) => (s.metadata.name = 'Bad Name'));
  expectInvalid('missing trigger.event', (s) => (delete (s.spec.trigger as WorkflowSpec['spec']['trigger']).event));
  expectInvalid('non-string param value', (s) => ((s.spec.steps[0].params as Record<string, unknown>).fields = 42));
  expectInvalid('missing required createdBy', (s) => (delete (s.metadata as WorkflowSpec['metadata']).createdBy));

  it('parses invalid YAML into a SpecError', () => {
    expect(() => parseSpec('apiVersion: flowforge/v1\n  bad: : :')).toThrow(SpecError);
  });

  it('accepts the valid spec', () => {
    expect(() => validateSpec(JSON.parse(JSON.stringify(valid)))).not.toThrow();
  });
});
