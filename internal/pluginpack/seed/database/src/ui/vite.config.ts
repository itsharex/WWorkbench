import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

/**
 * IIFE → globalThis.WWPack_database
 * react / react-dom / @wworkbench/host 全部 external，使用 Host 注入的全局。
 */
export default defineConfig({
  plugins: [react({ jsxRuntime: 'classic' })],
  define: {
    'process.env.NODE_ENV': JSON.stringify('production'),
  },
  build: {
    lib: {
      entry: 'src/index.ts',
      name: 'WWPack_database',
      formats: ['iife'],
      fileName: () => 'index.js',
    },
    rollupOptions: {
      external: ['react', 'react-dom', '@wworkbench/host'],
      output: {
        exports: 'named',
        globals: {
          react: 'React',
          'react-dom': 'ReactDOM',
          '@wworkbench/host': 'WWHost',
        },
      },
    },
    sourcemap: false,
    emptyOutDir: true,
  },
})
