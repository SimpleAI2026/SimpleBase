/// <reference types="vite/client" />

// Vue SFC 模块声明：tsconfig 的 include 覆盖了 tests/**，测试大量直接
// import '@/components/xxx.vue'，缺此声明会产生成片 TS2307。
declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<Record<string, never>, Record<string, never>, unknown>
  export default component
}

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

declare module '*.md?raw' {
  const content: string
  export default content
}

declare module '*.md' {
  const content: string
  export default content
}

declare module '@docs/_meta.json' {
  const meta: {
    modules?: { id: string; title: string; order?: number }[]
  }
  export default meta
}
