/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

// ADR-0004: the frontend talks to the backend exclusively through Wails
// bindings, so there is no local HTTP API to proxy to in dev mode.
export default defineConfig({
  plugins: [tailwindcss(), react()],
  server: {
    fs: {
      // @ha/ui is consumed as source through a file: link whose real path
      // lives outside this workspace; allow serving its raw SVG assets.
      allow: ['../..', '../../ha-ui'],
    },
  },
  // The app bundles to a single chunk (Radix + React). As a Wails desktop
  // app the assets load from the local disk, so code splitting buys nothing;
  // raise the warning limit instead (PENDING-DECISIONS #12, accepted status
  // quo).
  build: {
    chunkSizeWarningLimit: 700,
  },
  test: {
    environment: 'jsdom',
    // globals: true lets @testing-library/react register its automatic
    // per-test DOM cleanup (afterEach hook).
    globals: true,
    setupFiles: ['./src/vitest.setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
  },
});
