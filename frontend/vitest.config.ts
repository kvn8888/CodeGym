import { fileURLToPath, URL } from 'node:url';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  define: {
    // Tests must opt into fixtures explicitly by stubbing fetch.
    'import.meta.env.VITE_USE_MOCK_API': JSON.stringify('false'),
  },
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    env: {
      VITE_USE_MOCK_API: 'false',
    },
  },
});
