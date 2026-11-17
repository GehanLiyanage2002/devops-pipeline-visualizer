import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    host: true,
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://[::1]:8080',
        changeOrigin: true,
      },
      '/ws': {
        target: 'ws://[::1]:8080',
        ws: true,
        changeOrigin: true,
      },
    },
  },
  build: {
    chunkSizeWarningLimit: 1600,
    rollupOptions: {
      output: {
        manualChunks: {
          'vendor-react': ['react', 'react-dom', 'zustand'],
          'vendor-xyflow': ['@xyflow/react', 'lucide-react'],
          'vendor-elk': ['elkjs/lib/elk.bundled.js'],
        },
      },
    },
  },
});
