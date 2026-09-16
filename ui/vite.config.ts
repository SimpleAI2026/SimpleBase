import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// SIMPLEBASE_DEV_API_PROXY 由 ./build.sh dev 注入，默认本机 8080
const apiTarget = process.env.SIMPLEBASE_DEV_API_PROXY || 'http://127.0.0.1:8080'
const wsTarget = apiTarget.replace(/^http/, 'ws')

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/v1': { target: apiTarget, changeOrigin: true },
      '/health': { target: apiTarget, changeOrigin: true },
      '/ws': { target: wsTarget, changeOrigin: true, ws: true }
    }
  }
})
