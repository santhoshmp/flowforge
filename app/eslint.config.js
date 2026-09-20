import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'

export default defineConfig([
  globalIgnores(['dist']),
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommended,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
    },
  },
  {
    // Vendored shadcn/ui components export variant helpers (buttonVariants,
    // badgeVariants, ...) alongside components, and shared canvas/step/store
    // modules export non-component utilities by design — the fast-refresh
    // boundary rule does not fit those files.
    files: ['src/components/ui/**', 'src/components/FlowCanvas.tsx', 'src/components/step.tsx', 'src/lib/store.tsx'],
    rules: {
      'react-refresh/only-export-components': 'off',
    },
  },
  {
    // react-hooks v7 flags synchronous setState in effects. Our deliberate
    // load-on-mount patterns (store bootstrap, metrics/config fetch-then-set)
    // are safe and standard; keep them visible as warnings instead of errors.
    files: ['src/**/*.{ts,tsx}'],
    rules: {
      'react-hooks/set-state-in-effect': 'warn',
    },
  },
])
