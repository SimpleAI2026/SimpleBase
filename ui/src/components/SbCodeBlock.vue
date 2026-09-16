<template>
  <pre class="sb-code" :style="{ maxHeight }"><slot>{{ formatted }}</slot></pre>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { formatJson } from '../utils/format'

/** 统一代码/JSON 展示块（替代各页复制的 .sb-json / .sb-result 样式） */
const props = defineProps<{
  /** 传入数据则自动 JSON 格式化；否则用 slot 内容 */
  value?: unknown
  maxHeight?: string
}>()

const formatted = computed(() =>
  props.value !== undefined ? formatJson(props.value) : undefined
)
</script>
