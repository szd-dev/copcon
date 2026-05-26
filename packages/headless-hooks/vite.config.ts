import { defineConfig } from 'vite';
import { resolve } from 'path';

export default defineConfig({
  build: {
    lib: {
      entry: resolve(__dirname, 'src/index.ts'),
      name: 'CopConHeadlessHooks',
      formats: ['es', 'cjs'],
      fileName: (format) => `index.${format === 'es' ? 'js' : 'cjs'}`,
    },
    rollupOptions: {
      external: ['@copcon/chat-core'],
      output: {
        globals: {
          '@copcon/chat-core': 'CopConChatCore',
        },
      },
    },
  },
});