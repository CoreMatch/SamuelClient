import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// ADR-0004: the frontend talks to the backend exclusively through Wails
// bindings, so there is no local HTTP API to proxy to in dev mode.
export default defineConfig({
  plugins: [react()],
});
