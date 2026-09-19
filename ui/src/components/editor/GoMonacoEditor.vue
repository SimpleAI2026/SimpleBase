<template>
  <!-- 显式高度：父容器是 min-height 的 auto 高度，h-full 百分比无法解析会塌成 0 -->
  <div
    ref="containerRef"
    class="w-full min-w-0 overflow-hidden rounded-md border border-border"
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
  // sb-light：贴近现有 token（米白背景、关键字 foreground、字符串偏暖）
  monaco.editor.defineTheme('sb-light', {
    base: 'vs',
    inherit: true,
    rules: [
      { token: 'comment', foreground: '8a8578', fontStyle: 'italic' },
      { token: 'keyword', foreground: '44403c' },
      { token: 'type', foreground: '57534e' },
      { token: 'string', foreground: 'b45309' },
      { token: 'string.escape', foreground: '92400e' },
      { token: 'number', foreground: '92400e' },
      { token: 'operator', foreground: '57534e' }
    ],
    colors: {
      'editor.background': '#faf9f5',
      'editorLineNumber.foreground': 'a8a29e',
      'editor.selectionBackground': '#ece7dd',
      'editor.lineHighlightBackground': '#f5f2ea'
    }
  })
}

let editor: monaco.editor.IStandaloneCodeEditor | null = null

onMounted(() => {
  bootstrap()
  if (!containerRef.value) return
  editor = monaco.editor.create(containerRef.value, {
    value: props.modelValue,
    language: 'go',
    theme: 'sb-light',
    readOnly: props.readOnly,
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
  editor?.dispose()
  editor = null
})
</script>
