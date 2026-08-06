import { describe, it, expect } from 'vitest';
import { toYAML } from '../src/yaml.js';
import { sampleWorkflow } from './helpers.js';

// Scenario IDs: DSL-01 (see docs/test-strategy.md). Feature F-DSL.

describe('dsl: flowforge/v1 serialization (DSL)', () => {
  it('DSL-01 serializes a workflow to a flowforge/v1 document', () => {
    const wf = sampleWorkflow({ name: 'Sample Flow', version: 2, approvedBy: 'Ravi', createdBy: 'Priya' });
    const y = toYAML(wf);
    expect(y).toContain('apiVersion: flowforge/v1');
    expect(y).toContain('kind: Workflow');
    expect(y).toContain('name: sample-flow');
    expect(y).toContain('version: 2');
    expect(y).toContain('createdBy: Priya');
    expect(y).toContain('approvedBy: Ravi');
    // trigger params surfaced under spec.trigger
    expect(y).toContain('event: record.created');
    // non-trigger steps listed under spec.steps
    expect(y).toContain('type: human.approval');
    expect(y).toContain('type: integration.post');
    expect(y).toContain('on_sla_breach: escalate');
  });
});
