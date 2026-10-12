import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

// outDir 指向 Go embed 目录，构建后 go build 直接嵌入
export default defineConfig({
  plugins: [vue(), tailwindcss()],
  build: {
    outDir: '../internal/static/dist',
    emptyOutDir: true,
  },
  server: {
    port: 5175,
    proxy: {
      '/api': 'http://127.0.0.1:8080',
    },
  },
})
