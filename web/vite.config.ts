import { defineConfig } from 'vitest/config';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// The built UI is embedded in the Go binary (web/embed.go) and served at "/".
// In development, `npm run dev` proxies API calls to a local server.
export default defineConfig({
  plugins: [svelte()],
  base: './',
  build: { outDir: 'dist', emptyOutDir: false, sourcemap: false },
  server: {
    proxy: { '/api': process.env.API_URL ?? 'http://localhost:8080' },
  },
  test: {
    environment: 'jsdom',
    globals: true, // lets @testing-library/svelte clean up after each test
    setupFiles: ['./src/test-setup.ts'],
    include: ['src/**/*.test.ts'],
  },
  resolve: process.env.VITEST ? { conditions: ['browser'] } : undefined,
});
