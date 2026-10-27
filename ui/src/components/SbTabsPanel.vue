<template>
  <Tabs :model-value="modelValue" class="gap-1" @update:model-value="onUpdate">
    <TabsList :variant="variant" :class="['max-w-full overflow-x-auto', variant === 'line' ? 'w-full justify-start' : undefined]">
      <TabsTrigger v-for="item in items" :key="item.value" :value="item.value" class="px-3">
        {{ item.label }}
      </TabsTrigger>
    </TabsList>
    <div :class="contentClass" :style="contentStyle">
      <TabsContent v-for="item in items" :key="item.value" :value="item.value" :class="itemContentClass">
        <slot v-if="$slots[item.value]" :name="item.value" />
        <slot v-else />
      </TabsContent>
    </div>
  </Tabs>
</template>

<script setup lang="ts">
/**
 * SbTabsPanel：统一的多页签面板骨架。
 *
 * SettingsModal 与 KV 面板此前各自复制了
 * 「Tabs + TabsList(可带 variant=line) + 滚动容器 + TabsContent」这套结构，
 * 页签增删要改多处。这里收敛成 items + 每个 value 一个具名 slot。
 *
 * - `variant='line'`：下划线页签（设置面板用）。
 * - `variant='default'`：胶囊页签（KV 面板用）。
 * - 提供对应 value 的具名 slot 时用它，否则回退到默认 slot。
 */
import { computed } from 'vue'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

export interface SbTabsPanelItem {
  /** 稳定标识，同时作为 TabsTrigger 的 value */
  value: string
  /** 已翻译的页签文案 */
  label: string
}

export interface SbTabsPanelProps {
  items: SbTabsPanelItem[]
  modelValue: string
  /** 页签样式：line（下划线）/ default（胶囊） */
  variant?: 'line' | 'default'
  /** 面板内容滚动区最大高度；不传则不限制高度 */
  contentMaxHeight?: string
  /** 内容区额外 class */
  contentClass?: string
  /** 单个 TabsContent 的额外 class */
  itemContentClass?: string
}

const props = withDefaults(defineProps<SbTabsPanelProps>(), {
  variant: 'default'
})

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const contentClass = computed(() =>
  [props.contentMaxHeight ? 'overflow-y-auto' : '', props.contentClass || ''].filter(Boolean).join(' ')
)

const contentStyle = computed(() =>
  props.contentMaxHeight ? { maxHeight: props.contentMaxHeight } : undefined
)

function onUpdate(value: string | number) {
  emit('update:modelValue', String(value))
}
</script>
