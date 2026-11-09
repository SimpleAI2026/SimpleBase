<template>
  <!-- 显式高度：父容器是 min-height 的 auto 高度，h-full 百分比无法解析会塌成 0 -->
  <div
    ref="containerRef"
    class="sb-code-view w-full max-w-full min-w-0 overflow-hidden whitespace-pre-wrap [word-break:break-word] [overflow-wrap:anywhere] rounded-md border border-border"
    :style="{ height: minHeight + 'px' }"
  ></div>
</template>

<script setup lang="ts">
/**
 * Go 源码 Monaco 编辑器（ui-gofunction-plan §8.5，依赖例外：用户点名 Monaco）。
 * 只加载 editor worker（不用 TS/JSON language worker，减小体积）；
 * Go 高亮用自注册的 Monarch；查看模式 readOnly。
 */
import { onMounted, onUnmounted, ref, watch } from 'vue'
// monaco-editor 0.56 的 exports 映射（./* → ./esm/vs/*）；
// 仅加载 editor 核心（无 TS/JSON language worker，减小体积）
import * as monaco from 'monaco-editor/editor.js'
import EditorWorker from 'monaco-editor/editor/editor.worker.js?worker'
import { goMonarchLanguage } from './goMonarch'

const props = withDefaults(
  defineProps<{
    modelValue: string
    readOnly?: boolean
    minHeight?: number
  }>(),
  { readOnly: false, minHeight: 420 }
)

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const containerRef = ref<HTMLElement>()

// 全局一次性：worker + 语言 + 主题
let bootstrapped = false
function bootstrap() {
  if (bootstrapped) return
  bootstrapped = true
  self.MonacoEnvironment = {
    getWorker() {
      return new EditorWorker()
    }
  }
  monaco.languages.register(goMonarchLanguage)
  // sb-light：与浅色主题的白色卡片、蓝色强调及冷灰中性色保持一致
  monaco.editor.defineTheme('sb-light', {
    base: 'vs',
    inherit: true,
    rules: [
      { token: 'comment', foreground: '64748b', fontStyle: 'italic' },
      { token: 'keyword', foreground: '1d4ed8' },
      { token: 'type', foreground: '0369a1' },
      { token: 'string', foreground: '047857' },
      { token: 'string.escape', foreground: '0f766e' },
      { token: 'number', foreground: '7c3aed' },
      { token: 'operator', foreground: '475569' }
    ],
    colors: {
      'editor.background': '#ffffff',
      'editor.foreground': '#0f172a',
      'editorLineNumber.foreground': '#94a3b8',
      'editor.selectionBackground': '#dbeafe',
      'editor.lineHighlightBackground': '#f1f5f9'
    }
  })
  // sb-dark：与 style.css `.dark` 的卡片、正文和蓝色强调保持一致
  monaco.editor.defineTheme('sb-dark', {
    base: 'vs-dark',
    inherit: true,
    rules: [
      { token: 'comment', foreground: '94a3b8', fontStyle: 'italic' },
      { token: 'keyword', foreground: '93c5fd' },
      { token: 'type', foreground: '7dd3fc' },
      { token: 'string', foreground: '86efac' },
      { token: 'string.escape', foreground: '5eead4' },
      { token: 'number', foreground: 'c4b5fd' },
      { token: 'operator', foreground: 'cbd5e1' }
    ],
    colors: {
      'editor.background': '#111c2e',
      'editor.foreground': '#e2e8f0',
      'editorLineNumber.foreground': '#94a3b8',
      'editor.selectionBackground': '#1d3556',
      'editor.lineHighlightBackground': '#1b2940'
    }
  })
}

function themeName(): 'sb-light' | 'sb-dark' {
  return document.documentElement.classList.contains('dark') ? 'sb-dark' : 'sb-light'
}

function syncTheme() {
  editor?.updateOptions({ theme: themeName() })
}

let editor: monaco.editor.IStandaloneCodeEditor | null = null
let themeObserver: MutationObserver | null = null

onMounted(() => {
  bootstrap()
  syncTheme()
  if (!containerRef.value) return
  editor = monaco.editor.create(containerRef.value, {
    value: props.modelValue,
    language: 'go',
    theme: themeName(),
    readOnly: props.readOnly,
    // 长行在编辑器宽度内折行，保留原行缩进，避免把弹窗撑宽
    wordWrap: 'on',
    wrappingIndent: 'same',
    tabSize: 4,
    minimap: { enabled: false },
    fontFamily:
      'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
    fontSize: 13,
    lineNumbers: 'on',
    scrollBeyondLastLine: false,
    automaticLayout: true,
    scrollbar: { verticalScrollbarSize: 8, horizontalScrollbarSize: 8 }
  })
  editor.onDidChangeModelContent(() => {
    emit('update:modelValue', editor!.getValue())
  })
  syncTheme()
  themeObserver = new MutationObserver(syncTheme)
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
})

watch(
  () => props.readOnly,
  (ro) => editor?.updateOptions({ readOnly: ro })
)

// 外部值变更（如切换文件）时同步进编辑器，避免光标抖动触发回写
watch(
  () => props.modelValue,
  (v) => {
    if (editor && editor.getValue() !== v) {
      editor.setValue(v)
    }
  }
)

onUnmounted(() => {
  themeObserver?.disconnect()
  themeObserver = null
  editor?.dispose()
  editor = null
})
</script>
