<template>
  <div class="flex flex-col gap-3" data-testid="bi-filter-bar">
    <div v-if="dateFields.length > 0" class="flex flex-wrap items-center gap-3" data-testid="bi-range-row">
      <Select v-if="dateFields.length > 1" :model-value="timeField" @update:model-value="emit('update:timeField', String($event))">
        <SelectTrigger class="w-40" :aria-label="t('components.bi.range.field')" data-field="time-field">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem v-for="f in dateFields" :key="f" :value="f">{{ f }}</SelectItem>
        </SelectContent>
      </Select>

      <ToggleGroup
        type="single"
        variant="outline"
        size="sm"
        class="h-8"
        :model-value="range"
        :aria-label="t('components.bi.range.label')"
        @update:model-value="onRange"
      >
        <ToggleGroupItem v-for="key in RANGE_KEYS" :key="key" :value="key" class="h-8 px-2.5 text-xs" :data-range="key">
          {{ t(`components.bi.range.${key}`) }}
        </ToggleGroupItem>
      </ToggleGroup>
    </div>

    <!-- 条件本身写错时所有卡片都会失败：在这里说一次，而不是让每张图各报一遍 -->
    <p v-if="error" class="text-sm text-destructive" data-testid="bi-filter-error">{{ error }}</p>

    <div class="grid gap-3 md:grid-cols-[minmax(0,1fr)_auto]">
      <Input
        :model-value="filter"
        :placeholder="t('pages.dataBrowser.filter')"
        data-field="filter"
        @update:model-value="emit('update:filter', String($event))"
        @keydown.enter="emit('apply')"
      />
      <div class="flex gap-2">
        <Button data-action="apply" @click="emit('apply')">{{ t('pages.dataBrowser.applyFilter') }}</Button>
        <Button v-if="dirty" variant="ghost" data-action="clear" @click="emit('clear')">{{ t('pages.dataBrowser.clearFilter') }}</Button>
      </div>
    </div>

    <p v-if="pending" class="text-xs text-muted-foreground" data-testid="bi-filter-pending">{{ t('components.bi.filter.pending') }}</p>

    <div v-if="fieldNames.length" class="flex flex-wrap items-center gap-1">
      <span class="mr-1 text-xs text-muted-foreground">{{ t('pages.dataBrowser.fieldChips') }}</span>
      <Button v-for="name in fieldNames" :key="name" size="xs" variant="ghost" data-chip="field" @click="emit('append-field', name)">
        {{ name }}
      </Button>
    </div>

    <div v-if="history.length" class="flex flex-wrap items-center gap-1">
      <span class="mr-1 text-xs text-muted-foreground">{{ t('pages.dataBrowser.recentFilters') }}</span>
      <Button v-for="item in history" :key="item" size="xs" variant="outline" data-chip="history" @click="emit('use-history', item)">
        {{ item }}
      </Button>
    </div>

    <!-- 联动筛选 chip：点击图元产生，可单独移除 -->
    <div v-if="chips.length" class="flex flex-wrap items-center gap-2" data-testid="bi-chips">
      <span v-for="chip in chips" :key="chip.id" class="inline-flex items-center gap-1 rounded-full border border-border bg-muted px-2.5 py-1 text-xs" data-chip="link">
        {{ chip.label }}
        <button type="button" class="rounded-full p-0.5 hover:bg-accent" :aria-label="t('common.remove')" data-chip-remove @click="emit('remove-chip', chip.id)">
          <XIcon class="size-3" />
        </button>
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
// BiFilterBar：时间范围 + 全局 filter + 联动 chip（planv6.7 §4.1，F5 / F6 / F7）。
// 只负责输入与展示；条件的合成在 bi-filter.ts，请求在各 widget。
import { useI18n } from 'vue-i18n'
import { XIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { RANGE_KEYS, type FilterChip, type RangeKey } from '@/utils/bi-filter'

defineProps<{
  range: RangeKey
  timeField: string
  /** 可选时间字段：created_at / updated_at 与已登记 date 字段；只有一个时隐藏下拉。 */
  dateFields: string[]
  filter: string
  history: string[]
  chips: FilterChip[]
  fieldNames: string[]
  /** 有任何已应用的条件时显示「清除条件」。 */
  dirty: boolean
  /** 输入框里的表达式与已应用的不一致：提示用户还没按「应用」。 */
  pending?: boolean
  /** 条件本身导致的失败（后端拒绝 filter / 超过扫描上限），由页面判定后集中展示。 */
  error?: string
}>()

const emit = defineEmits<{
  'update:range': [key: RangeKey]
  'update:timeField': [name: string]
  'update:filter': [value: string]
  apply: []
  clear: []
  'append-field': [name: string]
  'use-history': [filter: string]
  'remove-chip': [id: string]
}>()

const { t } = useI18n()

/** ToggleGroup 取消选中时会回传空值：时间范围必须始终有一个选中项，空值忽略。 */
function onRange(value: unknown) {
  if (typeof value === 'string' && (RANGE_KEYS as string[]).includes(value)) emit('update:range', value as RangeKey)
}
</script>
