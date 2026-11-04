/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// ADR-0004: the frontend talks to the backend exclusively through Wails
// bindings, so there is no local HTTP API to proxy to in dev mode.
export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    // globals: true lets @testing-library/react register its automatic
    // per-test DOM cleanup (afterEach hook).
    globals: true,
    setupFiles: ['./src/vitest.setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
  },
});
