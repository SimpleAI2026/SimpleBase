import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const rootDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const docsDir = path.resolve(rootDir, 'docs')
const srcDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), 'src')

// SIMPLEBASE_DEV_API_PROXY 由 ./build.sh dev 注入，默认本机 8080
const apiTarget = process.env.SIMPLEBASE_DEV_API_PROXY || 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: {
    alias: {
      '@docs': docsDir,
      '@': srcDir
    }
  },
  server: {
    port: 5173,
    fs: {
      // repo-root docs/ lives outside Vite root (ui/)
      allow: [rootDir, docsDir]
    },
    proxy: {
      '/v1': { target: apiTarget, changeOrigin: true },
      '/health': { target: apiTarget, changeOrigin: true },
      // 云函数调用面前缀（ui-gofunction-plan §7.2）
      '/go': { target: apiTarget, changeOrigin: true }
    }
  },
  preview: {
    fs: {
      allow: [rootDir, docsDir]
    }
  }
})
