<template>
  <Select :model-value="current" @update:model-value="onChange">
    <SelectTrigger
      size="sm"
      class="w-auto gap-0 px-2 [&>svg]:hidden"
      :class="props.class"
      :aria-label="t('layout.language')"
    >
      <SelectValue>
        <span :class="BADGE_CLASS" :data-locale="current" aria-hidden="true">{{ BADGES[current] }}</span>
      </SelectValue>
    </SelectTrigger>
    <SelectContent>
      <SelectGroup>
        <SelectItem v-for="opt in options" :key="opt.value" :value="opt.value">
          <span :class="BADGE_CLASS" :data-locale="opt.value" aria-hidden="true">{{ BADGES[opt.value] }}</span>
          <span class="sr-only">{{ opt.label }}</span>
        </SelectItem>
      </SelectGroup>
    </SelectContent>
  </Select>
</template>

<script setup lang="ts">
/**
 * 界面语言下拉：用「中 / EN」文字图标表示语言，不展示完整语言名（名称仅作无障碍文本）。
 * 顶栏与移动端侧栏共用，切换即持久化并同步 i18n 与 <html lang>（见 useLocale）。
 */
import type { HTMLAttributes } from 'vue'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useLocale } from '../composables/useLocale'
import type { LocaleCode } from '../locales'

const props = defineProps<{ class?: HTMLAttributes['class'] }>()

const BADGES: Record<LocaleCode, string> = {
  'zh-CN': '中',
  'en-US': 'EN'
}

const BADGE_CLASS =
  'inline-flex h-5 min-w-5 items-center justify-center rounded border border-current/30 px-1 text-[11px] font-semibold leading-none'

const { current, options, setLocale, t } = useLocale()

function onChange(value: string | number) {
  setLocale(value as LocaleCode)
}
</script>
