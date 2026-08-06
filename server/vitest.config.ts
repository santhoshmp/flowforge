import { defineConfig } from 'vitest/config';

// Server test config. Vite resolves the `.js`-extension imports used in src/
// to their `.ts` sources, so the same ESM sources run under tsx and vitest.
export default defineConfig({
  test: {
    environment: 'node',
    include: ['tests/**/*.test.ts'],
    globals: false,
    pool: 'forks',
  },
});
