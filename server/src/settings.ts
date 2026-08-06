import type { DB } from './db.js';

// ---------------------------------------------------------------------------
// Persistent AI authoring configuration. Stored in the `settings` table and
// editable at runtime from the Admin console (no restart needed). Falls back
// to environment variables when nothing is stored.
// ---------------------------------------------------------------------------

export type ProviderId = 'openai' | 'openrouter' | 'groq' | 'together' | 'ollama' | 'lmstudio' | 'custom';

export interface AIConfig {
  provider: ProviderId;
  apiKey: string;
  baseURL: string;
  model: string;
}

export interface ProviderPreset {
  id: ProviderId;
  label: string;
  baseURL: string;
  defaultModel: string;
  needsKey: boolean;
  local: boolean;
  hint: string;
}

export const PROVIDERS: ProviderPreset[] = [
  { id: 'openai', label: 'OpenAI', baseURL: 'https://api.openai.com/v1', defaultModel: 'gpt-4o-mini', needsKey: true, local: false, hint: 'Get a key at platform.openai.com' },
  { id: 'openrouter', label: 'OpenRouter', baseURL: 'https://openrouter.ai/api/v1', defaultModel: 'anthropic/claude-3.5-sonnet', needsKey: true, local: false, hint: 'One key for Anthropic, Google, Meta and more' },
  { id: 'groq', label: 'Groq', baseURL: 'https://api.groq.com/openai/v1', defaultModel: 'llama-3.3-70b-versatile', needsKey: true, local: false, hint: 'Very fast inference for open models' },
  { id: 'together', label: 'Together AI', baseURL: 'https://api.together.xyz/v1', defaultModel: 'meta-llama/Llama-3.3-70B-Instruct-Turbo', needsKey: true, local: false, hint: 'Hosted open models' },
  { id: 'ollama', label: 'Ollama (Local)', baseURL: 'http://localhost:11434/v1', defaultModel: 'llama3.1', needsKey: false, local: true, hint: 'Runs on your machine — no key, works offline. Install at ollama.com' },
  { id: 'lmstudio', label: 'LM Studio (Local)', baseURL: 'http://localhost:1234/v1', defaultModel: 'local-model', needsKey: false, local: true, hint: 'Local server with an OpenAI-compatible endpoint' },
  { id: 'custom', label: 'Custom (OpenAI-compatible)', baseURL: '', defaultModel: 'gpt-3.5-turbo', needsKey: true, local: false, hint: 'Any endpoint speaking the OpenAI Chat Completions API (incl. Azure-style proxies)' },
];

const SETTINGS_KEY = 'ai';

function readStored(d: DB): Partial<AIConfig> {
  try {
    const row = d.db.prepare('SELECT value FROM settings WHERE key = ?').get(SETTINGS_KEY) as { value?: string } | undefined;
    return row?.value ? JSON.parse(row.value) : {};
  } catch {
    return {};
  }
}

function writeStored(d: DB, cfg: AIConfig) {
  d.db.prepare(
    'INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value',
  ).run(SETTINGS_KEY, JSON.stringify(cfg));
}

export function presetOf(provider: ProviderId): ProviderPreset {
  return PROVIDERS.find((p) => p.id === provider) ?? PROVIDERS[0];
}

export function isLLMActive(cfg: AIConfig): boolean {
  return presetOf(cfg.provider).local ? true : cfg.apiKey.trim().length > 0;
}

export function getAIConfig(d: DB): AIConfig {
  const s = readStored(d);
  const provider = (s.provider as ProviderId) || (process.env.OPENAI_API_KEY ? 'openai' : 'ollama');
  const preset = presetOf(provider);
  return {
    provider,
    apiKey: s.apiKey || process.env.OPENAI_API_KEY || '',
    baseURL: s.baseURL || preset.baseURL || process.env.OPENAI_BASE_URL || 'https://api.openai.com/v1',
    model: s.model || preset.defaultModel || process.env.OPENAI_MODEL || 'gpt-4o-mini',
  };
}

export function setAIConfig(d: DB, incoming: Partial<AIConfig>): AIConfig {
  const cur = getAIConfig(d);
  const provider = (incoming.provider as ProviderId) || cur.provider;
  const preset = presetOf(provider);
  const next: AIConfig = {
    provider,
    baseURL: incoming.baseURL?.trim() || preset.baseURL || cur.baseURL,
    model: incoming.model?.trim() || preset.defaultModel || cur.model,
    // Empty/absent key preserves the existing one (so model/URL can change without re-entering it).
    apiKey: incoming.apiKey && incoming.apiKey.trim() ? incoming.apiKey.trim() : cur.apiKey,
  };
  writeStored(d, next);
  return next;
}

export function maskKey(key: string): string {
  if (!key) return '';
  if (key.length <= 8) return '••••';
  return `${key.slice(0, 3)}••••${key.slice(-4)}`;
}

export interface PublicAISettings {
  provider: ProviderId;
  baseURL: string;
  model: string;
  hasKey: boolean;
  maskedKey: string;
  active: boolean;
  activeLabel: string;
  needsKey: boolean;
  local: boolean;
  providers: ProviderPreset[];
}

export function publicAISettings(d: DB): PublicAISettings {
  const cfg = getAIConfig(d);
  const preset = presetOf(cfg.provider);
  return {
    provider: cfg.provider,
    baseURL: cfg.baseURL,
    model: cfg.model,
    hasKey: cfg.apiKey.trim().length > 0,
    maskedKey: maskKey(cfg.apiKey),
    active: isLLMActive(cfg),
    activeLabel: isLLMActive(cfg) ? `${preset.label} · ${cfg.model}` : 'Deterministic fallback (no model configured)',
    needsKey: preset.needsKey,
    local: preset.local,
    providers: PROVIDERS,
  };
}
