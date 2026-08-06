import { parse as parseYAML } from 'yaml';
import Ajv, { type ErrorObject } from 'ajv';
import schema from './schema.json';
import type { WorkflowSpec } from './types.js';

// Parse + validate a flowforge/v1 document (YAML or JSON text) into a typed
// WorkflowSpec. Throws SpecError with a list of human-readable problems.

const ajv = new Ajv({ allErrors: true });
const validate = ajv.compile(schema);

export class SpecError extends Error {
  readonly errors: string[];
  constructor(message: string, errors: string[] = []) {
    super(message);
    this.name = 'SpecError';
    this.errors = errors;
  }
}

function fmt(errors: ErrorObject[] | null | undefined): string[] {
  return (errors ?? []).map((e) => `${e.instancePath || '/'} ${e.message ?? ''}`.trim());
}

export function validateSpec(obj: unknown): WorkflowSpec {
  if (!validate(obj)) {
    throw new SpecError('Invalid flowforge/v1 spec', fmt(validate.errors));
  }
  return obj as unknown as WorkflowSpec;
}

export function parseSpec(text: string): WorkflowSpec {
  let obj: unknown;
  try {
    obj = parseYAML(text);
  } catch (e) {
    throw new SpecError(`Invalid YAML: ${(e as Error).message}`);
  }
  if (obj === null || typeof obj !== 'object' || Array.isArray(obj)) {
    throw new SpecError('Expected a YAML mapping (object) at the top level');
  }
  return validateSpec(obj);
}
