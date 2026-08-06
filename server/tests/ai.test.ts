import { describe, it, expect } from 'vitest';
import { generateDraft } from '../src/ai.js';
import type { AIConfig } from '../src/settings.js';

// Scenario IDs: AI-01, AI-02 (see docs/test-strategy.md). Feature F-AI.
// Deterministic path only (no model configured) — LLM live tests are separate.

const inactive: AIConfig = { provider: 'openai', apiKey: '', baseURL: 'https://api.openai.com/v1', model: 'gpt-4o-mini' };

const KNOWN = new Set([
  'trigger', 'ai.extract', 'ai.classify', 'mdm.lookup', 'mdm.validate', 'condition',
  'human.approval', 'notify', 'integration.post', 'integration.http', 'script', 'wait',
]);

const PROMPT = 'When a vendor invoice over $10K arrives, extract line items, validate against the vendor master, route to the manager for approval, then post to the ERP.';

describe('ai authoring (AI)', () => {
  it('AI-01 falls back to a deterministic, well-formed draft when no model is configured', async () => {
    const res = await generateDraft(PROMPT, inactive);
    expect(res.source).toBe('fallback');
    expect(res.draft.steps.length).toBeGreaterThan(1);
    // exactly one trigger, and it is first
    const triggers = res.draft.steps.filter((s) => s.type === 'trigger');
    expect(triggers).toHaveLength(1);
    expect(res.draft.steps[0].type).toBe('trigger');
    // every step type is a known control
    for (const s of res.draft.steps) expect(KNOWN.has(s.type)).toBe(true);
    // confidence in [50,99]
    for (const s of res.draft.steps) {
      expect(s.confidence).toBeGreaterThanOrEqual(50);
      expect(s.confidence).toBeLessThanOrEqual(99);
    }
    expect(res.draft.overallConfidence).toBeGreaterThan(0);
  });

  it('AI-02 infers condition + human approval from the prompt', async () => {
    const res = await generateDraft(PROMPT, inactive);
    expect(res.draft.steps.some((s) => s.type === 'condition')).toBe(true);
    expect(res.draft.steps.some((s) => s.type === 'human.approval')).toBe(true);
    expect(res.draft.steps.some((s) => s.type === 'integration.post')).toBe(true);
  });
});
